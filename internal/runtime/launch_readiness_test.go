package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

func TestWithLaunchReadinessMarksAllEightGatesMet(t *testing.T) {
	materializedAt := time.Date(2026, 7, 6, 10, 57, 1, 0, time.UTC)
	status := Status{
		RuntimeProfile:          RuntimeProfileTeamPreview,
		HubBridgeMode:           "trusted-header",
		DemoContextEnabled:      false,
		OwnerBoundaryState:      "met",
		OwnerBoundaryEvidence:   "analytics=insight-postgres · stores=Dashboard=postgres · Rankings=postgres · Comment Insights=postgres · Report Summary=postgres · Workspace=postgres · Ingest=postgres",
		OwnerBoundarySummary:    "当前 analytics 热路径仍留在 Insight 自己的 store owner 内。",
		OwnerBoundaryAction:     "继续保持当前 owner 边界。",
		DashboardStoreMode:      "postgres",
		RankingStoreMode:        "postgres",
		CommentInsightStoreMode: "postgres",
		ReportSummaryStoreMode:  "postgres",
		WorkspaceStoreMode:      "postgres",
		IngestStoreMode:         "postgres",
		PostgresConfigured:      true,
		PostgresReady:           true,
		RedisConfigured:         true,
		RedisReady:              true,
		ReadCacheConfigured:     true,
		ReadCacheActive:         true,
		ReadCacheScope:          []string{"dashboard", "hot_videos", "hot_topics", "comment_insights", "report_summary"},
		ReadCacheScopeStats: map[string]insight.ReadCacheScopeSnapshot{
			"dashboard":      {HitCount: 4},
			"hot_videos":     {HitCount: 1},
			"hot_topics":     {HitCount: 1},
			"report_summary": {HitCount: 1},
		},
		JobsLastMode:             "operator-loop",
		JobsLastStatus:           "ok",
		JobsLastFinishedAt:       materializedAt,
		JobsLastRequestedCount:   2,
		JobsLastRequestedSources: []string{"wx_comment_export", "wx_video_search"},
		TrustedRuntimeEvidence: TrustedRuntimeEvidence{
			State:                  "verified",
			Source:                 "verify-insight-docker-stack",
			VerifiedAt:             time.Date(2026, 7, 6, 10, 58, 44, 0, time.UTC),
			HubUserID:              "hub:user:team-runtime-20260706185558",
			APIURL:                 "http://127.0.0.1:44422",
			WebURL:                 "http://127.0.0.1:44423",
			TrustedBoundaryMode:    "trusted-only",
			APITrustedContextState: "verified",
			APIBrowserQueryState:   "blocked",
			WebTrustedContextState: "verified",
			WebBrowserQueryState:   "isolated",
		},
	}

	dashboard := insight.Dashboard{
		ReadModel: insight.ReadModelState{
			Mode:           "materialized",
			LatestBatchID:  "batch-launch-1",
			MaterializedAt: materializedAt,
			ViewStatuses: []insight.ReadModelViewStatus{
				{Key: "videos", LatestBatchID: "batch-launch-1", MaterializedAt: materializedAt},
				{Key: "topics", LatestBatchID: "batch-launch-1", MaterializedAt: materializedAt},
				{Key: "comments", LatestBatchID: "batch-launch-1", MaterializedAt: materializedAt},
				{Key: "reports", LatestBatchID: "batch-launch-1", MaterializedAt: materializedAt},
			},
		},
		WorkspaceBridge: insight.WorkspaceBridge{
			State:         "ready",
			WorkspaceID:   "ws-default-hub-user-team-runtime-20260706185558",
			HubUserID:     "hub:user:team-runtime-20260706185558",
			ContextSource: "trusted_header",
			BootstrapMode: "lazy_create",
			Source:        "insight_default_workspace",
			Resolution:    "reused",
			LastSyncedAt:  time.Date(2026, 7, 6, 10, 56, 37, 0, time.UTC),
		},
		IngestSummary: insight.IngestSummary{
			State:          "ready",
			FreshnessState: "fresh",
			LatestBatchID:  "batch-launch-1",
			LastSuccessAt:  materializedAt,
			BindingStates: []insight.IngestBindingState{
				{
					Source:            "wx_comment_export",
					Runnable:          true,
					LastRunStatus:     "success",
					LastAutoRunStatus: "success",
					LastAutoRunAt:     materializedAt.Add(-time.Minute),
					LastAutoSuccessAt: materializedAt.Add(-time.Minute),
					LastAutoBatchID:   "batch-launch-older-comment",
				},
				{
					Source:            "wx_comment_export",
					Runnable:          true,
					LastRunStatus:     "success",
					LastAutoRunStatus: "success",
					LastAutoRunAt:     materializedAt,
					LastAutoSuccessAt: materializedAt,
					LastAutoBatchID:   "batch-launch-1",
				},
				{
					Source:            "wx_video_search",
					Runnable:          true,
					LastRunStatus:     "success",
					LastAutoRunStatus: "success",
					LastAutoRunAt:     materializedAt,
					LastAutoSuccessAt: materializedAt,
					LastAutoBatchID:   "batch-launch-1",
				},
			},
		},
	}

	status = WithLaunchReadiness(status, dashboard)
	if status.LaunchReadiness == nil {
		t.Fatalf("expected launch readiness to be attached, got %+v", status)
	}
	if status.LaunchReadiness.MetCount != 8 || status.LaunchReadiness.BlockedCount != 0 || status.LaunchReadiness.UnknownCount != 0 {
		t.Fatalf("expected all eight launch gates met, got %+v", status.LaunchReadiness)
	}
	if status.LaunchReadiness.Tone != "good" {
		t.Fatalf("expected good tone, got %+v", status.LaunchReadiness)
	}
	if len(status.LaunchReadiness.Gates) != 8 {
		t.Fatalf("expected eight launch gates, got %+v", status.LaunchReadiness)
	}
	if status.LaunchReadiness.Gates[0].Key != "trusted-bridge" || status.LaunchReadiness.Gates[0].Status != "met" {
		t.Fatalf("expected trusted bridge gate met, got %+v", status.LaunchReadiness.Gates[0])
	}
	realSources := findLaunchReadinessGateForTest(status.LaunchReadiness, "real-sources")
	if realSources == nil || strings.Count(realSources.Evidence, "wx_comment_export=") != 1 ||
		strings.Contains(realSources.Evidence, "batch-launch-older-comment") {
		t.Fatalf("expected deduplicated latest comment source evidence, got %+v", realSources)
	}

	status = WithLaunchReadinessMonitoring(status, dashboard, monitoring.AutomationHealth{
		Checked:            true,
		EnabledTaskCount:   1,
		UnhealthyTaskCount: 1,
		TaskID:             "default-monitoring",
		TaskName:           "默认监控",
		LastStatus:         monitoring.RunStatusAbandoned,
		LastRunAt:          materializedAt.Add(time.Minute),
		LastFailureAt:      materializedAt.Add(time.Minute),
		LastError:          "monitoring task lease expired",
	})
	if status.LaunchReadiness.MetCount == 8 || status.LaunchReadiness.UnknownCount != 1 {
		t.Fatalf("abandoned monitoring automation must prevent 8/8 readiness, got %+v", status.LaunchReadiness)
	}
	realSources = findLaunchReadinessGateForTest(status.LaunchReadiness, "real-sources")
	if realSources == nil || realSources.Status != "unknown" || !strings.Contains(realSources.Evidence, "monitoring task lease expired") {
		t.Fatalf("expected monitoring failure in automation proof, got %+v", realSources)
	}
	visibility := findLaunchReadinessGateForTest(status.LaunchReadiness, "visibility")
	if visibility == nil || !strings.Contains(visibility.Evidence, "monitoring=abandoned") {
		t.Fatalf("expected monitoring failure in visibility evidence, got %+v", visibility)
	}
}

func TestWithLaunchReadinessUsesSpecificPrimarySourceGapForRealSourcesGate(t *testing.T) {
	materializedAt := time.Date(2026, 7, 7, 3, 47, 8, 0, time.UTC)
	status := Status{
		JobsLastMode:             "operator-loop",
		JobsLastStatus:           "partial",
		JobsLastRequestedCount:   2,
		JobsLastRequestedSources: []string{"wx_comment_export", "wx_video_search"},
	}

	dashboard := insight.Dashboard{
		IngestSummary: insight.IngestSummary{
			BindingStates: []insight.IngestBindingState{
				{
					Source:            "wx_comment_export",
					Runnable:          true,
					LastRunStatus:     "success",
					LastAutoRunStatus: "success",
					LastAutoSuccessAt: materializedAt,
					LastAutoBatchID:   "batch-real-sources-1",
				},
			},
			OperatorReadiness: insight.OperatorReadiness{
				State:           "partial",
				ExpectedStatus:  "partial",
				BlockedSources:  1,
				RunnableSources: 1,
				Blocked: []insight.OperatorReadinessSource{
					{
						Source:      "wx_video_search",
						State:       "blocked",
						Summary:     "video search has no ready page client",
						RetryReason: "video search has no ready page client",
						BaseURL:     "http://127.0.0.1:2026",
						Action:      "先确认视频号独立客户端已通过 wx_channel 代理连上 API，再回填 base URL 和视频搜索关键词。",
					},
				},
			},
		},
	}

	status = WithLaunchReadiness(status, dashboard)
	if status.LaunchReadiness == nil {
		t.Fatalf("expected launch readiness, got %+v", status)
	}

	var realSourcesGate *LaunchGate
	for index := range status.LaunchReadiness.Gates {
		item := &status.LaunchReadiness.Gates[index]
		if item.Key == "real-sources" {
			realSourcesGate = item
			break
		}
	}
	if realSourcesGate == nil {
		t.Fatalf("expected real-sources gate, got %+v", status.LaunchReadiness.Gates)
	}
	if realSourcesGate.Status != "blocked" {
		t.Fatalf("expected blocked real-sources gate, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Evidence, "wx_video_search=blocked:video search has no ready page client") {
		t.Fatalf("expected blocked video-search evidence, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Evidence, "jobs=operator-loop/partial") {
		t.Fatalf("expected operator-loop proof details in evidence, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Action, "api/channels/status") {
		t.Fatalf("expected status endpoint guidance in blocked-source action, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Action, "http://127.0.0.1:2026/api/channels/status") {
		t.Fatalf("expected blocked-source action to carry status url, got %+v", realSourcesGate)
	}
}

func TestWithLaunchReadinessAcceptsFeedListAsVideoSource(t *testing.T) {
	materializedAt := time.Date(2026, 8, 5, 10, 57, 1, 0, time.UTC)
	status := Status{
		RuntimeProfile:          RuntimeProfileTeamPreview,
		HubBridgeMode:           "local-operator",
		DemoContextEnabled:      false,
		OwnerBoundaryState:      "met",
		OwnerBoundaryEvidence:   "analytics=insight-postgres",
		OwnerBoundarySummary:    "analytics owner is local Insight postgres",
		OwnerBoundaryAction:     "keep owner boundary",
		DashboardStoreMode:      "postgres",
		RankingStoreMode:        "postgres",
		CommentInsightStoreMode: "postgres",
		ReportSummaryStoreMode:  "postgres",
		WorkspaceStoreMode:      "postgres",
		IngestStoreMode:         "postgres",
		PostgresConfigured:      true,
		PostgresReady:           true,
		RedisConfigured:         true,
		RedisReady:              true,
		ReadCacheConfigured:     true,
		ReadCacheActive:         true,
		ReadCacheScope:          []string{"dashboard", "hot_videos", "hot_topics", "comment_insights", "report_summary"},
		ReadCacheScopeStats: map[string]insight.ReadCacheScopeSnapshot{
			"dashboard":      {HitCount: 4},
			"hot_videos":     {HitCount: 1},
			"hot_topics":     {HitCount: 1},
			"report_summary": {HitCount: 1},
		},
		JobsLastMode:             "operator-loop",
		JobsLastStatus:           "ok",
		JobsLastFinishedAt:       materializedAt,
		JobsLastRequestedCount:   2,
		JobsLastRequestedSources: []string{"wx_comment_export", "wx_feed_list"},
		TrustedRuntimeEvidence: TrustedRuntimeEvidence{
			State:                  "verified",
			Source:                 "verify-insight-docker-stack",
			VerifiedAt:             materializedAt,
			HubUserID:              "insight:operator:local",
			APIURL:                 "http://127.0.0.1:18081",
			WebURL:                 "http://127.0.0.1:5176",
			TrustedBoundaryMode:    "trusted-only",
			APITrustedContextState: "verified",
			APIBrowserQueryState:   "blocked",
			WebTrustedContextState: "verified",
			WebBrowserQueryState:   "isolated",
		},
	}

	dashboard := insight.Dashboard{
		ReadModel: insight.ReadModelState{
			Mode:           "materialized",
			LatestBatchID:  "batch-feed-list",
			MaterializedAt: materializedAt,
			ViewStatuses: []insight.ReadModelViewStatus{
				{Key: "videos", LatestBatchID: "batch-feed-list", MaterializedAt: materializedAt},
				{Key: "topics", LatestBatchID: "batch-feed-list", MaterializedAt: materializedAt},
				{Key: "comments", LatestBatchID: "batch-feed-list", MaterializedAt: materializedAt},
				{Key: "reports", LatestBatchID: "batch-feed-list", MaterializedAt: materializedAt},
			},
		},
		WorkspaceBridge: insight.WorkspaceBridge{
			State:         "ready",
			WorkspaceID:   "ws-default-insight-operator-local",
			HubUserID:     "insight:operator:local",
			ContextSource: "local_operator",
			BootstrapMode: "lazy_create",
			Source:        "insight_default_workspace",
			Resolution:    "reused",
			LastSyncedAt:  materializedAt,
		},
		IngestSummary: insight.IngestSummary{
			State:          "ready",
			FreshnessState: "fresh",
			LatestBatchID:  "batch-feed-list",
			LastSuccessAt:  materializedAt,
			BindingStates: []insight.IngestBindingState{
				{
					Source:            "wx_comment_export",
					Runnable:          true,
					LastRunStatus:     "success",
					LastAutoRunStatus: "success",
					LastAutoRunAt:     materializedAt,
					LastAutoSuccessAt: materializedAt,
					LastAutoBatchID:   "batch-feed-list",
				},
				{
					Source:            "wx_feed_list",
					Runnable:          true,
					LastRunStatus:     "success",
					LastAutoRunStatus: "success",
					LastAutoRunAt:     materializedAt,
					LastAutoSuccessAt: materializedAt,
					LastAutoBatchID:   "batch-feed-list",
				},
			},
		},
	}

	status = WithLaunchReadiness(status, dashboard)
	if status.LaunchReadiness == nil {
		t.Fatalf("expected launch readiness to be attached, got %+v", status)
	}
	if status.LaunchReadiness.MetCount != 8 || status.LaunchReadiness.BlockedCount != 0 || status.LaunchReadiness.UnknownCount != 0 {
		t.Fatalf("expected feed-list video source to satisfy all eight launch gates, got %+v", status.LaunchReadiness)
	}
	realSourcesGate := findLaunchReadinessGateForTest(status.LaunchReadiness, "real-sources")
	if realSourcesGate == nil {
		t.Fatalf("expected real-sources gate, got %+v", status.LaunchReadiness.Gates)
	}
	if strings.Contains(realSourcesGate.Evidence, "wx_video_search=missing") {
		t.Fatalf("feed-list success should not be downgraded by missing wx_video_search, got %+v", realSourcesGate)
	}
}

func TestWithLaunchReadinessBlocksRealSourcesWhenWXChannelRoutesAreMissing(t *testing.T) {
	materializedAt := time.Date(2026, 8, 5, 10, 57, 1, 0, time.UTC)
	status := Status{
		WXChannelCapabilitiesChecked: true,
		WXChannelCapabilitiesReady:   false,
		WXChannelMissingCapabilities: []string{"feedSearch", "commentExport"},
		WXChannelCapabilityRoutes: []WXChannelCapabilityRoute{
			{Name: "feedSearch", URL: "http://127.0.0.1:2026/api/channels/feed/search?keyword=__insight_capability_probe__", StatusCode: 404, RoutePresent: false},
			{Name: "commentExport", URL: "http://127.0.0.1:2026/api/channels/feed/comment/export", StatusCode: 404, RoutePresent: false},
		},
	}
	dashboard := insight.Dashboard{
		IngestSummary: insight.IngestSummary{
			BindingStates: []insight.IngestBindingState{
				{
					Source:            "wx_comment_export",
					Runnable:          true,
					LastAutoRunStatus: "success",
					LastAutoRunAt:     materializedAt,
					LastAutoSuccessAt: materializedAt,
					LastAutoBatchID:   "batch-old-success",
				},
				{
					Source:            "wx_feed_list",
					Runnable:          true,
					LastAutoRunStatus: "success",
					LastAutoRunAt:     materializedAt,
					LastAutoSuccessAt: materializedAt,
					LastAutoBatchID:   "batch-old-success",
				},
			},
		},
	}

	status = WithLaunchReadiness(status, dashboard)
	realSourcesGate := findLaunchReadinessGateForTest(status.LaunchReadiness, "real-sources")
	if realSourcesGate == nil {
		t.Fatalf("expected real-sources gate, got %+v", status.LaunchReadiness)
	}
	if realSourcesGate.Status != "blocked" {
		t.Fatalf("expected missing wx_channel routes to block real sources, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Evidence, "feedSearch") || !strings.Contains(realSourcesGate.Evidence, "commentExport") {
		t.Fatalf("expected missing capability names in evidence, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Action, "switch-insight-data-plane-admin.cmd") {
		t.Fatalf("expected admin switch action, got %+v", realSourcesGate)
	}
}

func TestWithLaunchReadinessSurfacesFeedListBlockerWhenItIsTheSavedVideoSource(t *testing.T) {
	status := Status{
		JobsLastMode:             "operator-cycle",
		JobsLastStatus:           "ok",
		JobsLastRequestedCount:   2,
		JobsLastRequestedSources: []string{"wx_comment_export", "wx_feed_list"},
	}

	dashboard := insight.Dashboard{
		IngestSummary: insight.IngestSummary{
			OperatorReadiness: insight.OperatorReadiness{
				State:           "partial",
				ExpectedStatus:  "partial",
				BlockedSources:  2,
				RunnableSources: 0,
				Blocked: []insight.OperatorReadinessSource{
					{
						Source:      "wx_feed_list",
						State:       "blocked",
						Summary:     "feed list ws/health is unreachable",
						RetryReason: "feed list ws/health is unreachable",
						BaseURL:     "http://127.0.0.1:2026",
						Action:      "先确认视频号独立客户端已通过 wx_channel 代理连上 API，再回填 base URL 和 username。",
					},
					{
						Source:      "wx_comment_export",
						State:       "blocked",
						Summary:     "comment export ws/health is unreachable",
						RetryReason: "comment export ws/health is unreachable",
						BaseURL:     "http://127.0.0.1:2026",
						Action:      "先确认目标视频已经在视频号独立客户端里切到位且接口已就绪；如果只是路径失效，再回填新的评论导出 JSON 文件路径。",
					},
				},
			},
		},
	}

	status = WithLaunchReadiness(status, dashboard)
	if status.LaunchReadiness == nil {
		t.Fatalf("expected launch readiness, got %+v", status)
	}

	var realSourcesGate *LaunchGate
	for index := range status.LaunchReadiness.Gates {
		item := &status.LaunchReadiness.Gates[index]
		if item.Key == "real-sources" {
			realSourcesGate = item
			break
		}
	}
	if realSourcesGate == nil {
		t.Fatalf("expected real-sources gate, got %+v", status.LaunchReadiness.Gates)
	}
	if strings.Contains(realSourcesGate.Evidence, "wx_video_search=missing") {
		t.Fatalf("feed-list binding should satisfy the video-source slot instead of forcing wx_video_search, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Evidence, "wx_feed_list=blocked:feed list ws/health is unreachable") {
		t.Fatalf("expected feed-list blocker to stay visible as video-source evidence, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Action, "api/channels/status") {
		t.Fatalf("expected action to guide wx_channel status recovery, got %+v", realSourcesGate)
	}
}

func TestWithLaunchReadinessRecommendsCurrentConfigForDemoRuntime(t *testing.T) {
	status := Status{
		HubBridgeMode:      "demo-context",
		DemoContextEnabled: true,
		DashboardStoreMode: "file",
		RankingStoreMode:   "file",
	}

	status = WithLaunchReadiness(status, insight.Dashboard{})
	if status.LaunchReadiness == nil {
		t.Fatalf("expected launch readiness, got %+v", status)
	}

	var trustedBridgeGate *LaunchGate
	for index := range status.LaunchReadiness.Gates {
		item := &status.LaunchReadiness.Gates[index]
		if item.Key == "trusted-bridge" {
			trustedBridgeGate = item
			break
		}
	}
	if trustedBridgeGate == nil {
		t.Fatalf("expected trusted-bridge gate, got %+v", status.LaunchReadiness.Gates)
	}
	if trustedBridgeGate.Status != "blocked" {
		t.Fatalf("expected blocked trusted-bridge gate, got %+v", trustedBridgeGate)
	}
	for _, field := range []string{"bridge.local_operator", "bridge.trusted_token", "bridge.demo_context_enabled"} {
		if !strings.Contains(trustedBridgeGate.Action, field) {
			t.Fatalf("expected current config field %q in action: %+v", field, trustedBridgeGate)
		}
	}
}

func TestWithLaunchReadinessIncludesMultiplePrimarySourceBlockers(t *testing.T) {
	status := Status{
		JobsLastMode:             "operator-cycle",
		JobsLastStatus:           "failed",
		JobsLastRequestedCount:   1,
		JobsLastRequestedSources: []string{"wx_comment_export"},
	}

	dashboard := insight.Dashboard{
		IngestSummary: insight.IngestSummary{
			OperatorReadiness: insight.OperatorReadiness{
				State:          "blocked",
				ExpectedStatus: "failed",
				BlockedSources: 2,
				Blocked: []insight.OperatorReadinessSource{
					{
						Source:      "wx_comment_export",
						State:       "blocked",
						Summary:     "当前客户端上下文与保存的评论导出视频不匹配",
						RetryReason: "当前客户端上下文与保存的评论导出视频不匹配",
						Action:      "先在视频号独立客户端中切到这条评论对应的视频，等待当前视频信息完全加载后再重试；如果这是旧绑定，请重新导出评论或重新绑定当前视频。",
					},
					{
						Source:      "wx_video_search",
						State:       "blocked",
						Summary:     "video search has no ready page client",
						RetryReason: "video search has no ready page client",
						BaseURL:     "http://127.0.0.1:2026",
						Action:      "先确认视频号独立客户端已通过 wx_channel 代理连上 API，再回填 base URL 和视频搜索关键词。",
					},
				},
			},
		},
	}

	status = WithLaunchReadiness(status, dashboard)
	if status.LaunchReadiness == nil {
		t.Fatalf("expected launch readiness, got %+v", status)
	}

	var realSourcesGate *LaunchGate
	for index := range status.LaunchReadiness.Gates {
		item := &status.LaunchReadiness.Gates[index]
		if item.Key == "real-sources" {
			realSourcesGate = item
			break
		}
	}
	if realSourcesGate == nil {
		t.Fatalf("expected real-sources gate, got %+v", status.LaunchReadiness.Gates)
	}
	if !strings.Contains(realSourcesGate.Evidence, "wx_comment_export=blocked:当前客户端上下文与保存的评论导出视频不匹配") {
		t.Fatalf("expected comment-export blocker in evidence, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Evidence, "wx_video_search=blocked:video search has no ready page client") {
		t.Fatalf("expected video-search blocker in evidence, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Action, "api/channels/status") {
		t.Fatalf("expected feed-list status guidance in action, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Action, "重新导出评论或重新绑定当前视频") {
		t.Fatalf("expected comment-export recovery action, got %+v", realSourcesGate)
	}
}

func TestWithLaunchReadinessRecommendsOperatorLoopWhenPrimarySourcesAreRunnableButLackAutoProof(t *testing.T) {
	status := Status{
		JobsLastMode:             "operator-cycle",
		JobsLastStatus:           "ok",
		JobsLastRequestedCount:   2,
		JobsLastRequestedSources: []string{"wx_comment_export", "wx_video_search"},
	}

	dashboard := insight.Dashboard{
		IngestSummary: insight.IngestSummary{
			BindingStates: []insight.IngestBindingState{
				{
					Source:        "wx_comment_export",
					Runnable:      true,
					LastRunStatus: "success",
					LatestBatchID: "batch-manual-1",
				},
				{
					Source:        "wx_video_search",
					Runnable:      true,
					LastRunStatus: "success",
					LatestBatchID: "batch-manual-2",
				},
			},
			OperatorReadiness: insight.OperatorReadiness{
				State:           "ready",
				ExpectedStatus:  "ok",
				RunnableSources: 2,
				Headline:        "今天这轮 operator 可以直接跑，2 条来源都已就绪",
				NextAction:      "直接运行今天这轮 operator，然后检查新批次是否已经进入 materialized read model。",
			},
		},
	}

	status = WithLaunchReadiness(status, dashboard)
	if status.LaunchReadiness == nil {
		t.Fatalf("expected launch readiness, got %+v", status)
	}

	var realSourcesGate *LaunchGate
	for index := range status.LaunchReadiness.Gates {
		item := &status.LaunchReadiness.Gates[index]
		if item.Key == "real-sources" {
			realSourcesGate = item
			break
		}
	}
	if realSourcesGate == nil {
		t.Fatalf("expected real-sources gate, got %+v", status.LaunchReadiness.Gates)
	}
	if realSourcesGate.Status != "blocked" {
		t.Fatalf("expected blocked real-sources gate, got %+v", realSourcesGate)
	}
	if !strings.Contains(realSourcesGate.Action, "operator-loop") {
		t.Fatalf("expected operator-loop guidance, got %+v", realSourcesGate)
	}
}

func findLaunchReadinessGateForTest(readiness *LaunchReadiness, key string) *LaunchGate {
	if readiness == nil {
		return nil
	}
	for index := range readiness.Gates {
		item := &readiness.Gates[index]
		if item.Key == key {
			return item
		}
	}
	return nil
}
