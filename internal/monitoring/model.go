package monitoring

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

const (
	StatusEnabled  = "enabled"
	StatusPaused   = "paused"
	StatusArchived = "archived"

	TargetKindBinding = "binding"
	TargetKindAccount = "account"

	TaskTypeLegacy         = "legacy"
	TaskTypeAccountProfile = "account_profile"
	TaskTypeVideoList      = "video_list"
	TaskTypeCommentCollect = "comment_collection"

	CommentModeIncremental  = "incremental"
	CommentModeFullBackfill = "full_backfill"

	ScopeAllSavedBindings = "all_saved_bindings"
	ScopeAllAccounts      = "all_accounts"
	ScopeSelected         = "selected"

	TriggerScheduled = "scheduled"
	TriggerManual    = "manual"
	TriggerRetry     = "retry"

	RunStatusRunning   = "running"
	RunStatusSuccess   = "success"
	RunStatusPartial   = "partial"
	RunStatusFailed    = "failed"
	RunStatusSkipped   = "skipped"
	RunStatusAbandoned = "abandoned"

	DefaultTaskName        = "默认监控"
	DefaultTaskInterval    = 30 * time.Minute
	MinTaskInterval        = time.Minute
	MinCommentTaskInterval = 30 * time.Minute
	MaxTaskInterval        = 7 * 24 * time.Hour
)

var (
	ErrTaskNotFound       = errors.New("monitoring task not found")
	ErrTaskArchived       = errors.New("monitoring task is archived")
	ErrNoDueTask          = errors.New("no monitoring task is due")
	ErrTaskAlreadyExists  = errors.New("monitoring task already exists")
	ErrTaskRunNotFound    = errors.New("monitoring task run not found")
	ErrTaskRunInProgress  = errors.New("monitoring task run is already in progress")
	ErrTaskRunCompleted   = errors.New("monitoring task run is already completed")
	ErrNoRetryableRun     = errors.New("monitoring task has no retryable run")
	ErrOperatorNotReady   = errors.New("monitoring operator runner is not configured")
	ErrSystemTaskReadOnly = errors.New("system monitoring task is read-only")
)

type BindingRef struct {
	Source    string `json:"source"`
	TargetKey string `json:"targetKey"`
}

type TaskTarget struct {
	BindingRef
	TargetKind  string `json:"targetKind,omitempty"`
	AccountID   string `json:"accountId,omitempty"`
	CommentMode string `json:"commentMode,omitempty"`
	Label       string `json:"label,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

type Task struct {
	ID            string        `json:"id"`
	HubUserID     string        `json:"hubUserId"`
	Name          string        `json:"name"`
	TaskType      string        `json:"taskType"`
	Status        string        `json:"status"`
	ScopeMode     string        `json:"scopeMode"`
	Interval      time.Duration `json:"interval"`
	NextRunAt     time.Time     `json:"nextRunAt"`
	LastRunAt     time.Time     `json:"lastRunAt"`
	LastSuccessAt time.Time     `json:"lastSuccessAt"`
	LastFailureAt time.Time     `json:"lastFailureAt"`
	LastStatus    string        `json:"lastStatus,omitempty"`
	LastError     string        `json:"lastError,omitempty"`
	System        bool          `json:"system"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
	ArchivedAt    time.Time     `json:"archivedAt"`
	Targets       []TaskTarget  `json:"targets,omitempty"`
}

type TaskRunCounts struct {
	Attempted int `json:"attempted"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

type TaskRunError struct {
	Source    string `json:"source"`
	TargetKey string `json:"targetKey,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type TaskRun struct {
	ID            string         `json:"id"`
	TaskID        string         `json:"taskId"`
	HubUserID     string         `json:"hubUserId"`
	Trigger       string         `json:"trigger"`
	Status        string         `json:"status"`
	Attempt       int            `json:"attempt"`
	StartedAt     time.Time      `json:"startedAt"`
	FinishedAt    time.Time      `json:"finishedAt"`
	LeaseUntil    time.Time      `json:"leaseUntil"`
	OperatorRunID string         `json:"operatorRunId,omitempty"`
	Counts        TaskRunCounts  `json:"counts"`
	Message       string         `json:"message,omitempty"`
	Errors        []TaskRunError `json:"errors,omitempty"`
}

type TaskClaim struct {
	Task Task    `json:"task"`
	Run  TaskRun `json:"run"`
}

type TaskRunUpdate struct {
	Status        string
	FinishedAt    time.Time
	OperatorRunID string
	Counts        TaskRunCounts
	Message       string
	Errors        []TaskRunError
	NextRunAt     time.Time
}

func (t Task) Validate() error {
	if strings.TrimSpace(t.TaskType) == "" {
		t.TaskType = TaskTypeLegacy
	}
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("monitoring task id is required")
	}
	if strings.TrimSpace(t.HubUserID) == "" {
		return errors.New("monitoring task hub user id is required")
	}
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("monitoring task name is required")
	}
	switch t.TaskType {
	case TaskTypeLegacy, TaskTypeAccountProfile, TaskTypeVideoList, TaskTypeCommentCollect:
	default:
		return fmt.Errorf("unsupported monitoring task type %q", t.TaskType)
	}
	switch t.Status {
	case StatusEnabled, StatusPaused, StatusArchived:
	default:
		return fmt.Errorf("unsupported monitoring task status %q", t.Status)
	}
	switch t.ScopeMode {
	case ScopeAllSavedBindings:
		if len(t.Targets) != 0 {
			return errors.New("all-saved-bindings task cannot have selected targets")
		}
	case ScopeAllAccounts:
		if len(t.Targets) != 0 {
			return errors.New("all-accounts task cannot have selected targets")
		}
	case ScopeSelected:
		if err := ValidateTargets(t.Targets); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported monitoring task scope %q", t.ScopeMode)
	}
	if t.ScopeMode == ScopeAllAccounts && t.TaskType != TaskTypeAccountProfile {
		return fmt.Errorf("%s task does not support all-accounts scope", t.TaskType)
	}
	if IsSeparatedTaskType(t.TaskType) {
		if t.ScopeMode == ScopeAllAccounts {
			return validateAllAccountsTask(t)
		}
		if t.ScopeMode != ScopeSelected {
			return fmt.Errorf("%s task requires selected account targets", t.TaskType)
		}
		for index, target := range t.Targets {
			if target.TargetKind != TargetKindAccount {
				return fmt.Errorf("%s task target %d must be an account", t.TaskType, index)
			}
		}
	}
	if t.Interval < MinTaskInterval || t.Interval > MaxTaskInterval {
		return fmt.Errorf(
			"monitoring task interval must be between %s and %s, got %s",
			MinTaskInterval,
			MaxTaskInterval,
			t.Interval,
		)
	}
	if TaskCollectsComments(t) && t.Interval < MinCommentTaskInterval {
		return fmt.Errorf(
			"monitoring task interval for comment collection must be at least %s, got %s",
			MinCommentTaskInterval,
			t.Interval,
		)
	}
	return nil
}

func validateAllAccountsTask(task Task) error {
	if task.TaskType != TaskTypeAccountProfile {
		return fmt.Errorf("%s task does not support all-accounts scope", task.TaskType)
	}
	if len(task.Targets) != 0 {
		return errors.New("all-accounts task cannot have selected targets")
	}
	return nil
}

// TaskCollectsComments identifies tasks that can issue comment-list requests.
// All-saved tasks are treated conservatively because bindings may change after
// the task is created.
func TaskCollectsComments(task Task) bool {
	if task.TaskType == TaskTypeAccountProfile || task.TaskType == TaskTypeVideoList {
		return false
	}
	if task.TaskType == TaskTypeCommentCollect {
		return true
	}
	if task.ScopeMode == ScopeAllSavedBindings {
		return true
	}
	for _, target := range task.Targets {
		if strings.EqualFold(strings.TrimSpace(target.TargetKind), TargetKindAccount) ||
			strings.EqualFold(strings.TrimSpace(target.Source), TargetKindAccount) ||
			strings.TrimSpace(target.Source) == ingest.RunSourceWXCommentExport {
			return true
		}
	}
	return false
}

// SafeTaskInterval keeps legacy persisted tasks from scheduling comment
// collection more frequently than the current safety policy permits.
func SafeTaskInterval(task Task) time.Duration {
	if TaskCollectsComments(task) && task.Interval < MinCommentTaskInterval {
		return MinCommentTaskInterval
	}
	return task.Interval
}

func ApplyTaskSafetyPolicy(task Task) Task {
	task.Interval = SafeTaskInterval(task)
	return task
}

// NormalizeTask keeps storage adapters from persisting alternate spellings of
// task identity fields or target references.
func NormalizeTask(task Task) (Task, error) {
	task.ID = strings.TrimSpace(task.ID)
	task.HubUserID = strings.TrimSpace(task.HubUserID)
	task.Name = strings.TrimSpace(task.Name)
	task.TaskType = strings.ToLower(strings.TrimSpace(task.TaskType))
	if task.TaskType == "" {
		task.TaskType = TaskTypeLegacy
	}
	for index := range task.Targets {
		target, err := NormalizeTaskTarget(task.Targets[index])
		if err != nil {
			return Task{}, fmt.Errorf("target %d: %w", index, err)
		}
		task.Targets[index] = target
	}
	if err := task.Validate(); err != nil {
		return Task{}, err
	}
	if task.Status == StatusArchived && task.ArchivedAt.IsZero() {
		task.ArchivedAt = task.UpdatedAt
	}
	if task.Status != StatusArchived {
		task.ArchivedAt = time.Time{}
	}
	return task, nil
}

func IsSeparatedTaskType(taskType string) bool {
	switch strings.ToLower(strings.TrimSpace(taskType)) {
	case TaskTypeAccountProfile, TaskTypeVideoList, TaskTypeCommentCollect:
		return true
	default:
		return false
	}
}

func ValidateTargets(targets []TaskTarget) error {
	if len(targets) == 0 {
		return errors.New("selected monitoring task requires at least one target")
	}
	seen := make(map[string]struct{}, len(targets))
	for index, target := range targets {
		normalized, err := NormalizeTaskTarget(target)
		if err != nil {
			return fmt.Errorf("target %d: %w", index, err)
		}
		key := taskTargetKey(normalized)
		if _, exists := seen[key]; exists {
			if normalized.TargetKind == TargetKindAccount {
				return fmt.Errorf("duplicate monitoring account target %s", normalized.AccountID)
			}
			return fmt.Errorf("duplicate monitoring task target %s[%s]", normalized.Source, normalized.TargetKey)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func NormalizeTaskTarget(target TaskTarget) (TaskTarget, error) {
	target.TargetKind = strings.ToLower(strings.TrimSpace(target.TargetKind))
	if target.TargetKind == "" {
		target.TargetKind = TargetKindBinding
	}
	target.Label = strings.TrimSpace(target.Label)
	target.DisplayName = strings.TrimSpace(target.DisplayName)
	switch target.TargetKind {
	case TargetKindBinding:
		ref, err := NormalizeBindingRef(target.BindingRef)
		if err != nil {
			return TaskTarget{}, err
		}
		target.BindingRef = ref
		target.AccountID = ""
		target.CommentMode = ""
		return target, nil
	case TargetKindAccount:
		target.AccountID = strings.TrimSpace(target.AccountID)
		if target.AccountID == "" {
			return TaskTarget{}, errors.New("monitoring account target account id is required")
		}
		if target.CommentMode == "" {
			target.CommentMode = CommentModeIncremental
		}
		target.CommentMode = strings.ToLower(strings.TrimSpace(target.CommentMode))
		if target.CommentMode != CommentModeIncremental && target.CommentMode != CommentModeFullBackfill {
			return TaskTarget{}, fmt.Errorf("unsupported monitoring account comment mode %q", target.CommentMode)
		}
		// Keep the legacy composite key populated so existing Postgres primary
		// keys remain unique. Account identity still comes only from AccountID.
		target.BindingRef = BindingRef{Source: TargetKindAccount, TargetKey: target.AccountID}
		return target, nil
	default:
		return TaskTarget{}, fmt.Errorf("unsupported monitoring target kind %q", target.TargetKind)
	}
}

func taskTargetKey(target TaskTarget) string {
	if target.TargetKind == TargetKindAccount {
		return TargetKindAccount + "\x00" + target.AccountID
	}
	return TargetKindBinding + "\x00" + target.Source + "\x00" + target.TargetKey
}

func NormalizeBindingRef(ref BindingRef) (BindingRef, error) {
	ref.Source = strings.TrimSpace(ref.Source)
	ref.TargetKey = strings.TrimSpace(ref.TargetKey)
	switch ref.Source {
	case ingest.RunSourceWXCommentExport, ingest.RunSourceWXFeedList, ingest.RunSourceWXVideoSearch, ingest.RunSourceWXManualShare:
	default:
		return BindingRef{}, fmt.Errorf("unsupported monitoring target source %q", ref.Source)
	}
	if ref.TargetKey == "" {
		return BindingRef{}, errors.New("monitoring target key is required")
	}
	return ref, nil
}

func CloneTask(task Task) Task {
	task.Targets = append([]TaskTarget(nil), task.Targets...)
	return task
}

func CloneTaskRun(run TaskRun) TaskRun {
	run.Errors = append([]TaskRunError(nil), run.Errors...)
	return run
}
