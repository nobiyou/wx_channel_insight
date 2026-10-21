package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

func TestResolveStorePathReturnsAbsolutePath(t *testing.T) {
	path, err := ResolveStorePath("var/test-state.json")
	if err != nil {
		t.Fatalf("expected absolute path, got %v", err)
	}
	if path == "" || path == "var/test-state.json" {
		t.Fatalf("expected absolute path, got %q", path)
	}
}

func TestBuildRepositoryRejectsPostgresModeWithoutClient(t *testing.T) {
	cfg := Config{
		DashboardStore: StoreConfig{Mode: StoreModePostgres},
		WorkspaceStore: StoreConfig{Mode: StoreModePostgres},
		IngestStore:    StoreConfig{Mode: StoreModePostgres},
	}

	if _, err := BuildDashboardRepository(cfg, nil); err == nil {
		t.Fatalf("expected dashboard postgres mode to require client")
	}
	if _, err := BuildWorkspaceRepository(cfg, nil); err == nil {
		t.Fatalf("expected workspace postgres mode to require client")
	}
	if _, err := BuildIngestRepository(cfg, nil); err == nil {
		t.Fatalf("expected ingest postgres mode to require client")
	}
	if _, err := BuildMonitoringRepository(Config{MonitoringStore: StoreConfig{Mode: StoreModePostgres}}, nil); err == nil {
		t.Fatalf("expected monitoring postgres mode to require client")
	}
	if _, err := BuildPromptRepository(Config{PromptStore: StoreConfig{Mode: StoreModePostgres}}, nil); err == nil {
		t.Fatalf("expected prompt postgres mode to require client")
	}
}

func TestBuildAIServiceRepositorySupportsMemoryAndRejectsMissingPostgresClient(t *testing.T) {
	memoryRepo, err := BuildAIServiceRepository(Config{AIServiceStore: StoreConfig{Mode: StoreModeMemory}}, nil)
	if err != nil || memoryRepo == nil {
		t.Fatalf("expected memory AI service repository, got %T %v", memoryRepo, err)
	}
	if _, err := BuildAIServiceRepository(Config{AIServiceStore: StoreConfig{Mode: StoreModePostgres}}, nil); err == nil {
		t.Fatal("expected postgres AI service repository to require a client")
	}
}

func TestBuildAIAnalysisRepositorySupportsMemoryAndRejectsMissingPostgresClient(t *testing.T) {
	memoryRepo, err := BuildAIAnalysisRepository(Config{AIAnalysisStore: StoreConfig{Mode: StoreModeMemory}}, nil)
	if err != nil || memoryRepo == nil {
		t.Fatalf("expected memory AI analysis repository, repo=%v err=%v", memoryRepo, err)
	}
	if _, err := BuildAIAnalysisRepository(Config{AIAnalysisStore: StoreConfig{Mode: StoreModePostgres}}, nil); err == nil {
		t.Fatalf("expected missing postgres client error for AI analysis repository")
	}
}

func TestBuildAIAnalysisRepositoryUsesDedicatedFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai-analysis.json")
	repo, err := BuildAIAnalysisRepository(Config{AIAnalysisStore: StoreConfig{Mode: StoreModeFile, Path: path}}, nil)
	if err != nil || repo == nil {
		t.Fatalf("expected file AI analysis repository, repo=%v err=%v", repo, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file repository to initialize at %q: %v", path, err)
	}
}

func TestBuildMonitoringRepositoryUsesDedicatedFilePath(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	repo, err := BuildMonitoringRepository(Config{MonitoringStore: StoreConfig{Mode: StoreModeFile}}, nil)
	if err != nil {
		t.Fatalf("build monitoring repository failed: %v", err)
	}
	task := monitoring.Task{
		ID: "runtime-monitoring-task", HubUserID: "hub:user:runtime",
		Name: "Runtime task", Status: monitoring.StatusEnabled,
		ScopeMode: monitoring.ScopeAllSavedBindings, Interval: monitoring.DefaultTaskInterval,
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("write monitoring task failed: %v", err)
	}
	monitoringPath := filepath.Join(root, "var", "monitoring-tasks.json")
	if _, err := os.Stat(monitoringPath); err != nil {
		t.Fatalf("expected dedicated monitoring store at %q: %v", monitoringPath, err)
	}
}

func TestBuildRecommendationRepositoryUsesDedicatedDefaultWhenConfigIsOmitted(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	repo, err := BuildRecommendationRepository(Config{
		DashboardStore: StoreConfig{Mode: StoreModeFile},
	}, nil)
	if err != nil {
		t.Fatalf("build recommendation repository failed: %v", err)
	}

	snapshot := insight.RecommendationSnapshot{
		WorkspaceID: "ws-runtime-default",
		HubUserID:   "hub:user:runtime-default",
	}
	if err := repo.ReplaceRecommendations(context.Background(), snapshot.WorkspaceID, snapshot); err != nil {
		t.Fatalf("write recommendation snapshot failed: %v", err)
	}

	recommendationPath := filepath.Join(root, "var", "recommendations.json")
	if _, err := os.Stat(recommendationPath); err != nil {
		t.Fatalf("expected dedicated recommendation store at %q: %v", recommendationPath, err)
	}
	if _, err := os.Stat(filepath.Join(root, "var", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("expected generic state store to remain unused, got err=%v", err)
	}
}

func TestBuildDashboardRuntimeProvidesServices(t *testing.T) {
	cfg := Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		DashboardStore: StoreConfig{
			Mode: StoreModeFile,
			Path: "var/test-dashboard-runtime-dashboard.json",
		},
		RankingStore: StoreConfig{
			Mode: StoreModeFile,
			Path: "var/test-dashboard-runtime-rankings.json",
		},
		CommentInsightStore: StoreConfig{
			Mode: StoreModeFile,
			Path: "var/test-dashboard-runtime-comment-insights.json",
		},
		ReportSummaryStore: StoreConfig{
			Mode: StoreModeFile,
			Path: "var/test-dashboard-runtime-report-summary.json",
		},
		WorkspaceStore: StoreConfig{
			Mode: StoreModeFile,
			Path: "var/test-dashboard-runtime-workspace.json",
		},
		IngestStore: StoreConfig{
			Mode: StoreModeFile,
			Path: "var/test-dashboard-runtime-ingest.json",
		},
	}

	runtime, err := BuildDashboardRuntime(cfg)
	if err != nil {
		t.Fatalf("expected dashboard runtime build success, got %v", err)
	}
	defer runtime.Close()

	if runtime.DashboardService == nil || runtime.IngestService == nil || runtime.ContextResolver == nil || runtime.MonitoringRepository == nil || runtime.MonitoringService == nil {
		t.Fatalf("expected assembled services, got %+v", runtime)
	}
	if runtime.ReportSummaryService == nil {
		t.Fatalf("expected report summary service, got %+v", runtime)
	}
	if runtime.RankingService == nil {
		t.Fatalf("expected ranking service, got %+v", runtime)
	}
	if runtime.CommentInsightService == nil {
		t.Fatalf("expected comment insight service, got %+v", runtime)
	}
	if runtime.PromptRepository == nil {
		t.Fatalf("expected prompt repository, got %+v", runtime)
	}
	if runtime.AIAnalysisRepository == nil {
		t.Fatalf("expected AI analysis repository, got %+v", runtime)
	}
	if runtime.RuntimeStatus.Phase != "team-edition-phase-a" {
		t.Fatalf("expected runtime status to be assembled, got %+v", runtime.RuntimeStatus)
	}

	payload, err := runtime.DashboardService.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	})
	if err != nil {
		t.Fatalf("expected dashboard query success, got %v", err)
	}
	if payload.Workspace.Owner != "hub:user:alice" {
		t.Fatalf("expected resolved workspace owner, got %+v", payload.Workspace)
	}
}

func TestBuildDashboardRuntimeReturnsEmptyRankingsWithoutEvents(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	cfg := Config{
		DefaultHubUserID:   "hub:user:live",
		DefaultDeviceCount: 1,
		DashboardStore:     StoreConfig{Mode: StoreModeFile, Path: "var/dashboard.json"},
		RankingStore:       StoreConfig{Mode: StoreModeFile, Path: "var/rankings.json"},
		CommentInsightStore: StoreConfig{
			Mode: StoreModeFile,
			Path: "var/comment-insights.json",
		},
		ReportSummaryStore: StoreConfig{Mode: StoreModeFile, Path: "var/report-summary.json"},
		WorkspaceStore:     StoreConfig{Mode: StoreModeFile, Path: "var/workspace.json"},
		IngestStore:        StoreConfig{Mode: StoreModeFile, Path: "var/ingest.json"},
	}

	runtimePack, err := BuildDashboardRuntime(cfg)
	if err != nil {
		t.Fatalf("build dashboard runtime failed: %v", err)
	}
	defer runtimePack.Close()

	query := insight.DashboardQuery{
		HubUserID:   "hub:user:live",
		DeviceCount: 1,
	}
	videos, err := runtimePack.ReadQueryService.GetHotVideos(context.Background(), query)
	if err != nil {
		t.Fatalf("get hot videos failed: %v", err)
	}
	if len(videos) != 0 {
		t.Fatalf("expected empty hot videos without events, got %+v", videos)
	}

	topics, err := runtimePack.ReadQueryService.GetHotTopics(context.Background(), query)
	if err != nil {
		t.Fatalf("get hot topics failed: %v", err)
	}
	if len(topics) != 0 {
		t.Fatalf("expected empty hot topics without events, got %+v", topics)
	}
}
