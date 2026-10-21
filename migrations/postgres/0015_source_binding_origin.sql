ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS origin TEXT NOT NULL DEFAULT 'manual';
