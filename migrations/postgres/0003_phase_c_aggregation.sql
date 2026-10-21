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
);

CREATE TABLE IF NOT EXISTS insight_video_metrics_hourly (
  workspace_id TEXT NOT NULL,
  metric_id TEXT NOT NULL,
  event_id TEXT NOT NULL DEFAULT '',
  batch_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  observed_at TIMESTAMPTZ NOT NULL,
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
  growth_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
  engagement_score INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (workspace_id, metric_id)
);

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
);

CREATE TABLE IF NOT EXISTS insight_comment_insight_facts (
  workspace_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  event_id TEXT NOT NULL DEFAULT '',
  batch_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  observed_at TIMESTAMPTZ NOT NULL,
  theme TEXT NOT NULL DEFAULT '',
  sentiment TEXT NOT NULL DEFAULT '',
  representative TEXT NOT NULL DEFAULT '',
  action_suggestion TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  related_keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  question_rate INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (workspace_id, fact_id)
);
