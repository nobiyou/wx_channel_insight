package runtime

import (
	"context"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func syncObservedAccounts(ctx context.Context, service *insight.AccountService, query ingest.SummaryQuery, events []ingest.Event) error {
	if service == nil {
		return nil
	}
	profiles := make([]insight.AccountObservedProfile, 0, len(events))
	for _, event := range events {
		if strings.TrimSpace(event.ChannelUsername) == "" {
			continue
		}
		profiles = append(profiles, insight.AccountObservedProfile{
			Platform:        insight.AccountPlatformWXChannels,
			ChannelUsername: event.ChannelUsername,
			ChannelName:     event.ChannelName,
			ObservedAt:      event.CollectedAt,
		})
	}
	_, err := service.SyncObservedProfiles(ctx, insight.AccountQuery{
		HubUserID:     query.HubUserID,
		DeviceCount:   query.DeviceCount,
		ContextSource: "runtime_account_projection",
	}, profiles)
	return err
}
