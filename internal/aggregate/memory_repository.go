package aggregate

import (
	"context"
	"sync"
)

type MemoryRepository struct {
	mu         sync.RWMutex
	aggregates map[string]Aggregate
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		aggregates: map[string]Aggregate{},
	}
}

func (r *MemoryRepository) ReplaceAggregate(_ context.Context, workspaceID string, aggregate Aggregate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.aggregates[workspaceID] = cloneAggregate(aggregate)
	return nil
}

func (r *MemoryRepository) GetSummary(_ context.Context, workspaceID string) (Summary, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	aggregate, ok := r.aggregates[workspaceID]
	if !ok {
		return Summary{}, false, nil
	}
	return aggregate.Summary, true, nil
}

func (r *MemoryRepository) GetAggregate(_ context.Context, workspaceID string) (Aggregate, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	aggregate, ok := r.aggregates[workspaceID]
	if !ok {
		return Aggregate{}, false, nil
	}
	return cloneAggregate(aggregate), true, nil
}

func (r *MemoryRepository) Reset(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.aggregates, workspaceID)
	return nil
}
