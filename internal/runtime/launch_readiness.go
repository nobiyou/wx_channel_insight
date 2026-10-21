package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

var requiredPrimaryAutoSources = []string{
	"wx_comment_export",
}

var primaryVideoAutoSources = []string{
	"wx_video_search",
	"wx_feed_list",
}

var compatibleVideoAutoSources = []string{
	"wx_feed_list",
}

type launchSourceEvidence struct {
	source     string
	evidence   string
	observedAt time.Time
}

const runtimeIdentitySetupAction = "在当前配置文件中启用 bridge.local_operator，或配置 bridge.trusted_token 和可信 Web 代理；关闭 bridge.demo_context_enabled 并重启 API/Web 后重新验证身份边界。"

func WithLaunchReadiness(status Status, dashboard insight.Dashboard) Status {
	return WithLaunchReadinessMonitoring(status, dashboard, monitoring.AutomationHealth{})
}

func WithLaunchReadinessMonitoring(status Status, dashboard insight.Dashboard, health monitoring.AutomationHealth) Status {
	readiness := buildLaunchReadiness(status, dashboard, health)
	status.LaunchReadiness = &readiness
	return status
}

func buildLaunchReadiness(status Status, dashboard insight.Dashboard, monitoringHealth monitoring.AutomationHealth) LaunchReadiness {
	ingest := dashboard.IngestSummary
	readModel := dashboard.ReadModel
	bridge := dashboard.WorkspaceBridge
	nonPostgresStores := collectNonPostgresStoreModes(status)
	workspaceReady := bridge.State == "ready"
	workspaceMappingGaps := collectWorkspaceMappingGaps(bridge, status)
	workspaceMappingStable := len(workspaceMappingGaps) == 0
	trustedProofState := deriveLaunchTrustedProofState(status)
	trustedProofBadge := launchTrustedProofBadge(trustedProofState, status)
	allPostgres := len(nonPostgresStores) == 0
	cacheCoversCoreViews := status.ReadCacheActive && hasRequiredReadCacheScopeCoverage(status)
	realAutoSources := collectPrimaryAutoSources(ingest)
	coreViewKeys := []string{"videos", "topics", "comments", "reports"}
	formalReadModelGaps := collectFormalReadModelGaps(readModel, coreViewKeys)
	hasFormalReadModels := len(formalReadModelGaps) == 0
	freshnessVisible := hasVisibilityFreshnessEvidence(ingest, readModel)
	dutyVisible := hasVisibilityDutyEvidence(ingest, status, monitoringHealth)
	failureSurfaceVisible := hasVisibilityFailureSurface(ingest, status, monitoringHealth)
	visibilityReady := freshnessVisible && dutyVisible && failureSurfaceVisible
	ownerBoundaryGaps := collectOwnerBoundaryGaps(status)
	ownerBoundaryReady := len(ownerBoundaryGaps) == 0
	ownerBoundaryState := status.OwnerBoundaryState
	ownerBoundaryAction := status.OwnerBoundaryAction
	if ownerBoundaryAction == "" {
		ownerBoundaryAction = "补齐 owner-boundary runtime 证据，确认 analytics 查询没有越界到 Hub SQLite。"
	}
	automationProofReady := hasCurrentAutomationProof(status) && !monitoringInvalidatesAutomationProof(monitoringHealth)
	automationProofEvidence := summarizeAutomationProofWithMonitoring(status, monitoringHealth)
	cacheActivityProofReady := hasRequiredReadCacheScopeActivityProof(status)
	cacheActivityGaps := collectMissingRequiredReadCacheActivityScopes(status)
	cacheActivityEvidence := summarizeReadCacheActivity(status)

	gates := []LaunchGate{
		buildTrustedBridgeLaunchGate(status, trustedProofState, trustedProofBadge),
		buildWorkspaceMappingLaunchGate(status, bridge, workspaceReady, workspaceMappingStable, workspaceMappingGaps, trustedProofState),
		buildPostgresOwnerLaunchGate(status, allPostgres, nonPostgresStores),
		buildRedisCacheLaunchGate(status, cacheCoversCoreViews, cacheActivityProofReady, cacheActivityEvidence, cacheActivityGaps),
		buildRealSourcesLaunchGate(status, ingest, realAutoSources, automationProofReady, automationProofEvidence),
		buildFormalReadModelsLaunchGate(readModel, coreViewKeys, hasFormalReadModels, formalReadModelGaps),
		buildVisibilityLaunchGate(ingest, readModel, status, monitoringHealth, visibilityReady, freshnessVisible, dutyVisible),
		buildOwnerBoundaryLaunchGate(status, ownerBoundaryReady, ownerBoundaryState, ownerBoundaryGaps, ownerBoundaryAction),
	}

	readiness := LaunchReadiness{
		Gates:      gates,
		TotalCount: len(gates),
	}
	for _, gate := range gates {
		switch gate.Status {
		case "met":
			readiness.MetCount++
		case "blocked":
			readiness.BlockedCount++
		case "unknown":
			readiness.UnknownCount++
		}
	}
	readiness.Tone = pickLaunchTone(readiness.BlockedCount, readiness.UnknownCount)
	readiness.Headline = buildLaunchHeadline(readiness.BlockedCount, readiness.UnknownCount)
	readiness.Summary = buildLaunchSummary(readiness.BlockedCount, readiness.UnknownCount)
	return readiness
}

func buildTrustedBridgeLaunchGate(status Status, trustedProofState, trustedProofBadge string) LaunchGate {
	if isTrustedOnlyLiveRuntime(status) {
		evidence := fmt.Sprintf("Bridge=%s · 证据 %s", status.HubBridgeMode, trustedProofBadge)
		if trustedProofState == "partial" {
			gaps := collectLaunchTrustedProofGaps(status.TrustedRuntimeEvidence)
			if len(gaps) > 0 {
				evidence = fmt.Sprintf("Bridge=%s · 最近证据缺口=%s", status.HubBridgeMode, strings.Join(gaps, " / "))
			}
		}
		if trustedProofState == "verified" {
			return LaunchGate{
				Key:      "trusted-bridge",
				Label:    "真实 Hub 用户身份可进入 Insight",
				Status:   "met",
				Evidence: evidence,
				Action:   "继续把 trusted bridge 证据和 freshness 一起作为值班判断前提。",
			}
		}
		return LaunchGate{
			Key:      "trusted-bridge",
			Label:    "真实 Hub 用户身份可进入 Insight",
			Status:   "unknown",
			Evidence: evidence,
			Action:   "补跑 trusted runtime 验证，让 API / Web 两侧证据落进当前运行态。",
		}
	}

	return LaunchGate{
		Key:      "trusted-bridge",
		Label:    "真实 Hub 用户身份可进入 Insight",
		Status:   "blocked",
		Evidence: fmt.Sprintf("当前 Bridge=%s，demoContext=%s", status.HubBridgeMode, onOffLabel(status.DemoContextEnabled)),
		Action:   runtimeIdentitySetupAction,
	}
}

func buildWorkspaceMappingLaunchGate(status Status, bridge insight.WorkspaceBridge, workspaceReady, workspaceMappingStable bool, workspaceMappingGaps []string, trustedProofState string) LaunchGate {
	if workspaceReady {
		if workspaceMappingStable {
			if isTrustedOnlyLiveRuntime(status) && trustedProofState == "verified" {
				return LaunchGate{
					Key:      "workspace-mapping",
					Label:    "默认工作区映射基于真实用户稳定建立",
					Status:   "met",
					Evidence: summarizeWorkspaceMappingEvidence(bridge),
					Action:   "继续用当前默认工作区映射承接 operator 和日报读模型。",
				}
			}
			return LaunchGate{
				Key:      "workspace-mapping",
				Label:    "默认工作区映射基于真实用户稳定建立",
				Status:   "unknown",
				Evidence: summarizeWorkspaceMappingEvidence(bridge),
				Action:   "默认工作区映射结构已齐，但还需要把当前 live 身份入口补成 trusted-only 已验证。",
			}
		}
		action := "补齐 workspace bridge 的真实映射字段，并确认当前响应来自 trusted_header 的真实 Hub 用户。"
		if !isTrustedOnlyLiveRuntime(status) {
			action = runtimeIdentitySetupAction
		}
		return LaunchGate{
			Key:      "workspace-mapping",
			Label:    "默认工作区映射基于真实用户稳定建立",
			Status:   "blocked",
			Evidence: fmt.Sprintf("%s · 缺口=%s", summarizeWorkspaceMappingEvidence(bridge), strings.Join(workspaceMappingGaps, " / ")),
			Action:   action,
		}
	}

	action := "先让 workspace bridge 进入 ready，并确认真实 Hub 用户能稳定复用默认工作区。"
	if !isTrustedOnlyLiveRuntime(status) {
		action = runtimeIdentitySetupAction
	}
	return LaunchGate{
		Key:      "workspace-mapping",
		Label:    "默认工作区映射基于真实用户稳定建立",
		Status:   "blocked",
		Evidence: fmt.Sprintf("workspaceBridge=%s", fallbackText(bridge.State, "missing")),
		Action:   action,
	}
}

func buildPostgresOwnerLaunchGate(status Status, allPostgres bool, nonPostgresStores []string) LaunchGate {
	if allPostgres {
		if status.PostgresReady {
			return LaunchGate{
				Key:      "postgres-owner",
				Label:    "Postgres 成为正式分析 owner",
				Status:   "met",
				Evidence: "Dashboard / Rankings / Comment Insights / Report Summary / Workspace / Ingest 都已切到 postgres，且当前 live 已拿到连通证据",
				Action:   "继续保持当前正式 owner 边界。",
			}
		}
		if status.PostgresConfigured {
			return LaunchGate{
				Key:      "postgres-owner",
				Label:    "Postgres 成为正式分析 owner",
				Status:   "unknown",
				Evidence: "store mode 已是 postgres，但当前 live 还缺 postgresReady 证据",
				Action:   "补充当前运行态的 postgres 连通验证。",
			}
		}
		return LaunchGate{
			Key:      "postgres-owner",
			Label:    "Postgres 成为正式分析 owner",
			Status:   "blocked",
			Evidence: "store mode 已是 postgres，但当前 live 还没有 Postgres 配置",
			Action:   "在当前配置文件中设置 storage.postgres_dsn，启动 Postgres 并重启 API 后验证连通性。",
		}
	}

	evidence := strings.Join(nonPostgresStores, " · ")
	if isTeamPreviewRuntime(status) && !status.PostgresConfigured {
		action := "在当前配置文件中设置 storage.driver=postgres 和 storage.postgres_dsn，重启 API 后验证连接。"
		return LaunchGate{
			Key:      "postgres-owner",
			Label:    "Postgres 成为正式分析 owner",
			Status:   "blocked",
			Evidence: fmt.Sprintf("team-preview 仍未接上 Postgres：%s", evidence),
			Action:   action,
		}
	}
	action := "团队部署需要 PostgreSQL 时，在配置文件中设置 storage.driver=postgres 和 storage.postgres_dsn，并检查 stores 下的独立覆盖项。"
	return LaunchGate{
		Key:      "postgres-owner",
		Label:    "Postgres 成为正式分析 owner",
		Status:   "blocked",
		Evidence: evidence,
		Action:   action,
	}
}

func buildRedisCacheLaunchGate(status Status, cacheCoversCoreViews, cacheActivityProofReady bool, cacheActivityEvidence string, cacheActivityGaps []string) LaunchGate {
	if cacheCoversCoreViews {
		evidence := fmt.Sprintf("scope=%s · %s", strings.Join(status.ReadCacheScope, ", "), cacheActivityEvidence)
		if cacheActivityProofReady {
			return LaunchGate{
				Key:      "redis-cache",
				Label:    "Redis 缓存覆盖 dashboard / hot rankings / reports",
				Status:   "met",
				Evidence: evidence,
				Action:   "继续观察真实命中与回退计数，确认缓存稳定承载热点读请求。",
			}
		}
		if len(cacheActivityGaps) > 0 {
			evidence = fmt.Sprintf("%s · 缺口=%s", evidence, strings.Join(cacheActivityGaps, " / "))
		}
		return LaunchGate{
			Key:      "redis-cache",
			Label:    "Redis 缓存覆盖 dashboard / hot rankings / reports",
			Status:   "unknown",
			Evidence: evidence,
			Action:   "先对 dashboard / hot rankings / reports 做真实访问，让这些 scope 的命中、未命中或写回证据进入当前 live runtime。",
		}
	}

	switch {
	case !status.RedisConfigured:
		evidence := "Redis 未配置"
		action := "在当前配置文件中设置 redis.addr，启动 Redis 并重启 API，再验证核心视图的缓存活动。"
		if isTeamPreviewRuntime(status) {
			evidence = "team-preview 仍未接上 Redis"
		}
		return LaunchGate{
			Key:      "redis-cache",
			Label:    "Redis 缓存覆盖 dashboard / hot rankings / reports",
			Status:   "blocked",
			Evidence: evidence,
			Action:   action,
		}
	case !status.RedisReady:
		return LaunchGate{
			Key:      "redis-cache",
			Label:    "Redis 缓存覆盖 dashboard / hot rankings / reports",
			Status:   "blocked",
			Evidence: "Redis 已配置，但当前 live 还缺 redisReady 证据",
			Action:   "先让当前 API / jobs 进程真正连上 Redis，再继续确认 read cache 是否已进入 live 读路径。",
		}
	default:
		return LaunchGate{
			Key:      "redis-cache",
			Label:    "Redis 缓存覆盖 dashboard / hot rankings / reports",
			Status:   "unknown",
			Evidence: fmt.Sprintf("readCacheActive=%t · scope=%s", status.ReadCacheActive, fallbackText(strings.Join(status.ReadCacheScope, ", "), "--")),
			Action:   "把 read cache 真正接到 dashboard / hot rankings / reports，并确认 scope 覆盖核心视图。",
		}
	}
}

func buildRealSourcesLaunchGate(status Status, ingest insight.IngestSummary, realAutoSources []launchSourceEvidence, automationProofReady bool, automationProofEvidence string) LaunchGate {
	if status.WXChannelCapabilitiesChecked && !status.WXChannelCapabilitiesReady {
		missing := joinRequirementParts(status.WXChannelMissingCapabilities)
		if routeEvidence := summarizeWXChannelCapabilityRoutes(status.WXChannelCapabilityRoutes); routeEvidence != "" && routeEvidence != "未记录" {
			missing = missing + " · " + routeEvidence
		}
		return LaunchGate{
			Key:      "real-sources",
			Label:    "至少两类真实 source 可自动 ingest",
			Status:   "blocked",
			Evidence: "当前 wx_channel:2026 缺少 Insight 必需数据面接口：" + missing,
			Action:   "运行 wx_channel/scripts/switch-insight-data-plane-admin.cmd，把 2026 切到包含 feed search、feed list、comment export 的版本；如果源码仍缺接口，就在 wx_channel 补齐后再重跑 operator。",
		}
	}
	if hasPrimaryAutoSourceSlots(realAutoSources) {
		parts := make([]string, 0, len(realAutoSources))
		for _, item := range realAutoSources {
			parts = append(parts, formatLaunchSourceEvidence(item))
		}
		return LaunchGate{
			Key:      "real-sources",
			Label:    "至少两类真实 source 可自动 ingest",
			Status:   ternaryStatus(automationProofReady, "met", "unknown"),
			Evidence: fmt.Sprintf("%s · %s", strings.Join(parts, " · "), automationProofEvidence),
			Action:   ternaryString(automationProofReady, "继续让 operator loop 复用这两类来源，避免再退回手工单次导入。", "补一轮成功的 operator-loop 自动 ingest 证据，并确认最近一次 jobs 至少请求了 2 条真实来源。"),
		}
	}
	evidence := "当前没有两类 primary source 的 runnable + 成功事实"
	if len(realAutoSources) > 0 {
		parts := make([]string, 0, len(realAutoSources))
		for _, item := range realAutoSources {
			parts = append(parts, formatLaunchSourceEvidence(item))
		}
		evidence = strings.Join(parts, " · ")
	}
	if gapEvidence := summarizePrimarySourceGapEvidence(ingest); gapEvidence != "" {
		evidence = evidence + " · " + gapEvidence
	}
	if automationProofEvidence != "" {
		evidence = evidence + " · " + automationProofEvidence
	}
	action := "至少补齐 comment export + 一条视频来源的保存、成功和可重跑事实。"
	if hasAllRunnablePrimarySources(ingest) {
		action = "补一轮成功的 operator-loop 自动 ingest 证据，并确认最近一次 jobs 至少请求了 2 条真实来源。"
	} else if gapAction := summarizePrimarySourceGapAction(ingest); gapAction != "" {
		action = gapAction
	}
	return LaunchGate{
		Key:      "real-sources",
		Label:    "至少两类真实 source 可自动 ingest",
		Status:   "blocked",
		Evidence: evidence,
		Action:   action,
	}
}

func hasAllRunnablePrimarySources(summary insight.IngestSummary) bool {
	if len(summary.BindingStates) > 0 {
		runnableComment := false
		runnableVideo := false
		for _, item := range summary.BindingStates {
			if !item.Runnable {
				continue
			}
			if strings.TrimSpace(item.Source) == "wx_comment_export" {
				runnableComment = true
			}
			if containsTextSlice(primaryVideoAutoSources, item.Source) {
				runnableVideo = true
			}
		}
		return runnableComment && runnableVideo
	}

	runnableComment := false
	runnableVideo := false
	for _, item := range summary.OperatorReadiness.Runnable {
		if !item.Runnable {
			continue
		}
		if strings.TrimSpace(item.Source) == "wx_comment_export" {
			runnableComment = true
		}
		if containsTextSlice(primaryVideoAutoSources, item.Source) {
			runnableVideo = true
		}
	}
	return runnableComment && runnableVideo
}

func hasPrimaryAutoSourceSlots(items []launchSourceEvidence) bool {
	hasComment := false
	hasVideo := false
	for _, item := range items {
		source := strings.TrimSpace(item.source)
		if source == "wx_comment_export" {
			hasComment = true
		}
		if containsTextSlice(primaryVideoAutoSources, source) {
			hasVideo = true
		}
	}
	return hasComment && hasVideo
}

func formatLaunchSourceEvidence(item launchSourceEvidence) string {
	source := strings.TrimSpace(item.source)
	label := source
	if containsTextSlice(compatibleVideoAutoSources, source) {
		label = source + "(兼容视频来源)"
	}
	return fmt.Sprintf("%s=%s", label, fallbackText(item.evidence, "--"))
}

func summarizePrimarySourceGapEvidence(ingest insight.IngestSummary) string {
	parts := []string{}
	seen := map[string]struct{}{}
	appendPart := func(source string, part string) {
		source = strings.TrimSpace(source)
		part = strings.TrimSpace(part)
		if source == "" || part == "" {
			return
		}
		if _, found := seen[source]; found {
			return
		}
		seen[source] = struct{}{}
		parts = append(parts, part)
	}

	for _, source := range requiredPrimaryAutoSources {
		appendPart(source, primarySourceGapEvidenceForSource(ingest, source))
	}
	if !hasAnyVideoOperatorReadinessSource(ingest) {
		appendPart("video_source", "video_source=missing:还没有保存可自动运行的视频来源（wx_video_search 或 wx_feed_list）")
	}
	for _, item := range ingest.OperatorReadiness.Blocked {
		if containsTextSlice(primaryVideoAutoSources, item.Source) {
			appendPart(item.Source, fmt.Sprintf("%s=blocked:%s", item.Source, fallbackText(firstNonEmptyText(item.RetryReason, item.Summary), "--")))
		}
	}
	for _, item := range ingest.OperatorReadiness.Missing {
		if containsTextSlice(primaryVideoAutoSources, item.Source) {
			appendPart(item.Source, fmt.Sprintf("%s=missing:%s", item.Source, fallbackText(firstNonEmptyText(item.Summary, item.Action), "--")))
		}
	}
	return strings.Join(parts, " · ")
}

func summarizePrimarySourceGapAction(ingest insight.IngestSummary) string {
	actions := []string{}
	seen := map[string]struct{}{}
	appendAction := func(action string) {
		action = strings.TrimSpace(action)
		if action == "" {
			return
		}
		if _, found := seen[action]; found {
			return
		}
		seen[action] = struct{}{}
		actions = append(actions, action)
	}

	for _, source := range requiredPrimaryAutoSources {
		appendAction(primarySourceGapActionForSource(ingest, source))
	}
	if !hasAnyVideoOperatorReadinessSource(ingest) {
		appendAction("先保存或导入一次关键词视频搜索，或保留已有 wx_feed_list 绑定，确保系统至少有一条可自动运行的视频来源。")
	}
	for _, item := range ingest.OperatorReadiness.Blocked {
		if containsTextSlice(primaryVideoAutoSources, item.Source) {
			appendAction(firstNonEmptyText(buildPrimarySourceGapAction(item), item.Action))
		}
	}
	for _, item := range ingest.OperatorReadiness.Missing {
		if containsTextSlice(primaryVideoAutoSources, item.Source) {
			appendAction(firstNonEmptyText(buildPrimarySourceGapAction(item), item.Action))
		}
	}
	if len(actions) == 0 {
		return strings.TrimSpace(ingest.OperatorReadiness.NextAction)
	}
	return strings.Join(actions, "；")
}

func primarySourceGapEvidenceForSource(ingest insight.IngestSummary, source string) string {
	if hasExactAutoSuccessForSource(ingest, source) {
		return ""
	}
	if item, ok := findOperatorReadinessSource(ingest.OperatorReadiness.Blocked, source); ok {
		return fmt.Sprintf("%s=blocked:%s", source, fallbackText(firstNonEmptyText(item.RetryReason, item.Summary), "--"))
	}
	if item, ok := findOperatorReadinessSource(ingest.OperatorReadiness.Missing, source); ok {
		return fmt.Sprintf("%s=missing:%s", source, fallbackText(firstNonEmptyText(item.Summary, item.Action), "--"))
	}
	if !hasExactOperatorReadinessSource(ingest, source) {
		return fmt.Sprintf("%s=missing:%s", source, primaryMissingSourceSummary(source, ingest))
	}
	return ""
}

func primarySourceGapActionForSource(ingest insight.IngestSummary, source string) string {
	if hasExactAutoSuccessForSource(ingest, source) {
		return ""
	}
	if item, ok := findOperatorReadinessSource(ingest.OperatorReadiness.Blocked, source); ok {
		return firstNonEmptyText(buildPrimarySourceGapAction(item), item.Action)
	}
	if item, ok := findOperatorReadinessSource(ingest.OperatorReadiness.Missing, source); ok {
		return firstNonEmptyText(buildPrimarySourceGapAction(item), item.Action)
	}
	if source == "wx_video_search" && !hasExactOperatorReadinessSource(ingest, source) {
		if item, ok := findAnyOperatorReadinessSource(ingest, "wx_feed_list"); ok && isWXChannelClientGap(item) {
			return buildWXVideoSearchClientRecoveryAction(item.BaseURL) + "；再保存或导入一次关键词视频搜索，让 operator 默认使用 wx_video_search。"
		}
		return "先保存或导入一次关键词视频搜索，让系统记住 base URL 和关键词；旧 wx_feed_list 只作为已有绑定的兼容视频来源。"
	}
	if source == "wx_comment_export" && !hasExactOperatorReadinessSource(ingest, source) {
		return "先导入一份评论导出 JSON，让系统保存评论来源参数。"
	}
	return ""
}

func primaryMissingSourceSummary(source string, ingest insight.IngestSummary) string {
	if source == "wx_video_search" && hasExactOperatorReadinessSource(ingest, "wx_feed_list") {
		return "默认 operator 需要关键词视频搜索，旧 wx_feed_list 只作为兼容视频来源"
	}
	switch source {
	case "wx_comment_export":
		return "还没有保存评论导出来源，今天这轮会缺少评论洞察输入"
	case "wx_video_search":
		return "还没有保存关键词视频搜索来源，今天这轮会缺少按关键词发现的候选热视频"
	default:
		return "当前来源还没有保存，今天这轮无法覆盖这部分输入"
	}
}

func findAnyOperatorReadinessSource(ingest insight.IngestSummary, source string) (insight.OperatorReadinessSource, bool) {
	if item, ok := findOperatorReadinessSource(ingest.OperatorReadiness.Blocked, source); ok {
		return item, true
	}
	if item, ok := findOperatorReadinessSource(ingest.OperatorReadiness.Missing, source); ok {
		return item, true
	}
	return findOperatorReadinessSource(ingest.OperatorReadiness.Runnable, source)
}

func findOperatorReadinessSource(items []insight.OperatorReadinessSource, source string) (insight.OperatorReadinessSource, bool) {
	needle := strings.TrimSpace(source)
	for _, item := range items {
		if strings.TrimSpace(item.Source) == needle {
			return item, true
		}
	}
	return insight.OperatorReadinessSource{}, false
}

func hasExactOperatorReadinessSource(ingest insight.IngestSummary, source string) bool {
	needle := strings.TrimSpace(source)
	for _, item := range ingest.BindingStates {
		if strings.TrimSpace(item.Source) == needle {
			return true
		}
	}
	if _, ok := findAnyOperatorReadinessSource(ingest, needle); ok {
		return true
	}
	return false
}

func hasAnyVideoOperatorReadinessSource(ingest insight.IngestSummary) bool {
	for _, source := range primaryVideoAutoSources {
		if hasExactOperatorReadinessSource(ingest, source) {
			return true
		}
	}
	return false
}

func hasExactAutoSuccessForSource(ingest insight.IngestSummary, source string) bool {
	needle := strings.TrimSpace(source)
	for _, item := range ingest.BindingStates {
		if strings.TrimSpace(item.Source) == needle && hasAutoSuccessEvidence(item.Source, item.Runnable, item.LastAutoRunStatus, item.LastAutoRunAt, item.LastAutoSuccessAt, item.LastAutoBatchID) {
			return true
		}
	}
	for _, item := range ingest.OperatorReadiness.Runnable {
		if strings.TrimSpace(item.Source) == needle && hasAutoSuccessEvidence(item.Source, item.Runnable, item.LastAutoRunStatus, item.LastAutoRunAt, item.LastAutoSuccessAt, item.LastAutoBatchID) {
			return true
		}
	}
	return false
}

func buildPrimarySourceGapAction(item insight.OperatorReadinessSource) string {
	if item.Source == "wx_feed_list" && isWXChannelClientGap(item) {
		return buildWXFeedListClientRecoveryAction(item.BaseURL)
	}
	if item.Source == "wx_video_search" && isWXChannelClientGap(item) {
		return buildWXVideoSearchClientRecoveryAction(item.BaseURL)
	}
	return ""
}

func isWXChannelClientGap(item insight.OperatorReadinessSource) bool {
	text := strings.ToLower(firstNonEmptyText(item.RetryReason, item.Summary, item.Action))
	return strings.Contains(text, "page client") ||
		strings.Contains(text, "页面客户端") ||
		strings.Contains(text, "ws/health is unreachable")
}

func buildWXFeedListClientRecoveryAction(baseURL string) string {
	statusURL := "http://127.0.0.1:2026/api/channels/status"
	if trimmed := strings.TrimSpace(baseURL); trimmed != "" {
		statusURL = strings.TrimRight(trimmed, "/") + "/api/channels/status"
	}
	return fmt.Sprintf("先确认视频号独立客户端仍在运行且请求经过 wx_channel 代理拦截，并检查 %s 至少出现 1 个 ready/feed_ready client 后，再重跑下一轮 operator。", statusURL)
}

func buildWXVideoSearchClientRecoveryAction(baseURL string) string {
	statusURL := "http://127.0.0.1:2026/api/channels/status"
	if trimmed := strings.TrimSpace(baseURL); trimmed != "" {
		statusURL = strings.TrimRight(trimmed, "/") + "/api/channels/status"
	}
	return fmt.Sprintf("先确认视频号独立客户端仍在运行且请求经过 wx_channel 代理拦截，并检查 %s 至少出现 1 个 ready/search_ready client 后，再重跑下一轮 operator。", statusURL)
}

func buildFormalReadModelsLaunchGate(readModel insight.ReadModelState, coreViewKeys []string, hasFormalReadModels bool, formalReadModelGaps []string) LaunchGate {
	if hasFormalReadModels {
		return LaunchGate{
			Key:      "formal-read-models",
			Label:    "5 个核心视图来自正式 read model",
			Status:   "met",
			Evidence: summarizeFormalReadModelEvidence(readModel, coreViewKeys),
			Action:   "继续让 Dashboard / Hot Videos / Hot Topics / Comment Insights / Reports 只吃正式 read model。",
		}
	}
	return LaunchGate{
		Key:      "formal-read-models",
		Label:    "5 个核心视图来自正式 read model",
		Status:   "blocked",
		Evidence: fmt.Sprintf("%s · 缺口=%s", summarizeFormalReadModelEvidence(readModel, coreViewKeys), strings.Join(formalReadModelGaps, " / ")),
		Action:   "先让 Dashboard 与四个核心视图都明确挂到 materialized read model。",
	}
}

func buildVisibilityLaunchGate(ingest insight.IngestSummary, readModel insight.ReadModelState, status Status, health monitoring.AutomationHealth, visibilityReady, freshnessVisible, dutyVisible bool) LaunchGate {
	if visibilityReady {
		return LaunchGate{
			Key:      "visibility",
			Label:    "页面可见 freshness、任务状态、失败提示",
			Status:   "met",
			Evidence: summarizeVisibilityEvidence(ingest, readModel, status, health),
			Action:   "继续把 runtime health 当成值班前置检查面板。",
		}
	}

	action := "把当前失败原因接到页面可见的 banner / issue 列表里，让值班同学能直接看到风险来源。"
	switch {
	case !freshnessVisible:
		action = "补齐最新批次 / 最近成功 / materializedAt 的页面证据，让 freshness 不只是一个状态字。"
	case !dutyVisible:
		action = "补齐 operator 最近完成时间或 jobs 完成时间，让任务状态带上可核对的时间锚点。"
	}
	return LaunchGate{
		Key:      "visibility",
		Label:    "页面可见 freshness、任务状态、失败提示",
		Status:   "unknown",
		Evidence: summarizeVisibilityEvidence(ingest, readModel, status, health),
		Action:   action,
	}
}

func buildOwnerBoundaryLaunchGate(status Status, ownerBoundaryReady bool, ownerBoundaryState string, ownerBoundaryGaps []string, ownerBoundaryAction string) LaunchGate {
	if ownerBoundaryReady {
		gateStatus := ownerBoundaryState
		if gateStatus == "" {
			gateStatus = "unknown"
		}
		return LaunchGate{
			Key:      "owner-boundary",
			Label:    "不依赖 Hub SQLite 做热榜 / 报告查询",
			Status:   gateStatus,
			Evidence: summarizeOwnerBoundaryEvidence(status),
			Action:   ownerBoundaryAction,
		}
	}
	return LaunchGate{
		Key:      "owner-boundary",
		Label:    "不依赖 Hub SQLite 做热榜 / 报告查询",
		Status:   "unknown",
		Evidence: fmt.Sprintf("%s · 缺口=%s", summarizeOwnerBoundaryEvidence(status), strings.Join(ownerBoundaryGaps, " / ")),
		Action:   ownerBoundaryAction,
	}
}

func pickLaunchTone(blockedCount, unknownCount int) string {
	if blockedCount > 0 {
		return "risk"
	}
	if unknownCount > 0 {
		return "warn"
	}
	return "good"
}

func buildLaunchHeadline(blockedCount, unknownCount int) string {
	switch {
	case blockedCount > 0:
		return fmt.Sprintf("团队运营版仍有 %d 条上线门槛未满足", blockedCount)
	case unknownCount > 0:
		return fmt.Sprintf("团队运营版还差 %d 条证据补齐后才能放行", unknownCount)
	default:
		return "团队运营版的 8 条上线门槛都已有当前证据"
	}
}

func buildLaunchSummary(blockedCount, unknownCount int) string {
	switch {
	case blockedCount > 0:
		return "先解决明确阻塞项，再谈值班团队把这套环境当正式 live runtime。"
	case unknownCount > 0:
		return "当前没有明显硬阻塞，但仍有门槛缺少足够证据，先补验证再放行。"
	default:
		return "当前运行态已经满足团队运营版上线门槛，可以继续把注意力放在日常值班和数据判断上。"
	}
}

func deriveLaunchTrustedProofState(status Status) string {
	baseState := status.TrustedRuntimeEvidence.State
	if baseState == "" {
		return "missing"
	}
	gaps := collectLaunchTrustedProofGaps(status.TrustedRuntimeEvidence)
	if baseState == "verified" && len(gaps) > 0 {
		return "partial"
	}
	if baseState == "verified" && !isTrustedOnlyLiveRuntime(status) {
		return "historical"
	}
	return baseState
}

func collectLaunchTrustedProofGaps(evidence TrustedRuntimeEvidence) []string {
	gaps := []string{}
	switch evidence.TrustedBoundaryMode {
	case "trusted-only", "local-operator", "hub-session":
		// accepted live boundary modes
	default:
		gaps = append(gaps, "trustedBoundaryMode 不是 trusted-only/local-operator/hub-session")
	}
	if evidence.APITrustedContextState != "verified" {
		gaps = append(gaps, "apiTrustedContextState 不是 verified")
	}
	if evidence.APIBrowserQueryState != "blocked" && evidence.APIBrowserQueryState != "isolated" {
		gaps = append(gaps, "apiBrowserQueryState 不是 blocked/isolated")
	}
	if evidence.WebTrustedContextState != "verified" {
		gaps = append(gaps, "webTrustedContextState 不是 verified")
	}
	if evidence.WebBrowserQueryState != "isolated" {
		gaps = append(gaps, "webBrowserQueryState 不是 isolated")
	}
	if evidence.VerifiedAt.IsZero() {
		gaps = append(gaps, "verifiedAt 缺失")
	}
	if evidence.Source == "" {
		gaps = append(gaps, "source 缺失")
	}
	if evidence.HubUserID == "" {
		gaps = append(gaps, "hubUserId 缺失")
	}
	return gaps
}

func launchTrustedProofBadge(state string, status Status) string {
	switch state {
	case "verified":
		return "API + Web 已验证"
	case "stale":
		return "旧 proof 不对应当前 live"
	case "historical":
		return "历史 proof，非当前 live"
	case "partial":
		if len(collectLaunchTrustedProofGaps(status.TrustedRuntimeEvidence)) > 0 {
			return "proof 结构不完整"
		}
		return "仅部分记录"
	default:
		return "未记录"
	}
}

func collectNonPostgresStoreModes(status Status) []string {
	parts := []string{}
	appendIfNotPostgres := func(label, mode string) {
		if mode != "" && mode != string(StoreModePostgres) {
			parts = append(parts, fmt.Sprintf("%s=%s", label, mode))
		}
	}
	appendIfNotPostgres("Dashboard", status.DashboardStoreMode)
	appendIfNotPostgres("Rankings", status.RankingStoreMode)
	appendIfNotPostgres("Comment Insights", status.CommentInsightStoreMode)
	appendIfNotPostgres("Report Summary", status.ReportSummaryStoreMode)
	appendIfNotPostgres("Workspace", status.WorkspaceStoreMode)
	appendIfNotPostgres("Ingest", status.IngestStoreMode)
	return parts
}

func hasRecoveredIngestFailure(summary insight.IngestSummary) bool {
	if summary.LastFailureMessage == "" || summary.State != "ready" {
		return false
	}
	if summary.LastFailureAt.IsZero() || summary.LastSuccessAt.IsZero() || summary.LastSuccessAt.Before(summary.LastFailureAt) {
		return false
	}
	for _, item := range summary.SourceStatuses {
		if item.State == "failing" || item.LastError != "" {
			return false
		}
	}
	for _, item := range summary.BindingStates {
		if !item.Runnable && item.RetryReason != "" {
			return false
		}
	}
	return true
}

func collectPrimaryAutoSources(summary insight.IngestSummary) []launchSourceEvidence {
	bySource := map[string]launchSourceEvidence{}
	collect := func(source string, runnable bool, lastAutoRunStatus string, lastAutoRunAt, lastAutoSuccessAt time.Time, lastAutoBatchID string) {
		if !hasAutoSuccessEvidence(source, runnable, lastAutoRunStatus, lastAutoRunAt, lastAutoSuccessAt, lastAutoBatchID) {
			return
		}
		observedAt := lastAutoRunAt
		if lastAutoSuccessAt.After(observedAt) {
			observedAt = lastAutoSuccessAt
		}
		candidate := launchSourceEvidence{
			source:     source,
			evidence:   firstLaunchEvidence(lastAutoBatchID, lastAutoRunAt, lastAutoSuccessAt),
			observedAt: observedAt,
		}
		current, exists := bySource[source]
		if !exists || candidate.observedAt.After(current.observedAt) {
			bySource[source] = candidate
		}
	}
	if len(summary.BindingStates) > 0 {
		for _, item := range summary.BindingStates {
			collect(item.Source, item.Runnable, item.LastAutoRunStatus, item.LastAutoRunAt, item.LastAutoSuccessAt, item.LastAutoBatchID)
		}
	} else {
		for _, item := range summary.OperatorReadiness.Runnable {
			collect(item.Source, item.Runnable, item.LastAutoRunStatus, item.LastAutoRunAt, item.LastAutoSuccessAt, item.LastAutoBatchID)
		}
	}
	items := make([]launchSourceEvidence, 0, len(bySource))
	for _, source := range append(append([]string{}, requiredPrimaryAutoSources...), primaryVideoAutoSources...) {
		if item, ok := bySource[source]; ok {
			items = append(items, item)
		}
	}
	return items
}

func isPrimaryAutoSourceCandidate(source string) bool {
	trimmed := strings.TrimSpace(source)
	return trimmed == "wx_comment_export" || containsTextSlice(primaryVideoAutoSources, trimmed)
}

func isCompatibleAutoSourceCandidate(source string) bool {
	return containsTextSlice(compatibleVideoAutoSources, strings.TrimSpace(source))
}

func hasAutoSuccessEvidence(source string, runnable bool, lastAutoRunStatus string, lastAutoRunAt, lastAutoSuccessAt time.Time, lastAutoBatchID string) bool {
	if !isPrimaryAutoSourceCandidate(source) || !runnable || lastAutoRunStatus != "success" {
		return false
	}
	if lastAutoRunAt.IsZero() && lastAutoSuccessAt.IsZero() {
		return false
	}
	return lastAutoBatchID != "" || !lastAutoSuccessAt.IsZero()
}

func firstLaunchEvidence(batchID string, times ...time.Time) string {
	if batchID != "" {
		return batchID
	}
	for _, item := range times {
		if !item.IsZero() {
			return item.Format(time.RFC3339)
		}
	}
	return "ready"
}

func summarizeFormalReadModelEvidence(readModel insight.ReadModelState, coreViewKeys []string) string {
	parts := []string{}
	if readModel.Mode != "" {
		parts = append(parts, "mode="+readModel.Mode)
	}
	if readModel.LatestBatchID != "" {
		parts = append(parts, "batch="+readModel.LatestBatchID)
	}
	if !readModel.MaterializedAt.IsZero() {
		parts = append(parts, "materializedAt="+readModel.MaterializedAt.Format(time.RFC3339))
	}
	if len(coreViewKeys) > 0 {
		parts = append(parts, "views="+strings.Join(coreViewKeys, ", "))
	}
	if len(parts) == 0 {
		return "readModel=missing"
	}
	return strings.Join(parts, " · ")
}

func collectFormalReadModelGaps(readModel insight.ReadModelState, coreViewKeys []string) []string {
	gaps := []string{}
	if readModel.Mode != "materialized" {
		gaps = append(gaps, "readModel.mode="+fallbackText(readModel.Mode, "--"))
	}
	if readModel.LatestBatchID == "" {
		gaps = append(gaps, "readModel.latestBatchId 缺失")
	}
	if readModel.MaterializedAt.IsZero() {
		gaps = append(gaps, "readModel.materializedAt 缺失")
	}
	for _, key := range coreViewKeys {
		view, ok := findReadModelView(readModel.ViewStatuses, key)
		if !ok {
			gaps = append(gaps, "view:"+key+" 缺失")
			continue
		}
		if view.LatestBatchID == "" || view.MaterializedAt.IsZero() {
			gaps = append(gaps, "view:"+key+" 缺批次或物化时间")
			continue
		}
		if readModel.LatestBatchID != "" && view.LatestBatchID != readModel.LatestBatchID {
			gaps = append(gaps, "view:"+key+" 批次不一致")
		}
	}
	return gaps
}

func findReadModelView(items []insight.ReadModelViewStatus, key string) (insight.ReadModelViewStatus, bool) {
	for _, item := range items {
		if item.Key == key {
			return item, true
		}
	}
	return insight.ReadModelViewStatus{}, false
}

func hasVisibilityFreshnessEvidence(ingest insight.IngestSummary, readModel insight.ReadModelState) bool {
	if ingest.FreshnessState == "" {
		return false
	}
	return ingest.LatestBatchID != "" || !ingest.LastSuccessAt.IsZero() || !readModel.MaterializedAt.IsZero()
}

func hasVisibilityDutyEvidence(ingest insight.IngestSummary, status Status, health monitoring.AutomationHealth) bool {
	if status.JobsLastStatus != "" && (!status.JobsLastFinishedAt.IsZero() || !ingest.LastSuccessAt.IsZero() || hasAnySourceLastRun(ingest)) {
		return true
	}
	return health.Checked && !health.LastRunAt.IsZero()
}

func hasAnySourceLastRun(ingest insight.IngestSummary) bool {
	for _, item := range ingest.SourceStatuses {
		if !item.LastRunAt.IsZero() {
			return true
		}
	}
	return false
}

func hasActiveVisibilityFailure(ingest insight.IngestSummary, status Status, health monitoring.AutomationHealth) bool {
	return (ingest.LastFailureMessage != "" && !hasRecoveredIngestFailure(ingest)) ||
		status.JobsLastStatus == "failed" || status.JobsLastStatus == "partial" ||
		monitoringInvalidatesAutomationProof(health)
}

func hasVisibilityFailureSurface(ingest insight.IngestSummary, status Status, health monitoring.AutomationHealth) bool {
	if !hasActiveVisibilityFailure(ingest, status, health) {
		return true
	}
	return ingest.LastFailureMessage != "" || status.JobsLastError != "" ||
		status.JobsLastStatus == "failed" || status.JobsLastStatus == "partial" ||
		health.Error != "" || health.LastError != "" || isMonitoringFailureStatus(health.LastStatus)
}

func summarizeVisibilityEvidence(ingest insight.IngestSummary, readModel insight.ReadModelState, status Status, health monitoring.AutomationHealth) string {
	freshnessMarker := firstNonEmptyText(
		ingest.LatestBatchID,
		formatTimeForLaunch(ingest.LastSuccessAt),
		formatTimeForLaunch(readModel.MaterializedAt),
		"--",
	)
	dutyMarker := firstNonEmptyText(
		formatTimeForLaunch(status.JobsLastFinishedAt),
		formatTimeForLaunch(health.LastRunAt),
		formatTimeForLaunch(ingest.LastSuccessAt),
		firstSourceLastRunAt(ingest),
		"--",
	)

	failureMarker := "无活跃失败"
	if hasActiveVisibilityFailure(ingest, status, health) {
		switch {
		case health.Error != "":
			failureMarker = "monitoring.healthError"
		case health.LastError != "" || isMonitoringFailureStatus(health.LastStatus):
			failureMarker = "monitoring.lastError"
		case ingest.LastFailureMessage != "":
			failureMarker = "ingest.lastFailureMessage"
		case status.JobsLastError != "":
			failureMarker = "jobsLastError"
		case status.JobsLastStatus == "failed" || status.JobsLastStatus == "partial":
			failureMarker = "jobsLastStatus"
		default:
			failureMarker = "--"
		}
	}

	monitoringMarker := "未检查"
	if health.Checked {
		monitoringMarker = fmt.Sprintf("%s@%s", fallbackText(health.LastStatus, "idle"), firstNonEmptyText(formatTimeForLaunch(health.LastRunAt), "--"))
	}
	return fmt.Sprintf("freshness=%s@%s · jobs=%s@%s · monitoring=%s · 失败提示=%s",
		fallbackText(ingest.FreshnessState, "--"),
		freshnessMarker,
		fallbackText(status.JobsLastStatus, "--"),
		dutyMarker,
		monitoringMarker,
		failureMarker,
	)
}

func firstSourceLastRunAt(ingest insight.IngestSummary) string {
	for _, item := range ingest.SourceStatuses {
		if !item.LastRunAt.IsZero() {
			return item.LastRunAt.Format(time.RFC3339)
		}
	}
	return ""
}

func collectWorkspaceMappingGaps(bridge insight.WorkspaceBridge, status Status) []string {
	gaps := []string{}
	if bridge.State != "ready" {
		gaps = append(gaps, "workspaceBridge.state="+fallbackText(bridge.State, "--"))
	}
	if bridge.WorkspaceID == "" {
		gaps = append(gaps, "workspaceId 缺失")
	}
	if bridge.HubUserID == "" {
		gaps = append(gaps, "hubUserId 缺失")
	} else {
		if bridge.HubUserID == "hub:user:demo" {
			gaps = append(gaps, "hubUserId 仍是 demo")
		}
		expectedWorkspaceID := expectedDefaultWorkspaceID(bridge.HubUserID)
		if bridge.WorkspaceID != "" && expectedWorkspaceID != "" && bridge.WorkspaceID != expectedWorkspaceID {
			gaps = append(gaps, fmt.Sprintf("workspaceId=%s，期望 %s", bridge.WorkspaceID, expectedWorkspaceID))
		}
	}
	if !isAcceptedLiveContextSource(bridge.ContextSource, status) {
		gaps = append(gaps, "contextSource="+fallbackText(bridge.ContextSource, "--"))
	}
	if bridge.BootstrapMode != "lazy_create" {
		gaps = append(gaps, "bootstrapMode="+fallbackText(bridge.BootstrapMode, "--"))
	}
	if bridge.Source != "insight_default_workspace" {
		gaps = append(gaps, "source="+fallbackText(bridge.Source, "--"))
	}
	if !hasStableWorkspaceResolution(bridge.Resolution) {
		gaps = append(gaps, "resolution="+fallbackText(bridge.Resolution, "--"))
	}
	if bridge.LastSyncedAt.IsZero() {
		gaps = append(gaps, "lastSyncedAt 缺失")
	}
	trustedProofState := deriveLaunchTrustedProofState(status)
	if trustedProofState == "verified" && status.TrustedRuntimeEvidence.HubUserID != "" && bridge.HubUserID != "" && status.TrustedRuntimeEvidence.HubUserID != bridge.HubUserID {
		gaps = append(gaps, "trustedProof.hubUserId="+status.TrustedRuntimeEvidence.HubUserID)
	}
	return gaps
}

func isAcceptedLiveContextSource(source string, status Status) bool {
	switch strings.TrimSpace(source) {
	case "trusted_header", "hub_session":
		return true
	case "local_operator":
		return status.HubBridgeMode == "local-operator" ||
			status.HubBridgeMode == "local-operator+trusted-header" ||
			strings.HasPrefix(status.HubBridgeMode, "local-operator")
	default:
		return false
	}
}

func hasStableWorkspaceResolution(value string) bool {
	return value == "created" || value == "reused" || value == "updated"
}

func summarizeWorkspaceMappingEvidence(bridge insight.WorkspaceBridge) string {
	return strings.Join([]string{
		"workspace=" + fallbackText(bridge.WorkspaceID, "--"),
		"expected=" + fallbackText(expectedDefaultWorkspaceID(bridge.HubUserID), "--"),
		"hub=" + fallbackText(bridge.HubUserID, "--"),
		"context=" + fallbackText(bridge.ContextSource, "--"),
		"bootstrap=" + fallbackText(bridge.BootstrapMode, "--"),
		"source=" + fallbackText(bridge.Source, "--"),
		"resolution=" + fallbackText(bridge.Resolution, "--"),
		"syncedAt=" + fallbackText(formatTimeForLaunch(bridge.LastSyncedAt), "--"),
	}, " · ")
}

func collectOwnerBoundaryGaps(status Status) []string {
	gaps := []string{}
	if status.OwnerBoundaryState == "" {
		gaps = append(gaps, "ownerBoundaryState 缺失")
	}
	if status.OwnerBoundaryEvidence == "" {
		gaps = append(gaps, "ownerBoundaryEvidence 缺失")
	}
	if status.OwnerBoundarySummary == "" {
		gaps = append(gaps, "ownerBoundarySummary 缺失")
	}
	if status.OwnerBoundaryAction == "" {
		gaps = append(gaps, "ownerBoundaryAction 缺失")
	}
	return gaps
}

func summarizeOwnerBoundaryEvidence(status Status) string {
	if status.OwnerBoundaryEvidence != "" {
		return status.OwnerBoundaryEvidence
	}
	if status.OwnerBoundarySummary != "" {
		return status.OwnerBoundarySummary
	}
	return "ownerBoundary=missing"
}

func hasCurrentAutomationProof(status Status) bool {
	if status.JobsLastMode != "operator-loop" || status.JobsLastStatus != "ok" {
		return false
	}
	if !containsTextSlice(status.JobsLastRequestedSources, "wx_comment_export") {
		return false
	}
	for _, source := range primaryVideoAutoSources {
		if containsTextSlice(status.JobsLastRequestedSources, source) {
			return true
		}
	}
	return false
}

func summarizeAutomationProof(status Status) string {
	return fmt.Sprintf("jobs=%s/%s · requested=%d · sources=%s",
		fallbackText(status.JobsLastMode, "--"),
		fallbackText(status.JobsLastStatus, "--"),
		status.JobsLastRequestedCount,
		fallbackText(strings.Join(status.JobsLastRequestedSources, ", "), "--"),
	)
}

func summarizeAutomationProofWithMonitoring(status Status, health monitoring.AutomationHealth) string {
	jobsEvidence := summarizeAutomationProof(status)
	if !health.Checked {
		return jobsEvidence
	}
	return jobsEvidence + " · " + summarizeMonitoringAutomationHealth(health)
}

func summarizeMonitoringAutomationHealth(health monitoring.AutomationHealth) string {
	if health.Error != "" {
		return "monitoring=unavailable:" + strings.TrimSpace(health.Error)
	}
	if health.EnabledTaskCount == 0 {
		return "monitoring=no-enabled-tasks"
	}
	parts := []string{"monitoring=" + fallbackText(strings.TrimSpace(health.LastStatus), "idle")}
	if task := firstNonEmptyText(strings.TrimSpace(health.TaskName), strings.TrimSpace(health.TaskID)); task != "" {
		parts = append(parts, "task="+task)
	}
	if !health.LastRunAt.IsZero() {
		parts = append(parts, "lastRunAt="+health.LastRunAt.Format(time.RFC3339))
	}
	if health.LastError != "" {
		parts = append(parts, "error="+strings.TrimSpace(health.LastError))
	}
	return strings.Join(parts, " · ")
}

func monitoringInvalidatesAutomationProof(health monitoring.AutomationHealth) bool {
	if !health.Checked {
		return false
	}
	return health.Error != "" || health.UnhealthyTaskCount > 0 || isMonitoringFailureStatus(health.LastStatus)
}

func isMonitoringFailureStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case monitoring.RunStatusFailed, monitoring.RunStatusPartial, monitoring.RunStatusAbandoned:
		return true
	default:
		return false
	}
}

func summarizeReadCacheActivity(status Status) string {
	scoped := []string{}
	for _, scope := range requiredReadCacheScopes {
		item, ok := status.ReadCacheScopeStats[scope]
		if ok && hasReadCacheScopeActivity(item) {
			scoped = append(scoped, fmt.Sprintf("%s=hit:%d miss:%d fallback:%d write:%d", scope, item.HitCount, item.MissCount, item.FallbackCount, item.WriteCount))
			continue
		}
		scoped = append(scoped, scope+"=--")
	}
	return fmt.Sprintf("cache=hit:%d miss:%d fallback:%d write:%d · scoped=%s",
		status.ReadCacheHitCount,
		status.ReadCacheMissCount,
		status.ReadCacheFallbackCount,
		status.ReadCacheWriteCount,
		fallbackText(strings.Join(scoped, " · "), "--"),
	)
}

func expectedDefaultWorkspaceID(hubUserID string) string {
	if hubUserID == "" {
		return ""
	}
	var builder strings.Builder
	for _, char := range hubUserID {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char + ('a' - 'A'))
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		default:
			builder.WriteRune('-')
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		result = "workspace"
	}
	return "ws-default-" + result
}

func formatTimeForLaunch(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func fallbackText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmptyText(values ...string) string {
	for _, item := range values {
		if item != "" {
			return item
		}
	}
	return ""
}

func containsTextSlice(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func onOffLabel(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func ternaryString(condition bool, whenTrue, whenFalse string) string {
	if condition {
		return whenTrue
	}
	return whenFalse
}

func ternaryStatus(condition bool, whenTrue, whenFalse string) string {
	return ternaryString(condition, whenTrue, whenFalse)
}
