package insightdemo

import (
	"context"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type RecommendationRepository struct {
	mu        sync.RWMutex
	snapshots map[string]insight.RecommendationSnapshot
}

func NewRecommendationRepository() *RecommendationRepository {
	return &RecommendationRepository{snapshots: map[string]insight.RecommendationSnapshot{}}
}

var _ insight.RecommendationRepository = (*RecommendationRepository)(nil)

func (r *RecommendationRepository) GetRecommendations(_ context.Context, workspaceID string) (insight.RecommendationSnapshot, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot, ok := r.snapshots[workspaceID]
	if !ok {
		return insight.RecommendationSnapshot{}, false, nil
	}
	return insight.CloneRecommendationSnapshot(snapshot), true, nil
}

func (r *RecommendationRepository) ReplaceRecommendations(_ context.Context, workspaceID string, snapshot insight.RecommendationSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshots[workspaceID] = insight.CloneRecommendationSnapshot(snapshot)
	return nil
}

func (r *RecommendationRepository) ResetRecommendations(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.snapshots, workspaceID)
	return nil
}
