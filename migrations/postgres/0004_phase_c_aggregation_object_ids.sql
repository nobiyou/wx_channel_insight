ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_comment_insight_facts
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_comment_insight_facts
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT '';
