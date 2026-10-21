package insight

import (
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func TestBuildVideoDetailSortsHistoryAndComputesFourMetricDeltas(t *testing.T) {
	firstAt := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(2 * time.Hour)
	observations := []ingest.VideoMetricObservation{
		{
			ID: "observation-2", ObjectID: "object-1", ObjectNonceID: "nonce-1", ObservedAt: secondAt,
			Title: "视频标题", ChannelName: "账号", LikeCount: 18, CommentCount: 9, FavCount: 7, ForwardCount: 4,
		},
		{
			ID: "observation-1", ObjectID: "object-1", ObjectNonceID: "nonce-1", ObservedAt: firstAt,
			Title: "视频标题", ChannelName: "账号", LikeCount: 10, CommentCount: 5, FavCount: 3, ForwardCount: 1,
		},
	}

	detail, found := BuildVideoDetail("object-1", observations, nil)
	if !found {
		t.Fatal("expected video detail")
	}
	if len(detail.History) != 2 || !detail.History[0].ObservedAt.Equal(firstAt) || !detail.History[1].ObservedAt.Equal(secondAt) {
		t.Fatalf("expected ascending history, got %+v", detail.History)
	}
	if detail.Metrics != (VideoMetricSnapshot{Likes: 18, Comments: 9, Favorites: 7, Forwards: 4}) {
		t.Fatalf("unexpected current metrics: %+v", detail.Metrics)
	}
	if detail.PositiveDeltas != (RecommendationDeltas{Likes: 8, Comments: 4, Favorites: 4, Forwards: 3, Total: 19}) {
		t.Fatalf("unexpected positive deltas: %+v", detail.PositiveDeltas)
	}
	if detail.ObservationCount != 2 || !detail.HasGrowthBaseline || detail.InsufficientHistory {
		t.Fatalf("unexpected history state: %+v", detail)
	}
}

func TestBuildVideoDetailUsesProfileForIncompleteMetadataWithoutAddingHistory(t *testing.T) {
	profile := &ingest.WXFeedListItem{
		ID: "object-2", ObjectNonceID: "nonce-profile", Title: "实时标题", ChannelName: "实时账号",
		CoverURL: "https://cdn.example.test/cover.jpg", PublishedAt: time.Date(2026, 8, 8, 1, 2, 3, 0, time.UTC),
		LikeCount: 12, CommentCount: 4, FavCount: 2, ForwardCount: 1,
	}
	detail, found := BuildVideoDetail("object-2", nil, profile)
	if !found {
		t.Fatal("expected profile-backed detail")
	}
	if detail.Title != "实时标题" || detail.CoverURL != profile.CoverURL || detail.ObjectNonceID != "nonce-profile" {
		t.Fatalf("expected profile metadata, got %+v", detail)
	}
	if detail.Metrics != (VideoMetricSnapshot{Likes: 12, Comments: 4, Favorites: 2, Forwards: 1}) || len(detail.History) != 0 {
		t.Fatalf("profile data must not become persisted history: %+v", detail)
	}
}
