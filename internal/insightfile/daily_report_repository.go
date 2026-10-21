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

type persistedDailyReports struct {
	Items map[string]insight.DailyReport `json:"items"`
}

type DailyReportRepository struct {
	mu      sync.Mutex
	path    string
	reports map[string]insight.DailyReport
}

var _ insight.DailyReportRepository = (*DailyReportRepository)(nil)

func NewDailyReportRepository(path string) (*DailyReportRepository, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("daily report repository path is required")
	}
	repo := &DailyReportRepository{path: path, reports: map[string]insight.DailyReport{}}
	if err := repo.load(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *DailyReportRepository) List(_ context.Context, workspaceID, dataWindow string, limit int) ([]insight.DailyReport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	items := make([]insight.DailyReport, 0)
	for _, report := range r.reports {
		if report.WorkspaceID != workspaceID || (dataWindow != "" && report.DataWindow != dataWindow) {
			continue
		}
		items = append(items, insight.CloneDailyReport(report))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ReportDate != items[j].ReportDate {
			return items[i].ReportDate > items[j].ReportDate
		}
		if !items[i].GeneratedAt.Equal(items[j].GeneratedAt) {
			return items[i].GeneratedAt.After(items[j].GeneratedAt)
		}
		return items[i].ID > items[j].ID
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *DailyReportRepository) Get(_ context.Context, workspaceID, reportID string) (insight.DailyReport, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return insight.DailyReport{}, false, err
	}
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
	if err := r.loadLocked(); err != nil {
		return err
	}
	r.reports[report.ID] = insight.CloneDailyReport(report)
	return r.persistLocked()
}

func (r *DailyReportRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *DailyReportRepository) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) || len(data) == 0 {
		return r.persistLocked()
	}
	if err != nil {
		return err
	}
	var payload persistedDailyReports
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.Items == nil {
		payload.Items = map[string]insight.DailyReport{}
	}
	r.reports = payload.Items
	return nil
}

func (r *DailyReportRepository) persistLocked() error {
	data, err := json.MarshalIndent(persistedDailyReports{Items: r.reports}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}
