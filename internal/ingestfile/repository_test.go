package ingestfile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestfile"
)

func TestRepositoryRefreshesStateAcrossLiveInstances(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "ingest-state.json")

	writer, err := ingestfile.NewRepository(storePath)
	if err != nil {
		t.Fatalf("new writer repository failed: %v", err)
	}
	reader, err := ingestfile.NewRepository(storePath)
	if err != nil {
		t.Fatalf("new reader repository failed: %v", err)
	}

	ctx := context.Background()
	hubUserID := "hub:user:alice"
	eventTime := time.Date(2026, 7, 3, 20, 0, 0, 0, time.FixedZone("CST", 8*3600))

	if err := writer.AppendEvents(ctx, hubUserID, []ingest.Event{{
		ID:           "event-1",
		BatchID:      "batch-1",
		HubUserID:    hubUserID,
		Source:       ingest.RunSourceWXCommentExport,
		EventType:    "comment_export",
		CollectedAt:  eventTime,
		Title:        "alpha",
		VideoCount:   1,
		CommentCount: 12,
	}}); err != nil {
		t.Fatalf("append events failed: %v", err)
	}
	if err := writer.AppendRun(ctx, hubUserID, ingest.Run{
		ID:            "run-1",
		Source:        ingest.RunSourceWXCommentExport,
		Status:        "ok",
		StartedAt:     eventTime,
		FinishedAt:    eventTime.Add(2 * time.Minute),
		BatchID:       "batch-1",
		EventCount:    1,
		LatestEventAt: eventTime,
		LastEventType: "comment_export",
	}); err != nil {
		t.Fatalf("append run failed: %v", err)
	}
	if err := writer.AppendOperatorRun(ctx, hubUserID, ingest.OperatorRun{
		ID:               "operator-1",
		Status:           "ok",
		StartedAt:        eventTime,
		FinishedAt:       eventTime.Add(3 * time.Minute),
		AttemptedSources: 1,
		SucceededSources: 1,
		FreshnessState:   "fresh",
		Outcomes: []ingest.OperatorRunOutcome{{
			Source:  ingest.RunSourceWXCommentExport,
			Status:  "success",
			Summary: "批次 batch-1 · 1 条事件",
		}},
	}); err != nil {
		t.Fatalf("append operator run failed: %v", err)
	}
	if err := writer.UpsertBinding(ctx, hubUserID, ingest.SourceBinding{
		Source:    ingest.RunSourceWXCommentExport,
		Path:      "E:/exports/comments.json",
		UpdatedAt: eventTime,
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}

	events, err := reader.ListEvents(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}
	if len(events) != 1 || events[0].ID != "event-1" {
		t.Fatalf("expected live event refresh, got %+v", events)
	}

	runs, err := reader.ListRuns(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list runs failed: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "run-1" {
		t.Fatalf("expected live run refresh, got %+v", runs)
	}

	operatorRuns, err := reader.ListOperatorRuns(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list operator runs failed: %v", err)
	}
	if len(operatorRuns) != 1 || operatorRuns[0].ID != "operator-1" {
		t.Fatalf("expected live operator run refresh, got %+v", operatorRuns)
	}
	if len(operatorRuns[0].Outcomes) != 1 || operatorRuns[0].Outcomes[0].Status != "success" {
		t.Fatalf("expected operator outcomes persistence, got %+v", operatorRuns[0])
	}

	bindings, err := reader.ListBindings(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 || bindings[0].Path != "E:/exports/comments.json" {
		t.Fatalf("expected live binding refresh, got %+v", bindings)
	}
}

func TestRepositoryDeletesOneBindingWithoutDeletingHistory(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "ingest-state.json")
	repo, err := ingestfile.NewRepository(storePath)
	if err != nil {
		t.Fatalf("new repository failed: %v", err)
	}

	ctx := context.Background()
	hubUserID := "hub:user:delete-binding"
	when := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	if err := repo.AppendEvents(ctx, hubUserID, []ingest.Event{{
		ID: "history-1", HubUserID: hubUserID, Source: ingest.RunSourceWXFeedList,
		EventType: "video_snapshot", CollectedAt: when, Title: "保留历史",
	}}); err != nil {
		t.Fatalf("append history failed: %v", err)
	}
	for _, binding := range []ingest.SourceBinding{
		{Source: ingest.RunSourceWXFeedList, Username: "keep@finder", UpdatedAt: when},
		{Source: ingest.RunSourceWXFeedList, Username: "remove@finder", UpdatedAt: when.Add(time.Minute)},
	} {
		if err := repo.UpsertBinding(ctx, hubUserID, binding); err != nil {
			t.Fatalf("upsert binding failed: %v", err)
		}
	}

	if err := repo.DeleteBinding(ctx, hubUserID, ingest.RunSourceWXFeedList, "remove@finder"); err != nil {
		t.Fatalf("delete binding failed: %v", err)
	}
	bindings, err := repo.ListBindings(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 || bindings[0].Username != "keep@finder" {
		t.Fatalf("expected one remaining binding, got %+v", bindings)
	}
	events, err := repo.ListEvents(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list history failed: %v", err)
	}
	if len(events) != 1 || events[0].ID != "history-1" {
		t.Fatalf("expected history to remain, got %+v", events)
	}
	if err := repo.DeleteBinding(ctx, hubUserID, ingest.RunSourceWXFeedList, "missing@finder"); err != ingest.ErrSourceBindingNotFound {
		t.Fatalf("expected not-found sentinel, got %v", err)
	}
}

func TestRepositoryAppendsVideoMetricHistoryAcrossRefreshes(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "ingest-state.json")
	repo, err := ingestfile.NewRepository(storePath)
	if err != nil {
		t.Fatalf("new repository failed: %v", err)
	}

	ctx := context.Background()
	hubUserID := "hub:user:metrics"
	firstAt := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(5 * time.Minute)
	observations := []ingest.VideoMetricObservation{
		{
			ID:           "video-1@" + firstAt.Format(time.RFC3339Nano),
			HubUserID:    hubUserID,
			EventID:      "video-1",
			ObservedAt:   firstAt,
			ObjectID:     "video-1",
			PlayCount:    100,
			LikeCount:    20,
			CommentCount: 3,
			FavCount:     4,
			ForwardCount: 5,
		},
		{
			ID:           "video-1@" + secondAt.Format(time.RFC3339Nano),
			HubUserID:    hubUserID,
			EventID:      "video-1",
			ObservedAt:   secondAt,
			ObjectID:     "video-1",
			PlayCount:    140,
			LikeCount:    28,
			CommentCount: 6,
			FavCount:     7,
			ForwardCount: 9,
		},
	}
	if err := repo.AppendVideoMetricObservations(ctx, hubUserID, observations); err != nil {
		t.Fatalf("append metric history failed: %v", err)
	}

	reloaded, err := ingestfile.NewRepository(storePath)
	if err != nil {
		t.Fatalf("reload repository failed: %v", err)
	}
	got, err := reloaded.ListVideoMetricObservations(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list metric history failed: %v", err)
	}
	if len(got) != 2 || got[0].PlayCount != 100 || got[1].PlayCount != 140 {
		t.Fatalf("expected two historical metric observations, got %+v", got)
	}
}

func TestRepositoryPreservesOriginWhenRefreshOmitsIt(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "ingest-state.json")
	repo, err := ingestfile.NewRepository(storePath)
	if err != nil {
		t.Fatalf("new repository failed: %v", err)
	}

	ctx := context.Background()
	hubUserID := "hub:user:origin"
	updatedAt := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	if err := repo.UpsertBinding(ctx, hubUserID, ingest.SourceBinding{
		Source:    ingest.RunSourceWXFeedList,
		Username:  "account-one",
		Origin:    ingest.BindingOriginMonitoring,
		UpdatedAt: updatedAt,
	}); err != nil {
		t.Fatalf("seed binding failed: %v", err)
	}
	if err := repo.UpsertBinding(ctx, hubUserID, ingest.SourceBinding{
		Source:    ingest.RunSourceWXFeedList,
		Username:  "account-one",
		UpdatedAt: updatedAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("refresh binding failed: %v", err)
	}

	bindings, err := repo.ListBindings(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 || bindings[0].Origin != ingest.BindingOriginMonitoring {
		t.Fatalf("expected one binding with preserved origin, got %+v", bindings)
	}
}
