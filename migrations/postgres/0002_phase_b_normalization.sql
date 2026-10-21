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
);

CREATE TABLE IF NOT EXISTS insight_content_snapshots (
  workspace_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  batch_id TEXT NOT NULL DEFAULT '',
  event_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  collected_at TIMESTAMPTZ NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  channel_name TEXT NOT NULL DEFAULT '',
  keyword TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  prompt_angle TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  video_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  topic_count INTEGER NOT NULL DEFAULT 0,
  tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (workspace_id, snapshot_id)
);

CREATE TABLE IF NOT EXISTS insight_comment_snapshots (
  workspace_id TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  batch_id TEXT NOT NULL DEFAULT '',
  event_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  collected_at TIMESTAMPTZ NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  channel_name TEXT NOT NULL DEFAULT '',
  keyword TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  representative_comment TEXT NOT NULL DEFAULT '',
  target_audience TEXT NOT NULL DEFAULT '',
  video_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  topic_count INTEGER NOT NULL DEFAULT 0,
  keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (workspace_id, snapshot_id)
);

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
  target_audience TEXT NOT NULL DEFAULT '',
  video_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  topic_count INTEGER NOT NULL DEFAULT 0,
  keywords_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (workspace_id, observation_id)
);

CREATE TABLE IF NOT EXISTS insight_account_observations (
  workspace_id TEXT NOT NULL,
  observation_id TEXT NOT NULL,
  batch_id TEXT NOT NULL DEFAULT '',
  event_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  collected_at TIMESTAMPTZ NOT NULL,
  channel_name TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  signal TEXT NOT NULL DEFAULT '',
  video_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  topic_count INTEGER NOT NULL DEFAULT 0,
  tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (workspace_id, observation_id)
);
