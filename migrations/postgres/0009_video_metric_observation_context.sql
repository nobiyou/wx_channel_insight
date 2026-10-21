ALTER TABLE insight_video_metric_observations
  ADD COLUMN IF NOT EXISTS channel_username TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_video_metric_observations
  ADD COLUMN IF NOT EXISTS keyword TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_video_metric_observations
  ADD COLUMN IF NOT EXISTS tags_json JSONB NOT NULL DEFAULT '[]'::jsonb;
