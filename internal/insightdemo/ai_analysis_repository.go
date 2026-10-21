package insightdemo

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type AIAnalysisRepository struct {
	mu    sync.RWMutex
	items []insight.AIAnalysisRun
}

var _ insight.AIAnalysisRepository = (*AIAnalysisRepository)(nil)

func NewAIAnalysisRepository() *AIAnalysisRepository {
	return &AIAnalysisRepository{items: make([]insight.AIAnalysisRun, 0)}
}

func (r *AIAnalysisRepository) Save(_ context.Context, run insight.AIAnalysisRun) error {
	normalized, err := insight.NormalizeAIAnalysisRun(run)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, existing := range r.items {
		if existing.WorkspaceID == normalized.WorkspaceID && existing.ID == normalized.ID {
			r.items[index] = insight.CloneAIAnalysisRun(normalized)
			return nil
		}
	}
	r.items = append(r.items, insight.CloneAIAnalysisRun(normalized))
	return nil
}

func (r *AIAnalysisRepository) List(_ context.Context, workspaceID, module, scopeKey, window string, limit int) ([]insight.AIAnalysisRun, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	module = strings.ToLower(strings.TrimSpace(module))
	scopeKey = strings.TrimSpace(scopeKey)
	window = strings.ToLower(strings.TrimSpace(window))
	if window != "" {
		var ok bool
		window, ok = insight.NormalizeTimeWindow(window)
		if !ok {
			return nil, insight.ErrInvalidTimeWindow
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]insight.AIAnalysisRun, 0)
	for _, item := range r.items {
		if item.WorkspaceID != workspaceID || (module != "" && item.Module != module) || (scopeKey != "" && item.ScopeKey != scopeKey) || item.Window != window {
			continue
		}
		items = append(items, insight.CloneAIAnalysisRun(item))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].RunAt.Equal(items[j].RunAt) {
			return items[i].RunAt.After(items[j].RunAt)
		}
		return items[i].ID > items[j].ID
	})
	if limit <= 0 || limit > insight.MaxAIAnalysisHistoryLimit {
		limit = insight.MaxAIAnalysisHistoryLimit
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *AIAnalysisRepository) Get(_ context.Context, workspaceID, runID string) (insight.AIAnalysisRun, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.WorkspaceID == strings.TrimSpace(workspaceID) && item.ID == strings.TrimSpace(runID) {
			return insight.CloneAIAnalysisRun(item), true, nil
		}
	}
	return insight.AIAnalysisRun{}, false, nil
}
