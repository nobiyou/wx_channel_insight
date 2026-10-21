package insightpg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func TestVideoCurationPostgresPersistsIsolatesAndRollsBackBatch(t *testing.T) {
	db, _ := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewVideoCurationRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	workspace := fmt.Sprintf("curation-test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM insight_video_curations WHERE workspace_id IN ($1,$2)", workspace, workspace+"-other"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	base := insight.VideoCuration{WorkspaceID: workspace, VideoID: "video:1", Status: "selected", Note: "复盘备注", Tags: []string{"高转发"}, UpdatedAt: time.Now().UTC(), UpdatedBy: "tester"}
	other := base
	other.WorkspaceID += "-other"
	other.Status = "excluded"
	if err = r.SaveBatch(ctx, []insight.VideoCuration{base, other}); err != nil {
		t.Fatal(err)
	}
	r, err = NewVideoCurationRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	items, err := r.List(ctx, workspace)
	if err != nil || len(items) != 1 || items[0].Status != "selected" || items[0].Note != "复盘备注" || items[0].Tags[0] != "高转发" {
		t.Fatalf("round trip: %+v %v", items, err)
	}
	changed := base
	changed.Status = "watching"
	invalid := base
	invalid.VideoID = "video:2"
	invalid.Status = "invalid"
	if err = r.SaveBatch(ctx, []insight.VideoCuration{changed, invalid}); err == nil {
		t.Fatal("invalid batch unexpectedly committed")
	}
	items, err = r.List(ctx, workspace)
	if err != nil || len(items) != 1 || items[0].Status != "selected" {
		t.Fatalf("failed batch partially committed: %+v %v", items, err)
	}
}
