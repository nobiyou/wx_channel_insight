package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	operatorflow "github.com/nobiyou/wx_channel_insight/internal/operator"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

type partialMetricMonitor struct{ accountMonitorRunner }

func (partialMetricMonitor) RunTask(context.Context, ingest.SummaryQuery, monitoring.TaskTarget, string) (operatorflow.AccountMonitorResult, error) {
	return operatorflow.AccountMonitorResult{Status: monitoring.RunStatusPartial, Succeeded: 1, Errors: []operatorflow.AccountMonitorError{{Message: "后续分页将从断点继续"}}}, nil
}

func TestAccountMetricRefreshReportsPartialCoverage(t *testing.T) {
	srv, account := newAccountAPITestServer(t)
	srv.accountMonitor = partialMetricMonitor{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/refresh-metrics", strings.NewReader(`{"kind":"account","id":"`+account.ID+`"}`))
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"collectionStatus":"partial"`) || !strings.Contains(rec.Body.String(), "后续分页将从断点继续") {
		t.Fatalf("partial coverage hidden: %d %s", rec.Code, rec.Body.String())
	}
}

type metricRefreshIngest struct {
	stubIngestService
	calls int
	fail  bool
}

type refreshOwnerLocker struct{ key string }

func (l *refreshOwnerLocker) TryAcquire(_ context.Context, key string) (func(), bool, error) {
	l.key = key
	if key == "" {
		return nil, false, errors.New("workspace id is required")
	}
	return func() {}, true, nil
}

func TestMetricRefreshLocksResolvedDefaultWorkspace(t *testing.T) {
	for _, owner := range []string{"", "another-owner"} {
		store := &metricRefreshIngest{stubIngestService: stubIngestService{observations: []ingest.VideoMetricObservation{{ObjectID: "video", ObservedAt: time.Now().Add(-time.Hour)}}}}
		srv := newServer(nil, nil, store, fixedAccountContextResolver{workspaceID: "workspace"}, runtime.Status{})
		locker := &refreshOwnerLocker{}
		srv.metricRefreshLocker = locker
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/refresh-metrics", strings.NewReader(`{"kind":"video","id":"video"}`))
		if owner != "" {
			req.Header.Set(headerHubUserID, owner)
		}
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, req)
		want := owner
		if want == "" {
			want = "hub:user:accounts"
		}
		if rec.Code != 200 || locker.key != want || store.calls != 1 {
			t.Fatalf("owner %q: lock=%q calls=%d response=%d %s", owner, locker.key, store.calls, rec.Code, rec.Body.String())
		}
	}
}

func (s *metricRefreshIngest) RefreshWXVideoMetrics(context.Context, ingest.SummaryQuery, string, string) (ingest.Summary, error) {
	s.calls++
	if s.fail {
		return ingest.Summary{}, errors.New("collector offline")
	}
	return ingest.Summary{}, nil
}

func TestMetricRefreshUsesSavedFreshnessAndRetriesWithoutLosingHistory(t *testing.T) {
	store := &metricRefreshIngest{stubIngestService: stubIngestService{observations: []ingest.VideoMetricObservation{{ObjectID: "video", ObservedAt: time.Now(), LikeCount: 7}}}, fail: true}
	srv := newServer(nil, nil, store, nil, runtime.Status{})
	post := func(id string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/refresh-metrics", strings.NewReader(`{"kind":"video","id":"`+id+`"}`)))
		return rec
	}
	if rec := post("video"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"fresh"`) || store.calls != 0 {
		t.Fatalf("fresh data should not collect: %s calls=%d", rec.Body.String(), store.calls)
	}
	if rec := post("unknown"); rec.Code != 404 || store.calls != 0 {
		t.Fatalf("unknown video collected: %d %s", rec.Code, rec.Body.String())
	}
	store.observations[0].ObservedAt = time.Now().Add(-time.Hour)
	if rec := post("video"); rec.Code != 502 || store.calls != 1 {
		t.Fatalf("collector failure not surfaced: %d %s", rec.Code, rec.Body.String())
	}
	if store.observations[0].LikeCount != 7 {
		t.Fatal("collector failure changed historical metrics")
	}
	if rec := post("video"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"cooldown"`) || store.calls != 1 {
		t.Fatalf("failed collector should cool down: %s calls=%d", rec.Body.String(), store.calls)
	}
}

func TestMetricRefreshReservationSerializesDuplicateRequests(t *testing.T) {
	srv := &server{}
	release, ok := srv.reserveMetricRefresh("owner:video:a", time.Now())
	if !ok {
		t.Fatal("first reservation rejected")
	}
	if _, ok := srv.reserveMetricRefresh("owner:video:a", time.Now()); ok {
		t.Fatal("duplicate request allowed")
	}
	other, ok := srv.reserveMetricRefresh("other:video:a", time.Now())
	if !ok {
		t.Fatal("workspace reservations incorrectly shared")
	}
	other()
	release()
	if _, ok := srv.reserveMetricRefresh("owner:video:a", time.Now()); ok {
		t.Fatal("completion bypassed retry cooldown")
	}
}
