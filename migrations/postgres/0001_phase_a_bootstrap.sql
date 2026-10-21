CREATE TABLE IF NOT EXISTS insight_default_workspaces (
  owner_user_id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  name TEXT NOT NULL,
  device_count INTEGER NOT NULL,
  bootstrap_mode TEXT NOT NULL,
  source TEXT NOT NULL,
  state TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  last_synced_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS insight_ingest_events (
  hub_user_id TEXT NOT NULL,
  event_id TEXT NOT NULL,
  batch_id TEXT NOT NULL,
  machine_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  source TEXT NOT NULL,
  collected_at TIMESTAMPTZ NOT NULL,
  video_count INTEGER NOT NULL,
  comment_count INTEGER NOT NULL,
  topic_count INTEGER NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  channel_name TEXT NOT NULL DEFAULT '',
  keyword TEXT NOT NULL DEFAULT '',
  cluster TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  prompt_angle TEXT NOT NULL DEFAULT '',
  representative_comment TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (hub_user_id, event_id)
);

CREATE TABLE IF NOT EXISTS insight_ingest_runs (
  hub_user_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  source TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  finished_at TIMESTAMPTZ NOT NULL,
  batch_id TEXT NOT NULL DEFAULT '',
  event_count INTEGER NOT NULL DEFAULT 0,
  latest_event_at TIMESTAMPTZ,
  last_event_type TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (hub_user_id, run_id)
);

CREATE TABLE IF NOT EXISTS insight_operator_runs (
  hub_user_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  finished_at TIMESTAMPTZ NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  latest_batch_id TEXT NOT NULL DEFAULT '',
  attempted_sources INTEGER NOT NULL DEFAULT 0,
  succeeded_sources INTEGER NOT NULL DEFAULT 0,
  failed_sources INTEGER NOT NULL DEFAULT 0,
  skipped_sources INTEGER NOT NULL DEFAULT 0,
  freshness_state TEXT NOT NULL DEFAULT '',
  report_headline TEXT NOT NULL DEFAULT '',
  top_video_title TEXT NOT NULL DEFAULT '',
  top_topic_keyword TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (hub_user_id, run_id)
);

ALTER TABLE insight_operator_runs
ADD COLUMN IF NOT EXISTS trigger_mode TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS insight_operator_run_outcomes (
  hub_user_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  outcome_index INTEGER NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (hub_user_id, run_id, outcome_index)
);

CREATE TABLE IF NOT EXISTS insight_ingest_source_bindings (
  hub_user_id TEXT NOT NULL,
  source TEXT NOT NULL,
  path TEXT NOT NULL DEFAULT '',
  base_url TEXT NOT NULL DEFAULT '',
  username TEXT NOT NULL DEFAULT '',
  device_count INTEGER NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (hub_user_id, source)
);

CREATE TABLE IF NOT EXISTS insight_dashboard_snapshots (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS insight_report_summaries (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS insight_hot_video_rankings (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS insight_hot_topic_rankings (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS insight_comment_insights (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
