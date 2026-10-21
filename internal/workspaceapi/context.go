package workspaceapi

import (
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type ContextResponse struct {
	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName"`
	HubUserID     string `json:"hubUserId"`
	DeviceCount   int    `json:"deviceCount"`
	ContextSource string `json:"contextSource"`
	BootstrapMode string `json:"bootstrapMode"`
	Source        string `json:"source"`
	State         string `json:"state"`
	Resolution    string `json:"resolution"`
	LastSyncedAt  string `json:"lastSyncedAt"`
}

func NewContextResponse(resolved workspace.ResolvedContext) ContextResponse {
	return ContextResponse{
		WorkspaceID:   resolved.Workspace.ID,
		WorkspaceName: resolved.Workspace.Name,
		HubUserID:     resolved.Workspace.OwnerUserID,
		DeviceCount:   resolved.Workspace.DeviceCount,
		ContextSource: resolved.ContextSource,
		BootstrapMode: resolved.Workspace.BootstrapMode,
		Source:        resolved.Workspace.Source,
		State:         resolved.Workspace.State,
		Resolution:    resolved.Resolution,
		LastSyncedAt:  formatTime(resolved.Workspace.LastSyncedAt),
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
