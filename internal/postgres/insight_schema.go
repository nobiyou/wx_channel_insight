package postgres

import "database/sql"

func EnsurePhaseABootstrapSchema(db *sql.DB) error {
	return ensureSchema(
		db,
		`
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
)`,
		`
CREATE TABLE IF NOT EXISTS insight_ingest_events (
  hub_user_id TEXT NOT NULL,
  event_id TEXT NOT NULL,
  batch_id TEXT NOT NULL,
  machine_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  source TEXT NOT NULL,
  collected_at TIMESTAMPTZ NOT NULL,
  published_at TIMESTAMPTZ,
  object_id TEXT NOT NULL DEFAULT '',
  object_nonce_id TEXT NOT NULL DEFAULT '',
  video_count INTEGER NOT NULL,
  play_count INTEGER NOT NULL DEFAULT 0,
  like_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL,
  fav_count INTEGER NOT NULL DEFAULT 0,
  forward_count INTEGER NOT NULL DEFAULT 0,
  topic_count INTEGER NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  channel_name TEXT NOT NULL DEFAULT '',
  channel_username TEXT NOT NULL DEFAULT '',
  cover_url TEXT NOT NULL DEFAULT '',
  keyword TEXT NOT NULL DEFAULT '',
  cluster TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  prompt_angle TEXT NOT NULL DEFAULT '',
  representative_comment TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (hub_user_id, event_id)
)`,
		`
ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS play_count INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS like_count INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS fav_count INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS forward_count INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ
`,
		`
ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS channel_username TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS cover_url TEXT NOT NULL DEFAULT ''
`,
		`
CREATE TABLE IF NOT EXISTS insight_video_metric_observations (
  hub_user_id TEXT NOT NULL,
  observation_id TEXT NOT NULL,
  event_id TEXT NOT NULL DEFAULT '',
  batch_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  observed_at TIMESTAMPTZ NOT NULL,
  published_at TIMESTAMPTZ,
  object_id TEXT NOT NULL DEFAULT '',
  object_nonce_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  channel_name TEXT NOT NULL DEFAULT '',
  channel_username TEXT NOT NULL DEFAULT '',
  cover_url TEXT NOT NULL DEFAULT '',
  keyword TEXT NOT NULL DEFAULT '',
  tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  play_count INTEGER NOT NULL DEFAULT 0,
  like_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  fav_count INTEGER NOT NULL DEFAULT 0,
  forward_count INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (hub_user_id, observation_id)
)
`,
		`
ALTER TABLE insight_video_metric_observations
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ
`,
		`
ALTER TABLE insight_video_metric_observations
  ADD COLUMN IF NOT EXISTS channel_username TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_video_metric_observations
  ADD COLUMN IF NOT EXISTS cover_url TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_video_metric_observations
  ADD COLUMN IF NOT EXISTS keyword TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_video_metric_observations
  ADD COLUMN IF NOT EXISTS tags_json JSONB NOT NULL DEFAULT '[]'::jsonb
`,
		`
ALTER TABLE insight_ingest_events ADD COLUMN IF NOT EXISTS missing_metrics INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_video_metric_observations ADD COLUMN IF NOT EXISTS missing_metrics INTEGER NOT NULL DEFAULT 0
`,
		`
CREATE INDEX IF NOT EXISTS idx_insight_video_metric_latest
  ON insight_video_metric_observations (
    hub_user_id,
    (COALESCE(NULLIF(object_id, ''), NULLIF(object_nonce_id, ''), observation_id)),
    observed_at DESC,
    observation_id ASC
  )
`,
		`
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
)`,
		`
ALTER TABLE insight_ingest_runs ADD COLUMN IF NOT EXISTS collection_json JSONB NOT NULL DEFAULT 'null'::jsonb
`,
		`
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
)`,
		`
ALTER TABLE insight_operator_runs
ADD COLUMN IF NOT EXISTS trigger_mode TEXT NOT NULL DEFAULT ''
`,
		`
CREATE TABLE IF NOT EXISTS insight_operator_run_outcomes (
  hub_user_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  outcome_index INTEGER NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (hub_user_id, run_id, outcome_index)
)`,
		`
CREATE TABLE IF NOT EXISTS insight_ingest_source_bindings (
  hub_user_id TEXT NOT NULL,
  source TEXT NOT NULL,
  origin TEXT NOT NULL DEFAULT 'manual',
  target_key TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  base_url TEXT NOT NULL DEFAULT '',
  share_url TEXT NOT NULL DEFAULT '',
  object_id TEXT NOT NULL DEFAULT '',
  object_nonce_id TEXT NOT NULL DEFAULT '',
  username TEXT NOT NULL DEFAULT '',
  keyword TEXT NOT NULL DEFAULT '',
  device_count INTEGER NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (hub_user_id, source, target_key)
)`,
		`
ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS keyword TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS origin TEXT NOT NULL DEFAULT 'manual'
`,
		`
ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS target_key TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS share_url TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT ''
`,
		`
UPDATE insight_ingest_source_bindings
SET target_key = COALESCE(
  CASE
    WHEN source IN ('wx_feed_list', 'wx_channel_feed_list') THEN NULLIF(username, '')
    WHEN source IN ('wx_video_search', 'wx_channel_video_search') THEN NULLIF(keyword, '')
    WHEN source IN ('wx_comment_export', 'wx_channel_comment_export', 'finderGetCommentList', 'finderGetCommentList.partial') THEN NULLIF(path, '')
    ELSE NULLIF(source, '')
  END,
  source
)
WHERE target_key = ''
`,
		`
ALTER TABLE insight_ingest_source_bindings
  DROP CONSTRAINT IF EXISTS insight_ingest_source_bindings_pkey
`,
		`
ALTER TABLE insight_ingest_source_bindings
  ADD CONSTRAINT insight_ingest_source_bindings_pkey PRIMARY KEY (hub_user_id, source, target_key)
`,
		`
CREATE TABLE IF NOT EXISTS insight_dashboard_snapshots (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
		`
CREATE TABLE IF NOT EXISTS insight_report_summaries (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
		`
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
)`,
		`
CREATE INDEX IF NOT EXISTS idx_insight_daily_reports_workspace_date
  ON insight_daily_reports (workspace_id, report_date DESC, generated_at DESC)
`,
		`
CREATE TABLE IF NOT EXISTS insight_hot_video_rankings (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
		`
CREATE TABLE IF NOT EXISTS insight_hot_topic_rankings (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
		`
CREATE TABLE IF NOT EXISTS insight_comment_insights (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
	)
}

func EnsureDefaultWorkspaceSchema(db *sql.DB) error {
	return EnsurePhaseABootstrapSchema(db)
}

func EnsureIngestSchema(db *sql.DB) error {
	return EnsurePhaseABootstrapSchema(db)
}

func EnsureDashboardSchema(db *sql.DB) error {
	return EnsurePhaseABootstrapSchema(db)
}

func EnsureReportSummarySchema(db *sql.DB) error {
	return EnsurePhaseABootstrapSchema(db)
}

func EnsureDailyReportSchema(db *sql.DB) error {
	return EnsurePhaseABootstrapSchema(db)
}

func EnsureRankingSchema(db *sql.DB) error {
	return EnsurePhaseABootstrapSchema(db)
}

func EnsureCommentInsightSchema(db *sql.DB) error {
	return EnsurePhaseABootstrapSchema(db)
}

func EnsureDataCenterExclusionSchema(db *sql.DB) error {
	return ensureSchema(
		db,
		`
CREATE TABLE IF NOT EXISTS insight_data_center_exclusions (
  workspace_id TEXT NOT NULL,
  entity TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  deleted_at TIMESTAMPTZ NOT NULL,
  deleted_by TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (workspace_id, entity, entity_id)
)`,
	)
}

func EnsureRecommendationSchema(db *sql.DB) error {
	return ensureSchema(db, `
CREATE TABLE IF NOT EXISTS insight_recommendation_snapshots (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`)
}

func EnsureAIAnalysisSchema(db *sql.DB) error {
	return ensureSchema(db, `
CREATE TABLE IF NOT EXISTS insight_ai_analysis_runs (
  workspace_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  module TEXT NOT NULL,
  scope_key TEXT NOT NULL,
  data_window TEXT NOT NULL,
  input_fingerprint TEXT NOT NULL,
  status TEXT NOT NULL,
  result JSONB,
  model_provider TEXT NOT NULL DEFAULT '',
  model_name TEXT NOT NULL DEFAULT '',
  model_attempted BOOLEAN NOT NULL DEFAULT false,
  prompt_version TEXT NOT NULL DEFAULT '',
  config_owner_id TEXT NOT NULL DEFAULT '',
  config_source TEXT NOT NULL DEFAULT '',
  config_verification_status TEXT NOT NULL DEFAULT '',
  analysis_engine TEXT NOT NULL DEFAULT '',
  fallback_policy TEXT NOT NULL DEFAULT '',
  fallback_reason TEXT NOT NULL DEFAULT '',
  duration_ms BIGINT NOT NULL DEFAULT 0,
  run_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, run_id)
)`, `
CREATE INDEX IF NOT EXISTS idx_insight_ai_analysis_runs_scope_time
  ON insight_ai_analysis_runs (
    workspace_id, module, scope_key, data_window, run_at DESC, run_id DESC
  )`, `
CREATE INDEX IF NOT EXISTS idx_insight_ai_analysis_runs_workspace_time
  ON insight_ai_analysis_runs (workspace_id, run_at DESC, run_id DESC)
`)
}

func EnsureNormalizationSchema(db *sql.DB) error {
	return ensureSchema(
		db,
		`
CREATE TABLE IF NOT EXISTS insight_normalized_batches (
  workspace_id TEXT PRIMARY KEY,
  hub_user_id TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT '',
  latest_batch_id TEXT NOT NULL DEFAULT '',
  generated_at TIMESTAMPTZ NOT NULL,
  latest_event_at TIMESTAMPTZ,
  content_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  topic_count INTEGER NOT NULL DEFAULT 0,
  account_count INTEGER NOT NULL DEFAULT 0
)`,
		`
CREATE TABLE IF NOT EXISTS insight_content_snapshots (
  workspace_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  batch_id TEXT NOT NULL DEFAULT '',
  event_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  collected_at TIMESTAMPTZ NOT NULL,
  published_at TIMESTAMPTZ,
  title TEXT NOT NULL DEFAULT '',
  channel_name TEXT NOT NULL DEFAULT '',
  channel_username TEXT NOT NULL DEFAULT '',
  cover_url TEXT NOT NULL DEFAULT '',
  object_id TEXT NOT NULL DEFAULT '',
  object_nonce_id TEXT NOT NULL DEFAULT '',
  keyword TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  prompt_angle TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  video_count INTEGER NOT NULL DEFAULT 0,
  play_count INTEGER NOT NULL DEFAULT 0,
  like_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  fav_count INTEGER NOT NULL DEFAULT 0,
  forward_count INTEGER NOT NULL DEFAULT 0,
  topic_count INTEGER NOT NULL DEFAULT 0,
  tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (workspace_id, snapshot_id)
)`,
		`
ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS play_count INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS like_count INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS fav_count INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS forward_count INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ
`,
		`
ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS channel_username TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS cover_url TEXT NOT NULL DEFAULT ''
`,
		`
CREATE TABLE IF NOT EXISTS insight_comment_snapshots (
  workspace_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  batch_id TEXT NOT NULL DEFAULT '',
  event_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  collected_at TIMESTAMPTZ NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  channel_name TEXT NOT NULL DEFAULT '',
  object_id TEXT NOT NULL DEFAULT '',
  object_nonce_id TEXT NOT NULL DEFAULT '',
  keyword TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  representative_comment TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  video_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  topic_count INTEGER NOT NULL DEFAULT 0,
  keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (workspace_id, snapshot_id)
)`,
		`
ALTER TABLE insight_comment_snapshots
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_comment_snapshots
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT ''
`,
		`
CREATE TABLE IF NOT EXISTS insight_topic_observations (
  workspace_id TEXT NOT NULL,
  observation_id TEXT NOT NULL,
  batch_id TEXT NOT NULL DEFAULT '',
  event_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  collected_at TIMESTAMPTZ NOT NULL,
  keyword TEXT NOT NULL DEFAULT '',
  cluster TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  prompt_angle TEXT NOT NULL DEFAULT '',
  object_id TEXT NOT NULL DEFAULT '',
  object_nonce_id TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  video_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  topic_count INTEGER NOT NULL DEFAULT 0,
  keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (workspace_id, observation_id)
)`,
		`
ALTER TABLE insight_topic_observations
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_topic_observations
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT ''
`,
		`
CREATE TABLE IF NOT EXISTS insight_account_observations (
  workspace_id TEXT NOT NULL,
  observation_id TEXT NOT NULL,
  batch_id TEXT NOT NULL DEFAULT '',
  event_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  collected_at TIMESTAMPTZ NOT NULL,
  channel_username TEXT NOT NULL DEFAULT '',
  channel_name TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  video_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  topic_count INTEGER NOT NULL DEFAULT 0,
  tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (workspace_id, observation_id)
)`,
		`
ALTER TABLE insight_account_observations
  ADD COLUMN IF NOT EXISTS channel_username TEXT NOT NULL DEFAULT ''
`,
	)
}

func EnsureAggregationSchema(db *sql.DB) error {
	return ensureSchema(
		db,
		`
CREATE TABLE IF NOT EXISTS insight_aggregate_batches (
  workspace_id TEXT PRIMARY KEY,
  hub_user_id TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT '',
  latest_batch_id TEXT NOT NULL DEFAULT '',
  generated_at TIMESTAMPTZ NOT NULL,
  latest_observed_at TIMESTAMPTZ,
  video_metric_count INTEGER NOT NULL DEFAULT 0,
  topic_metric_count INTEGER NOT NULL DEFAULT 0,
  comment_fact_count INTEGER NOT NULL DEFAULT 0,
  source_account_count INTEGER NOT NULL DEFAULT 0
)`,
		`
CREATE TABLE IF NOT EXISTS insight_video_metrics_hourly (
  workspace_id TEXT NOT NULL,
  metric_id TEXT NOT NULL,
  event_id TEXT NOT NULL DEFAULT '',
  batch_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  observed_at TIMESTAMPTZ NOT NULL,
  published_at TIMESTAMPTZ,
  object_id TEXT NOT NULL DEFAULT '',
  object_nonce_id TEXT NOT NULL DEFAULT '',
  cover_url TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  channel_name TEXT NOT NULL DEFAULT '',
  keyword TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  prompt_angle TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  views INTEGER NOT NULL DEFAULT 0,
  likes INTEGER NOT NULL DEFAULT 0,
  comments INTEGER NOT NULL DEFAULT 0,
  favorites INTEGER NOT NULL DEFAULT 0,
  forwards INTEGER NOT NULL DEFAULT 0,
  growth_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
  engagement_score INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (workspace_id, metric_id)
)`,
		`
ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS cover_url TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS favorites INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS forwards INTEGER NOT NULL DEFAULT 0
`,
		`
ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ
`,
		`
CREATE TABLE IF NOT EXISTS insight_topic_metrics_hourly (
  workspace_id TEXT NOT NULL,
  metric_id TEXT NOT NULL,
  event_id TEXT NOT NULL DEFAULT '',
  batch_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  observed_at TIMESTAMPTZ NOT NULL,
  keyword TEXT NOT NULL DEFAULT '',
  cluster TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  prompt_angle TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  mentions INTEGER NOT NULL DEFAULT 0,
  growth_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
  related_videos INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (workspace_id, metric_id)
)`,
		`
CREATE TABLE IF NOT EXISTS insight_comment_insight_facts (
  workspace_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  event_id TEXT NOT NULL DEFAULT '',
  batch_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  observed_at TIMESTAMPTZ NOT NULL,
  object_id TEXT NOT NULL DEFAULT '',
  object_nonce_id TEXT NOT NULL DEFAULT '',
  theme TEXT NOT NULL DEFAULT '',
  sentiment TEXT NOT NULL DEFAULT '',
  representative TEXT NOT NULL DEFAULT '',
  action_suggestion TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  related_keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  question_rate INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (workspace_id, fact_id)
)`,
		`
ALTER TABLE insight_comment_insight_facts
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT ''
`,
		`
ALTER TABLE insight_comment_insight_facts
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT ''
`,
	)
}
