package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

var requiredReadCacheScopes = []string{
	"dashboard",
	"hot_videos",
	"hot_topics",
	"report_summary",
}

type Status struct {
	Phase                        string
	RuntimeProfile               string
	HubBridgeMode                string
	DemoContextEnabled           bool
	LaunchReadiness              *LaunchReadiness
	OwnerBoundaryState           string
	OwnerBoundaryEvidence        string
	OwnerBoundarySummary         string
	OwnerBoundaryAction          string
	LiveRuntimeClass             string
	LiveRuntimeProofState        string
	LiveRuntimeSummary           string
	LiveRuntimeAction            string
	LiveRuntimeRequirements      []LiveRuntimeRequirement
	TrustedRuntimeEvidence       TrustedRuntimeEvidence
	DashboardStoreMode           string
	RankingStoreMode             string
	CommentInsightStoreMode      string
	ReportSummaryStoreMode       string
	WorkspaceStoreMode           string
	IngestStoreMode              string
	PostgresConfigured           bool
	PostgresReady                bool
	RedisConfigured              bool
	RedisReady                   bool
	ReadCacheConfigured          bool
	ReadCacheActive              bool
	ReadCacheScope               []string
	ReadCacheHitCount            int64
	ReadCacheMissCount           int64
	ReadCacheFallbackCount       int64
	ReadCacheWriteCount          int64
	ReadCacheInvalidateCount     int64
	ReadCacheLastHitAt           time.Time
	ReadCacheLastMissAt          time.Time
	ReadCacheLastFallbackAt      time.Time
	ReadCacheLastWriteAt         time.Time
	ReadCacheLastInvalidateAt    time.Time
	ReadCacheScopeStats          map[string]insight.ReadCacheScopeSnapshot
	JobsEntrypointState          string
	JobsRuntimeState             string
	JobsLastMode                 string
	JobsLastStatus               string
	JobsLastHubUserID            string
	JobsLastStartedAt            time.Time
	JobsLastFinishedAt           time.Time
	JobsLastIteration            int
	JobsLastIterations           int
	JobsLastInterval             string
	JobsLastError                string
	JobsLastRequestedCount       int
	JobsLastRequestedSources     []string
	JobsLastUpdatedAt            time.Time
	WXChannelBaseURL             string
	WXChannelConnected           bool
	WXChannelReadyClients        int
	WXChannelFeedReady           int
	WXChannelCommentReady        int
	WXChannelCapabilitiesChecked bool
	WXChannelCapabilitiesReady   bool
	WXChannelMissingCapabilities []string
	WXChannelCapabilityRoutes    []WXChannelCapabilityRoute
	OperatorIdentity             string
	Notes                        []string
	// Multi-LAN edge gateway (Insight-hosted /ws/client)
	EdgeEnabled     bool `json:"edgeEnabled"`
	EdgeOnlineNodes int  `json:"edgeOnlineNodes"`
	EdgeReadyNodes  int  `json:"edgeReadyNodes"`
	EdgeFeedNodes   int  `json:"edgeFeedNodes"`
}

type LiveRuntimeRequirement struct {
	Label   string
	Key     string
	Status  string
	Summary string
	Action  string
}

type WXChannelCapabilityStatus struct {
	BaseURL string
	Checked bool
	Ready   bool
	Missing []string
	Routes  []WXChannelCapabilityRoute
}

type WXChannelCapabilityRoute struct {
	Name         string
	Method       string
	URL          string
	StatusCode   int
	RoutePresent bool
	Error        string
}

type LaunchReadiness struct {
	Gates        []LaunchGate
	MetCount     int
	BlockedCount int
	UnknownCount int
	TotalCount   int
	Tone         string
	Headline     string
	Summary      string
}

type LaunchGate struct {
	Key      string
	Label    string
	Status   string
	Evidence string
	Action   string
}

func BuildStatus(cfg Config) Status {
	readCacheScope := []string{}
	if cfg.Redis.Enabled() {
		readCacheScope = []string{
			"dashboard",
			"hot_videos",
			"hot_topics",
			"comment_insights",
			"report_summary",
		}
	}
	notes := []string{
		"当前仍保留 file/memory 兼容路径，便于 prototype 与团队版 Phase A 并行推进。",
	}
	bridgeMode := "demo-context"
	if cfg.Bridge.LocalOperatorEnabled() && !cfg.Bridge.HubSessionEnabled() && strings.TrimSpace(cfg.Bridge.TrustedToken) == "" {
		bridgeMode = "local-operator"
		if cfg.Bridge.DemoContextEnabled {
			notes = append(notes, "Hub-less local operator mode with demo fallback enabled.")
		} else {
			notes = append(notes, "Hub-less local operator mode: Insight owns operator identity; data plane is wx_channel :2026 (no Hub required).")
		}
	} else if cfg.Bridge.LocalOperatorEnabled() && strings.TrimSpace(cfg.Bridge.TrustedToken) != "" && !cfg.Bridge.HubSessionEnabled() {
		bridgeMode = "local-operator+trusted-header"
		notes = append(notes, "Hub-less local operator with optional trusted-header proxy path; Hub is not required.")
	} else if cfg.Bridge.HubSessionEnabled() && strings.TrimSpace(cfg.Bridge.TrustedToken) != "" {
		bridgeMode = "hub-session+trusted-header"
		if cfg.Bridge.DemoContextEnabled {
			notes = append(notes, "Hub JWT session bridge + trusted header enabled; demo query/header fallback still allowed.")
		} else {
			notes = append(notes, "Hub JWT session bridge (Authorization: Bearer validated via Hub /api/auth/profile) + trusted header; /api/v1/* rejects demo live identity.")
		}
	} else if cfg.Bridge.HubSessionEnabled() {
		bridgeMode = "hub-session"
		if cfg.Bridge.DemoContextEnabled {
			notes = append(notes, "Hub JWT session bridge enabled; demo query/header fallback still allowed.")
		} else {
			notes = append(notes, "Hub JWT session bridge enabled: /api/v1/* validates Authorization: Bearer against Hub without sharing JWT secret.")
		}
	} else if cfg.Bridge.TrustedModeEnabled() {
		bridgeMode = "trusted-header"
		if cfg.Bridge.DemoContextEnabled {
			notes = append(notes, "已启用 trusted Hub bridge，但当前仍允许 demo query/header 上下文作为联调回退。")
		} else {
			notes = append(notes, "已启用 trusted Hub bridge，/api/v1/* 将优先要求可信 header 注入；demo 上下文仅保留给显式 demo 路径。")
		}
	} else if cfg.Bridge.RequireTrustedForLive && !cfg.Bridge.DemoContextEnabled {
		bridgeMode = "trusted-required"
		notes = append(notes, "当前 runtime profile 已要求 trusted Hub bridge，但还没有配置 INSIGHT_TRUSTED_BRIDGE_TOKEN / INSIGHT_HUB_BASE_URL；/api/v1/* 不应继续回退到 demo live 身份。")
	} else {
		notes = append(notes, "当前仍处于 demo request-context 模式，query/header/default 仅适合原型或本地联调。")
	}
	if cfg.DashboardStore.Mode == StoreModePostgres || cfg.WorkspaceStore.Mode == StoreModePostgres || cfg.IngestStore.Mode == StoreModePostgres {
		notes = append(notes, "已启用 postgres store mode，当前支持 default workspace / ingest events / dashboard snapshot 的最小持久化路径。")
	}
	if cfg.Redis.Enabled() {
		notes = append(notes, "Redis read cache 已配置为 dashboard / hot rankings / comment insights / report summary 的热点读承接层；当前 live 是否已连通仍以 runtime truth 为准。")
	}
	notes = append(notes, "当前 analytics 热榜 / 报告 / 评论洞察查询仍留在 Insight 自己的 store owner 内，没有配置 Hub SQLite analytics query path。")
	notes = append(notes, "jobs 现已支持 bootstrap-schema、真实导入、dashboard read model 刷新与 operator 查询，可作为团队版 operator 入口。")

	return Status{
		Phase:                   "team-edition-phase-a",
		RuntimeProfile:          cfg.RuntimeProfile,
		HubBridgeMode:           bridgeMode,
		WXChannelBaseURL:        cfg.WXChannelBaseURL,
		OperatorIdentity:        cfg.DefaultHubUserID,
		DemoContextEnabled:      cfg.Bridge.DemoContextEnabled,
		DashboardStoreMode:      string(cfg.DashboardStore.Mode),
		RankingStoreMode:        string(cfg.RankingStore.Mode),
		CommentInsightStoreMode: string(cfg.CommentInsightStore.Mode),
		ReportSummaryStoreMode:  string(cfg.ReportSummaryStore.Mode),
		WorkspaceStoreMode:      string(cfg.WorkspaceStore.Mode),
		IngestStoreMode:         string(cfg.IngestStore.Mode),
		PostgresConfigured:      cfg.Postgres.Enabled(),
		RedisConfigured:         cfg.Redis.Enabled(),
		ReadCacheConfigured:     cfg.Redis.Enabled(),
		ReadCacheActive:         false,
		ReadCacheScope:          append([]string(nil), readCacheScope...),
		JobsEntrypointState:     "available",
		JobsRuntimeState:        "ingest-and-read-model-ops-available",
		Notes:                   notes,
	}.refreshLiveRuntimeAssessment()
}

func WithInfrastructureAvailability(status Status, postgresReady, redisReady bool) Status {
	status.PostgresReady = postgresReady
	status.RedisReady = redisReady
	return status.refreshLiveRuntimeAssessment()
}

func WithReadCacheSnapshot(status Status, snapshot insight.ReadCacheSnapshot) Status {
	status.ReadCacheActive = snapshot.Enabled
	status.ReadCacheHitCount = snapshot.HitCount
	status.ReadCacheMissCount = snapshot.MissCount
	status.ReadCacheFallbackCount = snapshot.FallbackCount
	status.ReadCacheWriteCount = snapshot.WriteCount
	status.ReadCacheInvalidateCount = snapshot.InvalidateCount
	status.ReadCacheLastHitAt = snapshot.LastHitAt
	status.ReadCacheLastMissAt = snapshot.LastMissAt
	status.ReadCacheLastFallbackAt = snapshot.LastFallbackAt
	status.ReadCacheLastWriteAt = snapshot.LastWriteAt
	status.ReadCacheLastInvalidateAt = snapshot.LastInvalidateAt
	status.ReadCacheScopeStats = cloneReadCacheScopeStats(snapshot.ScopeStats)
	return status.refreshLiveRuntimeAssessment()
}

func cloneReadCacheScopeStats(input map[string]insight.ReadCacheScopeSnapshot) map[string]insight.ReadCacheScopeSnapshot {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]insight.ReadCacheScopeSnapshot, len(input))
	for key, item := range input {
		result[key] = item
	}
	return result
}

func WithJobsRuntimeState(status Status, jobsState JobsRuntimeState) Status {
	if jobsState.LastFinishedAt.IsZero() && jobsState.UpdatedAt.IsZero() {
		return status
	}
	status.JobsLastMode = jobsState.LastMode
	status.JobsLastStatus = jobsState.LastStatus
	status.JobsLastHubUserID = jobsState.LastHubUserID
	status.JobsLastStartedAt = jobsState.LastStartedAt
	status.JobsLastFinishedAt = jobsState.LastFinishedAt
	status.JobsLastIteration = jobsState.LastIteration
	status.JobsLastIterations = jobsState.LastIterations
	status.JobsLastInterval = jobsState.LastInterval
	status.JobsLastError = jobsState.LastError
	status.JobsLastRequestedCount = jobsState.LastRequestedCount
	status.JobsLastRequestedSources = append([]string(nil), jobsState.LastRequestedSources...)
	status.JobsLastUpdatedAt = jobsState.UpdatedAt
	if jobsState.LastStatus != "" {
		status.JobsRuntimeState = "recent-" + jobsState.LastStatus
	}
	return status.refreshLiveRuntimeAssessment()
}

func WithWXChannelCapabilities(status Status, capabilities WXChannelCapabilityStatus) Status {
	if capabilities.BaseURL != "" {
		status.WXChannelBaseURL = capabilities.BaseURL
	}
	status.WXChannelCapabilitiesChecked = capabilities.Checked
	status.WXChannelCapabilitiesReady = capabilities.Ready
	status.WXChannelMissingCapabilities = append([]string(nil), capabilities.Missing...)
	status.WXChannelCapabilityRoutes = append([]WXChannelCapabilityRoute(nil), capabilities.Routes...)
	return status.refreshLiveRuntimeAssessment()
}

func WithTrustedRuntimeEvidence(status Status, evidence TrustedRuntimeEvidence) Status {
	status.TrustedRuntimeEvidence = finalizeTrustedRuntimeEvidence(evidence)
	return status.refreshLiveRuntimeAssessment()
}

func (status Status) refreshLiveRuntimeAssessment() Status {
	status.OwnerBoundaryState, status.OwnerBoundaryEvidence, status.OwnerBoundarySummary, status.OwnerBoundaryAction = deriveOwnerBoundaryAssessment(status)
	status.LiveRuntimeClass = deriveLiveRuntimeClass(status)
	status.LiveRuntimeProofState = deriveLiveRuntimeProofState(status)
	status.LiveRuntimeSummary = buildLiveRuntimeSummary(status)
	status.LiveRuntimeAction = buildLiveRuntimeAction(status)
	status.LiveRuntimeRequirements = buildLiveRuntimeRequirements(status)
	return status
}

func deriveOwnerBoundaryAssessment(status Status) (string, string, string, string) {
	storeParts := collectAnalyticsStoreParts(status)
	if len(storeParts) == 0 {
		return "unknown", "", "当前还没有拿到 analytics owner 的 runtime store 事实。", "先恢复 runtime status，再确认热榜 / 报告查询没有回到 Hub SQLite。"
	}

	if allStoreModesArePostgres(status) {
		return "met",
			"analytics=insight-postgres · stores=" + joinRequirementParts(storeParts),
			"当前热榜 / 报告 / 评论洞察 / dashboard / workspace / ingest 都由 Insight 自己的 postgres owner 承接，没有配置 Hub SQLite analytics query path。",
			"继续保持 analytics owner 留在 Insight 的正式存储与缓存层。"
	}

	return "met",
		"analytics=insight-compatible-store · stores=" + joinRequirementParts(storeParts),
		"当前 analytics 热路径仍在 Insight 自己的 file / memory / postgres store owner 内，没有看到 Hub SQLite analytics query path。",
		"继续在不越界的前提下，把 remaining owner 迁到 postgres。"
}

func collectAnalyticsStoreParts(status Status) []string {
	parts := []string{}
	appendStorePart := func(label, mode string) {
		if mode == "" {
			return
		}
		parts = append(parts, label+"="+mode)
	}

	appendStorePart("Dashboard", status.DashboardStoreMode)
	appendStorePart("Rankings", status.RankingStoreMode)
	appendStorePart("Comment Insights", status.CommentInsightStoreMode)
	appendStorePart("Report Summary", status.ReportSummaryStoreMode)
	appendStorePart("Workspace", status.WorkspaceStoreMode)
	appendStorePart("Ingest", status.IngestStoreMode)

	return parts
}

func deriveLiveRuntimeClass(status Status) string {
	if isTrustedOnlyLiveRuntime(status) &&
		status.PostgresReady &&
		status.RedisReady &&
		status.ReadCacheActive &&
		hasRequiredReadCacheScopeCoverage(status) &&
		hasRequiredReadCacheScopeActivityProof(status) &&
		allStoreModesArePostgres(status) &&
		wxChannelDataPlaneAllowsFormalReady(status) {
		return "formal-ready"
	}

	if status.HubBridgeMode == "trusted-header" ||
		status.HubBridgeMode == "hub-session" ||
		status.HubBridgeMode == "hub-session+trusted-header" ||
		status.HubBridgeMode == "local-operator" ||
		status.HubBridgeMode == "local-operator+trusted-header" ||
		status.HubBridgeMode == "trusted-required" ||
		status.PostgresConfigured ||
		status.RedisConfigured ||
		status.PostgresReady ||
		status.RedisReady ||
		status.ReadCacheConfigured ||
		status.ReadCacheActive {
		return "transition"
	}

	return "prototype-compatible"
}

func deriveLiveRuntimeProofState(status Status) string {
	baseState := status.TrustedRuntimeEvidence.State
	if baseState == "" {
		return "missing"
	}

	if baseState == "verified" {
		if !isTrustedOnlyLiveRuntime(status) {
			return "historical"
		}
	}

	return baseState
}

func buildLiveRuntimeSummary(status Status) string {
	switch status.LiveRuntimeClass {
	case "formal-ready":
		if status.LiveRuntimeProofState == "verified" {
			return "当前 live runtime 已切到 trusted-only + postgres/redis/read cache，且最近 trusted proof 与这套运行边界一致。"
		}
		if status.LiveRuntimeProofState == "missing" || status.LiveRuntimeProofState == "partial" {
			return "当前 live runtime 已接近正式团队版边界，但 bridge proof 还没有和这套运行态完全对齐。"
		}
		return "当前 live runtime 已切到 trusted-only + postgres/redis/read cache，可以继续结合 freshness 与 jobs 状态判断是否放行值班。"
	case "transition":
		if isTrustedOnlyLiveRuntime(status) {
			return "当前 live runtime 已进入团队版过渡态：trusted bridge 已收紧，但正式 owner、cache 或 proof 还没有全部补齐。"
		}
		if status.HubBridgeMode == "trusted-required" {
			return "当前 live runtime 已要求 trusted bridge 才能放行，但 trusted bridge token 还没有配置到位，正式身份入口仍未完成。"
		}
		return "当前 live runtime 仍在过渡态：部分团队版能力已接入，但 live 身份边界或正式 owner 还没有收紧到位。"
	default:
		return "当前 live runtime 仍是 prototype 兼容运行，demo 上下文或 file/memory owner 仍参与 live 路径。"
	}
}

func buildLiveRuntimeAction(status Status) string {
	switch status.LiveRuntimeClass {
	case "formal-ready":
		if status.LiveRuntimeProofState == "verified" {
			return "继续把 trusted proof、freshness 和 jobs 状态一起作为值班放行前提。"
		}
		return "对当前 live runtime 补跑 verify-insight-docker-stack.ps1，让 bridge proof 与当前端口和边界对齐。"
	case "transition":
		if isTrustedOnlyLiveRuntime(status) {
			return "继续把 remaining store owner、read cache 和 trusted proof 补齐，再把这套 runtime 当成正式团队值班底座。"
		}
		if status.HubBridgeMode == "trusted-required" {
			return "先配置 trusted bridge token，让 /api/v1/* 真正具备可信身份入口，再继续收敛正式 owner。"
		}
		return "先把 /api/v1/* 固定到 trusted bridge 并关闭 demo live fallback，再继续收敛正式 owner。"
	default:
		return "先收紧 trusted bridge，并切走 file/memory 兼容 owner，再谈正式团队值班。"
	}
}

func isTrustedOnlyLiveRuntime(status Status) bool {
	if status.DemoContextEnabled {
		return false
	}
	switch status.HubBridgeMode {
	case "trusted-header", "hub-session", "hub-session+trusted-header", "local-operator", "local-operator+trusted-header":
		return true
	default:
		return false
	}
}

func isTeamPreviewRuntime(status Status) bool {
	return status.RuntimeProfile == RuntimeProfileTeamPreview
}

func requiresWXChannelDataPlane(status Status) bool {
	if strings.HasPrefix(status.HubBridgeMode, "local-operator") {
		return true
	}
	return strings.TrimSpace(status.WXChannelBaseURL) != ""
}

func wxChannelDataPlaneAllowsFormalReady(status Status) bool {
	if !requiresWXChannelDataPlane(status) {
		return true
	}
	if !status.WXChannelCapabilitiesChecked {
		return true
	}
	return status.WXChannelCapabilitiesReady
}

func allStoreModesArePostgres(status Status) bool {
	return status.DashboardStoreMode == string(StoreModePostgres) &&
		status.RankingStoreMode == string(StoreModePostgres) &&
		status.CommentInsightStoreMode == string(StoreModePostgres) &&
		status.ReportSummaryStoreMode == string(StoreModePostgres) &&
		status.WorkspaceStoreMode == string(StoreModePostgres) &&
		status.IngestStoreMode == string(StoreModePostgres)
}

func hasRequiredReadCacheScopeCoverage(status Status) bool {
	if len(requiredReadCacheScopes) == 0 {
		return false
	}
	for _, scope := range requiredReadCacheScopes {
		if !containsReadCacheScope(status.ReadCacheScope, scope) {
			return false
		}
	}
	return true
}

func containsReadCacheScope(scopes []string, target string) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

func hasRequiredReadCacheScopeActivityProof(status Status) bool {
	if len(requiredReadCacheScopes) == 0 {
		return false
	}
	for _, scope := range requiredReadCacheScopes {
		item, ok := status.ReadCacheScopeStats[scope]
		if !ok || !hasReadCacheScopeActivity(item) {
			return false
		}
	}
	return true
}

func collectMissingRequiredReadCacheActivityScopes(status Status) []string {
	missing := []string{}
	for _, scope := range requiredReadCacheScopes {
		item, ok := status.ReadCacheScopeStats[scope]
		if !ok || !hasReadCacheScopeActivity(item) {
			missing = append(missing, scope)
		}
	}
	return missing
}

func collectMissingRequiredReadCacheCoverageScopes(status Status) []string {
	missing := []string{}
	for _, scope := range requiredReadCacheScopes {
		if !containsReadCacheScope(status.ReadCacheScope, scope) {
			missing = append(missing, scope)
		}
	}
	return missing
}

func hasReadCacheScopeActivity(item insight.ReadCacheScopeSnapshot) bool {
	return item.HitCount > 0 ||
		item.MissCount > 0 ||
		item.FallbackCount > 0 ||
		item.WriteCount > 0 ||
		!item.LastHitAt.IsZero() ||
		!item.LastMissAt.IsZero() ||
		!item.LastFallbackAt.IsZero() ||
		!item.LastWriteAt.IsZero()
}

func buildLiveRuntimeRequirements(status Status) []LiveRuntimeRequirement {
	requirements := []LiveRuntimeRequirement{
		buildLiveRuntimeBridgeRequirement(status),
		buildLiveRuntimeStoreRequirement(status),
		buildLiveRuntimeCacheRequirement(status),
		buildLiveRuntimeProofRequirement(status),
	}
	if status.WXChannelCapabilitiesChecked {
		requirements = append(requirements, buildWXChannelDataPlaneRequirement(status))
	}
	return requirements
}

func buildWXChannelDataPlaneRequirement(status Status) LiveRuntimeRequirement {
	if status.WXChannelCapabilitiesReady {
		return LiveRuntimeRequirement{
			Label:   "WX Channel Data Plane",
			Key:     "wx-channel-data-plane",
			Status:  "met",
			Summary: "当前 wx_channel 数据面已具备 feedSearch / feedList / commentExport 路由，Insight 可以通过 2026 API 获取视频列表与评论输入。",
			Action:  "继续把 wx_channel:2026 作为 Insight 的本地数据面，Hub 不参与数据采集路径。",
		}
	}

	missing := joinRequirementParts(status.WXChannelMissingCapabilities)
	if missing == "未记录" {
		missing = "required routes"
	}
	routeEvidence := summarizeWXChannelCapabilityRoutes(status.WXChannelCapabilityRoutes)
	if routeEvidence != "" {
		missing = missing + " · " + routeEvidence
	}
	return LiveRuntimeRequirement{
		Label:   "WX Channel Data Plane",
		Key:     "wx-channel-data-plane",
		Status:  "blocked",
		Summary: "当前 wx_channel:2026 响应了 status，但 Insight 必需接口缺失：" + missing + "。",
		Action:  "运行 wx_channel/scripts/switch-insight-data-plane-admin.cmd，把 2026 切到包含 feed search、feed list、comment export 的 Insight 数据面版本；如果源码仍缺接口，就在 wx_channel 补齐后再切换。",
	}
}

func summarizeWXChannelCapabilityRoutes(routes []WXChannelCapabilityRoute) string {
	parts := []string{}
	for _, route := range routes {
		if route.RoutePresent {
			continue
		}
		name := strings.TrimSpace(route.Name)
		if name == "" {
			name = "route"
		}
		if route.StatusCode > 0 {
			parts = append(parts, fmt.Sprintf("%s=HTTP %d", name, route.StatusCode))
			continue
		}
		if route.Error != "" {
			parts = append(parts, name+"="+route.Error)
			continue
		}
		parts = append(parts, name+"=missing")
	}
	return joinRequirementParts(parts)
}

func buildLiveRuntimeBridgeRequirement(status Status) LiveRuntimeRequirement {
	if isTrustedOnlyLiveRuntime(status) {
		return LiveRuntimeRequirement{
			Label:   "Trusted Bridge",
			Key:     "trusted-bridge",
			Status:  "met",
			Summary: "当前 live bridge 已固定为 trusted-only，demo 上下文不会再参与 /api/v1/* 的 live 身份判断。",
			Action:  "继续把 trusted proof 和 freshness 一起作为值班放行前提。",
		}
	}

	if status.HubBridgeMode == "trusted-header" && status.DemoContextEnabled {
		return LiveRuntimeRequirement{
			Label:   "Trusted Bridge",
			Key:     "trusted-bridge",
			Status:  "blocked",
			Summary: "当前虽然已启用 trusted header，但 demo query/header 仍保留为 live fallback，正式身份边界还没有完全收紧。",
			Action:  "关闭 demo 上下文对 /api/v1/* 的 live fallback，让当前 bridge 收紧为 trusted-only。",
		}
	}

	if status.HubBridgeMode == "trusted-required" {
		return LiveRuntimeRequirement{
			Label:   "Trusted Bridge",
			Key:     "trusted-bridge",
			Status:  "blocked",
			Summary: "当前 runtime profile 已要求 trusted bridge，但 INSIGHT_TRUSTED_BRIDGE_TOKEN 还没有配置，/api/v1/* 还缺可信身份入口。",
			Action:  "先配置 trusted bridge token，并让当前 live runtime 通过 trusted_header 进入，再继续推进正式团队版运行态。",
		}
	}

	return LiveRuntimeRequirement{
		Label:   "Trusted Bridge",
		Key:     "trusted-bridge",
		Status:  "blocked",
		Summary: "当前 live bridge 仍是 demo-context，浏览器里的 query / header 仍可能直接决定 /api/v1/* 的 live 身份。",
		Action:  "先把当前 /api/v1/* 切到 trusted bridge，再继续推进正式团队版运行态。",
	}
}

func buildLiveRuntimeStoreRequirement(status Status) LiveRuntimeRequirement {
	if allStoreModesArePostgres(status) {
		if status.PostgresReady {
			return LiveRuntimeRequirement{
				Label:   "Formal Stores",
				Key:     "formal-stores",
				Status:  "met",
				Summary: "Dashboard / Rankings / Comment Insights / Report Summary / Workspace / Ingest 都已切到 postgres owner，且当前 live runtime 已拿到 postgres 连通证据。",
				Action:  "继续保持 analytics owner 留在 Insight 自己的正式存储里。",
			}
		}
		if status.PostgresConfigured {
			return LiveRuntimeRequirement{
				Label:   "Formal Stores",
				Key:     "formal-stores",
				Status:  "unknown",
				Summary: "Dashboard / Rankings / Comment Insights / Report Summary / Workspace / Ingest 都已切到 postgres owner，但当前 live runtime 还缺 postgres 连通证据。",
				Action:  "补一条当前 API / jobs 运行态的 postgres 连通证据，再把这套环境当正式团队值班底座。",
			}
		}
		if isTeamPreviewRuntime(status) {
			return LiveRuntimeRequirement{
				Label:   "Formal Stores",
				Key:     "formal-stores",
				Status:  "blocked",
				Summary: "当前 runtime profile 已要求 postgres owner，store mode 也已全切到 postgres，但 Postgres 还没有配置到当前 live runtime。",
				Action:  "先配置 INSIGHT_POSTGRES_DSN 并启动本地 Postgres，再补当前 API / jobs 运行态的连通证据。",
			}
		}
		return LiveRuntimeRequirement{
			Label:   "Formal Stores",
			Key:     "formal-stores",
			Status:  "blocked",
			Summary: "Dashboard / Rankings / Comment Insights / Report Summary / Workspace / Ingest 都已切到 postgres owner，但当前 live runtime 还没有 Postgres 配置。",
			Action:  "先配置 INSIGHT_POSTGRES_DSN，再补当前运行态的 postgres 连通证据，确认正式 owner 真的已在 live runtime 落地。",
		}
	}

	parts := []string{}
	appendIfNotPostgres := func(label, mode string) {
		if mode != string(StoreModePostgres) {
			parts = append(parts, label+"="+mode)
		}
	}
	appendIfNotPostgres("Dashboard", status.DashboardStoreMode)
	appendIfNotPostgres("Rankings", status.RankingStoreMode)
	appendIfNotPostgres("Comment Insights", status.CommentInsightStoreMode)
	appendIfNotPostgres("Report Summary", status.ReportSummaryStoreMode)
	appendIfNotPostgres("Workspace", status.WorkspaceStoreMode)
	appendIfNotPostgres("Ingest", status.IngestStoreMode)

	if isTeamPreviewRuntime(status) && !status.PostgresConfigured {
		return LiveRuntimeRequirement{
			Label:   "Formal Stores",
			Key:     "formal-stores",
			Status:  "blocked",
			Summary: "当前 runtime profile 已要求 postgres owner，但 Postgres 还没有配置到当前 live runtime，store mode 仍停留在兼容路径：" + joinRequirementParts(parts),
			Action:  "先配置 INSIGHT_POSTGRES_DSN 并启动本地 Postgres，再让 team-preview runtime 真正切到 postgres owner。",
		}
	}

	return LiveRuntimeRequirement{
		Label:   "Formal Stores",
		Key:     "formal-stores",
		Status:  "blocked",
		Summary: "当前仍有 owner 停留在 file / memory 兼容路径：" + joinRequirementParts(parts),
		Action:  "把 remaining store mode 全部切到 postgres，再把这套 runtime 当成正式团队值班底座。",
	}
}

func buildLiveRuntimeCacheRequirement(status Status) LiveRuntimeRequirement {
	if status.RedisReady && status.ReadCacheActive && hasRequiredReadCacheScopeCoverage(status) {
		if hasRequiredReadCacheScopeActivityProof(status) {
			return LiveRuntimeRequirement{
				Label:   "Redis Cache",
				Key:     "redis-read-cache",
				Status:  "met",
				Summary: "Redis 已连通，read cache 已覆盖 dashboard / hot rankings / reports，且当前 live runtime 已拿到核心 scope 的活动证据。",
				Action:  "继续观察命中、未命中、回退和写回证据，确认热点读路径稳定落在 cache 上。",
			}
		}
		missingScopes := collectMissingRequiredReadCacheActivityScopes(status)
		return LiveRuntimeRequirement{
			Label:   "Redis Cache",
			Key:     "redis-read-cache",
			Status:  "unknown",
			Summary: "Redis 已连通，read cache scope 也已覆盖核心视图，但当前 live runtime 还缺这些 scope 的活动证据：" + joinRequirementParts(missingScopes),
			Action:  "先对 dashboard / hot rankings / reports 做真实访问，让这些 scope 的命中、未命中或写回证据进入当前 live runtime。",
		}
	}

	if status.RedisConfigured && !status.RedisReady {
		return LiveRuntimeRequirement{
			Label:   "Redis Cache",
			Key:     "redis-read-cache",
			Status:  "blocked",
			Summary: "Redis 已配置，但当前 live runtime 还没有拿到 Redis 连通证据。",
			Action:  "先确认当前 API / jobs 进程能真正连上 Redis，再继续判断 read cache 是否已进入 live 读路径。",
		}
	}

	if status.RedisReady {
		if status.ReadCacheActive {
			missingScopes := collectMissingRequiredReadCacheCoverageScopes(status)
			return LiveRuntimeRequirement{
				Label:   "Redis Cache",
				Key:     "redis-read-cache",
				Status:  "unknown",
				Summary: "Redis 已连通，read cache 也已启用，但当前 scope 还没有覆盖核心视图：" + joinRequirementParts(missingScopes),
				Action:  "把 read cache 真正接到 dashboard / hot rankings / reports，并确认 scope 覆盖核心视图。",
			}
		}
		return LiveRuntimeRequirement{
			Label:   "Redis Cache",
			Key:     "redis-read-cache",
			Status:  "unknown",
			Summary: "Redis 已连通，但当前 read cache 还没有真正进入可用 live 状态。",
			Action:  "确认 cache 已启用且 live 读路径真的会先命中 Redis，再回退正式 repository。",
		}
	}

	if isTeamPreviewRuntime(status) {
		return LiveRuntimeRequirement{
			Label:   "Redis Cache",
			Key:     "redis-read-cache",
			Status:  "blocked",
			Summary: "当前 runtime profile 已要求 Redis / read cache，但 Redis 还没有配置到当前 live runtime。",
			Action:  "先配置 INSIGHT_REDIS_ADDR 并启动本地 Redis，再让 team-preview runtime 接入正式缓存层。",
		}
	}

	return LiveRuntimeRequirement{
		Label:   "Redis Cache",
		Key:     "redis-read-cache",
		Status:  "blocked",
		Summary: "当前 live runtime 还没有 Redis / read cache，热点读请求仍缺少正式缓存承接层。",
		Action:  "先把 Redis 和 read cache 接入 live runtime，再继续收紧团队版运行底座。",
	}
}

func buildLiveRuntimeProofRequirement(status Status) LiveRuntimeRequirement {
	switch status.LiveRuntimeProofState {
	case "verified":
		return LiveRuntimeRequirement{
			Label:   "Trusted Proof",
			Key:     "trusted-proof",
			Status:  "met",
			Summary: "最近一次 trusted runtime proof 已经和当前 live 运行边界对齐。",
			Action:  "继续把这份 proof 和 freshness、jobs 状态一起作为值班放行依据。",
		}
	case "stale":
		return LiveRuntimeRequirement{
			Label:   "Trusted Proof",
			Key:     "trusted-proof",
			Status:  "unknown",
			Summary: "最近一次 trusted proof 指向的是另一套 live runtime 端口或 Hub 用户，当前运行态还缺同一套证据。",
			Action:  "对当前部署 再跑一次 verify-insight-docker-stack.ps1，让 trusted proof 和当前端口对齐。",
		}
	case "historical":
		return LiveRuntimeRequirement{
			Label:   "Trusted Proof",
			Key:     "trusted-proof",
			Status:  "unknown",
			Summary: "历史上已经跑通过 trusted runtime proof，但当前 live runtime 还没有收紧到同一套 trusted-only 边界。",
			Action:  "先把当前 bridge 收紧为 trusted-only，再对这套 live runtime 补跑 proof。",
		}
	case "partial":
		return LiveRuntimeRequirement{
			Label:   "Trusted Proof",
			Key:     "trusted-proof",
			Status:  "unknown",
			Summary: "当前 trusted runtime proof 只有部分记录，还不足以把这套 live runtime 当成正式值班底座。",
			Action:  "补齐 API plain-query 阻断和 Web proxy query-isolation 两侧证据。",
		}
	default:
		return LiveRuntimeRequirement{
			Label:   "Trusted Proof",
			Key:     "trusted-proof",
			Status:  "unknown",
			Summary: "当前还没有找到和这套 live runtime 对应的 trusted proof。",
			Action:  "对当前 live runtime 跑一次 verify-insight-docker-stack.ps1，补齐 bridge proof。",
		}
	}
}

func joinRequirementParts(parts []string) string {
	if len(parts) == 0 {
		return "未记录"
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += " · " + parts[i]
	}
	return result
}

// WithEdgeSummary attaches multi-client edge gateway counters to runtime status.
func WithEdgeSummary(status Status, enabled bool, online, ready, feed int) Status {
	status.EdgeEnabled = enabled
	status.EdgeOnlineNodes = online
	status.EdgeReadyNodes = ready
	status.EdgeFeedNodes = feed
	return status
}
