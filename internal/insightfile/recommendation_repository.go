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

type recommendationFileState struct {
	Snapshots map[string]insight.RecommendationSnapshot `json:"snapshots"`
}

type RecommendationRepository struct {
	mu        sync.RWMutex
	path      string
	snapshots map[string]insight.RecommendationSnapshot
}

func NewRecommendationRepository(path string) (*RecommendationRepository, error) {
	if path == "" {
		return nil, errors.New("recommendation repository path is required")
	}
	repo := &RecommendationRepository{path: path, snapshots: map[string]insight.RecommendationSnapshot{}}
	if err := repo.load(); err != nil {
		return nil, err
	}
	return repo, nil
}

var _ insight.RecommendationRepository = (*RecommendationRepository)(nil)

func (r *RecommendationRepository) GetRecommendations(_ context.Context, workspaceID string) (insight.RecommendationSnapshot, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return insight.RecommendationSnapshot{}, false, err
	}
	snapshot, ok := r.snapshots[workspaceID]
	if !ok {
		return insight.RecommendationSnapshot{}, false, nil
	}
	return insight.CloneRecommendationSnapshot(snapshot), true, nil
}

func (r *RecommendationRepository) ReplaceRecommendations(_ context.Context, workspaceID string, snapshot insight.RecommendationSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	r.snapshots[workspaceID] = insight.CloneRecommendationSnapshot(snapshot)
	return r.persist()
}

func (r *RecommendationRepository) ResetRecommendations(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	delete(r.snapshots, workspaceID)
	return r.persist()
}

func (r *RecommendationRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *RecommendationRepository) loadLocked() error {
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
	var state recommendationFileState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Snapshots != nil {
		r.snapshots = state.Snapshots
	}
	return nil
}

func (r *RecommendationRepository) persist() error {
	data, err := json.MarshalIndent(recommendationFileState{Snapshots: r.snapshots}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}
