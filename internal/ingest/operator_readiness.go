package ingest

import (
	"fmt"
	"strings"
)

var operatorExpectedSources = []string{
	RunSourceWXCommentExport,
	RunSourceWXVideoSearch,
}

func buildOperatorReadiness(bindingStates []SourceBindingState) OperatorReadiness {
	runnable := make([]OperatorReadinessSource, 0, len(bindingStates))
	blocked := make([]OperatorReadinessSource, 0, len(bindingStates))
	seen := map[string]struct{}{}

	for _, binding := range bindingStates {
		source := canonicalSource(binding.Source)
		if source == "" {
			continue
		}
		seen[source] = struct{}{}

		item := OperatorReadinessSource{
			Source:            source,
			TargetKey:         binding.TargetKey,
			Label:             operatorSourceLabel(source),
			Path:              binding.Path,
			BaseURL:           binding.BaseURL,
			ShareURL:          binding.ShareURL,
			ObjectID:          binding.ObjectID,
			ObjectNonceID:     binding.ObjectNonceID,
			Username:          binding.Username,
			Keyword:           binding.Keyword,
			DeviceCount:       binding.DeviceCount,
			UpdatedAt:         binding.UpdatedAt,
			LastRunStatus:     binding.LastRunStatus,
			LastSuccessAt:     binding.LastSuccessAt,
			LastFailureAt:     binding.LastFailureAt,
			LatestBatchID:     binding.LatestBatchID,
			LastAutoRunStatus: binding.LastAutoRunStatus,
			LastAutoRunAt:     binding.LastAutoRunAt,
			LastAutoSuccessAt: binding.LastAutoSuccessAt,
			LastAutoBatchID:   binding.LastAutoBatchID,
			Runnable:          binding.Runnable,
			CanRetry:          binding.CanRetry,
			RetryReason:       binding.RetryReason,
			Diagnostic:        cloneSourceBindingDiagnostic(binding.Diagnostic),
		}

		if binding.Runnable {
			item.State = "runnable"
			item.Summary = operatorRunnableSummary(binding)
			item.Action = operatorRunnableAction(binding)
			runnable = append(runnable, item)
			continue
		}

		item.State = "blocked"
		item.Summary = firstNonEmpty(binding.RetryReason, "当前保存的来源参数还不能直接跑")
		item.Action = operatorBlockedAction(binding)
		blocked = append(blocked, item)
	}
	if _, ok := seen[RunSourceWXFeedList]; ok {
		seen[RunSourceWXVideoSearch] = struct{}{}
	}

	missing := make([]OperatorReadinessSource, 0, len(operatorExpectedSources))
	for _, source := range operatorExpectedSources {
		if _, ok := seen[source]; ok {
			continue
		}
		missing = append(missing, OperatorReadinessSource{
			Source:   source,
			Label:    operatorSourceLabel(source),
			State:    "missing",
			Summary:  operatorMissingSummary(source),
			Action:   operatorMissingAction(source),
			Runnable: false,
		})
	}

	state := "blocked"
	expectedStatus := "failed"
	headline := "今天这轮 operator 还不能直接跑"
	summary := "至少有一个关键来源还没准备好，先把缺失或失效的来源补齐。"
	nextAction := "先去 Settings 补齐缺失来源，或修复已经失效的 saved binding。"

	switch {
	case len(runnable) == 0 && len(blocked) == 0 && len(missing) == 0:
		state = "idle"
		expectedStatus = "failed"
		headline = "当前还没有可运行的 operator 来源"
		summary = "还没保存任何来源参数，operator 还没有今天这轮可执行的输入。"
		nextAction = "先在 Settings 导入评论导出或视频列表，让系统保存第一条来源参数。"
	case len(blocked) == 0 && len(missing) == 0 && len(runnable) > 0:
		state = "ready"
		expectedStatus = "ok"
		headline = fmt.Sprintf("今天这轮 operator 可以直接跑，%d 条来源都已就绪", len(runnable))
		summary = "当前保存的来源参数都能直接复用，跑完就能得到最新 dashboard 和团队日报判断。"
		nextAction = "直接运行今天这轮 operator，然后检查新批次是否已经进入 materialized read model。"
	case len(runnable) > 0:
		state = "partial"
		expectedStatus = "partial"
		headline = fmt.Sprintf("今天这轮 operator 只能部分完成，可跑 %d 条，仍有 %d 条待处理", len(runnable), len(blocked)+len(missing))
		summary = "已经有一部分来源可以直接跑，但仍会出现跳过或缺口，首页判断要带着覆盖范围一起看。"
		nextAction = "先决定是否允许部分完成；如果要完整日报，先修复 blocked / missing 来源。"
	}

	return OperatorReadiness{
		State:           state,
		ExpectedStatus:  expectedStatus,
		RunnableSources: len(runnable),
		BlockedSources:  len(blocked),
		MissingSources:  len(missing),
		Headline:        headline,
		Summary:         summary,
		NextAction:      nextAction,
		Runnable:        runnable,
		Blocked:         blocked,
		Missing:         missing,
	}
}

func operatorSourceLabel(source string) string {
	switch canonicalSource(source) {
	case RunSourceWXCommentExport:
		return "评论导出"
	case RunSourceWXFeedList:
		return "视频列表"
	case RunSourceWXVideoSearch:
		return "视频搜索"
	case RunSourceWXManualShare:
		return "手动视频"
	default:
		if strings.TrimSpace(source) == "" {
			return "未知来源"
		}
		return source
	}
}

func operatorRunnableSummary(binding SourceBindingState) string {
	switch canonicalSource(binding.Source) {
	case RunSourceWXCommentExport:
		if binding.Path != "" {
			return "评论导出路径或目录可直接复用，当前这条来源可以直接重跑。"
		}
		return "评论导出来源已就绪，可以直接跑今天这轮。"
	case RunSourceWXFeedList:
		if binding.Username != "" {
			return fmt.Sprintf("视频列表参数已保存，当前将复用 %s。", binding.Username)
		}
		return "视频列表来源已就绪，可以直接跑今天这轮。"
	case RunSourceWXVideoSearch:
		if binding.Keyword != "" {
			return fmt.Sprintf("关键词视频搜索已保存，当前将复用 %s。", binding.Keyword)
		}
		return "视频搜索来源已就绪，可以直接跑今天这轮。"
	case RunSourceWXManualShare:
		if binding.ObjectID != "" {
			return fmt.Sprintf("手动视频 %s 的分享链接已保存，当前会继续采集互动数据。", binding.ObjectID)
		}
		return "手动视频分享链接已保存，可以继续采集互动数据。"
	default:
		return "当前来源参数已经就绪，可以直接复用。"
	}
}

func operatorRunnableAction(binding SourceBindingState) string {
	if binding.LastRunStatus == "failed" && binding.CanRetry {
		return "最近一次失败，但参数仍可复用，可以直接重试今天这轮。"
	}
	return "直接跑今天这轮 operator 即可。"
}

func operatorBlockedAction(binding SourceBindingState) string {
	switch canonicalSource(binding.Source) {
	case RunSourceWXCommentExport:
		if isWXCommentExportProofPlaceholderPath(binding.Path) {
			return "当前保存的是 team runtime proof 占位评论导出。请先在 wx_channel 中对真实目标视频重新导出评论 JSON，再用新的文件重新绑定这条来源。"
		}
		if binding.Diagnostic != nil && strings.TrimSpace(binding.Diagnostic.Code) == wxCommentExportContextMismatchCode {
			return "先在视频号独立客户端中切到这条评论对应的视频，等待当前视频信息完全加载后再重试；如果这是旧绑定，请重新导出评论或重新绑定当前视频。"
		}
		if binding.Diagnostic != nil && strings.TrimSpace(binding.Diagnostic.Code) == wxCommentExportProxyMismatchCode {
			return "先确认视频号独立客户端已通过 wx_channel 代理连上 API，再回到目标视频重试评论导出。"
		}
		if action := buildWXChannelStatusRecoveryAction(binding.Diagnostic); action != "" {
			return action
		}
		return "先确认目标视频已经在视频号独立客户端里切到位且接口已就绪；如果只是路径失效，再回填新的评论导出 JSON 文件路径。"
	case RunSourceWXFeedList:
		if action := buildWXChannelStatusRecoveryAction(binding.Diagnostic); action != "" {
			return action
		}
		return "先确认视频号独立客户端已通过 wx_channel 代理连上 API，再回填 base URL 和 username。"
	case RunSourceWXVideoSearch:
		if action := buildWXChannelStatusRecoveryAction(binding.Diagnostic); action != "" {
			return action
		}
		return "先确认视频号独立客户端已通过 wx_channel 代理连上 API，再回填 base URL 和视频搜索关键词。"
	case RunSourceWXManualShare:
		return "请重新粘贴有效的视频号分享链接，让系统重新识别这条视频。"
	default:
		return "先修复来源参数，再重新导入。"
	}
}

func buildWXChannelStatusRecoveryAction(diagnostic *SourceBindingDiagnostic) string {
	if diagnostic == nil || strings.TrimSpace(diagnostic.WXChannelStatusError) == "" {
		return ""
	}
	statusURL := strings.TrimSpace(diagnostic.StatusURL)
	if statusURL == "" && strings.TrimSpace(diagnostic.ResolvedBaseURL) != "" {
		statusURL = strings.TrimRight(strings.TrimSpace(diagnostic.ResolvedBaseURL), "/") + "/api/channels/status"
	}
	if statusURL == "" {
		statusURL = "http://127.0.0.1:2026/api/channels/status"
	}
	return "先确认 wx_channel API 本身可响应 " + statusURL + "（当前检查失败：" + strings.TrimSpace(diagnostic.WXChannelStatusError) + "）。如果 2025/2026 端口已监听但请求超时，先重启 wx_channel，再打开视频号独立客户端进入目标页面后重跑 operator。"
}

func operatorMissingSummary(source string) string {
	switch canonicalSource(source) {
	case RunSourceWXCommentExport:
		return "还没有保存评论导出来源，今天这轮会缺少评论洞察输入。"
	case RunSourceWXFeedList:
		return "还没有保存视频列表或视频搜索来源，今天这轮会缺少热视频和热话题输入。"
	case RunSourceWXVideoSearch:
		return "还没有保存视频搜索来源，今天这轮会缺少按关键词发现的候选热视频。"
	default:
		return "当前来源还没有保存，今天这轮无法覆盖这部分输入。"
	}
}

func operatorMissingAction(source string) string {
	switch canonicalSource(source) {
	case RunSourceWXCommentExport:
		return "先导入一份评论导出 JSON，让系统保存评论来源参数。"
	case RunSourceWXFeedList:
		return "先保存或导入一次视频列表，或保存关键词视频搜索，让系统记住视频输入来源。"
	case RunSourceWXVideoSearch:
		return "先保存或导入一次关键词视频搜索，让系统记住 base URL 和关键词。"
	default:
		return "先导入这个来源，再开始跑 operator。"
	}
}
