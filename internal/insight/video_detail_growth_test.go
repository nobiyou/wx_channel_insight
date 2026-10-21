package insight

import (
	"encoding/json"
	"reflect"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"testing"
	"time"
)

func TestVideoDetailPreservesFullCollectionRelations(t *testing.T) {
	base := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	detail, _ := BuildVideoDetail("v1", []ingest.VideoMetricObservation{
		{ID: "a", ObjectID: "v1", Source: ingest.RunSourceWXVideoSearch, Keyword: "AI", ObservedAt: base, LikeCount: 10},
		{ID: "b", ObjectID: "v1", Source: ingest.RunSourceWXFeedList, Keyword: "机器人", ObservedAt: base.Add(time.Hour), LikeCount: 15},
	}, nil)
	var result struct { SourceKeywords []string `json:"sourceKeywords"`; Sources []string `json:"sources"` }
	raw, _ := json.Marshal(detail); _ = json.Unmarshal(raw, &result)
	if !reflect.DeepEqual(result.SourceKeywords, []string{"AI", "机器人"}) || !reflect.DeepEqual(result.Sources, []string{ingest.RunSourceWXFeedList, ingest.RunSourceWXVideoSearch}) { t.Fatalf("detail must preserve full collection provenance: %s", raw) }
}

func TestVideoDetailDoesNotInventGrowthFromSameTimeOrCounterCorrection(t *testing.T) {
	base := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, status string
		latestAt     time.Time
		likes        int
	}{
		{"equal time", "insufficient_history", base, 15},
		{"rollback", "counter_correction", base.Add(time.Hour), 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			detail, _ := BuildVideoDetail("v1", []ingest.VideoMetricObservation{
				{ID: "a", ObjectID: "v1", ObservedAt: base, LikeCount: 10},
				{ID: "b", ObjectID: "v1", ObservedAt: test.latestAt, LikeCount: test.likes},
			}, nil)
			var result struct {
				GrowthStatus string   `json:"growthStatus"`
				GrowthRate   *float64 `json:"growthRate"`
			}
			raw, _ := json.Marshal(detail)
			_ = json.Unmarshal(raw, &result)
			if result.GrowthStatus != test.status || result.GrowthRate != nil || detail.HasGrowthBaseline || detail.HasComparableInteractions || detail.PositiveDeltas.Total != 0 || len(detail.History) != 2 {
				t.Fatalf("history must be preserved without fabricated growth: %s", raw)
			}
		})
	}
}
