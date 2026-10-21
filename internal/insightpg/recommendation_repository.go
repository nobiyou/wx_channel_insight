package insightpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type RecommendationRepository struct {
	client *postgres.Client
	db     *sql.DB
}

func NewRecommendationRepository(client *postgres.Client) (*RecommendationRepository, error) {
	if client == nil {
		return nil, errors.New("postgres client is required")
	}
	if err := postgres.EnsureRecommendationSchema(client.DB()); err != nil {
		return nil, err
	}
	return &RecommendationRepository{client: client, db: client.DB()}, nil
}

var _ insight.RecommendationRepository = (*RecommendationRepository)(nil)

func (r *RecommendationRepository) GetRecommendations(ctx context.Context, workspaceID string) (insight.RecommendationSnapshot, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var payload []byte
	err := r.db.QueryRowContext(ctx, `SELECT payload FROM insight_recommendation_snapshots WHERE snapshot_id = $1`, workspaceID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return insight.RecommendationSnapshot{}, false, nil
	}
	if err != nil {
		return insight.RecommendationSnapshot{}, false, err
	}
	var snapshot insight.RecommendationSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return insight.RecommendationSnapshot{}, true, err
	}
	return insight.CloneRecommendationSnapshot(snapshot), true, nil
}

func (r *RecommendationRepository) ReplaceRecommendations(ctx context.Context, workspaceID string, snapshot insight.RecommendationSnapshot) error {
	if ctx == nil {
		ctx = context.Background()
	}
	snapshot.WorkspaceID = workspaceID
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO insight_recommendation_snapshots (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET payload = EXCLUDED.payload, updated_at = now()`, workspaceID, string(payload))
	return err
}

func (r *RecommendationRepository) ResetRecommendations(ctx context.Context, workspaceID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM insight_recommendation_snapshots WHERE snapshot_id = $1`, workspaceID)
	return err
}
