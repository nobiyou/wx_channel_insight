package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

func TestAccountSummariesIncludeFeedMonitorAndPausedState(t *testing.T) {
	srv, account := newAccountAPITestServer(t)
	repo := monitoring.NewMemoryRepository()
	service, err := monitoring.NewService(repo, monitoring.ServiceConfig{DefaultHubUserID: "hub:user:accounts"})
	if err != nil {
		t.Fatal(err)
	}
	srv.monitoringService = service
	task := monitoring.Task{ID: "feed-monitor", HubUserID: "hub:user:accounts", Name: "账号视频", TaskType: monitoring.TaskTypeLegacy, ScopeMode: monitoring.ScopeSelected, Status: monitoring.StatusEnabled, Interval: time.Minute, Targets: []monitoring.TaskTarget{{BindingRef: monitoring.BindingRef{Source: ingest.RunSourceWXFeedList, TargetKey: account.ChannelUsername}}}}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{monitoring.StatusEnabled, monitoring.StatusPaused} {
		task.Status = status
		if err := repo.UpdateTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/api/v1/accounts", "/api/v1/accounts/" + account.ID} {
			rec := httptest.NewRecorder()
			srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+"?hub_user_id=hub%3Auser%3Aaccounts", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: %d: %s", path, rec.Code, rec.Body.String())
			}
			var response struct {
				Items   []accountAPIItem `json:"items"`
				Account accountAPIItem   `json:"account"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			item := response.Account
			if len(response.Items) > 0 {
				item = response.Items[0]
			}
			if item.Monitoring["taskCount"] != float64(1) || item.Monitoring["enabled"] != (status == monitoring.StatusEnabled) {
				t.Fatalf("%s status=%s monitoring=%+v", path, status, item.Monitoring)
			}
		}
	}
}

type failingAccountMetricReader struct{ stubIngestService }

func (failingAccountMetricReader) ListAccountMetricRows(context.Context, ingest.SummaryQuery, []string) ([]ingest.AccountMetricRow, error) {
	return nil, errors.New("account metric storage offline")
}

func TestAccountMetricStorageFailureStaysVisible(t *testing.T) {
	srv, _ := newAccountAPITestServer(t)
	srv.ingestService = failingAccountMetricReader{}
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/accounts?hub_user_id=hub%3Auser%3Aaccounts", nil))
	var response accountAPIPage
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(response.Items) != 1 || response.Items[0].Metrics != nil || response.DataNotice != "账户派生指标暂时不可用：account metric storage offline" {
		t.Fatalf("storage error was masked: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalAccountMetricsUseSavedFactsWithoutBoundedProvider(t *testing.T) {
	srv, _ := newAccountAPITestServer(t)
	service := ingest.NewService(ingestdemo.NewRepository(), ingest.ServiceConfig{DefaultHubUserID: "hub:user:accounts"})
	srv.ingestService = service
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/accounts?hub_user_id=hub%3Auser%3Aaccounts", nil))
	var response accountAPIPage
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(response.Items) != 1 || response.Items[0].Metrics == nil || response.Items[0].Metrics.Likes == nil || *response.Items[0].Metrics.Likes != 3 {
		t.Fatalf("local saved metrics missing: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
