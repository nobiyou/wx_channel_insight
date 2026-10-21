package ingest_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestfile"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestVideoKeywordProvenanceSurvivesSameTimeUpsertsRestartAndWorkspaceIsolation(t *testing.T) {
	ctx := context.Background()
	observedAt := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "events.json")
	repo, err := ingestfile.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	service := ingest.NewService(repo, ingest.ServiceConfig{Now: func() time.Time { return observedAt }})
	observations := []ingest.VideoMetricObservation{{ID: "same@time@keyword=AI", HubUserID: "alice", Source: ingest.RunSourceWXVideoSearch, ObjectID: "shared", Keyword: "AI", ObservedAt: observedAt, LikeCount: 10}, {ID: "same@time@keyword=ai", HubUserID: "alice", Source: ingest.RunSourceWXVideoSearch, ObjectID: "shared", Keyword: "ai", ObservedAt: observedAt, LikeCount: 10}}
	if err := repo.AppendVideoMetricObservations(ctx, "alice", observations); err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendVideoMetricObservations(ctx, "alice", observations); err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendVideoMetricObservations(ctx, "bob", []ingest.VideoMetricObservation{{ID: "same@time@keyword=secret", HubUserID: "bob", Source: ingest.RunSourceWXVideoSearch, ObjectID: "shared", Keyword: "secret", ObservedAt: observedAt, LikeCount: 999}}); err != nil {
		t.Fatal(err)
	}
	restartedRepo, err := ingestfile.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	service = ingest.NewService(restartedRepo, ingest.ServiceConfig{Now: func() time.Time { return observedAt }})
	observations, err = service.ListVideoMetricObservations(ctx, ingest.SummaryQuery{HubUserID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 2 {
		t.Fatalf("same time must preserve two exact keywords and deduplicate repeat: %+v", observations)
	}
	dataCenter := insight.NewDataCenterService(insight.DataCenterServiceConfig{DataProvider: service})
	for _, keyword := range []string{"AI", "ai"} {
		page, err := dataCenter.Query(ctx, insight.DataCenterQuery{HubUserID: "alice", Entity: insight.DataCenterEntityVideos, MonitorKeyword: keyword})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 1 || !reflect.DeepEqual(page.Items[0].SourceKeywords, []string{"AI", "ai"}) || *page.Items[0].Likes != 10 || page.Items[0].GrowthStatus != "insufficient_history" {
			t.Fatalf("persisted workspace facts: %+v", page)
		}
	}
	page, err := dataCenter.Query(ctx, insight.DataCenterQuery{HubUserID: "alice", Entity: insight.DataCenterEntityVideos, MonitorKeyword: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || page.Summary.Videos != 0 {
		t.Fatalf("other workspace provenance leaked: %+v", page)
	}
}
