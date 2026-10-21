package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

type apiDataCenterProvider struct {
	events       []ingest.Event
	observations []ingest.VideoMetricObservation
}

func (p apiDataCenterProvider) ListEvents(_ context.Context, _ ingest.SummaryQuery) ([]ingest.Event, error) {
	return append([]ingest.Event(nil), p.events...), nil
}

func (p apiDataCenterProvider) ListVideoMetricObservations(_ context.Context, _ ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	return append([]ingest.VideoMetricObservation(nil), p.observations...), nil
}

func TestDataCenterVideoAPIUsesExactMonitorKeywordAndQualifiedGrowth(t *testing.T) {
	base := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	srv := newDataCenterTestServer()
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{DataProvider: apiDataCenterProvider{observations: []ingest.VideoMetricObservation{
		{ID: "a", ObjectID: "shared", Source: ingest.RunSourceWXVideoSearch, Keyword: "AI", ObservedAt: base, LikeCount: 10},
		{ID: "b", ObjectID: "shared", Source: ingest.RunSourceWXFeedList, Keyword: "机器人", ObservedAt: base.Add(time.Hour), LikeCount: 15},
		{ID: "c", ObjectID: "fuzzy", Title: "AI", Keyword: "AI 应用", ObservedAt: base},
	}}})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, dataCenterRoutePrefix+"videos?monitorKeyword=AI", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Total   int `json:"total"`
		Summary struct {
			Videos int `json:"videos"`
		} `json:"summary"`
		Items []struct {
			ID             string   `json:"id"`
			SourceKeywords []string `json:"sourceKeywords"`
			Sources        []string `json:"sources"`
			GrowthRate     *float64 `json:"growthRate"`
			GrowthStatus   string   `json:"growthStatus"`
			GrowthBasis    string   `json:"growthBasis"`
			Views          *int     `json:"views"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 1 || response.Summary.Videos != 1 || len(response.Items) != 1 {
		t.Fatalf("exact keyword filter and summary: %s", rec.Body.String())
	}
	item := response.Items[0]
	if item.ID != "video:shared" || len(item.SourceKeywords) != 2 || len(item.Sources) != 2 || item.GrowthStatus != "available" || item.GrowthRate == nil || *item.GrowthRate != 50 || item.GrowthBasis == "" || item.Views != nil {
		t.Fatalf("video API factual contract: %s", rec.Body.String())
	}
}

func newDataCenterTestServer() *server {
	provider := apiDataCenterProvider{events: []ingest.Event{
		{ID: "video-1", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC), ObjectID: "v1", Title: "测试视频", ChannelName: "测试账号", ChannelUsername: "@test", LikeCount: 8, CommentCount: 2, FavCount: 1, ForwardCount: 1, VideoCount: 1},
		{ID: "topic-1", EventType: "topic_cluster", Source: ingest.RunSourceWXVideoSearch, CollectedAt: time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC), Keyword: "测试话题", VideoCount: 2, TopicCount: 4, CommentCount: 3, ChannelName: "测试账号"},
		{ID: "comment-1", EventType: "comment_snapshot", Source: ingest.RunSourceWXCommentExport, CollectedAt: time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC), Keyword: "评论主题", RepresentativeComment: "如何使用", CommentCount: 9, ChannelName: "测试账号"},
	}}
	dataCenter := insight.NewDataCenterService(insight.DataCenterServiceConfig{
		DataProvider:        provider,
		ExclusionRepository: insightdemo.NewDataCenterExclusionRepository(),
	})
	srv := newServer(nil, nil, nil, nil, runtime.Status{})
	srv.dataCenterService = dataCenter
	return srv
}

func TestHandleDataCenterListsAndValidatesQuery(t *testing.T) {
	srv := newDataCenterTestServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/data-center/videos?q=%E6%B5%8B%E8%AF%95&pageSize=20", nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Entity   string                           `json:"entity"`
		Total    int                              `json:"total"`
		Items    []json.RawMessage                `json:"items"`
		Analysis *insight.VideoOperationsAnalysis `json:"analysis"`
		Summary  struct {
			Videos   int `json:"videos"`
			Topics   int `json:"topics"`
			Accounts int `json:"accounts"`
			Comments int `json:"comments"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Entity != "videos" || payload.Total != 1 || len(payload.Items) != 1 {
		t.Fatalf("unexpected data-center response: %+v body=%s", payload, rec.Body.String())
	}
	if payload.Analysis == nil || payload.Analysis.ObservedVideos != 1 || len(payload.Analysis.Groups) != 1 {
		t.Fatalf("video analysis missing from API response: %+v", payload.Analysis)
	}
	if payload.Summary.Videos != 1 || payload.Summary.Topics != 0 || payload.Summary.Accounts != 0 || payload.Summary.Comments != 0 {
		t.Fatalf("unexpected data-center summary: %+v body=%s", payload.Summary, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/read-models/data-center/comments?sort=likes", nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid sort 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleDataCenterHistoryDeleteRestore(t *testing.T) {
	srv := newDataCenterTestServer()
	entityID := "video:v1"
	encodedID := url.PathEscape(entityID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/data-center/videos/"+encodedID+"/history", nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected history 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/read-models/data-center/videos/"+encodedID, nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deletionMode":"soft"`) {
		t.Fatalf("expected soft delete response, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/read-models/data-center/videos?includeArchived=true", nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"archived"`) {
		t.Fatalf("expected archived row, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/read-models/data-center/videos/"+encodedID+"/restore", nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"restored":true`) {
		t.Fatalf("expected restore response, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleDataCenterRejectsUnsupportedPost(t *testing.T) {
	srv := newDataCenterTestServer()
	srv.ingestService = stubIngestService{summary: ingest.Summary{TotalEvents: 1}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/read-models/data-center/topics?hub_user_id=hub:user:alice&device_count=2",
		bytes.NewBufferString(`{"keyword":" AI 应用 "}`),
	)
	req.Header.Set("Content-Type", "application/json")
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected unsupported method, got %d body=%s", rec.Code, rec.Body.String())
	}
}
