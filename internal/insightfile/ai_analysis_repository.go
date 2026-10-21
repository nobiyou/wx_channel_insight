package insightfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type persistedAIAnalysisRuns struct {
	Items []insight.AIAnalysisRun `json:"items"`
}

type AIAnalysisRepository struct {
	mu    sync.Mutex
	path  string
	items []insight.AIAnalysisRun
}

var _ insight.AIAnalysisRepository = (*AIAnalysisRepository)(nil)

func NewAIAnalysisRepository(path string) (*AIAnalysisRepository, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("AI analysis repository path is required")
	}
	repo := &AIAnalysisRepository{path: path, items: make([]insight.AIAnalysisRun, 0)}
	if err := repo.load(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *AIAnalysisRepository) Save(_ context.Context, run insight.AIAnalysisRun) error {
	normalized, err := insight.NormalizeAIAnalysisRun(run)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	for index, existing := range r.items {
		if existing.WorkspaceID == normalized.WorkspaceID && existing.ID == normalized.ID {
			r.items[index] = insight.CloneAIAnalysisRun(normalized)
			return r.persistLocked()
		}
	}
	r.items = append(r.items, insight.CloneAIAnalysisRun(normalized))
	return r.persistLocked()
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	items := make([]insight.AIAnalysisRun, 0)
	for _, item := range r.items {
		if item.WorkspaceID != workspaceID || (module != "" && item.Module != module) || (scopeKey != "" && item.ScopeKey != scopeKey) || (window != "" && item.Window != window) {
			continue
		}
		items = append(items, insight.CloneAIAnalysisRun(item))
	}
	sortAIAnalysisRuns(items)
	if limit <= 0 || limit > insight.MaxAIAnalysisHistoryLimit {
		limit = insight.MaxAIAnalysisHistoryLimit
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *AIAnalysisRepository) Get(_ context.Context, workspaceID, runID string) (insight.AIAnalysisRun, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return insight.AIAnalysisRun{}, false, err
	}
	for _, item := range r.items {
		if item.WorkspaceID == workspaceID && item.ID == runID {
			return insight.CloneAIAnalysisRun(item), true, nil
		}
	}
	return insight.AIAnalysisRun{}, false, nil
}

func (r *AIAnalysisRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *AIAnalysisRepository) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) || len(data) == 0 {
		r.items = make([]insight.AIAnalysisRun, 0)
		return r.persistLocked()
	}
	if err != nil {
		return err
	}
	var payload persistedAIAnalysisRuns
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	items := make([]insight.AIAnalysisRun, 0, len(payload.Items))
	for _, item := range payload.Items {
		normalized, err := insight.NormalizeAIAnalysisRun(item)
		if err != nil {
			return err
		}
		items = append(items, normalized)
	}
	r.items = items
	return nil
}

func (r *AIAnalysisRepository) persistLocked() error {
	data, err := json.MarshalIndent(persistedAIAnalysisRuns{Items: r.items}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}

func sortAIAnalysisRuns(items []insight.AIAnalysisRun) {
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].RunAt.Equal(items[j].RunAt) {
			return items[i].RunAt.After(items[j].RunAt)
		}
		return items[i].ID > items[j].ID
	})
}
