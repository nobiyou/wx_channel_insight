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

type persistedCommentInsights struct {
	Items map[string][]insight.CommentInsight      `json:"items"`
	Meta  map[string]insight.ReadModelArtifactMeta `json:"meta"`
}

type CommentInsightRepository struct {
	mu    sync.RWMutex
	path  string
	seed  []insight.CommentInsight
	items map[string][]insight.CommentInsight
	meta  map[string]insight.ReadModelArtifactMeta
}

func NewCommentInsightRepository(path string, seed []insight.CommentInsight) (*CommentInsightRepository, error) {
	if path == "" {
		return nil, errors.New("comment insight repository path is required")
	}

	repo := &CommentInsightRepository{
		path: path,
		seed: insight.CloneCommentInsights(seed),
		items: map[string][]insight.CommentInsight{
			"": insight.CloneCommentInsights(seed),
		},
		meta: map[string]insight.ReadModelArtifactMeta{},
	}
	if err := repo.loadOrSeed(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *CommentInsightRepository) GetCommentInsights(_ context.Context, workspaceID string) ([]insight.CommentInsight, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return nil, false, err
	}
	items, ok := r.items[workspaceID]
	if ok {
		return insight.CloneCommentInsights(items), true, nil
	}
	return insight.CloneCommentInsights(r.seed), false, nil
}

func (r *CommentInsightRepository) ReplaceCommentInsights(_ context.Context, workspaceID string, items []insight.CommentInsight) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return err
	}
	r.items[workspaceID] = insight.CloneCommentInsights(items)
	return r.persist()
}

func (r *CommentInsightRepository) ResetCommentInsights(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return err
	}
	r.items[workspaceID] = insight.CloneCommentInsights(r.seed)
	return r.persist()
}

func (r *CommentInsightRepository) GetMeta(_ context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return insight.ReadModelArtifactMeta{}, false, err
	}
	meta, ok := r.meta[workspaceID]
	return meta, ok, nil
}

func (r *CommentInsightRepository) ReplaceCommentInsightMeta(_ context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return err
	}
	r.meta[workspaceID] = meta
	return r.persist()
}

func (r *CommentInsightRepository) loadOrSeed() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadOrSeedLocked()
}

func (r *CommentInsightRepository) loadOrSeedLocked() error {
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

	var payload persistedCommentInsights
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.Items == nil {
		return errors.New("comment insight storage requires a workspace items object")
	}
	r.items = payload.Items
	r.meta = payload.Meta
	if r.meta == nil {
		r.meta = map[string]insight.ReadModelArtifactMeta{}
	}
	if _, ok := r.items[""]; !ok {
		r.items[""] = insight.CloneCommentInsights(r.seed)
	}
	return nil
}

func (r *CommentInsightRepository) persist() error {
	payload := persistedCommentInsights{
		Items: r.items,
		Meta:  r.meta,
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
