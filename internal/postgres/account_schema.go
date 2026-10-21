package postgres

import (
	"database/sql"
)

func EnsureAccountSchema(db *sql.DB) error {
	return ensureSchema(db, `
CREATE TABLE IF NOT EXISTS insight_accounts (
  account_id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  platform TEXT NOT NULL,
  channel_username TEXT NOT NULL,
  normalized_username TEXT NOT NULL,
  identity_status TEXT NOT NULL,
  channel_name TEXT NOT NULL DEFAULT '',
  signature TEXT NOT NULL DEFAULT '',
  avatar_url TEXT NOT NULL DEFAULT '',
  region TEXT NOT NULL DEFAULT '',
  industry TEXT NOT NULL DEFAULT '',
  tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  note TEXT NOT NULL DEFAULT '',
  is_priority BOOLEAN NOT NULL DEFAULT FALSE,
  profile_source TEXT NOT NULL DEFAULT 'upstream',
  profile_observed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, platform, normalized_username)
);
CREATE INDEX IF NOT EXISTS idx_insight_accounts_workspace_updated
  ON insight_accounts (workspace_id, updated_at DESC, account_id);
CREATE INDEX IF NOT EXISTS idx_insight_accounts_workspace_priority
  ON insight_accounts (workspace_id, is_priority, updated_at DESC);
`)
}
