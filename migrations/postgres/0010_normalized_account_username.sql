ALTER TABLE insight_account_observations
  ADD COLUMN IF NOT EXISTS channel_username TEXT NOT NULL DEFAULT '';
