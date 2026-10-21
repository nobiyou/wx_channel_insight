CREATE TABLE IF NOT EXISTS insight_monitoring_tasks (
  hub_user_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  scope_mode TEXT NOT NULL,
  interval_seconds BIGINT NOT NULL,
  next_run_at TIMESTAMPTZ NOT NULL,
  last_run_at TIMESTAMPTZ,
  last_success_at TIMESTAMPTZ,
  last_failure_at TIMESTAMPTZ,
  last_status TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  system_task BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  archived_at TIMESTAMPTZ,
  PRIMARY KEY (hub_user_id, task_id),
  CONSTRAINT insight_monitoring_tasks_status_check
    CHECK (status IN ('enabled', 'paused', 'archived')),
  CONSTRAINT insight_monitoring_tasks_scope_check
    CHECK (scope_mode IN ('all_saved_bindings', 'selected')),
  CONSTRAINT insight_monitoring_tasks_interval_check
    CHECK (interval_seconds >= 60 AND interval_seconds <= 604800)
);

CREATE UNIQUE INDEX IF NOT EXISTS insight_monitoring_system_task_unique
  ON insight_monitoring_tasks (hub_user_id)
  WHERE system_task = TRUE;

CREATE INDEX IF NOT EXISTS insight_monitoring_tasks_due_idx
  ON insight_monitoring_tasks (hub_user_id, status, next_run_at)
  WHERE status = 'enabled';

CREATE TABLE IF NOT EXISTS insight_monitoring_task_targets (
  hub_user_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  source TEXT NOT NULL,
  target_key TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (hub_user_id, task_id, source, target_key),
  CONSTRAINT insight_monitoring_task_targets_task_fk
    FOREIGN KEY (hub_user_id, task_id)
    REFERENCES insight_monitoring_tasks (hub_user_id, task_id)
);

CREATE INDEX IF NOT EXISTS insight_monitoring_task_targets_task_idx
  ON insight_monitoring_task_targets (hub_user_id, task_id);

CREATE TABLE IF NOT EXISTS insight_monitoring_task_runs (
  hub_user_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  trigger TEXT NOT NULL,
  status TEXT NOT NULL,
  attempt INTEGER NOT NULL DEFAULT 1,
  started_at TIMESTAMPTZ NOT NULL,
  finished_at TIMESTAMPTZ,
  lease_until TIMESTAMPTZ,
  operator_run_id TEXT NOT NULL DEFAULT '',
  attempted_count INTEGER NOT NULL DEFAULT 0,
  succeeded_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  skipped_count INTEGER NOT NULL DEFAULT 0,
  message TEXT NOT NULL DEFAULT '',
  errors_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (hub_user_id, run_id),
  CONSTRAINT insight_monitoring_task_runs_task_fk
    FOREIGN KEY (hub_user_id, task_id)
    REFERENCES insight_monitoring_tasks (hub_user_id, task_id),
  CONSTRAINT insight_monitoring_task_runs_trigger_check
    CHECK (trigger IN ('scheduled', 'manual', 'retry')),
  CONSTRAINT insight_monitoring_task_runs_status_check
    CHECK (status IN ('running', 'success', 'partial', 'failed', 'skipped', 'abandoned')),
  CONSTRAINT insight_monitoring_task_runs_attempt_check
    CHECK (attempt >= 1)
);

CREATE INDEX IF NOT EXISTS insight_monitoring_task_runs_task_idx
  ON insight_monitoring_task_runs (hub_user_id, task_id, started_at DESC);

CREATE INDEX IF NOT EXISTS insight_monitoring_task_runs_lease_idx
  ON insight_monitoring_task_runs (hub_user_id, status, lease_until)
  WHERE status = 'running';
