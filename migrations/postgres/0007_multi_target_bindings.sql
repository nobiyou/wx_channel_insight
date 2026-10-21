ALTER TABLE insight_ingest_source_bindings
  ADD COLUMN IF NOT EXISTS target_key TEXT NOT NULL DEFAULT '';

UPDATE insight_ingest_source_bindings
SET target_key = COALESCE(
  CASE
    WHEN source IN ('wx_feed_list', 'wx_channel_feed_list') THEN NULLIF(username, '')
    WHEN source IN ('wx_video_search', 'wx_channel_video_search') THEN NULLIF(keyword, '')
    WHEN source IN ('wx_comment_export', 'wx_channel_comment_export', 'finderGetCommentList', 'finderGetCommentList.partial') THEN NULLIF(path, '')
    ELSE NULLIF(source, '')
  END,
  source
)
WHERE target_key = '';

ALTER TABLE insight_ingest_source_bindings
  DROP CONSTRAINT IF EXISTS insight_ingest_source_bindings_pkey;

ALTER TABLE insight_ingest_source_bindings
  ADD CONSTRAINT insight_ingest_source_bindings_pkey PRIMARY KEY (hub_user_id, source, target_key);
