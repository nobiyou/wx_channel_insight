package monitoringfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

type Repository struct {
	mu     sync.Mutex
	path   string
	memory *monitoring.MemoryRepository
}

var _ monitoring.Repository = (*Repository)(nil)

func NewRepository(path string) (*Repository, error) {
	if path == "" {
		return nil, errors.New("monitoring repository path is required")
	}
	repo := &Repository{
		path:   path,
		memory: monitoring.NewMemoryRepository(),
	}
	if err := repo.loadLocked(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *Repository) ListTasks(ctx context.Context, hubUserID string, includeArchived bool) ([]monitoring.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	return r.memory.ListTasks(ctx, hubUserID, includeArchived)
}

func (r *Repository) GetTask(ctx context.Context, hubUserID, taskID string) (monitoring.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return monitoring.Task{}, err
	}
	return r.memory.GetTask(ctx, hubUserID, taskID)
}

func (r *Repository) CreateTask(ctx context.Context, task monitoring.Task) error {
	return r.mutate(func() error { return r.memory.CreateTask(ctx, task) })
}

func (r *Repository) UpdateTask(ctx context.Context, task monitoring.Task) error {
	return r.mutate(func() error { return r.memory.UpdateTask(ctx, task) })
}

func (r *Repository) ArchiveTask(ctx context.Context, hubUserID, taskID string, archivedAt time.Time) error {
	return r.mutate(func() error { return r.memory.ArchiveTask(ctx, hubUserID, taskID, archivedAt) })
}

func (r *Repository) ReplaceTaskTargets(ctx context.Context, hubUserID, taskID string, targets []monitoring.TaskTarget) error {
	return r.mutate(func() error { return r.memory.ReplaceTaskTargets(ctx, hubUserID, taskID, targets) })
}

func (r *Repository) EnsureDefaultTask(ctx context.Context, task monitoring.Task) (monitoring.Task, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return monitoring.Task{}, false, err
	}
	result, created, err := r.memory.EnsureDefaultTask(ctx, task)
	if err != nil {
		return monitoring.Task{}, false, err
	}
	if err := r.persistLocked(); err != nil {
		return monitoring.Task{}, false, err
	}
	return result, created, nil
}

func (r *Repository) RecoverStaleRuns(ctx context.Context, hubUserID string, now time.Time) error {
	return r.mutate(func() error { return r.memory.RecoverStaleRuns(ctx, hubUserID, now) })
}

func (r *Repository) ClaimDueTask(ctx context.Context, hubUserID, runID string, now, leaseUntil time.Time) (monitoring.TaskClaim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return monitoring.TaskClaim{}, err
	}
	claim, err := r.memory.ClaimDueTask(ctx, hubUserID, runID, now, leaseUntil)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	if err := r.persistLocked(); err != nil {
		return monitoring.TaskClaim{}, err
	}
	return claim, nil
}

func (r *Repository) StartManualRun(ctx context.Context, hubUserID, taskID, runID, trigger string, now, leaseUntil time.Time) (monitoring.TaskClaim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return monitoring.TaskClaim{}, err
	}
	claim, err := r.memory.StartManualRun(ctx, hubUserID, taskID, runID, trigger, now, leaseUntil)
	if err != nil {
		return monitoring.TaskClaim{}, err
	}
	if err := r.persistLocked(); err != nil {
		return monitoring.TaskClaim{}, err
	}
	return claim, nil
}

func (r *Repository) RenewTaskRunLease(ctx context.Context, hubUserID, runID string, leaseUntil time.Time) error {
	return r.mutate(func() error {
		return r.memory.RenewTaskRunLease(ctx, hubUserID, runID, leaseUntil)
	})
}

func (r *Repository) CompleteTaskRun(ctx context.Context, hubUserID, runID string, update monitoring.TaskRunUpdate) error {
	return r.mutate(func() error { return r.memory.CompleteTaskRun(ctx, hubUserID, runID, update) })
}

func (r *Repository) ListTaskRuns(ctx context.Context, hubUserID, taskID string, limit int) ([]monitoring.TaskRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	return r.memory.ListTaskRuns(ctx, hubUserID, taskID, limit)
}

func (r *Repository) mutate(fn func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return r.persistLocked()
}

func (r *Repository) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		if _, backupErr := os.Stat(r.path + ".bak"); errors.Is(backupErr, os.ErrNotExist) {
			r.memory.Restore(monitoring.MemoryState{})
			return nil
		}
		return r.recoverLocked(nil, err)
	}
	if err != nil {
		return fmt.Errorf("read monitoring store %q: %w", r.path, err)
	}
	state, err := decodeState(data)
	if err != nil {
		return r.recoverLocked(data, err)
	}
	r.memory.Restore(state)
	return nil
}

func (r *Repository) persistLocked() error {
	data, err := json.MarshalIndent(r.memory.Snapshot(), "", "  ")
	if err != nil {
		return err
	}
	// Preserve a validated revision before replacing the primary file. Never
	// rotate a corrupt primary over the last usable backup.
	previous, readErr := os.ReadFile(r.path)
	if errors.Is(readErr, os.ErrNotExist) {
		previous = data
	} else if readErr != nil {
		return fmt.Errorf("read monitoring store %q: %w", r.path, readErr)
	}
	if _, err := decodeState(previous); err != nil {
		return fmt.Errorf("refuse to replace invalid monitoring store %q: %w", r.path, err)
	}
	if err := writeAtomic(r.path+".bak", previous); err != nil {
		return err
	}
	return writeAtomic(r.path, data)
}

func decodeState(data []byte) (monitoring.MemoryState, error) {
	var state monitoring.MemoryState
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	if state.Tasks == nil {
		return state, errors.New("missing tasks object")
	}
	for key, task := range state.Tasks {
		if err := task.Validate(); err != nil {
			return state, fmt.Errorf("invalid task %q: %w", key, err)
		}
	}
	return state, nil
}

func (r *Repository) recoverLocked(corrupt []byte, cause error) error {
	backupPath := r.path + ".bak"
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("monitoring store %q is unavailable (%v); no readable backup %q: %w", r.path, cause, backupPath, err)
	}
	state, err := decodeState(backup)
	if err != nil {
		return fmt.Errorf("monitoring store %q is invalid (%v); backup %q is invalid: %w", r.path, cause, backupPath, err)
	}
	if corrupt != nil {
		archive := r.path + ".corrupt-" + time.Now().UTC().Format("20060102T150405.000000000")
		if err := writeAtomic(archive, corrupt); err != nil {
			return err
		}
	}
	if err := writeAtomic(r.path, backup); err != nil {
		return err
	}
	r.memory.Restore(state)
	log.Printf("monitoring store %q recovered from validated backup %q; corrupt primary preserved", r.path, backupPath)
	return nil
}

// Unique temporary files avoid collisions; Sync makes the contents durable
// before the atomic replacement becomes visible to readers.
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".monitoring-*")
	if err != nil {
		return fmt.Errorf("prepare monitoring store %q: %w", path, err)
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpPath, path)
	}
	if err != nil {
		return fmt.Errorf("persist monitoring store %q: %w", path, err)
	}
	return nil
}
