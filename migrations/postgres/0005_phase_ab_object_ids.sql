ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_comment_snapshots
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_comment_snapshots
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_topic_observations
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_topic_observations
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT '';
