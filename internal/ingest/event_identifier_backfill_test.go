package ingest

import (
	"context"
	"testing"
	"time"
)

type backfillTestRepository struct {
	events       []Event
	observations []VideoMetricObservation
	runs         []Run
}

func (r *backfillTestRepository) ListEvents(_ context.Context, hubUserID string) ([]Event, error) {
	filtered := make([]Event, 0, len(r.events))
	for _, event := range r.events {
		if event.HubUserID == hubUserID {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

func (r *backfillTestRepository) AppendEvents(_ context.Context, hubUserID string, events []Event) error {
	for _, event := range events {
		next := event
		if next.HubUserID == "" {
			next.HubUserID = hubUserID
		}
		r.events = append(r.events, next)
	}
	return nil
}

func (r *backfillTestRepository) ReplaceEvents(_ context.Context, hubUserID string, events []Event) error {
	filtered := make([]Event, 0, len(r.events))
	for _, event := range r.events {
		if event.HubUserID != hubUserID {
			filtered = append(filtered, event)
		}
	}
	r.events = filtered
	return r.AppendEvents(context.Background(), hubUserID, events)
}

func (r *backfillTestRepository) ListVideoMetricObservations(_ context.Context, hubUserID string) ([]VideoMetricObservation, error) {
	filtered := make([]VideoMetricObservation, 0, len(r.observations))
	for _, item := range r.observations {
		if item.HubUserID == hubUserID {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (r *backfillTestRepository) AppendVideoMetricObservations(_ context.Context, hubUserID string, observations []VideoMetricObservation) error {
	for _, item := range observations {
		if item.HubUserID == "" {
			item.HubUserID = hubUserID
		}
		r.observations = append(r.observations, item)
	}
	return nil
}

func (r *backfillTestRepository) ListRuns(_ context.Context, hubUserID string) ([]Run, error) {
	filtered := make([]Run, 0, len(r.runs))
	for _, run := range r.runs {
		if run.ID != "" && hubUserID != "" {
			filtered = append(filtered, run)
		}
	}
	return filtered, nil
}

func (r *backfillTestRepository) AppendRun(_ context.Context, _ string, run Run) error {
	r.runs = append(r.runs, run)
	return nil
}

func (r *backfillTestRepository) ListOperatorRuns(context.Context, string) ([]OperatorRun, error) {
	return nil, nil
}

func (r *backfillTestRepository) AppendOperatorRun(context.Context, string, OperatorRun) error {
	return nil
}

func (r *backfillTestRepository) ListBindings(context.Context, string) ([]SourceBinding, error) {
	return nil, nil
}

func (r *backfillTestRepository) UpsertBinding(context.Context, string, SourceBinding) error {
	return nil
}

func (r *backfillTestRepository) DeleteBinding(context.Context, string, string, string) error {
	return nil
}

func (r *backfillTestRepository) Reset(context.Context, string) error {
	r.events = nil
	r.runs = nil
	return nil
}

func TestBackfillEventObjectIdentifiersMatchesContainedVideoTitles(t *testing.T) {
	base := time.Date(2026, 7, 7, 19, 38, 37, 0, time.FixedZone("CST", 8*3600))
	events := []Event{
		{
			ID:           "wx-feed-video-contained",
			HubUserID:    "hub:user:test",
			EventType:    "video_snapshot",
			Source:       "wx_channel_feed_list",
			CollectedAt:  base,
			VideoCount:   1,
			CommentCount: 20,
			TopicCount:   3,
			Title:        "Don't be limited to my circle of friends.⭕ What do you need? Outdoor lamps\n\nDon't worry and ask the price boldly",
			ChannelName:  "TEAMLIGHT-",
		},
		{
			ID:            "wx-comment-video-contained",
			HubUserID:     "hub:user:test",
			EventType:     "video_snapshot",
			Source:        "finderGetCommentList",
			CollectedAt:   base.Add(-4 * time.Hour),
			ObjectID:      "14460821406575827104",
			ObjectNonceID: "4322735004537466098_0_140_2_32_5217834106691783_demo",
			VideoCount:    1,
			CommentCount:  207,
			TopicCount:    4,
			Title:         "Don't be limited to my circle of friends.",
			ChannelName:   "TEAMLIGHT-",
		},
	}

	result := backfillEventObjectIdentifiers(events)
	if result[0].ObjectID != events[1].ObjectID || result[0].ObjectNonceID != events[1].ObjectNonceID {
		t.Fatalf("expected helper to backfill identifiers, got %+v", result[0])
	}
}

func TestListEventsBackfillsFeedListIdentifiersFromRelatedVideoSnapshot(t *testing.T) {
	repo := &backfillTestRepository{}
	service := NewService(repo, ServiceConfig{
		DefaultHubUserID: "hub:user:test",
		Now: func() time.Time {
			return time.Date(2026, 7, 8, 0, 10, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	hubUserID := "hub:user:test"
	base := time.Date(2026, 7, 7, 19, 38, 37, 0, time.FixedZone("CST", 8*3600))
	feedListEvent := Event{
		ID:           "wx-feed-video-1",
		HubUserID:    hubUserID,
		EventType:    "video_snapshot",
		Source:       "wx_channel_feed_list",
		CollectedAt:  base,
		VideoCount:   1,
		CommentCount: 20,
		TopicCount:   3,
		Title:        "Don't be limited to my circle of friends.⭕ What do you need? Outdoor lamps\n\nDon't worry and ask the price boldly",
		ChannelName:  "TEAMLIGHT-",
		Tags:         []string{"Don't", "be", "limited"},
	}
	commentVideoEvent := Event{
		ID:            "wx-comment-video-1",
		HubUserID:     hubUserID,
		EventType:     "video_snapshot",
		Source:        "finderGetCommentList",
		CollectedAt:   base.Add(-4 * time.Hour),
		ObjectID:      "14460821406575827104",
		ObjectNonceID: "4322735004537466098_0_140_2_32_5217834106691783_demo",
		VideoCount:    1,
		CommentCount:  207,
		TopicCount:    4,
		Title:         "Don't be limited to my circle of friends.",
		ChannelName:   "TEAMLIGHT-",
	}
	if err := repo.AppendEvents(context.Background(), hubUserID, []Event{
		feedListEvent,
		commentVideoEvent,
	}); err != nil {
		t.Fatalf("append events failed: %v", err)
	}

	events, err := service.ListEvents(context.Background(), SummaryQuery{
		HubUserID: hubUserID,
	})
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}

	var matched *Event
	for index := range events {
		if events[index].ID == feedListEvent.ID {
			matched = &events[index]
			break
		}
	}
	if matched == nil {
		t.Fatalf("expected feed-list event in results, got %+v", events)
	}
	if matched.ObjectID != commentVideoEvent.ObjectID || matched.ObjectNonceID != commentVideoEvent.ObjectNonceID {
		t.Fatalf("expected feed-list event identifiers to be backfilled, got %+v", matched)
	}
}

func TestGetSummaryBackfillsRecentFeedListIdentifiersFromRelatedVideoSnapshot(t *testing.T) {
	repo := &backfillTestRepository{}
	service := NewService(repo, ServiceConfig{
		DefaultHubUserID: "hub:user:test",
		Now: func() time.Time {
			return time.Date(2026, 7, 8, 0, 20, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	hubUserID := "hub:user:test"
	base := time.Date(2026, 7, 7, 19, 38, 37, 0, time.FixedZone("CST", 8*3600))
	feedListEvent := Event{
		ID:           "wx-feed-video-2",
		HubUserID:    hubUserID,
		EventType:    "video_snapshot",
		Source:       "wx_channel_feed_list",
		CollectedAt:  base,
		VideoCount:   1,
		CommentCount: 20,
		TopicCount:   3,
		Title:        "Proxy-safe outdoor lamps and circle of friends sample title",
		ChannelName:  "TEAMLIGHT-",
		Tags:         []string{"outdoor", "lamps", "sample"},
	}
	commentVideoEvent := Event{
		ID:            "wx-comment-video-2",
		HubUserID:     hubUserID,
		EventType:     "video_snapshot",
		Source:        "finderGetCommentList",
		CollectedAt:   base.Add(-2 * time.Hour),
		ObjectID:      "oid-comment-2",
		ObjectNonceID: "nid-comment-2",
		VideoCount:    1,
		CommentCount:  80,
		TopicCount:    3,
		Title:         "Proxy-safe outdoor lamps and circle of friends",
		ChannelName:   "TEAMLIGHT-",
	}
	if err := repo.AppendEvents(context.Background(), hubUserID, []Event{
		feedListEvent,
		commentVideoEvent,
	}); err != nil {
		t.Fatalf("append events failed: %v", err)
	}

	summary, err := service.GetSummary(context.Background(), SummaryQuery{
		HubUserID: hubUserID,
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}

	var matched *Event
	for index := range summary.RecentEvents {
		if summary.RecentEvents[index].ID == feedListEvent.ID {
			matched = &summary.RecentEvents[index]
			break
		}
	}
	if matched == nil {
		t.Fatalf("expected feed-list recent event in summary, got %+v", summary.RecentEvents)
	}
	if matched.ObjectID != commentVideoEvent.ObjectID || matched.ObjectNonceID != commentVideoEvent.ObjectNonceID {
		t.Fatalf("expected recent feed-list event identifiers to be backfilled, got %+v", matched)
	}
}
