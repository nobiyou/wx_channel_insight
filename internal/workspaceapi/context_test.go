package workspaceapi_test

import (
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspaceapi"
)

func TestNewContextResponseMapsResolvedContext(t *testing.T) {
	response := workspaceapi.NewContextResponse(workspace.ResolvedContext{
		Workspace: workspace.DefaultWorkspace{
			ID:            "ws-default-hub-user-alice",
			Name:          "默认运营工作区",
			OwnerUserID:   "hub:user:alice",
			DeviceCount:   9,
			BootstrapMode: "lazy_create",
			Source:        "insight_default_workspace",
			State:         "ready",
			LastSyncedAt:  time.Date(2026, 6, 30, 16, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		},
		ContextSource: "header",
		Resolution:    "created",
	})

	if response.WorkspaceID != "ws-default-hub-user-alice" {
		t.Fatalf("expected workspace id to map, got %q", response.WorkspaceID)
	}
	if response.ContextSource != "header" {
		t.Fatalf("expected header context source, got %q", response.ContextSource)
	}
	if response.LastSyncedAt != "2026-06-30T16:00:00+08:00" {
		t.Fatalf("expected RFC3339 lastSyncedAt, got %q", response.LastSyncedAt)
	}
}
