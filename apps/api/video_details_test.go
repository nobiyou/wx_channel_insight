package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

type savedEventVideoIngest struct {
	historyOnlyIngest
	events []ingest.Event
}

func TestVideoDetailSeparatesComparableDeltasFromUndefinedGrowthRate(t *testing.T) {
	base := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	srv := newServer(nil, nil, stubIngestService{observations: []ingest.VideoMetricObservation{
		{ID: "zero", ObjectID: "new-video", ObservedAt: base},
		{ID: "positive", ObjectID: "new-video", ObservedAt: base.Add(time.Hour), LikeCount: 20, CommentCount: 3},
	}}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/new-video", nil))
	var body struct {
		GrowthStatus              string                               `json:"growthStatus"`
		GrowthRate                *float64                             `json:"growthRate"`
		HasGrowthBaseline         bool                                 `json:"hasGrowthBaseline"`
		HasComparableInteractions bool                                 `json:"hasComparableInteractions"`
		PositiveDeltas            struct{ Likes, Comments, Total int } `json:"positiveDeltas"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || body.GrowthStatus != "zero_baseline" || body.GrowthRate != nil || body.HasGrowthBaseline || !body.HasComparableInteractions || body.PositiveDeltas.Likes != 20 || body.PositiveDeltas.Comments != 3 || body.PositiveDeltas.Total != 23 {
		t.Fatalf("absolute increments hidden or percentage invented: %d %s", rec.Code, rec.Body.String())
	}
}

func (s savedEventVideoIngest) ListEvents(context.Context, ingest.SummaryQuery) ([]ingest.Event, error) {
	return s.events, nil
}

func TestVideoDetailReadsSavedEventWhenMetricHistoryAbsent(t *testing.T) {
	srv := newServer(nil, nil, savedEventVideoIngest{events: []ingest.Event{{ID: "comment-video", ObjectID: "saved-video", EventType: "video_snapshot", Source: "edge_fetch_feed_comment_list", CollectedAt: time.Now(), Title: "已保存快照", CommentCount: 19}}}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/saved-video", nil))
	var detail struct {
		Title    string            `json:"title"`
		History  []json.RawMessage `json:"history"`
		Playback struct {
			Status string `json:"status"`
		} `json:"playback"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(detail.History) != 1 || detail.Title != "已保存快照" || detail.Playback.Status != "deferred" {
		t.Fatalf("saved event unavailable: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleVideoDetailReturnsHistoryAndRequestTimePlaybackURL(t *testing.T) {
	firstAt := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Hour)
	srv := newServer(nil, nil, stubIngestService{
		observations: []ingest.VideoMetricObservation{
			{ID: "observation-2", ObjectID: "object-1", ObjectNonceID: "nonce-1", ObservedAt: secondAt, Title: "详情标题", LikeCount: 22, CommentCount: 8, FavCount: 5, ForwardCount: 3},
			{ID: "observation-1", ObjectID: "object-1", ObjectNonceID: "nonce-1", ObservedAt: firstAt, Title: "详情标题", LikeCount: 12, CommentCount: 5, FavCount: 2, ForwardCount: 1},
		},
		videoProfile: ingest.WXFeedListItem{
			ID: "object-1", ObjectNonceID: "nonce-1", Title: "详情标题", CoverURL: "https://cdn.example.test/cover.jpg",
			VideoURL: "https://cdn.example.test/video.mp4?temporary=1", DecryptKey: "123456789",
		},
	}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/object-1", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		ObjectID         string `json:"objectId"`
		ObservationCount int    `json:"observationCount"`
		Metrics          struct {
			Likes     int `json:"likes"`
			Comments  int `json:"comments"`
			Favorites int `json:"favorites"`
			Forwards  int `json:"forwards"`
		} `json:"metrics"`
		History  []map[string]any `json:"history"`
		Playback struct {
			Status string `json:"status"`
			URL    string `json:"url"`
		} `json:"playback"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if payload.ObjectID != "object-1" || payload.ObservationCount != 2 || len(payload.History) != 2 {
		t.Fatalf("unexpected detail shape: %+v", payload)
	}
	if payload.Metrics.Likes != 22 || payload.Metrics.Comments != 8 || payload.Metrics.Favorites != 5 || payload.Metrics.Forwards != 3 {
		t.Fatalf("unexpected four metrics: %+v", payload.Metrics)
	}
	if payload.Playback.Status != "deferred" || payload.Playback.URL != "" {
		t.Fatalf("expected independent playback loading, got %+v", payload.Playback)
	}
	playback := httptest.NewRecorder()
	srv.routes().ServeHTTP(playback, httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/object-1/playback", nil))
	if playback.Code != 200 || !strings.Contains(playback.Body.String(), `"status":"ready"`) || !strings.Contains(playback.Body.String(), "/api/v1/read-models/videos/play/object-1?nonce_id=nonce-1") {
		t.Fatalf("playback failed: %s", playback.Body.String())
	}
}

func TestHandleVideoDetailDoesNotFabricatePlaybackWhenProfileFails(t *testing.T) {
	srv := newServer(nil, nil, stubIngestService{
		observations:    []ingest.VideoMetricObservation{{ID: "observation-1", ObjectID: "object-1", ObservedAt: time.Now().UTC(), LikeCount: 1}},
		videoProfileErr: errors.New("no ready profile client"),
	}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/object-1/playback", strings.NewReader(""))
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected detail to remain available, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Status  string `json:"status"`
		URL     string `json:"url"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode unavailable playback response: %v", err)
	}
	if payload.Status != "unavailable" || payload.URL != "" || payload.Message == "" {
		t.Fatalf("expected explicit unavailable playback state, got %+v", payload)
	}
}

type historyOnlyIngest struct{ stubIngestService }

func (s historyOnlyIngest) FetchWXVideoProfile(context.Context, string, string) (ingest.WXFeedListItem, error) {
	panic("history must not call live upstream")
}

func TestVideoHistoryLoadsWithoutLiveUpstream(t *testing.T) {
	srv := newServer(nil, nil, historyOnlyIngest{stubIngestService{observations: []ingest.VideoMetricObservation{{ID: "saved", ObjectID: "video", ObservedAt: time.Now(), LikeCount: 5}}}}, nil, runtime.Status{})
	response := httptest.NewRecorder()
	srv.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/video", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"likes":5`) {
		t.Fatalf("saved history unavailable: %d %s", response.Code, response.Body.String())
	}
}

func TestHandleVideoCommentsFiltersByVideoAndPaginates(t *testing.T) {
	base := time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)
	comments := make([]ingest.RawCommentRecord, 0, 22)
	for i := 0; i < 21; i++ {
		comments = append(comments, ingest.RawCommentRecord{
			ID: fmt.Sprintf("object-1\x00comment-%02d", i), VideoObjectID: "object-1",
			VideoObjectNonceID: "nonce-1", CommentID: fmt.Sprintf("comment-%02d", i),
			Content: fmt.Sprintf("评论 %02d", i), CollectedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	comments = append(comments, ingest.RawCommentRecord{
		ID: "object-2\x00other-comment", VideoObjectID: "object-2", CommentID: "other-comment",
		Content: "其他视频评论", CollectedAt: base.Add(30 * time.Minute),
	})
	srv := newServer(nil, nil, stubIngestService{
		rawComments: ingest.RawCommentPage{Items: comments},
	}, nil, runtime.Status{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/object-1/comments?page=2&pageSize=20", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload ingest.RawCommentPage
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode comments response: %v", err)
	}
	if payload.Page != 2 || payload.PageSize != 20 || payload.Total != 21 || payload.TotalPages != 2 || len(payload.Items) != 1 {
		t.Fatalf("unexpected pagination: %+v", payload)
	}
	if payload.Items[0].VideoObjectID != "object-1" || payload.Items[0].CommentID != "comment-00" {
		t.Fatalf("expected only object-1 comments in descending order, got %+v", payload.Items[0])
	}
}

func TestHandleVideoCommentsRejectsUnsupportedMethod(t *testing.T) {
	srv := newServer(nil, nil, stubIngestService{}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/read-models/videos/object-1/comments", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("expected GET-only comments route, got status=%d allow=%q body=%s", rec.Code, rec.Header().Get("Allow"), rec.Body.String())
	}
}

func TestHandleVideoCommentsRequiresIngestService(t *testing.T) {
	srv := newServer(nil, nil, nil, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/object-1/comments", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable comments service, got %d body=%s", rec.Code, rec.Body.String())
	}
}
