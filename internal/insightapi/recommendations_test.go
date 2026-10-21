package insightapi_test

import (
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
)

func TestNewVideoRecommendationsResponseMapsCoverURL(t *testing.T) {
	result := insightapi.NewVideoRecommendationsResponse([]insight.VideoRecommendation{{
		ID:          "video-1",
		ObjectID:    "object-1",
		CoverURL:    "https://cdn.example.test/video-cover.jpg",
		Title:       "真实视频",
		PublishedAt: time.Date(2026, 8, 8, 8, 0, 0, 0, time.FixedZone("CST", 8*3600)),
	}}, insight.RecommendationSnapshot{})

	if len(result.Items) != 1 || result.Items[0].CoverURL != "https://cdn.example.test/video-cover.jpg" {
		t.Fatalf("expected video recommendation cover URL to map, got %+v", result.Items)
	}
}
