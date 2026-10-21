package insightfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type persistedDataCenterExclusions struct {
	Items []insight.DataCenterExclusion `json:"items"`
}

type DataCenterExclusionRepository struct {
	mu    sync.RWMutex
	path  string
	items map[string]insight.DataCenterExclusion
}

var _ insight.DataCenterExclusionRepository = (*DataCenterExclusionRepository)(nil)

func NewDataCenterExclusionRepository(path string) (*DataCenterExclusionRepository, error) {
	if path == "" {
		return nil, errors.New("data center exclusion repository path is required")
	}
	repo := &DataCenterExclusionRepository{path: path, items: map[string]insight.DataCenterExclusion{}}
	if err := repo.loadOrSeed(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *DataCenterExclusionRepository) List(_ context.Context, workspaceID string) ([]insight.DataCenterExclusion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadOrSeedLocked(); err != nil {
		return nil, err
	}
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
	if err := r.loadOrSeedLocked(); err != nil {
		return insight.DataCenterExclusion{}, err
	}
	key := exclusionKey(exclusion)
	if existing, ok := r.items[key]; ok {
		return existing, nil
	}
	r.items[key] = exclusion
	if err := r.persistLocked(); err != nil {
		return insight.DataCenterExclusion{}, err
	}
	return exclusion, nil
}

func (r *DataCenterExclusionRepository) Restore(_ context.Context, workspaceID string, entity insight.DataCenterEntity, entityID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadOrSeedLocked(); err != nil {
		return false, err
	}
	key := workspaceID + "\x00" + string(entity) + "\x00" + entityID
	item, ok := r.items[key]
	if !ok || item.WorkspaceID != workspaceID {
		return false, nil
	}
	delete(r.items, key)
	if err := r.persistLocked(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *DataCenterExclusionRepository) loadOrSeed() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadOrSeedLocked()
}

func (r *DataCenterExclusionRepository) loadOrSeedLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r.persistLocked()
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return r.persistLocked()
	}
	var payload persistedDataCenterExclusions
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	r.items = map[string]insight.DataCenterExclusion{}
	for _, item := range payload.Items {
		r.items[exclusionKey(item)] = item
	}
	return nil
}

func (r *DataCenterExclusionRepository) persistLocked() error {
	items := make([]insight.DataCenterExclusion, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return exclusionKey(items[i]) < exclusionKey(items[j]) })
	data, err := json.MarshalIndent(persistedDataCenterExclusions{Items: items}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}

func exclusionKey(item insight.DataCenterExclusion) string {
	return item.WorkspaceID + "\x00" + string(item.Entity) + "\x00" + item.EntityID
}
