package insightdemo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func demoAIAnalysisRun(id, workspace string, at time.Time) insight.AIAnalysisRun {
	return insight.AIAnalysisRun{
		ID: id, WorkspaceID: workspace, Module: "daily-report", ScopeKey: "report-center",
		Window: "all", InputFingerprint: "sha256:" + id, Status: insight.AIAnalysisStatusCompleted,
		Result: json.RawMessage(`{"headline":"daily"}`), RunAt: at,
	}
}

func TestAIAnalysisRepositoryLimitsAndIsolatesHistory(t *testing.T) {
	repo := NewAIAnalysisRepository()
	ctx := context.Background()
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	for index := 0; index < insight.MaxAIAnalysisHistoryLimit+3; index++ {
		if err := repo.Save(ctx, demoAIAnalysisRun(string(rune('a'+index)), "workspace-a", base.Add(time.Duration(index)*time.Minute))); err != nil {
			t.Fatalf("save run %d failed: %v", index, err)
		}
	}
	items, err := repo.List(ctx, "workspace-a", "daily-report", "report-center", "all", 100)
	if err != nil {
		t.Fatalf("list demo runs failed: %v", err)
	}
	if len(items) != insight.MaxAIAnalysisHistoryLimit || items[0].ID != string(rune('a'+insight.MaxAIAnalysisHistoryLimit+2)) {
		t.Fatalf("expected capped latest-first history, got len=%d first=%q", len(items), items[0].ID)
	}
	if _, found, err := repo.Get(ctx, "workspace-b", items[0].ID); err != nil || found {
		t.Fatalf("expected demo workspace isolation, found=%v err=%v", found, err)
	}
}
