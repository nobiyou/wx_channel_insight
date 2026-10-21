package workspacepg

import (
	"context"
	"database/sql"
	"errors"

	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type Repository struct {
	client *postgres.Client
	db     *sql.DB
}

var _ workspace.DefaultWorkspaceRepository = (*Repository)(nil)

func NewDefaultWorkspaceRepository(client *postgres.Client) (*Repository, error) {
	if client == nil {
		return nil, errors.New("postgres client is required")
	}
	db := client.DB()
	if err := postgres.EnsureDefaultWorkspaceSchema(db); err != nil {
		return nil, err
	}
	return &Repository{client: client, db: db}, nil
}

func (r *Repository) UpsertDefaultWorkspace(ctx context.Context, ownerUserID string, build func(existing workspace.DefaultWorkspace, found bool) (workspace.DefaultWorkspace, string, error)) (workspace.DefaultWorkspace, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if build == nil {
		return workspace.DefaultWorkspace{}, "", errors.New("workspace build func is required")
	}

	existing, found, err := r.load(ctx, ownerUserID)
	if err != nil {
		return workspace.DefaultWorkspace{}, "", err
	}
	next, resolution, err := build(existing, found)
	if err != nil {
		return workspace.DefaultWorkspace{}, "", err
	}
	if err := r.Save(ctx, next); err != nil {
		return workspace.DefaultWorkspace{}, "", err
	}
	return next, resolution, nil
}

func (r *Repository) Save(ctx context.Context, item workspace.DefaultWorkspace) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if item.OwnerUserID == "" {
		return errors.New("workspace owner user id is required")
	}
	if item.ID == "" {
		return errors.New("workspace id is required")
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO insight_default_workspaces (
  owner_user_id, workspace_id, name, device_count, bootstrap_mode, source, state, created_at, last_synced_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (owner_user_id) DO UPDATE SET
  workspace_id = EXCLUDED.workspace_id,
  name = EXCLUDED.name,
  device_count = EXCLUDED.device_count,
  bootstrap_mode = EXCLUDED.bootstrap_mode,
  source = EXCLUDED.source,
  state = EXCLUDED.state,
  created_at = EXCLUDED.created_at,
  last_synced_at = EXCLUDED.last_synced_at
`, item.OwnerUserID, item.ID, item.Name, item.DeviceCount, item.BootstrapMode, item.Source, item.State, item.CreatedAt, item.LastSyncedAt)
	return err
}

func (r *Repository) load(ctx context.Context, ownerUserID string) (workspace.DefaultWorkspace, bool, error) {
	if ownerUserID == "" {
		return workspace.DefaultWorkspace{}, false, errors.New("workspace owner user id is required")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT workspace_id, name, owner_user_id, device_count, bootstrap_mode, source, state, created_at, last_synced_at
FROM insight_default_workspaces
WHERE owner_user_id = $1
`, ownerUserID)

	var item workspace.DefaultWorkspace
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.OwnerUserID,
		&item.DeviceCount,
		&item.BootstrapMode,
		&item.Source,
		&item.State,
		&item.CreatedAt,
		&item.LastSyncedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return workspace.DefaultWorkspace{}, false, nil
	}
	if err != nil {
		return workspace.DefaultWorkspace{}, false, err
	}
	return item, true, nil
}
