package operator

import (
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func BuildImportResult(summary ingest.Summary) map[string]any {
	return map[string]any{
		"hubUserId":          summary.HubUserID,
		"state":              summary.State,
		"freshnessState":     summary.FreshnessState,
		"totalEvents":        summary.TotalEvents,
		"latestBatchId":      summary.LatestBatchID,
		"lastEventType":      summary.LastEventType,
		"aggregatedVideos":   summary.AggregatedVideos,
		"aggregatedComments": summary.AggregatedComments,
		"aggregatedTopics":   summary.AggregatedTopics,
	}
}

func BuildReadModelResult(payload insight.Dashboard) map[string]any {
	viewStatuses := make([]map[string]any, 0, len(payload.ReadModel.ViewStatuses))
	for _, item := range payload.ReadModel.ViewStatuses {
		viewStatuses = append(viewStatuses, map[string]any{
			"key":            item.Key,
			"label":          item.Label,
			"freshnessState": item.FreshnessState,
			"summary":        item.Summary,
			"sourceNote":     item.SourceNote,
			"latestBatchId":  item.LatestBatchID,
			"materializedAt": item.MaterializedAt,
		})
	}
	return map[string]any{
		"mode":           payload.ReadModel.Mode,
		"materializedAt": payload.ReadModel.MaterializedAt,
		"latestBatchId":  payload.ReadModel.LatestBatchID,
		"workspaceId":    payload.WorkspaceBridge.WorkspaceID,
		"hubUserId":      payload.WorkspaceBridge.HubUserID,
		"contextSource":  payload.WorkspaceBridge.ContextSource,
		"freshnessState": payload.IngestSummary.FreshnessState,
		"viewStatuses":   viewStatuses,
	}
}

func BuildSyncSavedBindingsLogResult(result Result) map[string]any {
	imports := make([]map[string]any, 0, len(result.Imports))
	for _, item := range result.Imports {
		importResult := BuildImportResult(item.Summary)
		importResult["source"] = item.Source
		imports = append(imports, importResult)
	}

	payload := map[string]any{
		"status":           result.Status,
		"finalStatus":      result.FinalStatus,
		"attemptedSources": result.AttemptedSources,
		"succeededSources": len(result.Imports),
		"failedSources":    len(result.Failures),
		"skippedSources":   len(result.Skipped),
		"startedAt":        result.StartedAt.UTC().Format(time.RFC3339Nano),
		"finishedAt":       result.FinishedAt.UTC().Format(time.RFC3339Nano),
		"imports":          imports,
		"readModel":        BuildReadModelResult(result.ReadModel),
	}
	if result.Message != "" {
		payload["message"] = result.Message
	}
	if len(result.Failures) > 0 {
		payload["failures"] = result.Failures
	}
	if len(result.Skipped) > 0 {
		payload["skipped"] = result.Skipped
	}
	return payload
}

func BuildCycleStatus(result Result) string {
	if result.FinalStatus != "" {
		return result.FinalStatus
	}
	if result.Status == "failed" {
		return "failed"
	}
	if result.Status == "partial" {
		return "partial"
	}
	return "ok"
}

func BuildBrief(payload insight.Dashboard, status runtime.Status) map[string]any {
	result := map[string]any{
		"workspace": map[string]any{
			"workspaceId": payload.WorkspaceBridge.WorkspaceID,
			"hubUserId":   payload.WorkspaceBridge.HubUserID,
			"name":        payload.Workspace.Name,
			"deviceCount": payload.Workspace.DeviceCount,
		},
		"readModel": BuildReadModelResult(payload),
		"runtime": map[string]any{
			"phase":                   status.Phase,
			"runtimeProfile":          status.RuntimeProfile,
			"dashboardStoreMode":      status.DashboardStoreMode,
			"rankingStoreMode":        status.RankingStoreMode,
			"commentInsightStoreMode": status.CommentInsightStoreMode,
			"reportSummaryStoreMode":  status.ReportSummaryStoreMode,
			"workspaceStoreMode":      status.WorkspaceStoreMode,
			"ingestStoreMode":         status.IngestStoreMode,
			"postgresConfigured":      status.PostgresConfigured,
			"postgresReady":           status.PostgresReady,
			"redisConfigured":         status.RedisConfigured,
			"redisReady":              status.RedisReady,
			"readCacheConfigured":     status.ReadCacheConfigured,
			"readCacheActive":         status.ReadCacheActive,
			"cache": map[string]any{
				"hitCount":        status.ReadCacheHitCount,
				"missCount":       status.ReadCacheMissCount,
				"fallbackCount":   status.ReadCacheFallbackCount,
				"writeCount":      status.ReadCacheWriteCount,
				"invalidateCount": status.ReadCacheInvalidateCount,
			},
		},
		"ingest": map[string]any{
			"state":              payload.IngestSummary.State,
			"freshnessState":     payload.IngestSummary.FreshnessState,
			"latestBatchId":      payload.IngestSummary.LatestBatchID,
			"lastSuccessAt":      payload.IngestSummary.LastSuccessAt,
			"lastFailureAt":      payload.IngestSummary.LastFailureAt,
			"lastFailureMessage": payload.IngestSummary.LastFailureMessage,
			"sourceCount":        payload.IngestSummary.SourceCount,
			"totalEvents":        payload.IngestSummary.TotalEvents,
		},
		"overview": map[string]any{
			"monitoredAccounts": payload.Overview.MonitoredAccounts,
			"trackedVideos":     payload.Overview.TrackedVideos,
			"emergingTopics":    payload.Overview.EmergingTopics,
			"newCommentBursts":  payload.Overview.NewCommentBursts,
			"dailyCoverageRate": payload.Overview.DailyCoverageRate,
			"reportFreshness":   payload.Overview.ReportFreshness,
		},
		"signals": map[string]any{
			"report": map[string]any{
				"period":            payload.ReportSummary.Period,
				"headline":          payload.ReportSummary.Headline,
				"keyFinding":        payload.ReportSummary.KeyFinding,
				"recommendedAction": payload.ReportSummary.RecommendedAction,
				"watchout":          payload.ReportSummary.Watchout,
				"autoGeneratedAt":   payload.ReportSummary.AutoGeneratedAt,
			},
		},
	}
	if len(payload.HotVideos) > 0 {
		resultSignals := result["signals"].(map[string]any)
		resultSignals["topVideo"] = map[string]any{
			"id":              payload.HotVideos[0].ID,
			"title":           payload.HotVideos[0].Title,
			"channelName":     payload.HotVideos[0].ChannelName,
			"comments":        payload.HotVideos[0].Comments,
			"growthRate":      payload.HotVideos[0].GrowthRate,
			"signal":          payload.HotVideos[0].Signal,
			"engagementScore": payload.HotVideos[0].EngagementScore,
		}
	}
	if len(payload.HotTopics) > 0 {
		resultSignals := result["signals"].(map[string]any)
		resultSignals["topTopic"] = map[string]any{
			"id":            payload.HotTopics[0].ID,
			"keyword":       payload.HotTopics[0].Keyword,
			"cluster":       payload.HotTopics[0].Cluster,
			"mentions":      payload.HotTopics[0].Mentions,
			"growthRate":    payload.HotTopics[0].GrowthRate,
			"relatedVideos": payload.HotTopics[0].RelatedVideos,
			"promptAngle":   payload.HotTopics[0].PromptAngle,
		}
	}
	if len(payload.CommentInsights) > 0 {
		resultSignals := result["signals"].(map[string]any)
		resultSignals["topCommentInsight"] = map[string]any{
			"id":               payload.CommentInsights[0].ID,
			"theme":            payload.CommentInsights[0].Theme,
			"sentiment":        payload.CommentInsights[0].Sentiment,
			"questionRate":     payload.CommentInsights[0].QuestionRate,
			"representative":   payload.CommentInsights[0].Representative,
			"actionSuggestion": payload.CommentInsights[0].ActionSuggestion,
		}
	}
	return result
}

func WithRuntimeCacheSnapshot(status runtime.Status, readCache interface {
	Snapshot() insight.ReadCacheSnapshot
}) runtime.Status {
	if readCache == nil {
		return status
	}
	return runtime.WithReadCacheSnapshot(status, readCache.Snapshot())
}

func BuildRunPayload(result Result, status runtime.Status) map[string]any {
	payload := map[string]any{
		"status": BuildCycleStatus(result),
		"sync":   BuildSyncSavedBindingsLogResult(result),
		"brief":  BuildBrief(result.ReadModel, status),
	}
	if result.Message != "" && BuildCycleStatus(result) == "failed" {
		payload["error"] = result.Message
	}
	return payload
}

func EnsureFinishedAt(result *Result, now func() time.Time) {
	if result != nil && result.FinishedAt.IsZero() {
		result.FinishedAt = now()
	}
}
