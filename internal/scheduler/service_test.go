package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingRunner struct {
	mu        sync.Mutex
	started   int
	active    int
	maxActive int
	errors    []error
	block     bool
	startedCh chan struct{}
}

func (r *recordingRunner) RunCycle(ctx context.Context) error {
	r.mu.Lock()
	r.started++
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	index := r.started - 1
	if r.startedCh != nil {
		select {
		case r.startedCh <- struct{}{}:
		default:
		}
	}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.active--
		r.mu.Unlock()
	}()

	if r.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if index < len(r.errors) {
		return r.errors[index]
	}
	return nil
}

func (r *recordingRunner) stats() (started, maxActive int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started, r.maxActive
}

func TestServiceRunsSynchronouslyContinuesAfterRunnerErrorAndWaitsAfterEachRun(t *testing.T) {
	runner := &recordingRunner{errors: []error{errors.New("temporary cycle failure")}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits []time.Duration
	wait := func(ctx context.Context, interval time.Duration) error {
		waits = append(waits, interval)
		if len(waits) == 2 {
			cancel()
		}
		return nil
	}
	var logs bytes.Buffer
	service := NewService(runner, Config{Interval: time.Minute, RunOnStart: true}, WithWait(wait), WithLogger(log.New(&logs, "", 0)))

	if err := service.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	started, maxActive := runner.stats()
	if started != 2 || maxActive != 1 {
		t.Fatalf("expected two non-overlapping runs, got started=%d maxActive=%d", started, maxActive)
	}
	if len(waits) != 2 || waits[0] != time.Minute || waits[1] != time.Minute {
		t.Fatalf("expected one wait after each completed run, got %v", waits)
	}
	if !strings.Contains(logs.String(), "temporary cycle failure") {
		t.Fatalf("expected runner error to be logged, got %q", logs.String())
	}
}

func TestServiceWaitsBeforeFirstRunWhenRunOnStartIsFalse(t *testing.T) {
	runner := &recordingRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []string
	waitCount := 0
	wait := func(context.Context, time.Duration) error {
		events = append(events, "wait")
		waitCount++
		if waitCount == 2 {
			cancel()
		}
		return nil
	}
	wrappedRunner := cycleRunnerFunc(func(ctx context.Context) error {
		events = append(events, "run")
		return runner.RunCycle(ctx)
	})
	service := NewService(wrappedRunner, Config{Interval: time.Second, RunOnStart: false}, WithWait(wait))

	if err := service.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if strings.Join(events, ",") != "wait,run,wait" {
		t.Fatalf("expected wait before first run and after it, got %v", events)
	}
}

func TestServiceCancellationCancelsActiveRunnerAndReturnsNil(t *testing.T) {
	started := make(chan struct{}, 1)
	runner := &recordingRunner{block: true, startedCh: started}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(runner, Config{Interval: time.Minute, RunOnStart: true}, WithWait(func(context.Context, time.Duration) error {
		t.Fatal("wait should not be called while the active runner is canceled")
		return nil
	}))

	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after canceling the active runner")
	}
}

func TestServiceReturnsNonCancellationWaitErrors(t *testing.T) {
	waitErr := errors.New("wait failed")
	service := NewService(&recordingRunner{}, Config{Interval: time.Minute, RunOnStart: true}, WithWait(func(context.Context, time.Duration) error {
		return waitErr
	}))
	if err := service.Run(context.Background()); !errors.Is(err, waitErr) {
		t.Fatalf("expected wait error %v, got %v", waitErr, err)
	}
}

type cycleRunnerFunc func(context.Context) error

func (f cycleRunnerFunc) RunCycle(ctx context.Context) error {
	return f(ctx)
}
