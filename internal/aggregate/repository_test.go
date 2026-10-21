package aggregate_test

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
)

func TestMemoryRepositoryRoundTrip(t *testing.T) {
	repo := aggregate.NewMemoryRepository()
	input := aggregate.Aggregate{
		Summary: aggregate.Summary{
			WorkspaceID:      "ws-default-hub-user-alice",
			HubUserID:        "hub:user:alice",
			State:            "ready",
			LatestBatchID:    "batch-001",
			GeneratedAt:      time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
			LatestObservedAt: time.Date(2026, 7, 5, 11, 59, 0, 0, time.UTC),
			VideoMetricCount: 1,
		},
		VideoMetrics: []aggregate.VideoMetric{{
			ID:         "video-1",
			Title:      "team ops",
			Views:      18000,
			ObservedAt: time.Date(2026, 7, 5, 11, 59, 0, 0, time.UTC),
		}},
	}

	if err := repo.ReplaceAggregate(context.Background(), input.Summary.WorkspaceID, input); err != nil {
		t.Fatalf("replace aggregate failed: %v", err)
	}

	summary, found, err := repo.GetSummary(context.Background(), input.Summary.WorkspaceID)
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}
	if !found || summary.LatestBatchID != input.Summary.LatestBatchID {
		t.Fatalf("unexpected summary: found=%t summary=%+v", found, summary)
	}

	loaded, found, err := repo.GetAggregate(context.Background(), input.Summary.WorkspaceID)
	if err != nil {
		t.Fatalf("get aggregate failed: %v", err)
	}
	if !found || len(loaded.VideoMetrics) != 1 || loaded.VideoMetrics[0].Title != "team ops" {
		t.Fatalf("unexpected aggregate: found=%t aggregate=%+v", found, loaded)
	}
}
