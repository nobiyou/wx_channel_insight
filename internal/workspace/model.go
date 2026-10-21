package workspace

import "time"

type DefaultWorkspace struct {
	ID            string
	Name          string
	OwnerUserID   string
	DeviceCount   int
	BootstrapMode string
	Source        string
	State         string
	CreatedAt     time.Time
	LastSyncedAt  time.Time
}

type EnsureDefaultWorkspaceInput struct {
	HubUserID   string
	DeviceCount int
}

type EnsureDefaultWorkspaceResult struct {
	Workspace  DefaultWorkspace
	Resolution string
}
