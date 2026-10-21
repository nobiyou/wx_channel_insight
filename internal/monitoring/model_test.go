package monitoring

import (
	"strings"
	"testing"
	"time"
)

func validTask() Task {
	return Task{
		ID:        "task-1",
		HubUserID: "operator-1",
		Name:      "视频监控",
		Status:    StatusEnabled,
		ScopeMode: ScopeSelected,
		Interval:  5 * time.Minute,
		Targets: []TaskTarget{
			{BindingRef: BindingRef{Source: "wx_feed_list", TargetKey: "account-1"}},
		},
	}
}

func TestTaskValidateAcceptsSelectedTask(t *testing.T) {
	if err := validTask().Validate(); err != nil {
		t.Fatalf("expected valid task, got %v", err)
	}
}

func TestTaskValidateAcceptsAllAccountProfileScope(t *testing.T) {
	task := Task{
		ID: "all-accounts-profile", HubUserID: "operator-1", Name: "全部账号资料",
		TaskType: TaskTypeAccountProfile, Status: StatusEnabled,
		ScopeMode: ScopeAllAccounts, Interval: 30 * time.Minute,
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("expected all-account profile task to be valid, got %v", err)
	}
}

func TestTaskValidateRejectsAllAccountScopeForOtherTaskTypes(t *testing.T) {
	for _, taskType := range []string{TaskTypeVideoList, TaskTypeCommentCollect, TaskTypeLegacy} {
		task := Task{
			ID: "all-accounts-invalid", HubUserID: "operator-1", Name: "全部账号",
			TaskType: taskType, Status: StatusEnabled,
			ScopeMode: ScopeAllAccounts, Interval: 30 * time.Minute,
		}
		if err := task.Validate(); err == nil || !strings.Contains(err.Error(), "does not support all-accounts scope") {
			t.Fatalf("task type %q should reject all-account scope, got %v", taskType, err)
		}
	}
}

func TestTaskValidateRequiresSafeIntervalForCommentCollection(t *testing.T) {
	tests := []struct {
		name string
		task Task
	}{
		{
			name: "account target",
			task: Task{
				ID: "account-task", HubUserID: "operator-1", Name: "账号监控", Status: StatusEnabled,
				ScopeMode: ScopeSelected, Interval: 5 * time.Minute,
				Targets: []TaskTarget{{TargetKind: TargetKindAccount, AccountID: "account-1"}},
			},
		},
		{
			name: "comment binding",
			task: Task{
				ID: "comment-task", HubUserID: "operator-1", Name: "评论监控", Status: StatusEnabled,
				ScopeMode: ScopeSelected, Interval: 15 * time.Minute,
				Targets: []TaskTarget{{BindingRef: BindingRef{Source: "wx_comment_export", TargetKey: "video-1"}}},
			},
		},
		{
			name: "all saved bindings",
			task: Task{
				ID: "all-task", HubUserID: "operator-1", Name: "全部来源", Status: StatusEnabled,
				ScopeMode: ScopeAllSavedBindings, Interval: 5 * time.Minute,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.task.Validate()
			if err == nil || !strings.Contains(err.Error(), "comment collection must be at least 30m") {
				t.Fatalf("expected comment safety interval error, got %v", err)
			}
			if got := SafeTaskInterval(test.task); got != MinCommentTaskInterval {
				t.Fatalf("expected safe interval %s, got %s", MinCommentTaskInterval, got)
			}
		})
	}
}

func TestTaskValidateRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Task)
		want   string
	}{
		{
			name: "missing name",
			mutate: func(task *Task) {
				task.Name = " "
			},
			want: "name is required",
		},
		{
			name: "unsupported status",
			mutate: func(task *Task) {
				task.Status = "running"
			},
			want: "unsupported monitoring task status",
		},
		{
			name: "interval too short",
			mutate: func(task *Task) {
				task.Interval = 30 * time.Second
			},
			want: "interval must be between",
		},
		{
			name: "selected task without targets",
			mutate: func(task *Task) {
				task.Targets = nil
			},
			want: "requires at least one target",
		},
		{
			name: "all task with targets",
			mutate: func(task *Task) {
				task.ScopeMode = ScopeAllSavedBindings
			},
			want: "cannot have selected targets",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := validTask()
			test.mutate(&task)
			err := task.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestValidateTargetsRejectsUnsupportedAndDuplicateTargets(t *testing.T) {
	tests := []struct {
		name    string
		targets []TaskTarget
		want    string
	}{
		{
			name: "unsupported source",
			targets: []TaskTarget{
				{BindingRef: BindingRef{Source: "unknown", TargetKey: "x"}},
			},
			want: "unsupported monitoring target source",
		},
		{
			name: "duplicate ref",
			targets: []TaskTarget{
				{BindingRef: BindingRef{Source: "wx_feed_list", TargetKey: "x"}},
				{BindingRef: BindingRef{Source: "wx_feed_list", TargetKey: "x"}},
			},
			want: "duplicate monitoring task target",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateTargets(test.targets)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestNormalizeBindingRefTrimsValues(t *testing.T) {
	ref, err := NormalizeBindingRef(BindingRef{
		Source:    " wx_video_search ",
		TargetKey: " keyword ",
	})
	if err != nil {
		t.Fatalf("normalize target: %v", err)
	}
	if ref.Source != "wx_video_search" || ref.TargetKey != "keyword" {
		t.Fatalf("unexpected normalized ref: %#v", ref)
	}
}

func TestCloneTaskAndTaskRunDoNotShareMutableSlices(t *testing.T) {
	task := validTask()
	taskCopy := CloneTask(task)
	taskCopy.Targets[0].TargetKey = "changed"
	if task.Targets[0].TargetKey != "account-1" {
		t.Fatal("cloned task shares target backing array")
	}

	run := TaskRun{
		Errors: []TaskRunError{{Source: "wx_feed_list", Message: "failed"}},
	}
	runCopy := CloneTaskRun(run)
	runCopy.Errors[0].Message = "changed"
	if run.Errors[0].Message != "failed" {
		t.Fatal("cloned run shares error backing array")
	}
}
