package monitoring

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryState is the portable state representation used by the file adapter.
// It intentionally contains only monitoring data and never source credentials.
type MemoryState struct {
	Tasks map[string]Task      `json:"tasks"`
	Runs  map[string][]TaskRun `json:"runs"`
}

// MemoryRepository provides deterministic repository semantics for demo mode
// and is the state engine used by the file-backed adapter.
type MemoryRepository struct {
	mu    sync.RWMutex
	tasks map[string]Task
	runs  map[string][]TaskRun
}

var _ Repository = (*MemoryRepository)(nil)

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		tasks: map[string]Task{},
		runs:  map[string][]TaskRun{},
	}
}

func (r *MemoryRepository) Snapshot() MemoryState {
	if r == nil {
		return MemoryState{Tasks: map[string]Task{}, Runs: map[string][]TaskRun{}}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshotLocked()
}

func (r *MemoryRepository) Restore(state MemoryState) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreLocked(state)
}

func (r *MemoryRepository) ListTasks(_ context.Context, hubUserID string, includeArchived bool) ([]Task, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]Task, 0)
	for _, task := range r.tasks {
		if task.HubUserID != hubUserID || (!includeArchived && task.Status == StatusArchived) {
			continue
		}
		items = append(items, CloneTask(task))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].NextRunAt.Equal(items[j].NextRunAt) {
			if items[i].CreatedAt.Equal(items[j].CreatedAt) {
				return items[i].ID < items[j].ID
			}
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return items[i].NextRunAt.Before(items[j].NextRunAt)
	})
	return items, nil
}

func (r *MemoryRepository) GetTask(_ context.Context, hubUserID, taskID string) (Task, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return Task{}, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Task{}, ErrTaskNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[taskKey(hubUserID, taskID)]
	if !ok {
		return Task{}, ErrTaskNotFound
	}
	return CloneTask(task), nil
}

func (r *MemoryRepository) CreateTask(_ context.Context, task Task) error {
	task, err := normalizeForStorage(task)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tasks[taskKey(task.HubUserID, task.ID)]; exists {
		return ErrTaskAlreadyExists
	}
	if task.System && r.hasSystemTaskLocked(task.HubUserID) {
		return ErrTaskAlreadyExists
	}
	r.tasks[taskKey(task.HubUserID, task.ID)] = CloneTask(task)
	return nil
}

func (r *MemoryRepository) UpdateTask(_ context.Context, task Task) error {
	task, err := normalizeForStorage(task)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := taskKey(task.HubUserID, task.ID)
	existing, ok := r.tasks[key]
	if !ok {
		return ErrTaskNotFound
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = existing.CreatedAt
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = time.Now().UTC()
	}
	if task.Status == StatusArchived && task.ArchivedAt.IsZero() {
		task.ArchivedAt = task.UpdatedAt
	}
	if task.System && !existing.System && r.hasOtherSystemTaskLocked(task.HubUserID, task.ID) {
		return ErrTaskAlreadyExists
	}
	r.tasks[key] = CloneTask(task)
	return nil
}

func (r *MemoryRepository) ArchiveTask(_ context.Context, hubUserID, taskID string, archivedAt time.Time) error {
	if err := requireHubUserID(hubUserID); err != nil {
		return err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ErrTaskNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := taskKey(hubUserID, taskID)
	task, ok := r.tasks[key]
	if !ok {
		return ErrTaskNotFound
	}
	if archivedAt.IsZero() {
		archivedAt = time.Now().UTC()
	}
	task.Status = StatusArchived
	task.ArchivedAt = archivedAt
	task.UpdatedAt = archivedAt
	r.tasks[key] = CloneTask(task)
	return nil
}

func (r *MemoryRepository) ReplaceTaskTargets(_ context.Context, hubUserID, taskID string, targets []TaskTarget) error {
	if err := requireHubUserID(hubUserID); err != nil {
		return err
	}
	if err := ValidateTargets(targets); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := taskKey(hubUserID, strings.TrimSpace(taskID))
	task, ok := r.tasks[key]
	if !ok {
		return ErrTaskNotFound
	}
	if task.ScopeMode != ScopeSelected {
		return errors.New("all-saved-bindings task cannot replace selected targets")
	}
	task.Targets = cloneTargets(targets)
	task.UpdatedAt = time.Now().UTC()
	r.tasks[key] = CloneTask(task)
	return nil
}

func (r *MemoryRepository) EnsureDefaultTask(_ context.Context, task Task) (Task, bool, error) {
	task.System = true
	task, err := normalizeForStorage(task)
	if err != nil {
		return Task{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.tasks {
		if existing.HubUserID == task.HubUserID && existing.System {
			return CloneTask(existing), false, nil
		}
	}
	if _, exists := r.tasks[taskKey(task.HubUserID, task.ID)]; exists {
		return Task{}, false, ErrTaskAlreadyExists
	}
	r.tasks[taskKey(task.HubUserID, task.ID)] = CloneTask(task)
	return CloneTask(task), true, nil
}

func (r *MemoryRepository) RecoverStaleRuns(_ context.Context, hubUserID string, now time.Time) error {
	if err := requireHubUserID(hubUserID); err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, runs := range r.runs {
		for index := range runs {
			run := &runs[index]
			if run.HubUserID != hubUserID || run.Status != RunStatusRunning || run.LeaseUntil.IsZero() || run.LeaseUntil.After(now) {
				continue
			}
			run.Status = RunStatusAbandoned
			run.FinishedAt = now
			run.LeaseUntil = time.Time{}
			if strings.TrimSpace(run.Message) == "" {
				run.Message = "monitoring task lease expired"
			}
			if task, ok := r.tasks[taskKey(hubUserID, run.TaskID)]; ok {
				task.LastStatus = RunStatusAbandoned
				task.LastError = run.Message
				task.LastFailureAt = now
				task.UpdatedAt = now
				r.tasks[taskKey(hubUserID, run.TaskID)] = CloneTask(task)
			}
		}
		r.runs[key] = append([]TaskRun(nil), runs...)
	}
	return nil
}

func (r *MemoryRepository) ClaimDueTask(_ context.Context, hubUserID, runID string, now, leaseUntil time.Time) (TaskClaim, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return TaskClaim{}, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return TaskClaim{}, errors.New("monitoring task run id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if leaseUntil.IsZero() {
		leaseUntil = now.Add(15 * time.Minute)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.findDueTaskLocked(hubUserID, now)
	if !ok {
		return TaskClaim{}, ErrNoDueTask
	}
	return r.startRunLocked(task, runID, TriggerScheduled, now, leaseUntil)
}

func (r *MemoryRepository) StartManualRun(_ context.Context, hubUserID, taskID, runID, trigger string, now, leaseUntil time.Time) (TaskClaim, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return TaskClaim{}, err
	}
	taskID = strings.TrimSpace(taskID)
	runID = strings.TrimSpace(runID)
	if taskID == "" {
		return TaskClaim{}, ErrTaskNotFound
	}
	if runID == "" {
		return TaskClaim{}, errors.New("monitoring task run id is required")
	}
	if trigger == "" {
		trigger = TriggerManual
	}
	if trigger != TriggerManual && trigger != TriggerRetry {
		return TaskClaim{}, errors.New("unsupported monitoring task trigger")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if leaseUntil.IsZero() {
		leaseUntil = now.Add(15 * time.Minute)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskKey(hubUserID, taskID)]
	if !ok {
		return TaskClaim{}, ErrTaskNotFound
	}
	if task.Status == StatusArchived {
		return TaskClaim{}, ErrTaskArchived
	}
	if r.hasActiveRunLocked(hubUserID, taskID, now) {
		return TaskClaim{}, ErrTaskRunInProgress
	}
	return r.startRunLocked(task, runID, trigger, now, leaseUntil)
}

func (r *MemoryRepository) RenewTaskRunLease(_ context.Context, hubUserID, runID string, leaseUntil time.Time) error {
	if err := requireHubUserID(hubUserID); err != nil {
		return err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ErrTaskRunNotFound
	}
	if leaseUntil.IsZero() {
		return errors.New("monitoring task lease deadline is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, runs := range r.runs {
		for index := range runs {
			run := &runs[index]
			if run.HubUserID != hubUserID || run.ID != runID {
				continue
			}
			if run.Status != RunStatusRunning {
				return ErrTaskRunCompleted
			}
			if leaseUntil.After(run.LeaseUntil) {
				run.LeaseUntil = leaseUntil
				r.runs[key] = append([]TaskRun(nil), runs...)
			}
			return nil
		}
	}
	return ErrTaskRunNotFound
}

func (r *MemoryRepository) CompleteTaskRun(_ context.Context, hubUserID, runID string, update TaskRunUpdate) error {
	if err := requireHubUserID(hubUserID); err != nil {
		return err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ErrTaskRunNotFound
	}
	if err := validateRunStatus(update.Status); err != nil {
		return err
	}
	now := time.Now().UTC()
	if update.FinishedAt.IsZero() {
		update.FinishedAt = now
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, runs := range r.runs {
		for index := range runs {
			run := &runs[index]
			if run.HubUserID != hubUserID || run.ID != runID {
				continue
			}
			if run.Status != RunStatusRunning {
				return ErrTaskRunCompleted
			}
			task, ok := r.tasks[taskKey(hubUserID, run.TaskID)]
			if !ok {
				return ErrTaskNotFound
			}
			if update.NextRunAt.IsZero() {
				update.NextRunAt = task.NextRunAt
			}
			run.Status = update.Status
			run.FinishedAt = update.FinishedAt
			run.LeaseUntil = time.Time{}
			run.OperatorRunID = strings.TrimSpace(update.OperatorRunID)
			run.Counts = update.Counts
			run.Message = strings.TrimSpace(update.Message)
			run.Errors = append([]TaskRunError(nil), update.Errors...)
			task.NextRunAt = update.NextRunAt
			task.LastStatus = update.Status
			task.LastError = run.Message
			task.UpdatedAt = update.FinishedAt
			if update.Status == RunStatusSuccess || update.Status == RunStatusPartial {
				task.LastSuccessAt = update.FinishedAt
			}
			if update.Status == RunStatusFailed || update.Status == RunStatusAbandoned {
				task.LastFailureAt = update.FinishedAt
			}
			r.tasks[taskKey(hubUserID, run.TaskID)] = CloneTask(task)
			r.runs[key] = append([]TaskRun(nil), runs...)
			return nil
		}
	}
	return ErrTaskRunNotFound
}

func (r *MemoryRepository) ListTaskRuns(_ context.Context, hubUserID, taskID string, limit int) ([]TaskRun, error) {
	if err := requireHubUserID(hubUserID); err != nil {
		return nil, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, ErrTaskNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.tasks[taskKey(hubUserID, taskID)]; !ok {
		return nil, ErrTaskNotFound
	}
	items := append([]TaskRun(nil), r.runs[taskKey(hubUserID, taskID)]...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if len(items) > limit {
		items = items[:limit]
	}
	result := make([]TaskRun, 0, len(items))
	for _, item := range items {
		result = append(result, CloneTaskRun(item))
	}
	return result, nil
}

func (r *MemoryRepository) snapshotLocked() MemoryState {
	state := MemoryState{
		Tasks: make(map[string]Task, len(r.tasks)),
		Runs:  make(map[string][]TaskRun, len(r.runs)),
	}
	for key, task := range r.tasks {
		state.Tasks[key] = CloneTask(task)
	}
	for key, runs := range r.runs {
		copied := make([]TaskRun, 0, len(runs))
		for _, run := range runs {
			copied = append(copied, CloneTaskRun(run))
		}
		state.Runs[key] = copied
	}
	return state
}

func (r *MemoryRepository) restoreLocked(state MemoryState) {
	r.tasks = map[string]Task{}
	r.runs = map[string][]TaskRun{}
	for key, task := range state.Tasks {
		r.tasks[key] = CloneTask(task)
	}
	for key, runs := range state.Runs {
		copied := make([]TaskRun, 0, len(runs))
		for _, run := range runs {
			copied = append(copied, CloneTaskRun(run))
		}
		r.runs[key] = copied
	}
}

func (r *MemoryRepository) hasSystemTaskLocked(hubUserID string) bool {
	for _, task := range r.tasks {
		if task.HubUserID == hubUserID && task.System {
			return true
		}
	}
	return false
}

func (r *MemoryRepository) hasOtherSystemTaskLocked(hubUserID, taskID string) bool {
	for _, task := range r.tasks {
		if task.HubUserID == hubUserID && task.ID != taskID && task.System {
			return true
		}
	}
	return false
}

func (r *MemoryRepository) findDueTaskLocked(hubUserID string, now time.Time) (Task, bool) {
	var candidates []Task
	for _, task := range r.tasks {
		if task.HubUserID != hubUserID || task.Status != StatusEnabled || task.NextRunAt.After(now) || r.hasActiveRunLocked(hubUserID, task.ID, now) {
			continue
		}
		candidates = append(candidates, task)
	}
	if len(candidates) == 0 {
		return Task{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].NextRunAt.Equal(candidates[j].NextRunAt) {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].NextRunAt.Before(candidates[j].NextRunAt)
	})
	return candidates[0], true
}

func (r *MemoryRepository) hasActiveRunLocked(hubUserID, taskID string, now time.Time) bool {
	for _, run := range r.runs[taskKey(hubUserID, taskID)] {
		if run.Status != RunStatusRunning || run.FinishedAt.IsZero() == false {
			continue
		}
		if run.LeaseUntil.IsZero() || run.LeaseUntil.After(now) {
			return true
		}
	}
	return false
}

func (r *MemoryRepository) startRunLocked(task Task, runID, trigger string, now, leaseUntil time.Time) (TaskClaim, error) {
	if _, exists := r.runs[taskKey(task.HubUserID, task.ID)]; !exists {
		r.runs[taskKey(task.HubUserID, task.ID)] = []TaskRun{}
	}
	attempt := 1
	for _, previous := range r.runs[taskKey(task.HubUserID, task.ID)] {
		if previous.Attempt >= attempt {
			attempt = previous.Attempt + 1
		}
	}
	run := TaskRun{
		ID:         runID,
		TaskID:     task.ID,
		HubUserID:  task.HubUserID,
		Trigger:    trigger,
		Status:     RunStatusRunning,
		Attempt:    attempt,
		StartedAt:  now,
		LeaseUntil: leaseUntil,
	}
	r.runs[taskKey(task.HubUserID, task.ID)] = append(r.runs[taskKey(task.HubUserID, task.ID)], run)
	task.LastRunAt = now
	task.LastStatus = RunStatusRunning
	task.LastError = ""
	task.UpdatedAt = now
	r.tasks[taskKey(task.HubUserID, task.ID)] = CloneTask(task)
	return TaskClaim{Task: CloneTask(task), Run: CloneTaskRun(run)}, nil
}

func normalizeForStorage(task Task) (Task, error) {
	normalized, err := NormalizeTask(task)
	if err != nil {
		return Task{}, err
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
	if normalized.Status == StatusArchived && normalized.ArchivedAt.IsZero() {
		normalized.ArchivedAt = normalized.UpdatedAt
	}
	return normalized, nil
}

func cloneTargets(targets []TaskTarget) []TaskTarget {
	return append([]TaskTarget(nil), targets...)
}

func taskKey(hubUserID, taskID string) string {
	return hubUserID + "\x00" + taskID
}

func requireHubUserID(hubUserID string) error {
	if strings.TrimSpace(hubUserID) == "" {
		return errors.New("monitoring task hub user id is required")
	}
	return nil
}

func validateRunStatus(status string) error {
	switch status {
	case RunStatusSuccess, RunStatusPartial, RunStatusFailed, RunStatusSkipped, RunStatusAbandoned:
		return nil
	default:
		return errors.New("unsupported monitoring task run status")
	}
}
