package aggregatefile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
)

type Repository struct {
	mu         sync.RWMutex
	path       string
	aggregates map[string]aggregate.Aggregate
}

type fileState struct {
	Aggregates map[string]aggregate.Aggregate `json:"aggregates"`
}

var _ aggregate.Repository = (*Repository)(nil)

func NewRepository(path string) (*Repository, error) {
	if path == "" {
		return nil, errors.New("aggregate repository path is required")
	}
	repo := &Repository{
		path:       path,
		aggregates: map[string]aggregate.Aggregate{},
	}
	if err := repo.load(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *Repository) ReplaceAggregate(_ context.Context, workspaceID string, input aggregate.Aggregate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.aggregates[workspaceID] = input
	return r.persist()
}

func (r *Repository) GetSummary(_ context.Context, workspaceID string) (aggregate.Summary, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return aggregate.Summary{}, false, err
	}
	item, ok := r.aggregates[workspaceID]
	if !ok {
		return aggregate.Summary{}, false, nil
	}
	return item.Summary, true, nil
}

func (r *Repository) GetAggregate(_ context.Context, workspaceID string) (aggregate.Aggregate, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return aggregate.Aggregate{}, false, err
	}
	item, ok := r.aggregates[workspaceID]
	if !ok {
		return aggregate.Aggregate{}, false, nil
	}
	return item, true, nil
}

func (r *Repository) Reset(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	delete(r.aggregates, workspaceID)
	return r.persist()
}

func (r *Repository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *Repository) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var state fileState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Aggregates != nil {
		r.aggregates = state.Aggregates
	} else {
		r.aggregates = map[string]aggregate.Aggregate{}
	}
	return nil
}

func (r *Repository) persist() error {
	data, err := json.MarshalIndent(fileState{Aggregates: r.aggregates}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}
