package insight

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func TestVideoFactsPreserveExactCollectionRelationsAndFilteredSummary(t *testing.T) {
	base := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	older := ingest.VideoMetricObservation{ID: "old", HubUserID: "alice", ObjectID: "shared", Source: ingest.RunSourceWXVideoSearch, Keyword: "AI", ObservedAt: base, LikeCount: 10}
	latest := older
	latest.ID, latest.Keyword, latest.Source, latest.ObservedAt, latest.LikeCount = "latest", "机器人", ingest.RunSourceWXFeedList, base.Add(time.Hour), 15
	provider := &latestDataCenterProviderStub{
		dataCenterProviderStub: dataCenterProviderStub{observations: []ingest.VideoMetricObservation{
			older, latest,
			{ID: "fuzzy", ObjectID: "fuzzy", Source: ingest.RunSourceWXVideoSearch, Title: "AI 视频", Keyword: "AI 应用", ObservedAt: base},
		}}, latest: []ingest.VideoMetricObservation{latest},
	}
	service := NewDataCenterService(DataCenterServiceConfig{DataProvider: provider})
	query := DataCenterQuery{HubUserID: "alice", Entity: DataCenterEntityVideos}
	// Decode through the public query contract so a missing filter fails as a
	// behavioral regression rather than a compile error before implementation.
	if err := json.Unmarshal([]byte(`{"MonitorKeyword":"AI"}`), &query); err != nil {
		t.Fatal(err)
	}
	page, err := service.Query(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "video:shared" {
		t.Fatalf("exact collection relation must select shared video only: %+v", page)
	}
	if page.Summary.Videos != 1 || *page.Items[0].Likes != 15 {
		t.Fatalf("filtered deduplicated facts: %+v", page)
	}
	var item struct {
		SourceKeywords []string `json:"sourceKeywords"`
		Sources        []string `json:"sources"`
	}
	raw, _ := json.Marshal(page.Items[0])
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(item.SourceKeywords, []string{"AI", "机器人"}) || !reflect.DeepEqual(item.Sources, []string{ingest.RunSourceWXFeedList, ingest.RunSourceWXVideoSearch}) {
		t.Fatalf("all factual keyword/source relations must survive latest reduction: %s", raw)
	}
	if provider.latestCalls != 0 {
		t.Fatalf("video relations cannot use latest-only evidence: calls=%d", provider.latestCalls)
	}
}

func TestVideoFactsGrowthUsesDistinctObservationsInsideWindow(t *testing.T) {
	base := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name         string
		observations []ingest.VideoMetricObservation
		from, to     time.Time
		status       string
		rate         *float64
	}{
		{name: "first", observations: []ingest.VideoMetricObservation{{ObservedAt: base, LikeCount: 10}}, status: "insufficient_history"},
		{name: "equal timestamps", observations: []ingest.VideoMetricObservation{{ObservedAt: base, LikeCount: 10}, {ObservedAt: base, LikeCount: 15}}, status: "insufficient_history"},
		{name: "rollback", observations: []ingest.VideoMetricObservation{{ObservedAt: base, LikeCount: 10, CommentCount: 5}, {ObservedAt: base.Add(time.Hour), LikeCount: 15, CommentCount: 4}}, status: "counter_correction"},
		{name: "zero baseline", observations: []ingest.VideoMetricObservation{{ObservedAt: base}, {ObservedAt: base.Add(time.Hour), LikeCount: 5}}, status: "zero_baseline"},
		{name: "available", observations: []ingest.VideoMetricObservation{{ObservedAt: base, LikeCount: 10}, {ObservedAt: base.Add(time.Hour), LikeCount: 15}}, status: "available", rate: factsFloat(50)},
		{name: "window excludes baseline", observations: []ingest.VideoMetricObservation{{ObservedAt: base, LikeCount: 10}, {ObservedAt: base.Add(time.Hour), LikeCount: 15}}, from: base.Add(time.Minute), status: "insufficient_history"},
		{name: "historical window latest", observations: []ingest.VideoMetricObservation{{ObservedAt: base, LikeCount: 10}, {ObservedAt: base.Add(time.Hour), LikeCount: 15}, {ObservedAt: base.Add(2 * time.Hour), LikeCount: 20}}, to: base.Add(time.Hour), status: "available", rate: factsFloat(50)},
	} {
		t.Run(test.name, func(t *testing.T) {
			for i := range test.observations {
				test.observations[i].ID = string(rune('a' + i))
				test.observations[i].ObjectID = "v1"
				test.observations[i].Source = ingest.RunSourceWXVideoSearch
			}
			service := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{observations: test.observations}})
			page, err := service.Query(context.Background(), DataCenterQuery{Entity: DataCenterEntityVideos, From: test.from, To: test.to})
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != 1 {
				t.Fatalf("window must retain the latest real in-window snapshot: %+v", page)
			}
			var item struct {
				GrowthRate   *float64 `json:"growthRate"`
				GrowthStatus string   `json:"growthStatus"`
				GrowthBasis  string   `json:"growthBasis"`
				Views        *int     `json:"views"`
			}
			raw, _ := json.Marshal(page.Items[0])
			_ = json.Unmarshal(raw, &item)
			if item.GrowthStatus != test.status || !reflect.DeepEqual(item.GrowthRate, test.rate) || item.GrowthBasis == "" || item.Views != nil {
				t.Fatalf("qualified growth and missing views: %s", raw)
			}
		})
	}
}

func factsFloat(value float64) *float64 { return &value }

func TestVideoFactsAbsoluteGrowthDoesNotRequireNonzeroBaseline(t *testing.T) {
	base := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name             string
		baseline, latest int
		span             time.Duration
		missing          ingest.MetricMask
		status           string
		speed            *float64
	}{
		{name: "zero to positive", latest: 20, span: 2 * time.Hour, status: "zero_baseline", speed: factsFloat(10)},
		{name: "zero to zero is a real zero", span: time.Hour, status: "zero_baseline", speed: factsFloat(0)},
		{name: "interval too short", latest: 20, span: 9 * time.Minute, status: "zero_baseline"},
		{name: "interval too long", latest: 20, span: 73 * time.Hour, status: "zero_baseline"},
		{name: "missing field is not a zero", latest: 20, span: time.Hour, missing: ingest.MetricLike, status: "metrics_unavailable"},
		{name: "correction is not growth", baseline: 20, latest: 10, span: time.Hour, status: "counter_correction"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			at := base.Add(tt.span)
			observations := []ingest.VideoMetricObservation{
				{ID: "a", ObjectID: "new-video", Source: ingest.RunSourceWXVideoSearch, ObservedAt: base, LikeCount: tt.baseline, MissingMetrics: tt.missing},
				{ID: "b", ObjectID: "new-video", Source: ingest.RunSourceWXVideoSearch, ObservedAt: at, LikeCount: tt.latest},
			}
			service := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{observations: observations}, Now: func() time.Time { return at }})
			query := DataCenterQuery{Entity: DataCenterEntityVideos, Sort: "growth_per_hour"}
			page, err := service.Query(context.Background(), query)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) != 1 {
				t.Fatalf("missing factual video: %+v", page)
			}
			row := page.Items[0]
			if row.GrowthStatus != tt.status || row.GrowthRate != nil || !reflect.DeepEqual(row.Performance.GrowthPerHour, tt.speed) {
				t.Fatalf("relative and absolute growth conflated: status=%s rate=%v speed=%v want=%v", row.GrowthStatus, row.GrowthRate, row.Performance.GrowthPerHour, tt.speed)
			}
			wantRising := 0
			if tt.speed != nil && *tt.speed > 0 {
				wantRising = 1
			}
			if page.Analysis.RisingVideos != wantRising {
				t.Fatalf("rising count=%d want=%d", page.Analysis.RisingVideos, wantRising)
			}
			query.MomentumOnly = true
			page, err = service.Query(context.Background(), query)
			if err != nil || page.Total != wantRising {
				t.Fatalf("rising filter excluded valid zero-baseline growth: %+v err=%v", page, err)
			}
		})
	}
}
