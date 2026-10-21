package workspacedemo

import (
	"context"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type DefaultWorkspaceRepository struct {
	mu    sync.RWMutex
	items map[string]workspace.DefaultWorkspace
}

func NewDefaultWorkspaceRepository() *DefaultWorkspaceRepository {
	return &DefaultWorkspaceRepository{
		items: make(map[string]workspace.DefaultWorkspace),
	}
}

func (r *DefaultWorkspaceRepository) Save(_ context.Context, item workspace.DefaultWorkspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[item.OwnerUserID] = item
	return nil
}

func (r *DefaultWorkspaceRepository) UpsertDefaultWorkspace(_ context.Context, ownerUserID string, build func(existing workspace.DefaultWorkspace, found bool) (workspace.DefaultWorkspace, string, error)) (workspace.DefaultWorkspace, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, found := r.items[ownerUserID]
	next, resolution, err := build(existing, found)
	if err != nil {
		return workspace.DefaultWorkspace{}, "", err
	}

	r.items[ownerUserID] = next
	return next, resolution, nil
}
