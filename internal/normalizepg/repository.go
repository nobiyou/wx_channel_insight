package normalizepg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type Repository struct {
	client *postgres.Client
	db     *sql.DB
}

var _ normalize.Repository = (*Repository)(nil)

func nullablePublishedAt(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func NewRepository(client *postgres.Client) (*Repository, error) {
	if client == nil {
		return nil, errors.New("postgres client is required")
	}
	db := client.DB()
	if err := postgres.EnsureNormalizationSchema(db); err != nil {
		return nil, err
	}
	return &Repository{client: client, db: db}, nil
}

func (r *Repository) ReplaceSnapshot(ctx context.Context, workspaceID string, snapshot normalize.Snapshot) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if workspaceID == "" {
		return errors.New("normalization workspace id is required")
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
	if err = postgres.AcquireWorkspaceAdvisoryLock(ctx, tx, "insight-normalize", workspaceID); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM insight_content_snapshots WHERE workspace_id = $1`, workspaceID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM insight_comment_snapshots WHERE workspace_id = $1`, workspaceID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM insight_topic_observations WHERE workspace_id = $1`, workspaceID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM insight_account_observations WHERE workspace_id = $1`, workspaceID); err != nil {
		return err
	}

	var latestEventAt any = nil
	if !snapshot.Summary.LatestEventAt.IsZero() {
		latestEventAt = snapshot.Summary.LatestEventAt
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO insight_normalized_batches (
  workspace_id, hub_user_id, state, latest_batch_id, generated_at, latest_event_at,
  content_count, comment_count, topic_count, account_count
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (workspace_id) DO UPDATE SET
  hub_user_id = EXCLUDED.hub_user_id,
  state = EXCLUDED.state,
  latest_batch_id = EXCLUDED.latest_batch_id,
  generated_at = EXCLUDED.generated_at,
  latest_event_at = EXCLUDED.latest_event_at,
  content_count = EXCLUDED.content_count,
  comment_count = EXCLUDED.comment_count,
  topic_count = EXCLUDED.topic_count,
  account_count = EXCLUDED.account_count
`, workspaceID, snapshot.Summary.HubUserID, snapshot.Summary.State, snapshot.Summary.LatestBatchID, snapshot.Summary.GeneratedAt, latestEventAt, snapshot.Summary.ContentCount, snapshot.Summary.CommentCount, snapshot.Summary.TopicCount, snapshot.Summary.AccountCount)
	if err != nil {
		return err
	}

	for _, item := range snapshot.ContentSnapshots {
		tagsJSON, marshalErr := json.Marshal(item.Tags)
		if marshalErr != nil {
			err = marshalErr
			return err
		}
		keywordsJSON, marshalErr := json.Marshal(item.Keywords)
		if marshalErr != nil {
			err = marshalErr
			return err
		}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO insight_content_snapshots (
  workspace_id, snapshot_id, batch_id, event_id, source, collected_at, published_at, title, channel_name, channel_username,
  cover_url, object_id, object_nonce_id, keyword, signal, prompt_angle, target_audience, video_count, play_count, like_count, comment_count, fav_count, forward_count, topic_count, tags_json, keywords_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25::jsonb, $26::jsonb)
`, workspaceID, item.ID, item.BatchID, item.EventID, item.Source, item.CollectedAt, nullablePublishedAt(item.PublishedAt), item.Title, item.ChannelName, item.ChannelUsername, item.CoverURL, item.ObjectID, item.ObjectNonceID, item.Keyword, item.Signal, item.PromptAngle, item.TargetAudience, item.VideoCount, item.PlayCount, item.LikeCount, item.CommentCount, item.FavCount, item.ForwardCount, item.TopicCount, tagsJSON, keywordsJSON); err != nil {
			return err
		}
	}
	for _, item := range snapshot.CommentSnapshots {
		keywordsJSON, marshalErr := json.Marshal(item.Keywords)
		if marshalErr != nil {
			err = marshalErr
			return err
		}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO insight_comment_snapshots (
  workspace_id, snapshot_id, batch_id, event_id, source, collected_at, title, channel_name,
  object_id, object_nonce_id, keyword, signal, representative_comment, target_audience, video_count, comment_count, topic_count, keywords_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
`, workspaceID, item.ID, item.BatchID, item.EventID, item.Source, item.CollectedAt, item.Title, item.ChannelName, item.ObjectID, item.ObjectNonceID, item.Keyword, item.Signal, item.RepresentativeComment, item.TargetAudience, item.VideoCount, item.CommentCount, item.TopicCount, keywordsJSON); err != nil {
			return err
		}
	}
	for _, item := range snapshot.TopicObservations {
		keywordsJSON, marshalErr := json.Marshal(item.Keywords)
		if marshalErr != nil {
			err = marshalErr
			return err
		}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO insight_topic_observations (
  workspace_id, observation_id, batch_id, event_id, source, collected_at, keyword, cluster, signal, prompt_angle,
  object_id, object_nonce_id, target_audience, video_count, comment_count, topic_count, keywords_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
`, workspaceID, item.ID, item.BatchID, item.EventID, item.Source, item.CollectedAt, item.Keyword, item.Cluster, item.Signal, item.PromptAngle, item.ObjectID, item.ObjectNonceID, item.TargetAudience, item.VideoCount, item.CommentCount, item.TopicCount, keywordsJSON); err != nil {
			return err
		}
	}
	for _, item := range snapshot.AccountObservations {
		tagsJSON, marshalErr := json.Marshal(item.Tags)
		if marshalErr != nil {
			err = marshalErr
			return err
		}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO insight_account_observations (
  workspace_id, observation_id, batch_id, event_id, source, collected_at, channel_username, channel_name, title, signal,
  video_count, comment_count, topic_count, tags_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb)
`, workspaceID, item.ID, item.BatchID, item.EventID, item.Source, item.CollectedAt, item.Username, item.ChannelName, item.Title, item.Signal, item.VideoCount, item.CommentCount, item.TopicCount, tagsJSON); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *Repository) GetSummary(ctx context.Context, workspaceID string) (normalize.Summary, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if workspaceID == "" {
		return normalize.Summary{}, false, errors.New("normalization workspace id is required")
	}

	var result normalize.Summary
	var latestEventAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT workspace_id, hub_user_id, state, latest_batch_id, generated_at, latest_event_at,
       content_count, comment_count, topic_count, account_count
FROM insight_normalized_batches
WHERE workspace_id = $1
`, workspaceID).Scan(
		&result.WorkspaceID,
		&result.HubUserID,
		&result.State,
		&result.LatestBatchID,
		&result.GeneratedAt,
		&latestEventAt,
		&result.ContentCount,
		&result.CommentCount,
		&result.TopicCount,
		&result.AccountCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return normalize.Summary{}, false, nil
	}
	if err != nil {
		return normalize.Summary{}, false, err
	}
	if latestEventAt.Valid {
		result.LatestEventAt = latestEventAt.Time
	}
	return result, true, nil
}

func (r *Repository) GetSnapshot(ctx context.Context, workspaceID string) (normalize.Snapshot, bool, error) {
	summary, found, err := r.GetSummary(ctx, workspaceID)
	if err != nil || !found {
		return normalize.Snapshot{}, found, err
	}
	content, err := r.listContentSnapshots(ctx, workspaceID)
	if err != nil {
		return normalize.Snapshot{}, false, err
	}
	comments, err := r.listCommentSnapshots(ctx, workspaceID)
	if err != nil {
		return normalize.Snapshot{}, false, err
	}
	topics, err := r.listTopicObservations(ctx, workspaceID)
	if err != nil {
		return normalize.Snapshot{}, false, err
	}
	accounts, err := r.listAccountObservations(ctx, workspaceID)
	if err != nil {
		return normalize.Snapshot{}, false, err
	}
	return normalize.Snapshot{
		Summary:             summary,
		ContentSnapshots:    content,
		CommentSnapshots:    comments,
		TopicObservations:   topics,
		AccountObservations: accounts,
	}, true, nil
}

func (r *Repository) Reset(ctx context.Context, workspaceID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if workspaceID == "" {
		return errors.New("normalization workspace id is required")
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
	statements := []string{
		`DELETE FROM insight_content_snapshots WHERE workspace_id = $1`,
		`DELETE FROM insight_comment_snapshots WHERE workspace_id = $1`,
		`DELETE FROM insight_topic_observations WHERE workspace_id = $1`,
		`DELETE FROM insight_account_observations WHERE workspace_id = $1`,
		`DELETE FROM insight_normalized_batches WHERE workspace_id = $1`,
	}
	for _, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement, workspaceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) listContentSnapshots(ctx context.Context, workspaceID string) ([]normalize.ContentSnapshot, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT snapshot_id, batch_id, event_id, source, collected_at, title, channel_name, channel_username, cover_url, object_id, object_nonce_id, signal, tags_json
       , published_at, keyword, prompt_angle, target_audience, video_count, play_count, like_count, comment_count, fav_count, forward_count, topic_count, keywords_json
FROM insight_content_snapshots
WHERE workspace_id = $1
ORDER BY collected_at DESC, snapshot_id ASC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []normalize.ContentSnapshot{}
	for rows.Next() {
		var item normalize.ContentSnapshot
		var publishedAt sql.NullTime
		var tagsJSON []byte
		var keywordsJSON []byte
		if err := rows.Scan(&item.ID, &item.BatchID, &item.EventID, &item.Source, &item.CollectedAt, &item.Title, &item.ChannelName, &item.ChannelUsername, &item.CoverURL, &item.ObjectID, &item.ObjectNonceID, &item.Signal, &tagsJSON, &publishedAt, &item.Keyword, &item.PromptAngle, &item.TargetAudience, &item.VideoCount, &item.PlayCount, &item.LikeCount, &item.CommentCount, &item.FavCount, &item.ForwardCount, &item.TopicCount, &keywordsJSON); err != nil {
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
		if len(keywordsJSON) > 0 {
			if err := json.Unmarshal(keywordsJSON, &item.Keywords); err != nil {
				return nil, err
			}
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *Repository) listCommentSnapshots(ctx context.Context, workspaceID string) ([]normalize.CommentSnapshot, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT snapshot_id, batch_id, event_id, source, collected_at, title, channel_name, object_id, object_nonce_id, keyword, signal, representative_comment, target_audience, video_count, comment_count, topic_count, keywords_json
FROM insight_comment_snapshots
WHERE workspace_id = $1
ORDER BY collected_at DESC, snapshot_id ASC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []normalize.CommentSnapshot{}
	for rows.Next() {
		var item normalize.CommentSnapshot
		var keywordsJSON []byte
		if err := rows.Scan(&item.ID, &item.BatchID, &item.EventID, &item.Source, &item.CollectedAt, &item.Title, &item.ChannelName, &item.ObjectID, &item.ObjectNonceID, &item.Keyword, &item.Signal, &item.RepresentativeComment, &item.TargetAudience, &item.VideoCount, &item.CommentCount, &item.TopicCount, &keywordsJSON); err != nil {
			return nil, err
		}
		if len(keywordsJSON) > 0 {
			if err := json.Unmarshal(keywordsJSON, &item.Keywords); err != nil {
				return nil, err
			}
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *Repository) listTopicObservations(ctx context.Context, workspaceID string) ([]normalize.TopicObservation, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT observation_id, batch_id, event_id, source, collected_at, keyword, cluster, signal, prompt_angle, object_id, object_nonce_id, target_audience, video_count, comment_count, topic_count, keywords_json
FROM insight_topic_observations
WHERE workspace_id = $1
ORDER BY collected_at DESC, observation_id ASC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []normalize.TopicObservation{}
	for rows.Next() {
		var item normalize.TopicObservation
		var keywordsJSON []byte
		if err := rows.Scan(&item.ID, &item.BatchID, &item.EventID, &item.Source, &item.CollectedAt, &item.Keyword, &item.Cluster, &item.Signal, &item.PromptAngle, &item.ObjectID, &item.ObjectNonceID, &item.TargetAudience, &item.VideoCount, &item.CommentCount, &item.TopicCount, &keywordsJSON); err != nil {
			return nil, err
		}
		if len(keywordsJSON) > 0 {
			if err := json.Unmarshal(keywordsJSON, &item.Keywords); err != nil {
				return nil, err
			}
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *Repository) listAccountObservations(ctx context.Context, workspaceID string) ([]normalize.AccountObservation, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT observation_id, batch_id, event_id, source, collected_at, channel_username, channel_name, title, signal, video_count, comment_count, topic_count, tags_json
FROM insight_account_observations
WHERE workspace_id = $1
ORDER BY collected_at DESC, observation_id ASC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []normalize.AccountObservation{}
	for rows.Next() {
		var item normalize.AccountObservation
		var tagsJSON []byte
		if err := rows.Scan(&item.ID, &item.BatchID, &item.EventID, &item.Source, &item.CollectedAt, &item.Username, &item.ChannelName, &item.Title, &item.Signal, &item.VideoCount, &item.CommentCount, &item.TopicCount, &tagsJSON); err != nil {
			return nil, err
		}
		if len(tagsJSON) > 0 {
			if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
				return nil, err
			}
		}
		results = append(results, item)
	}
	return results, rows.Err()
}
