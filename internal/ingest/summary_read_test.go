package ingest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
)

type boundedSummaryRepository struct {
	*ingestdemo.Repository
	eventFacts            ingest.EventSummaryFacts
	summaryRuns           []ingest.Run
	autoRunFacts          []ingest.SourceAutoRunFact
	eventFactsCalls       int
	summaryRunsCalls      int
	autoRunFactsCalls     int
	listEventsCalls       int
	listRunsCalls         int
	listOperatorRunsCalls int
}

func (r *boundedSummaryRepository) GetEventSummaryFacts(_ context.Context, _ string, _ int) (ingest.EventSummaryFacts, error) {
	r.eventFactsCalls++
	return r.eventFacts, nil
}

func (r *boundedSummaryRepository) ListSummaryRuns(_ context.Context, _ string, _ int) ([]ingest.Run, error) {
	r.summaryRunsCalls++
	return append([]ingest.Run(nil), r.summaryRuns...), nil
}

func (r *boundedSummaryRepository) ListEvents(context.Context, string) ([]ingest.Event, error) {
	r.listEventsCalls++
	return nil, errors.New("unbounded event read must not be used for summary")
}

func (r *boundedSummaryRepository) ListRuns(context.Context, string) ([]ingest.Run, error) {
	r.listRunsCalls++
	return nil, errors.New("unbounded run read must not be used for summary")
}

func (r *boundedSummaryRepository) GetSourceAutoRunFacts(context.Context, string) ([]ingest.SourceAutoRunFact, error) {
	r.autoRunFactsCalls++
	return append([]ingest.SourceAutoRunFact(nil), r.autoRunFacts...), nil
}

func (r *boundedSummaryRepository) ListOperatorRuns(context.Context, string) ([]ingest.OperatorRun, error) {
	r.listOperatorRunsCalls++
	return nil, errors.New("operator run history must not be used for summary")
}

func TestServiceGetSummaryReadOnlyUsesBoundedRepositoryFacts(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	latest := ingest.Event{
		ID:           "event-latest",
		BatchID:      "batch-latest",
		HubUserID:    "insight:operator:local",
		MachineID:    "machine-01",
		EventType:    "video_snapshot",
		Source:       ingest.RunSourceWXFeedList,
		CollectedAt:  now.Add(-time.Minute),
		VideoCount:   1,
		CommentCount: 7,
		TopicCount:   2,
		Title:        "latest video",
		ChannelName:  "latest account",
	}
	repo := &boundedSummaryRepository{
		Repository: ingestdemo.NewRepository(),
		eventFacts: ingest.EventSummaryFacts{
			TotalEvents:        5000,
			SourceCount:        17,
			AggregatedVideos:   7000,
			AggregatedComments: 9000,
			AggregatedTopics:   3000,
			LatestEvent:        latest,
			RecentEvents:       []ingest.Event{latest},
			Sources: []ingest.EventSourceFacts{{
				Source:      ingest.RunSourceWXFeedList,
				EventCount:  5000,
				LatestEvent: latest,
			}},
		},
		summaryRuns: []ingest.Run{
			{
				ID:         "run-success",
				Source:     ingest.RunSourceWXFeedList,
				Status:     "success",
				FinishedAt: now.Add(-2 * time.Minute),
				BatchID:    "batch-latest",
				EventCount: 5000,
			},
			{
				ID:         "run-failed",
				Source:     ingest.RunSourceWXFeedList,
				Status:     "failed",
				FinishedAt: now.Add(-time.Hour),
				Message:    "historical failure",
			},
		},
		autoRunFacts: []ingest.SourceAutoRunFact{{
			Source:        ingest.RunSourceWXFeedList,
			LastRunStatus: "success",
			LastRunAt:     now.Add(-3 * time.Minute),
			LastSuccessAt: now.Add(-3 * time.Minute),
			LastBatchID:   "batch-auto",
		}},
	}
	if err := repo.Repository.UpsertBinding(context.Background(), "insight:operator:local", ingest.SourceBinding{
		Source:    ingest.RunSourceWXFeedList,
		BaseURL:   "http://127.0.0.1:2026",
		Username:  "account-a",
		UpdatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed binding failed: %v", err)
	}
	service := ingest.NewService(repo, ingest.ServiceConfig{Now: func() time.Time { return now }})

	summary, err := service.GetSummaryReadOnly(context.Background(), ingest.SummaryQuery{
		HubUserID: "insight:operator:local",
	})
	if err != nil {
		t.Fatalf("get bounded summary failed: %v", err)
	}
	if repo.eventFactsCalls != 1 || repo.summaryRunsCalls != 1 || repo.autoRunFactsCalls != 1 {
		t.Fatalf("expected one bounded event/run/auto-fact read, got event=%d runs=%d auto=%d", repo.eventFactsCalls, repo.summaryRunsCalls, repo.autoRunFactsCalls)
	}
	if repo.listEventsCalls != 0 || repo.listRunsCalls != 0 || repo.listOperatorRunsCalls != 0 {
		t.Fatalf("unbounded repository reads were used: events=%d runs=%d operatorRuns=%d", repo.listEventsCalls, repo.listRunsCalls, repo.listOperatorRunsCalls)
	}
	if summary.TotalEvents != 5000 || summary.SourceCount != 17 || summary.AggregatedVideos != 7000 || summary.AggregatedComments != 9000 || summary.AggregatedTopics != 3000 {
		t.Fatalf("expected exact aggregate facts, got %+v", summary)
	}
	if len(summary.RecentEvents) != 1 || summary.RecentEvents[0].ID != latest.ID {
		t.Fatalf("expected bounded recent events, got %+v", summary.RecentEvents)
	}
	if len(summary.SourceStatuses) != 1 || summary.SourceStatuses[0].EventCount != 5000 {
		t.Fatalf("expected exact per-source facts, got %+v", summary.SourceStatuses)
	}
	if !summary.LastSuccessAt.Equal(now.Add(-2*time.Minute)) || !summary.LastFailureAt.Equal(now.Add(-time.Hour)) {
		t.Fatalf("expected latest success/failure facts, got success=%s failure=%s", summary.LastSuccessAt, summary.LastFailureAt)
	}
	if len(summary.BindingStates) != 1 || summary.BindingStates[0].LastAutoBatchID != "batch-auto" {
		t.Fatalf("expected bounded auto-run facts on the saved binding, got %+v", summary.BindingStates)
	}
}
