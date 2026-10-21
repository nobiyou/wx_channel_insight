package insight_test

import (
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestBuildReadModelViewStatusesPrefersCommentSourceFreshness(t *testing.T) {
	payload := insight.Dashboard{
		ReadModel: insight.ReadModelState{
			Mode:           "materialized",
			MaterializedAt: time.Date(2026, 7, 3, 10, 14, 0, 0, time.FixedZone("CST", 8*3600)),
			LatestBatchID:  "batch-1",
		},
		IngestSummary: insight.IngestSummary{
			FreshnessState: "fresh",
			LatestBatchID:  "batch-1",
			SourceStatuses: []insight.IngestSourceStatus{
				{Source: "wx_feed_list", FreshnessState: "fresh"},
				{Source: "wx_comment_export", FreshnessState: "lagging"},
				{Source: "feed_comment_list", FreshnessState: "stale"},
			},
		},
	}

	items := insight.BuildReadModelViewStatuses(payload)
	if len(items) != 4 {
		t.Fatalf("expected 4 view statuses, got %d", len(items))
	}

	var commentItem insight.ReadModelViewStatus
	for _, item := range items {
		if item.Key == "comments" {
			commentItem = item
			break
		}
	}

	if commentItem.FreshnessState != "lagging" {
		t.Fatalf("expected comment freshness to follow comment source, got %+v", commentItem)
	}
	if commentItem.SourceNote != "wx_comment_export lagging · feed_comment_list stale" {
		t.Fatalf("expected comment source note, got %+v", commentItem)
	}
}

func TestBuildReadModelViewStatusesMarksLaggingWhenReadModelBatchIsBehind(t *testing.T) {
	payload := insight.Dashboard{
		ReadModel: insight.ReadModelState{
			Mode:           "seed",
			MaterializedAt: time.Time{},
			LatestBatchID:  "batch-older",
		},
		IngestSummary: insight.IngestSummary{
			FreshnessState: "fresh",
			LatestBatchID:  "batch-new",
		},
	}

	items := insight.BuildReadModelViewStatuses(payload)
	if items[0].FreshnessState != "lagging" {
		t.Fatalf("expected video freshness to be lagging, got %+v", items[0])
	}
	if items[3].Summary == "" {
		t.Fatalf("expected report summary guidance, got %+v", items[3])
	}
}
