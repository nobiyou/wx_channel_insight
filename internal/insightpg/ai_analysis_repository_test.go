package insightpg

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func openAIAnalysisTestRepository(t *testing.T) (*AIAnalysisRepository, *postgres.Client) {
	t.Helper()
	db, _ := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatalf("new postgres client failed: %v", err)
	}
	repo, err := NewAIAnalysisRepository(client)
	if err != nil {
		t.Fatalf("new AI analysis postgres repository failed: %v", err)
	}
	return repo, client
}

func TestAIAnalysisRepositoryRoundTripAndWorkspaceIsolation(t *testing.T) {
	repo, client := openAIAnalysisTestRepository(t)
	ctx := context.Background()
	workspaceA := "workspace-pg-a"
	workspaceB := "workspace-pg-b"
	t.Cleanup(func() {
		_, _ = client.DB().ExecContext(ctx, "DELETE FROM insight_ai_analysis_runs WHERE workspace_id IN ($1, $2)", workspaceA, workspaceB)
	})
	run := insight.AIAnalysisRun{
		ID: "run-pg-1", WorkspaceID: workspaceA, Module: "content-analysis", ScopeKey: "dashboard",
		Window: "24h", InputFingerprint: "sha256:pg", Status: insight.AIAnalysisStatusCompleted,
		Result: json.RawMessage(`{"summary":"stored"}`), ModelProvider: "openai", ModelName: "gpt-5.6",
		ModelAttempted: true, RunAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}
	if err := repo.Save(ctx, run); err != nil {
		t.Fatalf("save postgres run failed: %v", err)
	}
	items, err := repo.List(ctx, workspaceA, "content-analysis", "dashboard", "24h", 1)
	if err != nil || len(items) != 1 || items[0].ID != run.ID {
		t.Fatalf("unexpected postgres list result: items=%+v err=%v", items, err)
	}
	if _, found, err := repo.Get(ctx, workspaceB, run.ID); err != nil || found {
		t.Fatalf("expected postgres workspace isolation, found=%v err=%v", found, err)
	}
}
