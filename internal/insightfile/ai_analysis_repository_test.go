package insightfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func fileAIAnalysisRun(id, workspace string, at time.Time, status string) insight.AIAnalysisRun {
	return insight.AIAnalysisRun{
		ID: id, WorkspaceID: workspace, Module: "content-analysis", ScopeKey: "dashboard",
		Window: "24h", InputFingerprint: "sha256:" + id, Status: status,
		Result: json.RawMessage(`{"summary":"ok"}`), RunAt: at,
	}
}

func TestAIAnalysisRepositoryRoundTripOrdersAndIsolatesRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.json")
	repo, err := NewAIAnalysisRepository(path)
	if err != nil {
		t.Fatalf("new file repository failed: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	for _, run := range []insight.AIAnalysisRun{
		fileAIAnalysisRun("run-old", "workspace-a", base, insight.AIAnalysisStatusCompleted),
		fileAIAnalysisRun("run-new", "workspace-a", base.Add(time.Minute), insight.AIAnalysisStatusCompleted),
		fileAIAnalysisRun("run-other", "workspace-b", base.Add(2*time.Minute), insight.AIAnalysisStatusCompleted),
	} {
		if err := repo.Save(ctx, run); err != nil {
			t.Fatalf("save %s failed: %v", run.ID, err)
		}
	}
	fallback := fileAIAnalysisRun("run-fallback", "workspace-a", base.Add(3*time.Minute), insight.AIAnalysisStatusFallback)
	if err := repo.Save(ctx, fallback); err != nil {
		t.Fatalf("save fallback failed: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected temporary file to be renamed away, stat err=%v", err)
	}

	items, err := repo.List(ctx, "workspace-a", "content-analysis", "dashboard", "24h", 100)
	if err != nil {
		t.Fatalf("list runs failed: %v", err)
	}
	if len(items) != 3 || items[0].ID != "run-fallback" || items[1].ID != "run-new" {
		t.Fatalf("expected latest-first workspace history, got %+v", items)
	}
	if items[0].Result != nil {
		t.Fatalf("expected persisted fallback result to be empty, got %s", items[0].Result)
	}
	if _, found, err := repo.Get(ctx, "workspace-b", "run-new"); err != nil || found {
		t.Fatalf("expected workspace isolation, found=%v err=%v", found, err)
	}

	reopened, err := NewAIAnalysisRepository(path)
	if err != nil {
		t.Fatalf("reopen file repository failed: %v", err)
	}
	items, err = reopened.List(ctx, "workspace-a", "", "", "", 0)
	if err != nil || len(items) != 3 {
		t.Fatalf("expected persisted history after reopen, len=%d err=%v", len(items), err)
	}
}

func TestAIAnalysisRepositoryRejectsMalformedStoredRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.json")
	payload := `{"items":[{"id":"bad","workspaceId":"workspace-a","module":"content-analysis","scopeKey":"dashboard","window":"24h","inputFingerprint":"sha256:bad","status":"completed","result":[],"runAt":"2026-09-01T08:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write malformed repository file failed: %v", err)
	}
	if _, err := NewAIAnalysisRepository(path); err == nil {
		t.Fatalf("expected malformed stored record to be rejected")
	}
}
