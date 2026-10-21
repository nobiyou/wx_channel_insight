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

type persistedReportSummaries struct {
	Items map[string]insight.ReportSummary         `json:"items"`
	Meta  map[string]insight.ReadModelArtifactMeta `json:"meta"`
}

type ReportSummaryRepository struct {
	mu        sync.RWMutex
	path      string
	seed      insight.ReportSummary
	summaries map[string]insight.ReportSummary
	meta      map[string]insight.ReadModelArtifactMeta
}

func NewReportSummaryRepository(path string, seed insight.ReportSummary) (*ReportSummaryRepository, error) {
	if path == "" {
		return nil, errors.New("report summary repository path is required")
	}

	repo := &ReportSummaryRepository{
		path: path,
		seed: seed,
		summaries: map[string]insight.ReportSummary{
			"": seed,
		},
		meta: map[string]insight.ReadModelArtifactMeta{},
	}
	if err := repo.loadOrSeed(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *ReportSummaryRepository) GetReportSummary(_ context.Context, workspaceID string) (insight.ReportSummary, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return insight.ReportSummary{}, false, err
	}
	summary, ok := r.summaries[workspaceID]
	if ok {
		return summary, true, nil
	}
	return r.seed, false, nil
}

func (r *ReportSummaryRepository) ReplaceReportSummary(_ context.Context, workspaceID string, summary insight.ReportSummary) error {
	if err := insight.ValidateReportSummary(summary); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return err
	}
	r.summaries[workspaceID] = summary
	return r.persist()
}

func (r *ReportSummaryRepository) ResetReportSummary(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return err
	}
	r.summaries[workspaceID] = r.seed
	return r.persist()
}

func (r *ReportSummaryRepository) GetMeta(_ context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return insight.ReadModelArtifactMeta{}, false, err
	}
	meta, ok := r.meta[workspaceID]
	return meta, ok, nil
}

func (r *ReportSummaryRepository) ReplaceReportSummaryMeta(_ context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadOrSeedLocked(); err != nil {
		return err
	}
	r.meta[workspaceID] = meta
	return r.persist()
}

func (r *ReportSummaryRepository) loadOrSeed() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadOrSeedLocked()
}

func (r *ReportSummaryRepository) loadOrSeedLocked() error {
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

	var payload persistedReportSummaries
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.Items == nil {
		return errors.New("report summary storage requires a workspace items object")
	}
	r.summaries = payload.Items
	r.meta = payload.Meta
	if r.meta == nil {
		r.meta = map[string]insight.ReadModelArtifactMeta{}
	}
	if _, ok := r.summaries[""]; !ok {
		r.summaries[""] = r.seed
	}
	return nil
}

func (r *ReportSummaryRepository) persist() error {
	payload := persistedReportSummaries{
		Items: r.summaries,
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
