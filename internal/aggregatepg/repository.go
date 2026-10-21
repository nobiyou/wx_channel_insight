package aggregatepg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type Repository struct {
	client *postgres.Client
	db     *sql.DB
}

var _ aggregate.Repository = (*Repository)(nil)

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
	if err := postgres.EnsureAggregationSchema(db); err != nil {
		return nil, err
	}
	return &Repository{client: client, db: db}, nil
}

func (r *Repository) ReplaceAggregate(ctx context.Context, workspaceID string, input aggregate.Aggregate) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if workspaceID == "" {
		return errors.New("aggregate workspace id is required")
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
	if err = postgres.AcquireWorkspaceAdvisoryLock(ctx, tx, "insight-aggregate", workspaceID); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM insight_video_metrics_hourly WHERE workspace_id = $1`, workspaceID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM insight_topic_metrics_hourly WHERE workspace_id = $1`, workspaceID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM insight_comment_insight_facts WHERE workspace_id = $1`, workspaceID); err != nil {
		return err
	}

	var latestObservedAt any = nil
	if !input.Summary.LatestObservedAt.IsZero() {
		latestObservedAt = input.Summary.LatestObservedAt
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO insight_aggregate_batches (
  workspace_id, hub_user_id, state, latest_batch_id, generated_at, latest_observed_at,
  video_metric_count, topic_metric_count, comment_fact_count, source_account_count
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (workspace_id) DO UPDATE SET
  hub_user_id = EXCLUDED.hub_user_id,
  state = EXCLUDED.state,
  latest_batch_id = EXCLUDED.latest_batch_id,
  generated_at = EXCLUDED.generated_at,
  latest_observed_at = EXCLUDED.latest_observed_at,
  video_metric_count = EXCLUDED.video_metric_count,
  topic_metric_count = EXCLUDED.topic_metric_count,
  comment_fact_count = EXCLUDED.comment_fact_count,
  source_account_count = EXCLUDED.source_account_count
`, workspaceID, input.Summary.HubUserID, input.Summary.State, input.Summary.LatestBatchID, input.Summary.GeneratedAt, latestObservedAt, input.Summary.VideoMetricCount, input.Summary.TopicMetricCount, input.Summary.CommentFactCount, input.Summary.SourceAccountCount)
	if err != nil {
		return err
	}

	for _, item := range input.VideoMetrics {
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
INSERT INTO insight_video_metrics_hourly (
  workspace_id, metric_id, event_id, batch_id, source, observed_at, published_at, object_id, object_nonce_id, cover_url, title, channel_name, keyword,
  signal, prompt_angle, target_audience, tags_json, keywords_json, views, likes, comments, favorites, forwards, growth_rate, engagement_score
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
`, workspaceID, item.ID, item.EventID, item.BatchID, item.Source, item.ObservedAt, nullablePublishedAt(item.PublishedAt), item.ObjectID, item.ObjectNonceID, item.CoverURL, item.Title, item.ChannelName, item.Keyword, item.Signal, item.PromptAngle, item.TargetAudience, tagsJSON, keywordsJSON, item.Views, item.Likes, item.Comments, item.Favorites, item.Forwards, item.GrowthRate, item.EngagementScore); err != nil {
			return err
		}
	}
	for _, item := range input.TopicMetrics {
		keywordsJSON, marshalErr := json.Marshal(item.Keywords)
		if marshalErr != nil {
			err = marshalErr
			return err
		}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO insight_topic_metrics_hourly (
  workspace_id, metric_id, event_id, batch_id, source, observed_at, keyword, cluster, signal,
  prompt_angle, target_audience, keywords_json, mentions, growth_rate, related_videos
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
`, workspaceID, item.ID, item.EventID, item.BatchID, item.Source, item.ObservedAt, item.Keyword, item.Cluster, item.Signal, item.PromptAngle, item.TargetAudience, keywordsJSON, item.Mentions, item.GrowthRate, item.RelatedVideos); err != nil {
			return err
		}
	}
	for _, item := range input.CommentInsightFacts {
		keywordsJSON, marshalErr := json.Marshal(item.RelatedKeywords)
		if marshalErr != nil {
			err = marshalErr
			return err
		}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO insight_comment_insight_facts (
  workspace_id, fact_id, event_id, batch_id, source, observed_at, object_id, object_nonce_id, theme, sentiment, representative,
  action_suggestion, target_audience, related_keywords_json, question_rate, comment_count
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
`, workspaceID, item.ID, item.EventID, item.BatchID, item.Source, item.ObservedAt, item.ObjectID, item.ObjectNonceID, item.Theme, item.Sentiment, item.Representative, item.ActionSuggestion, item.TargetAudience, keywordsJSON, item.QuestionRate, item.CommentCount); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *Repository) GetSummary(ctx context.Context, workspaceID string) (aggregate.Summary, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if workspaceID == "" {
		return aggregate.Summary{}, false, errors.New("aggregate workspace id is required")
	}
	var result aggregate.Summary
	var latestObservedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT workspace_id, hub_user_id, state, latest_batch_id, generated_at, latest_observed_at,
       video_metric_count, topic_metric_count, comment_fact_count, source_account_count
FROM insight_aggregate_batches
WHERE workspace_id = $1
`, workspaceID).Scan(
		&result.WorkspaceID,
		&result.HubUserID,
		&result.State,
		&result.LatestBatchID,
		&result.GeneratedAt,
		&latestObservedAt,
		&result.VideoMetricCount,
		&result.TopicMetricCount,
		&result.CommentFactCount,
		&result.SourceAccountCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return aggregate.Summary{}, false, nil
	}
	if err != nil {
		return aggregate.Summary{}, false, err
	}
	if latestObservedAt.Valid {
		result.LatestObservedAt = latestObservedAt.Time
	}
	return result, true, nil
}

func (r *Repository) GetAggregate(ctx context.Context, workspaceID string) (aggregate.Aggregate, bool, error) {
	summary, found, err := r.GetSummary(ctx, workspaceID)
	if err != nil || !found {
		return aggregate.Aggregate{}, found, err
	}
	videoMetrics, err := r.listVideoMetrics(ctx, workspaceID)
	if err != nil {
		return aggregate.Aggregate{}, false, err
	}
	topicMetrics, err := r.listTopicMetrics(ctx, workspaceID)
	if err != nil {
		return aggregate.Aggregate{}, false, err
	}
	commentFacts, err := r.listCommentFacts(ctx, workspaceID)
	if err != nil {
		return aggregate.Aggregate{}, false, err
	}
	return aggregate.Aggregate{
		Summary:             summary,
		VideoMetrics:        videoMetrics,
		TopicMetrics:        topicMetrics,
		CommentInsightFacts: commentFacts,
	}, true, nil
}

func (r *Repository) Reset(ctx context.Context, workspaceID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if workspaceID == "" {
		return errors.New("aggregate workspace id is required")
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
		`DELETE FROM insight_video_metrics_hourly WHERE workspace_id = $1`,
		`DELETE FROM insight_topic_metrics_hourly WHERE workspace_id = $1`,
		`DELETE FROM insight_comment_insight_facts WHERE workspace_id = $1`,
		`DELETE FROM insight_aggregate_batches WHERE workspace_id = $1`,
	}
	for _, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement, workspaceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) listVideoMetrics(ctx context.Context, workspaceID string) ([]aggregate.VideoMetric, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT metric_id, event_id, batch_id, source, observed_at, title, channel_name, keyword, signal,
       published_at, object_id, object_nonce_id, cover_url, prompt_angle, target_audience, tags_json, keywords_json, views, likes, comments, favorites, forwards, growth_rate, engagement_score
FROM insight_video_metrics_hourly
WHERE workspace_id = $1
ORDER BY observed_at DESC, metric_id ASC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []aggregate.VideoMetric{}
	for rows.Next() {
		var item aggregate.VideoMetric
		var publishedAt sql.NullTime
		var tagsJSON []byte
		var keywordsJSON []byte
		if err := rows.Scan(&item.ID, &item.EventID, &item.BatchID, &item.Source, &item.ObservedAt, &item.Title, &item.ChannelName, &item.Keyword, &item.Signal, &publishedAt, &item.ObjectID, &item.ObjectNonceID, &item.CoverURL, &item.PromptAngle, &item.TargetAudience, &tagsJSON, &keywordsJSON, &item.Views, &item.Likes, &item.Comments, &item.Favorites, &item.Forwards, &item.GrowthRate, &item.EngagementScore); err != nil {
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

func (r *Repository) listTopicMetrics(ctx context.Context, workspaceID string) ([]aggregate.TopicMetric, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT metric_id, event_id, batch_id, source, observed_at, keyword, cluster, signal,
       prompt_angle, target_audience, keywords_json, mentions, growth_rate, related_videos
FROM insight_topic_metrics_hourly
WHERE workspace_id = $1
ORDER BY observed_at DESC, metric_id ASC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []aggregate.TopicMetric{}
	for rows.Next() {
		var item aggregate.TopicMetric
		var keywordsJSON []byte
		if err := rows.Scan(&item.ID, &item.EventID, &item.BatchID, &item.Source, &item.ObservedAt, &item.Keyword, &item.Cluster, &item.Signal, &item.PromptAngle, &item.TargetAudience, &keywordsJSON, &item.Mentions, &item.GrowthRate, &item.RelatedVideos); err != nil {
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

func (r *Repository) listCommentFacts(ctx context.Context, workspaceID string) ([]aggregate.CommentInsightFact, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT fact_id, event_id, batch_id, source, observed_at, theme, sentiment, representative,
       object_id, object_nonce_id, action_suggestion, target_audience, related_keywords_json, question_rate, comment_count
FROM insight_comment_insight_facts
WHERE workspace_id = $1
ORDER BY observed_at DESC, fact_id ASC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []aggregate.CommentInsightFact{}
	for rows.Next() {
		var item aggregate.CommentInsightFact
		var keywordsJSON []byte
		if err := rows.Scan(&item.ID, &item.EventID, &item.BatchID, &item.Source, &item.ObservedAt, &item.Theme, &item.Sentiment, &item.Representative, &item.ObjectID, &item.ObjectNonceID, &item.ActionSuggestion, &item.TargetAudience, &keywordsJSON, &item.QuestionRate, &item.CommentCount); err != nil {
			return nil, err
		}
		if len(keywordsJSON) > 0 {
			if err := json.Unmarshal(keywordsJSON, &item.RelatedKeywords); err != nil {
				return nil, err
			}
		}
		results = append(results, item)
	}
	return results, rows.Err()
}
