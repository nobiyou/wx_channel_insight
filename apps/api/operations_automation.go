package main

import (
	"context"
	"sync"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

// One worker bounds model concurrency. Events received during an inspection
// retain one follow-up per workspace so the latest committed facts are checked.
type operationsAutomation struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	wake    chan struct{}
	mu      sync.Mutex
	pending map[string]insight.DataCenterQuery
	order   []string
	inspect func(context.Context, insight.DataCenterQuery) error
	report  func(string, error)
}

func newOperationsAutomation(ctx context.Context, inspect func(context.Context, insight.DataCenterQuery) error, report func(string, error)) *operationsAutomation {
	ctx, cancel := context.WithCancel(ctx)
	w := &operationsAutomation{ctx: ctx, cancel: cancel, done: make(chan struct{}), wake: make(chan struct{}, 1), pending: map[string]insight.DataCenterQuery{}, inspect: inspect, report: report}
	go w.run()
	return w
}

func (w *operationsAutomation) Notify(resolved workspace.ResolvedContext) {
	owner, id := resolved.Workspace.OwnerUserID, resolved.Workspace.ID
	if owner == "" || id == "" || w.ctx.Err() != nil {
		return
	}
	w.mu.Lock()
	if _, ok := w.pending[id]; !ok {
		w.order = append(w.order, id)
	}
	w.pending[id] = insight.DataCenterQuery{HubUserID: owner, DeviceCount: resolved.Workspace.DeviceCount, ContextSource: "collection"}
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *operationsAutomation) Close() { w.cancel(); <-w.done }

func (w *operationsAutomation) run() {
	defer close(w.done)
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.wake:
		}
		for w.ctx.Err() == nil {
			w.mu.Lock()
			if len(w.order) == 0 {
				w.mu.Unlock()
				break
			}
			id := w.order[0]
			w.order = w.order[1:]
			q := w.pending[id]
			delete(w.pending, id)
			w.mu.Unlock()
			ctx, cancel := context.WithTimeout(w.ctx, 2*time.Minute)
			err := w.inspect(ctx, q)
			cancel()
			if err != nil && w.ctx.Err() == nil && w.report != nil {
				w.report(id, err)
			}
		}
	}
}
