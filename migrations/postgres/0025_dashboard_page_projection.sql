UPDATE insight_dashboard_snapshots AS dashboard
SET payload = (
  SELECT jsonb_object_agg(
    entry_key,
    CASE entry_key
      WHEN 'accountRecommendations' THEN jsonb_path_query_array(entry_value, '$[0 to 49]')
      WHEN 'videoRecommendations' THEN jsonb_path_query_array(entry_value, '$[0 to 49]')
      WHEN 'hotVideos' THEN jsonb_path_query_array(entry_value, '$[0 to 9]')
      WHEN 'hotTopics' THEN jsonb_path_query_array(entry_value, '$[0 to 4]')
      WHEN 'hotVideoRankings' THEN (
        SELECT COALESCE(
          jsonb_object_agg(ranking_key, jsonb_path_query_array(ranking_value, '$[0 to 9]')),
          '{}'::jsonb
        )
        FROM jsonb_each(
          CASE WHEN jsonb_typeof(entry_value) = 'object' THEN entry_value ELSE '{}'::jsonb END
        ) AS rankings(ranking_key, ranking_value)
      )
      WHEN 'hotVideoWindowRankings' THEN (
        SELECT COALESCE(
          jsonb_object_agg(window_key, (
            SELECT COALESCE(
              jsonb_object_agg(ranking_key, jsonb_path_query_array(ranking_value, '$[0 to 9]')),
              '{}'::jsonb
            )
            FROM jsonb_each(
              CASE WHEN jsonb_typeof(window_value) = 'object' THEN window_value ELSE '{}'::jsonb END
            ) AS rankings(ranking_key, ranking_value)
          )),
          '{}'::jsonb
        )
        FROM jsonb_each(
          CASE WHEN jsonb_typeof(entry_value) = 'object' THEN entry_value ELSE '{}'::jsonb END
        ) AS windows(window_key, window_value)
      )
      ELSE entry_value
    END
  )
  FROM jsonb_each(dashboard.payload) AS entries(entry_key, entry_value)
)
WHERE EXISTS (
  SELECT 1 FROM insight_recommendation_snapshots AS recommendations
  WHERE recommendations.snapshot_id = dashboard.snapshot_id
)
AND EXISTS (
  SELECT 1 FROM insight_hot_video_rankings AS video_rankings
  WHERE video_rankings.snapshot_id = dashboard.snapshot_id
)
AND EXISTS (
  SELECT 1 FROM insight_hot_topic_rankings AS topic_rankings
  WHERE topic_rankings.snapshot_id = dashboard.snapshot_id
)
AND (
  CASE WHEN jsonb_typeof(dashboard.payload->'accountRecommendations') = 'array'
    THEN jsonb_array_length(dashboard.payload->'accountRecommendations') > 50 ELSE false END
  OR CASE WHEN jsonb_typeof(dashboard.payload->'videoRecommendations') = 'array'
    THEN jsonb_array_length(dashboard.payload->'videoRecommendations') > 50 ELSE false END
  OR CASE WHEN jsonb_typeof(dashboard.payload->'hotVideos') = 'array'
    THEN jsonb_array_length(dashboard.payload->'hotVideos') > 10 ELSE false END
  OR CASE WHEN jsonb_typeof(dashboard.payload->'hotTopics') = 'array'
    THEN jsonb_array_length(dashboard.payload->'hotTopics') > 5 ELSE false END
  OR EXISTS (
    SELECT 1
    FROM jsonb_each(
      CASE WHEN jsonb_typeof(dashboard.payload->'hotVideoRankings') = 'object'
        THEN dashboard.payload->'hotVideoRankings' ELSE '{}'::jsonb END
    ) AS rankings(ranking_key, ranking_value)
    WHERE jsonb_typeof(ranking_value) = 'array' AND jsonb_array_length(ranking_value) > 10
  )
  OR EXISTS (
    SELECT 1
    FROM jsonb_each(
      CASE WHEN jsonb_typeof(dashboard.payload->'hotVideoWindowRankings') = 'object'
        THEN dashboard.payload->'hotVideoWindowRankings' ELSE '{}'::jsonb END
    ) AS windows(window_key, window_value)
    CROSS JOIN LATERAL jsonb_each(
      CASE WHEN jsonb_typeof(window_value) = 'object' THEN window_value ELSE '{}'::jsonb END
    ) AS rankings(ranking_key, ranking_value)
    WHERE jsonb_typeof(ranking_value) = 'array' AND jsonb_array_length(ranking_value) > 10
  )
);
