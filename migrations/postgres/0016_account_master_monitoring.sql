CREATE TABLE IF NOT EXISTS insight_accounts (
  account_id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  platform TEXT NOT NULL,
  channel_username TEXT NOT NULL,
  normalized_username TEXT NOT NULL,
  identity_status TEXT NOT NULL,
  channel_name TEXT NOT NULL DEFAULT '',
  signature TEXT NOT NULL DEFAULT '',
  avatar_url TEXT NOT NULL DEFAULT '',
  region TEXT NOT NULL DEFAULT '',
  industry TEXT NOT NULL DEFAULT '',
  tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  note TEXT NOT NULL DEFAULT '',
  is_priority BOOLEAN NOT NULL DEFAULT FALSE,
  profile_source TEXT NOT NULL DEFAULT 'upstream',
  profile_observed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, platform, normalized_username)
);

CREATE INDEX IF NOT EXISTS idx_insight_accounts_workspace_updated
  ON insight_accounts (workspace_id, updated_at DESC, account_id);

CREATE INDEX IF NOT EXISTS idx_insight_accounts_workspace_priority
  ON insight_accounts (workspace_id, is_priority, updated_at DESC);

CREATE TABLE IF NOT EXISTS insight_comment_records (
  hub_user_id TEXT NOT NULL,
  comment_id TEXT NOT NULL,
  video_object_id TEXT NOT NULL,
  video_object_nonce_id TEXT NOT NULL DEFAULT '',
  account_id TEXT NOT NULL DEFAULT '',
  video_title TEXT NOT NULL DEFAULT '',
  channel_name TEXT NOT NULL DEFAULT '',
  channel_username TEXT NOT NULL DEFAULT '',
  parent_comment_id TEXT NOT NULL DEFAULT '',
  author_username TEXT NOT NULL DEFAULT '',
  author_nickname TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  ip_region TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ,
  like_count INTEGER NOT NULL DEFAULT 0,
  reply_count INTEGER NOT NULL DEFAULT 0,
  collected_at TIMESTAMPTZ NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  raw_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY (hub_user_id, video_object_id, comment_id)
);

CREATE INDEX IF NOT EXISTS idx_insight_comments_hub_collected
  ON insight_comment_records (hub_user_id, collected_at DESC, comment_id);

CREATE INDEX IF NOT EXISTS idx_insight_comments_hub_video
  ON insight_comment_records (hub_user_id, video_object_id, collected_at DESC);

CREATE TABLE IF NOT EXISTS insight_video_comment_sync (
  hub_user_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  video_object_id TEXT NOT NULL,
  video_object_nonce_id TEXT NOT NULL DEFAULT '',
  last_comment_count INTEGER NOT NULL DEFAULT 0,
  last_observed_at TIMESTAMPTZ,
  last_collected_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (hub_user_id, account_id, video_object_id)
);

ALTER TABLE insight_monitoring_task_targets
  ADD COLUMN IF NOT EXISTS target_kind TEXT NOT NULL DEFAULT 'binding';
ALTER TABLE insight_monitoring_task_targets
  ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT '';
ALTER TABLE insight_monitoring_task_targets
  ADD COLUMN IF NOT EXISTS comment_mode TEXT NOT NULL DEFAULT 'incremental';

CREATE INDEX IF NOT EXISTS idx_insight_monitoring_targets_account
  ON insight_monitoring_task_targets (hub_user_id, account_id)
  WHERE target_kind = 'account' AND account_id <> '';
