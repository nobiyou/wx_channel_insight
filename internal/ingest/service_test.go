package ingest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
	"github.com/nobiyou/wx_channel_insight/internal/ingestfile"
)

type rewriteHostTransport struct {
	base      http.RoundTripper
	rewrites  map[string]string
	failHosts map[string]error
}

func (t rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err, ok := t.failHosts[req.URL.Host]; ok {
		return nil, err
	}

	targetBaseURL, ok := t.rewrites[req.URL.Host]
	if !ok {
		return t.base.RoundTrip(req)
	}

	targetURL, err := url.Parse(targetBaseURL)
	if err != nil {
		return nil, err
	}

	clone := req.Clone(req.Context())
	nextURL := *clone.URL
	nextURL.Scheme = targetURL.Scheme
	nextURL.Host = targetURL.Host
	clone.URL = &nextURL
	clone.Host = targetURL.Host
	return t.base.RoundTrip(clone)
}

func TestServiceAppendMockBatchBuildsSummary(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	summary, err := service.AppendMockBatch(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 4,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if summary.HubUserID != "hub:user:alice" {
		t.Fatalf("expected hub user id alice, got %q", summary.HubUserID)
	}
	if summary.State != "ready" {
		t.Fatalf("expected state ready, got %q", summary.State)
	}
	if summary.TotalEvents != 3 {
		t.Fatalf("expected 3 events, got %d", summary.TotalEvents)
	}
	if summary.SourceCount != 2 {
		t.Fatalf("expected 2 sources, got %d", summary.SourceCount)
	}
	if summary.LatestBatchID != "mock-batch-001" {
		t.Fatalf("expected batch id mock-batch-001, got %q", summary.LatestBatchID)
	}
	if len(summary.RecentEvents) != 3 {
		t.Fatalf("expected 3 recent events, got %d", len(summary.RecentEvents))
	}
	videoEvent := summary.RecentEvents[2]
	if videoEvent.Title == "" || videoEvent.ChannelName == "" {
		t.Fatalf("expected mock batch video metadata, got %+v", videoEvent)
	}
	if len(videoEvent.Tags) == 0 {
		t.Fatalf("expected mock batch tags, got %+v", videoEvent)
	}
	commentEvent := summary.RecentEvents[1]
	if commentEvent.RepresentativeComment == "" || commentEvent.TargetAudience == "" {
		t.Fatalf("expected comment metadata, got %+v", commentEvent)
	}
	topicEvent := summary.RecentEvents[0]
	if topicEvent.Keyword != "世界模型" || topicEvent.Cluster == "" || topicEvent.PromptAngle == "" {
		t.Fatalf("expected topic metadata, got %+v", topicEvent)
	}
}

func TestServiceListBindingStatesCachesLiveHealthBySourceAndEndpoint(t *testing.T) {
	repo := ingestdemo.NewRepository()
	statusCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/status" {
			http.NotFound(w, r)
			return
		}
		statusCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"clients": 1,
				"ready_clients": 1,
				"feed_ready_clients": 1,
				"client_list": [{"api_ready": true, "supports_feed": true}]
			}
		}`))
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXFeedListClient: ingest.WXFeedListClient{Client: server.Client()},
	})
	for _, username := range []string{"account-a", "account-b"} {
		if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
			Source:   ingest.RunSourceWXFeedList,
			BaseURL:  server.URL,
			Username: username,
		}); err != nil {
			t.Fatalf("upsert %s binding failed: %v", username, err)
		}
	}

	states, err := service.ListBindingStates(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:alice"})
	if err != nil {
		t.Fatalf("list binding states failed: %v", err)
	}
	if len(states) != 2 || !states[0].Runnable || !states[1].Runnable {
		t.Fatalf("expected both bindings runnable, got %+v", states)
	}
	if statusCalls != 1 {
		t.Fatalf("expected one shared health probe, got %d", statusCalls)
	}
}

func TestServiceGetSummaryReadOnlySkipsLiveHealthProbe(t *testing.T) {
	repo := ingestdemo.NewRepository()
	statusCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/status" {
			http.NotFound(w, r)
			return
		}
		statusCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"clients": 1,
				"ready_clients": 1,
				"feed_ready_clients": 1,
				"client_list": [{"api_ready": true, "supports_feed": true}]
			}
		}`))
	}))
	defer server.Close()

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:   ingest.RunSourceWXFeedList,
		BaseURL:  server.URL,
		Username: "account-a",
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXFeedListClient: ingest.WXFeedListClient{BaseURL: server.URL, Client: server.Client()},
	})
	readOnly, err := service.GetSummaryReadOnly(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:alice"})
	if err != nil {
		t.Fatalf("get read-only summary failed: %v", err)
	}
	if len(readOnly.BindingStates) != 1 || !readOnly.BindingStates[0].Runnable {
		t.Fatalf("expected locally valid binding state, got %+v", readOnly.BindingStates)
	}
	if statusCalls != 0 {
		t.Fatalf("read-only summary must skip live health probes, got %d calls", statusCalls)
	}

	if _, err := service.GetSummary(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:alice"}); err != nil {
		t.Fatalf("get regular summary failed: %v", err)
	}
	if statusCalls != 1 {
		t.Fatalf("regular summary should retain live health probe, got %d calls", statusCalls)
	}
}

func TestServiceGetSummaryReadOnlyDoesNotParseCommentExports(t *testing.T) {
	repo := ingestdemo.NewRepository()
	path := filepath.Join(t.TempDir(), "comment-export.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o644); err != nil {
		t.Fatalf("write comment export failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source: ingest.RunSourceWXCommentExport,
		Path:   path,
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
	})
	readOnly, err := service.GetSummaryReadOnly(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:alice"})
	if err != nil {
		t.Fatalf("get read-only summary failed: %v", err)
	}
	if len(readOnly.BindingStates) != 1 || !readOnly.BindingStates[0].Runnable {
		t.Fatalf("read-only summary should only require the export path to exist, got %+v", readOnly.BindingStates)
	}
}

func TestServiceListBindingStatesInvalidatesCommentAssessmentWhenFileChanges(t *testing.T) {
	repo := ingestdemo.NewRepository()
	path := filepath.Join(t.TempDir(), "comment-export.json")
	valid := []byte(`{
  "objectId": "oid-cache-001",
  "objectNonceId": "nid-cache-001",
  "title": "缓存校验",
  "author": "Insight Test",
  "commentInfo": [],
  "countInfo": {"commentCount": 0},
  "originalCommentCount": 0,
  "source": "finderGetCommentList"
}`)
	if err := os.WriteFile(path, valid, 0o644); err != nil {
		t.Fatalf("write valid comment export failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source: ingest.RunSourceWXCommentExport,
		Path:   path,
	}); err != nil {
		t.Fatalf("upsert comment binding failed: %v", err)
	}

	service := ingest.NewService(repo, ingest.ServiceConfig{DefaultHubUserID: "hub:user:demo"})
	states, err := service.ListBindingStates(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:alice"})
	if err != nil {
		t.Fatalf("list initial binding states failed: %v", err)
	}
	if len(states) != 1 || !states[0].Runnable {
		t.Fatalf("expected valid comment binding to be runnable, got %+v", states)
	}

	if err := os.WriteFile(path, []byte("not-json"), 0o644); err != nil {
		t.Fatalf("replace comment export failed: %v", err)
	}
	if err := os.Chtimes(path, time.Now().Add(2*time.Second), time.Now().Add(2*time.Second)); err != nil {
		t.Fatalf("touch replaced comment export failed: %v", err)
	}
	states, err = service.ListBindingStates(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:alice"})
	if err != nil {
		t.Fatalf("list changed binding states failed: %v", err)
	}
	if len(states) != 1 || states[0].Runnable {
		t.Fatalf("expected changed malformed comment binding to be blocked, got %+v", states)
	}
}

func TestServiceImportEventsBuildsSummaryFromRealLikeEvents(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 1, 1, 30, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	summary, err := service.ImportEvents(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, []ingest.Event{
		{
			ID:                    "evt-real-1",
			HubUserID:             "hub:user:alice",
			MachineID:             "machine-01",
			EventType:             "comment_snapshot",
			Source:                "feed_comment_list",
			CollectedAt:           time.Date(2026, 7, 1, 1, 20, 0, 0, time.FixedZone("CST", 8*3600)),
			CommentCount:          88,
			TopicCount:            4,
			Keyword:               "世界模型怎么真正落地",
			RepresentativeComment: "别只讲路线，先说落地场景。",
			TargetAudience:        "泛科技内容受众",
			Keywords:              []string{"世界模型", "落地"},
		},
	})
	if err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	if summary.HubUserID != "hub:user:alice" {
		t.Fatalf("expected hub user id alice, got %q", summary.HubUserID)
	}
	if summary.TotalEvents != 1 {
		t.Fatalf("expected 1 event, got %d", summary.TotalEvents)
	}
	if summary.LastEventType != "comment_snapshot" {
		t.Fatalf("expected last event type comment_snapshot, got %q", summary.LastEventType)
	}
	if summary.AggregatedComments != 88 {
		t.Fatalf("expected 88 aggregated comments, got %d", summary.AggregatedComments)
	}
	if summary.LatestBatchID != "import-20260630T173000.000000000Z" {
		t.Fatalf("expected synthetic import batch id, got %q", summary.LatestBatchID)
	}
	if len(summary.RecentEvents) != 1 {
		t.Fatalf("expected 1 recent event, got %d", len(summary.RecentEvents))
	}
	if summary.RecentEvents[0].BatchID != summary.LatestBatchID {
		t.Fatalf("expected recent event batch to match summary latest batch, got %+v", summary.RecentEvents[0])
	}
}

func TestServiceImportWXCommentExportBuildsUsableSummary(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 1, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "comment-export.json")
	payload := `{
  "objectId": "oid-100",
  "objectNonceId": "nid-100",
  "title": "世界模型路线争议为什么还在升温",
  "author": "Insight Real Import",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "别只讲路线，先说落地场景。",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 1,
      "levelTwoComment": [
        {
          "commentId": "r1",
          "replyCommentId": "c1",
          "content": "更想知道世界模型到底怎么落地。",
          "nickname": "用户B",
          "username": "user_b",
          "createtime": "1719828060",
          "likeCount": 5
        }
      ]
    },
    {
      "commentId": "c2",
      "content": "世界模型是不是比大模型更适合真实工作流？",
      "nickname": "用户C",
      "username": "user_c",
      "createtime": "1719828120",
      "likeCount": 8,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": {
    "commentCount": 2
  },
  "originalCommentCount": 3,
  "savedAt": "2026-07-01T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write test export file failed: %v", err)
	}

	summary, err := service.ImportWXCommentExport(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, path)
	if err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	if summary.State != "ready" {
		t.Fatalf("expected ready state, got %q", summary.State)
	}
	if summary.TotalEvents != 3 {
		t.Fatalf("expected 3 events, got %d", summary.TotalEvents)
	}
	if summary.AggregatedComments != 6 {
		t.Fatalf("expected aggregated comments 6, got %d", summary.AggregatedComments)
	}
	if summary.LatestBatchID == "" {
		t.Fatalf("expected synthetic batch id, got empty")
	}
	if summary.RecentEvents[0].BatchID != summary.LatestBatchID {
		t.Fatalf("expected newest event batch id to match latest batch, got %+v", summary.RecentEvents[0])
	}
	if summary.RecentEvents[0].Title == "" && summary.RecentEvents[0].Keyword == "" {
		t.Fatalf("expected usable recent event content, got %+v", summary.RecentEvents[0])
	}
}

func TestServiceImportEventsReplacesExistingDuplicateEventIDs(t *testing.T) {
	repo := ingestdemo.NewRepository()
	ctx := context.Background()
	hubUserID := "hub:user:alice"
	baseTime := time.Date(2026, 7, 2, 18, 0, 0, 0, time.FixedZone("CST", 8*3600))

	if err := repo.AppendEvents(ctx, hubUserID, []ingest.Event{
		{
			ID:           "wx-feed-video-dup-1",
			BatchID:      "import-old-001",
			HubUserID:    hubUserID,
			MachineID:    "wx-channel-feed-list-02",
			EventType:    "video_snapshot",
			Source:       "wx_channel_feed_list",
			CollectedAt:  baseTime.Add(-10 * time.Minute),
			VideoCount:   1,
			CommentCount: 12,
			TopicCount:   1,
			Title:        "旧视频标题",
			ChannelName:  "旧作者",
		},
		{
			ID:           "wx-feed-video-dup-1",
			BatchID:      "import-old-002",
			HubUserID:    hubUserID,
			MachineID:    "wx-channel-feed-list-02",
			EventType:    "video_snapshot",
			Source:       "wx_channel_feed_list",
			CollectedAt:  baseTime.Add(-5 * time.Minute),
			VideoCount:   1,
			CommentCount: 18,
			TopicCount:   1,
			Title:        "重复旧视频标题",
			ChannelName:  "旧作者",
		},
	}); err != nil {
		t.Fatalf("seed duplicate events failed: %v", err)
	}

	currentNow := baseTime
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return currentNow
		},
	})

	currentNow = baseTime.Add(20 * time.Minute)
	summary, err := service.ImportEvents(ctx, ingest.SummaryQuery{
		HubUserID: hubUserID,
	}, []ingest.Event{
		{
			ID:           "wx-feed-video-dup-1",
			HubUserID:    hubUserID,
			MachineID:    "wx-channel-feed-list-02",
			EventType:    "video_snapshot",
			Source:       "wx_channel_feed_list",
			CollectedAt:  currentNow,
			VideoCount:   1,
			CommentCount: 77,
			TopicCount:   3,
			Title:        "刷新后视频标题",
			ChannelName:  "新作者",
		},
	})
	if err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	if summary.TotalEvents != 1 {
		t.Fatalf("expected deduped total events 1, got %d", summary.TotalEvents)
	}
	if summary.AggregatedComments != 77 {
		t.Fatalf("expected latest comment count 77, got %d", summary.AggregatedComments)
	}

	events, err := repo.ListEvents(ctx, hubUserID)
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected repository to self-heal to 1 event, got %d", len(events))
	}
	if events[0].Title != "刷新后视频标题" || events[0].ChannelName != "新作者" {
		t.Fatalf("expected latest event payload to replace duplicates, got %+v", events[0])
	}
}

func TestServiceImportWXFeedListBuildsVideoSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/contact/feed/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("username") != "gh_test_account" {
			t.Fatalf("unexpected username: %s", r.URL.Query().Get("username"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"object": [{
					"id": "feed-1",
					"objectNonceId": "nonce-1",
					"contact": { "nickname": "视频号作者" },
					"objectDesc": {
						"description": "世界模型路线争议为什么还在升温",
						"media": [{
							"commentCount": 56
						}]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 1, 15, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: server.URL,
		},
	})

	summary, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, "", "gh_test_account", "")
	if err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	if summary.TotalEvents != 1 {
		t.Fatalf("expected 1 event, got %d", summary.TotalEvents)
	}
	if summary.AggregatedVideos != 1 {
		t.Fatalf("expected aggregated videos 1, got %d", summary.AggregatedVideos)
	}
	if summary.RecentEvents[0].Title != "世界模型路线争议为什么还在升温" {
		t.Fatalf("expected title from feed list, got %+v", summary.RecentEvents[0])
	}
	if summary.RecentEvents[0].ChannelName != "视频号作者" {
		t.Fatalf("expected channel nickname, got %+v", summary.RecentEvents[0])
	}
}

func TestServiceImportWXVideoSharePersistsBindingAndRefreshesObservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("url") != "https://weixin.qq.com/sph/share-video-service" {
			t.Fatalf("unexpected share URL: %s", r.URL.Query().Get("url"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "code": 0,
  "data": {"object": {
    "id": "shared-service-video",
    "objectNonceId": "shared-service-nonce",
    "createtime": 1785901095,
    "likeCount": 101,
    "commentCount": 22,
    "favCount": 17,
    "forwardCount": 8,
    "contact": {"username": "service-share@finder", "nickname": "分享服务账号"},
    "objectDesc": {"description": "手动分享视频", "media": [{"thumbUrl": "https://cdn.example.test/service-cover.jpg"}]}
  }}
}`))
	}))
	defer server.Close()

	now := time.Date(2026, 8, 11, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
	currentTime := now
	service := ingest.NewService(ingestdemo.NewRepository(), ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now:              func() time.Time { return currentTime },
		WXFeedListClient: ingest.WXFeedListClient{BaseURL: server.URL},
	})

	item, summary, err := service.ImportWXVideoShare(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:share",
		DeviceCount: 1,
	}, "", "https://weixin.qq.com/sph/share-video-service", "")
	if err != nil {
		t.Fatalf("import shared video: %v", err)
	}
	if item.ID != "shared-service-video" || summary.AggregatedVideos != 1 {
		t.Fatalf("expected imported video summary, item=%+v summary=%+v", item, summary)
	}
	if item.PublishedAt.IsZero() {
		t.Fatalf("expected imported item publication time, got %+v", item)
	}

	observations, err := service.ListVideoMetricObservations(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:share"})
	if err != nil {
		t.Fatalf("list observations: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected one curve observation, got %d", len(observations))
	}
	observation := observations[0]
	if observation.Source != ingest.RunSourceWXManualShare || observation.ObjectID != item.ID || observation.PublishedAt.IsZero() {
		t.Fatalf("expected persisted manual-share observation, got %+v", observation)
	}
	if observation.LikeCount != 101 || observation.CommentCount != 22 || observation.FavCount != 17 || observation.ForwardCount != 8 {
		t.Fatalf("expected four persisted metrics, got %+v", observation)
	}

	bindings, err := service.ListBindings(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:share"})
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected one manual-share monitor binding, got %+v", bindings)
	}
	if bindings[0].Source != ingest.RunSourceWXManualShare || bindings[0].ShareURL != "https://weixin.qq.com/sph/share-video-service" || bindings[0].ObjectID != item.ID || bindings[0].ObjectNonceID != item.ObjectNonceID || bindings[0].TargetKey != item.ID {
		t.Fatalf("expected saved manual-share binding identity, got %+v", bindings[0])
	}

	currentTime = now.Add(5 * time.Minute)
	if _, err := service.RefreshWXVideoShareBinding(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:share",
		DeviceCount: 1,
	}, bindings[0]); err != nil {
		t.Fatalf("refresh manual-share binding: %v", err)
	}
	observations, err = service.ListVideoMetricObservations(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:share"})
	if err != nil {
		t.Fatalf("list refreshed observations: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("expected a second curve observation after refresh, got %d", len(observations))
	}
}

func TestServiceSearchWXFeedAccountsBuildsCandidatesFromNestedWXChannelPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/contact/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("keyword") != "teamlight" {
			t.Fatalf("unexpected keyword: %s", r.URL.Query().Get("keyword"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"errCode": 0,
				"errMsg": "ok",
				"data": {
					"BaseResponse": {
						"ErrMsg": { "String": "" },
						"Ret": 0
					},
					"infoList": [{
						"contact": {
							"nickname": "TEAMLIGHT-",
							"signature": "",
							"username": "v2_060000231003b20faec8c5e28b10c6d6ce01ea36b077f6a0e58226dc63fe48550812f2bdbca9@finder",
							"headUrl": "https://example.com/avatar.png"
						}
					}]
				}
			}
		}`))
	}))
	defer server.Close()

	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: server.URL,
		},
	})

	items, err := service.SearchWXFeedAccounts(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, "", "teamlight", "")
	if err != nil {
		t.Fatalf("expected search success, got %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one candidate, got %+v", items)
	}
	if items[0].Nickname != "TEAMLIGHT-" || items[0].Username == "" {
		t.Fatalf("expected parsed finder candidate, got %+v", items[0])
	}
}

func TestServiceImportWXFeedListTwiceKeepsSingleVideoSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"object": [{
					"id": "feed-repeat-1",
					"objectNonceId": "nonce-repeat-1",
					"contact": { "nickname": "重复导入作者" },
					"objectDesc": {
						"description": "当摄影学会了广西运镜#广西运镜",
						"media": [{
							"commentCount": 92
						}]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	repo := ingestdemo.NewRepository()
	currentNow := time.Date(2026, 7, 2, 18, 0, 0, 0, time.FixedZone("CST", 8*3600))
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return currentNow
		},
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: server.URL,
		},
	})

	first, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, "", "gh_repeat_account", "")
	if err != nil {
		t.Fatalf("expected first import success, got %v", err)
	}

	currentNow = currentNow.Add(10 * time.Minute)
	second, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, "", "gh_repeat_account", "")
	if err != nil {
		t.Fatalf("expected second import success, got %v", err)
	}

	if first.TotalEvents != 1 {
		t.Fatalf("expected first import total 1, got %d", first.TotalEvents)
	}
	if second.TotalEvents != 1 {
		t.Fatalf("expected second import total to stay 1, got %d", second.TotalEvents)
	}
	if second.LatestBatchID == first.LatestBatchID {
		t.Fatalf("expected refreshed import batch id to change, got first=%q second=%q", first.LatestBatchID, second.LatestBatchID)
	}

	events, err := repo.ListEvents(context.Background(), "hub:user:alice")
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected repository to keep 1 event after repeated import, got %d", len(events))
	}
	if events[0].CommentCount != 92 || events[0].Title != "当摄影学会了广西运镜#广西运镜" {
		t.Fatalf("expected repeated import to keep refreshed event payload, got %+v", events[0])
	}
}

func TestServiceImportWXFeedListBuildsVideoSummaryFromNestedWXChannelPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/contact/feed/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"errCode": 0,
				"errMsg": "ok",
				"data": {
					"object": [{
						"id": "feed-nested-1",
						"objectNonceId": "nonce-nested-1",
						"commentCount": 115,
						"contact": { "nickname": "嵌套视频号作者" },
						"objectDesc": {
							"description": "想好叫我什么了吗？#白毛 #a2#cos"
						}
					}],
					"lastBuffer": "next-buffer"
				}
			}
		}`))
	}))
	defer server.Close()

	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 2, 19, 10, 0, 0, time.FixedZone("CST", 8*3600))
		},
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: server.URL,
		},
	})

	summary, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, "", "gh_nested_account", "")
	if err != nil {
		t.Fatalf("expected nested import success, got %v", err)
	}

	if summary.TotalEvents != 1 {
		t.Fatalf("expected 1 event, got %d", summary.TotalEvents)
	}
	if summary.AggregatedVideos != 1 {
		t.Fatalf("expected aggregated videos 1, got %d", summary.AggregatedVideos)
	}
	if summary.RecentEvents[0].Title != "想好叫我什么了吗？#白毛 #a2#cos" {
		t.Fatalf("expected nested title, got %+v", summary.RecentEvents[0])
	}
	if summary.RecentEvents[0].ChannelName != "嵌套视频号作者" {
		t.Fatalf("expected nested channel nickname, got %+v", summary.RecentEvents[0])
	}
	if summary.RecentEvents[0].CommentCount != 115 {
		t.Fatalf("expected nested comment count 115, got %+v", summary.RecentEvents[0])
	}
}

func TestServiceImportWXFeedListReturnsFinderGuidanceForKeywordUsername(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/contact/feed/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"errCode": 0,
				"errMsg": "ok",
				"data": {
					"BaseResponse": {
						"ErrMsg": { "String": "" },
						"Ret": -2
					},
					"object": []
				}
			}
		}`))
	}))
	defer server.Close()

	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: server.URL,
		},
	})

	_, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, "", "gh_team_runtime", "")
	if err == nil || !strings.Contains(err.Error(), "真实的 @finder username") {
		t.Fatalf("expected finder guidance error, got %v", err)
	}
}

func TestServiceImportWXFeedListReturnsUpstreamMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":503,"message":"No ready WeChat page is available for feed list."}`))
	}))
	defer server.Close()

	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: server.URL,
		},
	})

	_, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, "", "gh_test_account", "")
	if err == nil {
		t.Fatal("expected import error, got nil")
	}
	if err.Error() != "wx_channel 已启动，但当前没有就绪的视频号客户端。请先确认视频号独立客户端正在运行，并且请求已被 wx_channel 代理拦截后，再导入视频列表" {
		t.Fatalf("expected upstream message to bubble up, got %q", err.Error())
	}

	bindings, listErr := service.ListBindings(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if listErr != nil {
		t.Fatalf("list bindings failed: %v", listErr)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected saved feed binding after upstream failure, got %+v", bindings)
	}
	if bindings[0].Source != ingest.RunSourceWXFeedList || bindings[0].Username != "gh_test_account" {
		t.Fatalf("expected saved feed binding with username, got %+v", bindings[0])
	}
}

func TestServiceImportWXFeedListReturnsFriendlyConnectionError(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: "http://127.0.0.1:1",
			Client: &http.Client{
				Timeout: 300 * time.Millisecond,
			},
		},
	})

	_, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, "", "gh_test_account", "")
	if err == nil {
		t.Fatal("expected import error, got nil")
	}
	if want := "无法连接到 wx_channel API"; err != nil && !strings.Contains(err.Error(), want) {
		t.Fatalf("expected friendly connection error containing %q, got %q", want, err.Error())
	}

	bindings, listErr := service.ListBindings(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if listErr != nil {
		t.Fatalf("list bindings failed: %v", listErr)
	}
	if len(bindings) != 1 || bindings[0].Username != "gh_test_account" {
		t.Fatalf("expected saved feed binding after connection failure, got %+v", bindings)
	}
}

func TestServiceSaveWXFeedListBindingPersistsRunnableBinding(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: "http://127.0.0.1:3030",
		},
	})

	summary, err := service.SaveWXFeedListBinding(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 4,
	}, "", "gh_saved_account")
	if err != nil {
		t.Fatalf("expected save binding success, got %v", err)
	}
	if summary.TotalEvents != 0 || len(summary.RecentRuns) != 0 {
		t.Fatalf("expected save-only binding to avoid ingest mutations, got %+v", summary)
	}
	if len(summary.BindingStates) != 1 || !summary.BindingStates[0].Runnable {
		t.Fatalf("expected runnable feed binding state in summary, got %+v", summary.BindingStates)
	}

	bindings, listErr := service.ListBindings(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if listErr != nil {
		t.Fatalf("list bindings failed: %v", listErr)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected one saved binding, got %+v", bindings)
	}
	if bindings[0].Source != ingest.RunSourceWXFeedList || bindings[0].BaseURL != "http://127.0.0.1:3030" {
		t.Fatalf("expected saved feed binding base URL, got %+v", bindings[0])
	}
	if bindings[0].Username != "gh_saved_account" || bindings[0].DeviceCount != 4 {
		t.Fatalf("expected saved feed binding username/device count, got %+v", bindings[0])
	}
}

func TestServiceSaveWXFeedListBindingRequiresUsername(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
	})

	_, err := service.SaveWXFeedListBinding(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, "http://127.0.0.1:2026", "   ")
	if err == nil || err.Error() != "username is required" {
		t.Fatalf("expected username required error, got %v", err)
	}

	bindings, listErr := service.ListBindings(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if listErr != nil {
		t.Fatalf("list bindings failed: %v", listErr)
	}
	if len(bindings) != 0 {
		t.Fatalf("expected no saved bindings after validation error, got %+v", bindings)
	}
}

func TestServiceSaveWXCommentExportBindingPersistsPathWithoutImportingEvents(t *testing.T) {
	tempDir := t.TempDir()
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	path := filepath.Join(tempDir, "comment-export.json")
	payload := `{
  "objectId": "oid-save-binding-1",
  "objectNonceId": "nid-save-binding-1",
  "title": "保存绑定测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先把来源记下来",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-04T10:24:00+08:00",
  "source": "wx_channel_comment_export"
}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}

	summary, err := service.SaveWXCommentExportBinding(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, path)
	if err != nil {
		t.Fatalf("expected save binding success, got %v", err)
	}
	if summary.TotalEvents != 0 || len(summary.RecentRuns) != 0 {
		t.Fatalf("expected save-only binding to avoid ingest mutations, got %+v", summary)
	}
	if len(summary.BindingStates) != 1 || !summary.BindingStates[0].Runnable {
		t.Fatalf("expected runnable comment binding state in summary, got %+v", summary.BindingStates)
	}

	bindings, err := service.ListBindings(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected one saved binding, got %+v", bindings)
	}
	if bindings[0].Source != ingest.RunSourceWXCommentExport || bindings[0].Path != path {
		t.Fatalf("expected saved comment-export binding path, got %+v", bindings[0])
	}
	if bindings[0].DeviceCount != 2 {
		t.Fatalf("expected saved comment-export device count 2, got %+v", bindings[0])
	}
}

func TestServiceImportWXCommentExportPersistsBinding(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "ingest-state.json")
	repo, err := ingestfile.NewRepository(repoPath)
	if err != nil {
		t.Fatalf("new file repo failed: %v", err)
	}
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 3, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	path := filepath.Join(tempDir, "comment-export.json")
	payload := `{
  "objectId": "oid-binding-1",
  "objectNonceId": "nid-binding-1",
  "title": "绑定测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先说说怎么落地",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}

	if _, err := service.ImportWXCommentExport(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, path); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	bindings, err := service.ListBindings(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %+v", bindings)
	}
	if bindings[0].Source != ingest.RunSourceWXCommentExport {
		t.Fatalf("expected comment export source, got %+v", bindings[0])
	}
	if bindings[0].Path != path {
		t.Fatalf("expected persisted path %q, got %+v", path, bindings[0])
	}
	if bindings[0].DeviceCount != 2 {
		t.Fatalf("expected persisted device count 2, got %+v", bindings[0])
	}

	raw, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatalf("read ingest state failed: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("unmarshal ingest state failed: %v", err)
	}
	if _, ok := state["bindings"]; !ok {
		t.Fatalf("expected bindings in persisted state, got %+v", state)
	}
}

func TestServiceRetryLastFailedImportReusesSavedFeedBinding(t *testing.T) {
	serverCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"object": [{
					"id": "feed-retry-1",
					"objectNonceId": "nonce-retry-1",
					"contact": { "nickname": "重试视频号作者" },
					"objectDesc": {
						"description": "重试后的视频列表标题",
						"media": [{ "commentCount": 42 }]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	repo := ingestdemo.NewRepository()
	currentNow := time.Date(2026, 7, 3, 11, 0, 0, 0, time.FixedZone("CST", 8*3600))
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return currentNow
		},
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: server.URL,
		},
	})

	if _, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 3,
	}, "", "gh_retry_account", ""); err != nil {
		t.Fatalf("expected initial import success, got %v", err)
	}

	currentNow = currentNow.Add(10 * time.Minute)
	_ = repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_feed_list-manual-failure",
		Source:     ingest.RunSourceWXFeedList,
		Status:     "failed",
		StartedAt:  currentNow,
		FinishedAt: currentNow,
		Message:    "temporary upstream failure",
	})

	currentNow = currentNow.Add(5 * time.Minute)
	summary, err := service.RetryLastFailedImport(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, ingest.RunSourceWXFeedList)
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if serverCalls < 2 {
		t.Fatalf("expected retry to call upstream again, got %d calls", serverCalls)
	}
	if summary.TotalEvents != 1 || summary.RecentEvents[0].Title != "重试后的视频列表标题" {
		t.Fatalf("expected refreshed feed summary, got %+v", summary)
	}

	bindings, err := service.ListBindings(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) == 0 || bindings[0].Username != "gh_retry_account" || bindings[0].DeviceCount != 3 {
		t.Fatalf("expected saved retry binding, got %+v", bindings)
	}
}

func TestServiceRetryLastFailedImportRefreshesCommentBindingViaWXChannelExport(t *testing.T) {
	repo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	commentDir := t.TempDir()
	stalePath := filepath.Join(commentDir, "comment-export-stale.json")
	if err := os.WriteFile(stalePath, []byte(`{
  "objectId": "oid-refresh-001",
  "objectNonceId": "nid-refresh-001",
  "title": "旧评论导出样本",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"旧评论","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T19:08:38+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write stale export failed: %v", err)
	}
	refreshedPath := filepath.Join(commentDir, "comment-export-refreshed.json")
	if err := os.WriteFile(refreshedPath, []byte(`{
  "objectId": "oid-refresh-001",
  "objectNonceId": "nid-refresh-001",
  "title": "刷新后的评论导出样本",
  "author": "Insight Test",
  "commentInfo": [
    {"commentId":"c1","content":"新评论一","nickname":"用户A","username":"user_a","createtime":"1719914400","likeCount":3,"expandCommentCount":0,"levelTwoComment":[]},
    {"commentId":"c2","content":"新评论二","nickname":"用户B","username":"user_b","createtime":"1719918000","likeCount":2,"expandCommentCount":0,"levelTwoComment":[]}
  ],
  "countInfo": { "commentCount": 2 },
  "originalCommentCount": 2,
  "savedAt": "2026-07-05T09:58:00+08:00",
  "source": "finderGetCommentList"
}`), 0o644); err != nil {
		t.Fatalf("write refreshed export failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/comment/export" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		if req["object_id"] != "oid-refresh-001" || req["nonce_id"] != "nid-refresh-001" {
			t.Fatalf("unexpected refresh request: %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"message":"success","data":{"object_id":"oid-refresh-001","saved_path":%q,"relative_path":"comment_data/2026-07-05/comment-export-refreshed.json","title":"刷新后的评论导出样本","author":"Insight Test","source":"finderGetCommentList"}}`, refreshedPath)))
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
		WXCommentClient: ingest.WXCommentExportClient{BaseURL: server.URL, Client: server.Client()},
	})

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        stalePath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 5, 9, 50, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_comment_export-failed-refresh",
		Source:     ingest.RunSourceWXCommentExport,
		Status:     "failed",
		StartedAt:  time.Date(2026, 7, 5, 9, 52, 0, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt: time.Date(2026, 7, 5, 9, 52, 10, 0, time.FixedZone("CST", 8*3600)),
		Message:    "stale comment export",
	}); err != nil {
		t.Fatalf("append failed run failed: %v", err)
	}

	summary, err := service.RetryLastFailedImport(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, ingest.RunSourceWXCommentExport)
	if err != nil {
		t.Fatalf("expected comment refresh retry success, got %v", err)
	}
	if summary.LastSuccessAt.IsZero() || !summary.LastSuccessAt.Equal(now) {
		t.Fatalf("expected retry success to use execution time, got %+v", summary)
	}
	if summary.TotalEvents < 2 {
		t.Fatalf("expected refreshed import events, got %+v", summary)
	}
	bindings, err := repo.ListBindings(context.Background(), "hub:user:alice")
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	binding, ok := findTestBindingBySource(bindings, ingest.RunSourceWXCommentExport)
	if !ok {
		t.Fatalf("expected saved comment binding, got %+v", bindings)
	}
	if filepath.Clean(binding.Path) != filepath.Clean(refreshedPath) {
		t.Fatalf("expected binding path to move to refreshed export, got %+v", binding)
	}
}

func TestServiceImportWXCommentExportByAPISavesLatestExport(t *testing.T) {
	repo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 5, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
	refreshedPath := filepath.Join(t.TempDir(), "comment-export-synced.json")
	if err := os.WriteFile(refreshedPath, []byte(`{
  "objectId": "oid-sync-001",
  "objectNonceId": "nid-sync-001",
  "title": "同步评论导出样本",
  "author": "Insight Test",
  "commentInfo": [
    {"commentId":"c1","content":"新评论一","nickname":"用户A","username":"user_a","createtime":"1719914400","likeCount":3,"expandCommentCount":0,"levelTwoComment":[]},
    {"commentId":"c2","content":"新评论二","nickname":"用户B","username":"user_b","createtime":"1719918000","likeCount":2,"expandCommentCount":0,"levelTwoComment":[]}
  ],
  "countInfo": { "commentCount": 2 },
  "originalCommentCount": 2,
  "savedAt": "2026-07-05T10:28:00+08:00",
  "source": "finderGetCommentList"
}`), 0o644); err != nil {
		t.Fatalf("write synced export failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"clients":1,"ready_clients":1,"feed_ready_clients":1,"client_list":[{"page_path":"/web/pages/video/detail","href":"https://channels.weixin.qq.com/web/pages/video/detail","api_ready":true,"supports_feed":true}]}}`))
		case "/api/channels/feed/comment/export":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request failed: %v", err)
			}
			if req["object_id"] != "oid-sync-001" || req["nonce_id"] != "nid-sync-001" {
				t.Fatalf("unexpected sync request: %+v", req)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"message":"success","data":{"object_id":"oid-sync-001","saved_path":%q,"relative_path":"comment_data/2026-07-05/comment-export-synced.json","title":"同步评论导出样本","author":"Insight Test","source":"finderGetCommentList"}}`, refreshedPath)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXCommentClient: ingest.WXCommentExportClient{
			BaseURL: server.URL,
			Client:  server.Client(),
		},
		Now: func() time.Time {
			return now
		},
	})

	summary, err := service.ImportWXCommentExportByAPI(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, server.URL, "oid-sync-001", "nid-sync-001", "同步评论导出样本", "Insight Test", "")
	if err != nil {
		t.Fatalf("expected comment sync success, got %v", err)
	}
	if summary.TotalEvents < 2 {
		t.Fatalf("expected synced import events, got %+v", summary)
	}

	bindings, err := repo.ListBindings(context.Background(), "hub:user:alice")
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	binding, ok := findTestBindingBySource(bindings, ingest.RunSourceWXCommentExport)
	if !ok {
		t.Fatalf("expected saved comment binding, got %+v", bindings)
	}
	if filepath.Clean(binding.Path) != filepath.Clean(refreshedPath) {
		t.Fatalf("expected synced binding path %s, got %+v", refreshedPath, binding)
	}
}

func findTestBindingBySource(bindings []ingest.SourceBinding, source string) (ingest.SourceBinding, bool) {
	for _, binding := range bindings {
		if strings.TrimSpace(binding.Source) == strings.TrimSpace(source) {
			return binding, true
		}
	}
	return ingest.SourceBinding{}, false
}

func TestServiceRetryLastFailedImportRequiresLatestFailure(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 3, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:    ingest.RunSourceWXCommentExport,
		Path:      "C:\\temp\\comment-export.json",
		UpdatedAt: time.Date(2026, 7, 3, 11, 0, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_comment_export-success",
		Source:     ingest.RunSourceWXCommentExport,
		Status:     "success",
		StartedAt:  time.Date(2026, 7, 3, 11, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt: time.Date(2026, 7, 3, 11, 1, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("append run failed: %v", err)
	}

	_, err := service.RetryLastFailedImport(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, ingest.RunSourceWXCommentExport)
	if err == nil || !strings.Contains(err.Error(), "is not failed") {
		t.Fatalf("expected latest-not-failed error, got %v", err)
	}
}

func TestServiceListBindingStatesMarksMissingCommentExportPathUnretryable(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 3, 12, 30, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	missingPath := filepath.Join(t.TempDir(), "missing-comment-export.json")
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        missingPath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 3, 12, 0, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_comment_export-failed",
		Source:     ingest.RunSourceWXCommentExport,
		Status:     "failed",
		StartedAt:  time.Date(2026, 7, 3, 12, 5, 0, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt: time.Date(2026, 7, 3, 12, 6, 0, 0, time.FixedZone("CST", 8*3600)),
		Message:    "open path failed",
	}); err != nil {
		t.Fatalf("append run failed: %v", err)
	}

	states, err := service.ListBindingStates(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list binding states failed: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 binding state, got %+v", states)
	}
	if states[0].CanRetry {
		t.Fatalf("expected missing path to be unretryable, got %+v", states[0])
	}
	if !strings.Contains(states[0].RetryReason, "文件不存在") {
		t.Fatalf("expected missing file retry reason, got %+v", states[0])
	}
	if !strings.Contains(states[0].LastError, "文件不存在") {
		t.Fatalf("expected normalized last error, got %+v", states[0])
	}
}

func TestServiceListBindingStatesClearsRecoveredFeedListErrorAndRetryReason(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 13, 40, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     "http://127.0.0.1:2026",
		Username:    "v2_demo_recovered@finder",
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 4, 13, 31, 24, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_feed_list-failed",
		Source:     ingest.RunSourceWXFeedList,
		Status:     "failed",
		StartedAt:  time.Date(2026, 7, 4, 12, 46, 29, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt: time.Date(2026, 7, 4, 12, 46, 29, 0, time.FixedZone("CST", 8*3600)),
		Message:    "wx_channel feed list returned no usable videos",
	}); err != nil {
		t.Fatalf("append failed run failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:            "wx_feed_list-success",
		Source:        ingest.RunSourceWXFeedList,
		Status:        "success",
		StartedAt:     time.Date(2026, 7, 4, 13, 31, 24, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt:    time.Date(2026, 7, 4, 13, 31, 25, 0, time.FixedZone("CST", 8*3600)),
		BatchID:       "import-20260704T053124.531270000Z",
		EventCount:    2,
		LastEventType: "video_snapshot",
	}); err != nil {
		t.Fatalf("append success run failed: %v", err)
	}

	states, err := service.ListBindingStates(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list binding states failed: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 binding state, got %+v", states)
	}
	if states[0].LastRunStatus != "success" || states[0].CanRetry {
		t.Fatalf("expected recovered binding to be successful and unretryable, got %+v", states[0])
	}
	if states[0].RetryReason != "" {
		t.Fatalf("expected recovered binding retry reason to clear, got %+v", states[0])
	}
	if states[0].LastError != "" {
		t.Fatalf("expected recovered binding last error to clear, got %+v", states[0])
	}
}

func TestServiceListBindingStatesMarksCommentExportContextMismatchBlocked(t *testing.T) {
	repo := ingestdemo.NewRepository()
	commentPath := filepath.Join(t.TempDir(), "comment-export-context-mismatch.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-context-mismatch-1",
  "objectNonceId": "nid-context-mismatch-1",
  "title": "context mismatch",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"页面错位时应该阻塞来源","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T19:08:38+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write comment export fixture failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"client_list":[{"page_path":"/web/pages/home","href":"https://channels.weixin.qq.com/web/pages/home","api_ready":true,"supports_feed":true}],"clients":1,"ready_clients":1,"comment_ready_clients":1,"feed_ready_clients":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXCommentClient: ingest.WXCommentExportClient{
			BaseURL: server.URL,
			Client:  server.Client(),
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 5, 11, 20, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        commentPath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 5, 11, 0, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_comment_export-context-mismatch",
		Source:     ingest.RunSourceWXCommentExport,
		Status:     "failed",
		StartedAt:  time.Date(2026, 7, 5, 11, 16, 0, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt: time.Date(2026, 7, 5, 11, 16, 36, 0, time.FixedZone("CST", 8*3600)),
		Message:    "API error (code=-70003): JSAPI_JSONPARSE_FAILED",
	}); err != nil {
		t.Fatalf("append run failed: %v", err)
	}

	states, err := service.ListBindingStates(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list binding states failed: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 binding state, got %+v", states)
	}
	if states[0].Runnable || states[0].CanRetry {
		t.Fatalf("expected context mismatch binding to be blocked and unretryable, got %+v", states[0])
	}
	if !strings.Contains(states[0].RetryReason, "当前客户端上下文与保存的评论导出视频不匹配") {
		t.Fatalf("expected context mismatch retry reason, got %+v", states[0])
	}
	if !strings.Contains(states[0].LastError, "当前客户端上下文与保存的评论导出视频不匹配") {
		t.Fatalf("expected normalized last error, got %+v", states[0])
	}
	if states[0].Diagnostic == nil || states[0].Diagnostic.Code != "wx_comment_export_context_mismatch" {
		t.Fatalf("expected context mismatch diagnostic code, got %+v", states[0])
	}
	if states[0].Diagnostic.CurrentPagePath != "/web/pages/home" {
		t.Fatalf("expected current page path in diagnostic, got %+v", states[0].Diagnostic)
	}
	if states[0].Diagnostic.StatusURL != server.URL+"/api/channels/status" {
		t.Fatalf("expected status URL in diagnostic, got %+v", states[0].Diagnostic)
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}
	if summary.OperatorReadiness.State != "blocked" || summary.OperatorReadiness.BlockedSources != 1 {
		t.Fatalf("expected blocked operator readiness, got %+v", summary.OperatorReadiness)
	}
	if len(summary.OperatorReadiness.Blocked) != 1 || summary.OperatorReadiness.Blocked[0].Diagnostic == nil {
		t.Fatalf("expected blocked operator source with diagnostic, got %+v", summary.OperatorReadiness.Blocked)
	}
	if summary.OperatorReadiness.Blocked[0].Diagnostic.Code != "wx_comment_export_context_mismatch" {
		t.Fatalf("expected context mismatch diagnostic in operator readiness, got %+v", summary.OperatorReadiness.Blocked[0])
	}
	if !strings.Contains(summary.OperatorReadiness.Blocked[0].Action, "切到这条评论对应的视频") {
		t.Fatalf("expected operator action to point at target video, got %+v", summary.OperatorReadiness.Blocked[0])
	}
	if !strings.Contains(summary.LastFailureMessage, "当前客户端上下文与保存的评论导出视频不匹配") {
		t.Fatalf("expected summary failure message normalization, got %+v", summary)
	}
	if len(summary.SourceStatuses) != 1 || !strings.Contains(summary.SourceStatuses[0].LastError, "当前客户端上下文与保存的评论导出视频不匹配") {
		t.Fatalf("expected source status error normalization, got %+v", summary.SourceStatuses)
	}
}

func TestServiceGetSummaryBuildsOperatorReadinessReady(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 3, 14, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	commentPath := filepath.Join(t.TempDir(), "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-ready-001",
  "objectNonceId": "nid-ready-001",
  "title": "ready comment export",
  "author": "Insight Test",
  "commentInfo": [],
  "countInfo": { "commentCount": 0 },
  "originalCommentCount": 0,
  "savedAt": "2026-07-03T13:29:00+08:00",
  "source": "finderGetCommentList"
}`), 0o644); err != nil {
		t.Fatalf("write comment export fixture failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        commentPath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 3, 13, 30, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert comment binding failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     "http://127.0.0.1:2026",
		Username:    "gh_demo",
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 3, 13, 31, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert feed binding failed: %v", err)
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}

	if summary.OperatorReadiness.State != "ready" {
		t.Fatalf("expected ready operator readiness, got %+v", summary.OperatorReadiness)
	}
	if summary.OperatorReadiness.RunnableSources != 2 || summary.OperatorReadiness.BlockedSources != 0 || summary.OperatorReadiness.MissingSources != 0 {
		t.Fatalf("expected all operator sources runnable, got %+v", summary.OperatorReadiness)
	}
}

func TestServiceGetSummaryCarriesLastAutoRunFactsIntoBindingStates(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 14, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	commentPath := filepath.Join(t.TempDir(), "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-ready-002",
  "objectNonceId": "nid-ready-002",
  "title": "ready comment export",
  "author": "Insight Test",
  "commentInfo": [],
  "countInfo": { "commentCount": 0 },
  "originalCommentCount": 0,
  "savedAt": "2026-07-04T13:29:00+08:00",
  "source": "finderGetCommentList"
}`), 0o644); err != nil {
		t.Fatalf("write comment export fixture failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        commentPath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 4, 13, 30, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert comment binding failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     "http://127.0.0.1:2026",
		Username:    "gh_demo",
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 4, 13, 31, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert feed binding failed: %v", err)
	}
	if err := service.AppendOperatorRun(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, ingest.OperatorRun{
		ID:               "operator-cycle-002",
		Status:           "ok",
		TriggerMode:      ingest.OperatorTriggerScheduled,
		StartedAt:        time.Date(2026, 7, 4, 13, 40, 0, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt:       time.Date(2026, 7, 4, 13, 45, 0, 0, time.FixedZone("CST", 8*3600)),
		LatestBatchID:    "import-auto-002",
		AttemptedSources: 2,
		SucceededSources: 2,
		Outcomes: []ingest.OperatorRunOutcome{
			{Source: ingest.RunSourceWXCommentExport, Status: "success", Summary: "批次 import-auto-002"},
			{Source: ingest.RunSourceWXFeedList, Status: "success", Summary: "批次 import-auto-002"},
		},
	}); err != nil {
		t.Fatalf("append operator run failed: %v", err)
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}

	if len(summary.BindingStates) != 2 {
		t.Fatalf("expected two binding states, got %+v", summary.BindingStates)
	}
	for _, item := range summary.BindingStates {
		if item.LastAutoRunStatus != "success" {
			t.Fatalf("expected last auto run status success, got %+v", item)
		}
		if item.LastAutoSuccessAt.IsZero() || item.LastAutoBatchID != "import-auto-002" {
			t.Fatalf("expected auto success evidence on binding state, got %+v", item)
		}
	}
	if len(summary.OperatorReadiness.Runnable) != 2 {
		t.Fatalf("expected runnable operator readiness items, got %+v", summary.OperatorReadiness)
	}
	for _, item := range summary.OperatorReadiness.Runnable {
		if item.LastAutoRunStatus != "success" || item.LastAutoBatchID != "import-auto-002" {
			t.Fatalf("expected operator readiness to inherit auto evidence, got %+v", item)
		}
	}
}

func TestServiceGetSummaryBuildsOperatorReadinessPartialForMissingSource(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 3, 14, 30, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     "http://127.0.0.1:2026",
		Username:    "gh_demo",
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 3, 14, 10, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert feed binding failed: %v", err)
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}

	if summary.OperatorReadiness.State != "partial" {
		t.Fatalf("expected partial operator readiness, got %+v", summary.OperatorReadiness)
	}
	if summary.OperatorReadiness.RunnableSources != 1 || summary.OperatorReadiness.MissingSources != 1 {
		t.Fatalf("expected one runnable and one missing source, got %+v", summary.OperatorReadiness)
	}
	if len(summary.OperatorReadiness.Missing) != 1 || summary.OperatorReadiness.Missing[0].Source != ingest.RunSourceWXCommentExport {
		t.Fatalf("expected missing comment export source, got %+v", summary.OperatorReadiness.Missing)
	}
}

func TestServiceGetSummaryBuildsOperatorReadinessPartialForFeedListWithoutReadyClient(t *testing.T) {
	repo := ingestdemo.NewRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"clients":0}`))
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXFeedListClient: ingest.WXFeedListClient{
			Client: server.Client(),
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 9, 45, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	commentPath := filepath.Join(t.TempDir(), "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-ready-003",
  "objectNonceId": "nid-ready-003",
  "title": "ready comment export",
  "author": "Insight Test",
  "commentInfo": [],
  "countInfo": { "commentCount": 0 },
  "originalCommentCount": 0,
  "savedAt": "2026-07-04T09:29:00+08:00",
  "source": "finderGetCommentList"
}`), 0o644); err != nil {
		t.Fatalf("write comment export fixture failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        commentPath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 4, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert comment binding failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     server.URL,
		Username:    "gh_demo",
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 4, 9, 31, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert feed binding failed: %v", err)
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}

	if summary.OperatorReadiness.State != "partial" {
		t.Fatalf("expected partial operator readiness, got %+v", summary.OperatorReadiness)
	}
	if summary.OperatorReadiness.RunnableSources != 1 || summary.OperatorReadiness.BlockedSources != 1 {
		t.Fatalf("expected one runnable and one blocked source, got %+v", summary.OperatorReadiness)
	}
	if len(summary.OperatorReadiness.Blocked) != 1 || summary.OperatorReadiness.Blocked[0].Source != ingest.RunSourceWXFeedList {
		t.Fatalf("expected blocked feed list source, got %+v", summary.OperatorReadiness.Blocked)
	}
	if !strings.Contains(summary.OperatorReadiness.Blocked[0].RetryReason, "feed list has no ready page client") {
		t.Fatalf("expected live health retry reason, got %+v", summary.OperatorReadiness.Blocked[0])
	}
	if summary.OperatorReadiness.Blocked[0].Diagnostic == nil || summary.OperatorReadiness.Blocked[0].Diagnostic.ReadyClientCount == nil || *summary.OperatorReadiness.Blocked[0].Diagnostic.ReadyClientCount != 0 {
		t.Fatalf("expected blocked feed list diagnostic with zero ready clients, got %+v", summary.OperatorReadiness.Blocked[0])
	}
}

func TestServiceGetSummaryPrefersStatusEndpointWhenFeedClientIsReady(t *testing.T) {
	repo := ingestdemo.NewRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"message": "success",
				"data": {
					"clients": 1,
					"ready_clients": 1,
					"feed_ready_clients": 1,
					"client_list": [{
						"page_path": "/web/pages/home",
						"href": "https://channels.weixin.qq.com/web/pages/home",
						"api_ready": true,
						"supports_feed": true
					}]
				}
			}`))
		case "/ws/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":0,"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXFeedListClient: ingest.WXFeedListClient{
			Client: server.Client(),
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 10, 5, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	commentPath := filepath.Join(t.TempDir(), "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-ready-004",
  "objectNonceId": "nid-ready-004",
  "title": "ready comment export",
  "author": "Insight Test",
  "commentInfo": [],
  "countInfo": { "commentCount": 0 },
  "originalCommentCount": 0,
  "savedAt": "2026-07-04T09:29:00+08:00",
  "source": "finderGetCommentList"
}`), 0o644); err != nil {
		t.Fatalf("write comment export fixture failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        commentPath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 4, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert comment binding failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     server.URL,
		Username:    "gh_demo",
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 4, 9, 31, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert feed binding failed: %v", err)
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}

	if summary.OperatorReadiness.BlockedSources != 0 {
		t.Fatalf("expected feed list to stay runnable when status endpoint reports a ready feed client, got %+v", summary.OperatorReadiness)
	}
	if summary.OperatorReadiness.RunnableSources != 2 {
		t.Fatalf("expected both saved sources runnable, got %+v", summary.OperatorReadiness)
	}
	if len(summary.BindingStates) != 2 {
		t.Fatalf("expected two binding states, got %+v", summary.BindingStates)
	}
	for _, state := range summary.BindingStates {
		if state.Source != ingest.RunSourceWXFeedList {
			continue
		}
		if !state.Runnable {
			t.Fatalf("expected feed list binding runnable, got %+v", state)
		}
		if state.Diagnostic != nil {
			t.Fatalf("expected no blocker diagnostic for runnable feed binding, got %+v", state)
		}
		return
	}
	t.Fatalf("expected feed list binding state, got %+v", summary.BindingStates)
}

func TestServiceGetSummarySeparatesSourceDataFreshnessFromSyncFreshness(t *testing.T) {
	repo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 4, 11, 0, 0, 0, time.FixedZone("CST", 8*3600))
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	oldCollectedAt := time.Date(2026, 7, 3, 19, 8, 38, 0, time.FixedZone("CST", 8*3600))
	if err := repo.ReplaceEvents(context.Background(), "hub:user:alice", []ingest.Event{{
		ID:          "wx-comment-video-team-runtime-proof-001",
		BatchID:     "import-20260704T030359.717133500Z",
		HubUserID:   "hub:user:alice",
		MachineID:   "wx-channel-import-02",
		EventType:   "video_snapshot",
		Source:      ingest.RunSourceWXCommentExport,
		CollectedAt: oldCollectedAt,
		VideoCount:  1,
	}}); err != nil {
		t.Fatalf("replace events failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:            "wx_comment_export-20260704T030359.717133500Z",
		Source:        ingest.RunSourceWXCommentExport,
		Status:        "success",
		StartedAt:     now,
		FinishedAt:    now,
		BatchID:       "import-20260704T030359.717133500Z",
		EventCount:    1,
		LatestEventAt: oldCollectedAt,
		LastEventType: "video_snapshot",
	}); err != nil {
		t.Fatalf("append run failed: %v", err)
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}

	if len(summary.SourceStatuses) != 1 {
		t.Fatalf("expected one source status, got %+v", summary.SourceStatuses)
	}
	status := summary.SourceStatuses[0]
	if status.FreshnessState != "stale" {
		t.Fatalf("expected data freshness stale, got %+v", status)
	}
	if status.SyncFreshnessState != "fresh" {
		t.Fatalf("expected sync freshness fresh, got %+v", status)
	}
	if !status.LatestEventAt.Equal(oldCollectedAt) {
		t.Fatalf("expected latest event timestamp to stay on imported data time, got %+v", status)
	}
	if !status.LastSuccessAt.Equal(now) {
		t.Fatalf("expected last success timestamp to stay on sync completion time, got %+v", status)
	}
}

func TestServiceListBindingStatesFallsBackToCanonicalLocalWXChannelHealth(t *testing.T) {
	repo := ingestdemo.NewRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"clients":0}`))
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = rewriteHostTransport{
		base: client.Transport,
		rewrites: map[string]string{
			"127.0.0.1:2026": server.URL,
		},
		failHosts: map[string]error{
			"127.0.0.1:15347": errors.New("dial tcp 127.0.0.1:15347: connectex: No connection could be made because the target machine actively refused it."),
		},
	}

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXFeedListClient: ingest.WXFeedListClient{
			Client: client,
		},
	})

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:    ingest.RunSourceWXFeedList,
		BaseURL:   "http://127.0.0.1:15347",
		Username:  "gh_demo",
		UpdatedAt: time.Date(2026, 7, 4, 10, 10, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}

	states, err := service.ListBindingStates(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list binding states failed: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected one binding state, got %+v", states)
	}
	if states[0].BaseURL != "http://127.0.0.1:2026" {
		t.Fatalf("expected canonical local fallback base URL, got %+v", states[0])
	}
	if states[0].Runnable {
		t.Fatalf("expected live health with zero clients to stay blocked, got %+v", states[0])
	}
	if !strings.Contains(states[0].RetryReason, "feed list has no ready page client") {
		t.Fatalf("expected no-ready-client guidance after fallback, got %+v", states[0])
	}
	if states[0].Diagnostic == nil || states[0].Diagnostic.Code == "" || states[0].Diagnostic.WSHealthURL == "" {
		t.Fatalf("expected structured feed list diagnostic after fallback, got %+v", states[0])
	}
}

func TestServiceImportWXFeedListSelfHealsStaleLoopbackBindingToCanonicalLocalAPI(t *testing.T) {
	repo := ingestdemo.NewRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":1}`))
		case "/api/channels/contact/feed/list":
			if r.URL.Query().Get("username") != "gh_self_heal" {
				t.Fatalf("unexpected username: %s", r.URL.Query().Get("username"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"message": "success",
				"data": {
					"object": [{
						"id": "feed-heal-1",
						"objectNonceId": "nonce-heal-1",
						"contact": { "nickname": "自愈作者" },
						"objectDesc": {
							"description": "自愈回退后的视频列表样本",
							"media": [{
								"commentCount": 21
							}]
						}
					}]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = rewriteHostTransport{
		base: client.Transport,
		rewrites: map[string]string{
			"127.0.0.1:2026": server.URL,
		},
		failHosts: map[string]error{
			"127.0.0.1:15347": errors.New("dial tcp 127.0.0.1:15347: connectex: No connection could be made because the target machine actively refused it."),
		},
	}

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXFeedListClient: ingest.WXFeedListClient{
			Client: client,
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 10, 20, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	summary, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, "http://127.0.0.1:15347", "gh_self_heal", "")
	if err != nil {
		t.Fatalf("expected import success after local fallback, got %v", err)
	}
	if summary.TotalEvents != 1 {
		t.Fatalf("expected 1 imported event, got %+v", summary)
	}
	if len(summary.BindingStates) != 1 || summary.BindingStates[0].BaseURL != "http://127.0.0.1:2026" {
		t.Fatalf("expected summary to expose canonical local binding, got %+v", summary.BindingStates)
	}

	bindings, err := service.ListBindings(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected one saved binding, got %+v", bindings)
	}
	if bindings[0].BaseURL != "http://127.0.0.1:2026" {
		t.Fatalf("expected stale local port to self-heal to canonical local API, got %+v", bindings[0])
	}
}

func TestServiceImportWXFeedListShortCircuitsWhenLiveHealthHasNoReadyClient(t *testing.T) {
	repo := ingestdemo.NewRepository()
	fetchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":0}`))
		case "/api/channels/contact/feed/list":
			fetchCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"object":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXFeedListClient: ingest.WXFeedListClient{
			Client: server.Client(),
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	_, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, server.URL, "gh_blocked", "")
	if err == nil {
		t.Fatal("expected import error when live health has no ready client")
	}
	if !strings.Contains(err.Error(), "没有就绪的视频号客户端") {
		t.Fatalf("expected live health guidance error, got %v", err)
	}
	if fetchCalls != 0 {
		t.Fatalf("expected feed list fetch to be skipped when health is already blocked, got %d calls", fetchCalls)
	}
}

func TestServiceRefreshWXCommentExportBindingShortCircuitsWhenLiveHealthHasNoReadyClient(t *testing.T) {
	repo := ingestdemo.NewRepository()
	exportCalls := 0
	commentPath := filepath.Join(t.TempDir(), "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-blocked-1",
  "objectNonceId": "nid-blocked-1",
  "title": "blocked comment export",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"需要先等页面 ready","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-04T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write comment export fixture failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"clients":1,"ready_clients":0,"feed_ready_clients":0,"client_list":[{"page_path":"/web/pages/video/detail","href":"https://channels.weixin.qq.com/web/pages/video/detail","api_ready":false,"supports_feed":false}]}}`))
		case "/api/channels/feed/comment/export":
			exportCalls++
			t.Fatalf("comment export endpoint should not be called when live health is already blocked")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXCommentClient: ingest.WXCommentExportClient{
			BaseURL: server.URL,
			Client:  server.Client(),
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	_, err := service.RefreshWXCommentExportBinding(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, ingest.SourceBinding{
		Source: ingest.RunSourceWXCommentExport,
		Path:   commentPath,
	})
	if err == nil {
		t.Fatal("expected refresh error when live health has no ready client")
	}
	if !strings.Contains(err.Error(), "没有就绪的视频号客户端") {
		t.Fatalf("expected live health guidance error, got %v", err)
	}
	if exportCalls != 0 {
		t.Fatalf("expected comment export endpoint to be skipped when health is blocked, got %d calls", exportCalls)
	}
}

func TestServiceGetSummaryNormalizesFeedListNoUsableVideosIntoLiveBindingReason(t *testing.T) {
	repo := ingestdemo.NewRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"clients":0}`))
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXFeedListClient: ingest.WXFeedListClient{
			Client: server.Client(),
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 10, 40, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:    ingest.RunSourceWXFeedList,
		BaseURL:   server.URL,
		Username:  "gh_demo",
		UpdatedAt: time.Date(2026, 7, 4, 10, 35, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_feed_list-failed",
		Source:     ingest.RunSourceWXFeedList,
		Status:     "failed",
		StartedAt:  time.Date(2026, 7, 4, 10, 36, 0, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt: time.Date(2026, 7, 4, 10, 37, 0, 0, time.FixedZone("CST", 8*3600)),
		Message:    "wx_channel feed list returned no usable videos",
	}); err != nil {
		t.Fatalf("append run failed: %v", err)
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}
	if !strings.Contains(summary.LastFailureMessage, "feed list has no ready page client") {
		t.Fatalf("expected summary-level binding reason normalization, got %+v", summary)
	}
	if len(summary.SourceStatuses) != 1 || !strings.Contains(summary.SourceStatuses[0].LastError, "feed list has no ready page client") {
		t.Fatalf("expected source status error to normalize to live binding reason, got %+v", summary.SourceStatuses)
	}
}

func TestServiceGetSummaryBuildsOperatorReadinessBlockedForCommentExportWithoutReadyClient(t *testing.T) {
	repo := ingestdemo.NewRepository()
	commentPath := filepath.Join(t.TempDir(), "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-comment-blocked-1",
  "objectNonceId": "nid-comment-blocked-1",
  "title": "comment blocked",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"页面没 ready 时要直接提示","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-04T09:20:00+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write comment export fixture failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"clients":1,"ready_clients":0,"feed_ready_clients":0,"client_list":[{"page_path":"/web/pages/video/detail","href":"https://channels.weixin.qq.com/web/pages/video/detail","api_ready":false,"supports_feed":false}]}}`))
		case "/ws/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXCommentClient: ingest.WXCommentExportClient{
			BaseURL: server.URL,
			Client:  server.Client(),
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 9, 45, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        commentPath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 4, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert comment binding failed: %v", err)
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}

	if summary.OperatorReadiness.State != "blocked" {
		t.Fatalf("expected blocked operator readiness, got %+v", summary.OperatorReadiness)
	}
	if summary.OperatorReadiness.BlockedSources != 1 {
		t.Fatalf("expected one blocked source, got %+v", summary.OperatorReadiness)
	}
	if len(summary.OperatorReadiness.Blocked) != 1 || summary.OperatorReadiness.Blocked[0].Source != ingest.RunSourceWXCommentExport {
		t.Fatalf("expected blocked comment export source, got %+v", summary.OperatorReadiness.Blocked)
	}
	if !strings.Contains(summary.OperatorReadiness.Blocked[0].RetryReason, "comment export has no ready page client") {
		t.Fatalf("expected live health retry reason, got %+v", summary.OperatorReadiness.Blocked[0])
	}
	if !strings.Contains(summary.OperatorReadiness.Blocked[0].Action, "视频号独立客户端里切到位且接口已就绪") {
		t.Fatalf("expected blocked action to point to standalone client readiness, got %+v", summary.OperatorReadiness.Blocked[0])
	}
	if summary.OperatorReadiness.Blocked[0].Diagnostic == nil || summary.OperatorReadiness.Blocked[0].Diagnostic.ReadyClientCount == nil || *summary.OperatorReadiness.Blocked[0].Diagnostic.ReadyClientCount != 0 {
		t.Fatalf("expected blocked comment export diagnostic with zero ready clients, got %+v", summary.OperatorReadiness.Blocked[0])
	}
}

func TestServiceGetSummaryClearsRecoveredSourceErrorWithoutOverwritingHistoricalFailure(t *testing.T) {
	repo := ingestdemo.NewRepository()
	currentNow := time.Date(2026, 7, 4, 13, 40, 0, 0, time.FixedZone("CST", 8*3600))
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return currentNow
		},
	})

	eventTime := time.Date(2026, 7, 4, 13, 31, 24, 0, time.FixedZone("CST", 8*3600))
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_feed_list-failed",
		Source:     ingest.RunSourceWXFeedList,
		Status:     "failed",
		StartedAt:  time.Date(2026, 7, 4, 12, 46, 29, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt: time.Date(2026, 7, 4, 12, 46, 29, 0, time.FixedZone("CST", 8*3600)),
		Message:    "wx_channel feed list returned no usable videos",
	}); err != nil {
		t.Fatalf("append failed run failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/contact/feed/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"object": [{
					"id": "feed-recovered-1",
					"objectNonceId": "nonce-recovered-1",
					"contact": { "nickname": "Recovered Channel" },
					"objectDesc": {
						"description": "Recovered feed list video",
						"media": [{ "commentCount": 12 }]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	currentNow = eventTime
	service = ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return currentNow
		},
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: server.URL,
		},
	})
	if _, err := service.ImportWXFeedList(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, "", "v2_demo_recovered@finder", ""); err != nil {
		t.Fatalf("import wx feed list failed: %v", err)
	}

	currentNow = time.Date(2026, 7, 4, 13, 40, 0, 0, time.FixedZone("CST", 8*3600))
	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}
	if summary.State != "ready" || summary.FreshnessState != "fresh" {
		t.Fatalf("expected recovered summary to be ready/fresh, got %+v", summary)
	}
	if summary.LastFailureMessage != "wx_channel feed list returned no usable videos" {
		t.Fatalf("expected historical failure message to stay concrete, got %+v", summary)
	}
	if len(summary.SourceStatuses) != 1 {
		t.Fatalf("expected one source status, got %+v", summary.SourceStatuses)
	}
	if summary.SourceStatuses[0].State != "ready" || summary.SourceStatuses[0].LastError != "" {
		t.Fatalf("expected recovered source status to clear current error, got %+v", summary.SourceStatuses[0])
	}
}

func TestServiceImportWXCommentExportRecordsRunFinishedAtFromExecutionTime(t *testing.T) {
	repo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 4, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	commentPath := filepath.Join(t.TempDir(), "comment-export-stale.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-stale-run-001",
  "objectNonceId": "nid-stale-run-001",
  "title": "历史评论导出样本",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"导入完成时间应该记录为这次执行时间","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T19:08:38+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}

	summary, err := service.ImportWXCommentExport(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, commentPath)
	if err != nil {
		t.Fatalf("expected import success, got %v", err)
	}
	if !summary.LastSuccessAt.Equal(now) {
		t.Fatalf("expected summary lastSuccessAt to reflect execution time, got %+v", summary)
	}
	if summary.LatestEventAt.Equal(now) {
		t.Fatalf("expected latestEventAt to keep payload recency rather than execution time, got %+v", summary)
	}
	if summary.LastEventType != "video_snapshot" {
		t.Fatalf("expected summary lastEventType to stay source-specific, got %+v", summary)
	}

	runs, err := repo.ListRuns(context.Background(), "hub:user:alice")
	if err != nil {
		t.Fatalf("list runs failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %+v", runs)
	}
	if !runs[0].FinishedAt.Equal(now) {
		t.Fatalf("expected run finishedAt to reflect execution time, got %+v", runs[0])
	}
	if runs[0].LatestEventAt.Equal(now) {
		t.Fatalf("expected run latestEventAt to preserve payload event time, got %+v", runs[0])
	}
	if runs[0].LastEventType != "video_snapshot" {
		t.Fatalf("expected run lastEventType to follow latest event metadata, got %+v", runs[0])
	}
}

func TestServiceImportHistoricalCommentExportDoesNotLeaveSummaryInErrorAfterNewSuccess(t *testing.T) {
	repo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 4, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_feed_list-failed-before-comment-recover",
		Source:     ingest.RunSourceWXFeedList,
		Status:     "failed",
		StartedAt:  time.Date(2026, 7, 4, 10, 16, 0, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt: time.Date(2026, 7, 4, 10, 16, 19, 0, time.FixedZone("CST", 8*3600)),
		Message:    "feed list has no ready page client",
	}); err != nil {
		t.Fatalf("append failed run failed: %v", err)
	}

	commentPath := filepath.Join(t.TempDir(), "comment-export-historical.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-historical-run-001",
  "objectNonceId": "nid-historical-run-001",
  "title": "历史评论导出恢复样本",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"新的导入成功不该因为旧数据时间被判成 error","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T19:08:38+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}

	summary, err := service.ImportWXCommentExport(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, commentPath)
	if err != nil {
		t.Fatalf("expected import success, got %v", err)
	}
	if summary.FreshnessState == "error" {
		t.Fatalf("expected historical import success to downgrade recency to stale rather than error, got %+v", summary)
	}
	if summary.LastFailureAt.After(summary.LastSuccessAt) {
		t.Fatalf("expected latest success execution time to outrank earlier failure, got %+v", summary)
	}
}

func TestServiceRetryLastFailedImportReturnsBindingGuidanceForFeedListWithoutReadyClient(t *testing.T) {
	repo := ingestdemo.NewRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`clients=0`))
	}))
	defer server.Close()

	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		WXFeedListClient: ingest.WXFeedListClient{
			Client: server.Client(),
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:    ingest.RunSourceWXFeedList,
		BaseURL:   server.URL,
		Username:  "gh_demo",
		UpdatedAt: time.Date(2026, 7, 4, 9, 40, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_feed_list-failed",
		Source:     ingest.RunSourceWXFeedList,
		Status:     "failed",
		StartedAt:  time.Date(2026, 7, 4, 9, 50, 0, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt: time.Date(2026, 7, 4, 9, 51, 0, 0, time.FixedZone("CST", 8*3600)),
		Message:    "upstream client unavailable",
	}); err != nil {
		t.Fatalf("append run failed: %v", err)
	}

	_, err := service.RetryLastFailedImport(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, ingest.RunSourceWXFeedList)
	if err == nil {
		t.Fatal("expected retry failure for feed list without ready page client")
	}
	if !strings.Contains(err.Error(), "saved binding") || !strings.Contains(err.Error(), "feed list has no ready page client") {
		t.Fatalf("expected live binding guidance error, got %v", err)
	}
}

func TestServiceRetryLastFailedImportReturnsBindingGuidanceForMissingPath(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 3, 13, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	missingPath := filepath.Join(t.TempDir(), "missing-comment-export.json")
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:    ingest.RunSourceWXCommentExport,
		Path:      missingPath,
		UpdatedAt: time.Date(2026, 7, 3, 12, 40, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_comment_export-failed",
		Source:     ingest.RunSourceWXCommentExport,
		Status:     "failed",
		StartedAt:  time.Date(2026, 7, 3, 12, 50, 0, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt: time.Date(2026, 7, 3, 12, 51, 0, 0, time.FixedZone("CST", 8*3600)),
		Message:    "open path failed",
	}); err != nil {
		t.Fatalf("append run failed: %v", err)
	}

	_, err := service.RetryLastFailedImport(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, ingest.RunSourceWXCommentExport)
	if err == nil {
		t.Fatal("expected retry failure for missing path")
	}
	if !strings.Contains(err.Error(), "saved binding") || !strings.Contains(err.Error(), "文件不存在") {
		t.Fatalf("expected binding guidance error, got %v", err)
	}
}

func TestServiceResetClearsSummary(t *testing.T) {
	repo := ingestdemo.NewRepository()
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	if _, err := service.AppendMockBatch(context.Background(), ingest.SummaryQuery{}); err != nil {
		t.Fatalf("expected append success, got %v", err)
	}
	summary, err := service.Reset(context.Background(), ingest.SummaryQuery{})
	if err != nil {
		t.Fatalf("expected reset success, got %v", err)
	}

	if summary.State != "idle" {
		t.Fatalf("expected idle state after reset, got %q", summary.State)
	}

	after, err := service.GetSummary(context.Background(), ingest.SummaryQuery{})
	if err != nil {
		t.Fatalf("expected get summary success, got %v", err)
	}
	if after.TotalEvents != 0 {
		t.Fatalf("expected 0 events after reset, got %d", after.TotalEvents)
	}
}

func TestServiceAppendMultipleMockBatchesKeepsBroaderRecentEventWindow(t *testing.T) {
	repo := ingestdemo.NewRepository()
	base := time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600))
	callCount := 0
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			value := base.Add(time.Duration(callCount) * 10 * time.Minute)
			callCount++
			return value
		},
	})

	for i := 0; i < 3; i++ {
		if _, err := service.AppendMockBatch(context.Background(), ingest.SummaryQuery{
			HubUserID:   "hub:user:alice",
			DeviceCount: 3,
		}); err != nil {
			t.Fatalf("append batch %d failed: %v", i+1, err)
		}
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("expected get summary success, got %v", err)
	}

	if summary.TotalEvents != 9 {
		t.Fatalf("expected 9 events after 3 batches, got %d", summary.TotalEvents)
	}
	if len(summary.RecentEvents) != 9 {
		t.Fatalf("expected recent event window to keep 9 events, got %d", len(summary.RecentEvents))
	}
	if summary.RecentEvents[0].BatchID != "mock-batch-003" {
		t.Fatalf("expected newest event from batch 003, got %+v", summary.RecentEvents[0])
	}
	if summary.RecentEvents[0].Keyword != "门店团购预算" {
		t.Fatalf("expected newest topic metadata from third profile, got %+v", summary.RecentEvents[0])
	}
	if summary.RecentEvents[3].BatchID != "mock-batch-002" {
		t.Fatalf("expected second newest group from batch 002, got %+v", summary.RecentEvents[3])
	}
	if summary.RecentEvents[6].BatchID != "mock-batch-001" {
		t.Fatalf("expected oldest retained group from batch 001, got %+v", summary.RecentEvents[6])
	}
}

func TestServiceAppendAndListOperatorRuns(t *testing.T) {
	repo := ingestdemo.NewRepository()
	currentNow := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return currentNow
		},
	})

	if err := service.AppendOperatorRun(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, ingest.OperatorRun{
		ID:               "operator-cycle-001",
		Status:           "ok",
		StartedAt:        currentNow.Add(-2 * time.Minute),
		FinishedAt:       currentNow,
		WorkspaceID:      "ws-default-hub-user-alice",
		LatestBatchID:    "import-001",
		AttemptedSources: 2,
		SucceededSources: 2,
		FreshnessState:   "fresh",
		ReportHeadline:   "日报 headline",
		TopVideoTitle:    "热视频标题",
		TopTopicKeyword:  "热门话题",
	}); err != nil {
		t.Fatalf("append operator run failed: %v", err)
	}

	runs, err := service.ListOperatorRuns(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list operator runs failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one operator run, got %+v", runs)
	}
	if runs[0].ReportHeadline != "日报 headline" || runs[0].WorkspaceID != "ws-default-hub-user-alice" {
		t.Fatalf("expected persisted operator run payload, got %+v", runs[0])
	}
}
