package operator

import (
	"context"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

// RunMonitoringTask adapts the operator execution owner to the monitoring
// service without making monitoring depend on operator implementation types.
func (s *Service) RunMonitoringTask(ctx context.Context, options monitoring.OperatorRunOptions) (monitoring.OperatorRunResult, error) {
	result, err := s.RunCycle(ctx, Options{
		HubUserID:      options.HubUserID,
		DeviceCount:    options.DeviceCount,
		TaskType:       options.TaskType,
		TargetRefs:     append([]monitoring.BindingRef(nil), options.TargetRefs...),
		AccountTargets: append([]monitoring.TaskTarget(nil), options.AccountTargets...),
		TriggerMode:    strings.TrimSpace(options.TriggerMode),
	})
	mapped := monitoring.OperatorRunResult{
		Status:        result.Status,
		FinalStatus:   result.FinalStatus,
		Attempted:     result.AttemptedSources,
		Succeeded:     len(result.Imports),
		Failed:        len(result.Failures),
		Skipped:       len(result.Skipped),
		OperatorRunID: result.OperatorRunID,
		StartedAt:     result.StartedAt,
		FinishedAt:    result.FinishedAt,
		Message:       result.Message,
	}
	for _, failure := range result.Failures {
		mapped.Errors = append(mapped.Errors, monitoring.TaskRunError{
			Source: failure.Source, TargetKey: failure.TargetKey, Status: monitoring.RunStatusFailed, Message: failure.Error,
		})
	}
	for _, skipped := range result.Skipped {
		mapped.Errors = append(mapped.Errors, monitoring.TaskRunError{
			Source: skipped.Source, TargetKey: skipped.TargetKey, Status: monitoring.RunStatusSkipped, Message: skipped.Reason,
		})
	}
	return mapped, err
}
