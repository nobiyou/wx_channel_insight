package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	runtimecfg "github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestBuildIngestServiceSQLitePersistsLightweightCommands(t *testing.T) {
	root := t.TempDir()
	cfg := runtimecfg.LoadConfigFromLookup(func(key string) string {
		switch key {
		case "INSIGHT_STORE":
			return "sqlite"
		case "INSIGHT_SQLITE_PATH":
			return filepath.Join(root, "insight.db")
		case "INSIGHT_DATA_DIR":
			return root
		case "INSIGHT_POSTGRES_DSN":
			return "unused-postgres-dsn"
		}
		return ""
	})
	service, closeService, err := buildIngestService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	query := ingest.SummaryQuery{HubUserID: "test-owner", DeviceCount: 1}
	_, err = service.ImportEvents(ctx, query, []ingest.Event{{ID: "event", HubUserID: "test-owner", BatchID: "batch", Source: ingest.RunSourceWXFeedList, MachineID: "test", CollectedAt: time.Now().UTC(), EventType: "video_snapshot", Title: "persisted", VideoCount: 1}})
	closeService()
	if err != nil {
		t.Fatal(err)
	}
	service, closeService, err = buildIngestService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer closeService()
	items, err := service.ListEvents(ctx, query)
	if err != nil || len(items) != 1 {
		t.Fatalf("lightweight jobs did not persist SQLite events: %v", err)
	}
}
