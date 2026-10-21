package insightapi_test

import (
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
)

func TestNewHotVideosResponseMapsOptionalViewStatus(t *testing.T) {
	viewStatus := &insight.ReadModelViewStatus{
		Key:            "videos",
		Label:          "Hot Videos",
		FreshnessState: "fresh",
		Summary:        "当前热视频榜已经跟到最新物化批次。",
		SourceNote:     "wx_feed_list fresh",
		LatestBatchID:  "batch-1",
		MaterializedAt: time.Date(2026, 7, 3, 12, 0, 0, 0, time.FixedZone("CST", 8*3600)),
	}

	result := insightapi.NewHotVideosResponse([]insight.HotVideo{{ID: "vid-1", Title: "A"}}, viewStatus)

	if len(result.Items) != 1 || result.Items[0].ID != "vid-1" {
		t.Fatalf("expected hot videos items, got %+v", result.Items)
	}
	if result.Ranking != insight.HotVideoRankingHot {
		t.Fatalf("expected default hot ranking, got %+v", result)
	}
	if result.ViewStatus == nil || result.ViewStatus.Key != "videos" {
		t.Fatalf("expected mapped view status, got %+v", result.ViewStatus)
	}
	if result.ViewStatus.MaterializedAt != "2026-07-03T12:00:00+08:00" {
		t.Fatalf("expected RFC3339 materializedAt, got %+v", result.ViewStatus)
	}
}

func TestNewReportSummaryOnlyResponseOmitsNilViewStatus(t *testing.T) {
	result := insightapi.NewReportSummaryOnlyResponse(insight.ReportSummary{
		Headline: "日报",
	}, nil)

	if result.Item.Headline != "日报" {
		t.Fatalf("expected report summary item, got %+v", result.Item)
	}
	if result.ViewStatus != nil {
		t.Fatalf("expected nil view status, got %+v", result.ViewStatus)
	}
}
