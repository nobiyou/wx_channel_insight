package aggregatepg_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/aggregatepg"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func openAggregateTestRepository(t *testing.T) *aggregatepg.Repository {
	t.Helper()

	db, _ := testpostgres.Open(t)

	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatalf("new postgres client failed: %v", err)
	}

	repo, err := aggregatepg.NewRepository(client)
	if err != nil {
		t.Fatalf("new aggregate postgres repository failed: %v", err)
	}
	return repo
}

func TestRepositorySerializesConcurrentReplaceAggregate(t *testing.T) {
	repo := openAggregateTestRepository(t)
	ctx := context.Background()
	workspaceID := "ws-default-aggregate-pg-lock"
	now := time.Date(2026, 7, 7, 11, 30, 0, 0, time.FixedZone("CST", 8*3600))
	t.Cleanup(func() {
		_ = repo.Reset(ctx, workspaceID)
	})

	makeAggregate := func(batchID, suffix string, observedAt time.Time) aggregate.Aggregate {
		return aggregate.Aggregate{
			Summary: aggregate.Summary{
				WorkspaceID:        workspaceID,
				HubUserID:          "hub:user:aggregate-pg",
				State:              "ready",
				LatestBatchID:      batchID,
				GeneratedAt:        observedAt,
				LatestObservedAt:   observedAt,
				VideoMetricCount:   1,
				TopicMetricCount:   1,
				CommentFactCount:   1,
				SourceAccountCount: 1,
			},
			VideoMetrics: []aggregate.VideoMetric{{
				ID:            "shared-video",
				EventID:       "event-video-" + suffix,
				BatchID:       batchID,
				Source:        "wx_feed_list",
				ObservedAt:    observedAt,
				ObjectID:      "oid-video-" + suffix,
				ObjectNonceID: "nid-video-" + suffix,
				Title:         "shared title",
				ChannelName:   "shared channel",
			}},
			TopicMetrics: []aggregate.TopicMetric{{
				ID:            "shared-topic",
				EventID:       "event-topic-" + suffix,
				BatchID:       batchID,
				Source:        "wx_comment_export",
				ObservedAt:    observedAt,
				Keyword:       "shared keyword",
				Cluster:       "shared cluster",
				Mentions:      1,
				GrowthRate:    1,
				RelatedVideos: 1,
			}},
			CommentInsightFacts: []aggregate.CommentInsightFact{{
				ID:            "shared-comment",
				EventID:       "event-comment-" + suffix,
				BatchID:       batchID,
				Source:        "wx_comment_export",
				ObservedAt:    observedAt,
				ObjectID:      "oid-comment-" + suffix,
				ObjectNonceID: "nid-comment-" + suffix,
				Theme:         "shared theme",
			}},
		}
	}

	aggregates := []aggregate.Aggregate{
		makeAggregate("batch-aggregate-1", "1", now),
		makeAggregate("batch-aggregate-2", "2", now.Add(1*time.Minute)),
	}

	errCh := make(chan error, len(aggregates))
	var wg sync.WaitGroup
	for _, item := range aggregates {
		wg.Add(1)
		go func(item aggregate.Aggregate) {
			defer wg.Done()
			errCh <- repo.ReplaceAggregate(context.Background(), workspaceID, item)
		}(item)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("expected concurrent replace aggregate to succeed, got %v", err)
		}
	}
}
