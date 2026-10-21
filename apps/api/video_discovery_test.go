package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
)

func TestVideoDiscoveryAPIExposesRanksAndExplainsEmptyCurrentRanking(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	points := []ingest.VideoMetricObservation{}
	for i, id := range []string{"a", "b", "c", "d", "e"} {
		points = append(points, ingest.VideoMetricObservation{ID: id, ObjectID: id, Source: ingest.RunSourceWXVideoSearch, Keyword: "秋天", ObservedAt: now, PublishedAt: now.Add(-time.Hour), LikeCount: i})
	}
	srv := newDataCenterTestServer()
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{DataProvider: apiDataCenterProvider{observations: points}, Now: func() time.Time { return now }})
	query := func(search string) (int, insightapi.DataCenterPageResponse) {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, dataCenterRoutePrefix+"videos?"+search, nil))
		var page insightapi.DataCenterPageResponse
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
		}
		return rec.Code, page
	}
	code, page := query("sort=recent_heat&recentOnly=true&monitorKeyword=%E7%A7%8B%E5%A4%A9")
	if code != http.StatusOK || page.Total != 4 || page.Items[0].ObjectID != "e" {
		t.Fatalf("incorrect current discovery response: code=%d total=%d", code, page.Total)
	}
	if rank := page.Items[0].Performance.KeywordRanks[0]; rank.Rank == nil || *rank.Rank != 1 || rank.SampleCount != 5 || page.Items[0].Performance.RecentHeat.Score == nil {
		t.Fatalf("rank/score missing from DTO: %+v", page.Items[0].Performance)
	}
	now = now.Add(48 * time.Hour)
	code, page = query("sort=recent_heat&recentOnly=true")
	if code != http.StatusOK || page.Total != 0 || page.Analysis.RecentHot.State != "stale" {
		t.Fatalf("missing stale explanation: code=%d total=%d analysis=%+v", code, page.Total, page.Analysis)
	}
	code, page = query("sort=interactions")
	if code != http.StatusOK || page.Total != 5 || page.Items[0].Performance.KeywordRanks[0].State != "historical" {
		t.Fatal("historical reference disappeared")
	}
	if code, _ := query("recentOnly=unknown"); code != http.StatusBadRequest {
		t.Fatal("invalid boolean accepted")
	}
}
