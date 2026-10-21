package monitoringpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type Repository struct {
	client *postgres.Client
	db     *sql.DB
}

var _ monitoring.Repository = (*Repository)(nil)

func NewRepository(client *postgres.Client) (*Repository, error) {
	if client == nil {
		return nil, errors.New("postgres client is required")
	}
	db := client.DB()
	if err := postgres.EnsureMonitoringSchema(db); err != nil {
		return nil, err
	}
	return &Repository{client: client, db: db}, nil
}

func (r *Repository) ListTasks(ctx context.Context, hubUserID string, includeArchived bool) ([]monitoring.Task, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := taskSelectSQL + `
WHERE t.hub_user_id = $1`
	args := []any{hubUserID}
	if !includeArchived {
		query += ` AND t.status <> 'archived'`
	}
	query += ` ORDER BY t.next_run_at ASC, t.created_at ASC, t.task_id ASC`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []monitoring.Task
	for rows.Next() {
		task, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		task.Targets, err = r.listTargets(ctx, r.db, task.HubUserID, task.ID)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *Repository) GetAutomationHealth(ctx context.Context, hubUserID string) (monitoring.AutomationHealth, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return monitoring.AutomationHealth{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `
WITH enabled AS (
  SELECT task_id, name, last_status, last_error, last_run_at, last_failure_at, updated_at,
         last_status IN ('failed', 'partial', 'abandoned') AS unhealthy
  FROM insight_monitoring_tasks
  WHERE hub_user_id = $1 AND status = 'enabled'
), counts AS (
  SELECT COUNT(*) AS enabled_count,
         COUNT(*) FILTER (WHERE unhealthy) AS unhealthy_count
  FROM enabled
), candidate AS (
  SELECT task_id, name, last_status, last_error, last_run_at, last_failure_at
  FROM enabled
  ORDER BY unhealthy DESC,
           CASE WHEN unhealthy
             THEN COALESCE(last_failure_at, last_run_at, updated_at)
             ELSE COALESCE(last_run_at, updated_at)
           END DESC,
           task_id ASC
  LIMIT 1
)
SELECT counts.enabled_count, counts.unhealthy_count,
       candidate.task_id, candidate.name, candidate.last_status, candidate.last_error,
       candidate.last_run_at, candidate.last_failure_at
FROM counts
LEFT JOIN candidate ON TRUE
`, hubUserID)
	var health monitoring.AutomationHealth
	var taskID, taskName, lastStatus, lastError sql.NullString
	var lastRunAt, lastFailureAt sql.NullTime
	if err := row.Scan(
		&health.EnabledTaskCount, &health.UnhealthyTaskCount,
		&taskID, &taskName, &lastStatus, &lastError, &lastRunAt, &lastFailureAt,
	); err != nil {
		return monitoring.AutomationHealth{}, err
	}
	health.Checked = true
	health.TaskID = taskID.String
	health.TaskName = taskName.String
	health.LastStatus = lastStatus.String
	health.LastError = lastError.String
	if lastRunAt.Valid {
		health.LastRunAt = lastRunAt.Time
	}
	if lastFailureAt.Valid {
		health.LastFailureAt = lastFailureAt.Time
	}
	return health, nil
}

func (r *Repository) GetTask(ctx context.Context, hubUserID, taskID string) (monitoring.Task, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return monitoring.Task{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	task, err := loadTask(ctx, r.db, hubUserID, taskID, false)
	if err != nil {
		return monitoring.Task{}, err
	}
	task.Targets, err = r.listTargets(ctx, r.db, hubUserID, taskID)
	if err != nil {
		return monitoring.Task{}, err
	}
	return task, nil
}

func (r *Repository) CreateTask(ctx context.Context, task monitoring.Task) error {
	task, err := normalizeTask(task)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if task.System {
		var exists bool
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM insight_monitoring_tasks
  WHERE hub_user_id = $1 AND system_task = TRUE
)`, task.HubUserID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return monitoring.ErrTaskAlreadyExists
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO insight_monitoring_tasks (
  hub_user_id, task_id, name, task_type, status, scope_mode, interval_seconds,
  next_run_at, last_run_at, last_success_at, last_failure_at, last_status,
  last_error, system_task, created_at, updated_at, archived_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
`, task.HubUserID, task.ID, task.Name, task.TaskType, task.Status, task.ScopeMode,
		int64(task.Interval/time.Second), task.NextRunAt, nullableTime(task.LastRunAt),
		nullableTime(task.LastSuccessAt), nullableTime(task.LastFailureAt), task.LastStatus,
		task.LastError, task.System, task.CreatedAt, task.UpdatedAt, nullableTime(task.ArchivedAt)); err != nil {
		return err
	}
	if err := insertTargets(ctx, tx, task); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) UpdateTask(ctx context.Context, task monitoring.Task) error {
	task, err := normalizeTask(task)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	existing, err := loadTask(ctx, tx, task.HubUserID, task.ID, true)
	if err != nil {
		return err
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = existing.CreatedAt
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = time.Now().UTC()
	}
	if task.Status == monitoring.StatusArchived && task.ArchivedAt.IsZero() {
		task.ArchivedAt = task.UpdatedAt
	}
	if task.System && !existing.System {
		var exists bool
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM insight_monitoring_tasks
  WHERE hub_user_id = $1 AND system_task = TRUE AND task_id <> $2
)`, task.HubUserID, task.ID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return monitoring.ErrTaskAlreadyExists
		}
	}
	result, err := tx.ExecContext(ctx, `
UPDATE insight_monitoring_tasks
SET name = $3, task_type = $4, status = $5, scope_mode = $6, interval_seconds = $7,
    next_run_at = $8, last_run_at = $9, last_success_at = $10,
    last_failure_at = $11, last_status = $12, last_error = $13,
    system_task = $14, created_at = $15, updated_at = $16, archived_at = $17
WHERE hub_user_id = $1 AND task_id = $2
`, task.HubUserID, task.ID, task.Name, task.TaskType, task.Status, task.ScopeMode,
		int64(task.Interval/time.Second), task.NextRunAt, nullableTime(task.LastRunAt),
		nullableTime(task.LastSuccessAt), nullableTime(task.LastFailureAt), task.LastStatus,
		task.LastError, task.System, task.CreatedAt, task.UpdatedAt, nullableTime(task.ArchivedAt))
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return monitoring.ErrTaskNotFound
	}
	if err := replaceTargets(ctx, tx, task); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) ArchiveTask(ctx context.Context, hubUserID, taskID string, archivedAt time.Time) error {
	if err := requireHubUserID(hubUserID); err != nil {
		return err
	}
	if strings.TrimSpace(taskID) == "" {
		return monitoring.ErrTaskNotFound
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if archivedAt.IsZero() {
		archivedAt = time.Now().UTC()
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE insight_monitoring_tasks
SET status = 'archived', archived_at = $3, updated_at = $3
WHERE hub_user_id = $1 AND task_id = $2
`, hubUserID, strings.TrimSpace(taskID), archivedAt)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return monitoring.ErrTaskNotFound
	}
	return nil
}

func (r *Repository) ReplaceTaskTargets(ctx context.Context, hubUserID, taskID string, targets []monitoring.TaskTarget) error {
	if err := requireHubUserID(hubUserID); err != nil {
		return err
	}
	if err := monitoring.ValidateTargets(targets); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	task, err := loadTask(ctx, tx, hubUserID, taskID, true)
	if err != nil {
		return err
	}
	if task.ScopeMode != monitoring.ScopeSelected {
		return errors.New("all-saved-bindings task cannot replace selected targets")
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM insight_monitoring_task_targets
WHERE hub_user_id = $1 AND task_id = $2
`, hubUserID, taskID); err != nil {
		return err
	}
	for _, target := range targets {
		normalized, err := monitoring.NormalizeTaskTarget(target)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
		INSERT INTO insight_monitoring_task_targets (hub_user_id, task_id, source, target_key, target_kind, account_id, comment_mode, label, display_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, hubUserID, taskID, normalized.Source, normalized.TargetKey, normalized.TargetKind, normalized.AccountID, normalized.CommentMode, normalized.Label, normalized.DisplayName); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE insight_monitoring_tasks SET updated_at = $3 WHERE hub_user_id = $1 AND task_id = $2`, hubUserID, taskID, time.Now().UTC())
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) EnsureDefaultTask(ctx context.Context, task monitoring.Task) (monitoring.Task, bool, error) {
	task.System = true
	task, err := normalizeTask(task)
	if err != nil {
		return monitoring.Task{}, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return monitoring.Task{}, false, err
	}
	defer tx.Rollback()
	existing, err := loadSystemTask(ctx, tx, task.HubUserID)
	if err == nil {
		existing.Targets, err = listTargets(ctx, tx, existing.HubUserID, existing.ID)
		if err != nil {
			return monitoring.Task{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return monitoring.Task{}, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return monitoring.Task{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO insight_monitoring_tasks (
  hub_user_id, task_id, name, task_type, status, scope_mode, interval_seconds,
  next_run_at, last_run_at, last_success_at, last_failure_at, last_status,
  last_error, system_task, created_at, updated_at, archived_at
 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, NULL, NULL, $9, $10, TRUE, $11, $12, $13)
ON CONFLICT DO NOTHING
`, task.HubUserID, task.ID, task.Name, task.TaskType, task.Status, task.ScopeMode,
		int64(task.Interval/time.Second), task.NextRunAt, task.LastStatus, task.LastError,
		task.CreatedAt, task.UpdatedAt, nullableTime(task.ArchivedAt)); err != nil {
		return monitoring.Task{}, false, err
	}
	created, err := loadSystemTask(ctx, tx, task.HubUserID)
	if err != nil {
		return monitoring.Task{}, false, err
	}
	created.Targets, err = listTargets(ctx, tx, created.HubUserID, created.ID)
	if err != nil {
		return monitoring.Task{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return monitoring.Task{}, false, err
	}
	return created, created.ID == task.ID, nil
}

func (r *Repository) RecoverStaleRuns(ctx context.Context, hubUserID string, now time.Time) error {
	if err := requireHubUserID(hubUserID); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
UPDATE insight_monitoring_task_runs
SET status = 'abandoned', finished_at = $2, lease_until = NULL,
    message = CASE WHEN message = '' THEN 'monitoring task lease expired' ELSE message END
WHERE hub_user_id = $1 AND status = 'running'
  AND lease_until IS NOT NULL AND lease_until <= $2
RETURNING run_id, task_id, attempt
`, hubUserID, now)
	if err != nil {
		return err
	}
	type staleRun struct {
		id, taskID string
		attempt    int
	}
	var stale []staleRun
	for rows.Next() {
		var item staleRun
		if err := rows.Scan(&item.id, &item.taskID, &item.attempt); err != nil {
			rows.Close()
			return err
		}
		stale = append(stale, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range stale {
		if _, err := tx.ExecContext(ctx, `
UPDATE insight_monitoring_tasks
SET last_status = 'abandoned', last_error = 'monitoring task lease expired',
    last_failure_at = $3, updated_at = $3
WHERE hub_user_id = $1 AND task_id = $2
  AND NOT EXISTS (
    SELECT 1 FROM insight_monitoring_task_runs newer
    WHERE newer.hub_user_id = $1 AND newer.task_id = $2
      AND newer.attempt > $4
  )
`, hubUserID, item.taskID, now, item.attempt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ClaimDueTask(ctx context.Context, hubUserID, runID string, now, leaseUntil time.Time) (monitoring.TaskClaim, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return monitoring.TaskClaim{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return monitoring.TaskClaim{}, errors.New("monitoring task run id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if leaseUntil.IsZero() {
		leaseUntil = now.Add(15 * time.Minute)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	defer tx.Rollback()
	task, err := loadDueTask(ctx, tx, hubUserID, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return monitoring.TaskClaim{}, monitoring.ErrNoDueTask
		}
		return monitoring.TaskClaim{}, err
	}
	task.Targets, err = listTargets(ctx, tx, task.HubUserID, task.ID)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	attempt, err := nextAttempt(ctx, tx, task.HubUserID, task.ID)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	if _, err := insertRun(ctx, tx, monitoring.TaskRun{
		ID: runID, TaskID: task.ID, HubUserID: task.HubUserID, Trigger: monitoring.TriggerScheduled,
		Status: monitoring.RunStatusRunning, Attempt: attempt, StartedAt: now, LeaseUntil: leaseUntil,
	}); err != nil {
		return monitoring.TaskClaim{}, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE insight_monitoring_tasks
SET last_run_at = $3, last_status = 'running', last_error = '', updated_at = $3
WHERE hub_user_id = $1 AND task_id = $2
`, task.HubUserID, task.ID, now); err != nil {
		return monitoring.TaskClaim{}, err
	}
	task, err = loadTask(ctx, tx, task.HubUserID, task.ID, false)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	task.Targets, err = listTargets(ctx, tx, task.HubUserID, task.ID)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	if err := tx.Commit(); err != nil {
		return monitoring.TaskClaim{}, err
	}
	return monitoring.TaskClaim{Task: task, Run: monitoring.TaskRun{
		ID: runID, TaskID: task.ID, HubUserID: task.HubUserID, Trigger: monitoring.TriggerScheduled,
		Status: monitoring.RunStatusRunning, Attempt: attempt, StartedAt: now, LeaseUntil: leaseUntil,
	}}, nil
}

func (r *Repository) StartManualRun(ctx context.Context, hubUserID, taskID, runID, trigger string, now, leaseUntil time.Time) (monitoring.TaskClaim, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return monitoring.TaskClaim{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return monitoring.TaskClaim{}, errors.New("monitoring task run id is required")
	}
	if trigger == "" {
		trigger = monitoring.TriggerManual
	}
	if trigger != monitoring.TriggerManual && trigger != monitoring.TriggerRetry {
		return monitoring.TaskClaim{}, errors.New("unsupported monitoring task trigger")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if leaseUntil.IsZero() {
		leaseUntil = now.Add(15 * time.Minute)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	defer tx.Rollback()
	task, err := loadTask(ctx, tx, hubUserID, taskID, true)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	if task.Status == monitoring.StatusArchived {
		return monitoring.TaskClaim{}, monitoring.ErrTaskArchived
	}
	var active bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM insight_monitoring_task_runs
  WHERE hub_user_id = $1 AND task_id = $2 AND status = 'running'
    AND (lease_until IS NULL OR lease_until > $3)
)`, hubUserID, taskID, now).Scan(&active); err != nil {
		return monitoring.TaskClaim{}, err
	}
	if active {
		return monitoring.TaskClaim{}, monitoring.ErrTaskRunInProgress
	}
	attempt, err := nextAttempt(ctx, tx, hubUserID, taskID)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	run := monitoring.TaskRun{
		ID: runID, TaskID: task.ID, HubUserID: task.HubUserID, Trigger: trigger,
		Status: monitoring.RunStatusRunning, Attempt: attempt, StartedAt: now, LeaseUntil: leaseUntil,
	}
	if _, err := insertRun(ctx, tx, run); err != nil {
		return monitoring.TaskClaim{}, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE insight_monitoring_tasks
SET last_run_at = $3, last_status = 'running', last_error = '', updated_at = $3
WHERE hub_user_id = $1 AND task_id = $2
`, hubUserID, taskID, now); err != nil {
		return monitoring.TaskClaim{}, err
	}
	task, err = loadTask(ctx, tx, hubUserID, taskID, false)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	task.Targets, err = listTargets(ctx, tx, hubUserID, taskID)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	if err := tx.Commit(); err != nil {
		return monitoring.TaskClaim{}, err
	}
	run.TaskID = task.ID
	return monitoring.TaskClaim{Task: task, Run: run}, nil
}

func (r *Repository) RenewTaskRunLease(ctx context.Context, hubUserID, runID string, leaseUntil time.Time) error {
	if err := requireHubUserID(hubUserID); err != nil {
		return err
	}
	if strings.TrimSpace(runID) == "" {
		return monitoring.ErrTaskRunNotFound
	}
	if leaseUntil.IsZero() {
		return errors.New("monitoring task lease deadline is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE insight_monitoring_task_runs
SET lease_until = $3
WHERE hub_user_id = $1 AND run_id = $2 AND status = 'running'
  AND (lease_until IS NULL OR lease_until < $3)
`, hubUserID, runID, leaseUntil)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected > 0 {
		return nil
	}
	var status string
	if err := r.db.QueryRowContext(ctx, `
SELECT status FROM insight_monitoring_task_runs
WHERE hub_user_id = $1 AND run_id = $2
`, hubUserID, runID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return monitoring.ErrTaskRunNotFound
		}
		return err
	}
	if status != monitoring.RunStatusRunning {
		return monitoring.ErrTaskRunCompleted
	}
	return nil
}

func (r *Repository) CompleteTaskRun(ctx context.Context, hubUserID, runID string, update monitoring.TaskRunUpdate) error {
	if err := requireHubUserID(hubUserID); err != nil {
		return err
	}
	if strings.TrimSpace(runID) == "" {
		return monitoring.ErrTaskRunNotFound
	}
	if err := validateRunStatus(update.Status); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if update.FinishedAt.IsZero() {
		update.FinishedAt = time.Now().UTC()
	}
	errorsJSON, err := json.Marshal(update.Errors)
	if err != nil {
		return err
	}
	if len(errorsJSON) == 0 {
		errorsJSON = []byte("[]")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var taskID, runStatus string
	if err := tx.QueryRowContext(ctx, `
SELECT task_id, status
FROM insight_monitoring_task_runs
WHERE hub_user_id = $1 AND run_id = $2
FOR UPDATE
`, hubUserID, runID).Scan(&taskID, &runStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return monitoring.ErrTaskRunNotFound
		}
		return err
	}
	if runStatus != monitoring.RunStatusRunning {
		return monitoring.ErrTaskRunCompleted
	}
	task, err := loadTask(ctx, tx, hubUserID, taskID, true)
	if err != nil {
		return err
	}
	if update.NextRunAt.IsZero() {
		update.NextRunAt = task.NextRunAt
	}
	result, err := tx.ExecContext(ctx, `
UPDATE insight_monitoring_task_runs
SET status = $3, finished_at = $4, lease_until = NULL, operator_run_id = $5,
    attempted_count = $6, succeeded_count = $7, failed_count = $8,
    skipped_count = $9, message = $10, errors_json = $11
WHERE hub_user_id = $1 AND run_id = $2 AND status = 'running'
`, hubUserID, runID, update.Status, update.FinishedAt,
		strings.TrimSpace(update.OperatorRunID), update.Counts.Attempted, update.Counts.Succeeded,
		update.Counts.Failed, update.Counts.Skipped, strings.TrimSpace(update.Message), errorsJSON)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return monitoring.ErrTaskRunCompleted
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE insight_monitoring_tasks
SET next_run_at = $3, last_status = $4, last_error = $5, updated_at = $6,
    last_success_at = CASE WHEN $4 IN ('success', 'partial') THEN $6 ELSE last_success_at END,
    last_failure_at = CASE WHEN $4 IN ('failed', 'abandoned') THEN $6 ELSE last_failure_at END
WHERE hub_user_id = $1 AND task_id = $2
`, hubUserID, taskID, update.NextRunAt, update.Status, strings.TrimSpace(update.Message), update.FinishedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) ListTaskRuns(ctx context.Context, hubUserID, taskID string, limit int) ([]monitoring.TaskRun, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskID) == "" {
		return nil, monitoring.ErrTaskNotFound
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if _, err := loadTask(ctx, r.db, hubUserID, taskID, false); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT run_id, task_id, hub_user_id, trigger, status, attempt, started_at,
       finished_at, lease_until, operator_run_id, attempted_count,
       succeeded_count, failed_count, skipped_count, message, errors_json
FROM insight_monitoring_task_runs
WHERE hub_user_id = $1 AND task_id = $2
ORDER BY started_at DESC, run_id DESC
LIMIT $3
`, hubUserID, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []monitoring.TaskRun
	for rows.Next() {
		run, err := scanTaskRunRow(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

const taskSelectSQL = `
SELECT t.hub_user_id, t.task_id, t.name, t.task_type, t.status, t.scope_mode,
       t.interval_seconds, t.next_run_at, t.last_run_at, t.last_success_at,
       t.last_failure_at, t.last_status, t.last_error, t.system_task,
       t.created_at, t.updated_at, t.archived_at
FROM insight_monitoring_tasks t`

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadTask(ctx context.Context, q queryer, hubUserID, taskID string, forUpdate bool) (monitoring.Task, error) {
	query := taskSelectSQL + ` WHERE t.hub_user_id = $1 AND t.task_id = $2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	task, err := scanTaskRow(q.QueryRowContext(ctx, query, hubUserID, strings.TrimSpace(taskID)))
	if errors.Is(err, sql.ErrNoRows) {
		return monitoring.Task{}, monitoring.ErrTaskNotFound
	}
	return task, err
}

func loadDueTask(ctx context.Context, q queryer, hubUserID string, now time.Time) (monitoring.Task, error) {
	return scanTaskRow(q.QueryRowContext(ctx, taskSelectSQL+`
WHERE t.hub_user_id = $1
  AND t.status = 'enabled'
  AND t.next_run_at <= $2
  AND NOT EXISTS (
    SELECT 1 FROM insight_monitoring_task_runs r
    WHERE r.hub_user_id = t.hub_user_id AND r.task_id = t.task_id
      AND r.status = 'running'
      AND (r.lease_until IS NULL OR r.lease_until > $2)
  )
ORDER BY t.next_run_at ASC, t.created_at ASC, t.task_id ASC
LIMIT 1
FOR UPDATE SKIP LOCKED`, hubUserID, now))
}

func loadSystemTask(ctx context.Context, q queryer, hubUserID string) (monitoring.Task, error) {
	return scanTaskRow(q.QueryRowContext(ctx, taskSelectSQL+`
WHERE t.hub_user_id = $1 AND t.system_task = TRUE
ORDER BY t.created_at ASC, t.task_id ASC
LIMIT 1`, hubUserID))
}

func (r *Repository) listTargets(ctx context.Context, q queryer, hubUserID, taskID string) ([]monitoring.TaskTarget, error) {
	return listTargets(ctx, q, hubUserID, taskID)
}

func listTargets(ctx context.Context, q queryer, hubUserID, taskID string) ([]monitoring.TaskTarget, error) {
	rows, err := q.QueryContext(ctx, `
SELECT source, target_key, target_kind, account_id, comment_mode, label, display_name
FROM insight_monitoring_task_targets
WHERE hub_user_id = $1 AND task_id = $2
ORDER BY target_kind ASC, source ASC, target_key ASC, account_id ASC
`, hubUserID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []monitoring.TaskTarget
	for rows.Next() {
		var target monitoring.TaskTarget
		if err := rows.Scan(&target.Source, &target.TargetKey, &target.TargetKind, &target.AccountID, &target.CommentMode, &target.Label, &target.DisplayName); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func insertTargets(ctx context.Context, tx *sql.Tx, task monitoring.Task) error {
	for _, target := range task.Targets {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO insight_monitoring_task_targets (hub_user_id, task_id, source, target_key, target_kind, account_id, comment_mode, label, display_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, task.HubUserID, task.ID, target.Source, target.TargetKey, target.TargetKind, target.AccountID, target.CommentMode, target.Label, target.DisplayName); err != nil {
			return err
		}
	}
	return nil
}

func replaceTargets(ctx context.Context, tx *sql.Tx, task monitoring.Task) error {
	if _, err := tx.ExecContext(ctx, `
DELETE FROM insight_monitoring_task_targets
WHERE hub_user_id = $1 AND task_id = $2
`, task.HubUserID, task.ID); err != nil {
		return err
	}
	return insertTargets(ctx, tx, task)
}

func insertRun(ctx context.Context, tx *sql.Tx, run monitoring.TaskRun) (sql.Result, error) {
	errorsJSON, err := json.Marshal(run.Errors)
	if err != nil {
		return nil, err
	}
	if len(errorsJSON) == 0 {
		errorsJSON = []byte("[]")
	}
	return tx.ExecContext(ctx, `
INSERT INTO insight_monitoring_task_runs (
  hub_user_id, run_id, task_id, trigger, status, attempt, started_at,
  finished_at, lease_until, operator_run_id, attempted_count,
  succeeded_count, failed_count, skipped_count, message, errors_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, NULL, $8, $9, $10, $11, $12, $13, $14, $15)
`, run.HubUserID, run.ID, run.TaskID, run.Trigger, run.Status, run.Attempt,
		run.StartedAt, nullableTime(run.LeaseUntil), run.OperatorRunID,
		run.Counts.Attempted, run.Counts.Succeeded, run.Counts.Failed, run.Counts.Skipped,
		run.Message, errorsJSON)
}

func nextAttempt(ctx context.Context, q queryer, hubUserID, taskID string) (int, error) {
	var attempt int
	if err := q.QueryRowContext(ctx, `
SELECT COALESCE(MAX(attempt), 0) + 1
FROM insight_monitoring_task_runs
WHERE hub_user_id = $1 AND task_id = $2
`, hubUserID, taskID).Scan(&attempt); err != nil {
		return 0, err
	}
	return attempt, nil
}

func scanTaskRow(scanner interface{ Scan(...any) error }) (monitoring.Task, error) {
	var task monitoring.Task
	var intervalSeconds int64
	var lastRunAt, lastSuccessAt, lastFailureAt, archivedAt sql.NullTime
	if err := scanner.Scan(
		&task.HubUserID, &task.ID, &task.Name, &task.TaskType, &task.Status, &task.ScopeMode,
		&intervalSeconds, &task.NextRunAt, &lastRunAt, &lastSuccessAt,
		&lastFailureAt, &task.LastStatus, &task.LastError, &task.System,
		&task.CreatedAt, &task.UpdatedAt, &archivedAt,
	); err != nil {
		return monitoring.Task{}, err
	}
	task.Interval = time.Duration(intervalSeconds) * time.Second
	task.LastRunAt = nullTime(lastRunAt)
	task.LastSuccessAt = nullTime(lastSuccessAt)
	task.LastFailureAt = nullTime(lastFailureAt)
	task.ArchivedAt = nullTime(archivedAt)
	return task, nil
}

func scanTaskRunRow(scanner interface{ Scan(...any) error }) (monitoring.TaskRun, error) {
	var run monitoring.TaskRun
	var finishedAt, leaseUntil sql.NullTime
	var errorsJSON []byte
	if err := scanner.Scan(
		&run.ID, &run.TaskID, &run.HubUserID, &run.Trigger, &run.Status,
		&run.Attempt, &run.StartedAt, &finishedAt, &leaseUntil, &run.OperatorRunID,
		&run.Counts.Attempted, &run.Counts.Succeeded, &run.Counts.Failed,
		&run.Counts.Skipped, &run.Message, &errorsJSON,
	); err != nil {
		return monitoring.TaskRun{}, err
	}
	run.FinishedAt = nullTime(finishedAt)
	run.LeaseUntil = nullTime(leaseUntil)
	if len(errorsJSON) > 0 {
		if err := json.Unmarshal(errorsJSON, &run.Errors); err != nil {
			return monitoring.TaskRun{}, err
		}
	}
	return run, nil
}

func normalizeTask(task monitoring.Task) (monitoring.Task, error) {
	normalized, err := monitoring.NormalizeTask(task)
	if err != nil {
		return monitoring.Task{}, err
	}
	if normalized.Interval%time.Second != 0 {
		return monitoring.Task{}, errors.New("monitoring task interval must use whole seconds")
	}
	now := time.Now().UTC()
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = now
	}
	if normalized.UpdatedAt.IsZero() {
		normalized.UpdatedAt = normalized.CreatedAt
	}
	if normalized.NextRunAt.IsZero() {
		normalized.NextRunAt = normalized.UpdatedAt
	}
	if normalized.Status == monitoring.StatusArchived && normalized.ArchivedAt.IsZero() {
		normalized.ArchivedAt = normalized.UpdatedAt
	}
	return normalized, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func requireHubUserID(hubUserID string) error {
	if strings.TrimSpace(hubUserID) == "" {
		return errors.New("monitoring task hub user id is required")
	}
	return nil
}

func validateRunStatus(status string) error {
	switch status {
	case monitoring.RunStatusSuccess, monitoring.RunStatusPartial, monitoring.RunStatusFailed,
		monitoring.RunStatusSkipped, monitoring.RunStatusAbandoned:
		return nil
	default:
		return errors.New("unsupported monitoring task run status")
	}
}
