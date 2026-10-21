package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type fixedAccountContextResolver struct {
	workspaceID string
}

func (r fixedAccountContextResolver) Resolve(context.Context, workspace.ContextQuery) (workspace.ResolvedContext, error) {
	return workspace.ResolvedContext{Workspace: workspace.DefaultWorkspace{ID: r.workspaceID, OwnerUserID: "hub:user:accounts"}}, nil
}

type accountAPIDataProvider struct {
	events []ingest.Event
}

func (p accountAPIDataProvider) ListEvents(context.Context, ingest.SummaryQuery) ([]ingest.Event, error) {
	return append([]ingest.Event(nil), p.events...), nil
}

func (accountAPIDataProvider) ListVideoMetricObservations(context.Context, ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	return nil, nil
}

func newAccountAPITestServer(t *testing.T) (*server, insight.Account) {
	t.Helper()
	resolver := fixedAccountContextResolver{workspaceID: "workspace:accounts"}
	accountRepo := insightdemo.NewAccountRepository()
	accountService := insight.NewAccountService(insight.AccountServiceConfig{
		Repository:               accountRepo,
		WorkspaceContextResolver: resolver,
	})
	account, err := accountService.UpsertObservedProfile(context.Background(), insight.AccountQuery{HubUserID: "hub:user:accounts"}, insight.AccountObservedProfile{
		ChannelUsername: "alice_account",
		ChannelName:     "Alice 账号",
		Signature:       "真实账号简介",
		ObservedAt:      time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	dataCenter := insight.NewDataCenterService(insight.DataCenterServiceConfig{
		DataProvider: accountAPIDataProvider{events: []ingest.Event{{
			ID: "event-video-1", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList,
			CollectedAt: time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC), ObjectID: "video-1",
			ChannelUsername: "alice_account", ChannelName: "Alice 账号", Title: "真实视频",
			LikeCount: 3, CommentCount: 2, FavCount: 1, ForwardCount: 1, VideoCount: 1,
		}}},
		AccountProvider:          accountRepo,
		WorkspaceContextResolver: resolver,
		ExclusionRepository:      insightdemo.NewDataCenterExclusionRepository(),
	})

	srv := newServer(nil, nil, stubIngestService{}, nil, runtime.Status{})
	srv.accountService = accountService
	srv.dataCenterService = dataCenter
	return srv, account
}

func TestAccountAPIListDetailAndPatchKeepStableIdentityAndMetrics(t *testing.T) {
	srv, account := newAccountAPITestServer(t)
	baseURL := "/api/v1/accounts?hub_user_id=hub%3Auser%3Aaccounts&device_count=1"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, baseURL, nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listPayload struct {
		Items []struct {
			AccountID string `json:"accountId"`
			Metrics   struct {
				VideoCount *int `json:"videoCount"`
				Likes      *int `json:"likes"`
			} `json:"metrics"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listPayload.Items) != 1 || listPayload.Items[0].AccountID != account.ID {
		t.Fatalf("unexpected account list: %+v body=%s", listPayload, rec.Body.String())
	}
	if listPayload.Items[0].Metrics.VideoCount == nil || *listPayload.Items[0].Metrics.VideoCount != 1 || listPayload.Items[0].Metrics.Likes == nil || *listPayload.Items[0].Metrics.Likes != 3 {
		t.Fatalf("expected derived account metrics, got %+v", listPayload.Items[0].Metrics)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+account.ID+"?hub_user_id=hub%3Auser%3Aaccounts", nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"accountId":"`+account.ID+`"`)) {
		t.Fatalf("detail status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+account.ID+"?hub_user_id=hub%3Auser%3Aaccounts", bytes.NewBufferString(`{"region":"华东","isPriority":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"region":"华东"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"isPriority":true`)) {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"videoCount":1`)) {
		t.Fatalf("patch should keep derived metrics, body=%s", rec.Body.String())
	}
}

func TestAccountAPIVideosDefaultToPublishedAtDescending(t *testing.T) {
	resolver := fixedAccountContextResolver{workspaceID: "workspace:accounts"}
	accountRepo := insightdemo.NewAccountRepository()
	accountService := insight.NewAccountService(insight.AccountServiceConfig{
		Repository:               accountRepo,
		WorkspaceContextResolver: resolver,
	})
	account, err := accountService.UpsertObservedProfile(context.Background(), insight.AccountQuery{HubUserID: "hub:user:accounts"}, insight.AccountObservedProfile{
		ChannelUsername: "alice_account",
		ChannelName:     "Alice 账号",
	})
	if err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	dataCenter := insight.NewDataCenterService(insight.DataCenterServiceConfig{
		DataProvider: accountAPIDataProvider{events: []ingest.Event{
			{ID: "event-old", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList,
				CollectedAt: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC), PublishedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC), ObjectID: "video-old",
				ChannelUsername: "alice_account", Title: "较早发布时间", VideoCount: 1},
			{ID: "event-new", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList,
				CollectedAt: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC), PublishedAt: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC), ObjectID: "video-new",
				ChannelUsername: "alice_account", Title: "较新发布时间", VideoCount: 1},
		}},
		AccountProvider:          accountRepo,
		WorkspaceContextResolver: resolver,
		ExclusionRepository:      insightdemo.NewDataCenterExclusionRepository(),
	})

	srv := newServer(nil, nil, stubIngestService{}, nil, runtime.Status{})
	srv.accountService = accountService
	srv.dataCenterService = dataCenter
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+account.ID+"/videos?hub_user_id=hub%3Auser%3Aaccounts", nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("videos status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []struct {
			ID          string    `json:"id"`
			PublishedAt time.Time `json:"publishedAt"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode videos: %v", err)
	}
	if len(payload.Items) != 2 || payload.Items[0].ID != "video:video-new" || payload.Items[1].ID != "video:video-old" {
		t.Fatalf("expected videos ordered by publication time desc, got %+v body=%s", payload.Items, rec.Body.String())
	}
}

func TestSearchWXFeedAccountsPersistsObservedProfile(t *testing.T) {
	repo := insightdemo.NewAccountRepository()
	service := insight.NewAccountService(insight.AccountServiceConfig{Repository: repo})
	srv := newServer(nil, nil, stubIngestService{accounts: []ingest.WXFeedAccount{{
		Username:  "finder-search",
		Nickname:  "搜索账号",
		Signature: "搜索简介",
		HeadURL:   "https://example.com/search.png",
		Region:    "北京",
		Industry:  "科技",
		Tags:      []string{"AI", "科技"},
	}}}, nil, runtime.Status{})
	srv.accountService = service

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/wx-channel/feed-list/search?keyword=search&hub_user_id=hub%3Auser%3Aaccounts", nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", rec.Code, rec.Body.String())
	}
	page, err := service.List(context.Background(), insight.AccountQuery{HubUserID: "hub:user:accounts"})
	if err != nil || page.Total != 1 {
		t.Fatalf("expected searched account to be persisted, page=%+v err=%v", page, err)
	}
	stored := page.Items[0]
	if stored.Region != "北京" || stored.Industry != "科技" || len(stored.Tags) != 2 || stored.Signature != "搜索简介" {
		t.Fatalf("unexpected persisted search profile: %+v", stored)
	}
}

func TestAccountFiltersApplyBeforePagination(t *testing.T) {
	srv, alice := newAccountAPITestServer(t)
	query := insight.AccountQuery{HubUserID: "hub:user:accounts"}
	for i := 0; i < 105; i++ {
		_, err := srv.accountService.UpsertObservedProfile(context.Background(), query, insight.AccountObservedProfile{ChannelUsername: fmt.Sprintf("other-%03d", i), ChannelName: "其他账号"})
		if err != nil {
			t.Fatal(err)
		}
	}
	service, err := monitoring.NewService(monitoring.NewMemoryRepository(), monitoring.ServiceConfig{DefaultHubUserID: query.HubUserID})
	if err != nil {
		t.Fatal(err)
	}
	srv.monitoringService = service
	_, err = service.CreateTask(context.Background(), query.HubUserID, monitoring.Task{
		Name: "暂停的账号任务", Status: "paused", ScopeMode: monitoring.ScopeSelected, Interval: 30 * time.Minute,
		Targets: []monitoring.TaskTarget{{TargetKind: monitoring.TargetKindAccount, AccountID: alice.ID, BindingRef: monitoring.BindingRef{Source: ingest.RunSourceWXFeedList, TargetKey: alice.ChannelUsername}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, filter := range []string{"monitoringStatus=paused", "from=2026-08-16&to=2026-08-16", "monitoringStatus=paused&from=2026-08-16&to=2026-08-16"} {
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/accounts?hub_user_id=hub%3Auser%3Aaccounts&pageSize=20&"+filter, nil))
		var page accountAPIPage
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != alice.ID {
			t.Fatalf("filter %s: status=%d body=%s", filter, rec.Code, rec.Body.String())
		}
	}
	for _, filter := range []string{"monitoringStatus=invalid", "from=bad", "from=2026-08-17&to=2026-08-16"} {
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/accounts?hub_user_id=hub%3Auser%3Aaccounts&"+filter, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("filter %s: status=%d body=%s", filter, rec.Code, rec.Body.String())
		}
	}
}
