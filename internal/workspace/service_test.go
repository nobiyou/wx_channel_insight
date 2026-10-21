package workspace_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type stubRepo struct {
	mu    sync.Mutex
	items map[string]workspace.DefaultWorkspace
}

func newStubRepo() *stubRepo {
	return &stubRepo{
		items: make(map[string]workspace.DefaultWorkspace),
	}
}

func (r *stubRepo) Save(_ context.Context, item workspace.DefaultWorkspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.OwnerUserID] = item
	return nil
}

func (r *stubRepo) UpsertDefaultWorkspace(_ context.Context, ownerUserID string, build func(existing workspace.DefaultWorkspace, found bool) (workspace.DefaultWorkspace, string, error)) (workspace.DefaultWorkspace, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, found := r.items[ownerUserID]
	next, resolution, err := build(existing, found)
	if err != nil {
		return workspace.DefaultWorkspace{}, "", err
	}
	r.items[ownerUserID] = next
	return next, resolution, nil
}

func TestEnsureDefaultWorkspaceCreatesDefaultWorkspace(t *testing.T) {
	repo := newStubRepo()
	service := workspace.NewDefaultWorkspaceService(repo)
	service.SetNow(func() time.Time {
		return time.Date(2026, 6, 30, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	})

	result, err := service.EnsureDefaultWorkspace(context.Background(), workspace.EnsureDefaultWorkspaceInput{
		HubUserID:   "hub:user:alice",
		DeviceCount: 3,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if result.Resolution != "created" {
		t.Fatalf("expected created resolution, got %q", result.Resolution)
	}
	if result.Workspace.ID != "ws-default-hub-user-alice" {
		t.Fatalf("expected normalized workspace id, got %q", result.Workspace.ID)
	}
	if result.Workspace.Name != "默认运营工作区" {
		t.Fatalf("expected default workspace name, got %q", result.Workspace.Name)
	}
	if result.Workspace.DeviceCount != 3 {
		t.Fatalf("expected device count 3, got %d", result.Workspace.DeviceCount)
	}
}

func TestEnsureDefaultWorkspaceReusesExistingWorkspace(t *testing.T) {
	repo := newStubRepo()
	repo.items["hub:user:alice"] = workspace.DefaultWorkspace{
		ID:            "ws-default-hub-user-alice",
		Name:          "默认运营工作区",
		OwnerUserID:   "hub:user:alice",
		DeviceCount:   3,
		BootstrapMode: "lazy_create",
		Source:        "insight_default_workspace",
		State:         "ready",
	}
	service := workspace.NewDefaultWorkspaceService(repo)

	result, err := service.EnsureDefaultWorkspace(context.Background(), workspace.EnsureDefaultWorkspaceInput{
		HubUserID:   "hub:user:alice",
		DeviceCount: 3,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if result.Resolution != "reused" {
		t.Fatalf("expected reused resolution, got %q", result.Resolution)
	}
}

func TestEnsureDefaultWorkspaceUpdatesDeviceCount(t *testing.T) {
	repo := newStubRepo()
	repo.items["hub:user:alice"] = workspace.DefaultWorkspace{
		ID:            "ws-default-hub-user-alice",
		Name:          "默认运营工作区",
		OwnerUserID:   "hub:user:alice",
		DeviceCount:   3,
		BootstrapMode: "lazy_create",
		Source:        "insight_default_workspace",
		State:         "ready",
	}
	service := workspace.NewDefaultWorkspaceService(repo)
	service.SetNow(func() time.Time {
		return time.Date(2026, 6, 30, 13, 0, 0, 0, time.FixedZone("CST", 8*3600))
	})

	result, err := service.EnsureDefaultWorkspace(context.Background(), workspace.EnsureDefaultWorkspaceInput{
		HubUserID:   "hub:user:alice",
		DeviceCount: 6,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if result.Resolution != "updated" {
		t.Fatalf("expected updated resolution, got %q", result.Resolution)
	}
	if result.Workspace.DeviceCount != 6 {
		t.Fatalf("expected updated device count 6, got %d", result.Workspace.DeviceCount)
	}
}

func TestEnsureDefaultWorkspaceConcurrentFirstVisitCreatesOnlyOnce(t *testing.T) {
	repo := newStubRepo()
	service := workspace.NewDefaultWorkspaceService(repo)
	service.SetNow(func() time.Time {
		return time.Date(2026, 6, 30, 13, 30, 0, 0, time.FixedZone("CST", 8*3600))
	})

	results := make(chan string, 2)
	run := func() {
		result, err := service.EnsureDefaultWorkspace(context.Background(), workspace.EnsureDefaultWorkspaceInput{
			HubUserID:   "hub:user:race",
			DeviceCount: 5,
		})
		if err != nil {
			results <- "error"
			return
		}
		results <- result.Resolution
	}

	go run()
	go run()

	first := <-results
	second := <-results

	if first == "error" || second == "error" {
		t.Fatal("expected no errors from concurrent ensure calls")
	}
	if !((first == "created" && second == "reused") || (first == "reused" && second == "created")) {
		t.Fatalf("expected created/reused pair, got %q and %q", first, second)
	}
}
