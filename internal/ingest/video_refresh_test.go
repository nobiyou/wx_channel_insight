package ingest_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
)

func TestVideoMetricRefreshPersistsRealZeroAndMissingFieldsSeparately(t *testing.T) {
	id := "video"
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":{"object":{"id":%q,"objectDesc":{"description":"actual profile"},"contact":{"username":"account@finder"},"likeCount":0,"commentCount":7}}}`, id)
	}))
	defer collector.Close()
	repo := ingestdemo.NewRepository()
	now := time.Now()
	svc := ingest.NewService(repo, ingest.ServiceConfig{DefaultHubUserID: "owner", Now: func() time.Time { return now }, WXFeedListClient: ingest.WXFeedListClient{BaseURL: collector.URL, Client: collector.Client()}})
	query := ingest.SummaryQuery{HubUserID: "owner"}
	if _, err := svc.RefreshWXVideoMetrics(context.Background(), query, "video", "nonce"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if _, err := svc.RefreshWXVideoMetrics(context.Background(), query, "video", "nonce"); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListVideoMetricObservations(context.Background(), "owner")
	if err != nil || len(items) != 2 {
		t.Fatalf("expected two actual observations: items=%v err=%v", items, err)
	}
	for _, item := range items {
		if item.Source != ingest.RunSourceWXVideoProfile || item.LikeCount != 0 || item.CommentCount != 7 || item.MissingMetrics&ingest.MetricLike != 0 || item.MissingMetrics&ingest.MetricFavorite == 0 {
			t.Fatalf("availability was lost: %+v", item)
		}
	}
	id = "another-video"
	if _, err := svc.RefreshWXVideoMetrics(context.Background(), query, "video", "nonce"); err == nil {
		t.Fatal("mismatched video must be rejected")
	}
	items, _ = repo.ListVideoMetricObservations(context.Background(), "owner")
	if len(items) != 2 {
		t.Fatal("bad upstream response changed history")
	}
}
