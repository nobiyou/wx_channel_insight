package monitoringapi

import (
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

func TestTaskRequestApplyParsesDurationAndTargets(t *testing.T) {
	task, err := (TaskRequest{
		Name:      "selected videos",
		ScopeMode: monitoring.ScopeSelected,
		Interval:  "15m",
		Targets: []monitoring.TaskTarget{{BindingRef: monitoring.BindingRef{
			Source: "wx_video_search", TargetKey: "ai",
		}}},
	}).Apply(monitoring.Task{Status: monitoring.StatusEnabled})
	if err != nil {
		t.Fatalf("apply task request: %v", err)
	}
	if task.Name != "selected videos" || task.Interval != 15*time.Minute || len(task.Targets) != 1 {
		t.Fatalf("unexpected task: %#v", task)
	}
}

func TestNewTaskRunResponseMapsCountsAndErrors(t *testing.T) {
	response := NewTaskRunResponse(monitoring.TaskRun{
		ID: "run-1", TaskID: "task-1", HubUserID: "operator-1",
		Trigger: monitoring.TriggerScheduled, Status: monitoring.RunStatusPartial,
		Attempt: 2, StartedAt: time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC),
		Counts: monitoring.TaskRunCounts{Attempted: 2, Succeeded: 1, Skipped: 1},
		Errors: []monitoring.TaskRunError{{Source: "wx_feed_list", TargetKey: "account-1", Status: monitoring.RunStatusSkipped, Message: "missing"}},
	})
	if response.Counts.Attempted != 2 || response.Counts.Succeeded != 1 || len(response.Errors) != 1 || response.Errors[0].TargetKey != "account-1" {
		t.Fatalf("unexpected run response: %#v", response)
	}
	if response.StartedAt == "" {
		t.Fatalf("expected formatted timestamp: %#v", response)
	}
}

func TestNewTaskResponsePreservesAccountTargetContract(t *testing.T) {
	response := NewTaskResponse(monitoring.Task{
		ID: "task-account", HubUserID: "operator-1", Name: "重点账号", Status: monitoring.StatusEnabled,
		ScopeMode: monitoring.ScopeSelected, Interval: monitoring.DefaultTaskInterval,
		Targets: []monitoring.TaskTarget{{
			BindingRef: monitoring.BindingRef{Source: monitoring.TargetKindAccount, TargetKey: "account-1"},
			TargetKind: monitoring.TargetKindAccount, AccountID: "account-1",
			CommentMode: monitoring.CommentModeIncremental, DisplayName: "团队账号",
		}},
	})
	if len(response.Targets) != 1 {
		t.Fatalf("expected one target, got %#v", response.Targets)
	}
	target := response.Targets[0]
	if target.TargetKind != monitoring.TargetKindAccount || target.AccountID != "account-1" || target.CommentMode != monitoring.CommentModeIncremental {
		t.Fatalf("account target contract was lost: %#v", target)
	}
}
