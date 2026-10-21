package insightfile

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestVideoCurationReopensWithWorkspaceIsolationAndIndependentTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "curations.json")
	r, err := NewVideoCurationRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	items := []insight.VideoCuration{{WorkspaceID: "a", VideoID: "video:1", Status: "selected", Note: "优质开场", Tags: []string{"参考"}, UpdatedAt: time.Now().UTC()}, {WorkspaceID: "b", VideoID: "video:1", Status: "excluded", Tags: []string{}}}
	if err = r.SaveBatch(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	items[0].Tags[0] = "mutated"
	r, err = NewVideoCurationRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := r.List(context.Background(), "a")
	if err != nil || len(rows) != 1 || rows[0].Status != "selected" || rows[0].Note != "优质开场" || rows[0].Tags[0] != "参考" {
		t.Fatalf("reopen lost decision or leaked scope: %+v %v", rows, err)
	}
	rows[0].Tags[0] = "mutated-again"
	rows, _ = r.List(context.Background(), "a")
	if rows[0].Tags[0] != "参考" {
		t.Fatal("caller mutated stored tags")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = r.SaveBatch(ctx, items); err == nil {
		t.Fatal("cancelled write saved")
	}
}
