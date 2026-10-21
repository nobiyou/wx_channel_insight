package normalizepg_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	"github.com/nobiyou/wx_channel_insight/internal/normalizepg"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func openNormalizeTestRepository(t *testing.T) *normalizepg.Repository {
	t.Helper()

	db, _ := testpostgres.Open(t)

	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatalf("new postgres client failed: %v", err)
	}

	repo, err := normalizepg.NewRepository(client)
	if err != nil {
		t.Fatalf("new normalize postgres repository failed: %v", err)
	}
	return repo
}

func TestRepositoryPersistsObjectIdentifiersAcrossSnapshots(t *testing.T) {
	repo := openNormalizeTestRepository(t)
	ctx := context.Background()
	workspaceID := "ws-default-normalize-pg"
	now := time.Date(2026, 7, 7, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
	t.Cleanup(func() {
		_ = repo.Reset(ctx, workspaceID)
	})

	err := repo.ReplaceSnapshot(ctx, workspaceID, normalize.Snapshot{
		Summary: normalize.Summary{
			WorkspaceID:   workspaceID,
			HubUserID:     "hub:user:normalize-pg",
			State:         "ready",
			LatestBatchID: "batch-normalize-pg",
			GeneratedAt:   now,
			LatestEventAt: now,
			ContentCount:  1,
			CommentCount:  1,
			TopicCount:    1,
			AccountCount:  0,
		},
		ContentSnapshots: []normalize.ContentSnapshot{{
			ID:             "content-1",
			BatchID:        "batch-normalize-pg",
			EventID:        "event-content-1",
			Source:         "wx_feed_list",
			CollectedAt:    now,
			ObjectID:       "oid-content-1",
			ObjectNonceID:  "nid-content-1",
			Title:          "content title",
			ChannelName:    "content channel",
			Keyword:        "AI",
			Signal:         "hot",
			PromptAngle:    "angle",
			TargetAudience: "operators",
			VideoCount:     1,
			CommentCount:   10,
			TopicCount:     2,
			Tags:           []string{"tag-1"},
			Keywords:       []string{"kw-1"},
		}},
		CommentSnapshots: []normalize.CommentSnapshot{{
			ID:                    "comment-1",
			BatchID:               "batch-normalize-pg",
			EventID:               "event-comment-1",
			Source:                "wx_comment_export",
			CollectedAt:           now.Add(1 * time.Minute),
			ObjectID:              "oid-comment-1",
			ObjectNonceID:         "nid-comment-1",
			Title:                 "comment title",
			ChannelName:           "comment channel",
			Keyword:               "debate",
			Signal:                "question",
			RepresentativeComment: "代表评论",
			TargetAudience:        "editors",
			VideoCount:            1,
			CommentCount:          33,
			TopicCount:            3,
			Keywords:              []string{"kw-comment"},
		}},
		TopicObservations: []normalize.TopicObservation{{
			ID:             "topic-1",
			BatchID:        "batch-normalize-pg",
			EventID:        "event-topic-1",
			Source:         "wx_comment_export",
			CollectedAt:    now.Add(2 * time.Minute),
			ObjectID:       "oid-topic-1",
			ObjectNonceID:  "nid-topic-1",
			Keyword:        "world-model",
			Cluster:        "ai",
			Signal:         "rising",
			PromptAngle:    "angle-topic",
			TargetAudience: "strategists",
			VideoCount:     1,
			CommentCount:   20,
			TopicCount:     5,
			Keywords:       []string{"kw-topic"},
		}},
		AccountObservations: []normalize.AccountObservation{{
			ID:           "account-1",
			BatchID:      "batch-normalize-pg",
			EventID:      "event-account-1",
			Source:       "wx_video_search",
			CollectedAt:  now.Add(3 * time.Minute),
			Username:     "real-account@finder",
			ChannelName:  "真实账号",
			Title:        "account title",
			Signal:       "rising",
			VideoCount:   2,
			CommentCount: 7,
			TopicCount:   1,
			Tags:         []string{"account-tag"},
		}},
	})
	if err != nil {
		t.Fatalf("replace snapshot failed: %v", err)
	}

	snapshot, found, err := repo.GetSnapshot(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if !found {
		t.Fatalf("expected snapshot to exist")
	}
	if len(snapshot.ContentSnapshots) != 1 || snapshot.ContentSnapshots[0].ObjectID != "oid-content-1" || snapshot.ContentSnapshots[0].ObjectNonceID != "nid-content-1" {
		t.Fatalf("expected content object identifiers to round-trip, got %+v", snapshot.ContentSnapshots)
	}
	if len(snapshot.CommentSnapshots) != 1 || snapshot.CommentSnapshots[0].ObjectID != "oid-comment-1" || snapshot.CommentSnapshots[0].ObjectNonceID != "nid-comment-1" {
		t.Fatalf("expected comment object identifiers to round-trip, got %+v", snapshot.CommentSnapshots)
	}
	if len(snapshot.TopicObservations) != 1 || snapshot.TopicObservations[0].ObjectID != "oid-topic-1" || snapshot.TopicObservations[0].ObjectNonceID != "nid-topic-1" {
		t.Fatalf("expected topic object identifiers to round-trip, got %+v", snapshot.TopicObservations)
	}
	if len(snapshot.AccountObservations) != 1 || snapshot.AccountObservations[0].Username != "real-account@finder" {
		t.Fatalf("expected account username to round-trip, got %+v", snapshot.AccountObservations)
	}
}

func TestRepositorySerializesConcurrentReplaceSnapshot(t *testing.T) {
	repo := openNormalizeTestRepository(t)
	ctx := context.Background()
	workspaceID := "ws-default-normalize-pg-lock"
	now := time.Date(2026, 7, 7, 11, 0, 0, 0, time.FixedZone("CST", 8*3600))
	t.Cleanup(func() {
		_ = repo.Reset(ctx, workspaceID)
	})

	makeSnapshot := func(batchID, suffix string, collectedAt time.Time) normalize.Snapshot {
		return normalize.Snapshot{
			Summary: normalize.Summary{
				WorkspaceID:   workspaceID,
				HubUserID:     "hub:user:normalize-pg",
				State:         "ready",
				LatestBatchID: batchID,
				GeneratedAt:   collectedAt,
				LatestEventAt: collectedAt,
				ContentCount:  1,
				CommentCount:  1,
				TopicCount:    1,
			},
			ContentSnapshots: []normalize.ContentSnapshot{{
				ID:            "shared-content",
				BatchID:       batchID,
				EventID:       "event-content-" + suffix,
				Source:        "wx_feed_list",
				CollectedAt:   collectedAt,
				ObjectID:      "oid-content-" + suffix,
				ObjectNonceID: "nid-content-" + suffix,
				Title:         "shared title",
				ChannelName:   "shared channel",
			}},
			CommentSnapshots: []normalize.CommentSnapshot{{
				ID:                    "shared-comment",
				BatchID:               batchID,
				EventID:               "event-comment-" + suffix,
				Source:                "wx_comment_export",
				CollectedAt:           collectedAt,
				ObjectID:              "oid-comment-" + suffix,
				ObjectNonceID:         "nid-comment-" + suffix,
				Title:                 "shared comment",
				RepresentativeComment: "shared representative",
			}},
			TopicObservations: []normalize.TopicObservation{{
				ID:            "shared-topic",
				BatchID:       batchID,
				EventID:       "event-topic-" + suffix,
				Source:        "wx_comment_export",
				CollectedAt:   collectedAt,
				ObjectID:      "oid-topic-" + suffix,
				ObjectNonceID: "nid-topic-" + suffix,
				Keyword:       "shared keyword",
				Cluster:       "shared cluster",
			}},
		}
	}

	snapshots := []normalize.Snapshot{
		makeSnapshot("batch-lock-1", "1", now),
		makeSnapshot("batch-lock-2", "2", now.Add(1*time.Minute)),
	}

	errCh := make(chan error, len(snapshots))
	var wg sync.WaitGroup
	for _, snapshot := range snapshots {
		wg.Add(1)
		go func(snapshot normalize.Snapshot) {
			defer wg.Done()
			errCh <- repo.ReplaceSnapshot(context.Background(), workspaceID, snapshot)
		}(snapshot)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("expected concurrent replace snapshot to succeed, got %v", err)
		}
	}
}
