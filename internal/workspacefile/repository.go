package workspacefile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type DefaultWorkspaceRepository struct {
	mu    sync.RWMutex
	path  string
	items map[string]workspace.DefaultWorkspace
}

func NewDefaultWorkspaceRepository(path string) (*DefaultWorkspaceRepository, error) {
	if path == "" {
		return nil, errors.New("workspace repository path is required")
	}

	repo := &DefaultWorkspaceRepository{
		path:  path,
		items: make(map[string]workspace.DefaultWorkspace),
	}

	if err := repo.load(); err != nil {
		return nil, err
	}

	return repo, nil
}

func (r *DefaultWorkspaceRepository) Save(_ context.Context, item workspace.DefaultWorkspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.items[item.OwnerUserID] = item
	return r.persistLocked()
}

func (r *DefaultWorkspaceRepository) UpsertDefaultWorkspace(_ context.Context, ownerUserID string, build func(existing workspace.DefaultWorkspace, found bool) (workspace.DefaultWorkspace, string, error)) (workspace.DefaultWorkspace, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return workspace.DefaultWorkspace{}, "", err
	}
	existing, found := r.items[ownerUserID]
	next, resolution, err := build(existing, found)
	if err != nil {
		return workspace.DefaultWorkspace{}, "", err
	}

	r.items[ownerUserID] = next
	if err := r.persistLocked(); err != nil {
		return workspace.DefaultWorkspace{}, "", err
	}

	return next, resolution, nil
}

func (r *DefaultWorkspaceRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *DefaultWorkspaceRepository) loadLocked() error {
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

	var items map[string]workspace.DefaultWorkspace
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	if items == nil {
		items = make(map[string]workspace.DefaultWorkspace)
	}
	r.items = items
	return nil
}

func (r *DefaultWorkspaceRepository) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(r.items, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}
