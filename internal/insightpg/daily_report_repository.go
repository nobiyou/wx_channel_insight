package insightpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type DailyReportRepository struct {
	db *sql.DB
}

var _ insight.DailyReportRepository = (*DailyReportRepository)(nil)

func NewDailyReportRepository(client *postgres.Client) (*DailyReportRepository, error) {
	if client == nil || client.DB() == nil {
		return nil, errors.New("postgres client is required")
	}
	if err := postgres.EnsureDailyReportSchema(client.DB()); err != nil {
		return nil, err
	}
	return &DailyReportRepository{db: client.DB()}, nil
}

func (r *DailyReportRepository) List(ctx context.Context, workspaceID, dataWindow string, limit int) ([]insight.DailyReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = 30
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT payload
FROM insight_daily_reports
WHERE workspace_id = $1 AND ($2 = '' OR data_window = $2)
ORDER BY report_date DESC, generated_at DESC, report_id DESC
LIMIT $3`, workspaceID, dataWindow, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]insight.DailyReport, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var report insight.DailyReport
		if err := json.Unmarshal(payload, &report); err != nil {
			return nil, err
		}
		items = append(items, insight.CloneDailyReport(report))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *DailyReportRepository) Get(ctx context.Context, workspaceID, reportID string) (insight.DailyReport, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var payload []byte
	err := r.db.QueryRowContext(ctx, `
SELECT payload FROM insight_daily_reports
WHERE workspace_id = $1 AND report_id = $2`, workspaceID, reportID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return insight.DailyReport{}, false, nil
	}
	if err != nil {
		return insight.DailyReport{}, false, err
	}
	var report insight.DailyReport
	if err := json.Unmarshal(payload, &report); err != nil {
		return insight.DailyReport{}, true, err
	}
	return insight.CloneDailyReport(report), true, nil
}

func (r *DailyReportRepository) Upsert(ctx context.Context, report insight.DailyReport) error {
	if err := insight.ValidateDailyReport(report); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_daily_reports (workspace_id, report_id, report_date, data_window, generated_at, payload)
VALUES ($1, $2, $3, $4, $5, $6::jsonb)
ON CONFLICT (workspace_id, report_date, data_window) DO UPDATE SET
  report_id = EXCLUDED.report_id,
  generated_at = EXCLUDED.generated_at,
  payload = EXCLUDED.payload,
  updated_at = now()`,
		report.WorkspaceID, report.ID, report.ReportDate, report.DataWindow, report.GeneratedAt, string(payload))
	return err
}

func normalizeDailyReportWindow(value string) string {
	return strings.TrimSpace(value)
}
