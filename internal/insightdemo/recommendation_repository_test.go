package insightdemo_test

import (
	"context"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
)

func TestRecommendationRepositoryRoundTripsSnapshot(t *testing.T) {
	repo := insightdemo.NewRecommendationRepository()
	snapshot := insight.RecommendationSnapshot{
		WorkspaceID: "ws-demo",
		VideoRecommendations: []insight.VideoRecommendation{{
			ID:    "video-demo",
			Title: "real candidate",
			Metrics: insight.RecommendationMetrics{
				Likes:     8,
				Comments:  2,
				Favorites: 1,
				Forwards:  1,
			},
		}},
	}

	ctx := context.Background()
	if err := repo.ReplaceRecommendations(ctx, snapshot.WorkspaceID, snapshot); err != nil {
		t.Fatalf("replace recommendations failed: %v", err)
	}
	got, found, err := repo.GetRecommendations(ctx, snapshot.WorkspaceID)
	if err != nil || !found {
		t.Fatalf("get recommendations failed: found=%v err=%v", found, err)
	}
	if len(got.VideoRecommendations) != 1 || got.VideoRecommendations[0].Metrics.Likes != 8 {
		t.Fatalf("unexpected demo recommendation: %+v", got)
	}
}
