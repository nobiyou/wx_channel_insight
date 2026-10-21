package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/edge"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	operatorflow "github.com/nobiyou/wx_channel_insight/internal/operator"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	runtimecfg "github.com/nobiyou/wx_channel_insight/internal/runtime"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

func main() {
	cfg, err := runtimecfg.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if err := run(os.Args[1:], cfg, defaultLogf); err != nil {
		var silentErr silentExitError
		if errors.As(err, &silentErr) {
			os.Exit(1)
		}
		log.Fatal(err)
	}
}

type silentExitError struct {
	err error
}

func (e silentExitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func defaultLogf(format string, args ...any) {
	fmt.Println(fmt.Sprintf(format, args...))
}

func run(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	command := "status"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		command = strings.TrimSpace(args[0])
	}

	switch command {
	case "migrate-file-store":
		return runFileStoreMigration(args[1:], cfg, logf)
	case "status":
		status := runtimecfg.BuildStatus(cfg)
		logf(
			"jobs status phase=%s dashboard=%s rankings=%s comment_insights=%s report_summary=%s workspace=%s ingest=%s postgres=%t redis=%t jobs=%s",
			status.Phase,
			status.DashboardStoreMode,
			status.RankingStoreMode,
			status.CommentInsightStoreMode,
			status.ReportSummaryStoreMode,
			status.WorkspaceStoreMode,
			status.IngestStoreMode,
			status.PostgresConfigured,
			status.RedisConfigured,
			status.JobsRuntimeState,
		)
		return nil
	case "bootstrap-schema":
		postgresClient, err := runtimecfg.BuildPostgresClient(cfg)
		if err != nil {
			return err
		}
		if postgresClient == nil {
			return fmt.Errorf("bootstrap-schema requires INSIGHT_POSTGRES_DSN")
		}
		defer postgresClient.Close()
		if err := postgres.RunMigrationsFromDir(postgresClient.DB(), "migrations/postgres"); err != nil {
			return err
		}
		logf("postgres phase-a bootstrap schema ensured from migrations/postgres")
		return nil
	case "check-migrations":
		dir, err := resolveMigrationsDir()
		if err != nil {
			return err
		}
		if _, err := os.Stat(dir); err != nil {
			return err
		}
		logf("postgres migrations directory is present: %s", dir)
		return nil
	case "save-trusted-runtime-evidence":
		return runSaveTrustedRuntimeEvidence(args[1:], cfg, logf)
	case "import-wx-comment-export":
		return runImportWXCommentExport(args[1:], cfg, logf)
	case "save-wx-comment-export-binding":
		return runSaveWXCommentExportBinding(args[1:], cfg, logf)
	case "search-wx-feed-accounts":
		return runSearchWXFeedAccounts(args[1:], cfg, logf)
	case "search-wx-videos":
		return runSearchWXVideos(args[1:], cfg, logf)
	case "import-wx-feed-list":
		return runImportWXFeedList(args[1:], cfg, logf)
	case "import-wx-video-search":
		return runImportWXVideoSearch(args[1:], cfg, logf)
	case "backfill-wx-video-covers":
		return runBackfillWXVideoCovers(args[1:], cfg, logf)
	case "save-wx-feed-list-binding":
		return runSaveWXFeedListBinding(args[1:], cfg, logf)
	case "save-wx-video-search-binding":
		return runSaveWXVideoSearchBinding(args[1:], cfg, logf)
	case "sync-wx-comment-export":
		return runSyncWXCommentExport(args[1:], cfg, logf)
	case "sync-wx-feed-list":
		return runSyncWXFeedList(args[1:], cfg, logf)
	case "sync-wx-video-search":
		return runSyncWXVideoSearch(args[1:], cfg, logf)
	case "sync-saved-bindings":
		return runSyncSavedBindings(args[1:], cfg, logf)
	case "run-operator-cycle":
		return runOperatorCycle(args[1:], cfg, logf)
	case "run-operator-loop":
		return runOperatorLoop(args[1:], cfg, logf)
	case "show-operator-runs":
		return runShowOperatorRuns(args[1:], cfg, logf)
	case "show-ingest-summary":
		return runShowIngestSummary(args[1:], cfg, logf)
	case "show-ingest-runs":
		return runShowIngestRuns(args[1:], cfg, logf)
	case "show-ingest-sources":
		return runShowIngestSources(args[1:], cfg, logf)
	case "show-ingest-bindings":
		return runShowIngestBindings(args[1:], cfg, logf)
	case "retry-last-failed-ingest":
		return runRetryLastFailedIngest(args[1:], cfg, logf)
	case "normalize-ingest":
		return runNormalizeIngest(args[1:], cfg, logf)
	case "show-normalized-summary":
		return runShowNormalizedSummary(args[1:], cfg, logf)
	case "aggregate-metrics":
		return runAggregateMetrics(args[1:], cfg, logf)
	case "show-aggregate-summary":
		return runShowAggregateSummary(args[1:], cfg, logf)
	case "refresh-dashboard":
		return runRefreshDashboard(args[1:], cfg, logf)
	case "show-hot-videos":
		return runShowHotVideos(args[1:], cfg, logf)
	case "show-hot-topics":
		return runShowHotTopics(args[1:], cfg, logf)
	case "show-comment-insights":
		return runShowCommentInsights(args[1:], cfg, logf)
	case "show-report-summary":
		return runShowReportSummary(args[1:], cfg, logf)
	case "show-dashboard":
		return runShowDashboard(args[1:], cfg, logf)
	default:
		return fmt.Errorf("unknown jobs command: %s", command)
	}
}

func runImportWXCommentExport(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseCommentExportArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	summary, err := service.ImportWXCommentExport(context.Background(), ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	}, options.path)
	if err != nil {
		return err
	}

	logf(
		"jobs import-wx-comment-export success hub_user_id=%s path=%s total=%d latest_batch=%s latest_event=%s freshness=%s",
		summary.HubUserID,
		options.path,
		summary.TotalEvents,
		summary.LatestBatchID,
		summary.LastEventType,
		summary.FreshnessState,
	)
	return nil
}

func runSaveWXCommentExportBinding(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseCommentExportArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	summary, err := service.SaveWXCommentExportBinding(context.Background(), ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	}, options.path)
	if err != nil {
		return err
	}

	return writeJSONLog(logf, map[string]any{
		"status":            "ok",
		"source":            "wx_comment_export",
		"hubUserId":         summary.HubUserID,
		"path":              options.path,
		"freshnessState":    summary.FreshnessState,
		"bindingStates":     summary.BindingStates,
		"operatorReadiness": summary.OperatorReadiness,
	})
}

func runSearchWXFeedAccounts(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseFeedListSearchArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	items, err := service.SearchWXFeedAccounts(context.Background(), ingest.SummaryQuery{
		HubUserID: options.hubUserID,
	}, options.baseURL, options.keyword, "")
	if err != nil {
		return err
	}

	return writeJSONLog(logf, map[string]any{
		"status":  "ok",
		"keyword": options.keyword,
		"baseURL": options.baseURL,
		"items":   items,
	})
}

func runSearchWXVideos(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseFeedListSearchArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	result, err := service.SearchWXVideos(context.Background(), ingest.SummaryQuery{
		HubUserID: options.hubUserID,
	}, options.baseURL, options.keyword, "")
	if err != nil {
		return err
	}

	return writeJSONLog(logf, map[string]any{
		"status":     "ok",
		"keyword":    options.keyword,
		"baseURL":    options.baseURL,
		"nextMarker": result.NextMarker,
		"items":      result.Items,
	})
}

func runImportWXFeedList(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseFeedListArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	summary, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	}, options.baseURL, options.username, "")
	if err != nil {
		return err
	}

	logf(
		"jobs import-wx-feed-list success hub_user_id=%s username=%s total=%d latest_batch=%s latest_event=%s freshness=%s",
		summary.HubUserID,
		options.username,
		summary.TotalEvents,
		summary.LatestBatchID,
		summary.LastEventType,
		summary.FreshnessState,
	)
	return nil
}

func runImportWXVideoSearch(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseFeedListSearchArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	summary, err := service.ImportWXVideoSearch(context.Background(), ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	}, options.baseURL, options.keyword, "")
	if err != nil {
		return err
	}

	logf(
		"jobs import-wx-video-search success hub_user_id=%s keyword=%s total=%d latest_batch=%s latest_event=%s freshness=%s",
		summary.HubUserID,
		options.keyword,
		summary.TotalEvents,
		summary.LatestBatchID,
		summary.LastEventType,
		summary.FreshnessState,
	)
	return nil
}

func runBackfillWXVideoCovers(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseBackfillWXVideoCoversArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	result, err := service.BackfillWXVideoCovers(context.Background(), ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
		ObjectIDs:   options.objectIDs,
	})
	if err != nil {
		return err
	}
	return writeJSONLog(logf, result)
}

type backfillWXVideoCoversOptions struct {
	summaryOptions
	objectIDs []string
}

func parseBackfillWXVideoCoversArgs(args []string) (backfillWXVideoCoversOptions, error) {
	options := backfillWXVideoCoversOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--hub-user-id":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--hub-user-id requires a value")
			}
			options.hubUserID = strings.TrimSpace(args[i])
		case "--device-count":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--device-count requires a value")
			}
			value, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return options, fmt.Errorf("invalid --device-count: %w", err)
			}
			options.deviceCount = value
		case "--object-id":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--object-id requires a value")
			}
			if objectID := strings.TrimSpace(args[i]); objectID != "" {
				options.objectIDs = append(options.objectIDs, objectID)
			}
		default:
			return options, fmt.Errorf("unknown backfill-wx-video-covers arg: %s", args[i])
		}
	}
	return options, nil
}

func runSaveWXFeedListBinding(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseFeedListArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	summary, err := service.SaveWXFeedListBinding(context.Background(), ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	}, options.baseURL, options.username)
	if err != nil {
		return err
	}

	return writeJSONLog(logf, map[string]any{
		"status":            "ok",
		"source":            "wx_feed_list",
		"hubUserId":         summary.HubUserID,
		"baseURL":           options.baseURL,
		"username":          options.username,
		"freshnessState":    summary.FreshnessState,
		"bindingStates":     summary.BindingStates,
		"operatorReadiness": summary.OperatorReadiness,
	})
}

func runSaveWXVideoSearchBinding(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseFeedListSearchArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	summary, err := service.SaveWXVideoSearchBinding(context.Background(), ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	}, options.baseURL, options.keyword)
	if err != nil {
		return err
	}

	return writeJSONLog(logf, map[string]any{
		"status":            "ok",
		"source":            "wx_video_search",
		"hubUserId":         summary.HubUserID,
		"baseURL":           options.baseURL,
		"keyword":           options.keyword,
		"freshnessState":    summary.FreshnessState,
		"bindingStates":     summary.BindingStates,
		"operatorReadiness": summary.OperatorReadiness,
	})
}

func runSyncWXCommentExport(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseCommentExportArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	if runtimePack.IngestService == nil {
		return fmt.Errorf("ingest service is not configured")
	}
	if runtimePack.ReadModelService == nil {
		return fmt.Errorf("read model refresh service is not configured")
	}

	query := ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	}
	summary, err := runtimePack.IngestService.ImportWXCommentExport(context.Background(), query, options.path)
	if err != nil {
		return err
	}

	payload, err := runtimePack.ReadModelService.RefreshDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	}, "jobs")
	if err != nil {
		return err
	}

	return writeSyncResult(logf, "wx_comment_export", summary, payload)
}

func runSyncWXFeedList(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseFeedListArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	if runtimePack.IngestService == nil {
		return fmt.Errorf("ingest service is not configured")
	}
	if runtimePack.ReadModelService == nil {
		return fmt.Errorf("read model refresh service is not configured")
	}

	query := ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	}
	summary, err := runtimePack.IngestService.ImportWXFeedList(context.Background(), query, options.baseURL, options.username, "")
	if err != nil {
		return err
	}

	payload, err := runtimePack.ReadModelService.RefreshDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	}, "jobs")
	if err != nil {
		return err
	}

	return writeSyncResult(logf, "wx_feed_list", summary, payload)
}

func runSyncWXVideoSearch(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseFeedListSearchArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	if runtimePack.IngestService == nil {
		return fmt.Errorf("ingest service is not configured")
	}
	if runtimePack.ReadModelService == nil {
		return fmt.Errorf("read model refresh service is not configured")
	}

	query := ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	}
	summary, err := runtimePack.IngestService.ImportWXVideoSearch(context.Background(), query, options.baseURL, options.keyword, "")
	if err != nil {
		return err
	}

	payload, err := runtimePack.ReadModelService.RefreshDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	}, "jobs")
	if err != nil {
		return err
	}

	return writeSyncResult(logf, "wx_video_search", summary, payload)
}

func writeSyncResult(logf func(string, ...any), source string, summary ingest.Summary, payload insight.Dashboard) error {
	return writeJSONLog(logf, map[string]any{
		"status":    "ok",
		"source":    source,
		"import":    operatorflow.BuildImportResult(summary),
		"readModel": operatorflow.BuildReadModelResult(payload),
	})
}

func writeJSONLog(logf func(string, ...any), payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	logf("%s", data)
	return nil
}

type commentExportOptions struct {
	path        string
	hubUserID   string
	deviceCount int
}

type feedListOptions struct {
	username    string
	baseURL     string
	hubUserID   string
	deviceCount int
}

type feedListSearchOptions struct {
	keyword     string
	baseURL     string
	hubUserID   string
	deviceCount int
}

type retryIngestOptions struct {
	source      string
	hubUserID   string
	deviceCount int
}

type saveTrustedRuntimeEvidenceOptions struct {
	proofPath  string
	outputPath string
}

func runSaveTrustedRuntimeEvidence(args []string, _ runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseSaveTrustedRuntimeEvidenceArgs(args)
	if err != nil {
		return err
	}

	evidence, outputPath, err := runtimecfg.SaveTrustedRuntimeEvidenceFromVerifyProof(options.proofPath, options.outputPath)
	if err != nil {
		return err
	}

	return writeJSONLog(logf, map[string]any{
		"status":                 "ok",
		"proofPath":              options.proofPath,
		"outputPath":             outputPath,
		"state":                  evidence.State,
		"source":                 evidence.Source,
		"hubUserId":              evidence.HubUserID,
		"verifiedAt":             evidence.VerifiedAt,
		"trustedBoundaryMode":    evidence.TrustedBoundaryMode,
		"apiTrustedContextState": evidence.APITrustedContextState,
		"apiBrowserQueryState":   evidence.APIBrowserQueryState,
		"webTrustedContextState": evidence.WebTrustedContextState,
		"webBrowserQueryState":   evidence.WebBrowserQueryState,
	})
}

func parseCommentExportArgs(args []string) (commentExportOptions, error) {
	options := commentExportOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--path":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--path requires a value")
			}
			options.path = strings.TrimSpace(args[i])
		case "--hub-user-id":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--hub-user-id requires a value")
			}
			options.hubUserID = strings.TrimSpace(args[i])
		case "--device-count":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--device-count requires a value")
			}
			value, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return options, fmt.Errorf("invalid --device-count: %w", err)
			}
			options.deviceCount = value
		default:
			return options, fmt.Errorf("unknown import-wx-comment-export arg: %s", args[i])
		}
	}
	if options.path == "" {
		return options, fmt.Errorf("import-wx-comment-export requires --path")
	}
	return options, nil
}

func parseFeedListArgs(args []string) (feedListOptions, error) {
	options := feedListOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--username":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--username requires a value")
			}
			options.username = strings.TrimSpace(args[i])
		case "--base-url":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--base-url requires a value")
			}
			options.baseURL = strings.TrimSpace(args[i])
		case "--hub-user-id":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--hub-user-id requires a value")
			}
			options.hubUserID = strings.TrimSpace(args[i])
		case "--device-count":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--device-count requires a value")
			}
			value, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return options, fmt.Errorf("invalid --device-count: %w", err)
			}
			options.deviceCount = value
		default:
			return options, fmt.Errorf("unknown import-wx-feed-list arg: %s", args[i])
		}
	}
	if options.username == "" {
		return options, fmt.Errorf("import-wx-feed-list requires --username")
	}
	return options, nil
}

func parseFeedListSearchArgs(args []string) (feedListSearchOptions, error) {
	options := feedListSearchOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--keyword":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--keyword requires a value")
			}
			options.keyword = strings.TrimSpace(args[i])
		case "--base-url":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--base-url requires a value")
			}
			options.baseURL = strings.TrimSpace(args[i])
		case "--hub-user-id":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--hub-user-id requires a value")
			}
			options.hubUserID = strings.TrimSpace(args[i])
		case "--device-count":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--device-count requires a value")
			}
			value, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return options, fmt.Errorf("invalid --device-count: %w", err)
			}
			options.deviceCount = value
		default:
			return options, fmt.Errorf("unknown wx feed search arg: %s", args[i])
		}
	}
	if options.keyword == "" {
		return options, fmt.Errorf("search-wx-feed-accounts requires --keyword")
	}
	return options, nil
}

func parseSaveTrustedRuntimeEvidenceArgs(args []string) (saveTrustedRuntimeEvidenceOptions, error) {
	options := saveTrustedRuntimeEvidenceOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--proof-path":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--proof-path requires a value")
			}
			options.proofPath = strings.TrimSpace(args[i])
		case "--output-path":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--output-path requires a value")
			}
			options.outputPath = strings.TrimSpace(args[i])
		default:
			return options, fmt.Errorf("unknown save-trusted-runtime-evidence arg: %s", args[i])
		}
	}
	if options.proofPath == "" {
		return options, fmt.Errorf("save-trusted-runtime-evidence requires --proof-path")
	}
	return options, nil
}

func buildIngestService(cfg runtimecfg.Config) (*ingest.Service, func(), error) {
	sqliteClient, err := runtimecfg.BuildSQLiteClient(runtimecfg.Config{SQLite: cfg.SQLite, IngestStore: cfg.IngestStore})
	if err != nil {
		return nil, nil, err
	}
	var postgresClient *postgres.Client
	if cfg.IngestStore.Mode == runtimecfg.StoreModePostgres {
		postgresClient, err = runtimecfg.BuildPostgresClient(cfg)
	}
	if err != nil {
		sqliteClient.Close()
		return nil, nil, err
	}
	cleanup := func() {
		sqliteClient.Close()
		if postgresClient != nil {
			postgresClient.Close()
		}
	}

	repo, err := runtimecfg.BuildIngestRepository(cfg, postgresClient, sqliteClient)
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: cfg.DefaultHubUserID,
		WXFeedListClient: ingest.WXFeedListClient{BaseURL: cfg.WXChannelBaseURL},
		WXCommentClient:  ingest.WXCommentExportClient{BaseURL: cfg.WXChannelBaseURL},
		EnableLiveChecks: true,
	})
	attachJobsEdgeFeedTransport(cfg, service)
	return service, cleanup, nil
}

func runShowDashboard(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseSummaryArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	payload, err := runtimePack.DashboardService.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	})
	if err != nil {
		return err
	}

	data, err := json.Marshal(insightapi.NewDashboardResponse(payload))
	if err != nil {
		return err
	}
	logf("%s", data)
	return nil
}

func runRefreshDashboard(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseSummaryArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	if runtimePack.ReadModelService == nil {
		return fmt.Errorf("read model refresh service is not configured")
	}

	payload, err := runtimePack.ReadModelService.RefreshDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	}, "jobs")
	if err != nil {
		return err
	}

	result := map[string]any{
		"status":         "ok",
		"mode":           payload.ReadModel.Mode,
		"materializedAt": payload.ReadModel.MaterializedAt,
		"latestBatchId":  payload.ReadModel.LatestBatchID,
		"workspaceId":    payload.WorkspaceBridge.WorkspaceID,
		"hubUserId":      payload.WorkspaceBridge.HubUserID,
		"freshnessState": payload.IngestSummary.FreshnessState,
	}
	if runtimePack.NormalizationService != nil {
		if summary, found, summaryErr := runtimePack.NormalizationService.GetSummary(context.Background(), payload.WorkspaceBridge.WorkspaceID); summaryErr != nil {
			return summaryErr
		} else if found {
			result["normalized"] = map[string]any{
				"workspaceId":   summary.WorkspaceID,
				"hubUserId":     summary.HubUserID,
				"state":         summary.State,
				"latestBatchId": summary.LatestBatchID,
				"generatedAt":   summary.GeneratedAt,
				"latestEventAt": summary.LatestEventAt,
				"contentCount":  summary.ContentCount,
				"commentCount":  summary.CommentCount,
				"topicCount":    summary.TopicCount,
				"accountCount":  summary.AccountCount,
			}
		}
	}
	if runtimePack.AggregationService != nil {
		if summary, found, summaryErr := runtimePack.AggregationService.GetSummary(context.Background(), payload.WorkspaceBridge.WorkspaceID); summaryErr != nil {
			return summaryErr
		} else if found {
			result["aggregate"] = map[string]any{
				"workspaceId":        summary.WorkspaceID,
				"hubUserId":          summary.HubUserID,
				"state":              summary.State,
				"latestBatchId":      summary.LatestBatchID,
				"generatedAt":        summary.GeneratedAt,
				"latestObservedAt":   summary.LatestObservedAt,
				"videoMetricCount":   summary.VideoMetricCount,
				"topicMetricCount":   summary.TopicMetricCount,
				"commentFactCount":   summary.CommentFactCount,
				"sourceAccountCount": summary.SourceAccountCount,
			}
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	logf("%s", data)
	return nil
}

func runNormalizeIngest(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseSummaryArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	if runtimePack.NormalizationService == nil {
		return fmt.Errorf("normalization service is not configured")
	}

	snapshot, err := runtimePack.NormalizationService.Normalize(context.Background(), normalize.Query{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	})
	if err != nil {
		return err
	}

	return writeJobsJSON(logf, map[string]any{
		"status":        "ok",
		"workspaceId":   snapshot.Summary.WorkspaceID,
		"hubUserId":     snapshot.Summary.HubUserID,
		"state":         snapshot.Summary.State,
		"latestBatchId": snapshot.Summary.LatestBatchID,
		"generatedAt":   snapshot.Summary.GeneratedAt,
		"latestEventAt": snapshot.Summary.LatestEventAt,
		"contentCount":  snapshot.Summary.ContentCount,
		"commentCount":  snapshot.Summary.CommentCount,
		"topicCount":    snapshot.Summary.TopicCount,
		"accountCount":  snapshot.Summary.AccountCount,
	})
}

func runShowNormalizedSummary(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseSummaryArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	if runtimePack.ContextResolver == nil || runtimePack.NormalizationService == nil {
		return fmt.Errorf("normalization service is not configured")
	}
	workspaceContext, err := runtimePack.ContextResolver.Resolve(context.Background(), workspace.ContextQuery{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	})
	if err != nil {
		return err
	}
	summary, found, err := runtimePack.NormalizationService.GetSummary(context.Background(), workspaceContext.Workspace.ID)
	if err != nil {
		return err
	}
	if !found {
		return writeJobsJSON(logf, map[string]any{
			"status":      "not_found",
			"workspaceId": workspaceContext.Workspace.ID,
			"hubUserId":   workspaceContext.Workspace.OwnerUserID,
		})
	}
	return writeJobsJSON(logf, map[string]any{
		"status":        "ok",
		"workspaceId":   summary.WorkspaceID,
		"hubUserId":     summary.HubUserID,
		"state":         summary.State,
		"latestBatchId": summary.LatestBatchID,
		"generatedAt":   summary.GeneratedAt,
		"latestEventAt": summary.LatestEventAt,
		"contentCount":  summary.ContentCount,
		"commentCount":  summary.CommentCount,
		"topicCount":    summary.TopicCount,
		"accountCount":  summary.AccountCount,
	})
}

func runAggregateMetrics(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseSummaryArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	if runtimePack.AggregationService == nil || runtimePack.ContextResolver == nil {
		return fmt.Errorf("aggregation service is not configured")
	}
	workspaceContext, err := runtimePack.ContextResolver.Resolve(context.Background(), workspace.ContextQuery{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	})
	if err != nil {
		return err
	}
	result, err := runtimePack.AggregationService.RefreshAggregate(context.Background(), workspaceContext)
	if err != nil {
		return err
	}
	return writeJobsJSON(logf, map[string]any{
		"status":             "ok",
		"workspaceId":        result.Summary.WorkspaceID,
		"hubUserId":          result.Summary.HubUserID,
		"state":              result.Summary.State,
		"latestBatchId":      result.Summary.LatestBatchID,
		"generatedAt":        result.Summary.GeneratedAt,
		"latestObservedAt":   result.Summary.LatestObservedAt,
		"videoMetricCount":   result.Summary.VideoMetricCount,
		"topicMetricCount":   result.Summary.TopicMetricCount,
		"commentFactCount":   result.Summary.CommentFactCount,
		"sourceAccountCount": result.Summary.SourceAccountCount,
	})
}

func runShowAggregateSummary(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseSummaryArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	if runtimePack.AggregationService == nil || runtimePack.ContextResolver == nil {
		return fmt.Errorf("aggregation service is not configured")
	}
	workspaceContext, err := runtimePack.ContextResolver.Resolve(context.Background(), workspace.ContextQuery{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	})
	if err != nil {
		return err
	}
	summary, found, err := runtimePack.AggregationService.GetSummary(context.Background(), workspaceContext.Workspace.ID)
	if err != nil {
		return err
	}
	if !found {
		return writeJobsJSON(logf, map[string]any{
			"status":      "not_found",
			"workspaceId": workspaceContext.Workspace.ID,
			"hubUserId":   workspaceContext.Workspace.OwnerUserID,
		})
	}
	return writeJobsJSON(logf, map[string]any{
		"status":             "ok",
		"workspaceId":        summary.WorkspaceID,
		"hubUserId":          summary.HubUserID,
		"state":              summary.State,
		"latestBatchId":      summary.LatestBatchID,
		"generatedAt":        summary.GeneratedAt,
		"latestObservedAt":   summary.LatestObservedAt,
		"videoMetricCount":   summary.VideoMetricCount,
		"topicMetricCount":   summary.TopicMetricCount,
		"commentFactCount":   summary.CommentFactCount,
		"sourceAccountCount": summary.SourceAccountCount,
	})
}

func runShowHotVideos(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	return runShowReadModelSlice(args, cfg, logf, "videos", func(runtimePack *runtimecfg.DashboardRuntime, query insight.DashboardQuery) (any, error) {
		return runtimePack.ReadQueryService.GetHotVideos(context.Background(), query)
	})
}

func runShowHotTopics(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	return runShowReadModelSlice(args, cfg, logf, "topics", func(runtimePack *runtimecfg.DashboardRuntime, query insight.DashboardQuery) (any, error) {
		return runtimePack.ReadQueryService.GetHotTopics(context.Background(), query)
	})
}

func runShowCommentInsights(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	return runShowReadModelSlice(args, cfg, logf, "comments", func(runtimePack *runtimecfg.DashboardRuntime, query insight.DashboardQuery) (any, error) {
		return runtimePack.ReadQueryService.GetCommentInsights(context.Background(), query)
	})
}

func runShowReportSummary(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	return runShowReadModelSlice(args, cfg, logf, "reports", func(runtimePack *runtimecfg.DashboardRuntime, query insight.DashboardQuery) (any, error) {
		return runtimePack.ReadQueryService.GetReportSummary(context.Background(), query)
	})
}

func runShowReadModelSlice(args []string, cfg runtimecfg.Config, logf func(string, ...any), viewKey string, load func(runtimePack *runtimecfg.DashboardRuntime, query insight.DashboardQuery) (any, error)) error {
	options, err := parseSummaryArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	if runtimePack.ReadQueryService == nil {
		return fmt.Errorf("read query service is not configured")
	}

	query := insight.DashboardQuery{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	}
	payload, err := load(runtimePack, query)
	if err != nil {
		return err
	}

	result := map[string]any{}
	switch typed := payload.(type) {
	case []insight.HotVideo:
		result["items"] = typed
	case []insight.HotTopic:
		result["items"] = typed
	case []insight.CommentInsight:
		result["items"] = typed
	case insight.ReportSummary:
		result["item"] = typed
	default:
		result["payload"] = payload
	}
	if viewKey != "" {
		if item, found, err := runtimePack.ReadQueryService.GetViewStatus(context.Background(), query, viewKey); err != nil {
			return err
		} else if found {
			result["viewStatus"] = item
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	logf("%s", data)
	return nil
}

func runShowIngestSummary(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	summary, err := loadIngestSummary(args, cfg)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"hubUserID":          summary.HubUserID,
		"state":              summary.State,
		"freshnessState":     summary.FreshnessState,
		"totalEvents":        summary.TotalEvents,
		"sourceCount":        summary.SourceCount,
		"latestEventAt":      summary.LatestEventAt,
		"lastSuccessAt":      summary.LastSuccessAt,
		"lastFailureAt":      summary.LastFailureAt,
		"lastFailureMessage": summary.LastFailureMessage,
		"latestBatchID":      summary.LatestBatchID,
		"lastEventType":      summary.LastEventType,
		"aggregatedVideos":   summary.AggregatedVideos,
		"aggregatedComments": summary.AggregatedComments,
		"aggregatedTopics":   summary.AggregatedTopics,
		"sourceStatuses":     summary.SourceStatuses,
		"recentRuns":         summary.RecentRuns,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	logf("%s", data)
	return nil
}

func runShowIngestRuns(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	summary, err := loadIngestSummary(args, cfg)
	if err != nil {
		return err
	}
	data, err := json.Marshal(summary.RecentRuns)
	if err != nil {
		return err
	}
	logf("%s", data)
	return nil
}

func runShowIngestSources(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	summary, err := loadIngestSummary(args, cfg)
	if err != nil {
		return err
	}
	data, err := json.Marshal(summary.SourceStatuses)
	if err != nil {
		return err
	}
	logf("%s", data)
	return nil
}

func runShowIngestBindings(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseSummaryArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	bindings, err := service.ListBindingStates(context.Background(), ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	})
	if err != nil {
		return err
	}
	data, err := json.Marshal(bindings)
	if err != nil {
		return err
	}
	logf("%s", data)
	return nil
}

func runRetryLastFailedIngest(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseRetryIngestArgs(args)
	if err != nil {
		return err
	}
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	defer runtimePack.Close()

	if runtimePack.IngestService == nil {
		return fmt.Errorf("ingest service is not configured")
	}
	if runtimePack.ReadModelService == nil {
		return fmt.Errorf("read model refresh service is not configured")
	}

	query := ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	}
	summary, err := runtimePack.IngestService.RetryLastFailedImport(context.Background(), query, options.source)
	if err != nil {
		return err
	}

	payload, err := runtimePack.ReadModelService.RefreshDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     options.hubUserID,
		DeviceCount:   options.deviceCount,
		ContextSource: "jobs",
	}, "jobs")
	if err != nil {
		return err
	}

	return writeSyncResult(logf, options.source, summary, payload)
}

type summaryOptions struct {
	hubUserID   string
	deviceCount int
}

func parseSummaryArgs(args []string) (summaryOptions, error) {
	options := summaryOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--hub-user-id":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--hub-user-id requires a value")
			}
			options.hubUserID = strings.TrimSpace(args[i])
		case "--device-count":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--device-count requires a value")
			}
			value, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return options, fmt.Errorf("invalid --device-count: %w", err)
			}
			options.deviceCount = value
		default:
			return options, fmt.Errorf("unknown summary arg: %s", args[i])
		}
	}
	return options, nil
}

func parseRetryIngestArgs(args []string) (retryIngestOptions, error) {
	options := retryIngestOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--source":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--source requires a value")
			}
			options.source = strings.TrimSpace(args[i])
		case "--hub-user-id":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--hub-user-id requires a value")
			}
			options.hubUserID = strings.TrimSpace(args[i])
		case "--device-count":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--device-count requires a value")
			}
			value, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return options, fmt.Errorf("invalid --device-count: %w", err)
			}
			options.deviceCount = value
		default:
			return options, fmt.Errorf("unknown retry-last-failed-ingest arg: %s", args[i])
		}
	}
	if options.source == "" {
		return options, fmt.Errorf("retry-last-failed-ingest requires --source")
	}
	return options, nil
}

func loadIngestSummary(args []string, cfg runtimecfg.Config) (ingest.Summary, error) {
	options, err := parseSummaryArgs(args)
	if err != nil {
		return ingest.Summary{}, err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return ingest.Summary{}, err
	}
	defer cleanup()

	return service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID:   options.hubUserID,
		DeviceCount: options.deviceCount,
	})
}

func resolveMigrationsDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(wd, "migrations", "postgres"),
		filepath.Join(wd, "..", "..", "migrations", "postgres"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("migrations/postgres directory not found from %s", wd)
}

func attachJobsEdgeFeedTransport(cfg runtimecfg.Config, service *ingest.Service) {
	if service == nil {
		return
	}
	// In hub-less local-operator mode the ingest client already targets the
	// local wx_channel data plane. Keeping the Edge HTTP wrapper here sends the
	// request back through /api/v1/edge/call and can wait on a LAN node instead.
	if cfg.Bridge.LocalOperatorEnabled() && !cfg.Bridge.HubSessionEnabled() && strings.TrimSpace(cfg.Bridge.TrustedToken) == "" {
		return
	}
	base := strings.TrimSpace(os.Getenv("INSIGHT_EDGE_HTTP_BASE"))
	if base == "" {
		addr := strings.TrimSpace(cfg.APIAddr)
		if addr == "" {
			addr = ":18081"
		}
		if strings.HasPrefix(addr, ":") {
			base = "http://127.0.0.1" + addr
		} else if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
			base = addr
		} else {
			base = "http://" + addr
		}
	}
	// Hybrid: try local base first, then Insight edge HTTP call for LAN nodes.
	service.SetWXFeedListTransport(&edge.HTTPFeedClient{
		InsightBaseURL: base,
		OperatorID:     cfg.DefaultHubUserID,
		BridgeToken:    cfg.Bridge.TrustedToken,
	})
	service.SetWXCommentTransport(&edge.HTTPCommentClient{
		InsightBaseURL: base,
		OperatorID:     cfg.DefaultHubUserID,
		BridgeToken:    cfg.Bridge.TrustedToken,
	})
}
