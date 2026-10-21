package ingest

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSourceMetricAbsenceSurvivesObservationPersistence(t *testing.T) {
	item := parseWXFeedListItem("account", map[string]any{"id": "video", "likeCount": float64(0), "commentCount": "invalid", "favCount": float64(-1)})
	events := BuildEventsFromWXFeedList("owner", "machine", WXFeedListResult{Items: []WXFeedListItem{item}}, time.Now())
	observations := buildVideoMetricObservations(events)
	if len(observations) != 1 {
		t.Fatal("missing observation")
	}
	raw, err := json.Marshal(observations[0])
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip VideoMetricObservation
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if MetricValue(roundTrip.LikeCount, roundTrip.MissingMetrics, MetricLike) == nil {
		t.Fatal("real zero must remain available")
	}
	for _, flag := range []MetricMask{MetricPlay, MetricComment, MetricFavorite, MetricForward} {
		if MetricValue(0, roundTrip.MissingMetrics, flag) != nil {
			t.Fatalf("missing metric %d became zero", flag)
		}
	}
}

func TestSourceMetricZeroIsNotReplacedByNestedFallback(t *testing.T) {
	item := parseWXFeedListItem("account", map[string]any{"id": "v", "likeCount": float64(0), "objectExtend": map[string]any{"monotonicData": map[string]any{"countInfo": map[string]any{"likeCount": float64(10), "commentCount": float64(3)}}}})
	if item.LikeCount != 0 || item.CommentCount != 3 {
		t.Fatalf("wrong precedence: %+v", item)
	}
	if item.MissingMetrics&(MetricLike|MetricComment) != 0 {
		t.Fatal("valid metrics marked missing")
	}
}
