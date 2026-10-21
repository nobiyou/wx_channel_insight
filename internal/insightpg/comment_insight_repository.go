package insightpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type CommentInsightRepository struct {
	client *postgres.Client
	db     *sql.DB
	seed   []insight.CommentInsight
}

type commentInsightSnapshotPayload struct {
	Items []insight.CommentInsight      `json:"items"`
	Meta  insight.ReadModelArtifactMeta `json:"meta"`
}

var _ insight.CommentInsightRepository = (*CommentInsightRepository)(nil)

func NewCommentInsightRepository(client *postgres.Client, seed []insight.CommentInsight) (*CommentInsightRepository, error) {
	if client == nil {
		return nil, errors.New("postgres client is required")
	}
	db := client.DB()
	if err := postgres.EnsureCommentInsightSchema(db); err != nil {
		return nil, err
	}
	repo := &CommentInsightRepository{
		client: client,
		db:     db,
		seed:   insight.CloneCommentInsights(seed),
	}
	if err := repo.ensureSeed(context.Background(), ""); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *CommentInsightRepository) GetCommentInsights(ctx context.Context, workspaceID string) ([]insight.CommentInsight, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload, updated_at FROM insight_comment_insights WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	var updatedAt time.Time
	if err := row.Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return insight.CloneCommentInsights(r.seed), false, nil
		}
		return nil, false, err
	}
	items, _, err := parseCommentInsightSnapshotPayload(payload, updatedAt)
	if err != nil {
		return nil, true, err
	}
	return insight.CloneCommentInsights(items), true, nil
}

func (r *CommentInsightRepository) ReplaceCommentInsights(ctx context.Context, workspaceID string, items []insight.CommentInsight) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.save(ctx, workspaceID, items)
}

func (r *CommentInsightRepository) ResetCommentInsights(ctx context.Context, workspaceID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.save(ctx, workspaceID, r.seed)
}

func (r *CommentInsightRepository) GetMeta(ctx context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload, updated_at FROM insight_comment_insights WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	var updatedAt time.Time
	if err := row.Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return insight.ReadModelArtifactMeta{}, false, nil
		}
		return insight.ReadModelArtifactMeta{}, false, err
	}
	_, meta, err := parseCommentInsightSnapshotPayload(payload, updatedAt)
	if err != nil {
		return insight.ReadModelArtifactMeta{}, true, err
	}
	return meta, true, nil
}

func (r *CommentInsightRepository) ReplaceCommentInsightMeta(ctx context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	if ctx == nil {
		ctx = context.Background()
	}
	items, found, err := r.GetCommentInsights(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !found {
		items = insight.CloneCommentInsights(r.seed)
	}
	payload, err := json.Marshal(commentInsightSnapshotPayload{
		Items: items,
		Meta:  meta,
	})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_comment_insights (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = now()
`, snapshotKey(workspaceID), string(payload))
	return err
}

func (r *CommentInsightRepository) ensureSeed(ctx context.Context, workspaceID string) error {
	row := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM insight_comment_insights WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var count int
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return r.save(ctx, workspaceID, r.seed)
}

func (r *CommentInsightRepository) save(ctx context.Context, workspaceID string, items []insight.CommentInsight) error {
	meta, _, err := r.GetMeta(ctx, workspaceID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(commentInsightSnapshotPayload{
		Items: items,
		Meta:  meta,
	})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_comment_insights (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = now()
`, snapshotKey(workspaceID), string(payload))
	return err
}

func parseCommentInsightSnapshotPayload(payload []byte, updatedAt time.Time) ([]insight.CommentInsight, insight.ReadModelArtifactMeta, error) {
	var wrapped commentInsightSnapshotPayload
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return nil, insight.ReadModelArtifactMeta{}, err
	}
	if wrapped.Meta.MaterializedAt.IsZero() {
		wrapped.Meta.MaterializedAt = updatedAt
	}
	return wrapped.Items, wrapped.Meta, nil
}
