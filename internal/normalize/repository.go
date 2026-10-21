package normalize

import "context"

type Repository interface {
	ReplaceSnapshot(ctx context.Context, workspaceID string, snapshot Snapshot) error
	GetSummary(ctx context.Context, workspaceID string) (Summary, bool, error)
	GetSnapshot(ctx context.Context, workspaceID string) (Snapshot, bool, error)
	Reset(ctx context.Context, workspaceID string) error
}
