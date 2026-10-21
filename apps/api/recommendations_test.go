package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type apiRecommendationProviderStub struct {
	events       []ingest.Event
	observations []ingest.VideoMetricObservation
	bindings     []ingest.SourceBinding
}

func (s apiRecommendationProviderStub) ListEvents(context.Context, ingest.SummaryQuery) ([]ingest.Event, error) {
	return append([]ingest.Event(nil), s.events...), nil
}

func (s apiRecommendationProviderStub) ListVideoMetricObservations(context.Context, ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	return append([]ingest.VideoMetricObservation(nil), s.observations...), nil
}

func (s apiRecommendationProviderStub) ListBindings(context.Context, ingest.SummaryQuery) ([]ingest.SourceBinding, error) {
	return append([]ingest.SourceBinding(nil), s.bindings...), nil
}

type apiRecommendationResolverStub struct {
	resolved workspace.ResolvedContext
}

func (s apiRecommendationResolverStub) Resolve(context.Context, workspace.ContextQuery) (workspace.ResolvedContext, error) {
	return s.resolved, nil
}

func newRecommendationAPIServer(t *testing.T) *server {
	t.Helper()
	firstAt := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Hour)
	source := ingest.RunSourceWXFeedList
	provider := apiRecommendationProviderStub{
		events: []ingest.Event{{
			ID:          "event-candidate-2",
			BatchID:     "batch-candidate-2",
			EventType:   "video_snapshot",
			Source:      source,
			CollectedAt: secondAt,
		}},
		observations: []ingest.VideoMetricObservation{
			{
				ID: "observation-candidate-1", EventID: "event-candidate-1", Source: source, ObservedAt: firstAt,
				ObjectID: "video-candidate", ChannelUsername: "candidate-real", ChannelName: "真实账号",
				Title: "真实候选视频", Tags: []string{"历史话题", "旧话题"}, LikeCount: 10, CommentCount: 2, FavCount: 1, ForwardCount: 1,
			},
			{
				ID: "observation-candidate-2", EventID: "event-candidate-2", Source: source, ObservedAt: secondAt,
				ObjectID: "video-candidate", ChannelUsername: "candidate-real", ChannelName: "真实账号",
				Title: "真实候选视频", Tags: []string{"历史话题", "新话题"}, LikeCount: 20, CommentCount: 5, FavCount: 4, ForwardCount: 2,
			},
		},
		bindings: []ingest.SourceBinding{{Source: source, Username: "already-monitored"}},
	}
	resolver := apiRecommendationResolverStub{resolved: workspace.ResolvedContext{
		Workspace: workspace.DefaultWorkspace{ID: "ws-api-recommendations", OwnerUserID: "hub:user:api", DeviceCount: 2},
	}}
	service := insight.NewRecommendationService(insight.RecommendationServiceConfig{
		DataProvider:             provider,
		WorkspaceContextResolver: resolver,
		Repository:               nil,
		Now:                      func() time.Time { return secondAt.Add(time.Hour) },
	})
	readModel := insight.NewReadModelService(nil,
		insight.WithRecommendationService(service),
		insight.WithWorkspaceContextResolver(resolver),
	)
	return newServer(nil, readModel, nil, nil, runtime.Status{})
}

func TestRecommendationEndpointsReturnRealMetricsAndDeltas(t *testing.T) {
	srv := newRecommendationAPIServer(t)

	accountRec := httptest.NewRecorder()
	accountReq := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/account-recommendations?limit=1", nil)
	srv.handleAccountRecommendations(accountRec, accountReq)
	if accountRec.Code != http.StatusOK {
		t.Fatalf("expected account recommendations 200, got %d: %s", accountRec.Code, accountRec.Body.String())
	}
	var accountPayload insightapi.AccountRecommendationsResponse
	if err := json.Unmarshal(accountRec.Body.Bytes(), &accountPayload); err != nil {
		t.Fatalf("decode account recommendations failed: %v", err)
	}
	if len(accountPayload.Items) != 1 || accountPayload.Items[0].Username != "candidate-real" {
		t.Fatalf("unexpected account recommendations: %+v", accountPayload)
	}
	if accountPayload.Items[0].Metrics.Likes != 20 || accountPayload.Items[0].PositiveDeltas.Total != 17 {
		t.Fatalf("expected latest metrics and positive deltas, got %+v", accountPayload.Items[0])
	}

	videoRec := httptest.NewRecorder()
	videoReq := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/video-recommendations?limit=1", nil)
	srv.handleVideoRecommendations(videoRec, videoReq)
	if videoRec.Code != http.StatusOK {
		t.Fatalf("expected video recommendations 200, got %d: %s", videoRec.Code, videoRec.Body.String())
	}
	var videoPayload insightapi.VideoRecommendationsResponse
	if err := json.Unmarshal(videoRec.Body.Bytes(), &videoPayload); err != nil {
		t.Fatalf("decode video recommendations failed: %v", err)
	}
	if len(videoPayload.Items) != 1 || videoPayload.Items[0].Metrics.Favorites != 4 || videoPayload.Items[0].Metrics.Forwards != 2 {
		t.Fatalf("unexpected video recommendations: %+v", videoPayload)
	}
	if !strings.Contains(videoPayload.Items[0].DataBasis, "播放/阅读数未参与评分") {
		t.Fatalf("expected explicit score basis, got %q", videoPayload.Items[0].DataBasis)
	}

}

func TestRecommendationEndpointRejectsInvalidLimit(t *testing.T) {
	srv := newRecommendationAPIServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/video-recommendations?limit=0", nil)
	srv.handleVideoRecommendations(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid limit 400, got %d", rec.Code)
	}
}
