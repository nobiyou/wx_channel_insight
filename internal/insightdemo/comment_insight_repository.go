package insightdemo

import (
	"context"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type CommentInsightRepository struct {
	mu    sync.RWMutex
	seed  []insight.CommentInsight
	items map[string][]insight.CommentInsight
	meta  map[string]insight.ReadModelArtifactMeta
}

func NewCommentInsightRepository(seed []insight.CommentInsight) *CommentInsightRepository {
	return &CommentInsightRepository{
		seed: insight.CloneCommentInsights(seed),
		items: map[string][]insight.CommentInsight{
			"": insight.CloneCommentInsights(seed),
		},
		meta: map[string]insight.ReadModelArtifactMeta{},
	}
}

func (r *CommentInsightRepository) GetCommentInsights(_ context.Context, workspaceID string) ([]insight.CommentInsight, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, ok := r.items[workspaceID]
	if ok {
		return insight.CloneCommentInsights(items), true, nil
	}
	return insight.CloneCommentInsights(r.seed), false, nil
}

func (r *CommentInsightRepository) ReplaceCommentInsights(_ context.Context, workspaceID string, items []insight.CommentInsight) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[workspaceID] = insight.CloneCommentInsights(items)
	return nil
}

func (r *CommentInsightRepository) GetMeta(_ context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, ok := r.meta[workspaceID]
	return meta, ok, nil
}

func (r *CommentInsightRepository) ReplaceCommentInsightMeta(_ context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.meta[workspaceID] = meta
	return nil
}

func (r *CommentInsightRepository) ResetCommentInsights(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[workspaceID] = insight.CloneCommentInsights(r.seed)
	return nil
}
