package insightfile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
)

func TestDashboardRepositorySeedsAndReloadsSnapshot(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "dashboard-snapshot.json")
	seed := insight.Dashboard{
		GeneratedAt: time.Date(2026, 6, 30, 17, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		Workspace: insight.WorkspaceSummary{
			Name:        "默认运营工作区",
			DeviceCount: 6,
		},
		Overview: insight.OverviewMetrics{
			MonitoredAccounts: 18,
			TrackedVideos:     326,
		},
	}

	repo, err := insightfile.NewDashboardRepository(storePath, seed)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	got, err := repo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("expected get success, got %v", err)
	}
	if got.GeneratedAt.IsZero() {
		t.Fatal("expected generatedAt to be seeded")
	}

	reloaded, err := insightfile.NewDashboardRepository(storePath, insight.Dashboard{})
	if err != nil {
		t.Fatalf("expected reload success, got %v", err)
	}

	reloadedSnapshot, err := reloaded.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("expected reload get success, got %v", err)
	}
	if reloadedSnapshot.GeneratedAt.Format(time.RFC3339) != "2026-06-30T17:00:00+08:00" {
		t.Fatalf("expected persisted generatedAt, got %s", reloadedSnapshot.GeneratedAt.Format(time.RFC3339))
	}
	if reloadedSnapshot.Overview.MonitoredAccounts != 18 {
		t.Fatalf("expected persisted overview, got %d", reloadedSnapshot.Overview.MonitoredAccounts)
	}
}

func TestDashboardRepositoryRefreshesAcrossLiveInstances(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "dashboard-snapshot.json")
	seed := insight.Dashboard{
		GeneratedAt: time.Date(2026, 7, 3, 18, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		Overview: insight.OverviewMetrics{
			MonitoredAccounts: 10,
		},
	}

	writer, err := insightfile.NewDashboardRepository(storePath, seed)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	reader, err := insightfile.NewDashboardRepository(storePath, seed)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	updated := seed
	updated.Overview.MonitoredAccounts = 42
	updated.Overview.TrackedVideos = 108
	if err := writer.ReplaceDashboard(context.Background(), "ws-alpha", updated); err != nil {
		t.Fatalf("replace dashboard failed: %v", err)
	}

	got, err := reader.GetDashboard(context.Background(), "ws-alpha")
	if err != nil {
		t.Fatalf("expected get success, got %v", err)
	}
	if got.Overview.MonitoredAccounts != 42 || got.Overview.TrackedVideos != 108 {
		t.Fatalf("expected live dashboard refresh, got %+v", got.Overview)
	}
}
