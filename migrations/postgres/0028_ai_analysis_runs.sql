CREATE TABLE IF NOT EXISTS insight_ai_analysis_runs (
  workspace_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  module TEXT NOT NULL,
  scope_key TEXT NOT NULL,
  data_window TEXT NOT NULL,
  input_fingerprint TEXT NOT NULL,
  status TEXT NOT NULL,
  result JSONB,
  model_provider TEXT NOT NULL DEFAULT '',
  model_name TEXT NOT NULL DEFAULT '',
  model_attempted BOOLEAN NOT NULL DEFAULT false,
  prompt_version TEXT NOT NULL DEFAULT '',
  config_owner_id TEXT NOT NULL DEFAULT '',
  config_source TEXT NOT NULL DEFAULT '',
  config_verification_status TEXT NOT NULL DEFAULT '',
  analysis_engine TEXT NOT NULL DEFAULT '',
  fallback_policy TEXT NOT NULL DEFAULT '',
  fallback_reason TEXT NOT NULL DEFAULT '',
  duration_ms BIGINT NOT NULL DEFAULT 0,
  run_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, run_id)
);

CREATE INDEX IF NOT EXISTS idx_insight_ai_analysis_runs_scope_time
  ON insight_ai_analysis_runs (
    workspace_id, module, scope_key, data_window, run_at DESC, run_id DESC
  );

CREATE INDEX IF NOT EXISTS idx_insight_ai_analysis_runs_workspace_time
  ON insight_ai_analysis_runs (workspace_id, run_at DESC, run_id DESC);
