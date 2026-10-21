package monitoring

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testOperatorRunner struct {
	options []OperatorRunOptions
	results []OperatorRunResult
	errors  []error
}

type blockingTestOperatorRunner struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingTestOperatorRunner) RunMonitoringTask(ctx context.Context, _ OperatorRunOptions) (OperatorRunResult, error) {
	r.once.Do(func() { close(r.started) })
	select {
	case <-r.release:
		return OperatorRunResult{
			Status: RunStatusSuccess, FinalStatus: RunStatusSuccess,
			Attempted: 1, Succeeded: 1, OperatorRunID: "operator-blocking",
		}, nil
	case <-ctx.Done():
		return OperatorRunResult{}, ctx.Err()
	}
}

type leaseObservingRepository struct {
	*MemoryRepository
	renewals chan time.Time
}

func (r *leaseObservingRepository) RenewTaskRunLease(ctx context.Context, hubUserID, runID string, leaseUntil time.Time) error {
	if err := r.MemoryRepository.RenewTaskRunLease(ctx, hubUserID, runID, leaseUntil); err != nil {
		return err
	}
	select {
	case r.renewals <- leaseUntil:
	default:
	}
	return nil
}

type completionContextRepository struct {
	*MemoryRepository
	completed atomic.Bool
}

func (r *completionContextRepository) CompleteTaskRun(ctx context.Context, hubUserID, runID string, update TaskRunUpdate) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("completion used canceled context: %w", err)
	}
	r.completed.Store(true)
	return r.MemoryRepository.CompleteTaskRun(ctx, hubUserID, runID, update)
}

type failingLifecycleRepository struct {
	*MemoryRepository
	renewErr      error
	completionErr error
	renewals      atomic.Int32
	completions   atomic.Int32
	completedRun  TaskRunUpdate
}

func (r *failingLifecycleRepository) RenewTaskRunLease(context.Context, string, string, time.Time) error {
	r.renewals.Add(1)
	return r.renewErr
}

func (r *failingLifecycleRepository) CompleteTaskRun(ctx context.Context, hubUserID, runID string, update TaskRunUpdate) error {
	r.completions.Add(1)
	r.completedRun = update
	if r.completionErr != nil {
		return r.completionErr
	}
	return r.MemoryRepository.CompleteTaskRun(ctx, hubUserID, runID, update)
}

type cancelAwareOperatorRunner struct {
	started chan struct{}
	once    sync.Once
}

func (r *cancelAwareOperatorRunner) RunMonitoringTask(ctx context.Context, _ OperatorRunOptions) (OperatorRunResult, error) {
	r.once.Do(func() { close(r.started) })
	<-ctx.Done()
	return OperatorRunResult{Message: "operator stopped after lease loss"}, ctx.Err()
}

func (r *testOperatorRunner) RunMonitoringTask(_ context.Context, options OperatorRunOptions) (OperatorRunResult, error) {
	r.options = append(r.options, options)
	index := len(r.options) - 1
	var result OperatorRunResult
	if index < len(r.results) {
		result = r.results[index]
	}
	var err error
	if index < len(r.errors) {
		err = r.errors[index]
	}
	return result, err
}

func testBindingResolver(_ context.Context, _ string, refs []BindingRef) ([]TaskTarget, error) {
	if len(refs) == 0 {
		return []TaskTarget{{BindingRef: BindingRef{Source: "wx_feed_list", TargetKey: "account-1"}}}, nil
	}
	for _, ref := range refs {
		if ref.TargetKey != "account-1" {
			return nil, errors.New("saved monitoring target was not found")
		}
	}
	return []TaskTarget{{BindingRef: refs[0], DisplayName: "account-1"}}, nil
}

func TestServiceEnsuresDefaultTaskIdempotently(t *testing.T) {
	now := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	service, err := NewService(NewMemoryRepository(), ServiceConfig{
		DefaultHubUserID:   "operator-default",
		DefaultDeviceCount: 2,
		Now:                func() time.Time { return now },
		BindingResolver:    testBindingResolver,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	first, created, err := service.EnsureDefaultTask(context.Background(), "operator-default")
	if err != nil || !created || first.ScopeMode != ScopeAllSavedBindings {
		t.Fatalf("create default task: task=%#v created=%v err=%v", first, created, err)
	}
	second, created, err := service.EnsureDefaultTask(context.Background(), "operator-default")
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("default task was not idempotent: task=%#v created=%v err=%v", second, created, err)
	}
}

func TestServiceKeepsDefaultTaskReadOnly(t *testing.T) {
	service, err := NewService(NewMemoryRepository(), ServiceConfig{
		DefaultHubUserID: "operator-readonly",
		Now:              func() time.Time { return time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC) },
		BindingResolver:  testBindingResolver,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, _, err := service.EnsureDefaultTask(context.Background(), "operator-readonly")
	if err != nil {
		t.Fatalf("ensure default task: %v", err)
	}

	created.Name = "试图修改默认任务"
	if _, err := service.UpdateTask(context.Background(), "operator-readonly", created); !errors.Is(err, ErrSystemTaskReadOnly) {
		t.Fatalf("expected system task update to be rejected, got %v", err)
	}
	if err := service.ArchiveTask(context.Background(), "operator-readonly", created.ID); !errors.Is(err, ErrSystemTaskReadOnly) {
		t.Fatalf("expected system task archive to be rejected, got %v", err)
	}

	unchanged, err := service.GetTask(context.Background(), "operator-readonly", created.ID)
	if err != nil {
		t.Fatalf("get default task: %v", err)
	}
	if !unchanged.System || unchanged.Name != DefaultTaskName || unchanged.Status != StatusEnabled {
		t.Fatalf("default task changed after rejected mutations: %#v", unchanged)
	}
}

func TestServiceRunDueTaskCreatesDefaultTaskBeforeClaim(t *testing.T) {
	now := time.Date(2026, 8, 11, 11, 30, 0, 0, time.UTC)
	service, err := NewService(NewMemoryRepository(), ServiceConfig{
		DefaultHubUserID: "operator-scheduler",
		Now:              func() time.Time { return now },
		BindingResolver:  testBindingResolver,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.SetOperatorRunner(&testOperatorRunner{results: []OperatorRunResult{{
		Status: "success", FinalStatus: "success", Attempted: 1, Succeeded: 1,
		OperatorRunID: "operator-default-run", FinishedAt: now,
	}}})

	run, err := service.RunDueTask(context.Background(), "operator-scheduler")
	if err != nil {
		t.Fatalf("run due default task: %v", err)
	}
	if run.TaskID != defaultTaskID("operator-scheduler") || run.Status != RunStatusSuccess {
		t.Fatalf("expected scheduler to create and run default task, got %#v", run)
	}
}

func TestServiceRunsSelectedTaskAndPreservesManualScheduleWhenPaused(t *testing.T) {
	now := time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	service, err := NewService(repo, ServiceConfig{
		DefaultHubUserID:   "operator-runner",
		DefaultDeviceCount: 3,
		Now:                func() time.Time { return now },
		BindingResolver:    testBindingResolver,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	runner := &testOperatorRunner{results: []OperatorRunResult{
		{
			Status: "partial", FinalStatus: "partial", Attempted: 1, Succeeded: 1,
			Skipped: 1, OperatorRunID: "operator-run-1", FinishedAt: now.Add(20 * time.Second),
			Errors: []TaskRunError{{Source: "wx_feed_list", TargetKey: "account-1", Status: RunStatusSkipped, Message: "test skip"}},
		},
		{
			Status: "success", FinalStatus: "success", Attempted: 1, Succeeded: 1,
			OperatorRunID: "operator-run-2", FinishedAt: now.Add(30 * time.Second),
		},
	}}
	service.SetOperatorRunner(runner)
	task, err := service.CreateTask(context.Background(), "operator-runner", Task{
		ID:        "task-selected-service",
		Name:      "selected account",
		ScopeMode: ScopeSelected,
		Interval:  5 * time.Minute,
		NextRunAt: now.Add(-time.Minute),
		Targets: []TaskTarget{{BindingRef: BindingRef{
			Source: "wx_feed_list", TargetKey: "account-1",
		}}},
	})
	if err != nil {
		t.Fatalf("create selected task: %v", err)
	}
	completed, err := service.RunDueTask(context.Background(), "operator-runner")
	if err != nil {
		t.Fatalf("run due task: %v", err)
	}
	if completed.Status != RunStatusPartial || completed.OperatorRunID != "operator-run-1" {
		t.Fatalf("unexpected due result: %#v", completed)
	}
	updated, err := service.GetTask(context.Background(), "operator-runner", task.ID)
	if err != nil {
		t.Fatalf("get completed task: %v", err)
	}
	expectedNext := now.Add(20 * time.Second).Add(task.Interval)
	if updated.NextRunAt != expectedNext || updated.LastStatus != RunStatusPartial {
		t.Fatalf("unexpected scheduled next run: %#v expected=%s", updated, expectedNext)
	}
	updated.Status = StatusPaused
	if _, err := service.UpdateTask(context.Background(), "operator-runner", updated); err != nil {
		t.Fatalf("pause task: %v", err)
	}
	manual, err := service.RunManualTask(context.Background(), "operator-runner", task.ID)
	if err != nil {
		t.Fatalf("run paused task manually: %v", err)
	}
	if manual.Status != RunStatusSuccess || manual.OperatorRunID != "operator-run-2" {
		t.Fatalf("unexpected manual result: %#v", manual)
	}
	manualTask, err := service.GetTask(context.Background(), "operator-runner", task.ID)
	if err != nil {
		t.Fatalf("get manual task: %v", err)
	}
	if manualTask.Status != StatusPaused || manualTask.NextRunAt != expectedNext {
		t.Fatalf("manual run changed paused schedule: %#v", manualTask)
	}
	if len(runner.options) != 2 || len(runner.options[0].TargetRefs) != 1 || runner.options[0].TargetRefs[0].TargetKey != "account-1" {
		t.Fatalf("selected refs were not passed to operator: %#v", runner.options)
	}
}

func TestServiceRejectsUnknownSelectedTarget(t *testing.T) {
	service, err := NewService(NewMemoryRepository(), ServiceConfig{BindingResolver: testBindingResolver})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.CreateTask(context.Background(), "operator-invalid", Task{
		Name:      "invalid target",
		ScopeMode: ScopeSelected,
		Interval:  5 * time.Minute,
		Targets:   []TaskTarget{{BindingRef: BindingRef{Source: "wx_feed_list", TargetKey: "missing"}}},
	})
	if err == nil {
		t.Fatal("expected unknown selected target to be rejected")
	}
}

func TestServiceResolvesAllAccountProfileScopeAtRunTime(t *testing.T) {
	now := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	service, err := NewService(NewMemoryRepository(), ServiceConfig{
		DefaultHubUserID: "operator-all-accounts",
		Now:              func() time.Time { return now },
		AccountResolver: func(_ context.Context, _ string, targets []TaskTarget) ([]TaskTarget, error) {
			if targets != nil {
				t.Fatalf("all-account scope should resolve with nil target snapshot: %#v", targets)
			}
			return []TaskTarget{
				{TargetKind: TargetKindAccount, AccountID: "account-1"},
				{TargetKind: TargetKindAccount, AccountID: "account-2"},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	runner := &testOperatorRunner{results: []OperatorRunResult{{
		Status: RunStatusSuccess, FinalStatus: RunStatusSuccess, Attempted: 2, Succeeded: 2,
		FinishedAt: now,
	}}}
	service.SetOperatorRunner(runner)
	task, err := service.CreateTask(context.Background(), "operator-all-accounts", Task{
		Name: "全部账号资料同步", TaskType: TaskTypeAccountProfile,
		Status: StatusEnabled, ScopeMode: ScopeAllAccounts,
		Interval: 30 * time.Minute, NextRunAt: now,
	})
	if err != nil {
		t.Fatalf("create all-account task: %v", err)
	}
	if len(task.Targets) != 0 {
		t.Fatalf("all-account task should not persist target snapshot: %#v", task.Targets)
	}
	if _, err := service.RunManualTask(context.Background(), "operator-all-accounts", task.ID); err != nil {
		t.Fatalf("run all-account task: %v", err)
	}
	if len(runner.options) != 1 || len(runner.options[0].AccountTargets) != 2 || len(runner.options[0].TargetRefs) != 0 {
		t.Fatalf("expected all accounts to be resolved at run time: %#v", runner.options)
	}
}

func TestServiceRenewsLeaseWhileOperatorRunIsActive(t *testing.T) {
	start := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	var nowNanos atomic.Int64
	nowNanos.Store(start.UnixNano())
	repo := &leaseObservingRepository{
		MemoryRepository: NewMemoryRepository(),
		renewals:         make(chan time.Time, 1),
	}
	service, err := NewService(repo, ServiceConfig{
		DefaultHubUserID: "operator-lease-renewal",
		LeaseDuration:    90 * time.Millisecond,
		Now: func() time.Time {
			return time.Unix(0, nowNanos.Load()).UTC()
		},
		BindingResolver: testBindingResolver,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	runner := &blockingTestOperatorRunner{started: make(chan struct{}), release: make(chan struct{})}
	service.SetOperatorRunner(runner)

	done := make(chan error, 1)
	go func() {
		_, runErr := service.RunDueTask(context.Background(), "operator-lease-renewal")
		done <- runErr
	}()
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("operator run did not start")
	}
	nowNanos.Store(start.Add(80 * time.Millisecond).UnixNano())
	select {
	case renewedUntil := <-repo.renewals:
		if !renewedUntil.After(start.Add(90 * time.Millisecond)) {
			t.Fatalf("lease was not extended beyond the original deadline: %s", renewedUntil)
		}
	case <-time.After(time.Second):
		t.Fatal("active operator run did not renew its monitoring lease")
	}
	close(runner.release)
	select {
	case runErr := <-done:
		if runErr != nil {
			t.Fatalf("run due task: %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("operator run did not finish after release")
	}
}

func TestServiceLeaseRenewalFailureCancelsRunnerAndPersistsCausalErrorOnce(t *testing.T) {
	renewErr := errors.New("lease store unavailable")
	repo := &failingLifecycleRepository{
		MemoryRepository: NewMemoryRepository(),
		renewErr:         renewErr,
	}
	service, err := NewService(repo, ServiceConfig{
		DefaultHubUserID: "operator-lease-failure",
		LeaseDuration:    30 * time.Millisecond,
		BindingResolver:  testBindingResolver,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	runner := &cancelAwareOperatorRunner{started: make(chan struct{})}
	service.SetOperatorRunner(runner)

	done := make(chan error, 1)
	go func() {
		_, runErr := service.RunDueTask(context.Background(), "operator-lease-failure")
		done <- runErr
	}()
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("operator run did not start")
	}
	select {
	case runErr := <-done:
		if !errors.Is(runErr, context.Canceled) || !errors.Is(runErr, renewErr) {
			t.Fatalf("expected runner cancellation and renewal failure, got %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("lease renewal failure did not stop the operator run")
	}
	if repo.completions.Load() != 1 {
		t.Fatalf("expected exactly one terminal write, got %d", repo.completions.Load())
	}
	if !strings.Contains(repo.completedRun.Message, "operator stopped after lease loss") ||
		!strings.Contains(repo.completedRun.Message, renewErr.Error()) {
		t.Fatalf("terminal message lost causal errors: %q", repo.completedRun.Message)
	}
	renewalsAtReturn := repo.renewals.Load()
	time.Sleep(50 * time.Millisecond)
	if repo.renewals.Load() != renewalsAtReturn {
		t.Fatalf("lease heartbeat continued after completion: before=%d after=%d", renewalsAtReturn, repo.renewals.Load())
	}
}

func TestServicePreservesExecutionAndCompletionFailures(t *testing.T) {
	runnerErr := errors.New("operator execution failed")
	completionErr := errors.New("monitoring terminal write failed")
	repo := &failingLifecycleRepository{
		MemoryRepository: NewMemoryRepository(),
		completionErr:    completionErr,
	}
	service, err := NewService(repo, ServiceConfig{
		DefaultHubUserID: "operator-completion-failure",
		LeaseDuration:    time.Hour,
		BindingResolver:  testBindingResolver,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.SetOperatorRunner(&testOperatorRunner{errors: []error{runnerErr}})

	_, runErr := service.RunDueTask(context.Background(), "operator-completion-failure")
	if !errors.Is(runErr, runnerErr) || !errors.Is(runErr, completionErr) {
		t.Fatalf("expected execution and completion failures, got %v", runErr)
	}
	if repo.completions.Load() != 1 {
		t.Fatalf("expected one completion attempt, got %d", repo.completions.Load())
	}
}

func TestServicePersistsTerminalStateAfterCallerCancellation(t *testing.T) {
	repo := &completionContextRepository{MemoryRepository: NewMemoryRepository()}
	service, err := NewService(repo, ServiceConfig{
		DefaultHubUserID: "operator-canceled-request",
		BindingResolver:  testBindingResolver,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	runner := &blockingTestOperatorRunner{started: make(chan struct{}), release: make(chan struct{})}
	service.SetOperatorRunner(runner)

	ctx, cancel := context.WithCancel(context.Background())
	type runResult struct {
		run TaskRun
		err error
	}
	done := make(chan runResult, 1)
	go func() {
		run, runErr := service.RunDueTask(ctx, "operator-canceled-request")
		done <- runResult{run: run, err: runErr}
	}()
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("operator run did not start")
	}
	cancel()

	select {
	case result := <-done:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("expected caller cancellation, got run=%#v err=%v", result.run, result.err)
		}
		if result.run.ID == "" || result.run.Status != RunStatusFailed {
			t.Fatalf("expected a persisted failed terminal run, got %#v", result.run)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled operator run did not return")
	}
	if !repo.completed.Load() {
		t.Fatal("terminal task state was not persisted with an independent context")
	}
}

func TestServiceBuildsBoundedAutomationHealthFromEnabledTasks(t *testing.T) {
	now := time.Date(2026, 8, 24, 18, 0, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	for _, task := range []Task{
		{
			ID: "task-success", HubUserID: "operator-health", Name: "healthy task",
			Status: StatusEnabled, ScopeMode: ScopeAllSavedBindings, Interval: 30 * time.Minute,
			NextRunAt: now.Add(time.Hour), LastRunAt: now.Add(-time.Hour),
			LastStatus: RunStatusSuccess, LastSuccessAt: now.Add(-time.Hour),
			CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-time.Hour),
		},
		{
			ID: "task-abandoned", HubUserID: "operator-health", Name: "stalled task",
			Status: StatusEnabled, ScopeMode: ScopeAllSavedBindings, Interval: 30 * time.Minute,
			NextRunAt: now.Add(time.Hour), LastRunAt: now.Add(-time.Minute),
			LastFailureAt: now.Add(-time.Minute), LastStatus: RunStatusAbandoned,
			LastError: "monitoring task lease expired",
			CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-time.Minute),
		},
		{
			ID: "task-paused", HubUserID: "operator-health", Name: "paused failure",
			Status: StatusPaused, ScopeMode: ScopeAllSavedBindings, Interval: 30 * time.Minute,
			NextRunAt: now, LastRunAt: now, LastFailureAt: now,
			LastStatus: RunStatusFailed, LastError: "paused task should not block launch",
			CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now,
		},
	} {
		if err := repo.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}
	service, err := NewService(repo, ServiceConfig{DefaultHubUserID: "operator-health"})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	health, err := service.GetAutomationHealth(context.Background(), "")
	if err != nil {
		t.Fatalf("get automation health: %v", err)
	}
	if !health.Checked || health.EnabledTaskCount != 2 || health.UnhealthyTaskCount != 1 {
		t.Fatalf("unexpected monitoring health counts: %#v", health)
	}
	if health.TaskID != "task-abandoned" || health.LastStatus != RunStatusAbandoned ||
		health.LastError != "monitoring task lease expired" {
		t.Fatalf("expected bounded unhealthy task evidence, got %#v", health)
	}
}
