package insightapi

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestNewAIAnalysisRunResponseMapsTraceFieldsWithoutSecrets(t *testing.T) {
	run := insight.AIAnalysisRun{
		ID: "run-1", WorkspaceID: "workspace-a", Module: "content-analysis", ScopeKey: "dashboard",
		Window: "24h", Status: insight.AIAnalysisStatusCompleted, Result: json.RawMessage(`{"summary":"ok"}`),
		InputFingerprint: "sha256:test", ModelName: "gpt-5.6", RunAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}
	response := NewAIAnalysisRunResponse(run)
	if response.WorkspaceID != run.WorkspaceID || response.Status != run.Status || !bytes.Equal(response.Result, run.Result) {
		t.Fatalf("unexpected mapped response: %+v", response)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response failed: %v", err)
	}
	if bytes.Contains(encoded, []byte("apiKey")) || bytes.Contains(encoded, []byte("ciphertext")) {
		t.Fatalf("response contains secret-shaped fields: %s", encoded)
	}
}

func TestNewAIAnalysisRunsResponseMapsItems(t *testing.T) {
	response := NewAIAnalysisRunsResponse("daily-report", "report-center", "all", []insight.AIAnalysisRun{{
		ID: "run-1", WorkspaceID: "workspace-a", Module: "daily-report", ScopeKey: "report-center",
		Window: "all", Status: insight.AIAnalysisStatusFallback, InputFingerprint: "sha256:test",
		RunAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}})
	if response.Module != "daily-report" || response.ScopeKey != "report-center" || len(response.Items) != 1 {
		t.Fatalf("unexpected list response: %+v", response)
	}
}
