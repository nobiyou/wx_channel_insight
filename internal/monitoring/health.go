package monitoring

import (
	"context"
	"strings"
	"time"
)

// AutomationHealth is the bounded monitoring projection consumed by launch readiness.
type AutomationHealth struct {
	Checked            bool
	EnabledTaskCount   int
	UnhealthyTaskCount int
	TaskID             string
	TaskName           string
	LastStatus         string
	LastError          string
	LastRunAt          time.Time
	LastFailureAt      time.Time
	Error              string
}

type automationHealthRepository interface {
	GetAutomationHealth(context.Context, string) (AutomationHealth, error)
}

func (s *Service) GetAutomationHealth(ctx context.Context, hubUserID string) (AutomationHealth, error) {
	hubUserID = s.resolveHubUserID(hubUserID)
	if repo, ok := s.repo.(automationHealthRepository); ok {
		health, err := repo.GetAutomationHealth(ctx, hubUserID)
		health.Checked = true
		return health, err
	}
	tasks, err := s.repo.ListTasks(ctx, hubUserID, false)
	if err != nil {
		return AutomationHealth{Checked: true}, err
	}
	return summarizeAutomationHealth(tasks), nil
}

func summarizeAutomationHealth(tasks []Task) AutomationHealth {
	health := AutomationHealth{Checked: true}
	var candidate Task
	for _, task := range tasks {
		if task.Status != StatusEnabled {
			continue
		}
		health.EnabledTaskCount++
		unhealthy := isUnhealthyAutomationStatus(task.LastStatus)
		if unhealthy {
			health.UnhealthyTaskCount++
		}
		if candidate.ID == "" || preferAutomationHealthCandidate(task, candidate) {
			candidate = task
		}
	}
	if candidate.ID == "" {
		return health
	}
	health.TaskID = strings.TrimSpace(candidate.ID)
	health.TaskName = strings.TrimSpace(candidate.Name)
	health.LastStatus = strings.TrimSpace(candidate.LastStatus)
	health.LastError = strings.TrimSpace(candidate.LastError)
	health.LastRunAt = candidate.LastRunAt
	health.LastFailureAt = candidate.LastFailureAt
	return health
}

func preferAutomationHealthCandidate(left, right Task) bool {
	leftUnhealthy := isUnhealthyAutomationStatus(left.LastStatus)
	rightUnhealthy := isUnhealthyAutomationStatus(right.LastStatus)
	if leftUnhealthy != rightUnhealthy {
		return leftUnhealthy
	}
	leftAt := automationHealthTimestamp(left, leftUnhealthy)
	rightAt := automationHealthTimestamp(right, rightUnhealthy)
	if leftAt.Equal(rightAt) {
		return left.ID < right.ID
	}
	return leftAt.After(rightAt)
}

func automationHealthTimestamp(task Task, unhealthy bool) time.Time {
	if unhealthy && !task.LastFailureAt.IsZero() {
		return task.LastFailureAt
	}
	if !task.LastRunAt.IsZero() {
		return task.LastRunAt
	}
	return task.UpdatedAt
}

func isUnhealthyAutomationStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case RunStatusFailed, RunStatusPartial, RunStatusAbandoned:
		return true
	default:
		return false
	}
}
