package runtime

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadJobsRuntimeState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs-runtime-state.json")
	input := JobsRuntimeState{
		LastMode:           "operator-loop",
		LastStatus:         "partial",
		LastHubUserID:      "hub:user:alice",
		LastStartedAt:      time.Date(2026, 7, 3, 7, 0, 0, 0, time.UTC),
		LastFinishedAt:     time.Date(2026, 7, 3, 7, 1, 0, 0, time.UTC),
		LastIteration:      2,
		LastIterations:     12,
		LastInterval:       "5m0s",
		LastError:          "wx_feed_list timeout",
		LastRequestedCount: 2,
		LastRequestedSources: []string{
			"wx_comment_export",
			"wx_feed_list",
		},
	}

	if err := SaveJobsRuntimeState(path, input); err != nil {
		t.Fatalf("expected save success, got %v", err)
	}

	result, err := LoadJobsRuntimeState(path)
	if err != nil {
		t.Fatalf("expected load success, got %v", err)
	}
	if result.LastMode != "operator-loop" || result.LastStatus != "partial" {
		t.Fatalf("expected persisted mode/status, got %+v", result)
	}
	if result.LastIteration != 2 || result.LastIterations != 12 {
		t.Fatalf("expected persisted iteration fields, got %+v", result)
	}
	if len(result.LastRequestedSources) != 2 || result.LastRequestedSources[0] != "wx_comment_export" || result.LastRequestedSources[1] != "wx_feed_list" {
		t.Fatalf("expected persisted requested sources, got %+v", result)
	}
	if result.UpdatedAt.IsZero() {
		t.Fatalf("expected updated timestamp, got %+v", result)
	}
}
