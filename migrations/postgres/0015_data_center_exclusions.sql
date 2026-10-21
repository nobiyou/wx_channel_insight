CREATE TABLE IF NOT EXISTS insight_data_center_exclusions (
  workspace_id TEXT NOT NULL,
  entity TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  deleted_at TIMESTAMPTZ NOT NULL,
  deleted_by TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (workspace_id, entity, entity_id)
);
