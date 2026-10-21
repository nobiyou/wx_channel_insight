ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS play_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS like_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS fav_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS forward_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS favorites INTEGER NOT NULL DEFAULT 0;

ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS forwards INTEGER NOT NULL DEFAULT 0;

ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS play_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS like_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS fav_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS forward_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

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
  play_count INTEGER NOT NULL DEFAULT 0,
  like_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  fav_count INTEGER NOT NULL DEFAULT 0,
  forward_count INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (hub_user_id, observation_id)
);

ALTER TABLE insight_video_metric_observations
  ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;
