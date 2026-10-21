package insightfile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
)

func TestDashboardRepositoryKeepsWorkspaceSnapshotsSeparate(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "dashboard-snapshot.json")
	seed := insight.Dashboard{
		GeneratedAt: time.Date(2026, 6, 30, 17, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		Workspace: insight.WorkspaceSummary{
			Name: "默认运营工作区",
		},
	}

	repo, err := insightfile.NewDashboardRepository(storePath, seed)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	alpha := seed
	alpha.Workspace.Owner = "hub:user:alpha"
	alpha.GeneratedAt = time.Date(2026, 6, 30, 18, 0, 0, 0, time.FixedZone("CST", 8*3600))
	beta := seed
	beta.Workspace.Owner = "hub:user:beta"
	beta.GeneratedAt = time.Date(2026, 6, 30, 19, 0, 0, 0, time.FixedZone("CST", 8*3600))

	if err := repo.ReplaceDashboard(context.Background(), "ws-default-hub-user-alpha", alpha); err != nil {
		t.Fatalf("expected alpha replace success, got %v", err)
	}
	if err := repo.ReplaceDashboard(context.Background(), "ws-default-hub-user-beta", beta); err != nil {
		t.Fatalf("expected beta replace success, got %v", err)
	}

	gotAlpha, err := repo.GetDashboard(context.Background(), "ws-default-hub-user-alpha")
	if err != nil {
		t.Fatalf("expected alpha get success, got %v", err)
	}
	gotBeta, err := repo.GetDashboard(context.Background(), "ws-default-hub-user-beta")
	if err != nil {
		t.Fatalf("expected beta get success, got %v", err)
	}
	if gotAlpha.Workspace.Owner != "hub:user:alpha" {
		t.Fatalf("expected alpha owner, got %+v", gotAlpha.Workspace)
	}
	if gotBeta.Workspace.Owner != "hub:user:beta" {
		t.Fatalf("expected beta owner, got %+v", gotBeta.Workspace)
	}
}
