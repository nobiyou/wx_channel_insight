package insight

import (
	"context"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"testing"
	"time"
)

func TestVideoFactsDoNotCompareUnavailableMetricsAsZero(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	observations := []ingest.VideoMetricObservation{{ID: "old", ObjectID: "v", ObservedAt: start, LikeCount: 2}, {ID: "new", ObjectID: "v", ObservedAt: start.Add(time.Hour), LikeCount: 10, MissingMetrics: ingest.MetricForward}}
	detail, ok := BuildVideoDetail("v", observations, nil)
	if !ok || detail.GrowthRate != nil || detail.GrowthStatus != "metrics_unavailable" {
		t.Fatalf("incomplete growth: %+v", detail)
	}
	row := itemFromVideoObservation(observations[1])
	if row.Forwards != nil || row.Likes == nil || *row.Likes != 10 {
		t.Fatalf("wrong availability: %+v", row)
	}
	history := []DataCenterHistoryItem{}
	for _, obs := range observations {
		item := itemFromVideoObservation(obs)
		history = append(history, DataCenterHistoryItem{ID: obs.ID, ObservedAt: obs.ObservedAt, EvidenceType: "metric_observation", Likes: item.Likes, Comments: item.Comments, Favorites: item.Favorites, Forwards: item.Forwards})
	}
	window, found := videoFactsInWindow(row, history, time.Time{}, time.Time{})
	if !found || window.GrowthStatus != "metrics_unavailable" || window.GrowthRate != nil {
		t.Fatalf("window lost missing fields: %+v", window)
	}
}

func TestAccountTotalsKeepLatestMissingMetricUnavailable(t *testing.T) {
	start := time.Now().UTC()
	for _, reverse := range []bool{false, true} {
		observations := []ingest.VideoMetricObservation{
			{ID: "old", ObjectID: "v", ChannelUsername: "account", ObservedAt: start, ForwardCount: 9},
			{ID: "new", ObjectID: "v", ChannelUsername: "account", ObservedAt: start.Add(time.Hour), LikeCount: 3, MissingMetrics: ingest.MetricForward},
		}
		if reverse {
			observations[0], observations[1] = observations[1], observations[0]
		}
		service := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{observations: observations}})
		rows, err := service.ListAccountRows(context.Background(), DataCenterQuery{HubUserID: "owner", Entity: DataCenterEntityAccounts})
		if err != nil || len(rows) != 1 || rows[0].Forwards != nil || rows[0].Likes == nil || *rows[0].Likes != 3 {
			t.Fatalf("account inherited stale metric: rows=%+v err=%v", rows, err)
		}
	}
}
