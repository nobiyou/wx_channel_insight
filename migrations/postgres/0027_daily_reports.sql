CREATE TABLE IF NOT EXISTS insight_daily_reports (
  workspace_id TEXT NOT NULL,
  report_id TEXT NOT NULL,
  report_date TEXT NOT NULL,
  data_window TEXT NOT NULL,
  generated_at TIMESTAMPTZ NOT NULL,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, report_id),
  UNIQUE (workspace_id, report_date, data_window)
);

CREATE INDEX IF NOT EXISTS idx_insight_daily_reports_workspace_date
  ON insight_daily_reports (workspace_id, report_date DESC, generated_at DESC);
