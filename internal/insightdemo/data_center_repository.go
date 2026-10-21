package insightdemo

import (
	"context"
	"sort"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type DataCenterExclusionRepository struct {
	mu    sync.RWMutex
	items map[string]insight.DataCenterExclusion
}

var _ insight.DataCenterExclusionRepository = (*DataCenterExclusionRepository)(nil)

func NewDataCenterExclusionRepository() *DataCenterExclusionRepository {
	return &DataCenterExclusionRepository{items: map[string]insight.DataCenterExclusion{}}
}

func (r *DataCenterExclusionRepository) List(_ context.Context, workspaceID string) ([]insight.DataCenterExclusion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]insight.DataCenterExclusion, 0, len(r.items))
	for _, item := range r.items {
		if item.WorkspaceID == workspaceID {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].DeletedAt.After(items[j].DeletedAt) })
	return items, nil
}

func (r *DataCenterExclusionRepository) Upsert(_ context.Context, exclusion insight.DataCenterExclusion) (insight.DataCenterExclusion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := exclusionKey(exclusion)
	if existing, ok := r.items[key]; ok {
		return existing, nil
	}
	r.items[key] = exclusion
	return exclusion, nil
}

func (r *DataCenterExclusionRepository) Restore(_ context.Context, workspaceID string, entity insight.DataCenterEntity, entityID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceID + "\x00" + string(entity) + "\x00" + entityID
	item, ok := r.items[key]
	if !ok || item.WorkspaceID != workspaceID {
		return false, nil
	}
	delete(r.items, key)
	return true, nil
}

func exclusionKey(item insight.DataCenterExclusion) string {
	return item.WorkspaceID + "\x00" + string(item.Entity) + "\x00" + item.EntityID
}
