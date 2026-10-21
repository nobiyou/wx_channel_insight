CREATE TABLE IF NOT EXISTS insight_recommendation_snapshots (
  snapshot_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
