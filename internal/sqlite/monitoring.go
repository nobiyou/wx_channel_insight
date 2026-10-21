package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

type MonitoringRepository struct{ c *Client }

func NewMonitoringRepository(c *Client) (*MonitoringRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &MonitoringRepository{c}, nil
}

// Reuse the domain's lease/state machine inside one IMMEDIATE transaction. The
// database lock covers load, claim and save across independent API/jobs handles.
func withMonitoring[T any](ctx context.Context, c *Client, owner string, write bool, fn func(*monitoring.MemoryRepository) (T, error)) (value T, err error) {
	run := func(q queryer) error {
		state, _, err := get[monitoring.MemoryState](ctx, q, "monitoring", owner, "state")
		if err != nil {
			return err
		}
		memory := monitoring.NewMemoryRepository()
		memory.Restore(state)
		value, err = fn(memory)
		if err != nil {
			return err
		}
		if write {
			return put(ctx, q, "monitoring", owner, "state", memory.Snapshot())
		}
		return nil
	}
	if write {
		err = c.update(ctx, func(tx *sql.Tx) error { return run(tx) })
	} else {
		err = run(c.db)
	}
	return
}
func (r *MonitoringRepository) mutate(ctx context.Context, owner string, fn func(*monitoring.MemoryRepository) error) error {
	_, err := withMonitoring(ctx, r.c, owner, true, func(memory *monitoring.MemoryRepository) (struct{}, error) { return struct{}{}, fn(memory) })
	return err
}
func (r *MonitoringRepository) ListTasks(ctx context.Context, owner string, archived bool) ([]monitoring.Task, error) {
	return withMonitoring(ctx, r.c, owner, false, func(m *monitoring.MemoryRepository) ([]monitoring.Task, error) {
		return m.ListTasks(ctx, owner, archived)
	})
}
func (r *MonitoringRepository) GetTask(ctx context.Context, owner, id string) (monitoring.Task, error) {
	return withMonitoring(ctx, r.c, owner, false, func(m *monitoring.MemoryRepository) (monitoring.Task, error) { return m.GetTask(ctx, owner, id) })
}
func (r *MonitoringRepository) CreateTask(ctx context.Context, task monitoring.Task) error {
	return r.mutate(ctx, task.HubUserID, func(m *monitoring.MemoryRepository) error { return m.CreateTask(ctx, task) })
}
func (r *MonitoringRepository) UpdateTask(ctx context.Context, task monitoring.Task) error {
	return r.mutate(ctx, task.HubUserID, func(m *monitoring.MemoryRepository) error { return m.UpdateTask(ctx, task) })
}
func (r *MonitoringRepository) ArchiveTask(ctx context.Context, owner, id string, at time.Time) error {
	return r.mutate(ctx, owner, func(m *monitoring.MemoryRepository) error { return m.ArchiveTask(ctx, owner, id, at) })
}
func (r *MonitoringRepository) ReplaceTaskTargets(ctx context.Context, owner, id string, targets []monitoring.TaskTarget) error {
	return r.mutate(ctx, owner, func(m *monitoring.MemoryRepository) error { return m.ReplaceTaskTargets(ctx, owner, id, targets) })
}
func (r *MonitoringRepository) EnsureDefaultTask(ctx context.Context, task monitoring.Task) (monitoring.Task, bool, error) {
	type result struct {
		task    monitoring.Task
		created bool
	}
	v, err := withMonitoring(ctx, r.c, task.HubUserID, true, func(m *monitoring.MemoryRepository) (result, error) {
		v, created, err := m.EnsureDefaultTask(ctx, task)
		return result{v, created}, err
	})
	return v.task, v.created, err
}
func (r *MonitoringRepository) RecoverStaleRuns(ctx context.Context, owner string, now time.Time) error {
	return r.mutate(ctx, owner, func(m *monitoring.MemoryRepository) error { return m.RecoverStaleRuns(ctx, owner, now) })
}
func (r *MonitoringRepository) ClaimDueTask(ctx context.Context, owner, run string, now, until time.Time) (monitoring.TaskClaim, error) {
	return withMonitoring(ctx, r.c, owner, true, func(m *monitoring.MemoryRepository) (monitoring.TaskClaim, error) {
		return m.ClaimDueTask(ctx, owner, run, now, until)
	})
}
func (r *MonitoringRepository) StartManualRun(ctx context.Context, owner, task, run, trigger string, now, until time.Time) (monitoring.TaskClaim, error) {
	return withMonitoring(ctx, r.c, owner, true, func(m *monitoring.MemoryRepository) (monitoring.TaskClaim, error) {
		return m.StartManualRun(ctx, owner, task, run, trigger, now, until)
	})
}
func (r *MonitoringRepository) RenewTaskRunLease(ctx context.Context, owner, run string, until time.Time) error {
	return r.mutate(ctx, owner, func(m *monitoring.MemoryRepository) error { return m.RenewTaskRunLease(ctx, owner, run, until) })
}
func (r *MonitoringRepository) CompleteTaskRun(ctx context.Context, owner, run string, update monitoring.TaskRunUpdate) error {
	return r.mutate(ctx, owner, func(m *monitoring.MemoryRepository) error { return m.CompleteTaskRun(ctx, owner, run, update) })
}
func (r *MonitoringRepository) ListTaskRuns(ctx context.Context, owner, task string, limit int) ([]monitoring.TaskRun, error) {
	return withMonitoring(ctx, r.c, owner, false, func(m *monitoring.MemoryRepository) ([]monitoring.TaskRun, error) {
		return m.ListTaskRuns(ctx, owner, task, limit)
	})
}

var _ monitoring.Repository = (*MonitoringRepository)(nil)
