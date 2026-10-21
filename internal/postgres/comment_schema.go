package postgres

import "database/sql"

func EnsureCommentSchema(db *sql.DB) error {
	return ensureSchema(db, `
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
CREATE INDEX IF NOT EXISTS idx_insight_comments_hub_activity
  ON insight_comment_records (hub_user_id, (COALESCE(created_at, collected_at)) DESC, collected_at DESC, video_object_id, comment_id);
CREATE INDEX IF NOT EXISTS idx_insight_comments_hub_video_activity
  ON insight_comment_records (hub_user_id, video_object_id, (COALESCE(created_at, collected_at)) DESC, collected_at DESC, comment_id);
CREATE INDEX IF NOT EXISTS idx_insight_comments_hub_account_activity
  ON insight_comment_records (hub_user_id, account_id, (COALESCE(created_at, collected_at)) DESC, collected_at DESC, comment_id);
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
`)
}
