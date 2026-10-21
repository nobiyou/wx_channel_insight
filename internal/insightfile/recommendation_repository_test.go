package insightfile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
)

func TestRecommendationRepositoryRoundTripsAndResets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recommendations.json")
	repo, err := insightfile.NewRecommendationRepository(path)
	if err != nil {
		t.Fatalf("new recommendation repository failed: %v", err)
	}

	snapshot := insight.RecommendationSnapshot{
		WorkspaceID:   "ws-file",
		HubUserID:     "hub:user:file",
		LatestBatchID: "batch-file",
		GeneratedAt:   time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC),
		AccountRecommendations: []insight.AccountRecommendation{{
			Username: "candidate-file",
			Metrics: insight.RecommendationMetrics{
				Likes:     12,
				Comments:  3,
				Favorites: 4,
				Forwards:  2,
			},
			CanMonitor: true,
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
	if got.AccountRecommendations[0].Username != "candidate-file" || got.AccountRecommendations[0].Metrics.Forwards != 2 {
		t.Fatalf("unexpected persisted recommendation: %+v", got)
	}

	if err := repo.ResetRecommendations(ctx, snapshot.WorkspaceID); err != nil {
		t.Fatalf("reset recommendations failed: %v", err)
	}
	if _, found, err := repo.GetRecommendations(ctx, snapshot.WorkspaceID); err != nil || found {
		t.Fatalf("expected reset recommendation snapshot, found=%v err=%v", found, err)
	}
}
