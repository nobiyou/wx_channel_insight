package monitoring

import (
	"context"
	"time"
)

type Repository interface {
	ListTasks(ctx context.Context, hubUserID string, includeArchived bool) ([]Task, error)
	GetTask(ctx context.Context, hubUserID, taskID string) (Task, error)
	CreateTask(ctx context.Context, task Task) error
	UpdateTask(ctx context.Context, task Task) error
	ArchiveTask(ctx context.Context, hubUserID, taskID string, archivedAt time.Time) error
	ReplaceTaskTargets(ctx context.Context, hubUserID, taskID string, targets []TaskTarget) error

	EnsureDefaultTask(ctx context.Context, task Task) (Task, bool, error)
	RecoverStaleRuns(ctx context.Context, hubUserID string, now time.Time) error
	ClaimDueTask(ctx context.Context, hubUserID, runID string, now, leaseUntil time.Time) (TaskClaim, error)
	StartManualRun(ctx context.Context, hubUserID, taskID, runID, trigger string, now, leaseUntil time.Time) (TaskClaim, error)
	RenewTaskRunLease(ctx context.Context, hubUserID, runID string, leaseUntil time.Time) error
	CompleteTaskRun(ctx context.Context, hubUserID, runID string, update TaskRunUpdate) error
	ListTaskRuns(ctx context.Context, hubUserID, taskID string, limit int) ([]TaskRun, error)
}
