package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryRepositoryClaimsCompletesAndListsHistory(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	task := validTask()
	task.NextRunAt = now.Add(-time.Minute)
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	claim, err := repo.ClaimDueTask(context.Background(), task.HubUserID, "run-1", now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim task: %v", err)
	}
	if claim.Run.Status != RunStatusRunning || claim.Run.Attempt != 1 {
		t.Fatalf("unexpected claim: %#v", claim.Run)
	}
	if _, err := repo.ClaimDueTask(context.Background(), task.HubUserID, "run-2", now, now.Add(time.Minute)); !errors.Is(err, ErrNoDueTask) {
		t.Fatalf("expected running task to be unavailable, got %v", err)
	}

	if err := repo.CompleteTaskRun(context.Background(), task.HubUserID, claim.Run.ID, TaskRunUpdate{
		Status:        RunStatusSuccess,
		FinishedAt:    now.Add(10 * time.Second),
		OperatorRunID: "operator-1",
		Counts:        TaskRunCounts{Attempted: 1, Succeeded: 1},
		NextRunAt:     now.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("complete task run: %v", err)
	}
	taskAfter, err := repo.GetTask(context.Background(), task.HubUserID, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if taskAfter.LastStatus != RunStatusSuccess || taskAfter.LastSuccessAt.IsZero() || taskAfter.NextRunAt != now.Add(5*time.Minute) {
		t.Fatalf("unexpected completed task: %#v", taskAfter)
	}
	runs, err := repo.ListTaskRuns(context.Background(), task.HubUserID, task.ID, 10)
	if err != nil {
		t.Fatalf("list task runs: %v", err)
	}
	if len(runs) != 1 || runs[0].OperatorRunID != "operator-1" || runs[0].Counts.Succeeded != 1 {
		t.Fatalf("unexpected task history: %#v", runs)
	}
}

func TestMemoryRepositoryRecoversExpiredLease(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 8, 9, 11, 0, 0, 0, time.UTC)
	task := validTask()
	task.NextRunAt = now.Add(-3 * time.Minute)
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.ClaimDueTask(context.Background(), task.HubUserID, "run-expired", now.Add(-2*time.Minute), now.Add(-time.Minute)); err != nil {
		t.Fatalf("claim expired task: %v", err)
	}
	if err := repo.RecoverStaleRuns(context.Background(), task.HubUserID, now); err != nil {
		t.Fatalf("recover stale run: %v", err)
	}
	runs, err := repo.ListTaskRuns(context.Background(), task.HubUserID, task.ID, 10)
	if err != nil {
		t.Fatalf("list task runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != RunStatusAbandoned || runs[0].FinishedAt != now {
		t.Fatalf("unexpected abandoned run: %#v", runs)
	}
	if _, err := repo.ClaimDueTask(context.Background(), task.HubUserID, "run-retry", now, now.Add(time.Minute)); err != nil {
		t.Fatalf("claim after lease recovery: %v", err)
	}
}

func TestMemoryRepositoryRenewedLeasePreventsStaleRecovery(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	task := Task{
		ID: "task-renewed", HubUserID: "operator-renewed", Name: "renewed task",
		Status: StatusEnabled, ScopeMode: ScopeAllSavedBindings,
		Interval: 30 * time.Minute, NextRunAt: now,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	claim, err := repo.ClaimDueTask(context.Background(), task.HubUserID, "run-renewed", now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim task: %v", err)
	}
	renewedUntil := now.Add(3 * time.Minute)
	if err := repo.RenewTaskRunLease(context.Background(), task.HubUserID, claim.Run.ID, renewedUntil); err != nil {
		t.Fatalf("renew task lease: %v", err)
	}
	if err := repo.RecoverStaleRuns(context.Background(), task.HubUserID, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("recover before renewed lease: %v", err)
	}
	runs, err := repo.ListTaskRuns(context.Background(), task.HubUserID, task.ID, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != RunStatusRunning || !runs[0].LeaseUntil.Equal(renewedUntil) {
		t.Fatalf("renewed run was recovered too early: %#v", runs)
	}
	if err := repo.RecoverStaleRuns(context.Background(), task.HubUserID, now.Add(4*time.Minute)); err != nil {
		t.Fatalf("recover after renewed lease: %v", err)
	}
	runs, err = repo.ListTaskRuns(context.Background(), task.HubUserID, task.ID, 10)
	if err != nil {
		t.Fatalf("list recovered runs: %v", err)
	}
	if runs[0].Status != RunStatusAbandoned {
		t.Fatalf("expired renewed run was not abandoned: %#v", runs[0])
	}
}

func TestMemoryRepositoryDefaultTaskIsIdempotentAcrossArchive(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	task := Task{
		ID:        "default-1",
		HubUserID: "operator-1",
		Name:      DefaultTaskName,
		Status:    StatusEnabled,
		ScopeMode: ScopeAllSavedBindings,
		Interval:  DefaultTaskInterval,
		NextRunAt: now,
		System:    true,
	}
	created, wasCreated, err := repo.EnsureDefaultTask(context.Background(), task)
	if err != nil || !wasCreated {
		t.Fatalf("create default task: task=%#v created=%v err=%v", created, wasCreated, err)
	}
	if err := repo.ArchiveTask(context.Background(), task.HubUserID, task.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("archive default task: %v", err)
	}
	again, wasCreated, err := repo.EnsureDefaultTask(context.Background(), Task{
		ID:        "default-2",
		HubUserID: task.HubUserID,
		Name:      DefaultTaskName,
		Status:    StatusEnabled,
		ScopeMode: ScopeAllSavedBindings,
		Interval:  DefaultTaskInterval,
		NextRunAt: now,
	})
	if err != nil || wasCreated || again.ID != task.ID || again.Status != StatusArchived {
		t.Fatalf("default task was not idempotent: task=%#v created=%v err=%v", again, wasCreated, err)
	}
}
