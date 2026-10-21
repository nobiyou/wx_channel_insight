package insightfile

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestAccountRepositoryPersistsManualProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	repo, err := NewAccountRepository(path)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	account, err := repo.UpsertObservedProfile(context.Background(), "workspace-a", insight.AccountObservedProfile{ChannelUsername: "finder-a", ChannelName: "昵称"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := repo.UpdateManualProfile(context.Background(), "workspace-a", account.ID, insight.AccountManualProfile{Region: "华北", Tags: []string{"重点"}}); err != nil {
		t.Fatalf("update: %v", err)
	}
	reloaded, err := NewAccountRepository(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	stored, found, err := reloaded.Get(context.Background(), "workspace-a", account.ID)
	if err != nil || !found || stored.Region != "华北" || len(stored.Tags) != 1 {
		t.Fatalf("persisted account mismatch: found=%v err=%v stored=%+v", found, err, stored)
	}
}
