package normalize

import (
	"context"
	"sync"
)

type MemoryRepository struct {
	mu        sync.RWMutex
	snapshots map[string]Snapshot
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		snapshots: map[string]Snapshot{},
	}
}

func (r *MemoryRepository) ReplaceSnapshot(_ context.Context, workspaceID string, snapshot Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.snapshots[workspaceID] = cloneSnapshot(snapshot)
	return nil
}

func (r *MemoryRepository) GetSummary(_ context.Context, workspaceID string) (Summary, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot, ok := r.snapshots[workspaceID]
	if !ok {
		return Summary{}, false, nil
	}
	return snapshot.Summary, true, nil
}

func (r *MemoryRepository) GetSnapshot(_ context.Context, workspaceID string) (Snapshot, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot, ok := r.snapshots[workspaceID]
	if !ok {
		return Snapshot{}, false, nil
	}
	return cloneSnapshot(snapshot), true, nil
}

func (r *MemoryRepository) Reset(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.snapshots, workspaceID)
	return nil
}
