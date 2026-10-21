package runtime

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/aggregatefile"
	"github.com/nobiyou/wx_channel_insight/internal/aggregatepg"
	"github.com/nobiyou/wx_channel_insight/internal/edge"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
	"github.com/nobiyou/wx_channel_insight/internal/ingestfile"
	"github.com/nobiyou/wx_channel_insight/internal/ingestpg"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	"github.com/nobiyou/wx_channel_insight/internal/insightaifile"
	"github.com/nobiyou/wx_channel_insight/internal/insightaipg"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
	"github.com/nobiyou/wx_channel_insight/internal/insightpg"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	"github.com/nobiyou/wx_channel_insight/internal/monitoringdemo"
	"github.com/nobiyou/wx_channel_insight/internal/monitoringfile"
	"github.com/nobiyou/wx_channel_insight/internal/monitoringpg"
	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	"github.com/nobiyou/wx_channel_insight/internal/normalizefile"
	"github.com/nobiyou/wx_channel_insight/internal/normalizepg"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/prompt"
	"github.com/nobiyou/wx_channel_insight/internal/promptfile"
	"github.com/nobiyou/wx_channel_insight/internal/promptpg"
	"github.com/nobiyou/wx_channel_insight/internal/rediscache"
	"github.com/nobiyou/wx_channel_insight/internal/sqlite"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspacedemo"
	"github.com/nobiyou/wx_channel_insight/internal/workspacefile"
	"github.com/nobiyou/wx_channel_insight/internal/workspacepg"
)

type DashboardRuntime struct {
	DashboardService           *insight.DashboardService
	ReadModelService           *insight.ReadModelRefreshService
	ReadQueryService           *insight.ReadModelService
	PostgresClient             *postgres.Client
	SQLiteClient               *sqlite.Client
	ReportSummaryService       insight.ReportSummaryRepository
	DailyReportService         insight.DailyReportRepository
	RankingService             insight.RankingRepository
	RecommendationService      *insight.RecommendationService
	CommentInsightService      insight.CommentInsightRepository
	DataCenterExclusionService insight.DataCenterExclusionRepository
	DataCenterService          *insight.DataCenterService
	ReadCache                  *insight.ReadCache
	RedisClient                *rediscache.Client
	RuntimeStatus              Status
	IngestService              *ingest.Service
	NormalizationService       *normalize.Service
	AggregationService         *insight.AggregationService
	WorkspaceService           *workspace.DefaultWorkspaceService
	ContextResolver            *workspace.ContextResolver
	MonitoringRepository       monitoring.Repository
	MonitoringService          *monitoring.Service
	AccountRepository          insight.AccountRepository
	AccountService             *insight.AccountService
	CommentRepository          commentRepository
	PromptRepository           prompt.Repository
	AIServiceRepository        insightai.ServiceSettingsRepository
	AIAnalysisRepository       insight.AIAnalysisRepository
	Close                      func()
}

type commentRepository interface {
	ingest.RawCommentRepository
	ingest.VideoCommentSyncRepository
}

func BuildDashboardRepository(cfg Config, postgresClient *postgres.Client) (insight.DashboardRepository, error) {
	switch cfg.DashboardStore.Mode {
	case StoreModeMemory:
		return insightdemo.NewDashboardRepository(), nil
	case StoreModeSQLite:
		seed, err := insightdemo.NewDashboardRepository().GetDashboard(context.Background(), "")
		if err != nil {
			return nil, err
		}
		return sqlite.NewDashboardRepository(cfg.sqliteClient, seed)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for dashboard store mode")
		}
		seedRepo := insightdemo.NewDashboardRepository()
		seed, err := seedRepo.GetDashboard(context.Background(), "")
		if err != nil {
			return nil, err
		}
		return insightpg.NewDashboardRepository(postgresClient, seed)
	}

	seedRepo := insightdemo.NewDashboardRepository()
	seed, err := seedRepo.GetDashboard(context.Background(), "")
	if err != nil {
		return nil, err
	}

	storePath, err := ResolveStorePath(cfg.DashboardStore.Path)
	if err != nil {
		return nil, err
	}
	return insightfile.NewDashboardRepository(storePath, seed)
}

func BuildReportSummaryRepository(cfg Config, postgresClient *postgres.Client) (insight.ReportSummaryRepository, error) {
	seedRepo := insightdemo.NewDashboardRepository()
	seedDashboard, err := seedRepo.GetDashboard(context.Background(), "")
	if err != nil {
		return nil, err
	}
	seed := seedDashboard.ReportSummary

	switch cfg.ReportSummaryStore.Mode {
	case StoreModeMemory:
		return insightdemo.NewReportSummaryRepository(seed), nil
	case StoreModeSQLite:
		return sqlite.NewReportSummaryRepository(cfg.sqliteClient, seed)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for report summary store mode")
		}
		return insightpg.NewReportSummaryRepository(postgresClient, seed)
	}

	storePath, err := ResolveStorePath(cfg.ReportSummaryStore.Path)
	if err != nil {
		return nil, err
	}
	return insightfile.NewReportSummaryRepository(storePath, seed)
}

func BuildDailyReportRepository(cfg Config, postgresClient *postgres.Client) (insight.DailyReportRepository, error) {
	switch cfg.ReportSummaryStore.Mode {
	case StoreModeMemory:
		return insightdemo.NewDailyReportRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewDailyReportRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for daily report store mode")
		}
		return insightpg.NewDailyReportRepository(postgresClient)
	default:
		storePath, err := resolveSiblingStorePath(cfg.ReportSummaryStore.Path, "daily-reports.json")
		if err != nil {
			return nil, err
		}
		return insightfile.NewDailyReportRepository(storePath)
	}
}

func BuildRankingRepository(cfg Config, postgresClient *postgres.Client) (insight.RankingRepository, error) {
	switch cfg.RankingStore.Mode {
	case StoreModeMemory:
		seedRepo := insightdemo.NewDashboardRepository()
		seedDashboard, err := seedRepo.GetDashboard(context.Background(), "")
		if err != nil {
			return nil, err
		}
		return insightdemo.NewRankingRepository(seedDashboard.HotVideos, seedDashboard.HotTopics), nil
	case StoreModeSQLite:
		return sqlite.NewRankingRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for ranking store mode")
		}
		return insightpg.NewRankingRepository(postgresClient)
	}

	storePath, err := ResolveStorePath(cfg.RankingStore.Path)
	if err != nil {
		return nil, err
	}
	return insightfile.NewRankingRepository(storePath)
}

func BuildCommentInsightRepository(cfg Config, postgresClient *postgres.Client) (insight.CommentInsightRepository, error) {
	seedRepo := insightdemo.NewDashboardRepository()
	seedDashboard, err := seedRepo.GetDashboard(context.Background(), "")
	if err != nil {
		return nil, err
	}

	switch cfg.CommentInsightStore.Mode {
	case StoreModeMemory:
		return insightdemo.NewCommentInsightRepository(seedDashboard.CommentInsights), nil
	case StoreModeSQLite:
		return sqlite.NewCommentInsightRepository(cfg.sqliteClient, seedDashboard.CommentInsights)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for comment insight store mode")
		}
		return insightpg.NewCommentInsightRepository(postgresClient, seedDashboard.CommentInsights)
	}

	storePath, err := ResolveStorePath(cfg.CommentInsightStore.Path)
	if err != nil {
		return nil, err
	}
	return insightfile.NewCommentInsightRepository(storePath, seedDashboard.CommentInsights)
}

func BuildDataCenterExclusionRepository(cfg Config, postgresClient *postgres.Client) (insight.DataCenterExclusionRepository, error) {
	store := cfg.DataCenterExclusionStore
	if store.Mode == "" {
		store.Mode = cfg.DashboardStore.Mode
	}
	if store.Mode == "" {
		store.Mode = StoreModeFile
	}
	if store.Path == "" {
		store.Path = filepath.Join("var", "data-center-exclusions.json")
	}
	switch store.Mode {
	case StoreModeMemory:
		return insightdemo.NewDataCenterExclusionRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewDataCenterExclusionRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for data center exclusion store mode")
		}
		return insightpg.NewDataCenterExclusionRepository(postgresClient)
	default:
		storePath, err := ResolveStorePath(store.Path)
		if err != nil {
			return nil, err
		}
		return insightfile.NewDataCenterExclusionRepository(storePath)
	}
}

// Curation follows the existing data-management store, without a second backend
// preference. It owns operator decisions, independently of source observations.
func BuildVideoCurationRepository(cfg Config, postgresClient *postgres.Client) (insight.VideoCurationRepository, error) {
	store := cfg.DataCenterExclusionStore
	if store.Mode == "" {
		store.Mode = cfg.DashboardStore.Mode
	}
	if store.Mode == StoreModeSQLite {
		return sqlite.NewVideoCurationRepository(cfg.sqliteClient)
	}
	if store.Mode == StoreModePostgres {
		return insightpg.NewVideoCurationRepository(postgresClient)
	}
	if store.Mode == StoreModeMemory {
		return insightfile.NewVideoCurationRepository("")
	}
	if store.Path == "" {
		store.Path = filepath.Join("var", "data-center-exclusions.json")
	}
	path, err := ResolveStorePath(store.Path + ".video-curations.json")
	if err != nil {
		return nil, err
	}
	return insightfile.NewVideoCurationRepository(path)
}

func BuildRecommendationRepository(cfg Config, postgresClient *postgres.Client) (insight.RecommendationRepository, error) {
	store := cfg.RecommendationStore
	if store.Mode == "" {
		store.Mode = cfg.DashboardStore.Mode
	}
	if store.Mode == "" {
		store.Mode = StoreModeFile
	}
	if store.Path == "" {
		store.Path = filepath.Join("var", "recommendations.json")
	}

	switch store.Mode {
	case StoreModeMemory:
		return insightdemo.NewRecommendationRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewRecommendationRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for recommendation store mode")
		}
		return insightpg.NewRecommendationRepository(postgresClient)
	}

	storePath, err := ResolveStorePath(store.Path)
	if err != nil {
		return nil, err
	}
	return insightfile.NewRecommendationRepository(storePath)
}

func BuildWorkspaceRepository(cfg Config, postgresClient *postgres.Client) (workspace.DefaultWorkspaceRepository, error) {
	switch cfg.WorkspaceStore.Mode {
	case StoreModeMemory:
		return workspacedemo.NewDefaultWorkspaceRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewWorkspaceRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for workspace store mode")
		}
		return workspacepg.NewDefaultWorkspaceRepository(postgresClient)
	}

	storePath, err := ResolveStorePath(cfg.WorkspaceStore.Path)
	if err != nil {
		return nil, err
	}
	return workspacefile.NewDefaultWorkspaceRepository(storePath)
}

func BuildIngestRepository(cfg Config, postgresClient *postgres.Client, sqliteClients ...*sqlite.Client) (ingest.Repository, error) {
	switch cfg.IngestStore.Mode {
	case StoreModeMemory:
		return ingestdemo.NewRepository(), nil
	case StoreModeSQLite:
		client := cfg.sqliteClient
		if len(sqliteClients) > 0 {
			client = sqliteClients[0]
		}
		return sqlite.NewIngestRepository(client)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for ingest store mode")
		}
		return ingestpg.NewRepository(postgresClient)
	}

	storePath, err := ResolveStorePath(cfg.IngestStore.Path)
	if err != nil {
		return nil, err
	}
	return ingestfile.NewRepository(storePath)
}

func BuildNormalizationRepository(cfg Config, postgresClient *postgres.Client) (normalize.Repository, error) {
	switch cfg.IngestStore.Mode {
	case StoreModeMemory:
		return normalize.NewMemoryRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewNormalizationRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for normalization store mode")
		}
		return normalizepg.NewRepository(postgresClient)
	}

	storePath, err := resolveNormalizationStorePath(cfg)
	if err != nil {
		return nil, err
	}
	return normalizefile.NewRepository(storePath)
}

func BuildAggregationRepository(cfg Config, postgresClient *postgres.Client) (aggregate.Repository, error) {
	switch cfg.IngestStore.Mode {
	case StoreModeMemory:
		return aggregate.NewMemoryRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewAggregationRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for aggregation store mode")
		}
		return aggregatepg.NewRepository(postgresClient)
	}

	storePath, err := resolveAggregationStorePath(cfg)
	if err != nil {
		return nil, err
	}
	return aggregatefile.NewRepository(storePath)
}

func BuildMonitoringRepository(cfg Config, postgresClient *postgres.Client) (monitoring.Repository, error) {
	switch cfg.MonitoringStore.Mode {
	case StoreModeMemory:
		return monitoringdemo.NewRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewMonitoringRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for monitoring store mode")
		}
		return monitoringpg.NewRepository(postgresClient)
	}

	storePath := cfg.MonitoringStore.Path
	if storePath == "" {
		storePath = filepath.Join("var", "monitoring-tasks.json")
	}
	storePath, err := ResolveStorePath(storePath)
	if err != nil {
		return nil, err
	}
	return monitoringfile.NewRepository(storePath)
}

func BuildAccountRepository(cfg Config, postgresClient *postgres.Client) (insight.AccountRepository, error) {
	switch cfg.DashboardStore.Mode {
	case StoreModeMemory:
		return insightdemo.NewAccountRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewAccountRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for account store mode")
		}
		return insightpg.NewAccountRepository(postgresClient)
	default:
		storePath, err := resolveSiblingStorePath(cfg.DashboardStore.Path, "accounts.json")
		if err != nil {
			return nil, err
		}
		return insightfile.NewAccountRepository(storePath)
	}
}

func BuildCommentRepository(cfg Config, postgresClient *postgres.Client) (commentRepository, error) {
	switch cfg.IngestStore.Mode {
	case StoreModeMemory:
		return ingestdemo.NewCommentRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewCommentRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for comment store mode")
		}
		return ingestpg.NewCommentRepository(postgresClient)
	default:
		storePath, err := resolveSiblingStorePath(cfg.IngestStore.Path, "comment-records.json")
		if err != nil {
			return nil, err
		}
		return ingestfile.NewCommentRepository(storePath)
	}
}

func BuildPromptRepository(cfg Config, postgresClient *postgres.Client) (prompt.Repository, error) {
	store := cfg.PromptStore
	if store.Mode == "" {
		store.Mode = cfg.DashboardStore.Mode
	}
	if store.Mode == "" {
		store.Mode = StoreModeFile
	}
	if store.Path == "" {
		store.Path = filepath.Join("var", "prompt-settings.json")
	}
	switch store.Mode {
	case StoreModeMemory:
		return prompt.NewMemoryRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewPromptRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for prompt store mode")
		}
		return promptpg.NewRepository(postgresClient)
	default:
		storePath, err := ResolveStorePath(store.Path)
		if err != nil {
			return nil, err
		}
		return promptfile.NewRepository(storePath)
	}
}

func BuildAIServiceRepository(cfg Config, postgresClient *postgres.Client) (insightai.ServiceSettingsRepository, error) {
	store := cfg.AIServiceStore
	if store.Mode == "" {
		store.Mode = cfg.DashboardStore.Mode
	}
	if store.Mode == "" {
		store.Mode = StoreModeFile
	}
	if store.Path == "" {
		store.Path = filepath.Join("var", "ai-service-settings.json")
	}
	var codec *insightai.SecretCodec
	if strings.TrimSpace(cfg.AIConfigEncryptionKey) != "" {
		var err error
		codec, err = insightai.NewSecretCodec(cfg.AIConfigEncryptionKey)
		if err != nil {
			return nil, err
		}
	}
	switch store.Mode {
	case StoreModeMemory:
		return insightai.NewMemoryServiceSettingsRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewAIServiceRepository(cfg.sqliteClient, codec)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for AI service store mode")
		}
		return insightaipg.NewRepository(postgresClient, codec)
	default:
		storePath, err := ResolveStorePath(store.Path)
		if err != nil {
			return nil, err
		}
		return insightaifile.NewRepository(storePath, codec)
	}
}

func BuildAIAnalysisRepository(cfg Config, postgresClient *postgres.Client) (insight.AIAnalysisRepository, error) {
	store := cfg.AIAnalysisStore
	if store.Mode == "" {
		store.Mode = cfg.DashboardStore.Mode
	}
	if store.Mode == "" {
		store.Mode = StoreModeFile
	}
	if store.Path == "" {
		store.Path = filepath.Join("var", "ai-analysis-runs.json")
	}
	switch store.Mode {
	case StoreModeMemory:
		return insightdemo.NewAIAnalysisRepository(), nil
	case StoreModeSQLite:
		return sqlite.NewAIAnalysisRepository(cfg.sqliteClient)
	case StoreModePostgres:
		if postgresClient == nil {
			return nil, errors.New("postgres client is required for AI analysis store mode")
		}
		return insightpg.NewAIAnalysisRepository(postgresClient)
	default:
		storePath, err := ResolveStorePath(store.Path)
		if err != nil {
			return nil, err
		}
		return insightfile.NewAIAnalysisRepository(storePath)
	}
}

func resolveSiblingStorePath(basePath, filename string) (string, error) {
	if strings.TrimSpace(basePath) == "" {
		basePath = filepath.Join("var", "state.json")
	}
	return ResolveStorePath(filepath.Join(filepath.Dir(basePath), filename))
}

func resolveNormalizationStorePath(cfg Config) (string, error) {
	if cfg.IngestStore.Path == "" {
		return filepath.Abs(filepath.Join("var", "normalized-snapshots.json"))
	}
	baseDir := filepath.Dir(cfg.IngestStore.Path)
	if baseDir == "" || baseDir == "." {
		baseDir = "var"
	}
	return filepath.Abs(filepath.Join(baseDir, "normalized-snapshots.json"))
}

func resolveAggregationStorePath(cfg Config) (string, error) {
	if cfg.IngestStore.Path == "" {
		return filepath.Abs(filepath.Join("var", "aggregated-metrics.json"))
	}
	baseDir := filepath.Dir(cfg.IngestStore.Path)
	if baseDir == "" || baseDir == "." {
		baseDir = "var"
	}
	return filepath.Abs(filepath.Join(baseDir, "aggregated-metrics.json"))
}

func ResolveStorePath(path string) (string, error) {
	if path == "" {
		path = filepath.Join("var", "state.json")
	}
	return filepath.Abs(path)
}

func BuildDashboardRuntime(cfg Config) (*DashboardRuntime, error) {
	sqliteClient, err := BuildSQLiteClient(cfg)
	if err != nil {
		return nil, err
	}
	cfg.sqliteClient = sqliteClient
	var postgresClient *postgres.Client
	if usesStoreMode(cfg, StoreModePostgres) {
		postgresClient, err = BuildPostgresClient(cfg)
	}
	if err != nil {
		sqliteClient.Close()
		return nil, err
	}
	redisClient, err := BuildRedisClient(cfg)
	if err != nil {
		sqliteClient.Close()
		if postgresClient != nil {
			postgresClient.Close()
		}
		return nil, err
	}
	cleanup := func() {
		sqliteClient.Close()
		if redisClient != nil {
			redisClient.Close()
		}
		if postgresClient != nil {
			postgresClient.Close()
		}
	}

	dashboardRepo, err := BuildDashboardRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	reportSummaryRepo, err := BuildReportSummaryRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	dailyReportRepo, err := BuildDailyReportRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	rankingRepo, err := BuildRankingRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	commentInsightRepo, err := BuildCommentInsightRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	dataCenterExclusionRepo, err := BuildDataCenterExclusionRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	recommendationRepo, err := BuildRecommendationRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	workspaceRepo, err := BuildWorkspaceRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	ingestRepo, err := BuildIngestRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	normalizationRepo, err := BuildNormalizationRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	aggregationRepo, err := BuildAggregationRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	monitoringRepo, err := BuildMonitoringRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	accountRepo, err := BuildAccountRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	commentRepo, err := BuildCommentRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	promptRepo, err := BuildPromptRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	aiServiceRepo, err := BuildAIServiceRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}
	aiAnalysisRepo, err := BuildAIAnalysisRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, err
	}

	workspaceService := workspace.NewDefaultWorkspaceService(workspaceRepo)
	contextResolver := workspace.NewContextResolver(workspaceService, workspace.ContextResolverConfig{
		DefaultHubUserID:   cfg.DefaultHubUserID,
		DefaultDeviceCount: cfg.DefaultDeviceCount,
	})
	ingestService := ingest.NewService(ingestRepo, ingest.ServiceConfig{
		DefaultHubUserID:           cfg.DefaultHubUserID,
		WXFeedListClient:           ingest.WXFeedListClient{BaseURL: cfg.WXChannelBaseURL},
		WXCommentClient:            ingest.WXCommentExportClient{BaseURL: cfg.WXChannelBaseURL},
		CommentDataRoot:            cfg.WXCommentDataRoot,
		EnableLiveChecks:           true,
		RawCommentRepository:       commentRepo,
		VideoCommentSyncRepository: commentRepo,
	})
	accountService := insight.NewAccountService(insight.AccountServiceConfig{
		Repository:               accountRepo,
		WorkspaceContextResolver: contextResolver,
	})
	accountQuery := ingest.SummaryQuery{HubUserID: cfg.DefaultHubUserID, DeviceCount: cfg.DefaultDeviceCount}
	ingestService.SetEventObserver(func(ctx context.Context, query ingest.SummaryQuery, events []ingest.Event) error {
		return syncObservedAccounts(ctx, accountService, query, events)
	})
	existingEvents, err := ingestService.ListEvents(context.Background(), accountQuery)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("load existing account observations: %w", err)
	}
	if err := syncObservedAccounts(context.Background(), accountService, accountQuery, existingEvents); err != nil {
		cleanup()
		return nil, fmt.Errorf("sync existing account observations: %w", err)
	}
	normalizationService := normalize.NewService(normalizationRepo, normalize.ServiceConfig{
		WorkspaceContextResolver: contextResolver,
		IngestEvents:             ingestService,
	})
	aggregationService := insight.NewAggregationService(aggregationRepo, insight.AggregationServiceConfig{
		Normalizer: normalizationService,
	})
	monitoringService, err := monitoring.NewService(monitoringRepo, monitoring.ServiceConfig{
		DefaultHubUserID:   cfg.DefaultHubUserID,
		DefaultDeviceCount: cfg.DefaultDeviceCount,
		BindingResolver: func(ctx context.Context, hubUserID string, refs []monitoring.BindingRef) ([]monitoring.TaskTarget, error) {
			bindings, err := ingestService.ListBindings(ctx, ingest.SummaryQuery{HubUserID: hubUserID})
			if err != nil {
				return nil, err
			}
			byRef := make(map[string]ingest.SourceBinding, len(bindings))
			all := make([]monitoring.TaskTarget, 0, len(bindings))
			for _, binding := range bindings {
				binding = ingest.NormalizeSourceBinding(binding)
				key := binding.Source + "\x00" + ingest.BindingTargetKey(binding)
				byRef[key] = binding
				all = append(all, monitoring.TaskTarget{
					BindingRef:  monitoring.BindingRef{Source: binding.Source, TargetKey: ingest.BindingTargetKey(binding)},
					Label:       binding.Source,
					DisplayName: firstBindingDisplayName(binding),
				})
			}
			if len(refs) == 0 {
				return all, nil
			}
			resolved := make([]monitoring.TaskTarget, 0, len(refs))
			for _, ref := range refs {
				normalized, err := monitoring.NormalizeBindingRef(ref)
				if err != nil {
					return nil, err
				}
				binding, ok := byRef[normalized.Source+"\x00"+normalized.TargetKey]
				if !ok {
					return nil, fmt.Errorf("saved monitoring target %s[%s] was not found", normalized.Source, normalized.TargetKey)
				}
				resolved = append(resolved, monitoring.TaskTarget{
					BindingRef:  normalized,
					Label:       binding.Source,
					DisplayName: firstBindingDisplayName(binding),
				})
			}
			return resolved, nil
		},
		AccountResolver: func(ctx context.Context, hubUserID string, targets []monitoring.TaskTarget) ([]monitoring.TaskTarget, error) {
			if len(targets) == 0 {
				resolved := make([]monitoring.TaskTarget, 0)
				for page := 1; ; page++ {
					accounts, err := accountService.List(ctx, insight.AccountQuery{
						HubUserID: hubUserID, DeviceCount: cfg.DefaultDeviceCount, ContextSource: "jobs",
						Page: page, PageSize: 100, Sort: "updated_at", Direction: "desc",
					})
					if err != nil {
						return nil, fmt.Errorf("list monitoring accounts: %w", err)
					}
					for _, account := range accounts.Items {
						normalized, err := monitoring.NormalizeTaskTarget(monitoring.TaskTarget{
							TargetKind: monitoring.TargetKindAccount,
							AccountID:  account.ID,
							Label:      "account",
							DisplayName: firstNonEmptyText(
								account.ChannelName,
								account.ChannelUsername,
							),
						})
						if err != nil {
							return nil, fmt.Errorf("resolve monitoring account %s: %w", account.ID, err)
						}
						resolved = append(resolved, normalized)
					}
					if accounts.Page >= accounts.TotalPages {
						return resolved, nil
					}
				}
			}
			resolved := make([]monitoring.TaskTarget, 0, len(targets))
			for _, target := range targets {
				account, err := accountService.Get(ctx, insight.AccountQuery{
					HubUserID: hubUserID, DeviceCount: cfg.DefaultDeviceCount, ContextSource: "jobs",
				}, target.AccountID)
				if err != nil {
					return nil, fmt.Errorf("resolve monitoring account %s: %w", target.AccountID, err)
				}
				if account.IdentityStatus != insight.AccountIdentityVerified || strings.TrimSpace(account.ChannelUsername) == "" {
					return nil, fmt.Errorf("monitoring account %s identity is incomplete", target.AccountID)
				}
				normalized, err := monitoring.NormalizeTaskTarget(target)
				if err != nil {
					return nil, err
				}
				normalized.Label = "account"
				normalized.DisplayName = firstNonEmptyText(account.ChannelName, account.ChannelUsername)
				resolved = append(resolved, normalized)
			}
			return resolved, nil
		},
	})
	if err != nil {
		cleanup()
		return nil, err
	}
	recommendationService := insight.NewRecommendationService(insight.RecommendationServiceConfig{
		DataProvider:             ingestService,
		Repository:               recommendationRepo,
		CommentInsightRepository: commentInsightRepo,
		WorkspaceContextResolver: contextResolver,
	})
	baseSeedRepo := insightdemo.NewDashboardRepository()
	baseSeed, err := baseSeedRepo.GetDashboard(context.Background(), "")
	if err != nil {
		cleanup()
		return nil, err
	}
	materializer := insight.NewMaterializer(baseSeed)
	readCache := insight.NewReadCache(redisClient, 15*time.Minute)
	dashboardService := insight.NewDashboardService(dashboardRepo, insight.DashboardServiceConfig{
		WorkspaceContextResolver:      contextResolver,
		IngestService:                 ingestService,
		Aggregator:                    aggregationService,
		Normalizer:                    normalizationService,
		ReportSummaryRepository:       reportSummaryRepo,
		DailyReportRepository:         dailyReportRepo,
		RankingRepository:             rankingRepo,
		CommentInsightRepository:      commentInsightRepo,
		DataCenterExclusionRepository: dataCenterExclusionRepo,
		RecommendationService:         recommendationService,
		ReadCache:                     readCache,
		Materializer:                  materializer,
	})
	readQueryService := insight.NewReadModelService(
		dashboardService,
		insight.WithReportSummaryRepository(reportSummaryRepo),
		insight.WithRankingRepository(rankingRepo),
		insight.WithCommentInsightRepository(commentInsightRepo),
		insight.WithRecommendationService(recommendationService),
		insight.WithWorkspaceContextResolver(contextResolver),
		insight.WithReadCache(readCache),
		insight.WithDataCenterExclusionRepository(dataCenterExclusionRepo),
	)
	videoCurationRepo, err := BuildVideoCurationRepository(cfg, postgresClient)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("build video curation repository: %w", err)
	}
	dataCenterService := insight.NewDataCenterService(insight.DataCenterServiceConfig{
		VideoEvidenceTTL:         30 * time.Second,
		CurationRepository:       videoCurationRepo,
		DataProvider:             ingestService,
		AccountProvider:          accountRepo,
		RawCommentProvider:       commentRepo,
		WorkspaceContextResolver: contextResolver,
		ExclusionRepository:      dataCenterExclusionRepo,
	})
	readModelService := insight.NewReadModelRefreshService(insight.ReadModelRefreshServiceConfig{
		Repository:               dashboardRepo,
		ReportSummaryRepository:  reportSummaryRepo,
		DailyReportRepository:    dailyReportRepo,
		RankingRepository:        rankingRepo,
		CommentInsightRepository: commentInsightRepo,
		RecommendationService:    recommendationService,
		ReadCache:                readCache,
		WorkspaceContextResolver: contextResolver,
		IngestService:            ingestService,
		Aggregator:               aggregationService,
		Normalizer:               normalizationService,
		Materializer:             materializer,
	})
	runtimeStatus := WithInfrastructureAvailability(BuildStatus(cfg), postgresClient != nil, redisClient != nil)
	runtimeStatus = WithReadCacheSnapshot(runtimeStatus, readCache.Snapshot())

	return &DashboardRuntime{
		DashboardService:           dashboardService,
		ReadModelService:           readModelService,
		ReadQueryService:           readQueryService,
		PostgresClient:             postgresClient,
		SQLiteClient:               sqliteClient,
		ReportSummaryService:       reportSummaryRepo,
		DailyReportService:         dailyReportRepo,
		RankingService:             rankingRepo,
		RecommendationService:      recommendationService,
		CommentInsightService:      commentInsightRepo,
		DataCenterExclusionService: dataCenterExclusionRepo,
		DataCenterService:          dataCenterService,
		ReadCache:                  readCache,
		RedisClient:                redisClient,
		RuntimeStatus:              runtimeStatus,
		IngestService:              ingestService,
		NormalizationService:       normalizationService,
		AggregationService:         aggregationService,
		WorkspaceService:           workspaceService,
		ContextResolver:            contextResolver,
		MonitoringRepository:       monitoringRepo,
		MonitoringService:          monitoringService,
		AccountRepository:          accountRepo,
		AccountService:             accountService,
		CommentRepository:          commentRepo,
		PromptRepository:           promptRepo,
		AIServiceRepository:        aiServiceRepo,
		AIAnalysisRepository:       aiAnalysisRepo,
		Close:                      cleanup,
	}, nil
}

func firstBindingDisplayName(binding ingest.SourceBinding) string {
	for _, value := range []string{binding.Username, binding.Keyword, binding.ObjectID, binding.Path, binding.ShareURL, binding.TargetKey} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return binding.Source
}

// AttachEdgeFeedTransport enables multi-LAN feed/search via Insight edge gateway.
func AttachEdgeFeedTransport(rt *DashboardRuntime, hub *edge.Hub, localBaseURL string) {
	if rt == nil || rt.IngestService == nil || hub == nil {
		return
	}
	rt.IngestService.SetWXFeedListTransport(&edge.FeedTransport{
		LocalBaseURL: localBaseURL,
		Hub:          hub,
	})
	rt.IngestService.SetWXCommentTransport(&edge.CommentTransport{
		LocalBaseURL: localBaseURL,
		Hub:          hub,
	})
}
