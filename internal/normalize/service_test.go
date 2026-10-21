package normalize_test

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspacedemo"
)

func TestServiceNormalizesAndPersistsSnapshot(t *testing.T) {
	repo := normalize.NewMemoryRepository()
	ingestRepo := ingestdemo.NewRepository()
	base := time.Date(2026, 7, 5, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	callCount := 0
	ingestService := ingest.NewService(ingestRepo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			value := base.Add(time.Duration(callCount) * 5 * time.Minute)
			callCount++
			return value
		},
	})
	for i := 0; i < 2; i++ {
		if _, err := ingestService.AppendMockBatch(context.Background(), ingest.SummaryQuery{
			HubUserID:   "hub:user:alice",
			DeviceCount: 3,
		}); err != nil {
			t.Fatalf("append mock batch failed: %v", err)
		}
	}

	contextResolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 6,
		},
	)

	service := normalize.NewService(repo, normalize.ServiceConfig{
		WorkspaceContextResolver: contextResolver,
		IngestEvents:             ingestService,
		Now: func() time.Time {
			return base.Add(30 * time.Minute)
		},
	})

	snapshot, err := service.Normalize(context.Background(), normalize.Query{
		HubUserID:     "hub:user:alice",
		DeviceCount:   3,
		ContextSource: "jobs",
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if snapshot.Summary.State != "ready" {
		t.Fatalf("expected ready summary, got %+v", snapshot.Summary)
	}
	if snapshot.Summary.ContentCount == 0 || snapshot.Summary.TopicCount == 0 {
		t.Fatalf("expected content/topic counts, got %+v", snapshot.Summary)
	}
	if snapshot.Summary.CommentCount == 0 || snapshot.Summary.AccountCount == 0 {
		t.Fatalf("expected comment/account counts, got %+v", snapshot.Summary)
	}
	if snapshot.Summary.WorkspaceID == "" || snapshot.Summary.HubUserID != "hub:user:alice" {
		t.Fatalf("expected workspace + hub user metadata, got %+v", snapshot.Summary)
	}
	if snapshot.Summary.GeneratedAt.IsZero() || snapshot.Summary.LatestEventAt.IsZero() {
		t.Fatalf("expected timestamps, got %+v", snapshot.Summary)
	}
	if len(snapshot.ContentSnapshots) != snapshot.Summary.ContentCount {
		t.Fatalf("expected persisted content snapshots, got %+v", snapshot.ContentSnapshots)
	}

	persisted, found, err := service.GetSummary(context.Background(), snapshot.Summary.WorkspaceID)
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}
	if !found {
		t.Fatalf("expected persisted normalization summary")
	}
	if persisted.LatestBatchID != snapshot.Summary.LatestBatchID || persisted.ContentCount != snapshot.Summary.ContentCount {
		t.Fatalf("unexpected persisted summary: %+v", persisted)
	}
}

func TestBuildSnapshotReturnsEmptyStateWithoutEvents(t *testing.T) {
	workspaceContext := workspace.ResolvedContext{
		Workspace: workspace.DefaultWorkspace{
			ID:          "ws-default-hub-user-demo",
			OwnerUserID: "hub:user:demo",
		},
	}
	generatedAt := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	snapshot := normalize.BuildSnapshot(workspaceContext, nil, generatedAt)
	if snapshot.Summary.State != "empty" {
		t.Fatalf("expected empty state, got %+v", snapshot.Summary)
	}
	if snapshot.Summary.GeneratedAt != generatedAt {
		t.Fatalf("expected generatedAt to be preserved, got %+v", snapshot.Summary)
	}
	if snapshot.Summary.ContentCount != 0 || snapshot.Summary.AccountCount != 0 {
		t.Fatalf("expected zero counts, got %+v", snapshot.Summary)
	}
}
