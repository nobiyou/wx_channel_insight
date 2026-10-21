CREATE TABLE IF NOT EXISTS insight_ai_service_settings (
  hub_user_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
