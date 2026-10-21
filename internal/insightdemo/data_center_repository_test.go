package insightdemo

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestDataCenterExclusionRepositoryUpsertListRestore(t *testing.T) {
	repo := NewDataCenterExclusionRepository()
	item := insight.DataCenterExclusion{WorkspaceID: "ws-1", Entity: insight.DataCenterEntityVideos, EntityID: "video:v1", DeletedAt: time.Now().UTC(), DeletedBy: "operator"}
	if _, err := repo.Upsert(context.Background(), item); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := repo.Upsert(context.Background(), item); err != nil {
		t.Fatalf("idempotent upsert: %v", err)
	}
	items, err := repo.List(context.Background(), "ws-1")
	if err != nil || len(items) != 1 {
		t.Fatalf("expected one exclusion, items=%+v err=%v", items, err)
	}
	found, err := repo.Restore(context.Background(), "ws-1", insight.DataCenterEntityVideos, "video:v1")
	if err != nil || !found {
		t.Fatalf("restore failed: found=%v err=%v", found, err)
	}
}
