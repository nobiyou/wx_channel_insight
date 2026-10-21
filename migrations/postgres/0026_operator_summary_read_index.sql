CREATE INDEX IF NOT EXISTS idx_insight_operator_runs_summary_recent
  ON insight_operator_runs (hub_user_id, finished_at DESC, run_id DESC)
  INCLUDE (trigger_mode, latest_batch_id);
