package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

func TestOperationsAutomationCoalescesWithoutBlockingCollectionAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan insight.DataCenterQuery, 8)
	release := make(chan struct{}, 8)
	failed := make(chan error, 8)
	worker := newOperationsAutomation(ctx, func(ctx context.Context, q insight.DataCenterQuery) error {
		started <- q
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return errors.New("AI failed; collection is already committed")
		}
	}, func(_ string, err error) { failed <- err })
	defer worker.Close()
	notify := func(id string) {
		worker.Notify(workspace.ResolvedContext{Workspace: workspace.DefaultWorkspace{ID: "ws-" + id, OwnerUserID: id, DeviceCount: 2}})
	}
	notify("alice")
	select {
	case q := <-started:
		if q.HubUserID != "alice" || q.DeviceCount != 2 {
			t.Fatalf("lost workspace: %+v", q)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	// Notification stays fast even while the model is blocked. Repeated events
	// retain one follow-up per workspace, and another workspace is not lost.
	for i := 0; i < 100; i++ {
		notify("alice")
	}
	notify("bob")
	release <- struct{}{}
	select {
	case <-failed:
	case <-time.After(time.Second):
		t.Fatal("background failure hidden")
	}
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case q := <-started:
			seen[q.HubUserID] = true
			release <- struct{}{}
		case <-time.After(time.Second):
			t.Fatal("pending workspace lost")
		}
	}
	if !seen["alice"] || !seen["bob"] {
		t.Fatalf("workspace collision: %+v", seen)
	}
	cancel()
	worker.Close()
	notify("after-stop")
	select {
	case q := <-started:
		t.Fatalf("work started after stop: %+v", q)
	default:
	}
}
