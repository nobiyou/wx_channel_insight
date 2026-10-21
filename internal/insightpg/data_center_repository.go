package insightpg

import (
	"context"
	"database/sql"
	"errors"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type DataCenterExclusionRepository struct {
	db *sql.DB
}

var _ insight.DataCenterExclusionRepository = (*DataCenterExclusionRepository)(nil)

func NewDataCenterExclusionRepository(client *postgres.Client) (*DataCenterExclusionRepository, error) {
	if client == nil || client.DB() == nil {
		return nil, errors.New("postgres client is required")
	}
	if err := postgres.EnsureDataCenterExclusionSchema(client.DB()); err != nil {
		return nil, err
	}
	return &DataCenterExclusionRepository{db: client.DB()}, nil
}

func (r *DataCenterExclusionRepository) List(ctx context.Context, workspaceID string) ([]insight.DataCenterExclusion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT workspace_id, entity, entity_id, deleted_at, deleted_by
FROM insight_data_center_exclusions
WHERE workspace_id = $1
ORDER BY deleted_at DESC, entity, entity_id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []insight.DataCenterExclusion{}
	for rows.Next() {
		var item insight.DataCenterExclusion
		if err := rows.Scan(&item.WorkspaceID, &item.Entity, &item.EntityID, &item.DeletedAt, &item.DeletedBy); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *DataCenterExclusionRepository) Upsert(ctx context.Context, exclusion insight.DataCenterExclusion) (insight.DataCenterExclusion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var result insight.DataCenterExclusion
	err := r.db.QueryRowContext(ctx, `
INSERT INTO insight_data_center_exclusions (workspace_id, entity, entity_id, deleted_at, deleted_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (workspace_id, entity, entity_id) DO UPDATE SET
  entity_id = EXCLUDED.entity_id
RETURNING workspace_id, entity, entity_id, deleted_at, deleted_by`,
		exclusion.WorkspaceID, exclusion.Entity, exclusion.EntityID, exclusion.DeletedAt, exclusion.DeletedBy,
	).Scan(&result.WorkspaceID, &result.Entity, &result.EntityID, &result.DeletedAt, &result.DeletedBy)
	return result, err
}

func (r *DataCenterExclusionRepository) Restore(ctx context.Context, workspaceID string, entity insight.DataCenterEntity, entityID string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := r.db.ExecContext(ctx, `
DELETE FROM insight_data_center_exclusions
WHERE workspace_id = $1 AND entity = $2 AND entity_id = $3`, workspaceID, entity, entityID)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}
