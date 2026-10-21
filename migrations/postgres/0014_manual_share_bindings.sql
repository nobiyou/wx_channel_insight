ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS share_url TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT '';

ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS object_nonce_id TEXT NOT NULL DEFAULT '';
