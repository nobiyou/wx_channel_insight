package monitoringpg_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	"github.com/nobiyou/wx_channel_insight/internal/monitoringpg"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func TestRepositoryPersistsClaimsAndHistory(t *testing.T) {
	db, dsn := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, dsn)
	if err != nil {
		_ = db.Close()
		t.Fatalf("new postgres client: %v", err)
	}
	repo, err := monitoringpg.NewRepository(client)
	if err != nil {
		t.Fatalf("new monitoring repository: %v", err)
	}

	hubUserID := "test:monitoring:repository:" + time.Now().UTC().Format("20060102150405.000000000")
	taskID := "task-pg-1"
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM insight_monitoring_task_runs WHERE hub_user_id = $1`, hubUserID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM insight_monitoring_task_targets WHERE hub_user_id = $1`, hubUserID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM insight_monitoring_tasks WHERE hub_user_id = $1`, hubUserID)
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	task := monitoring.Task{
		ID:        taskID,
		HubUserID: hubUserID,
		Name:      "Postgres task",
		Status:    monitoring.StatusEnabled,
		ScopeMode: monitoring.ScopeSelected,
		Interval:  5 * time.Minute,
		NextRunAt: now.Add(-time.Minute),
		CreatedAt: now.Add(-2 * time.Minute),
		UpdatedAt: now.Add(-2 * time.Minute),
		Targets: []monitoring.TaskTarget{{BindingRef: monitoring.BindingRef{
			Source: "wx_video_search", TargetKey: "topic-a",
		}}},
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	claim, err := repo.ClaimDueTask(context.Background(), hubUserID, "run-pg-1", now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim due task: %v", err)
	}
	if claim.Task.ID != taskID || claim.Run.Status != monitoring.RunStatusRunning {
		t.Fatalf("unexpected claim: %#v", claim)
	}
	if _, err := repo.ClaimDueTask(context.Background(), hubUserID, "run-pg-2", now, now.Add(time.Minute)); err != monitoring.ErrNoDueTask {
		t.Fatalf("expected due task contention, got %v", err)
	}
	renewedUntil := now.Add(2 * time.Minute)
	if err := repo.RenewTaskRunLease(context.Background(), hubUserID, claim.Run.ID, renewedUntil); err != nil {
		t.Fatalf("renew task lease: %v", err)
	}
	if err := repo.RecoverStaleRuns(context.Background(), hubUserID, now.Add(90*time.Second)); err != nil {
		t.Fatalf("recover before renewed lease: %v", err)
	}
	renewedRuns, err := repo.ListTaskRuns(context.Background(), hubUserID, taskID, 10)
	if err != nil {
		t.Fatalf("list renewed task runs: %v", err)
	}
	if len(renewedRuns) != 1 || renewedRuns[0].Status != monitoring.RunStatusRunning || !renewedRuns[0].LeaseUntil.Equal(renewedUntil) {
		t.Fatalf("renewed task run was recovered too early: %#v", renewedRuns)
	}
	if err := repo.CompleteTaskRun(context.Background(), hubUserID, claim.Run.ID, monitoring.TaskRunUpdate{
		Status:        monitoring.RunStatusSuccess,
		FinishedAt:    now.Add(10 * time.Second),
		OperatorRunID: "operator-run-pg-1",
		Counts:        monitoring.TaskRunCounts{Attempted: 1, Succeeded: 1},
		NextRunAt:     now.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("complete task run: %v", err)
	}
	loaded, err := repo.GetTask(context.Background(), hubUserID, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if loaded.LastStatus != monitoring.RunStatusSuccess || len(loaded.Targets) != 1 || loaded.Targets[0].TargetKey != "topic-a" {
		t.Fatalf("unexpected persisted task: %#v", loaded)
	}
	runs, err := repo.ListTaskRuns(context.Background(), hubUserID, taskID, 10)
	if err != nil {
		t.Fatalf("list task runs: %v", err)
	}
	if len(runs) != 1 || runs[0].OperatorRunID != "operator-run-pg-1" || runs[0].Counts.Succeeded != 1 {
		t.Fatalf("unexpected persisted run: %#v", runs)
	}
}

func TestRepositoryRecoveryAndCompletionChooseOneTerminalState(t *testing.T) {
	db, dsn := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, dsn)
	if err != nil {
		_ = db.Close()
		t.Fatalf("new postgres client: %v", err)
	}
	repo, err := monitoringpg.NewRepository(client)
	if err != nil {
		t.Fatalf("new monitoring repository: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	hubUserID := "test:monitoring:terminal-race:" + now.Format("20060102150405.000000000")
	taskID := "task-terminal-race"
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM insight_monitoring_task_runs WHERE hub_user_id = $1`, hubUserID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM insight_monitoring_task_targets WHERE hub_user_id = $1`, hubUserID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM insight_monitoring_tasks WHERE hub_user_id = $1`, hubUserID)
	})
	if err := repo.CreateTask(context.Background(), monitoring.Task{
		ID: taskID, HubUserID: hubUserID, Name: "terminal race",
		Status: monitoring.StatusEnabled, ScopeMode: monitoring.ScopeAllSavedBindings,
		Interval:  30 * time.Minute,
		NextRunAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	claim, err := repo.ClaimDueTask(context.Background(), hubUserID, "run-terminal-race", now, now.Add(-time.Second))
	if err != nil {
		t.Fatalf("claim expired task: %v", err)
	}

	start := make(chan struct{})
	recoverResult := make(chan error, 1)
	completeResult := make(chan error, 1)
	go func() {
		<-start
		recoverResult <- repo.RecoverStaleRuns(context.Background(), hubUserID, now)
	}()
	go func() {
		<-start
		completeResult <- repo.CompleteTaskRun(context.Background(), hubUserID, claim.Run.ID, monitoring.TaskRunUpdate{
			Status: monitoring.RunStatusSuccess, FinishedAt: now,
			OperatorRunID: "operator-terminal-race", NextRunAt: now.Add(5 * time.Minute),
		})
	}()
	close(start)
	if err := <-recoverResult; err != nil {
		t.Fatalf("recover stale run: %v", err)
	}
	completionErr := <-completeResult
	if completionErr != nil && !errors.Is(completionErr, monitoring.ErrTaskRunCompleted) {
		t.Fatalf("complete task run: %v", completionErr)
	}

	runs, err := repo.ListTaskRuns(context.Background(), hubUserID, taskID, 10)
	if err != nil {
		t.Fatalf("list task runs: %v", err)
	}
	task, err := repo.GetTask(context.Background(), hubUserID, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if len(runs) != 1 || (runs[0].Status != monitoring.RunStatusSuccess && runs[0].Status != monitoring.RunStatusAbandoned) {
		t.Fatalf("expected one terminal run, got %#v", runs)
	}
	if task.LastStatus != runs[0].Status {
		t.Fatalf("task and run terminal states disagree: task=%#v run=%#v", task, runs[0])
	}
	if runs[0].Status == monitoring.RunStatusSuccess && runs[0].OperatorRunID != "operator-terminal-race" {
		t.Fatalf("successful terminal state lost operator identity: %#v", runs[0])
	}
}

func TestRepositoryRecoversRunAtExactLeaseDeadlineBeforeReclaim(t *testing.T) {
	repo, _, hubUserID := newPostgresTestRepository(t, "exact-lease")
	now := time.Now().UTC().Truncate(time.Microsecond)
	taskID := "task-exact-lease"
	if err := repo.CreateTask(context.Background(), monitoring.Task{
		ID: taskID, HubUserID: hubUserID, Name: "exact lease",
		Status: monitoring.StatusEnabled, ScopeMode: monitoring.ScopeAllSavedBindings,
		Interval:  30 * time.Minute,
		NextRunAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.ClaimDueTask(context.Background(), hubUserID, "run-exact-a", now, now); err != nil {
		t.Fatalf("claim exact-deadline run: %v", err)
	}
	if err := repo.RecoverStaleRuns(context.Background(), hubUserID, now); err != nil {
		t.Fatalf("recover exact-deadline run: %v", err)
	}
	runs, err := repo.ListTaskRuns(context.Background(), hubUserID, taskID, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != monitoring.RunStatusAbandoned {
		t.Fatalf("exact-deadline run was not recovered: %#v", runs)
	}
	if _, err := repo.ClaimDueTask(context.Background(), hubUserID, "run-exact-b", now, now.Add(time.Minute)); err != nil {
		t.Fatalf("claim after exact-deadline recovery: %v", err)
	}
}

func TestRepositoryRecoveryDoesNotOverwriteNewerRunSummary(t *testing.T) {
	repo, db, hubUserID := newPostgresTestRepository(t, "newer-summary")
	now := time.Now().UTC().Truncate(time.Microsecond)
	taskID := "task-newer-summary"
	if err := repo.CreateTask(context.Background(), monitoring.Task{
		ID: taskID, HubUserID: hubUserID, Name: "newer summary",
		Status: monitoring.StatusEnabled, ScopeMode: monitoring.ScopeAllSavedBindings,
		Interval:  30 * time.Minute,
		NextRunAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.ClaimDueTask(context.Background(), hubUserID, "run-summary-a", now, now.Add(-time.Second)); err != nil {
		t.Fatalf("claim old run: %v", err)
	}
	if err := repo.RecoverStaleRuns(context.Background(), hubUserID, now); err != nil {
		t.Fatalf("recover old run: %v", err)
	}
	newerAt := now.Add(time.Second)
	if _, err := repo.ClaimDueTask(context.Background(), hubUserID, "run-summary-b", newerAt, newerAt.Add(time.Minute)); err != nil {
		t.Fatalf("claim newer run: %v", err)
	}
	if err := repo.CompleteTaskRun(context.Background(), hubUserID, "run-summary-b", monitoring.TaskRunUpdate{
		Status: monitoring.RunStatusSuccess, FinishedAt: newerAt.Add(time.Second),
		OperatorRunID: "operator-summary-b", NextRunAt: newerAt.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("complete newer run: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
UPDATE insight_monitoring_task_runs
SET status = 'running', finished_at = NULL, lease_until = $3
WHERE hub_user_id = $1 AND run_id = $2
`, hubUserID, "run-summary-a", newerAt); err != nil {
		t.Fatalf("stage stale recovery race: %v", err)
	}
	if err := repo.RecoverStaleRuns(context.Background(), hubUserID, newerAt); err != nil {
		t.Fatalf("recover old run after newer completion: %v", err)
	}
	task, err := repo.GetTask(context.Background(), hubUserID, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.LastStatus != monitoring.RunStatusSuccess || task.LastError != "" {
		t.Fatalf("old recovery overwrote newer run summary: %#v", task)
	}
	health, err := repo.GetAutomationHealth(context.Background(), hubUserID)
	if err != nil {
		t.Fatalf("get automation health: %v", err)
	}
	if !health.Checked || health.UnhealthyTaskCount != 0 || health.LastStatus != monitoring.RunStatusSuccess {
		t.Fatalf("bounded automation health disagrees with newer run: %#v", health)
	}
}

func newPostgresTestRepository(t *testing.T, prefix string) (*monitoringpg.Repository, *sql.DB, string) {
	t.Helper()
	db, dsn := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, dsn)
	if err != nil {
		_ = db.Close()
		t.Fatalf("new postgres client: %v", err)
	}
	repo, err := monitoringpg.NewRepository(client)
	if err != nil {
		_ = client.Close()
		t.Fatalf("new monitoring repository: %v", err)
	}
	hubUserID := "test:monitoring:" + prefix + ":" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM insight_monitoring_task_runs WHERE hub_user_id = $1`, hubUserID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM insight_monitoring_task_targets WHERE hub_user_id = $1`, hubUserID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM insight_monitoring_tasks WHERE hub_user_id = $1`, hubUserID)
		_ = client.Close()
	})
	return repo, db, hubUserID
}
