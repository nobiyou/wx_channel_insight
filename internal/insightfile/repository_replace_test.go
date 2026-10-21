package insightfile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
)

func TestDashboardRepositoryReplacePersistsSnapshot(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "dashboard-snapshot.json")
	seed := insight.Dashboard{
		GeneratedAt: time.Date(2026, 6, 30, 17, 0, 0, 0, time.FixedZone("CST", 8*3600)),
	}

	repo, err := insightfile.NewDashboardRepository(storePath, seed)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	next := insight.Dashboard{
		GeneratedAt: time.Date(2026, 6, 30, 18, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		Overview: insight.OverviewMetrics{
			MonitoredAccounts: 27,
		},
	}
	if err := repo.ReplaceDashboard(context.Background(), "", next); err != nil {
		t.Fatalf("expected replace success, got %v", err)
	}

	reloaded, err := insightfile.NewDashboardRepository(storePath, insight.Dashboard{})
	if err != nil {
		t.Fatalf("expected reload success, got %v", err)
	}
	got, err := reloaded.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("expected get success, got %v", err)
	}
	if got.GeneratedAt.Format(time.RFC3339) != "2026-06-30T18:00:00+08:00" {
		t.Fatalf("expected replaced generatedAt, got %s", got.GeneratedAt.Format(time.RFC3339))
	}
	if got.Overview.MonitoredAccounts != 27 {
		t.Fatalf("expected replaced overview, got %d", got.Overview.MonitoredAccounts)
	}
}
