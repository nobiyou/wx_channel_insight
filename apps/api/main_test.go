package main

import (
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestResolveStorePathReturnsAbsolutePath(t *testing.T) {
	path, err := runtime.ResolveStorePath("var/test-state.json")
	if err != nil {
		t.Fatalf("expected absolute path, got %v", err)
	}
	if path == "" || path == "var/test-state.json" {
		t.Fatalf("expected absolute path, got %q", path)
	}
}

func TestBuildRepositoryRejectsPostgresModeWithoutClient(t *testing.T) {
	cfg := runtime.Config{
		DashboardStore: runtime.StoreConfig{Mode: runtime.StoreModePostgres},
		WorkspaceStore: runtime.StoreConfig{Mode: runtime.StoreModePostgres},
		IngestStore:    runtime.StoreConfig{Mode: runtime.StoreModePostgres},
	}

	if _, err := runtime.BuildDashboardRepository(cfg, nil); err == nil {
		t.Fatalf("expected dashboard postgres mode to require client")
	}
	if _, err := runtime.BuildWorkspaceRepository(cfg, nil); err == nil {
		t.Fatalf("expected workspace postgres mode to require client")
	}
	if _, err := runtime.BuildIngestRepository(cfg, nil); err == nil {
		t.Fatalf("expected ingest postgres mode to require client")
	}
}

func TestBuildDashboardRuntimeIncludesRuntimeStatus(t *testing.T) {
	cfg := runtime.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtime.StoreConfig{Mode: runtime.StoreModeFile, Path: "var/test-api-main-dashboard.json"},
		RankingStore:        runtime.StoreConfig{Mode: runtime.StoreModeFile, Path: "var/test-api-main-rankings.json"},
		CommentInsightStore: runtime.StoreConfig{Mode: runtime.StoreModeFile, Path: "var/test-api-main-comment-insights.json"},
		ReportSummaryStore:  runtime.StoreConfig{Mode: runtime.StoreModeFile, Path: "var/test-api-main-report-summary.json"},
		WorkspaceStore:      runtime.StoreConfig{Mode: runtime.StoreModeFile, Path: "var/test-api-main-workspace.json"},
		IngestStore:         runtime.StoreConfig{Mode: runtime.StoreModeFile, Path: "var/test-api-main-ingest.json"},
	}

	runtimePack, err := runtime.BuildDashboardRuntime(cfg)
	if err != nil {
		t.Fatalf("expected dashboard runtime build success, got %v", err)
	}
	defer runtimePack.Close()

	if runtimePack.RuntimeStatus.Phase != "team-edition-phase-a" {
		t.Fatalf("expected runtime status phase, got %+v", runtimePack.RuntimeStatus)
	}
	if runtimePack.DashboardService == nil || runtimePack.ReadQueryService == nil || runtimePack.IngestService == nil {
		t.Fatalf("expected assembled runtime services, got %+v", runtimePack)
	}
	if runtimePack.ReadCache == nil {
		t.Fatalf("expected assembled runtime read cache owner, got %+v", runtimePack)
	}
}
