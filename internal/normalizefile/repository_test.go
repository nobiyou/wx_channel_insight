package normalizefile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	"github.com/nobiyou/wx_channel_insight/internal/normalizefile"
)

func TestRepositoryRoundTrip(t *testing.T) {
	repo, err := normalizefile.NewRepository(filepath.Join(t.TempDir(), "normalized-state.json"))
	if err != nil {
		t.Fatalf("new normalize file repository failed: %v", err)
	}

	snapshot := normalize.Snapshot{
		Summary: normalize.Summary{
			WorkspaceID:   "ws-default-hub-user-alice",
			HubUserID:     "hub:user:alice",
			State:         "ready",
			LatestBatchID: "mock-batch-002",
			GeneratedAt:   time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
			ContentCount:  2,
			AccountCount:  1,
		},
		ContentSnapshots: []normalize.ContentSnapshot{{
			ID:          "growth-lab::demo",
			Title:       "demo",
			ChannelName: "Growth Lab",
		}},
	}

	if err := repo.ReplaceSnapshot(context.Background(), snapshot.Summary.WorkspaceID, snapshot); err != nil {
		t.Fatalf("replace snapshot failed: %v", err)
	}

	summary, found, err := repo.GetSummary(context.Background(), snapshot.Summary.WorkspaceID)
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}
	if !found || summary.LatestBatchID != snapshot.Summary.LatestBatchID {
		t.Fatalf("unexpected summary result: found=%t summary=%+v", found, summary)
	}

	loaded, found, err := repo.GetSnapshot(context.Background(), snapshot.Summary.WorkspaceID)
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if !found || len(loaded.ContentSnapshots) != 1 {
		t.Fatalf("unexpected loaded snapshot: found=%t snapshot=%+v", found, loaded)
	}
}
