package ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type videoCoverBackfillTransport struct {
	payload        json.RawMessage
	profilePayload json.RawMessage
	calls          []string
	profileCalls   []string
}

func (t *videoCoverBackfillTransport) SearchAccounts(context.Context, string) (json.RawMessage, error) {
	return nil, nil
}

func (t *videoCoverBackfillTransport) SearchVideos(_ context.Context, keyword string) (json.RawMessage, error) {
	t.calls = append(t.calls, keyword)
	return t.payload, nil
}

func (t *videoCoverBackfillTransport) FetchVideoProfile(_ context.Context, objectID, _ string) (json.RawMessage, error) {
	t.profileCalls = append(t.profileCalls, objectID)
	return t.profilePayload, nil
}

func (t *videoCoverBackfillTransport) FetchFeedList(context.Context, string) (json.RawMessage, error) {
	return nil, nil
}

func TestBackfillWXVideoCoversUpdatesEventAndAppendsObservationWithoutBinding(t *testing.T) {
	hubUserID := "hub:user:cover-backfill"
	observedAt := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	backfillAt := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	repo := &backfillTestRepository{
		events: []Event{{
			ID:          "wx-video-search-video-1",
			HubUserID:   hubUserID,
			EventType:   "video_snapshot",
			Source:      RunSourceWXVideoSearch,
			CollectedAt: observedAt,
			PublishedAt: observedAt.Add(-24 * time.Hour),
			ObjectID:    "video-1",
			Keyword:     "折叠屏",
			Title:       "封面回填测试",
			LikeCount:   10,
		}},
		observations: []VideoMetricObservation{{
			ID:          "wx-video-search-video-1@" + observedAt.Format(time.RFC3339Nano),
			HubUserID:   hubUserID,
			EventID:     "wx-video-search-video-1",
			BatchID:     "batch-old",
			Source:      RunSourceWXVideoSearch,
			ObservedAt:  observedAt,
			PublishedAt: observedAt.Add(-24 * time.Hour),
			ObjectID:    "video-1",
			Keyword:     "折叠屏",
			Title:       "封面回填测试",
			LikeCount:   10,
		}},
	}
	transport := &videoCoverBackfillTransport{payload: json.RawMessage(`{
  "code": 0,
  "data": {"data": {"objectList": [{
    "id": "video-1",
    "objectDesc": {"description": "封面回填测试", "media": [{"coverUrl": "https://cdn.example.test/video-1.jpg"}]}
  }]}}
}`)}
	service := NewService(repo, ServiceConfig{
		DefaultHubUserID: hubUserID,
		Now:              func() time.Time { return backfillAt },
		WXFeedListClient: WXFeedListClient{Transport: transport},
	})

	result, err := service.BackfillWXVideoCovers(context.Background(), SummaryQuery{HubUserID: hubUserID})
	if err != nil {
		t.Fatalf("backfill failed: %v", err)
	}
	if result.Status != "success" || result.CandidateCount != 1 || result.MatchedCount != 1 || result.EventsUpdated != 1 || result.ObservationsAppended != 1 {
		t.Fatalf("unexpected backfill result: %+v", result)
	}
	if len(transport.calls) != 1 || transport.calls[0] != "折叠屏" {
		t.Fatalf("expected one grouped search call, got %+v", transport.calls)
	}

	events, err := repo.ListEvents(context.Background(), hubUserID)
	if err != nil {
		t.Fatalf("list events after backfill: %v", err)
	}
	if len(events) != 1 || events[0].CoverURL != "https://cdn.example.test/video-1.jpg" {
		t.Fatalf("expected event cover to be updated, got %+v", events)
	}
	observations, err := repo.ListVideoMetricObservations(context.Background(), hubUserID)
	if err != nil {
		t.Fatalf("list observations after backfill: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("expected appended observation with cover, got %+v", observations)
	}
	var coveredObservation VideoMetricObservation
	coveredObservationFound := false
	for _, observation := range observations {
		if observation.CoverURL == "https://cdn.example.test/video-1.jpg" {
			coveredObservation = observation
			coveredObservationFound = true
			break
		}
	}
	if !coveredObservationFound || coveredObservation.BatchID == "batch-old" {
		t.Fatalf("expected appended observation with cover, got %+v", observations)
	}

	second, err := service.BackfillWXVideoCovers(context.Background(), SummaryQuery{HubUserID: hubUserID})
	if err != nil {
		t.Fatalf("second backfill failed: %v", err)
	}
	observationsAfterSecond, err := repo.ListVideoMetricObservations(context.Background(), hubUserID)
	if err != nil {
		t.Fatalf("list observations after second backfill: %v", err)
	}
	if second.Status != "noop" || len(transport.calls) != 1 || len(observationsAfterSecond) != 2 {
		t.Fatalf("expected idempotent second backfill, result=%+v calls=%d observations=%d", second, len(transport.calls), len(observationsAfterSecond))
	}
}

func TestBackfillWXVideoCoversFallsBackToExactProfileForHistoricalVideo(t *testing.T) {
	hubUserID := "hub:user:cover-profile"
	observedAt := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	backfillAt := time.Date(2026, 8, 9, 11, 0, 0, 0, time.UTC)
	repo := &backfillTestRepository{events: []Event{{
		ID:            "wx-video-search-video-profile-1",
		HubUserID:     hubUserID,
		EventType:     "video_snapshot",
		Source:        RunSourceWXVideoSearch,
		CollectedAt:   observedAt,
		ObjectID:      "video-profile-1",
		ObjectNonceID: "nonce-profile-1",
		Keyword:       "历史关键词",
		Title:         "历史视频",
		LikeCount:     20,
		CommentCount:  3,
		FavCount:      4,
		ForwardCount:  5,
	}}}
	transport := &videoCoverBackfillTransport{
		payload: json.RawMessage(`{"code":0,"data":{"data":{"objectList":[]}}}`),
		profilePayload: json.RawMessage(`{
  "code": 0,
  "data": {"data": {"object": {
    "id": "video-profile-1",
    "createtime": 1785901095,
    "objectDesc": {"description": "历史视频", "media": [{"thumbUrl": "https://cdn.example.test/profile-1-thumb.jpg", "coverUrl": "https://cdn.example.test/profile-1-cover.jpg"}]}
  }}}
}`),
	}
	service := NewService(repo, ServiceConfig{
		DefaultHubUserID: hubUserID,
		Now:              func() time.Time { return backfillAt },
		WXFeedListClient: WXFeedListClient{Transport: transport},
	})

	result, err := service.BackfillWXVideoCovers(context.Background(), SummaryQuery{HubUserID: hubUserID})
	if err != nil {
		t.Fatalf("backfill failed: %v", err)
	}
	if result.Status != "success" || result.ProfileAttempts != 1 || result.ProfileMatched != 1 || result.EventsUpdated != 2 || result.ObservationsAppended != 1 {
		t.Fatalf("unexpected profile fallback result: %+v", result)
	}
	if len(transport.profileCalls) != 1 || transport.profileCalls[0] != "video-profile-1" {
		t.Fatalf("expected one exact profile call, got %+v", transport.profileCalls)
	}

	events, err := repo.ListEvents(context.Background(), hubUserID)
	if err != nil {
		t.Fatalf("list events after profile backfill: %v", err)
	}
	if len(events) != 1 || events[0].CoverURL != "https://cdn.example.test/profile-1-thumb.jpg" || !events[0].PublishedAt.Equal(time.Unix(1785901095, 0).UTC()) {
		t.Fatalf("expected profile thumb cover on event, got %+v", events)
	}
}
