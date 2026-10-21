package postgres

import "database/sql"

func EnsurePromptSettingsSchema(db *sql.DB) error {
	return ensureSchema(db, `
CREATE TABLE IF NOT EXISTS insight_prompt_settings (
  hub_user_id TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`)
}
