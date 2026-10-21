package insightfile

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestDataCenterExclusionRepositoryPersistsAcrossOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data-center-exclusions.json")
	item := insight.DataCenterExclusion{WorkspaceID: "ws-1", Entity: insight.DataCenterEntityTopics, EntityID: "topic:alpha", DeletedAt: time.Now().UTC(), DeletedBy: "operator"}
	repo, err := NewDataCenterExclusionRepository(path)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	if _, err := repo.Upsert(context.Background(), item); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	reloaded, err := NewDataCenterExclusionRepository(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	items, err := reloaded.List(context.Background(), "ws-1")
	if err != nil || len(items) != 1 || items[0].EntityID != item.EntityID {
		t.Fatalf("expected persisted exclusion, items=%+v err=%v", items, err)
	}
}
