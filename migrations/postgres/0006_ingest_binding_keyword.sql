ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS keyword TEXT NOT NULL DEFAULT '';
