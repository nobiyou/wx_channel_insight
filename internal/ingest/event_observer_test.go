package ingest_test

import (
	"context"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
)

func TestServiceNotifiesEventObserverAfterImport(t *testing.T) {
	var observed []ingest.Event
	service := ingest.NewService(ingestdemo.NewRepository(), ingest.ServiceConfig{
		DefaultHubUserID: "hub-observer",
		EventObserver: func(_ context.Context, query ingest.SummaryQuery, events []ingest.Event) error {
			if query.HubUserID != "hub-observer" {
				t.Fatalf("observer received wrong hub user: %+v", query)
			}
			observed = append(observed, events...)
			return nil
		},
	})
	_, err := service.ImportEvents(context.Background(), ingest.SummaryQuery{}, []ingest.Event{{
		ID: "observer-event", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList,
		ChannelUsername: "finder-observer", ChannelName: "观察账号",
	}})
	if err != nil {
		t.Fatalf("import events: %v", err)
	}
	if len(observed) != 1 || observed[0].ID != "observer-event" || observed[0].HubUserID != "hub-observer" {
		t.Fatalf("unexpected observed events: %+v", observed)
	}
}
