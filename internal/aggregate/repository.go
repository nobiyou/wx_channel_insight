package aggregate

import "context"

type Repository interface {
	ReplaceAggregate(ctx context.Context, workspaceID string, aggregate Aggregate) error
	GetSummary(ctx context.Context, workspaceID string) (Summary, bool, error)
	GetAggregate(ctx context.Context, workspaceID string) (Aggregate, bool, error)
	Reset(ctx context.Context, workspaceID string) error
}
