package insight

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func TestOperationsAnalysisDistinguishesUnavailableAssessmentsFromZero(t *testing.T) {
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	makeRows := func(stale, history bool) []DataCenterItem {
		observed := now
		if stale {
			observed = now.Add(-25 * time.Hour)
		}
		rows := make([]DataCenterItem, 5)
		for i := range rows {
			rows[i] = DataCenterItem{ID: fmt.Sprint(i), ChannelUsername: "a", SourceKeywords: []string{"秋天"}, PublishedAt: observed.Add(-4 * 24 * time.Hour), ObservedAt: observed, Likes: intPtr(100), Comments: intPtr(0), Favorites: intPtr(0), Forwards: intPtr(0)}
			if history {
				rows[i].GrowthStatus = "available"
				rows[i].growthInteractions = intPtr(0)
				rows[i].GrowthWindow.BaselineObservedAt = observed.Add(-time.Hour)
				rows[i].GrowthWindow.LatestObservedAt = observed
			}
		}
		return rows
	}
	missing := makeRows(false, false)
	zeroMedian := makeRows(false, true)
	young := makeRows(false, true)
	for i := range missing {
		missing[i].Comments = nil
		zeroMedian[i].Likes = intPtr(0)
		young[i].PublishedAt = now.Add(-24 * time.Hour)
	}
	tests := []struct {
		name     string
		rows     []DataCenterItem
		states   [3]string // growth, standout, review
		eligible [3]int
		standout int
	}{
		{name: "no samples", states: [3]string{"empty", "empty", "empty"}},
		{name: "one fresh sample cannot establish peers or growth", rows: makeRows(false, false)[:1], states: [3]string{"insufficient_data", "insufficient_data", "insufficient_data"}},
		{name: "peer comparison needs no growth history", rows: makeRows(false, false), states: [3]string{"insufficient_data", "available", "insufficient_data"}, eligible: [3]int{0, 5, 0}},
		{name: "qualified observations with genuinely zero growth", rows: makeRows(false, true), states: [3]string{"available", "available", "available"}, eligible: [3]int{5, 5, 5}},
		{name: "stale peers retain historical standout only", rows: makeRows(true, true), states: [3]string{"stale", "historical", "stale"}, eligible: [3]int{0, 5, 0}, standout: 1},
		{name: "missing metrics are not zero performance", rows: missing, states: [3]string{"insufficient_data", "insufficient_data", "insufficient_data"}},
		{name: "zero median cannot establish relative performance", rows: zeroMedian, states: [3]string{"available", "insufficient_data", "insufficient_data"}, eligible: [3]int{5, 0, 0}},
		{name: "young videos cannot be reviewed as underperforming", rows: young, states: [3]string{"available", "available", "insufficient_data"}, eligible: [3]int{5, 5, 0}},
	}
	tests[4].rows[4].Likes = intPtr(1000)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessVideoPerformance(tt.rows, now)
			for _, grouping := range []string{"account", "keyword"} {
				result := buildVideoOperationsAnalysis(tt.rows, nil, DataCenterQuery{Entity: DataCenterEntityVideos, CompareBy: grouping})
				// Check the public contract, including identical state semantics for
				// account/keyword groups and the full filtered result.
				var body struct {
					Readiness map[string]struct {
						State          string `json:"state"`
						EligibleVideos int    `json:"eligibleVideos"`
						Reason         string `json:"reason"`
					} `json:"readiness"`
					Groups []struct {
						Readiness json.RawMessage `json:"readiness"`
					} `json:"groups"`
				}
				encoded, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal(encoded, &body); err != nil {
					t.Fatal(err)
				}
				for i, key := range []string{"growth", "standout", "review"} {
					status := body.Readiness[key]
					if status.State != tt.states[i] || status.EligibleVideos != tt.eligible[i] || status.Reason == "" {
						t.Fatalf("%s %s assessment = %+v, want state %s, eligible %d", grouping, key, status, tt.states[i], tt.eligible[i])
					}
				}
				if result.RisingVideos != 0 || result.UnderperformingVideos != 0 || result.StandoutVideos != tt.standout {
					t.Fatalf("availability must not invent matches: %+v", result)
				}
				encodedReadiness, _ := json.Marshal(body.Readiness)
				var want any
				if err := json.Unmarshal(encodedReadiness, &want); err != nil {
					t.Fatal(err)
				}
				for _, group := range body.Groups {
					var got any
					if err := json.Unmarshal(group.Readiness, &got); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("group status differs: %+v vs %+v", got, want)
					}
				}
			}
		})
	}
}

func TestOperationsAnalysisReadinessRespectsEachSourceAndPartialFreshness(t *testing.T) {
	zero, one, two := 0.0, 1.0, 2.0
	rows := []DataCenterItem{
		{ChannelUsername: "current", SourceKeywords: []string{"新鲜"}, Performance: VideoPerformance{Freshness: "fresh", RelativeToMedian: &one, GrowthPerHour: &zero}},
		{ChannelUsername: "historical", SourceKeywords: []string{"历史"}, Performance: VideoPerformance{Freshness: "stale", RelativeToMedian: &two, Tier: "standout"}},
	}
	for _, grouping := range []string{"account", "keyword"} {
		result := buildVideoOperationsAnalysis(rows, nil, DataCenterQuery{Entity: DataCenterEntityVideos, CompareBy: grouping})
		if result.Readiness.Growth.State != "available" || result.Readiness.Growth.EligibleVideos != 1 || result.Readiness.Standout.State != "historical" || result.Readiness.Standout.EligibleVideos != 2 {
			t.Fatalf("partial freshness concealed: %+v", result.Readiness)
		}
		for _, group := range result.Groups {
			expected := "available"
			if group.Key == "historical" || group.Key == "历史" {
				expected = "stale"
			}
			if group.Readiness.Growth.State != expected {
				t.Fatalf("%s group inherited global availability: %+v", group.Key, group.Readiness)
			}
		}
	}
}

func TestOperationsAnalysisCountsFirstDiscoveryWithinSharedWindow(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows := []DataCenterItem{{ID: "old", ChannelUsername: "a", ChannelName: "A", SourceKeywords: []string{"秋天", "旅行"}, Likes: intPtr(10), Comments: intPtr(0), Favorites: intPtr(0), Forwards: intPtr(0), GrowthStatus: "available"}, {ID: "new", ChannelUsername: "b", ChannelName: "B", SourceKeywords: []string{"秋天"}, Likes: intPtr(0), Comments: nil, Favorites: intPtr(0), Forwards: intPtr(0)}}
	histories := map[string][]time.Time{"old": {start.Add(-time.Hour), start.Add(time.Hour)}, "new": {start.Add(time.Hour)}}
	query := DataCenterQuery{Entity: DataCenterEntityVideos, From: start, To: start.Add(24 * time.Hour), CompareBy: "account"}
	result := buildVideoOperationsAnalysis(rows, histories, query)
	if result.NewVideos == nil || *result.NewVideos != 1 || result.ComparableVideos != 1 || len(result.Groups) != 2 || result.Groups[0].Name != "A" || *result.Groups[0].Interactions != 10 || result.Groups[1].Interactions != nil {
		t.Fatalf("incorrect comparison: %+v", result)
	}
	query.CompareBy = "keyword"
	result = buildVideoOperationsAnalysis(rows, histories, query)
	if len(result.Groups) != 2 {
		t.Fatalf("keyword relationships lost: %+v", result)
	}
	query.From = time.Time{}
	query.To = time.Time{}
	result = buildVideoOperationsAnalysis(rows, histories, query)
	if result.NewVideos != nil {
		t.Fatal("unbounded range must not invent a period")
	}
}

func TestOperationsAnalysisUsesPublicationFilterBeforePagination(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	events := []ingest.Event{}
	for i := 0; i < 25; i++ {
		events = append(events, ingest.Event{ID: fmt.Sprint(i), ObjectID: fmt.Sprint(i), EventType: "video_snapshot", ChannelUsername: "a", Source: ingest.RunSourceWXFeedList, CollectedAt: start.Add(time.Hour), PublishedAt: start, LikeCount: 1})
	}
	events = append(events, ingest.Event{ID: "outside", ObjectID: "outside", EventType: "video_snapshot", CollectedAt: start.Add(time.Hour), PublishedAt: start.Add(-time.Hour), LikeCount: 99})
	service := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{events: events}})
	query := DataCenterQuery{Entity: DataCenterEntityVideos, From: start, To: start.Add(24 * time.Hour), PublishedFrom: start, PublishedTo: start, HubUserID: "owner", PageSize: 20}
	page, err := service.Query(context.Background(), query)
	if err != nil || page.Total != 25 || len(page.Items) != 20 || page.Analysis == nil || page.Analysis.ObservedVideos != 25 || *page.Analysis.NewVideos != 25 || *page.Analysis.Groups[0].Interactions != 25 {
		t.Fatalf("analysis omitted records or ignored publication filter: %+v err=%v", page, err)
	}
	query.PublishedTo = start.Add(-time.Second)
	if _, err := service.Query(context.Background(), query); err == nil {
		t.Fatal("reversed publication dates accepted")
	}
	query.PublishedTo = start
	query.CompareBy = "unsupported"
	if _, err := service.Query(context.Background(), query); err == nil {
		t.Fatal("unsupported grouping accepted")
	}
}
