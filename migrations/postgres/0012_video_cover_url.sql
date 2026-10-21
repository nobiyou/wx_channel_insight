ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS cover_url TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_video_metric_observations
  ADD COLUMN IF NOT EXISTS cover_url TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS cover_url TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_video_metrics_hourly
  ADD COLUMN IF NOT EXISTS cover_url TEXT NOT NULL DEFAULT '';
