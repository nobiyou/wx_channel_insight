package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const terminalPersistenceTimeout = 10 * time.Second

// BindingResolver returns the current saved bindings. With an empty refs
// argument it returns every saved binding; otherwise it must return one item
// for every requested ref or an error for an unknown ref.
type BindingResolver func(context.Context, string, []BindingRef) ([]TaskTarget, error)

// AccountResolver resolves stable account IDs into validated account targets.
// It owns identity completeness checks while monitoring owns task lifecycle.
type AccountResolver func(context.Context, string, []TaskTarget) ([]TaskTarget, error)

type OperatorRunOptions struct {
	HubUserID      string
	DeviceCount    int
	TaskType       string
	TargetRefs     []BindingRef
	AccountTargets []TaskTarget
	TriggerMode    string
}

type OperatorRunResult struct {
	Status        string
	FinalStatus   string
	Attempted     int
	Succeeded     int
	Failed        int
	Skipped       int
	Errors        []TaskRunError
	OperatorRunID string
	StartedAt     time.Time
	FinishedAt    time.Time
	Message       string
}

type OperatorRunner interface {
	RunMonitoringTask(context.Context, OperatorRunOptions) (OperatorRunResult, error)
}

type ServiceConfig struct {
	DefaultHubUserID   string
	DefaultDeviceCount int
	LeaseDuration      time.Duration
	Now                func() time.Time
	BindingResolver    BindingResolver
	AccountResolver    AccountResolver
}

type Service struct {
	repo               Repository
	defaultHubUserID   string
	defaultDeviceCount int
	leaseDuration      time.Duration
	leaseRenewInterval time.Duration
	now                func() time.Time

	mu              sync.RWMutex
	bindingResolver BindingResolver
	accountResolver AccountResolver
	operatorRunner  OperatorRunner
}

var runSequence atomic.Uint64

func NewService(repo Repository, config ServiceConfig) (*Service, error) {
	if repo == nil {
		return nil, errors.New("monitoring repository is required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	leaseDuration := config.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = 15 * time.Minute
	}
	leaseRenewInterval := leaseDuration / 3
	if leaseRenewInterval <= 0 {
		leaseRenewInterval = leaseDuration
	}
	deviceCount := config.DefaultDeviceCount
	if deviceCount <= 0 {
		deviceCount = 1
	}
	return &Service{
		repo:               repo,
		defaultHubUserID:   strings.TrimSpace(config.DefaultHubUserID),
		defaultDeviceCount: deviceCount,
		leaseDuration:      leaseDuration,
		leaseRenewInterval: leaseRenewInterval,
		now:                now,
		bindingResolver:    config.BindingResolver,
		accountResolver:    config.AccountResolver,
	}, nil
}

func (s *Service) SetBindingResolver(resolver BindingResolver) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.bindingResolver = resolver
	s.mu.Unlock()
}

func (s *Service) SetAccountResolver(resolver AccountResolver) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.accountResolver = resolver
	s.mu.Unlock()
}

func (s *Service) SetOperatorRunner(runner OperatorRunner) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.operatorRunner = runner
	s.mu.Unlock()
}

func (s *Service) ListTasks(ctx context.Context, hubUserID string, includeArchived bool) ([]Task, error) {
	hubUserID = s.resolveHubUserID(hubUserID)
	if _, _, err := s.EnsureDefaultTask(ctx, hubUserID); err != nil {
		return nil, err
	}
	items, err := s.repo.ListTasks(ctx, hubUserID, includeArchived)
	if err != nil {
		return nil, err
	}
	for index := range items {
		items[index] = ApplyTaskSafetyPolicy(items[index])
	}
	return items, nil
}

func (s *Service) GetTask(ctx context.Context, hubUserID, taskID string) (Task, error) {
	task, err := s.repo.GetTask(ctx, s.resolveHubUserID(hubUserID), taskID)
	if err != nil {
		return Task{}, err
	}
	return ApplyTaskSafetyPolicy(task), nil
}

func (s *Service) ListTaskRuns(ctx context.Context, hubUserID, taskID string, limit int) ([]TaskRun, error) {
	return s.repo.ListTaskRuns(ctx, s.resolveHubUserID(hubUserID), taskID, limit)
}

func (s *Service) CreateTask(ctx context.Context, hubUserID string, task Task) (Task, error) {
	hubUserID = s.resolveHubUserID(hubUserID)
	task.HubUserID = hubUserID
	if task.ID == "" {
		task.ID = newID("monitoring-task")
	}
	if task.Status == "" {
		task.Status = StatusEnabled
	}
	if strings.TrimSpace(task.TaskType) == "" {
		task.TaskType = TaskTypeLegacy
	}
	if task.ScopeMode == "" {
		task.ScopeMode = ScopeSelected
	}
	if task.Interval == 0 {
		task.Interval = DefaultTaskInterval
	}
	task, err := s.prepareTaskTargets(ctx, task)
	if err != nil {
		return Task{}, err
	}
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return Task{}, err
	}
	return s.repo.GetTask(ctx, hubUserID, task.ID)
}

func (s *Service) UpdateTask(ctx context.Context, hubUserID string, task Task) (Task, error) {
	hubUserID = s.resolveHubUserID(hubUserID)
	current, err := s.repo.GetTask(ctx, hubUserID, task.ID)
	if err != nil {
		return Task{}, err
	}
	if current.System {
		return Task{}, ErrSystemTaskReadOnly
	}
	task = mergeTaskUpdate(current, task)
	task.HubUserID = hubUserID
	task, err = s.prepareTaskTargets(ctx, task)
	if err != nil {
		return Task{}, err
	}
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return Task{}, err
	}
	return s.repo.GetTask(ctx, hubUserID, task.ID)
}

func (s *Service) ArchiveTask(ctx context.Context, hubUserID, taskID string) error {
	hubUserID = s.resolveHubUserID(hubUserID)
	task, err := s.repo.GetTask(ctx, hubUserID, taskID)
	if err != nil {
		return err
	}
	if task.System {
		return ErrSystemTaskReadOnly
	}
	return s.repo.ArchiveTask(ctx, hubUserID, taskID, s.now())
}

func (s *Service) EnsureDefaultTask(ctx context.Context, hubUserID string) (Task, bool, error) {
	hubUserID = s.resolveHubUserID(hubUserID)
	resolver := s.getBindingResolver()
	if resolver == nil {
		return Task{}, false, nil
	}
	targets, err := resolver(ctx, hubUserID, nil)
	if err != nil {
		return Task{}, false, err
	}
	if len(targets) == 0 {
		return Task{}, false, nil
	}
	now := s.now()
	return s.repo.EnsureDefaultTask(ctx, Task{
		ID:        defaultTaskID(hubUserID),
		HubUserID: hubUserID,
		Name:      DefaultTaskName,
		Status:    StatusEnabled,
		ScopeMode: ScopeAllSavedBindings,
		Interval:  DefaultTaskInterval,
		NextRunAt: now,
		CreatedAt: now,
		UpdatedAt: now,
		System:    true,
	})
}

func (s *Service) RunDueTask(ctx context.Context, hubUserID string) (TaskRun, error) {
	hubUserID = s.resolveHubUserID(hubUserID)
	if _, _, err := s.EnsureDefaultTask(ctx, hubUserID); err != nil {
		return TaskRun{}, err
	}
	now := s.now()
	if err := s.repo.RecoverStaleRuns(ctx, hubUserID, now); err != nil {
		return TaskRun{}, err
	}
	claim, err := s.repo.ClaimDueTask(ctx, hubUserID, newID("monitoring-run"), now, now.Add(s.leaseDuration))
	if err != nil {
		return TaskRun{}, err
	}
	return s.executeClaim(ctx, claim)
}

func (s *Service) RunManualTask(ctx context.Context, hubUserID, taskID string) (TaskRun, error) {
	return s.runManual(ctx, hubUserID, taskID, TriggerManual)
}

func (s *Service) RetryTask(ctx context.Context, hubUserID, taskID string) (TaskRun, error) {
	hubUserID = s.resolveHubUserID(hubUserID)
	runs, err := s.repo.ListTaskRuns(ctx, hubUserID, taskID, 1)
	if err != nil {
		return TaskRun{}, err
	}
	if len(runs) == 0 || !isRetryableStatus(runs[0].Status) {
		return TaskRun{}, ErrNoRetryableRun
	}
	return s.runManual(ctx, hubUserID, taskID, TriggerRetry)
}

func (s *Service) runManual(ctx context.Context, hubUserID, taskID, trigger string) (TaskRun, error) {
	hubUserID = s.resolveHubUserID(hubUserID)
	now := s.now()
	if err := s.repo.RecoverStaleRuns(ctx, hubUserID, now); err != nil {
		return TaskRun{}, err
	}
	claim, err := s.repo.StartManualRun(ctx, hubUserID, taskID, newID("monitoring-run"), trigger, now, now.Add(s.leaseDuration))
	if err != nil {
		return TaskRun{}, err
	}
	return s.executeClaim(ctx, claim)
}

func (s *Service) executeClaim(ctx context.Context, claim TaskClaim) (TaskRun, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	heartbeatStop := make(chan struct{})
	heartbeatDone := make(chan error, 1)
	go func() {
		heartbeatDone <- s.renewLeaseUntilStopped(ctx, claim, heartbeatStop, cancelRun)
	}()

	runner := s.getOperatorRunner()
	result := OperatorRunResult{}
	var runErr error
	if runner == nil {
		runErr = ErrOperatorNotReady
	} else {
		targetRefs := make([]BindingRef, 0, len(claim.Task.Targets))
		accountTargets := make([]TaskTarget, 0, len(claim.Task.Targets))
		switch claim.Task.ScopeMode {
		case ScopeSelected:
			for _, target := range claim.Task.Targets {
				if target.TargetKind == TargetKindAccount {
					accountTargets = append(accountTargets, target)
					continue
				}
				targetRefs = append(targetRefs, target.BindingRef)
			}
		case ScopeAllSavedBindings:
			resolver := s.getBindingResolver()
			if resolver == nil {
				// Legacy non-system tasks may still delegate the all-bindings
				// scope to the operator. The generated default task requires a
				// resolver so it can honor explicit object ownership.
				if claim.Task.System {
					runErr = errors.New("monitoring binding resolver is not configured")
				}
				break
			}
			var savedTargets []TaskTarget
			savedTargets, runErr = resolver(runCtx, claim.Task.HubUserID, nil)
			if runErr != nil {
				break
			}
			managed := make(map[string]struct{})
			// Explicit object tasks own their bindings, even while paused. The
			// default task must not silently resume those targets.
			tasks, listErr := s.repo.ListTasks(runCtx, claim.Task.HubUserID, true)
			if listErr != nil {
				runErr = listErr
				break
			}
			for _, task := range tasks {
				if task.ID == claim.Task.ID || task.ScopeMode != ScopeSelected || task.Status == StatusArchived {
					continue
				}
				for _, target := range task.Targets {
					if target.TargetKind == TargetKindAccount || strings.TrimSpace(target.Source) == "" || strings.TrimSpace(target.TargetKey) == "" {
						continue
					}
					managed[bindingIdentity(target.BindingRef)] = struct{}{}
				}
			}
			for _, target := range savedTargets {
				if _, found := managed[bindingIdentity(target.BindingRef)]; found {
					continue
				}
				targetRefs = append(targetRefs, target.BindingRef)
			}
			if len(targetRefs) == 0 {
				result = OperatorRunResult{Status: RunStatusSkipped, FinalStatus: RunStatusSkipped, Skipped: 1, Message: "没有未被对象任务管理的监控目标"}
			}
		case ScopeAllAccounts:
			resolver := s.getAccountResolver()
			if resolver == nil {
				runErr = errors.New("monitoring account resolver is not configured")
			} else {
				accountTargets, runErr = resolver(runCtx, claim.Task.HubUserID, nil)
				if runErr == nil && len(accountTargets) == 0 {
					runErr = errors.New("no accounts found for all-accounts monitoring task")
				}
			}
		}
		skipEmptyDefault := claim.Task.System && claim.Task.ScopeMode == ScopeAllSavedBindings && len(targetRefs) == 0
		if runErr == nil && !skipEmptyDefault {
			result, runErr = runner.RunMonitoringTask(runCtx, OperatorRunOptions{
				HubUserID:      claim.Task.HubUserID,
				DeviceCount:    s.defaultDeviceCount,
				TaskType:       claim.Task.TaskType,
				TargetRefs:     targetRefs,
				AccountTargets: accountTargets,
				TriggerMode:    "monitoring-" + claim.Run.Trigger,
			})
		}
	}
	close(heartbeatStop)
	heartbeatErr := <-heartbeatDone
	cancelRun()
	if heartbeatErr != nil {
		runErr = errors.Join(runErr, fmt.Errorf("renew monitoring task lease: %w", heartbeatErr))
	}
	finishedAt := result.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = s.now()
	}
	status := classifyRunStatus(result, runErr)
	message := strings.TrimSpace(result.Message)
	if runErr != nil {
		runErrMessage := runErr.Error()
		if message == "" {
			message = runErrMessage
		} else if !strings.Contains(message, runErrMessage) {
			message += "; " + runErrMessage
		}
	}
	update := TaskRunUpdate{
		Status:        status,
		FinishedAt:    finishedAt,
		OperatorRunID: result.OperatorRunID,
		Counts: TaskRunCounts{
			Attempted: result.Attempted,
			Succeeded: result.Succeeded,
			Failed:    result.Failed,
			Skipped:   result.Skipped,
		},
		Message:   message,
		Errors:    append([]TaskRunError(nil), result.Errors...),
		NextRunAt: claim.Task.NextRunAt,
	}
	if claim.Run.Trigger == TriggerScheduled {
		update.NextRunAt = finishedAt.Add(SafeTaskInterval(claim.Task))
	}
	completionCtx, cancelCompletion := context.WithTimeout(context.WithoutCancel(ctx), terminalPersistenceTimeout)
	completionErr := s.repo.CompleteTaskRun(completionCtx, claim.Task.HubUserID, claim.Run.ID, update)
	cancelCompletion()
	if completionErr != nil {
		return TaskRun{}, errors.Join(runErr, fmt.Errorf("complete monitoring task run: %w", completionErr))
	}
	completed := CloneTaskRun(claim.Run)
	completed.Status = update.Status
	completed.FinishedAt = update.FinishedAt
	completed.LeaseUntil = time.Time{}
	completed.OperatorRunID = update.OperatorRunID
	completed.Counts = update.Counts
	completed.Message = update.Message
	completed.Errors = append([]TaskRunError(nil), update.Errors...)
	if runErr != nil {
		return completed, runErr
	}
	return completed, nil
}

func bindingIdentity(ref BindingRef) string {
	return strings.ToLower(strings.TrimSpace(ref.Source)) + "\x00" + strings.TrimSpace(ref.TargetKey)
}

func (s *Service) renewLeaseUntilStopped(parent context.Context, claim TaskClaim, stop <-chan struct{}, cancelRun context.CancelFunc) error {
	ticker := time.NewTicker(s.leaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-ticker.C:
			leaseUntil := s.now().Add(s.leaseDuration)
			// Lease bookkeeping must outlive caller cancellation until the runner returns.
			renewCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), terminalPersistenceTimeout)
			err := s.repo.RenewTaskRunLease(renewCtx, claim.Task.HubUserID, claim.Run.ID, leaseUntil)
			cancel()
			if err != nil {
				cancelRun()
				return err
			}
		}
	}
}

func (s *Service) prepareTaskTargets(ctx context.Context, task Task) (Task, error) {
	if task.ScopeMode != ScopeSelected {
		return NormalizeTask(ApplyTaskSafetyPolicy(task))
	}
	bindingTargets := make([]TaskTarget, 0, len(task.Targets))
	accountTargets := make([]TaskTarget, 0, len(task.Targets))
	for _, target := range task.Targets {
		normalized, err := NormalizeTaskTarget(target)
		if err != nil {
			return Task{}, err
		}
		if normalized.TargetKind == TargetKindAccount {
			accountTargets = append(accountTargets, normalized)
		} else {
			bindingTargets = append(bindingTargets, normalized)
		}
	}
	if resolver := s.getBindingResolver(); resolver != nil && len(bindingTargets) > 0 {
		resolved, err := resolver(ctx, task.HubUserID, bindingRefs(bindingTargets))
		if err != nil {
			return Task{}, err
		}
		bindingTargets = resolved
	}
	if resolver := s.getAccountResolver(); resolver != nil && len(accountTargets) > 0 {
		resolved, err := resolver(ctx, task.HubUserID, accountTargets)
		if err != nil {
			return Task{}, err
		}
		accountTargets = resolved
	}
	task.Targets = append(bindingTargets, accountTargets...)
	return NormalizeTask(ApplyTaskSafetyPolicy(task))
}

func (s *Service) resolveHubUserID(hubUserID string) string {
	if value := strings.TrimSpace(hubUserID); value != "" {
		return value
	}
	return s.defaultHubUserID
}

func (s *Service) getBindingResolver() BindingResolver {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bindingResolver
}

func (s *Service) getAccountResolver() AccountResolver {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accountResolver
}

func (s *Service) getOperatorRunner() OperatorRunner {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.operatorRunner
}

func mergeTaskUpdate(current, update Task) Task {
	if strings.TrimSpace(update.Name) == "" {
		update.Name = current.Name
	}
	if update.Status == "" {
		update.Status = current.Status
	}
	if strings.TrimSpace(update.TaskType) == "" {
		update.TaskType = current.TaskType
	}
	if update.ScopeMode == "" {
		update.ScopeMode = current.ScopeMode
	}
	if update.Interval == 0 {
		update.Interval = current.Interval
	}
	if update.NextRunAt.IsZero() {
		update.NextRunAt = current.NextRunAt
	}
	if update.CreatedAt.IsZero() {
		update.CreatedAt = current.CreatedAt
	}
	if update.UpdatedAt.IsZero() {
		update.UpdatedAt = time.Now().UTC()
	}
	if update.Targets == nil && current.ScopeMode == ScopeSelected {
		update.Targets = append([]TaskTarget(nil), current.Targets...)
	}
	update.LastRunAt = current.LastRunAt
	update.LastSuccessAt = current.LastSuccessAt
	update.LastFailureAt = current.LastFailureAt
	update.LastStatus = current.LastStatus
	update.LastError = current.LastError
	update.System = current.System
	return update
}

func bindingRefs(targets []TaskTarget) []BindingRef {
	refs := make([]BindingRef, 0, len(targets))
	for _, target := range targets {
		refs = append(refs, target.BindingRef)
	}
	return refs
}

func classifyRunStatus(result OperatorRunResult, runErr error) string {
	if result.FinalStatus == RunStatusSkipped || result.Status == RunStatusSkipped {
		return RunStatusSkipped
	}
	if runErr != nil {
		if result.Succeeded > 0 {
			return RunStatusPartial
		}
		return RunStatusFailed
	}
	if result.FinalStatus == RunStatusFailed && result.Succeeded == 0 {
		return RunStatusFailed
	}
	if result.Status == RunStatusPartial || result.FinalStatus == RunStatusPartial || result.Failed > 0 || result.Skipped > 0 {
		return RunStatusPartial
	}
	return RunStatusSuccess
}

func isRetryableStatus(status string) bool {
	switch status {
	case RunStatusFailed, RunStatusPartial, RunStatusSkipped, RunStatusAbandoned:
		return true
	default:
		return false
	}
}

func defaultTaskID(hubUserID string) string {
	digest := sha256.Sum256([]byte(hubUserID))
	return "monitoring-default-" + hex.EncodeToString(digest[:8])
}

func newID(prefix string) string {
	sequence := runSequence.Add(1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UTC().UnixNano(), sequence)
}
