package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestSQLiteRuntimeSeesJobsWritesAndPersistsAfterRestart(t *testing.T) {
	root := t.TempDir()
	cfg := LoadConfigFromLookup(func(key string) string {
		switch key {
		case "INSIGHT_STORE":
			return "sqlite"
		case "INSIGHT_SQLITE_PATH":
			return filepath.Join(root, "insight.db")
		case "INSIGHT_DATA_DIR":
			return root
		case "INSIGHT_DEFAULT_HUB_USER_ID":
			return "test-owner"
		case "INSIGHT_DEFAULT_DEVICE_COUNT":
			return "1"
		}
		return ""
	})
	api, err := BuildDashboardRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	jobs, err := BuildDashboardRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	query := ingest.SummaryQuery{HubUserID: "test-owner", DeviceCount: 1}
	_, err = jobs.IngestService.ImportEvents(ctx, query, []ingest.Event{{ID: "e-1", BatchID: "batch-1", HubUserID: "test-owner", MachineID: "test-machine", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: time.Now().UTC(), ObjectID: "video-1", ChannelUsername: "account-1", ChannelName: "Test", Title: "persisted video", VideoCount: 1, LikeCount: 10}})
	if err != nil {
		jobs.Close()
		t.Fatal(err)
	}
	dashboardQuery := insight.DashboardQuery{HubUserID: "test-owner", DeviceCount: 1, ContextSource: "jobs"}
	snapshot, err := jobs.ReadModelService.RefreshDashboard(ctx, dashboardQuery, "jobs")
	if err != nil {
		jobs.Close()
		t.Fatal(err)
	}
	workspaceID := snapshot.WorkspaceBridge.WorkspaceID
	if err := jobs.CommentRepository.Upsert(ctx, "test-owner", []ingest.RawCommentRecord{{VideoObjectID: "video-1", CommentID: "comment-1", Content: "evidence"}}); err != nil {
		jobs.Close()
		t.Fatal(err)
	}
	jobs.Close()
	// The API handle was opened before the jobs write; it must not retain a stale snapshot.
	seen, err := api.DashboardService.GetDashboard(ctx, dashboardQuery)
	if err != nil || seen.ReadModel.LatestBatchID != "batch-1" {
		t.Fatalf("API did not see jobs refresh: %v", err)
	}
	restarted, err := BuildDashboardRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	events, err := restarted.IngestService.ListEvents(ctx, query)
	if err != nil || len(events) != 1 || events[0].ObjectID != "video-1" {
		t.Fatalf("events after restart: %v", err)
	}
	accounts, err := restarted.AccountRepository.List(ctx, workspaceID)
	if err != nil || len(accounts) != 1 || accounts[0].ChannelUsername != "account-1" {
		t.Fatalf("accounts after restart: %v", err)
	}
	comments, err := restarted.CommentRepository.List(ctx, "test-owner", ingest.RawCommentQuery{Page: 1, PageSize: 20})
	if err != nil || comments.Total != 1 || comments.Items[0].Content != "evidence" {
		t.Fatalf("comments after restart: %v", err)
	}
	if api.SQLiteClient == nil || api.PostgresClient != nil {
		t.Fatal("incorrect storage runtime")
	}
	jsonFiles, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil || len(jsonFiles) != 0 {
		t.Fatalf("SQLite mode unexpectedly created JSON stores: %v", jsonFiles)
	}
}

func TestSQLiteRuntimeFailsWithoutFallingBackToFile(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(bad, []byte("block directory creation"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := LoadConfigFromLookup(func(key string) string {
		if key == "INSIGHT_STORE" {
			return "sqlite"
		}
		if key == "INSIGHT_SQLITE_PATH" {
			return filepath.Join(bad, "insight.db")
		}
		return ""
	})
	if runtime, err := BuildDashboardRuntime(cfg); err == nil {
		runtime.Close()
		t.Fatal("SQLite initialization failure must not choose another backend")
	}
}

func TestSQLiteRuntimeDoesNotConnectUnusedPostgres(t *testing.T) {
	root := t.TempDir()
	cfg := LoadConfigFromLookup(func(key string) string {
		switch key {
		case "INSIGHT_STORE":
			return "sqlite"
		case "INSIGHT_SQLITE_PATH":
			return filepath.Join(root, "insight.db")
		case "INSIGHT_POSTGRES_DSN":
			return "unused-postgres-dsn"
		}
		return ""
	})
	r, err := BuildDashboardRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.PostgresClient != nil {
		t.Fatal("SQLite runtime opened an unused postgres connection")
	}
}
