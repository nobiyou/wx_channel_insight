package sqlite

import (
	"context"
	"database/sql"

	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type WorkspaceRepository struct{ c *Client }

func NewWorkspaceRepository(c *Client) (*WorkspaceRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &WorkspaceRepository{c}, nil
}
func (r *WorkspaceRepository) Save(ctx context.Context, item workspace.DefaultWorkspace) error {
	return put(ctx, r.c.db, "workspace", item.OwnerUserID, "default", item)
}
func (r *WorkspaceRepository) UpsertDefaultWorkspace(ctx context.Context, owner string, build func(workspace.DefaultWorkspace, bool) (workspace.DefaultWorkspace, string, error)) (next workspace.DefaultWorkspace, resolution string, err error) {
	err = r.c.update(ctx, func(tx *sql.Tx) error {
		existing, found, err := get[workspace.DefaultWorkspace](ctx, tx, "workspace", owner, "default")
		if err != nil {
			return err
		}
		next, resolution, err = build(existing, found)
		if err != nil {
			return err
		}
		return put(ctx, tx, "workspace", owner, "default", next)
	})
	return
}

var _ workspace.DefaultWorkspaceRepository = (*WorkspaceRepository)(nil)
