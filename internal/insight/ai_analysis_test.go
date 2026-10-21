package insight

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func testAIAnalysisRun(status string) AIAnalysisRun {
	run := AIAnalysisRun{
		ID:                       "run-1",
		WorkspaceID:              "workspace-a",
		Module:                   "Content-Analysis",
		ScopeKey:                 "dashboard",
		Window:                   "24H",
		InputFingerprint:         "sha256:test",
		Status:                   status,
		Result:                   json.RawMessage(`{"summary":"ok"}`),
		ModelProvider:            " openai ",
		ModelName:                " gpt-5.6 ",
		FallbackReason:           "  ",
		DurationMS:               12,
		RunAt:                    time.Date(2026, 9, 1, 8, 0, 0, 0, time.FixedZone("CST", 8*3600)),
	}
	return run
}

func TestNormalizeAIAnalysisRunNormalizesAndRedactsNonCompletedResult(t *testing.T) {
	run, err := NormalizeAIAnalysisRun(testAIAnalysisRun(AIAnalysisStatusFallback))
	if err != nil {
		t.Fatalf("normalize fallback run failed: %v", err)
	}
	if run.Module != "content-analysis" || run.Window != TimeWindow24h || run.ModelName != "gpt-5.6" {
		t.Fatalf("unexpected normalized run: %+v", run)
	}
	if run.Result != nil {
		t.Fatalf("expected fallback result to be redacted, got %s", run.Result)
	}

	completed, err := NormalizeAIAnalysisRun(testAIAnalysisRun(AIAnalysisStatusCompleted))
	if err != nil {
		t.Fatalf("normalize completed run failed: %v", err)
	}
	if !bytes.Equal(completed.Result, []byte(`{"summary":"ok"}`)) {
		t.Fatalf("expected completed result to be retained, got %s", completed.Result)
	}
}

func TestNormalizeAIAnalysisRunRejectsInvalidRecords(t *testing.T) {
	cases := []AIAnalysisRun{
		func() AIAnalysisRun { run := testAIAnalysisRun(AIAnalysisStatusCompleted); run.Result = json.RawMessage(`[]`); return run }(),
		func() AIAnalysisRun { run := testAIAnalysisRun("unknown"); return run }(),
		func() AIAnalysisRun { run := testAIAnalysisRun(AIAnalysisStatusFailed); run.Window = "7d"; return run }(),
		func() AIAnalysisRun { run := testAIAnalysisRun(AIAnalysisStatusCompleted); run.RunAt = time.Time{}; return run }(),
	}
	for index, run := range cases {
		if err := ValidateAIAnalysisRun(run); err == nil {
			t.Errorf("case %d: expected invalid run error", index)
		}
	}
}

func TestCloneAIAnalysisRunCopiesResult(t *testing.T) {
	run := testAIAnalysisRun(AIAnalysisStatusCompleted)
	clone := CloneAIAnalysisRun(run)
	clone.Result[0] = '['
	if bytes.Equal(run.Result, clone.Result) {
		t.Fatalf("expected cloned result to be independent")
	}
}
