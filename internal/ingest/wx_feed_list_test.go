package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWXFeedListClientResolveAccountByExactUsername(t *testing.T) {
	username := "v2_exact_account@finder"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/contact/feed/list" {
			t.Fatalf("expected feed-list path, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("username"); got != username {
			t.Fatalf("expected exact username query, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "code": 0,
  "data": {"data": {"BaseResponse": {"Ret": 0}, "contact": {
    "username": "v2_exact_account@finder",
    "nickname": "真实账号",
    "signature": "账号简介",
    "headUrl": "https://example.com/avatar.png",
    "extInfo": {"province": "Henan", "city": "Zhengzhou"},
    "authInfo": {"authProfession": "新闻传媒"},
    "tags": ["新闻", {"name": "媒体"}]
  }}}
}`))
	}))
	defer server.Close()

	account, err := (WXFeedListClient{BaseURL: server.URL}).ResolveAccount(context.Background(), username)
	if err != nil {
		t.Fatalf("resolve exact account: %v", err)
	}
	if account.Username != username || account.Nickname != "真实账号" || account.Signature != "账号简介" {
		t.Fatalf("unexpected resolved account: %+v", account)
	}
	if account.Region != "Henan Zhengzhou" || account.Industry != "新闻传媒" || len(account.Tags) != 2 || account.Tags[1] != "媒体" {
		t.Fatalf("expected account profile attributes, got %+v", account)
	}
}

func TestServiceSearchWXFeedAccountsDoesNotFuzzySearchExactUsername(t *testing.T) {
	username := "v2_exact_account@finder"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/channels/contact/search" {
			t.Fatalf("exact username must not use fuzzy contact search")
		}
		if r.URL.Path != "/api/channels/contact/feed/list" {
			t.Fatalf("expected exact account lookup, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "code": 0,
  "data": {"data": {"BaseResponse": {"Ret": 0}, "contact": {
    "username": "v2_exact_account@finder",
    "nickname": "真实账号"
  }}}
}`))
	}))
	defer server.Close()

	service := NewService(nil, ServiceConfig{
		WXFeedListClient: WXFeedListClient{BaseURL: server.URL},
	})
	accounts, err := service.SearchWXFeedAccounts(context.Background(), SummaryQuery{}, server.URL, username, "")
	if err != nil {
		t.Fatalf("search exact account: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Username != username || accounts[0].Nickname != "真实账号" {
		t.Fatalf("unexpected exact account results: %+v", accounts)
	}
}

func TestWXFeedListClientResolveAccountRejectsMismatchedUsername(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "code": 0,
  "data": {"data": {"BaseResponse": {"Ret": -2}, "contact": {
    "username": "v2_other_account@finder",
    "nickname": "相近账号"
  }}}
}`))
	}))
	defer server.Close()

	_, err := (WXFeedListClient{BaseURL: server.URL}).ResolveAccount(
		context.Background(),
		"v2_requested_account@finder",
	)
	if err == nil {
		t.Fatal("expected mismatched account lookup to fail")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "v2_requested_account@finder") || !strings.Contains(got, "检查账号 ID") {
		t.Fatalf("expected actionable exact username error, got %q", got)
	}
}

func TestWXFeedListClientFetchVideoProfileParsesRealCoverFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" {
			t.Fatalf("expected profile path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("object_id") != "video-profile-1" || r.URL.Query().Get("nonce_id") != "nonce-profile-1" {
			t.Fatalf("unexpected profile query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "code": 0,
  "data": {"data": {"object": {
    "id": "video-profile-1",
    "createtime": 1785901095,
    "objectDesc": {"description": "详情视频", "media": [{"thumbUrl": "https://cdn.example.test/profile-thumb.jpg", "coverUrl": "https://cdn.example.test/profile-cover.jpg", "url": "https://cdn.example.test/video.mp4?", "urlToken": "token=profile", "decodeKey": "987654321", "spec": [{"fileFormat": "xWT111", "bitRate": 43}, {"fileFormat": "xWT157", "bitRate": 20}]}]}
  }}}
}`))
	}))
	defer server.Close()

	item, err := (WXFeedListClient{BaseURL: server.URL}).FetchVideoProfile(context.Background(), "video-profile-1", "nonce-profile-1")
	if err != nil {
		t.Fatalf("fetch video profile: %v", err)
	}
	if item.ID != "video-profile-1" || item.CoverURL != "https://cdn.example.test/profile-thumb.jpg" {
		t.Fatalf("expected parsed profile cover, got %+v", item)
	}
	if item.VideoURL != "https://cdn.example.test/video.mp4?token=profile&X-snsvideoflag=xWT157" {
		t.Fatalf("expected request-time profile video URL, got %q", item.VideoURL)
	}
	if item.DecryptKey != "987654321" {
		t.Fatalf("expected request-time profile decrypt key, got %q", item.DecryptKey)
	}
	if !item.PublishedAt.Equal(time.Unix(1785901095, 0).UTC()) {
		t.Fatalf("expected profile publication time, got %v", item.PublishedAt)
	}
}

func TestJoinWXFeedMediaURLKeepsOriginalWhenNoUsableSpec(t *testing.T) {
	got := joinWXFeedMediaURL(map[string]any{
		"url":      "https://cdn.example.test/video.mp4",
		"urlToken": "?token=profile",
		"spec": []any{
			map[string]any{"fileFormat": "", "bitRate": 20},
			map[string]any{"fileFormat": "xWT111", "bitRate": 0},
		},
	})
	if got != "https://cdn.example.test/video.mp4?token=profile" {
		t.Fatalf("expected original URL without usable spec, got %q", got)
	}
}

func TestWXFeedListClientFetchVideoProfileParsesDirectRootObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "code": 0,
  "id": "video-profile-root",
  "objectNonceId": "nonce-root",
  "objectDesc": {"description": "根对象视频", "media": [{"url": "https://cdn.example.test/root.mp4", "urlToken": "?signed=1"}]}
}`))
	}))
	defer server.Close()

	item, err := (WXFeedListClient{BaseURL: server.URL}).FetchVideoProfile(context.Background(), "video-profile-root", "nonce-root")
	if err != nil {
		t.Fatalf("fetch direct-root profile: %v", err)
	}
	if item.ID != "video-profile-root" || item.VideoURL != "https://cdn.example.test/root.mp4?signed=1" {
		t.Fatalf("expected direct-root video profile, got %+v", item)
	}
}

func TestWXFeedListClientResolveVideoShareParsesVideoData(t *testing.T) {
	shareURL := "https://weixin.qq.com/sph/share-video-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" {
			t.Fatalf("expected shared profile path, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("url"); got != shareURL {
			t.Fatalf("expected share URL query, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "code": 0,
  "data": {"object": {
    "id": "shared-video-1",
    "objectNonceId": "shared-nonce-1",
    "createtime": 1785901095,
    "likeCount": 41,
    "commentCount": 12,
    "favCount": 9,
    "forwardCount": 3,
    "contact": {"username": "shared_account@finder", "nickname": "分享账号"},
    "objectDesc": {"description": "从分享链接识别的视频", "media": [{
      "thumbUrl": "https://cdn.example.test/shared-cover.jpg",
      "url": "https://cdn.example.test/shared.mp4?",
      "urlToken": "token=shared",
      "decodeKey": "shared-key",
      "spec": [{"fileFormat": "xWT111", "bitRate": 30}]
    }]}
  }}
}`))
	}))
	defer server.Close()

	item, err := (WXFeedListClient{BaseURL: server.URL}).ResolveVideoShare(context.Background(), shareURL)
	if err != nil {
		t.Fatalf("resolve shared video: %v", err)
	}
	if item.ID != "shared-video-1" || item.ObjectNonceID != "shared-nonce-1" {
		t.Fatalf("expected shared identifiers, got %+v", item)
	}
	if item.Title != "从分享链接识别的视频" || item.ChannelUsername != "shared_account@finder" || item.ChannelName != "分享账号" {
		t.Fatalf("expected shared metadata, got %+v", item)
	}
	if item.LikeCount != 41 || item.CommentCount != 12 || item.FavCount != 9 || item.ForwardCount != 3 {
		t.Fatalf("expected four interaction metrics, got %+v", item)
	}
	if item.CoverURL != "https://cdn.example.test/shared-cover.jpg" || item.VideoURL == "" || item.DecryptKey != "shared-key" {
		t.Fatalf("expected shared media metadata, got %+v", item)
	}
	if !item.PublishedAt.Equal(time.Unix(1785901095, 0).UTC()) {
		t.Fatalf("expected publication time, got %v", item.PublishedAt)
	}
}

func TestWXFeedListClientResolveVideoShareRejectsUntrustedURL(t *testing.T) {
	_, err := (WXFeedListClient{}).ResolveVideoShare(context.Background(), "https://example.com/video")
	if err == nil || !strings.Contains(err.Error(), "微信视频号") {
		t.Fatalf("expected unsupported shared URL error, got %v", err)
	}
}

func TestParseWXFeedListResultFromPayloadPreservesNumericPublicationAndMetrics(t *testing.T) {
	raw := []byte(`{
  "code": 0,
  "data": {
    "data": {
      "BaseResponse": {"Ret": 0},
      "objectList": [{
        "id": "14981381041469983151",
        "createtime": 1785919790,
        "likeCount": 241,
        "commentCount": 229,
        "favCount": 251,
        "forwardCount": 49,
        "objectDesc": {"description": "折叠屏样本"},
        "objectExtend": {"monotonicData": {"countInfo": {"readCount": 12345}}}
      }]
    }
  }
}`)

	result, err := parseWXFeedListResultFromPayload("折叠屏", raw)
	if err != nil {
		t.Fatalf("parse feed search payload: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(result.Items))
	}
	item := result.Items[0]
	if !item.PublishedAt.Equal(time.Unix(1785919790, 0).UTC()) {
		t.Fatalf("expected numeric createtime to survive, got %v", item.PublishedAt)
	}
	if item.PlayCount != 12345 || item.LikeCount != 241 || item.CommentCount != 229 || item.FavCount != 251 || item.ForwardCount != 49 {
		t.Fatalf("expected all source metrics, got %+v", item)
	}
}

func TestParseWXFeedListResultPreservesNestedContactUsername(t *testing.T) {
	raw := []byte(`{
  "code": 0,
  "data": {"data": {"BaseResponse": {"Ret": 0}, "objectList": [{
    "id": "video-with-account",
    "objectDesc": {"description": "真实账号视频"},
    "contact": {"username": "v2_real_account@finder", "nickname": "真实账号"}
  }]}}
}`)

	result, err := parseWXFeedListResultFromPayload("saved-account@finder", raw)
	if err != nil {
		t.Fatalf("parse feed payload: %v", err)
	}
	if got := result.Items[0].ChannelUsername; got != "v2_real_account@finder" {
		t.Fatalf("expected contact username, got %q", got)
	}
}

func TestWXFeedBatchUsesActualObservationTimeForEveryVideo(t *testing.T) {
	observedAt := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	result := WXFeedListResult{Items: []WXFeedListItem{{ID: "first"}, {ID: "second"}}}
	for _, events := range [][]Event{
		BuildEventsFromWXFeedList("alice", "machine", result, observedAt),
		BuildEventsFromWXVideoSearch("alice", "machine", result, "AI", observedAt),
	} {
		for _, event := range events {
			if !event.CollectedAt.Equal(observedAt) {
				t.Fatalf("batch observation time cannot depend on row index: %+v", events)
			}
		}
	}
}

func TestBuildEventsFromWXFeedListUsesSavedUsernameOnlyAsFeedFallback(t *testing.T) {
	result := WXFeedListResult{
		Username: "saved-account@finder",
		Items:    []WXFeedListItem{{ID: "video-without-contact", Title: "没有 contact 的视频"}},
	}

	feedEvents := BuildEventsFromWXFeedList("hub:user:test", "machine-1", result, time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC))
	if len(feedEvents) != 1 || feedEvents[0].ChannelUsername != "saved-account@finder" {
		t.Fatalf("expected feed binding fallback username, got %+v", feedEvents)
	}

	searchEvents := BuildEventsFromWXVideoSearch("hub:user:test", "machine-1", result, "关键词", time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC))
	if len(searchEvents) != 1 || searchEvents[0].ChannelUsername != "" {
		t.Fatalf("video search must not guess an account username, got %+v", searchEvents)
	}
}
