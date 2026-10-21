package insightfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type persistedDashboards struct {
	Items map[string]insight.Dashboard `json:"items"`
}

type DashboardRepository struct {
	mu        sync.RWMutex
	path      string
	seed      insight.Dashboard
	snapshots map[string]insight.Dashboard
}

func NewDashboardRepository(path string, seed insight.Dashboard) (*DashboardRepository, error) {
	if path == "" {
		return nil, errors.New("dashboard repository path is required")
	}

	repo := &DashboardRepository{
		path: path,
		seed: seed,
		snapshots: map[string]insight.Dashboard{
			"": seed,
		},
	}
	if err := repo.loadOrSeed(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *DashboardRepository) GetDashboard(_ context.Context, workspaceID string) (insight.Dashboard, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return insight.Dashboard{}, err
	}
	if snapshot, ok := r.snapshots[workspaceID]; ok {
		return snapshot, nil
	}
	return r.seed, nil
}

func (r *DashboardRepository) ReplaceDashboard(_ context.Context, workspaceID string, snapshot insight.Dashboard) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return err
	}
	r.snapshots[workspaceID] = snapshot
	return r.persist()
}

func (r *DashboardRepository) ResetDashboard(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return err
	}
	r.snapshots[workspaceID] = r.seed
	return r.persist()
}

func (r *DashboardRepository) loadOrSeed() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadOrSeedLocked()
}

func (r *DashboardRepository) loadOrSeedLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r.persist()
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return r.persist()
	}

	var payload persistedDashboards
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.Items == nil {
		return errors.New("dashboard storage requires a workspace items object")
	}
	r.snapshots = payload.Items
	if _, ok := r.snapshots[""]; !ok {
		r.snapshots[""] = r.seed
	}
	return nil
}

func (r *DashboardRepository) persist() error {
	payload := persistedDashboards{
		Items: r.snapshots,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}
