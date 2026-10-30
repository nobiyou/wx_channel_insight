package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func newVideoExportTestServer() *server {
	base := time.Date(2026, 10, 30, 4, 0, 0, 0, time.UTC)
	var observations []ingest.VideoMetricObservation
	for i := 0; i < 55; i++ {
		observations = append(observations, ingest.VideoMetricObservation{
			ID: fmt.Sprintf("observation-%02d", i), ObjectID: fmt.Sprintf("export-%02d", i),
			Title: fmt.Sprintf("中文视频 %02d", i), ChannelName: "示例账号", ChannelUsername: "export-account",
			Keyword: "导出", Source: ingest.RunSourceWXVideoSearch, ObservedAt: base,
			PublishedAt: base.Add(-48 * time.Hour), LikeCount: i, CommentCount: 2, FavCount: 3, ForwardCount: 4,
		})
	}
	observations = append(observations, ingest.VideoMetricObservation{ID: "other", ObjectID: "other", Title: "其他账号", ChannelUsername: "other-account", ObservedAt: base, LikeCount: 999})
	srv := newDataCenterTestServer()
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{
		DataProvider: apiDataCenterProvider{observations: observations}, Now: func() time.Time { return base },
	})
	return srv
}

func videoExportRequest(t *testing.T, srv *server, query, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, dataCenterRoutePrefix+"videos/export"+query, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.routes().ServeHTTP(rec, req)
	return rec
}

func readVideoExportCSV(t *testing.T, rec *httptest.ResponseRecorder) []map[string]string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("export status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") || !strings.HasPrefix(rec.Body.String(), "\ufeff") {
		t.Fatalf("expected UTF-8 CSV with BOM, headers=%v", rec.Header())
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), ".csv") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing download/privacy headers: %v", rec.Header())
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(rec.Body.String(), "\ufeff"))).ReadAll()
	if err != nil || len(records) == 0 {
		t.Fatalf("invalid CSV: %v", err)
	}
	rows := make([]map[string]string, 0, len(records)-1)
	for _, record := range records[1:] {
		row := map[string]string{}
		for i, header := range records[0] {
			row[header] = record[i]
		}
		rows = append(rows, row)
	}
	return rows
}

func TestVideoExportIncludesAllFilteredRowsInDisplayOrder(t *testing.T) {
	srv := newVideoExportTestServer()
	query := "?account=export-account&monitorKeyword=" + url.QueryEscape("导出") + "&sort=likes&direction=asc&page=2&pageSize=20"
	rows := readVideoExportCSV(t, videoExportRequest(t, srv, query, `{"scope":"filtered"}`))
	if len(rows) != 55 || rows[0]["视频ID"] != "export-00" || rows[54]["视频ID"] != "export-54" {
		t.Fatalf("export must ignore pagination and retain filter/sort: count=%d first=%v last=%v", len(rows), rows[0], rows[len(rows)-1])
	}
	if rows[0]["点赞"] != "0" || rows[0]["标题"] != "中文视频 00" || rows[0]["采集时间（北京时间）"] != "2026-10-30 12:00:00" {
		t.Fatalf("unexpected metrics or timestamp: %v", rows[0])
	}

	// A threshold/date/source combination must give exactly the same rows as the list API.
	query += "&minLikes=50&source=wx_video_search&publishedFrom=2026-10-28&publishedTo=2026-10-28"
	rows = readVideoExportCSV(t, videoExportRequest(t, srv, query, `{"scope":"filtered"}`))
	if len(rows) != 5 || rows[0]["视频ID"] != "export-50" {
		t.Fatalf("export ignored advanced filters: %v", rows)
	}
}

func TestVideoExportSelectedIsScopedAndOrdered(t *testing.T) {
	srv := newVideoExportTestServer()
	query := "?account=export-account&sort=likes&direction=desc"
	rows := readVideoExportCSV(t, videoExportRequest(t, srv, query, `{"scope":"selected","ids":["video:export-01","video:export-54","video:export-01"]}`))
	if len(rows) != 2 || rows[0]["视频ID"] != "export-54" || rows[1]["视频ID"] != "export-01" {
		t.Fatalf("selected export must deduplicate and keep display order: %v", rows)
	}
	for _, id := range []string{"video:other", "video:missing"} {
		rec := videoExportRequest(t, srv, query, `{"scope":"selected","ids":["video:export-01","`+id+`"]}`)
		if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Disposition") != "" {
			t.Fatalf("out-of-scope selection must fail before any download: status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestVideoExportValidatesScopeAndKeepsEmptyResultsEmpty(t *testing.T) {
	srv := newVideoExportTestServer()
	for _, body := range []string{
		`{}`, `{"scope":"unknown"}`, `{"scope":"selected"}`, `{"scope":"selected","ids":[""]}`,
		`{"scope":"filtered","ids":["video:export-01"]}`, `{"scope":"filtered","extra":true}`, `{"scope":"filtered"} {}`,
	} {
		rec := videoExportRequest(t, srv, "", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid request %s returned %d: %s", body, rec.Code, rec.Body.String())
		}
	}
	ids, _ := json.Marshal(make([]string, 51))
	if rec := videoExportRequest(t, srv, "", `{"scope":"selected","ids":`+string(ids)+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized selection returned %d", rec.Code)
	}
	rows := readVideoExportCSV(t, videoExportRequest(t, srv, "?q=no-matching-video", `{"scope":"filtered"}`))
	if len(rows) != 0 {
		t.Fatalf("empty filter exported unrelated videos: %v", rows)
	}
}

func TestVideoExportEscapesCellsAndPreservesMissingMetrics(t *testing.T) {
	srv := newDataCenterTestServer()
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{DataProvider: apiDataCenterProvider{
		observations: []ingest.VideoMetricObservation{{
			ID: "unsafe", ObjectID: "12345678901234567890", Title: " =HYPERLINK(\"https://example.invalid\",\"内容\")\n下一行",
			ChannelName: "@不执行公式", ObservedAt: time.Now(), MissingMetrics: ingest.MetricPlay | ingest.InteractionMetrics,
		}},
	}})
	rows := readVideoExportCSV(t, videoExportRequest(t, srv, "", `{"scope":"filtered"}`))
	if len(rows) != 1 || rows[0]["点赞"] != "" || rows[0]["总互动"] != "" || rows[0]["每小时新增互动"] != "" {
		t.Fatalf("missing evidence must stay blank: %v", rows)
	}
	if rows[0]["视频ID"] != "'12345678901234567890" || !strings.HasPrefix(rows[0]["标题"], "' =HYPERLINK") || rows[0]["账号名称"] != "'@不执行公式" || !strings.Contains(rows[0]["标题"], "\n下一行") {
		t.Fatalf("unsafe or lossy spreadsheet cells: %v", rows[0])
	}
}

type scopedVideoExportProvider struct{ apiDataCenterProvider }

func (scopedVideoExportProvider) ListVideoMetricObservations(_ context.Context, query ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	return []ingest.VideoMetricObservation{{ID: "owner-row", ObjectID: query.HubUserID, Title: query.HubUserID, ObservedAt: time.Now()}}, nil
}

func TestVideoExportUsesTrustedOwnerAndRejectsForgedSelection(t *testing.T) {
	srv := newDataCenterTestServer()
	srv.bridgeConfig = runtime.BridgeConfig{TrustedToken: "export-test-token", RequireTrustedForLive: true}
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{DataProvider: scopedVideoExportProvider{}})
	request := func(body, token string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, dataCenterRoutePrefix+"videos/export?hub_user_id=bob", strings.NewReader(body))
		req.Header.Set(headerHubUserID, "alice")
		req.Header.Set(headerBridgeToken, token)
		srv.routes().ServeHTTP(rec, req)
		return rec
	}
	if rec := request(`{"scope":"filtered"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("export bypassed trusted context: %d %s", rec.Code, rec.Body.String())
	}
	rows := readVideoExportCSV(t, request(`{"scope":"filtered"}`, "export-test-token"))
	if len(rows) != 1 || rows[0]["视频ID"] != "alice" {
		t.Fatalf("query spoofing changed export owner: %v", rows)
	}
	if rec := request(`{"scope":"selected","ids":["video:bob"]}`, "export-test-token"); rec.Code != http.StatusNotFound {
		t.Fatalf("export leaked another owner: %d %s", rec.Code, rec.Body.String())
	}
}

func TestVideoExportHonorsCurationLibraryAndPreservesNotes(t *testing.T) {
	store, err := insightfile.NewVideoCurationRepository("")
	if err != nil {
		t.Fatal(err)
	}
	srv := newVideoExportTestServer()
	// Use the existing curation API so the export sees the same saved decisions.
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{
		CurationRepository: store,
		DataProvider: apiDataCenterProvider{observations: []ingest.VideoMetricObservation{
			{ID: "one", ObjectID: "one", Title: "待研究内容", ObservedAt: time.Now()},
			{ID: "two", ObjectID: "two", Title: "其他内容", ObservedAt: time.Now()},
		}},
	})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, dataCenterRoutePrefix+"videos/curation", strings.NewReader(`{"ids":["video:one"],"status":"selected","tags":["开场参考"],"note":"=1+1\n仅作为备注"}`)))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rows := readVideoExportCSV(t, videoExportRequest(t, srv, "?curation=selected&tag="+url.QueryEscape("开场参考"), `{"scope":"filtered"}`))
	if len(rows) != 1 || rows[0]["视频ID"] != "one" || rows[0]["运营标记"] != "已精选" || rows[0]["标签"] != "开场参考" || rows[0]["备注"] != "'=1+1\n仅作为备注" {
		t.Fatalf("curation export lost scope, tags, or safe notes: %v", rows)
	}
}
