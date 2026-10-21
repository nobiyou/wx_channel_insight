package operator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

func (s *Service) syncSeparatedAccountTargets(
	ctx context.Context,
	options Options,
	query ingest.SummaryQuery,
	targets []monitoring.TaskTarget,
	startedAt time.Time,
	failResult func(string) Result,
) (Result, error) {
	if len(targets) == 0 {
		err := fmt.Errorf("%s task requires at least one account target", options.TaskType)
		return failResult(err.Error()), err
	}
	if s.accountMonitor == nil {
		err := fmt.Errorf("account monitor is not configured")
		return failResult(err.Error()), err
	}

	imports := make([]ImportedSummary, 0, len(targets))
	failures := make([]Failure, 0)
	skipped := make([]Skipped, 0)
	for _, target := range targets {
		result, runErr := s.accountMonitor.RunTask(ctx, query, target, options.TaskType)
		targetKey := target.AccountID
		if runErr != nil {
			failures = append(failures, Failure{Source: monitoring.TargetKindAccount, TargetKey: targetKey, Error: runErr.Error()})
			continue
		}
		if strings.TrimSpace(result.Summary.HubUserID) != "" {
			imports = append(imports, ImportedSummary{Source: separatedTaskSource(options.TaskType), Summary: result.Summary})
		}
		for _, item := range result.Errors {
			message := strings.TrimSpace(item.Message)
			if message == "" {
				message = "账号任务未提供错误详情"
			}
			if item.VideoObjectID != "" {
				message = fmt.Sprintf("视频[%s]: %s", item.VideoObjectID, message)
			}
			if item.Status == monitoring.RunStatusSkipped {
				skipped = append(skipped, Skipped{Source: separatedTaskSource(options.TaskType), TargetKey: targetKey, Reason: message})
			} else {
				failures = append(failures, Failure{Source: separatedTaskSource(options.TaskType), TargetKey: targetKey, Error: message})
			}
		}
	}

	if len(imports) == 0 {
		if len(failures) == 0 && len(skipped) > 0 {
			result := failResult("账号任务已跳过：" + buildSyncSavedBindingsFailure(failures, skipped).Error())
			result.Status = monitoring.RunStatusSkipped
			result.FinalStatus = monitoring.RunStatusSkipped
			result.AttemptedSources = len(targets)
			result.Failures = failures
			result.Skipped = skipped
			return result, nil
		}
		err := buildSyncSavedBindingsFailure(failures, skipped)
		result := failResult(err.Error())
		result.AttemptedSources = len(targets)
		result.Failures = failures
		result.Skipped = skipped
		return result, err
	}

	readModel, err := s.readModelService.RefreshDashboard(ctx, insight.DashboardQuery{
		HubUserID: query.HubUserID, DeviceCount: query.DeviceCount, ContextSource: "jobs",
	}, "jobs")
	if err != nil {
		result := failResult(err.Error())
		result.AttemptedSources = len(targets)
		result.Imports = imports
		result.Failures = failures
		result.Skipped = skipped
		return result, err
	}
	status := monitoring.RunStatusSuccess
	if len(failures) > 0 || len(skipped) > 0 {
		status = monitoring.RunStatusPartial
	}
	finalStatus := status
	if options.Strict && status != monitoring.RunStatusSuccess {
		finalStatus = monitoring.RunStatusFailed
	}
	result := Result{
		Status: status, FinalStatus: finalStatus, AttemptedSources: len(targets),
		Imports: imports, Failures: failures, Skipped: skipped, ReadModel: readModel,
		StartedAt: startedAt, FinishedAt: s.now(),
	}
	if options.Strict && status != monitoring.RunStatusSuccess {
		result.Message = buildStrictCycleFailure(status, failures, skipped)
		return result, ExecutionError{Err: fmt.Errorf("%s", result.Message)}
	}
	return result, nil
}

func separatedTaskSource(taskType string) string {
	switch strings.ToLower(strings.TrimSpace(taskType)) {
	case monitoring.TaskTypeAccountProfile:
		return "account_profile"
	case monitoring.TaskTypeVideoList:
		return "video_list"
	case monitoring.TaskTypeCommentCollect:
		return "comment_collection"
	default:
		return monitoring.TargetKindAccount
	}
}
