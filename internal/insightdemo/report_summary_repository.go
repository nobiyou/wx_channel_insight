package insightdemo

import (
	"context"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type ReportSummaryRepository struct {
	mu        sync.RWMutex
	seed      insight.ReportSummary
	summaries map[string]insight.ReportSummary
	meta      map[string]insight.ReadModelArtifactMeta
}

func NewReportSummaryRepository(seed insight.ReportSummary) *ReportSummaryRepository {
	return &ReportSummaryRepository{
		seed: seed,
		summaries: map[string]insight.ReportSummary{
			"": seed,
		},
		meta: map[string]insight.ReadModelArtifactMeta{},
	}
}

func (r *ReportSummaryRepository) GetReportSummary(_ context.Context, workspaceID string) (insight.ReportSummary, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summary, ok := r.summaries[workspaceID]
	if ok {
		return summary, true, nil
	}
	return r.seed, false, nil
}

func (r *ReportSummaryRepository) ReplaceReportSummary(_ context.Context, workspaceID string, summary insight.ReportSummary) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.summaries[workspaceID] = summary
	return nil
}

func (r *ReportSummaryRepository) GetMeta(_ context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, ok := r.meta[workspaceID]
	return meta, ok, nil
}

func (r *ReportSummaryRepository) ReplaceReportSummaryMeta(_ context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.meta[workspaceID] = meta
	return nil
}

func (r *ReportSummaryRepository) ResetReportSummary(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.summaries[workspaceID] = r.seed
	return nil
}
