package aggregatefile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/aggregatefile"
)

func TestRepositoryRoundTrip(t *testing.T) {
	repo, err := aggregatefile.NewRepository(filepath.Join(t.TempDir(), "aggregate-state.json"))
	if err != nil {
		t.Fatalf("new aggregate file repository failed: %v", err)
	}

	input := aggregate.Aggregate{
		Summary: aggregate.Summary{
			WorkspaceID:      "ws-default-hub-user-alice",
			HubUserID:        "hub:user:alice",
			State:            "ready",
			LatestBatchID:    "batch-001",
			GeneratedAt:      time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
			LatestObservedAt: time.Date(2026, 7, 5, 11, 59, 0, 0, time.UTC),
			TopicMetricCount: 1,
		},
		TopicMetrics: []aggregate.TopicMetric{{
			ID:         "topic-1",
			Keyword:    "team-preview",
			GrowthRate: 48.5,
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
		t.Fatalf("unexpected summary result: found=%t summary=%+v", found, summary)
	}

	loaded, found, err := repo.GetAggregate(context.Background(), input.Summary.WorkspaceID)
	if err != nil {
		t.Fatalf("get aggregate failed: %v", err)
	}
	if !found || len(loaded.TopicMetrics) != 1 || loaded.TopicMetrics[0].Keyword != "team-preview" {
		t.Fatalf("unexpected aggregate: found=%t aggregate=%+v", found, loaded)
	}
}
