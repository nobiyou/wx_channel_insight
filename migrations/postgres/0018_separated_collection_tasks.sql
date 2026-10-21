ALTER TABLE insight_monitoring_tasks
  ADD COLUMN IF NOT EXISTS task_type TEXT NOT NULL DEFAULT 'legacy';

ALTER TABLE insight_monitoring_tasks
  DROP CONSTRAINT IF EXISTS insight_monitoring_tasks_type_check;

ALTER TABLE insight_monitoring_tasks
  ADD CONSTRAINT insight_monitoring_tasks_type_check
  CHECK (task_type IN ('legacy', 'account_profile', 'video_list', 'comment_collection'));
