package workspacefile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspacefile"
)

func TestDefaultWorkspaceRepositoryPersistsAcrossInstances(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "workspace-state.json")

	repo, err := workspacefile.NewDefaultWorkspaceRepository(storePath)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	saved := workspace.DefaultWorkspace{
		ID:            "ws-default-hub-user-alice",
		Name:          "默认运营工作区",
		OwnerUserID:   "hub:user:alice",
		DeviceCount:   9,
		BootstrapMode: "lazy_create",
		Source:        "insight_default_workspace",
		State:         "ready",
		CreatedAt:     time.Date(2026, 6, 30, 16, 30, 0, 0, time.FixedZone("CST", 8*3600)),
		LastSyncedAt:  time.Date(2026, 6, 30, 16, 31, 0, 0, time.FixedZone("CST", 8*3600)),
	}
	if err := repo.Save(context.Background(), saved); err != nil {
		t.Fatalf("expected save success, got %v", err)
	}

	reloaded, err := workspacefile.NewDefaultWorkspaceRepository(storePath)
	if err != nil {
		t.Fatalf("expected reload success, got %v", err)
	}

	got, resolution, err := reloaded.UpsertDefaultWorkspace(context.Background(), "hub:user:alice", func(existing workspace.DefaultWorkspace, found bool) (workspace.DefaultWorkspace, string, error) {
		if !found {
			t.Fatal("expected persisted workspace to exist")
		}
		return existing, "reused", nil
	})
	if err != nil {
		t.Fatalf("expected upsert lookup success, got %v", err)
	}
	if resolution != "reused" {
		t.Fatalf("expected reused resolution, got %q", resolution)
	}
	if got.ID != "ws-default-hub-user-alice" {
		t.Fatalf("expected persisted workspace id, got %q", got.ID)
	}
	if got.DeviceCount != 9 {
		t.Fatalf("expected persisted device count 9, got %d", got.DeviceCount)
	}
}

func TestDefaultWorkspaceRepositoryRefreshesAcrossLiveInstances(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "workspace-state.json")

	writer, err := workspacefile.NewDefaultWorkspaceRepository(storePath)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	reader, err := workspacefile.NewDefaultWorkspaceRepository(storePath)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	saved := workspace.DefaultWorkspace{
		ID:            "ws-default-hub-user-bob",
		Name:          "默认运营工作区",
		OwnerUserID:   "hub:user:bob",
		DeviceCount:   5,
		BootstrapMode: "lazy_create",
		Source:        "insight_default_workspace",
		State:         "ready",
		CreatedAt:     time.Date(2026, 7, 3, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		LastSyncedAt:  time.Date(2026, 7, 3, 10, 1, 0, 0, time.FixedZone("CST", 8*3600)),
	}
	if err := writer.Save(context.Background(), saved); err != nil {
		t.Fatalf("expected save success, got %v", err)
	}

	got, resolution, err := reader.UpsertDefaultWorkspace(context.Background(), "hub:user:bob", func(existing workspace.DefaultWorkspace, found bool) (workspace.DefaultWorkspace, string, error) {
		if !found {
			t.Fatal("expected live workspace refresh to find persisted item")
		}
		return existing, "reused", nil
	})
	if err != nil {
		t.Fatalf("expected upsert lookup success, got %v", err)
	}
	if resolution != "reused" {
		t.Fatalf("expected reused resolution, got %q", resolution)
	}
	if got.ID != "ws-default-hub-user-bob" {
		t.Fatalf("expected persisted workspace id, got %q", got.ID)
	}
	if got.DeviceCount != 5 {
		t.Fatalf("expected persisted device count 5, got %d", got.DeviceCount)
	}
}
