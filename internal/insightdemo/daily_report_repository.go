package insightdemo

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type DailyReportRepository struct {
	mu      sync.RWMutex
	reports map[string]insight.DailyReport
}

var _ insight.DailyReportRepository = (*DailyReportRepository)(nil)

func NewDailyReportRepository() *DailyReportRepository {
	return &DailyReportRepository{reports: map[string]insight.DailyReport{}}
}

func (r *DailyReportRepository) List(_ context.Context, workspaceID, dataWindow string, limit int) ([]insight.DailyReport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]insight.DailyReport, 0)
	for _, report := range r.reports {
		if report.WorkspaceID != workspaceID || (dataWindow != "" && report.DataWindow != dataWindow) {
			continue
		}
		items = append(items, insight.CloneDailyReport(report))
	}
	sortDailyReports(items)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *DailyReportRepository) Get(_ context.Context, workspaceID, reportID string) (insight.DailyReport, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	report, ok := r.reports[reportID]
	if !ok || report.WorkspaceID != workspaceID {
		return insight.DailyReport{}, false, nil
	}
	return insight.CloneDailyReport(report), true, nil
}

func (r *DailyReportRepository) Upsert(_ context.Context, report insight.DailyReport) error {
	if err := insight.ValidateDailyReport(report); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reports == nil {
		r.reports = map[string]insight.DailyReport{}
	}
	r.reports[report.ID] = insight.CloneDailyReport(report)
	return nil
}

func sortDailyReports(items []insight.DailyReport) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].ReportDate != items[j].ReportDate {
			return items[i].ReportDate > items[j].ReportDate
		}
		if !items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].GeneratedAt.After(items[j].GeneratedAt)
		}
		return strings.Compare(items[i].ID, items[j].ID) > 0
	})
}
