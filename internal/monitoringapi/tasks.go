package monitoringapi

import (
	"errors"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

type TaskRequest struct {
	Name      string                  `json:"name"`
	TaskType  string                  `json:"taskType"`
	Status    string                  `json:"status"`
	ScopeMode string                  `json:"scopeMode"`
	Interval  string                  `json:"interval"`
	Targets   []monitoring.TaskTarget `json:"targets"`
}

func (request TaskRequest) Apply(task monitoring.Task) (monitoring.Task, error) {
	if strings.TrimSpace(request.Name) != "" {
		task.Name = strings.TrimSpace(request.Name)
	}
	if strings.TrimSpace(request.TaskType) != "" {
		task.TaskType = strings.TrimSpace(request.TaskType)
	}
	if strings.TrimSpace(request.Status) != "" {
		task.Status = strings.TrimSpace(request.Status)
	}
	if strings.TrimSpace(request.ScopeMode) != "" {
		task.ScopeMode = strings.TrimSpace(request.ScopeMode)
	}
	if strings.TrimSpace(request.Interval) != "" {
		interval, err := time.ParseDuration(strings.TrimSpace(request.Interval))
		if err != nil {
			return monitoring.Task{}, errors.New("monitoring task interval must be a duration such as 5m")
		}
		task.Interval = interval
	}
	if request.Targets != nil {
		task.Targets = append([]monitoring.TaskTarget(nil), request.Targets...)
	}
	return task, nil
}

type TasksResponse struct {
	Items []TaskResponse `json:"items"`
}

type TaskResponse struct {
	ID            string           `json:"id"`
	HubUserID     string           `json:"hubUserId"`
	Name          string           `json:"name"`
	TaskType      string           `json:"taskType"`
	Status        string           `json:"status"`
	ScopeMode     string           `json:"scopeMode"`
	Interval      string           `json:"interval"`
	NextRunAt     string           `json:"nextRunAt"`
	LastRunAt     string           `json:"lastRunAt"`
	LastSuccessAt string           `json:"lastSuccessAt"`
	LastFailureAt string           `json:"lastFailureAt"`
	LastStatus    string           `json:"lastStatus"`
	LastError     string           `json:"lastError"`
	System        bool             `json:"system"`
	CreatedAt     string           `json:"createdAt"`
	UpdatedAt     string           `json:"updatedAt"`
	ArchivedAt    string           `json:"archivedAt"`
	Targets       []TargetResponse `json:"targets"`
}

type TargetResponse struct {
	Source      string `json:"source"`
	TargetKey   string `json:"targetKey"`
	TargetKind  string `json:"targetKind,omitempty"`
	AccountID   string `json:"accountId,omitempty"`
	CommentMode string `json:"commentMode,omitempty"`
	Label       string `json:"label"`
	DisplayName string `json:"displayName"`
}

type TaskRunResponse struct {
	ID            string             `json:"id"`
	TaskID        string             `json:"taskId"`
	HubUserID     string             `json:"hubUserId"`
	Trigger       string             `json:"trigger"`
	Status        string             `json:"status"`
	Attempt       int                `json:"attempt"`
	StartedAt     string             `json:"startedAt"`
	FinishedAt    string             `json:"finishedAt"`
	LeaseUntil    string             `json:"leaseUntil"`
	OperatorRunID string             `json:"operatorRunId"`
	Counts        CountsResponse     `json:"counts"`
	Message       string             `json:"message"`
	Errors        []RunErrorResponse `json:"errors"`
}

type CountsResponse struct {
	Attempted int `json:"attempted"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

type RunErrorResponse struct {
	Source    string `json:"source"`
	TargetKey string `json:"targetKey"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

func NewTasksResponse(items []monitoring.Task) TasksResponse {
	result := TasksResponse{Items: make([]TaskResponse, 0, len(items))}
	for _, item := range items {
		result.Items = append(result.Items, NewTaskResponse(item))
	}
	return result
}

func NewTaskResponse(item monitoring.Task) TaskResponse {
	result := TaskResponse{
		ID:            item.ID,
		HubUserID:     item.HubUserID,
		Name:          item.Name,
		TaskType:      item.TaskType,
		Status:        item.Status,
		ScopeMode:     item.ScopeMode,
		Interval:      item.Interval.String(),
		NextRunAt:     formatTime(item.NextRunAt),
		LastRunAt:     formatTime(item.LastRunAt),
		LastSuccessAt: formatTime(item.LastSuccessAt),
		LastFailureAt: formatTime(item.LastFailureAt),
		LastStatus:    item.LastStatus,
		LastError:     item.LastError,
		System:        item.System,
		CreatedAt:     formatTime(item.CreatedAt),
		UpdatedAt:     formatTime(item.UpdatedAt),
		ArchivedAt:    formatTime(item.ArchivedAt),
		Targets:       make([]TargetResponse, 0, len(item.Targets)),
	}
	for _, target := range item.Targets {
		result.Targets = append(result.Targets, TargetResponse{
			Source: target.Source, TargetKey: target.TargetKey,
			TargetKind: target.TargetKind, AccountID: target.AccountID, CommentMode: target.CommentMode,
			Label: target.Label, DisplayName: target.DisplayName,
		})
	}
	return result
}

func NewTaskRunsResponse(items []monitoring.TaskRun) []TaskRunResponse {
	result := make([]TaskRunResponse, 0, len(items))
	for _, item := range items {
		result = append(result, NewTaskRunResponse(item))
	}
	return result
}

func NewTaskRunResponse(item monitoring.TaskRun) TaskRunResponse {
	result := TaskRunResponse{
		ID:            item.ID,
		TaskID:        item.TaskID,
		HubUserID:     item.HubUserID,
		Trigger:       item.Trigger,
		Status:        item.Status,
		Attempt:       item.Attempt,
		StartedAt:     formatTime(item.StartedAt),
		FinishedAt:    formatTime(item.FinishedAt),
		LeaseUntil:    formatTime(item.LeaseUntil),
		OperatorRunID: item.OperatorRunID,
		Counts: CountsResponse{
			Attempted: item.Counts.Attempted,
			Succeeded: item.Counts.Succeeded,
			Failed:    item.Counts.Failed,
			Skipped:   item.Counts.Skipped,
		},
		Message: item.Message,
		Errors:  make([]RunErrorResponse, 0, len(item.Errors)),
	}
	for _, item := range item.Errors {
		result.Errors = append(result.Errors, RunErrorResponse{
			Source: item.Source, TargetKey: item.TargetKey,
			Status: item.Status, Message: item.Message,
		})
	}
	return result
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}
