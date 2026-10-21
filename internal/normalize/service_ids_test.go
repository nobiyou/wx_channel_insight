package normalize

import (
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

func TestBuildStableIDBoundsOversizedPartsWithoutTruncationCollisions(t *testing.T) {
	base := strings.Repeat("long-title-", 80)

	first := buildStableID("channel", base+"a")
	second := buildStableID("channel", base+"b")

	if len(first) > maxStableIDBytes || len(second) > maxStableIDBytes {
		t.Fatalf("stable IDs exceed persistence key budget: first=%d second=%d", len(first), len(second))
	}
	if first == second {
		t.Fatalf("distinct oversized inputs must keep distinct stable IDs: %q", first)
	}
	if first != buildStableID("channel", base+"a") {
		t.Fatalf("stable ID must remain deterministic: %q", first)
	}
}

func TestBuildSnapshotPreservesChannelUsernameForContentAndAccount(t *testing.T) {
	context := workspace.ResolvedContext{
		Workspace: workspace.DefaultWorkspace{ID: "ws-test", OwnerUserID: "hub:user:test"},
	}
	snapshot := BuildSnapshot(context, []ingest.Event{{
		ID:              "event-1",
		BatchID:         "batch-1",
		HubUserID:       "hub:user:test",
		EventType:       "video_snapshot",
		Source:          ingest.RunSourceWXFeedList,
		CollectedAt:     time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC),
		ObjectID:        "video-1",
		ChannelUsername: "v2_real_account@finder",
		ChannelName:     "真实账号",
		Title:           "真实视频",
		LikeCount:       12,
	}}, time.Date(2026, 8, 6, 8, 1, 0, 0, time.UTC))

	if len(snapshot.ContentSnapshots) != 1 || snapshot.ContentSnapshots[0].ChannelUsername != "v2_real_account@finder" {
		t.Fatalf("expected normalized content username, got %+v", snapshot.ContentSnapshots)
	}
	if len(snapshot.AccountObservations) != 1 || snapshot.AccountObservations[0].Username != "v2_real_account@finder" {
		t.Fatalf("expected normalized account username, got %+v", snapshot.AccountObservations)
	}
}
