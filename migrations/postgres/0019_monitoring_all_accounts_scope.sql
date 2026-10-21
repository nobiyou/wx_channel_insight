ALTER TABLE IF EXISTS insight_monitoring_tasks
  DROP CONSTRAINT IF EXISTS insight_monitoring_tasks_scope_check;

ALTER TABLE IF EXISTS insight_monitoring_tasks
  ADD CONSTRAINT insight_monitoring_tasks_scope_check
  CHECK (scope_mode IN ('all_saved_bindings', 'all_accounts', 'selected'));
