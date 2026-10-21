package normalizefile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/normalize"
)

type Repository struct {
	mu        sync.RWMutex
	path      string
	snapshots map[string]normalize.Snapshot
}

type fileState struct {
	Snapshots map[string]normalize.Snapshot `json:"snapshots"`
}

var _ normalize.Repository = (*Repository)(nil)

func NewRepository(path string) (*Repository, error) {
	if path == "" {
		return nil, errors.New("normalize repository path is required")
	}
	repo := &Repository{
		path:      path,
		snapshots: map[string]normalize.Snapshot{},
	}
	if err := repo.load(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *Repository) ReplaceSnapshot(_ context.Context, workspaceID string, snapshot normalize.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.snapshots[workspaceID] = snapshot
	return r.persist()
}

func (r *Repository) GetSummary(_ context.Context, workspaceID string) (normalize.Summary, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return normalize.Summary{}, false, err
	}
	snapshot, ok := r.snapshots[workspaceID]
	if !ok {
		return normalize.Summary{}, false, nil
	}
	return snapshot.Summary, true, nil
}

func (r *Repository) GetSnapshot(_ context.Context, workspaceID string) (normalize.Snapshot, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return normalize.Snapshot{}, false, err
	}
	snapshot, ok := r.snapshots[workspaceID]
	if !ok {
		return normalize.Snapshot{}, false, nil
	}
	return snapshot, true, nil
}

func (r *Repository) Reset(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	delete(r.snapshots, workspaceID)
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
	if state.Snapshots != nil {
		r.snapshots = state.Snapshots
	} else {
		r.snapshots = map[string]normalize.Snapshot{}
	}
	return nil
}

func (r *Repository) persist() error {
	data, err := json.MarshalIndent(fileState{Snapshots: r.snapshots}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}
