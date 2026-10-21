package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestDailyReportAPIUpsertsListsAndReadsDetail(t *testing.T) {
	resolver := fixedAccountContextResolver{workspaceID: "workspace:reports"}
	repo := insightdemo.NewDailyReportRepository()
	dashboard := insightdemo.NewDashboardRepository()
	srv := newServer(dashboardServiceForTest(dashboard), nil, nil, resolver, runtime.Status{})
	srv.dailyReportRepo = repo

	defaultRec := httptest.NewRecorder()
	srv.handleDailyReports(defaultRec, httptest.NewRequest(http.MethodPost, "/api/v1/reports/snapshot", nil))
	if defaultRec.Code != http.StatusOK || !bytes.Contains(defaultRec.Body.Bytes(), []byte(`"dataWindow":"72h"`)) {
		t.Fatalf("expected daily report to default to 72h, got %d: %s", defaultRec.Code, defaultRec.Body.String())
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports/snapshot?window=24h", bytes.NewBufferString(`{"dataWindow":"72h"}`))
	srv.handleDailyReports(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected snapshot success, got %d: %s", rec.Code, rec.Body.String())
	}
	var detail map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode snapshot response: %v", err)
	}
	reportID, _ := detail["id"].(string)
	if reportID == "" || detail["dataWindow"] != "24h" {
		t.Fatalf("unexpected snapshot response: %+v", detail)
	}

	defaultListRec := httptest.NewRecorder()
	srv.handleDailyReports(defaultListRec, httptest.NewRequest(http.MethodGet, "/api/v1/reports", nil))
	if defaultListRec.Code != http.StatusOK || !bytes.Contains(defaultListRec.Body.Bytes(), []byte(`"dataWindow":"72h"`)) || bytes.Contains(defaultListRec.Body.Bytes(), []byte(reportID)) {
		t.Fatalf("expected default report list to use only 72h archives, got %d: %s", defaultListRec.Code, defaultListRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	srv.handleDailyReports(listRec, httptest.NewRequest(http.MethodGet, "/api/v1/reports?window=24h", nil))
	if listRec.Code != http.StatusOK || !bytes.Contains(listRec.Body.Bytes(), []byte(reportID)) {
		t.Fatalf("expected report in list, got %d: %s", listRec.Code, listRec.Body.String())
	}
	allRec := httptest.NewRecorder()
	srv.handleDailyReports(allRec, httptest.NewRequest(http.MethodGet, "/api/v1/reports?window=all", nil))
	if allRec.Code != http.StatusOK || !bytes.Contains(allRec.Body.Bytes(), []byte(`"dataWindow":"all"`)) || !bytes.Contains(allRec.Body.Bytes(), []byte(`"count":0`)) {
		t.Fatalf("expected all-time archive filter without a 72h substitution, got %d: %s", allRec.Code, allRec.Body.String())
	}

	detailRec := httptest.NewRecorder()
	srv.handleDailyReports(detailRec, httptest.NewRequest(http.MethodGet, "/api/v1/reports/"+url.PathEscape(reportID), nil))
	if detailRec.Code != http.StatusOK || !bytes.Contains(detailRec.Body.Bytes(), []byte(`"snapshot"`)) {
		t.Fatalf("expected report detail, got %d: %s", detailRec.Code, detailRec.Body.String())
	}
}

func TestDailyReportSnapshotRejectsUnboundedWindows(t *testing.T) {
	resolver := fixedAccountContextResolver{workspaceID: "workspace:reports"}
	repo := insightdemo.NewDailyReportRepository()
	srv := newServer(dashboardServiceForTest(insightdemo.NewDashboardRepository()), nil, nil, resolver, runtime.Status{})
	srv.dailyReportRepo = repo
	for _, test := range []struct {
		name  string
		query string
		body  string
	}{
		{name: "query", query: "?window=all"},
		{name: "body", body: `{"dataWindow":"all"}`},
		{name: "invalid", query: "?window=7d"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/reports/snapshot"+test.query, bytes.NewBufferString(test.body))
			srv.handleDailyReports(rec, req)
			if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("24h or 72h")) {
				t.Fatalf("expected bounded-window validation, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func dashboardServiceForTest(repo insight.DashboardRepository) *insight.DashboardService {
	return insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
}
