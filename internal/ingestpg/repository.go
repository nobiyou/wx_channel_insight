package ingestpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type Repository struct {
	client *postgres.Client
	db     *sql.DB
}

var _ ingest.Repository = (*Repository)(nil)
var _ ingest.AccountMetricProvider = (*Repository)(nil)
var _ ingest.VideoHistoryProvider = (*Repository)(nil)
var _ ingest.EventSummaryFactsProvider = (*Repository)(nil)
var _ ingest.SummaryRunsProvider = (*Repository)(nil)

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEvent(scanner rowScanner, extraDest ...any) (ingest.Event, error) {
	var event ingest.Event
	var publishedAt sql.NullTime
	var tagsJSON, keywordsJSON []byte
	dest := []any{
		&event.ID,
		&event.BatchID,
		&event.HubUserID,
		&event.MachineID,
		&event.EventType,
		&event.Source,
		&event.CollectedAt,
		&publishedAt,
		&event.ObjectID,
		&event.ObjectNonceID,
		&event.CoverURL,
		&event.VideoCount,
		&event.PlayCount,
		&event.LikeCount,
		&event.CommentCount,
		&event.FavCount,
		&event.ForwardCount,
		&event.TopicCount,
		&event.Title,
		&event.ChannelName,
		&event.ChannelUsername,
		&event.Keyword,
		&event.Cluster,
		&event.Signal,
		&event.PromptAngle,
		&event.RepresentativeComment,
		&event.TargetAudience,
		&tagsJSON,
		&keywordsJSON,
		&event.MissingMetrics,
	}
	dest = append(dest, extraDest...)
	if err := scanner.Scan(dest...); err != nil {
		return ingest.Event{}, err
	}
	if publishedAt.Valid {
		event.PublishedAt = publishedAt.Time
	}
	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &event.Tags); err != nil {
			return ingest.Event{}, err
		}
	}
	if len(keywordsJSON) > 0 {
		if err := json.Unmarshal(keywordsJSON, &event.Keywords); err != nil {
			return ingest.Event{}, err
		}
	}
	return event, nil
}

func scanRun(scanner rowScanner) (ingest.Run, error) {
	var run ingest.Run
	var latestEventAt sql.NullTime
	var collectionJSON []byte
	if err := scanner.Scan(
		&run.ID,
		&run.Source,
		&run.Status,
		&run.StartedAt,
		&run.FinishedAt,
		&run.BatchID,
		&run.EventCount,
		&latestEventAt,
		&run.LastEventType,
		&run.Message, &collectionJSON,
	); err != nil {
		return ingest.Run{}, err
	}
	if latestEventAt.Valid {
		run.LatestEventAt = latestEventAt.Time
	}
	if len(collectionJSON) > 0 && string(collectionJSON) != "null" {
		if err := json.Unmarshal(collectionJSON, &run.Collection); err != nil {
			return ingest.Run{}, err
		}
	}
	return run, nil
}

func NewRepository(client *postgres.Client) (*Repository, error) {
	if client == nil {
		return nil, errors.New("postgres client is required")
	}
	db := client.DB()
	if err := postgres.EnsureIngestSchema(db); err != nil {
		return nil, err
	}
	return &Repository{client: client, db: db}, nil
}

func (r *Repository) ListEvents(ctx context.Context, hubUserID string) ([]ingest.Event, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return nil, errors.New("ingest hub user id is required")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT event_id, batch_id, hub_user_id, machine_id, event_type, source, collected_at,
       published_at, object_id, object_nonce_id, cover_url, video_count, play_count, like_count, comment_count, fav_count, forward_count, topic_count, title, channel_name, channel_username, keyword, cluster, signal,
       prompt_angle, representative_comment, target_audience, tags_json, keywords_json, missing_metrics
FROM insight_ingest_events
WHERE hub_user_id = $1
`, hubUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []ingest.Event
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *Repository) GetEventSummaryFacts(ctx context.Context, hubUserID string, recentLimit int) (ingest.EventSummaryFacts, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return ingest.EventSummaryFacts{}, errors.New("ingest hub user id is required")
	}
	if recentLimit <= 0 {
		recentLimit = 9
	}

	var totalEvents, sourceCount, videos, comments, topics int64
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*),
       COUNT(DISTINCT NULLIF(machine_id, '')),
       COALESCE(SUM(video_count), 0),
       COALESCE(SUM(comment_count), 0),
       COALESCE(SUM(topic_count), 0)
FROM insight_ingest_events
WHERE hub_user_id = $1
`, hubUserID).Scan(&totalEvents, &sourceCount, &videos, &comments, &topics); err != nil {
		return ingest.EventSummaryFacts{}, err
	}
	facts := ingest.EventSummaryFacts{
		TotalEvents:        int(totalEvents),
		SourceCount:        int(sourceCount),
		AggregatedVideos:   int(videos),
		AggregatedComments: int(comments),
		AggregatedTopics:   int(topics),
	}
	if totalEvents == 0 {
		return facts, nil
	}

	recentRows, err := r.db.QueryContext(ctx, `
SELECT event_id, batch_id, hub_user_id, machine_id, event_type, source, collected_at,
       published_at, object_id, object_nonce_id, cover_url, video_count, play_count, like_count, comment_count, fav_count, forward_count, topic_count, title, channel_name, channel_username, keyword, cluster, signal,
       prompt_angle, representative_comment, target_audience, tags_json, keywords_json, missing_metrics
FROM insight_ingest_events
WHERE hub_user_id = $1
ORDER BY collected_at DESC, batch_id DESC, event_id DESC
LIMIT $2
`, hubUserID, recentLimit)
	if err != nil {
		return ingest.EventSummaryFacts{}, err
	}
	for recentRows.Next() {
		event, scanErr := scanEvent(recentRows)
		if scanErr != nil {
			recentRows.Close()
			return ingest.EventSummaryFacts{}, scanErr
		}
		facts.RecentEvents = append(facts.RecentEvents, event)
	}
	if err := recentRows.Err(); err != nil {
		recentRows.Close()
		return ingest.EventSummaryFacts{}, err
	}
	recentRows.Close()
	if len(facts.RecentEvents) > 0 {
		facts.LatestEvent = facts.RecentEvents[0]
	}

	sourceRows, err := r.db.QueryContext(ctx, `
WITH ranked AS (
  SELECT event_id, batch_id, hub_user_id, machine_id, event_type, source, collected_at,
         published_at, object_id, object_nonce_id, cover_url, video_count, play_count, like_count, comment_count, fav_count, forward_count, topic_count, title, channel_name, channel_username, keyword, cluster, signal,
         prompt_angle, representative_comment, target_audience, tags_json, keywords_json, missing_metrics,
         COUNT(*) OVER (PARTITION BY source) AS source_event_count,
         ROW_NUMBER() OVER (PARTITION BY source ORDER BY collected_at DESC, batch_id DESC, event_id DESC) AS source_rank
  FROM insight_ingest_events
  WHERE hub_user_id = $1
)
SELECT event_id, batch_id, hub_user_id, machine_id, event_type, source, collected_at,
       published_at, object_id, object_nonce_id, cover_url, video_count, play_count, like_count, comment_count, fav_count, forward_count, topic_count, title, channel_name, channel_username, keyword, cluster, signal,
       prompt_angle, representative_comment, target_audience, tags_json, keywords_json, missing_metrics, source_event_count
FROM ranked
WHERE source_rank = 1
ORDER BY source
`, hubUserID)
	if err != nil {
		return ingest.EventSummaryFacts{}, err
	}
	defer sourceRows.Close()
	for sourceRows.Next() {
		var eventCount int64
		event, scanErr := scanEvent(sourceRows, &eventCount)
		if scanErr != nil {
			return ingest.EventSummaryFacts{}, scanErr
		}
		facts.Sources = append(facts.Sources, ingest.EventSourceFacts{
			Source:      event.Source,
			EventCount:  int(eventCount),
			LatestEvent: event,
		})
	}
	if err := sourceRows.Err(); err != nil {
		return ingest.EventSummaryFacts{}, err
	}
	return facts, nil
}

func (r *Repository) AppendEvents(ctx context.Context, hubUserID string, events []ingest.Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return errors.New("ingest hub user id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, event := range events {
		if event.HubUserID == "" {
			event.HubUserID = hubUserID
		}
		if err = upsertEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ReplaceEvents(ctx context.Context, hubUserID string, events []ingest.Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return errors.New("ingest hub user id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `DELETE FROM insight_ingest_events WHERE hub_user_id = $1`, hubUserID); err != nil {
		return err
	}
	for _, event := range events {
		if event.HubUserID == "" {
			event.HubUserID = hubUserID
		}
		if err = upsertEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListVideoMetricObservations(ctx context.Context, hubUserID string) ([]ingest.VideoMetricObservation, error) {
	return r.listVideoMetricObservations(ctx, hubUserID, false)
}

func (r *Repository) ListVideoHistory(ctx context.Context, hubUserID, objectID string) ([]ingest.Event, []ingest.VideoMetricObservation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	hubUserID = strings.TrimSpace(hubUserID)
	objectID = strings.TrimSpace(objectID)
	if hubUserID == "" || objectID == "" {
		return []ingest.Event{}, []ingest.VideoMetricObservation{}, nil
	}
	events, err := r.listVideoHistoryEvents(ctx, hubUserID, objectID)
	if err != nil {
		return nil, nil, err
	}
	observations, err := r.listVideoHistoryObservations(ctx, hubUserID, objectID)
	if err != nil {
		return nil, nil, err
	}
	return events, observations, nil
}

func (r *Repository) listVideoHistoryEvents(ctx context.Context, hubUserID, objectID string) ([]ingest.Event, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT event_id, batch_id, hub_user_id, machine_id, event_type, source, collected_at,
       published_at, object_id, object_nonce_id, cover_url, video_count, play_count, like_count, comment_count, fav_count, forward_count, topic_count, title, channel_name, channel_username, keyword, cluster, signal,
       prompt_angle, representative_comment, target_audience, tags_json, keywords_json, missing_metrics
FROM insight_ingest_events
WHERE hub_user_id = $1 AND event_type = 'video_snapshot'
  AND (object_id = $2 OR object_nonce_id = $2 OR event_id = $2)
ORDER BY collected_at DESC, event_id DESC`, hubUserID, objectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ingest.Event{}
	for rows.Next() {
		var event ingest.Event
		var publishedAt sql.NullTime
		var tagsJSON, keywordsJSON []byte
		if err := rows.Scan(
			&event.ID, &event.BatchID, &event.HubUserID, &event.MachineID, &event.EventType, &event.Source, &event.CollectedAt,
			&publishedAt, &event.ObjectID, &event.ObjectNonceID, &event.CoverURL, &event.VideoCount, &event.PlayCount,
			&event.LikeCount, &event.CommentCount, &event.FavCount, &event.ForwardCount, &event.TopicCount, &event.Title,
			&event.ChannelName, &event.ChannelUsername, &event.Keyword, &event.Cluster, &event.Signal, &event.PromptAngle,
			&event.RepresentativeComment, &event.TargetAudience, &tagsJSON, &keywordsJSON,
			&event.MissingMetrics,
		); err != nil {
			return nil, err
		}
		if publishedAt.Valid {
			event.PublishedAt = publishedAt.Time
		}
		if len(tagsJSON) > 0 {
			if err := json.Unmarshal(tagsJSON, &event.Tags); err != nil {
				return nil, err
			}
		}
		if len(keywordsJSON) > 0 {
			if err := json.Unmarshal(keywordsJSON, &event.Keywords); err != nil {
				return nil, err
			}
		}
		items = append(items, event)
	}
	return items, rows.Err()
}

func (r *Repository) listVideoHistoryObservations(ctx context.Context, hubUserID, objectID string) ([]ingest.VideoMetricObservation, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT observation_id, hub_user_id, event_id, batch_id, source, observed_at,
       published_at, object_id, object_nonce_id, title, channel_name, channel_username, cover_url, keyword, tags_json, play_count, like_count, comment_count, fav_count, forward_count, missing_metrics
FROM insight_video_metric_observations
WHERE hub_user_id = $1 AND (object_id = $2 OR object_nonce_id = $2 OR observation_id = $2)
ORDER BY observed_at DESC, observation_id ASC`, hubUserID, objectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ingest.VideoMetricObservation{}
	for rows.Next() {
		var item ingest.VideoMetricObservation
		var publishedAt sql.NullTime
		var tagsJSON []byte
		if err := rows.Scan(
			&item.ID, &item.HubUserID, &item.EventID, &item.BatchID, &item.Source, &item.ObservedAt, &publishedAt,
			&item.ObjectID, &item.ObjectNonceID, &item.Title, &item.ChannelName, &item.ChannelUsername, &item.CoverURL,
			&item.Keyword, &tagsJSON, &item.PlayCount, &item.LikeCount, &item.CommentCount, &item.FavCount, &item.ForwardCount, &item.MissingMetrics,
		); err != nil {
			return nil, err
		}
		if publishedAt.Valid {
			item.PublishedAt = publishedAt.Time
		}
		if len(tagsJSON) > 0 {
			if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
				return nil, err
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListAccountMetricRows(ctx context.Context, hubUserID string, channelUsernames []string) ([]ingest.AccountMetricRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	hubUserID = strings.TrimSpace(hubUserID)
	usernames := make([]string, 0, len(channelUsernames))
	seen := map[string]struct{}{}
	for _, username := range channelUsernames {
		username = strings.TrimSpace(username)
		key := strings.ToLower(username)
		if username == "" || key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		usernames = append(usernames, username)
	}
	if hubUserID == "" || len(usernames) == 0 {
		return []ingest.AccountMetricRow{}, nil
	}
	args := []any{hubUserID}
	placeholders := make([]string, 0, len(usernames))
	for _, username := range usernames {
		args = append(args, username)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
WITH video_candidates AS (
  SELECT
    COALESCE(NULLIF(object_id, ''), NULLIF(object_nonce_id, ''), event_id) AS video_key,
    channel_username, channel_name, collected_at AS observed_at,
    like_count, comment_count, fav_count, forward_count, missing_metrics
  FROM insight_ingest_events
  WHERE hub_user_id = $1 AND event_type = 'video_snapshot'
    AND channel_username IN (%s)
  UNION ALL
  SELECT
    COALESCE(NULLIF(object_id, ''), NULLIF(object_nonce_id, ''), observation_id) AS video_key,
    channel_username, channel_name, observed_at,
    like_count, comment_count, fav_count, forward_count, missing_metrics
  FROM insight_video_metric_observations
  WHERE hub_user_id = $1 AND channel_username IN (%s)
), latest_videos AS (
  SELECT DISTINCT ON (video_key)
    channel_username, channel_name, observed_at,
    like_count, comment_count, fav_count, forward_count, missing_metrics
  FROM video_candidates
  ORDER BY video_key, observed_at DESC, channel_username
)
SELECT channel_username, channel_name, COUNT(*)::int,
       COALESCE(SUM(like_count), 0)::int,
       COALESCE(SUM(comment_count), 0)::int,
       COALESCE(SUM(fav_count), 0)::int,
       COALESCE(SUM(forward_count), 0)::int,
       MAX(observed_at), BIT_OR(missing_metrics)::int
FROM latest_videos
GROUP BY channel_username, channel_name
ORDER BY MAX(observed_at) DESC, channel_username`, strings.Join(placeholders, ","), strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ingest.AccountMetricRow{}
	for rows.Next() {
		var item ingest.AccountMetricRow
		if err := rows.Scan(&item.ChannelUsername, &item.ChannelName, &item.VideoCount, &item.Likes, &item.Comments, &item.Favorites, &item.Forwards, &item.RecentObservedAt, &item.MissingMetrics); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListLatestVideoMetricObservations(ctx context.Context, hubUserID string) ([]ingest.VideoMetricObservation, error) {
	return r.listVideoMetricObservations(ctx, hubUserID, true)
}

func (r *Repository) listVideoMetricObservations(ctx context.Context, hubUserID string, latestOnly bool) ([]ingest.VideoMetricObservation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return nil, errors.New("ingest hub user id is required")
	}
	query := `
SELECT observation_id, hub_user_id, event_id, batch_id, source, observed_at,
       published_at, object_id, object_nonce_id, title, channel_name, channel_username, cover_url, keyword, tags_json, play_count, like_count, comment_count, fav_count, forward_count, missing_metrics
FROM insight_video_metric_observations
WHERE hub_user_id = $1
ORDER BY observed_at DESC, observation_id ASC
`
	if latestOnly {
		query = `
SELECT observation_id, hub_user_id, event_id, batch_id, source, observed_at,
       published_at, object_id, object_nonce_id, title, channel_name, channel_username, cover_url, keyword, tags_json, play_count, like_count, comment_count, fav_count, forward_count, missing_metrics
FROM (
  SELECT DISTINCT ON (COALESCE(NULLIF(object_id, ''), NULLIF(object_nonce_id, ''), observation_id))
         observation_id, hub_user_id, event_id, batch_id, source, observed_at,
         published_at, object_id, object_nonce_id, title, channel_name, channel_username, cover_url, keyword, tags_json, play_count, like_count, comment_count, fav_count, forward_count, missing_metrics
  FROM insight_video_metric_observations
  WHERE hub_user_id = $1
  ORDER BY COALESCE(NULLIF(object_id, ''), NULLIF(object_nonce_id, ''), observation_id), observed_at DESC, observation_id ASC
) latest
ORDER BY observed_at DESC, observation_id ASC
`
	}
	rows, err := r.db.QueryContext(ctx, query, hubUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	observations := []ingest.VideoMetricObservation{}
	for rows.Next() {
		var item ingest.VideoMetricObservation
		var publishedAt sql.NullTime
		var tagsJSON []byte
		if err := rows.Scan(
			&item.ID,
			&item.HubUserID,
			&item.EventID,
			&item.BatchID,
			&item.Source,
			&item.ObservedAt,
			&publishedAt,
			&item.ObjectID,
			&item.ObjectNonceID,
			&item.Title,
			&item.ChannelName,
			&item.ChannelUsername,
			&item.CoverURL,
			&item.Keyword,
			&tagsJSON,
			&item.PlayCount,
			&item.LikeCount,
			&item.CommentCount,
			&item.FavCount,
			&item.ForwardCount, &item.MissingMetrics,
		); err != nil {
			return nil, err
		}
		if publishedAt.Valid {
			item.PublishedAt = publishedAt.Time
		}
		if len(tagsJSON) > 0 {
			if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
				return nil, err
			}
		}
		observations = append(observations, item)
	}
	return observations, rows.Err()
}

func (r *Repository) AppendVideoMetricObservations(ctx context.Context, hubUserID string, observations []ingest.VideoMetricObservation) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return errors.New("ingest hub user id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, item := range observations {
		if item.HubUserID == "" {
			item.HubUserID = hubUserID
		}
		if item.ID == "" {
			return errors.New("video metric observation id is required")
		}
		if item.HubUserID != hubUserID {
			return errors.New("video metric observation hub user id mismatch")
		}
		var publishedAt any
		if !item.PublishedAt.IsZero() {
			publishedAt = item.PublishedAt
		}
		tagsJSON, marshalErr := json.Marshal(item.Tags)
		if marshalErr != nil {
			err = marshalErr
			return err
		}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO insight_video_metric_observations (
  hub_user_id, observation_id, event_id, batch_id, source, observed_at,
  published_at, object_id, object_nonce_id, title, channel_name, channel_username, cover_url, keyword, tags_json, play_count, like_count, comment_count, fav_count, forward_count, missing_metrics
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15::jsonb, $16, $17, $18, $19, $20, $21)
ON CONFLICT (hub_user_id, observation_id) DO UPDATE SET
  event_id = EXCLUDED.event_id,
  batch_id = EXCLUDED.batch_id,
  source = EXCLUDED.source,
  observed_at = EXCLUDED.observed_at,
  published_at = EXCLUDED.published_at,
  object_id = EXCLUDED.object_id,
  object_nonce_id = EXCLUDED.object_nonce_id,
  title = EXCLUDED.title,
  channel_name = EXCLUDED.channel_name,
  channel_username = EXCLUDED.channel_username,
  cover_url = EXCLUDED.cover_url,
  keyword = EXCLUDED.keyword,
  tags_json = EXCLUDED.tags_json,
  play_count = EXCLUDED.play_count,
  like_count = EXCLUDED.like_count,
  comment_count = EXCLUDED.comment_count,
  fav_count = EXCLUDED.fav_count,
  forward_count = EXCLUDED.forward_count,
 missing_metrics = EXCLUDED.missing_metrics
		`, item.HubUserID, item.ID, item.EventID, item.BatchID, item.Source, item.ObservedAt, publishedAt,
			item.ObjectID, item.ObjectNonceID, item.Title, item.ChannelName, item.ChannelUsername, item.CoverURL, item.Keyword, string(tagsJSON), item.PlayCount, item.LikeCount,
			item.CommentCount, item.FavCount, item.ForwardCount, int(item.MissingMetrics)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListRuns(ctx context.Context, hubUserID string) ([]ingest.Run, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return nil, errors.New("ingest hub user id is required")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT run_id, source, status, started_at, finished_at, batch_id, event_count, latest_event_at, last_event_type, message, collection_json
FROM insight_ingest_runs
WHERE hub_user_id = $1
ORDER BY finished_at DESC, run_id DESC
`, hubUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []ingest.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *Repository) ListSummaryRuns(ctx context.Context, hubUserID string, recentLimit int) ([]ingest.Run, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return nil, errors.New("ingest hub user id is required")
	}
	if recentLimit <= 0 {
		recentLimit = 6
	}
	rows, err := r.db.QueryContext(ctx, `
WITH sources AS MATERIALIZED (
  SELECT DISTINCT source
  FROM insight_ingest_runs
  WHERE hub_user_id = $1
), candidates AS (
  (
    SELECT run_id, source, status, started_at, finished_at, batch_id, event_count, latest_event_at, last_event_type, message, collection_json
    FROM insight_ingest_runs
    WHERE hub_user_id = $1
    ORDER BY finished_at DESC, run_id DESC
    LIMIT $2
  )
  UNION
  (
    SELECT latest.run_id, latest.source, latest.status, latest.started_at, latest.finished_at,
           latest.batch_id, latest.event_count, latest.latest_event_at, latest.last_event_type, latest.message, latest.collection_json
    FROM sources
    CROSS JOIN LATERAL (
      SELECT run_id, source, status, started_at, finished_at, batch_id, event_count, latest_event_at, last_event_type, message, collection_json
      FROM insight_ingest_runs
      WHERE hub_user_id = $1 AND source = sources.source
      ORDER BY finished_at DESC, run_id DESC
      LIMIT 1
    ) latest
  )
  UNION
  (
    SELECT latest.run_id, latest.source, latest.status, latest.started_at, latest.finished_at,
           latest.batch_id, latest.event_count, latest.latest_event_at, latest.last_event_type, latest.message, latest.collection_json
    FROM sources
    CROSS JOIN (VALUES ('success'), ('failed')) AS wanted(status)
    CROSS JOIN LATERAL (
      SELECT run_id, source, status, started_at, finished_at, batch_id, event_count, latest_event_at, last_event_type, message, collection_json
      FROM insight_ingest_runs
      WHERE hub_user_id = $1 AND source = sources.source AND status = wanted.status
      ORDER BY finished_at DESC, run_id DESC
      LIMIT 1
    ) latest
  )
)
SELECT run_id, source, status, started_at, finished_at, batch_id, event_count, latest_event_at, last_event_type, message, collection_json
FROM candidates
ORDER BY finished_at DESC, run_id DESC
`, hubUserID, recentLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []ingest.Run
	for rows.Next() {
		run, scanErr := scanRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *Repository) AppendRun(ctx context.Context, hubUserID string, run ingest.Run) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return errors.New("ingest hub user id is required")
	}
	if run.ID == "" {
		return errors.New("ingest run id is required")
	}
	if run.Source == "" {
		return errors.New("ingest run source is required")
	}
	if run.Status == "" {
		return errors.New("ingest run status is required")
	}

	var latestEventAt any = nil
	if !run.LatestEventAt.IsZero() {
		latestEventAt = run.LatestEventAt
	}

	collectionJSON, err := json.Marshal(run.Collection)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_ingest_runs (
  hub_user_id, run_id, source, status, started_at, finished_at, batch_id, event_count, latest_event_at, last_event_type, message, collection_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb)
ON CONFLICT (hub_user_id, run_id) DO UPDATE SET
  source = EXCLUDED.source,
  status = EXCLUDED.status,
  started_at = EXCLUDED.started_at,
  finished_at = EXCLUDED.finished_at,
  batch_id = EXCLUDED.batch_id,
  event_count = EXCLUDED.event_count,
  latest_event_at = EXCLUDED.latest_event_at,
  last_event_type = EXCLUDED.last_event_type,
  message = EXCLUDED.message, collection_json=EXCLUDED.collection_json
`, hubUserID, run.ID, run.Source, run.Status, run.StartedAt, run.FinishedAt, run.BatchID, run.EventCount, latestEventAt, run.LastEventType, run.Message, string(collectionJSON))
	return err
}

func (r *Repository) ListOperatorRuns(ctx context.Context, hubUserID string) ([]ingest.OperatorRun, error) {
	return r.listOperatorRuns(ctx, hubUserID, 0)
}

// ListOperatorRunsLimited keeps dashboard/source-summary reads bounded while
// ListOperatorRuns remains available for callers that explicitly need the
// complete persisted history.
func (r *Repository) ListOperatorRunsLimited(ctx context.Context, hubUserID string, limit int) ([]ingest.OperatorRun, error) {
	return r.listOperatorRuns(ctx, hubUserID, limit)
}

func (r *Repository) GetSourceAutoRunFacts(ctx context.Context, hubUserID string) ([]ingest.SourceAutoRunFact, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return nil, errors.New("ingest hub user id is required")
	}
	rows, err := r.db.QueryContext(ctx, `
WITH recent_runs AS MATERIALIZED (
  SELECT run_id, finished_at, latest_batch_id
  FROM insight_operator_runs
  WHERE hub_user_id = $1
    AND trigger_mode IN ($2, $3)
  ORDER BY finished_at DESC, run_id DESC
  LIMIT 100
)
SELECT outcomes.source,
       (array_agg(outcomes.status ORDER BY recent_runs.finished_at DESC, recent_runs.run_id DESC, outcomes.outcome_index ASC))[1] AS last_run_status,
       MAX(recent_runs.finished_at) AS last_run_at,
       (array_agg(recent_runs.finished_at ORDER BY recent_runs.finished_at DESC, recent_runs.run_id DESC, outcomes.outcome_index ASC)
         FILTER (WHERE outcomes.status = 'success'))[1] AS last_success_at,
       (array_agg(recent_runs.latest_batch_id ORDER BY recent_runs.finished_at DESC, recent_runs.run_id DESC, outcomes.outcome_index ASC)
         FILTER (WHERE outcomes.status = 'success'))[1] AS last_batch_id
FROM recent_runs
JOIN insight_operator_run_outcomes AS outcomes
  ON outcomes.hub_user_id = $1 AND outcomes.run_id = recent_runs.run_id
GROUP BY outcomes.source
ORDER BY outcomes.source
`, hubUserID, ingest.OperatorTriggerLoop, ingest.OperatorTriggerScheduled)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []ingest.SourceAutoRunFact
	for rows.Next() {
		var item ingest.SourceAutoRunFact
		var lastSuccessAt sql.NullTime
		var lastBatchID sql.NullString
		if err := rows.Scan(&item.Source, &item.LastRunStatus, &item.LastRunAt, &lastSuccessAt, &lastBatchID); err != nil {
			return nil, err
		}
		if lastSuccessAt.Valid {
			item.LastSuccessAt = lastSuccessAt.Time
		}
		if lastBatchID.Valid {
			item.LastBatchID = lastBatchID.String
		}
		facts = append(facts, item)
	}
	return facts, rows.Err()
}

func (r *Repository) listOperatorRuns(ctx context.Context, hubUserID string, limit int) ([]ingest.OperatorRun, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return nil, errors.New("ingest hub user id is required")
	}
	query := `
SELECT run_id, status, started_at, finished_at, workspace_id, latest_batch_id,
       attempted_sources, succeeded_sources, failed_sources, skipped_sources,
       trigger_mode,
       freshness_state, report_headline, top_video_title, top_topic_keyword, message
FROM insight_operator_runs
WHERE hub_user_id = $1
ORDER BY finished_at DESC, run_id DESC
	`
	args := []any{hubUserID}
	if limit > 0 {
		query += "LIMIT $2\n"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []ingest.OperatorRun
	for rows.Next() {
		var run ingest.OperatorRun
		if err := rows.Scan(
			&run.ID,
			&run.Status,
			&run.StartedAt,
			&run.FinishedAt,
			&run.WorkspaceID,
			&run.LatestBatchID,
			&run.AttemptedSources,
			&run.SucceededSources,
			&run.FailedSources,
			&run.SkippedSources,
			&run.TriggerMode,
			&run.FreshnessState,
			&run.ReportHeadline,
			&run.TopVideoTitle,
			&run.TopTopicKeyword,
			&run.Message,
		); err != nil {
			return nil, err
		}
		outcomes, err := r.listOperatorRunOutcomes(ctx, hubUserID, run.ID)
		if err != nil {
			return nil, err
		}
		run.Outcomes = outcomes
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *Repository) AppendOperatorRun(ctx context.Context, hubUserID string, run ingest.OperatorRun) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return errors.New("ingest hub user id is required")
	}
	if run.ID == "" {
		return errors.New("operator run id is required")
	}
	if run.Status == "" {
		return errors.New("operator run status is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	_, err = tx.ExecContext(ctx, `
INSERT INTO insight_operator_runs (
  hub_user_id, run_id, status, started_at, finished_at, workspace_id, latest_batch_id,
  attempted_sources, succeeded_sources, failed_sources, skipped_sources,
  trigger_mode,
  freshness_state, report_headline, top_video_title, top_topic_keyword, message
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (hub_user_id, run_id) DO UPDATE SET
  status = EXCLUDED.status,
  started_at = EXCLUDED.started_at,
  finished_at = EXCLUDED.finished_at,
  workspace_id = EXCLUDED.workspace_id,
  latest_batch_id = EXCLUDED.latest_batch_id,
  attempted_sources = EXCLUDED.attempted_sources,
  succeeded_sources = EXCLUDED.succeeded_sources,
  failed_sources = EXCLUDED.failed_sources,
  skipped_sources = EXCLUDED.skipped_sources,
  trigger_mode = EXCLUDED.trigger_mode,
  freshness_state = EXCLUDED.freshness_state,
  report_headline = EXCLUDED.report_headline,
  top_video_title = EXCLUDED.top_video_title,
  top_topic_keyword = EXCLUDED.top_topic_keyword,
  message = EXCLUDED.message
`, hubUserID, run.ID, run.Status, run.StartedAt, run.FinishedAt, run.WorkspaceID, run.LatestBatchID,
		run.AttemptedSources, run.SucceededSources, run.FailedSources, run.SkippedSources,
		run.TriggerMode, run.FreshnessState, run.ReportHeadline, run.TopVideoTitle, run.TopTopicKeyword, run.Message)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
DELETE FROM insight_operator_run_outcomes
WHERE hub_user_id = $1 AND run_id = $2
`, hubUserID, run.ID); err != nil {
		return err
	}
	for index, outcome := range run.Outcomes {
		if _, err = tx.ExecContext(ctx, `
INSERT INTO insight_operator_run_outcomes (
  hub_user_id, run_id, outcome_index, source, status, summary
) VALUES ($1, $2, $3, $4, $5, $6)
`, hubUserID, run.ID, index, outcome.Source, outcome.Status, outcome.Summary); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListBindings(ctx context.Context, hubUserID string) ([]ingest.SourceBinding, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return nil, errors.New("ingest hub user id is required")
	}
	rows, err := r.db.QueryContext(ctx, `
	SELECT source, origin, target_key, path, base_url, share_url, object_id, object_nonce_id, username, keyword, device_count, updated_at
FROM insight_ingest_source_bindings
WHERE hub_user_id = $1
ORDER BY updated_at DESC, source ASC
`, hubUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bindings []ingest.SourceBinding
	for rows.Next() {
		var binding ingest.SourceBinding
		if err := rows.Scan(
			&binding.Source,
			&binding.Origin,
			&binding.TargetKey,
			&binding.Path,
			&binding.BaseURL,
			&binding.ShareURL,
			&binding.ObjectID,
			&binding.ObjectNonceID,
			&binding.Username,
			&binding.Keyword,
			&binding.DeviceCount,
			&binding.UpdatedAt,
		); err != nil {
			return nil, err
		}
		binding = ingest.NormalizeSourceBinding(binding)
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (r *Repository) UpsertBinding(ctx context.Context, hubUserID string, binding ingest.SourceBinding) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return errors.New("ingest hub user id is required")
	}
	if binding.Source == "" {
		return errors.New("ingest binding source is required")
	}
	if binding.UpdatedAt.IsZero() {
		return errors.New("ingest binding updated_at is required")
	}
	rawOrigin := strings.TrimSpace(binding.Origin)
	origin := ""
	if rawOrigin != "" {
		origin = ingest.NormalizeBindingOrigin(rawOrigin)
	}
	binding = ingest.NormalizeSourceBinding(binding)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO insight_ingest_source_bindings (
  hub_user_id, source, origin, target_key, path, base_url, share_url, object_id, object_nonce_id, username, keyword, device_count, updated_at
) VALUES ($1, $2, COALESCE(NULLIF($3, ''), 'manual'), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (hub_user_id, source, target_key) DO UPDATE SET
  origin = CASE WHEN NULLIF($3, '') IS NULL THEN insight_ingest_source_bindings.origin ELSE EXCLUDED.origin END,
  path = EXCLUDED.path,
  base_url = EXCLUDED.base_url,
  share_url = EXCLUDED.share_url,
  object_id = EXCLUDED.object_id,
  object_nonce_id = EXCLUDED.object_nonce_id,
  username = EXCLUDED.username,
  keyword = EXCLUDED.keyword,
  device_count = EXCLUDED.device_count,
		updated_at = EXCLUDED.updated_at
	`, hubUserID, binding.Source, origin, binding.TargetKey, binding.Path, binding.BaseURL, binding.ShareURL, binding.ObjectID, binding.ObjectNonceID, binding.Username, binding.Keyword, binding.DeviceCount, binding.UpdatedAt)
	return err
}

func (r *Repository) DeleteBinding(ctx context.Context, hubUserID, source, targetKey string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return errors.New("ingest hub user id is required")
	}
	result, err := r.db.ExecContext(ctx, `
DELETE FROM insight_ingest_source_bindings
WHERE hub_user_id = $1 AND source = $2 AND target_key = $3
`, hubUserID, source, targetKey)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ingest.ErrSourceBindingNotFound
	}
	return nil
}

func (r *Repository) Reset(ctx context.Context, hubUserID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if hubUserID == "" {
		return errors.New("ingest hub user id is required")
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM insight_ingest_runs WHERE hub_user_id = $1`, hubUserID); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM insight_operator_runs WHERE hub_user_id = $1`, hubUserID); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM insight_operator_run_outcomes WHERE hub_user_id = $1`, hubUserID); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM insight_ingest_source_bindings WHERE hub_user_id = $1`, hubUserID); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM insight_ingest_events WHERE hub_user_id = $1`, hubUserID)
	return err
}

func (r *Repository) listOperatorRunOutcomes(ctx context.Context, hubUserID, runID string) ([]ingest.OperatorRunOutcome, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT source, status, summary
FROM insight_operator_run_outcomes
WHERE hub_user_id = $1 AND run_id = $2
ORDER BY outcome_index ASC
`, hubUserID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outcomes []ingest.OperatorRunOutcome
	for rows.Next() {
		var outcome ingest.OperatorRunOutcome
		if err := rows.Scan(
			&outcome.Source,
			&outcome.Status,
			&outcome.Summary,
		); err != nil {
			return nil, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, rows.Err()
}

type eventExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func upsertEvent(ctx context.Context, exec eventExecutor, event ingest.Event) error {
	if event.HubUserID == "" {
		return errors.New("ingest event hub user id is required")
	}
	if event.ID == "" {
		return errors.New("ingest event id is required")
	}
	tagsJSON, err := json.Marshal(event.Tags)
	if err != nil {
		return err
	}
	keywordsJSON, err := json.Marshal(event.Keywords)
	if err != nil {
		return err
	}
	var publishedAt any
	if !event.PublishedAt.IsZero() {
		publishedAt = event.PublishedAt
	}
	_, err = exec.ExecContext(ctx, `
INSERT INTO insight_ingest_events (
	 hub_user_id, event_id, batch_id, machine_id, event_type, source, collected_at,
	 published_at, object_id, object_nonce_id, video_count, play_count, like_count, comment_count, fav_count, forward_count, topic_count, title, channel_name, channel_username, cover_url, keyword, cluster,
	 signal, prompt_angle, representative_comment, target_audience, tags_json, keywords_json, missing_metrics
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28::jsonb, $29::jsonb, $30)
ON CONFLICT (hub_user_id, event_id) DO UPDATE SET
  batch_id = EXCLUDED.batch_id,
  machine_id = EXCLUDED.machine_id,
  event_type = EXCLUDED.event_type,
  source = EXCLUDED.source,
  collected_at = EXCLUDED.collected_at,
	published_at = EXCLUDED.published_at,
  object_id = EXCLUDED.object_id,
	object_nonce_id = EXCLUDED.object_nonce_id,
	video_count = EXCLUDED.video_count,
	play_count = EXCLUDED.play_count,
	like_count = EXCLUDED.like_count,
	comment_count = EXCLUDED.comment_count,
	fav_count = EXCLUDED.fav_count,
	forward_count = EXCLUDED.forward_count,
	topic_count = EXCLUDED.topic_count,
  title = EXCLUDED.title,
		channel_name = EXCLUDED.channel_name,
		channel_username = EXCLUDED.channel_username,
		cover_url = EXCLUDED.cover_url,
  keyword = EXCLUDED.keyword,
  cluster = EXCLUDED.cluster,
  signal = EXCLUDED.signal,
  prompt_angle = EXCLUDED.prompt_angle,
  representative_comment = EXCLUDED.representative_comment,
  target_audience = EXCLUDED.target_audience,
  tags_json = EXCLUDED.tags_json,
  keywords_json = EXCLUDED.keywords_json,
 missing_metrics = EXCLUDED.missing_metrics
	`, event.HubUserID, event.ID, event.BatchID, event.MachineID, event.EventType, event.Source, event.CollectedAt, publishedAt,
		event.ObjectID, event.ObjectNonceID, event.VideoCount, event.PlayCount, event.LikeCount, event.CommentCount, event.FavCount, event.ForwardCount, event.TopicCount, event.Title, event.ChannelName, event.ChannelUsername, event.CoverURL, event.Keyword, event.Cluster,
		event.Signal, event.PromptAngle, event.RepresentativeComment, event.TargetAudience, string(tagsJSON), string(keywordsJSON), int(event.MissingMetrics))
	return err
}
