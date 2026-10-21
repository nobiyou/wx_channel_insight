package runtime

import (
	"context"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
)

func TestSyncObservedAccountsPromotesIngestIdentities(t *testing.T) {
	service := insight.NewAccountService(insight.AccountServiceConfig{Repository: insightdemo.NewAccountRepository()})
	events := []ingest.Event{
		{ChannelUsername: "finder-a", ChannelName: "账号 A"},
		{ChannelUsername: "finder-b", ChannelName: "账号 B"},
		{ChannelUsername: "finder-a", ChannelName: "账号 A 最新"},
	}
	if err := syncObservedAccounts(context.Background(), service, ingest.SummaryQuery{HubUserID: "hub-runtime"}, events); err != nil {
		t.Fatalf("sync observed accounts: %v", err)
	}
	page, err := service.List(context.Background(), insight.AccountQuery{HubUserID: "hub-runtime"})
	if err != nil {
		t.Fatalf("list synced accounts: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("expected two account masters, got %+v", page)
	}
}
