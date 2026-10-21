package insightdemo

import (
	"context"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestAccountRepositoryIsolatesWorkspaceAndClonesTags(t *testing.T) {
	repo := NewAccountRepository()
	account, err := repo.UpsertObservedProfile(context.Background(), "workspace-a", insight.AccountObservedProfile{ChannelUsername: "finder-a"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	updated, err := repo.UpdateManualProfile(context.Background(), "workspace-a", account.ID, insight.AccountManualProfile{Tags: []string{"重点"}, IsPriority: true})
	if err != nil {
		t.Fatalf("manual update: %v", err)
	}
	updated.Tags[0] = "外部修改"
	stored, found, err := repo.Get(context.Background(), "workspace-a", account.ID)
	if err != nil || !found || stored.Tags[0] != "重点" || !stored.IsPriority {
		t.Fatalf("repository did not clone/isolate state: found=%v err=%v stored=%+v", found, err, stored)
	}
	if _, found, err := repo.Get(context.Background(), "workspace-b", account.ID); err != nil || found {
		t.Fatalf("workspace isolation failed: found=%v err=%v", found, err)
	}
}
