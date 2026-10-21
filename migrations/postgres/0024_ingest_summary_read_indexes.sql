CREATE INDEX IF NOT EXISTS idx_insight_ingest_events_summary_recent
  ON insight_ingest_events (hub_user_id, collected_at DESC, batch_id DESC, event_id DESC);

CREATE INDEX IF NOT EXISTS idx_insight_ingest_events_summary_source
  ON insight_ingest_events (hub_user_id, source, collected_at DESC, batch_id DESC, event_id DESC);

CREATE INDEX IF NOT EXISTS idx_insight_ingest_runs_summary_recent
  ON insight_ingest_runs (hub_user_id, finished_at DESC, run_id DESC);

CREATE INDEX IF NOT EXISTS idx_insight_ingest_runs_summary_source_recent
  ON insight_ingest_runs (hub_user_id, source, finished_at DESC, run_id DESC);

CREATE INDEX IF NOT EXISTS idx_insight_ingest_runs_summary_source_status
  ON insight_ingest_runs (hub_user_id, source, status, finished_at DESC, run_id DESC);
