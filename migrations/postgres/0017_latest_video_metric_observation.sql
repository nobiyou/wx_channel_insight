CREATE INDEX IF NOT EXISTS idx_insight_video_metric_latest
  ON insight_video_metric_observations (
    hub_user_id,
    (COALESCE(NULLIF(object_id, ''), NULLIF(object_nonce_id, ''), observation_id)),
    observed_at DESC,
    observation_id ASC
  );
