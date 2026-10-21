package sqlite

import (
	"context"
	"sort"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type DailyReportRepository struct{ c *Client }

func NewDailyReportRepository(c *Client) (*DailyReportRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &DailyReportRepository{c}, nil
}
func (r *DailyReportRepository) List(ctx context.Context, scope, window string, limit int) ([]insight.DailyReport, error) {
	all, err := list[insight.DailyReport](ctx, r.c.db, "daily_report", scope)
	if err != nil {
		return nil, err
	}
	items := make([]insight.DailyReport, 0, len(all))
	for _, v := range all {
		if window == "" || v.DataWindow == window {
			items = append(items, v)
		}
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
func (r *DailyReportRepository) Get(ctx context.Context, scope, id string) (insight.DailyReport, bool, error) {
	return get[insight.DailyReport](ctx, r.c.db, "daily_report", scope, id)
}
func (r *DailyReportRepository) Upsert(ctx context.Context, v insight.DailyReport) error {
	if err := insight.ValidateDailyReport(v); err != nil {
		return err
	}
	return put(ctx, r.c.db, "daily_report", v.WorkspaceID, v.ID, v)
}

type AIAnalysisRepository struct{ c *Client }

func NewAIAnalysisRepository(c *Client) (*AIAnalysisRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &AIAnalysisRepository{c}, nil
}
func (r *AIAnalysisRepository) Save(ctx context.Context, v insight.AIAnalysisRun) error {
	v, err := insight.NormalizeAIAnalysisRun(v)
	if err != nil {
		return err
	}
	return put(ctx, r.c.db, "ai_analysis", v.WorkspaceID, v.ID, v)
}
func (r *AIAnalysisRepository) Get(ctx context.Context, scope, id string) (insight.AIAnalysisRun, bool, error) {
	return get[insight.AIAnalysisRun](ctx, r.c.db, "ai_analysis", strings.TrimSpace(scope), strings.TrimSpace(id))
}
func (r *AIAnalysisRepository) List(ctx context.Context, scope, module, key, window string, limit int) ([]insight.AIAnalysisRun, error) {
	scope = strings.TrimSpace(scope)
	module = strings.ToLower(strings.TrimSpace(module))
	key = strings.TrimSpace(key)
	window = strings.ToLower(strings.TrimSpace(window))
	if window != "" {
		var ok bool
		window, ok = insight.NormalizeTimeWindow(window)
		if !ok {
			return nil, insight.ErrInvalidTimeWindow
		}
	}
	all, err := list[insight.AIAnalysisRun](ctx, r.c.db, "ai_analysis", scope)
	if err != nil {
		return nil, err
	}
	items := make([]insight.AIAnalysisRun, 0, len(all))
	for _, v := range all {
		if (module == "" || v.Module == module) && (key == "" || v.ScopeKey == key) && (window == "" || v.Window == window) {
			items = append(items, v)
		}
	}
	sort.Slice(items, func(i, j int) bool {
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

var (
	_ insight.DailyReportRepository = (*DailyReportRepository)(nil)
	_ insight.AIAnalysisRepository  = (*AIAnalysisRepository)(nil)
)
