package ingestpg_test

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestpg"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func TestRepositoryPersistsEventObjectIdentifiers(t *testing.T) {
	db, _ := testpostgres.Open(t)

	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatalf("new postgres client failed: %v", err)
	}

	repo, err := ingestpg.NewRepository(client)
	if err != nil {
		t.Fatalf("new ingest postgres repository failed: %v", err)
	}

	ctx := context.Background()
	hubUserID := "hub:user:pg-event-test"
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	t.Cleanup(func() {
		_ = repo.Reset(ctx, hubUserID)
	})

	err = repo.AppendEvents(ctx, hubUserID, []ingest.Event{{
		ID:             "event-pg-1",
		BatchID:        "batch-pg-1",
		HubUserID:      hubUserID,
		MachineID:      "machine-pg-1",
		EventType:      "video_snapshot",
		Source:         ingest.RunSourceWXFeedList,
		CollectedAt:    now,
		ObjectID:       "oid-pg-1",
		ObjectNonceID:  "nid-pg-1",
		VideoCount:     1,
		CommentCount:   23,
		TopicCount:     4,
		Title:          "object id persistence",
		ChannelName:    "pg-channel",
		Keyword:        "AI",
		Cluster:        "pg-cluster",
		Signal:         "hot",
		PromptAngle:    "angle",
		TargetAudience: "operators",
		Tags:           []string{"t1"},
		Keywords:       []string{"k1"},
	}})
	if err != nil {
		t.Fatalf("append events failed: %v", err)
	}

	events, err := repo.ListEvents(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %+v", events)
	}
	if events[0].ObjectID != "oid-pg-1" || events[0].ObjectNonceID != "nid-pg-1" {
		t.Fatalf("expected object identifiers to round-trip, got %+v", events[0])
	}

	if err := repo.AppendVideoMetricObservations(ctx, hubUserID, []ingest.VideoMetricObservation{{
		ID:            "video-pg-1@" + now.Format(time.RFC3339Nano),
		HubUserID:     hubUserID,
		EventID:       "event-pg-1",
		BatchID:       "batch-pg-1",
		Source:        ingest.RunSourceWXFeedList,
		ObservedAt:    now,
		ObjectID:      "oid-pg-1",
		ObjectNonceID: "nid-pg-1",
		Title:         "object id persistence",
		ChannelName:   "pg-channel",
		PlayCount:     1000,
		LikeCount:     100,
		CommentCount:  23,
		FavCount:      25,
		ForwardCount:  10,
	}}); err != nil {
		t.Fatalf("append video metric observation failed: %v", err)
	}
	history, err := repo.ListVideoMetricObservations(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list video metric observations failed: %v", err)
	}
	if len(history) != 1 || history[0].PlayCount != 1000 || history[0].LikeCount != 100 || history[0].CommentCount != 23 || history[0].FavCount != 25 || history[0].ForwardCount != 10 {
		t.Fatalf("expected all video metric fields to round-trip, got %+v", history)
	}

	if err := repo.UpsertBinding(ctx, hubUserID, ingest.SourceBinding{
		Source:        ingest.RunSourceWXManualShare,
		ShareURL:      "https://weixin.qq.com/sph/pg-binding-video",
		ObjectID:      "oid-pg-1",
		ObjectNonceID: "nid-pg-1",
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert manual share binding failed: %v", err)
	}
	bindings, err := repo.ListBindings(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 || bindings[0].Source != ingest.RunSourceWXManualShare || bindings[0].TargetKey != "oid-pg-1" || bindings[0].ShareURL == "" || bindings[0].ObjectNonceID != "nid-pg-1" {
		t.Fatalf("expected manual share binding fields to round-trip, got %+v", bindings)
	}
}

func TestRepositoryPersistsOperatorRunOutcomes(t *testing.T) {
	db, _ := testpostgres.Open(t)

	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatalf("new postgres client failed: %v", err)
	}

	repo, err := ingestpg.NewRepository(client)
	if err != nil {
		t.Fatalf("new ingest postgres repository failed: %v", err)
	}

	ctx := context.Background()
	hubUserID := "hub:user:pg-operator-test"
	now := time.Date(2026, 7, 3, 14, 30, 0, 0, time.FixedZone("CST", 8*3600))
	t.Cleanup(func() {
		_ = repo.Reset(ctx, hubUserID)
	})

	err = repo.AppendOperatorRun(ctx, hubUserID, ingest.OperatorRun{
		ID:               "operator-pg-1",
		Status:           "partial",
		StartedAt:        now,
		FinishedAt:       now.Add(2 * time.Minute),
		WorkspaceID:      "ws-default-hub-user-pg",
		LatestBatchID:    "batch-pg-1",
		AttemptedSources: 1,
		SucceededSources: 1,
		FailedSources:    0,
		SkippedSources:   1,
		FreshnessState:   "lagging",
		Message:          "pg operator outcome persistence",
		Outcomes: []ingest.OperatorRunOutcome{
			{
				Source:  ingest.RunSourceWXCommentExport,
				Status:  "success",
				Summary: "批次 batch-pg-1 · 3 条事件",
			},
			{
				Source:  ingest.RunSourceWXFeedList,
				Status:  "skipped",
				Summary: "视频列表来源尚未保存，今天这轮会缺少热视频和热话题输入",
			},
		},
	})
	if err != nil {
		t.Fatalf("append operator run failed: %v", err)
	}

	runs, err := repo.ListOperatorRuns(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list operator runs failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one operator run, got %+v", runs)
	}
	if len(runs[0].Outcomes) != 2 {
		t.Fatalf("expected persisted outcomes, got %+v", runs[0])
	}
	if runs[0].Outcomes[0].Source != ingest.RunSourceWXCommentExport || runs[0].Outcomes[1].Source != ingest.RunSourceWXFeedList {
		t.Fatalf("expected ordered outcomes, got %+v", runs[0].Outcomes)
	}

	if err := repo.Reset(ctx, hubUserID); err != nil {
		t.Fatalf("reset failed: %v", err)
	}
	runs, err = repo.ListOperatorRuns(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list operator runs after reset failed: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected reset operator runs, got %+v", runs)
	}
}
