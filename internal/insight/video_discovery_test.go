package insight

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func discoveryRow(id string, total int, at time.Time) DataCenterItem {
	return DataCenterItem{ID: id, ObservedAt: at, PublishedAt: at.Add(-time.Hour), SourceKeywords: []string{"秋天"}, Likes: intPtr(total), Comments: intPtr(0), Favorites: intPtr(0), Forwards: intPtr(0)}
}

func discoveryRank(row DataCenterItem, keyword string) VideoKeywordRank {
	for _, rank := range row.Performance.KeywordRanks {
		if rank.Keyword == keyword {
			return rank
		}
	}
	return VideoKeywordRank{}
}

func TestRecentHeatDecaysAndProtectsUnavailableEvidence(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	rows := []DataCenterItem{}
	for _, id := range []string{"new", "older", "stale", "missing", "future", "old", "undated", "zero"} {
		rows = append(rows, discoveryRow(id, 100, now))
	}
	rows[1].PublishedAt = rows[0].PublishedAt.Add(-72 * time.Hour)
	rows[2].ObservedAt = now.Add(-25 * time.Hour)
	rows[2].PublishedAt = rows[2].ObservedAt.Add(-time.Hour)
	rows[3].Comments = nil
	rows[4].PublishedAt = now.Add(time.Minute)
	rows[5].PublishedAt = now.Add(-8 * 24 * time.Hour)
	rows[6].PublishedAt = time.Time{}
	rows[7].Likes = intPtr(0)
	assessVideoPerformance(rows, now)
	if rows[0].Performance.RecentHeat.Score == nil || rows[1].Performance.RecentHeat.Score == nil {
		t.Fatal("qualified recent videos not scored")
	}
	if got := *rows[0].Performance.RecentHeat.Score / *rows[1].Performance.RecentHeat.Score; math.Abs(got-2) > 1e-9 {
		t.Fatalf("three-day half-life not respected: %v", got)
	}
	for _, row := range rows[2:7] {
		if row.Performance.RecentHeat.Score != nil {
			t.Fatalf("unqualified video entered recent heat: %s", row.ID)
		}
	}
	if rows[7].Performance.RecentHeat.Score == nil || *rows[7].Performance.RecentHeat.Score != 0 {
		t.Fatal("observed zero was confused with missing evidence")
	}
	filtered, _ := filterVideoWorkbench(rows, DataCenterQuery{RecentOnly: true})
	if len(filtered) != 2 {
		t.Fatalf("recent hot should contain two qualified positive videos, got %d", len(filtered))
	}
	if rows[2].Performance.RecentHeat.State != "stale" {
		t.Fatal("stale evidence not explained")
	}
}

func TestKeywordRanksRespectTiesMissingMetricsAndHistoricalCohorts(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	rows := []DataCenterItem{}
	for i, total := range []int{0, 10, 10, 50, 100} {
		rows = append(rows, discoveryRow(fmt.Sprint(i), total, now))
	}
	rows[1].SourceKeywords = []string{"秋天", "秋天", "小词"}
	rows[2].ObservedAt = now.Add(-48 * time.Hour)
	missing := discoveryRow("missing", 999, now)
	missing.Forwards = nil
	rows = append(rows, missing)
	assessVideoPerformance(rows, now)
	first := discoveryRank(rows[4], "秋天")
	if first.Rank == nil || *first.Rank != 1 || first.SampleCount != 5 || *first.Percentile != 80 || first.State != "historical" {
		t.Fatalf("incorrect keyword cohort: %+v", first)
	}
	for _, i := range []int{1, 2} {
		rank := discoveryRank(rows[i], "秋天")
		if rank.Rank == nil || *rank.Rank != 3 || *rank.Percentile != 20 {
			t.Fatalf("ties should share rank and strictly-lower percentile: %+v", rank)
		}
	}
	if small := discoveryRank(rows[1], "小词"); small.SampleCount != 1 || small.Rank != nil || small.State != "insufficient_data" {
		t.Fatalf("small sample overclaimed: %+v", small)
	}
	if rank := rows[5].Performance.KeywordRanks[0]; rank.Rank != nil || rank.SampleCount != 5 {
		t.Fatalf("missing counters entered rank: %+v", rank)
	}
	for i := range rows[:5] {
		rows[i].Likes = intPtr(0)
	}
	assessVideoPerformance(rows, now)
	for _, row := range rows[:5] {
		if rank := discoveryRank(row, "秋天"); *rank.Percentile != 0 || *rank.Rank != 1 {
			t.Fatal("all-zero cohort falsely described as outperforming peers")
		}
	}
}

func TestDiscoveryRanksStableAcrossSearchCurationAndPagination(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	points := []ingest.VideoMetricObservation{}
	for i := 0; i < 25; i++ {
		points = append(points, ingest.VideoMetricObservation{ID: fmt.Sprint(i), ObjectID: fmt.Sprint(i), Source: ingest.RunSourceWXVideoSearch, Keyword: "秋天", ObservedAt: now, PublishedAt: now.Add(-2 * time.Hour), LikeCount: i, Title: fmt.Sprintf("content-%02d", i)})
	}
	store := &curationTestStore{items: map[string]VideoCuration{"one": {VideoID: "video:23", Status: "selected"}}}
	svc := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{observations: points}, CurationRepository: store, Now: func() time.Time { return now }})
	q := DataCenterQuery{Entity: DataCenterEntityVideos, MonitorKeyword: "秋天", Sort: "recent_heat", RecentOnly: true}
	p, err := svc.Query(context.Background(), q)
	if err != nil || p.Total != 24 || p.Items[0].ID != "video:24" {
		t.Fatalf("recent discovery query failed: total=%d err=%v", p.Total, err)
	}
	q.Keyword, q.Curation = "content-23", "selected"
	p, err = svc.Query(context.Background(), q)
	if err != nil || p.Total != 1 {
		t.Fatalf("search failed: %d %v", p.Total, err)
	}
	rank := p.Items[0].Performance.KeywordRanks[0]
	if rank.Rank == nil || *rank.Rank != 2 || rank.SampleCount != 25 {
		t.Fatalf("filters changed comparison baseline: %+v", rank)
	}
	q.Keyword, q.Curation, q.Page = "", "all", 2
	p, err = svc.Query(context.Background(), q)
	if err != nil || p.Items[0].Performance.KeywordRanks[0].SampleCount != 25 {
		t.Fatalf("pagination changed baseline: %v", err)
	}
	// Empty current rankings must retain the reason from the matching source scope.
	now = now.Add(48 * time.Hour)
	q.Page = 1
	p, err = svc.Query(context.Background(), q)
	if err != nil || p.Total != 0 || p.Analysis.RecentHot.State != "stale" {
		t.Fatalf("stale ranking lost explanation: total=%d analysis=%+v err=%v", p.Total, p.Analysis, err)
	}
}
