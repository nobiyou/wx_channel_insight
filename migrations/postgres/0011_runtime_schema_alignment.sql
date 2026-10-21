ALTER TABLE insight_ingest_events
  ADD COLUMN IF NOT EXISTS channel_username TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_content_snapshots
  ADD COLUMN IF NOT EXISTS channel_username TEXT NOT NULL DEFAULT '';
