package insightpg_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightpg"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func openRecommendationTestRepository(t *testing.T) (*insightpg.RecommendationRepository, *sql.DB) {
	t.Helper()
	db, _ := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatalf("new postgres client failed: %v", err)
	}
	repo, err := insightpg.NewRecommendationRepository(client)
	if err != nil {
		t.Fatalf("new recommendation postgres repository failed: %v", err)
	}
	return repo, db
}

func TestRecommendationRepositoryRoundTripsSnapshot(t *testing.T) {
	repo, _ := openRecommendationTestRepository(t)
	ctx := context.Background()
	workspaceID := "ws-recommendation-pg"
	t.Cleanup(func() { _ = repo.ResetRecommendations(ctx, workspaceID) })

	want := insight.RecommendationSnapshot{
		WorkspaceID: workspaceID,
		AccountRecommendations: []insight.AccountRecommendation{{
			Username: "candidate-pg",
			Metrics: insight.RecommendationMetrics{
				Likes:     21,
				Comments:  5,
				Favorites: 3,
				Forwards:  2,
			},
		}},
	}
	for _, test := range []struct {
		name   string
		topics []insight.TopicRecommendation
	}{
		{name: "populated", topics: []insight.TopicRecommendation{{ID: "topic-pg", Keyword: "秋天"}}},
		{name: "null"},
		{name: "empty", topics: []insight.TopicRecommendation{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			want.TopicRecommendations = test.topics
			if err := repo.ReplaceRecommendations(ctx, workspaceID, want); err != nil {
				t.Fatalf("replace recommendations failed: %v", err)
			}
			got, found, err := repo.GetRecommendations(ctx, workspaceID)
			if err != nil || !found {
				t.Fatalf("get recommendations failed: found=%v err=%v", found, err)
			}
			if len(got.AccountRecommendations) != 1 || got.AccountRecommendations[0].Username != "candidate-pg" || got.AccountRecommendations[0].Metrics.Likes != 21 {
				t.Fatalf("unexpected postgres recommendation: %+v", got)
			}
			if len(got.TopicRecommendations) != len(test.topics) {
				t.Fatalf("unexpected keyword facts: %+v", got.TopicRecommendations)
			}
			if len(test.topics) > 0 && got.TopicRecommendations[0].ID != test.topics[0].ID {
				t.Fatalf("unexpected keyword fact: %+v", got.TopicRecommendations)
			}
		})
	}
}
