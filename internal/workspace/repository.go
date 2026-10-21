package workspace

import "context"

type DefaultWorkspaceRepository interface {
	UpsertDefaultWorkspace(ctx context.Context, ownerUserID string, build func(existing DefaultWorkspace, found bool) (DefaultWorkspace, string, error)) (DefaultWorkspace, string, error)
	Save(ctx context.Context, workspace DefaultWorkspace) error
}
