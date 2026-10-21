CREATE INDEX IF NOT EXISTS idx_insight_comments_hub_activity
  ON insight_comment_records (hub_user_id, (COALESCE(created_at, collected_at)) DESC, collected_at DESC, video_object_id, comment_id);

CREATE INDEX IF NOT EXISTS idx_insight_comments_hub_video_activity
  ON insight_comment_records (hub_user_id, video_object_id, (COALESCE(created_at, collected_at)) DESC, collected_at DESC, comment_id);

CREATE INDEX IF NOT EXISTS idx_insight_comments_hub_account_activity
  ON insight_comment_records (hub_user_id, account_id, (COALESCE(created_at, collected_at)) DESC, collected_at DESC, comment_id);
