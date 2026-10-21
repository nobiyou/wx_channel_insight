package workspace_test

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

func TestContextResolverFallsBackToDefaults(t *testing.T) {
	repo := newStubRepo()
	bootstrap := workspace.NewDefaultWorkspaceService(repo)
	bootstrap.SetNow(func() time.Time {
		return time.Date(2026, 6, 30, 15, 30, 0, 0, time.FixedZone("CST", 8*3600))
	})
	resolver := workspace.NewContextResolver(bootstrap, workspace.ContextResolverConfig{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 6,
	})

	result, err := resolver.Resolve(context.Background(), workspace.ContextQuery{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if result.Workspace.OwnerUserID != "hub:user:demo" {
		t.Fatalf("expected default hub user id, got %q", result.Workspace.OwnerUserID)
	}
	if result.Workspace.DeviceCount != 6 {
		t.Fatalf("expected default device count 6, got %d", result.Workspace.DeviceCount)
	}
	if result.ContextSource != "default" {
		t.Fatalf("expected default context source, got %q", result.ContextSource)
	}
}

func TestContextResolverPreservesExplicitValues(t *testing.T) {
	repo := newStubRepo()
	bootstrap := workspace.NewDefaultWorkspaceService(repo)
	bootstrap.SetNow(func() time.Time {
		return time.Date(2026, 6, 30, 15, 31, 0, 0, time.FixedZone("CST", 8*3600))
	})
	resolver := workspace.NewContextResolver(bootstrap, workspace.ContextResolverConfig{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 6,
	})

	result, err := resolver.Resolve(context.Background(), workspace.ContextQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   9,
		ContextSource: "header",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if result.Workspace.OwnerUserID != "hub:user:alice" {
		t.Fatalf("expected explicit hub user id, got %q", result.Workspace.OwnerUserID)
	}
	if result.Workspace.DeviceCount != 9 {
		t.Fatalf("expected explicit device count 9, got %d", result.Workspace.DeviceCount)
	}
	if result.ContextSource != "header" {
		t.Fatalf("expected header context source, got %q", result.ContextSource)
	}
}
