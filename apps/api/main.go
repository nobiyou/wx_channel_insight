package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/edge"
	"github.com/nobiyou/wx_channel_insight/internal/hubbridge"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	operatorflow "github.com/nobiyou/wx_channel_insight/internal/operator"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	runtimecfg "github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func main() {
	cfg, err := runtimecfg.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer runtimePack.Close()

	if runtimePack.RedisClient != nil {
		log.Printf("redis cache skeleton configured addr=%s db=%d", runtimePack.RedisClient.Addr(), runtimePack.RedisClient.DB())
	}
	if cfg.Postgres.Enabled() {
		log.Printf("postgres skeleton configured for team edition")
	}

	operatorRunner, err := operatorflow.NewService(runtimePack.IngestService, runtimePack.ReadModelService)
	if err != nil {
		log.Fatal(err)
	}
	accountMonitor, err := operatorflow.NewAccountMonitor(runtimePack.IngestService, runtimePack.AccountService)
	if err != nil {
		log.Fatal(err)
	}
	operatorRunner.SetAccountMonitor(accountMonitor)
	if runtimePack.PostgresClient != nil {
		cycleLocker, err := postgres.NewAdvisoryCycleLocker(runtimePack.PostgresClient)
		if err != nil {
			log.Fatal(err)
		}
		operatorRunner.SetCycleLocker(cycleLocker)
	}
	runtimePack.MonitoringService.SetOperatorRunner(operatorRunner)

	var edgeHub *edge.Hub
	if cfg.Edge.Enabled {
		edgeHub = edge.NewHub(edge.WithSecret(cfg.Edge.Secret))
		log.Printf("edge gateway enabled path=/ws/client (set LAN wx_channel cloud_hub_url to this server)")
		// Prefer local WXChannelBaseURL when present; otherwise/also use edge nodes.
		runtimecfg.AttachEdgeFeedTransport(runtimePack, edgeHub, cfg.WXChannelBaseURL)
		log.Printf("ingest feed transport: local_base=%q edge=on", cfg.WXChannelBaseURL)
	}

	srv := newServerWithOperator(
		runtimePack.DashboardService,
		runtimePack.ReadModelService,
		runtimePack.ReadQueryService,
		runtimePack.IngestService,
		operatorRunner,
		runtimePack.ContextResolver,
		runtimePack.RuntimeStatus,
	)
	srv.dataCenterService = runtimePack.DataCenterService
	srv.monitoringService = runtimePack.MonitoringService
	srv.accountService = runtimePack.AccountService
	srv.accountMonitor = accountMonitor
	if runtimePack.PostgresClient != nil {
		srv.metricRefreshLocker, err = postgres.NewAdvisoryCycleLocker(runtimePack.PostgresClient)
		if err != nil {
			log.Fatal(err)
		}
	}
	srv.readCache = runtimePack.ReadCache
	srv.jobsStatePath = cfg.JobsStatePath
	srv.webStaticRoot = cfg.WebStaticRoot
	srv.bridgeConfig = cfg.Bridge
	srv.edgeHub = edgeHub
	srv.wxChannelBaseURL = cfg.WXChannelBaseURL
	srv.promptRepository = runtimePack.PromptRepository
	srv.corsOrigins = cfg.CORSOrigins
	aiServiceManager, err := insightai.NewServiceSettingsManager(runtimePack.AIServiceRepository, insightai.ServiceSettings{
		Provider: insightai.ProviderOpenAI,
		BaseURL:  cfg.OpenAI.BaseURL,
		Model:    cfg.OpenAI.Model,
		Timeout:  cfg.OpenAI.Timeout,
		APIKey:   cfg.OpenAI.APIKey,
	})
	if err != nil {
		log.Fatalf("configure AI service settings: %v", err)
	}
	srv.aiServiceManager = aiServiceManager
	srv.dailyReportRepo = runtimePack.DailyReportService
	srv.aiAnalysisRepository = runtimePack.AIAnalysisRepository
	analysisClient := insightai.NewClient(insightai.Config{
		APIKey:                 cfg.OpenAI.APIKey,
		BaseURL:                cfg.OpenAI.BaseURL,
		Model:                  cfg.OpenAI.Model,
		Timeout:                cfg.OpenAI.Timeout,
		PromptRepository:       runtimePack.PromptRepository,
		PromptOwnerID:          cfg.DefaultHubUserID,
		ServiceSettingsManager: aiServiceManager,
		ServiceOwnerID:         cfg.DefaultHubUserID,
	}, nil)
	srv.aiAnalysisRunner = analysisClient
	automation := newOperationsAutomation(context.Background(), func(ctx context.Context, q insight.DataCenterQuery) error {
		_, _, err := srv.inspectOperations(ctx, q, true, true)
		return err
	}, func(workspaceID string, err error) {
		log.Printf("automatic operations inspection failed workspace=%s: %v", workspaceID, err)
	})
	defer automation.Close()
	runtimePack.ReadModelService.SetAfterRefresh(automation.Notify)
	if cfg.OpenAI.Enabled() {
		log.Printf("optional AI data analysis enabled model=%s", cfg.OpenAI.Model)
	} else {
		log.Printf("optional AI data analysis awaits workspace configuration")
	}

	if cfg.Bridge.HubSessionEnabled() {
		hubClient, err := hubbridge.NewHTTPClient(cfg.Bridge.HubBaseURL, nil)
		if err != nil {
			log.Fatal(err)
		}
		srv.hubSessionResolver = hubClient
		log.Printf("hub session bridge configured base_url=%s", cfg.Bridge.HubBaseURL)
	}
	if repoRoot, err := os.Getwd(); err == nil {
		srv.trustedRuntimeEvidenceRoot = repoRoot
	}

	log.Printf("wx_channel_insight api listening on %s", cfg.APIAddr)
	server := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func withCORS(next http.Handler, allowedOrigins []string) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
		if origin != "" {
			if _, ok := allowed[origin]; !ok {
				if r.Method == http.MethodOptions {
					http.Error(w, "origin is not allowed", http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Hub-User-ID, X-Hub-Device-Count, X-Insight-Bridge-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
