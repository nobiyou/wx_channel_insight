package runtimeapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestNewStatusResponseMapsRuntimeStatus(t *testing.T) {
	lastFallbackAt := time.Date(2026, 7, 3, 8, 15, 0, 0, time.UTC)
	result := NewStatusResponse(runtime.Status{
		Phase:              "team-edition-phase-a",
		RuntimeProfile:     runtime.RuntimeProfileTeamPreview,
		HubBridgeMode:      "trusted-header",
		DemoContextEnabled: false,
		LaunchReadiness: &runtime.LaunchReadiness{
			Gates: []runtime.LaunchGate{{
				Key:      "trusted-bridge",
				Label:    "真实 Hub 用户身份可进入 Insight",
				Status:   "met",
				Evidence: "Bridge=trusted-header · 证据 API + Web 已验证",
				Action:   "继续保持当前 bridge 证据。",
			}},
			MetCount:   1,
			TotalCount: 1,
			Tone:       "good",
			Headline:   "团队运营版的 8 条上线门槛都已有当前证据",
			Summary:    "当前运行态已经满足团队运营版上线门槛。",
		},
		OwnerBoundaryState:    "met",
		OwnerBoundaryEvidence: "analytics=insight-postgres · stores=Dashboard=postgres",
		OwnerBoundarySummary:  "当前热榜 / 报告 / 评论洞察查询都留在 Insight owner 内。",
		OwnerBoundaryAction:   "继续保持 analytics owner 留在 Insight。",
		LiveRuntimeClass:      "formal-ready",
		LiveRuntimeProofState: "verified",
		LiveRuntimeSummary:    "current live runtime is formal-ready",
		LiveRuntimeAction:     "continue with trusted proof checks",
		LiveRuntimeRequirements: []runtime.LiveRuntimeRequirement{
			{
				Label:   "Trusted Bridge",
				Key:     "trusted-bridge",
				Status:  "met",
				Summary: "trusted-only",
				Action:  "keep it",
			},
			{
				Label:   "Trusted Proof",
				Key:     "trusted-proof",
				Status:  "met",
				Summary: "proof aligned",
				Action:  "keep verifying",
			},
		},
		TrustedRuntimeEvidence: runtime.TrustedRuntimeEvidence{
			State:                  "verified",
			VerifiedAt:             time.Date(2026, 7, 3, 9, 30, 0, 0, time.UTC),
			Source:                 "verify-insight-docker-stack",
			HubUserID:              "hub:user:alice",
			TrustedBoundaryMode:    "trusted-only",
			APITrustedContextState: "verified",
			APIBrowserQueryState:   "blocked",
			WebTrustedContextState: "verified",
			WebBrowserQueryState:   "isolated",
			Summary:                "recent bridge proof ok",
			Notes:                  []string{"note-proof"},
		},
		DashboardStoreMode:       "file",
		RankingStoreMode:         "postgres",
		CommentInsightStoreMode:  "file",
		ReportSummaryStoreMode:   "postgres",
		WorkspaceStoreMode:       "postgres",
		IngestStoreMode:          "memory",
		PostgresConfigured:       true,
		PostgresReady:            true,
		RedisConfigured:          false,
		RedisReady:               false,
		ReadCacheConfigured:      false,
		ReadCacheActive:          false,
		ReadCacheScope:           []string{"dashboard"},
		ReadCacheHitCount:        4,
		ReadCacheMissCount:       1,
		ReadCacheFallbackCount:   1,
		ReadCacheWriteCount:      3,
		ReadCacheInvalidateCount: 1,
		ReadCacheLastFallbackAt:  lastFallbackAt,
		ReadCacheScopeStats: map[string]insight.ReadCacheScopeSnapshot{
			"dashboard": {
				HitCount:       2,
				MissCount:      1,
				FallbackCount:  1,
				WriteCount:     1,
				LastFallbackAt: lastFallbackAt,
				LastWriteAt:    time.Date(2026, 7, 3, 8, 10, 0, 0, time.UTC),
			},
		},
		JobsEntrypointState:          "available",
		JobsRuntimeState:             "recent-failed",
		JobsLastMode:                 "operator-loop",
		JobsLastStatus:               "failed",
		JobsLastHubUserID:            "hub:user:alice",
		JobsLastFinishedAt:           time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC),
		JobsLastIteration:            1,
		JobsLastIterations:           3,
		JobsLastInterval:             "5m0s",
		JobsLastError:                "wx_feed_list timeout",
		JobsLastRequestedCount:       2,
		JobsLastRequestedSources:     []string{"wx_comment_export", "wx_feed_list"},
		WXChannelBaseURL:             "http://127.0.0.1:2026",
		WXChannelConnected:           true,
		WXChannelReadyClients:        2,
		WXChannelFeedReady:           1,
		WXChannelCommentReady:        1,
		WXChannelCapabilitiesChecked: true,
		WXChannelCapabilitiesReady:   false,
		WXChannelMissingCapabilities: []string{"feedSearch", "commentExport"},
		WXChannelCapabilityRoutes: []runtime.WXChannelCapabilityRoute{
			{Name: "feedSearch", Method: http.MethodGet, URL: "http://127.0.0.1:2026/api/channels/feed/search?keyword=__insight_capability_probe__", StatusCode: 404, RoutePresent: false},
			{Name: "feedList", Method: http.MethodGet, URL: "http://127.0.0.1:2026/api/channels/contact/feed/list?username=__insight_capability_probe__", StatusCode: 400, RoutePresent: true},
		},
		Notes: []string{"note-a", "note-b"},
	})

	if result.Phase != "team-edition-phase-a" {
		t.Fatalf("expected phase to map, got %q", result.Phase)
	}
	if result.RuntimeProfile != runtime.RuntimeProfileTeamPreview {
		t.Fatalf("expected runtime profile to map, got %q", result.RuntimeProfile)
	}
	if result.HubBridgeMode != "trusted-header" || result.DemoContextEnabled {
		t.Fatalf("unexpected bridge flags: %+v", result)
	}
	if result.LaunchReadiness == nil || result.LaunchReadiness.MetCount != 1 || len(result.LaunchReadiness.Gates) != 1 {
		t.Fatalf("expected launch readiness to map, got %+v", result.LaunchReadiness)
	}
	if result.LaunchReadiness.Gates[0].Evidence == "" || result.LaunchReadiness.Gates[0].Action == "" {
		t.Fatalf("expected launch gate evidence/action to map, got %+v", result.LaunchReadiness.Gates[0])
	}
	if result.OwnerBoundaryState != "met" || result.OwnerBoundaryEvidence == "" {
		t.Fatalf("expected owner boundary fields to map, got %+v", result)
	}
	if result.OwnerBoundarySummary == "" || result.OwnerBoundaryAction == "" {
		t.Fatalf("expected owner boundary summary/action to map, got %+v", result)
	}
	if result.LiveRuntimeClass != "formal-ready" || result.LiveRuntimeProofState != "verified" {
		t.Fatalf("expected live runtime assessment to map, got %+v", result)
	}
	if result.LiveRuntimeSummary == "" || result.LiveRuntimeAction == "" {
		t.Fatalf("expected live runtime summary/action to map, got %+v", result)
	}
	if len(result.LiveRuntimeRequirements) != 2 {
		t.Fatalf("expected live runtime requirements to map, got %+v", result.LiveRuntimeRequirements)
	}
	if result.LiveRuntimeRequirements[0].Label != "Trusted Bridge" || result.LiveRuntimeRequirements[0].Key != "trusted-bridge" || result.LiveRuntimeRequirements[1].Status != "met" {
		t.Fatalf("expected live runtime requirements fields to map, got %+v", result.LiveRuntimeRequirements)
	}
	if result.TrustedRuntimeEvidence.State != "verified" || result.TrustedRuntimeEvidence.VerifiedAt != "2026-07-03T09:30:00Z" {
		t.Fatalf("expected trusted runtime evidence to map, got %+v", result.TrustedRuntimeEvidence)
	}
	if result.TrustedRuntimeEvidence.APIBrowserQueryState != "blocked" || result.TrustedRuntimeEvidence.WebBrowserQueryState != "isolated" {
		t.Fatalf("expected trusted proof states to map, got %+v", result.TrustedRuntimeEvidence)
	}
	if !result.PostgresConfigured || result.RedisConfigured {
		t.Fatalf("unexpected runtime flags: %+v", result)
	}
	if !result.PostgresReady || result.RedisReady || result.ReadCacheConfigured {
		t.Fatalf("expected readiness flags to map, got %+v", result)
	}
	if result.ReportSummaryStoreMode != "postgres" {
		t.Fatalf("expected report summary mode to map, got %+v", result)
	}
	if result.ReadCacheActive {
		t.Fatalf("expected read cache active false, got %+v", result)
	}
	if len(result.ReadCacheScope) != 1 || result.ReadCacheScope[0] != "dashboard" {
		t.Fatalf("expected read cache scope to map, got %+v", result.ReadCacheScope)
	}
	if result.RankingStoreMode != "postgres" {
		t.Fatalf("expected ranking mode to map, got %+v", result)
	}
	if result.CommentInsightStoreMode != "file" {
		t.Fatalf("expected comment insight mode to map, got %+v", result)
	}
	if result.ReadCacheFallbackCount != 1 || result.ReadCacheWriteCount != 3 {
		t.Fatalf("expected cache counters to map, got %+v", result)
	}
	if result.ReadCacheLastFallbackAt != "2026-07-03T08:15:00Z" {
		t.Fatalf("expected fallback timestamp to map, got %+v", result)
	}
	if scope := result.ReadCacheScopeStats["dashboard"]; scope.HitCount != 2 || scope.LastFallbackAt != "2026-07-03T08:15:00Z" {
		t.Fatalf("expected scoped cache stats to map, got %+v", result.ReadCacheScopeStats)
	}
	if result.JobsLastMode != "operator-loop" || result.JobsLastStatus != "failed" {
		t.Fatalf("expected jobs runtime fields to map, got %+v", result)
	}
	if result.JobsLastFinishedAt != "2026-07-03T09:00:00Z" {
		t.Fatalf("expected jobs finished timestamp to map, got %+v", result)
	}
	if len(result.JobsLastRequestedSources) != 2 || result.JobsLastRequestedSources[0] != "wx_comment_export" || result.JobsLastRequestedSources[1] != "wx_feed_list" {
		t.Fatalf("expected jobs requested sources to map, got %+v", result)
	}
	if result.WXChannelBaseURL != "http://127.0.0.1:2026" || !result.WXChannelConnected || result.WXChannelReadyClients != 2 {
		t.Fatalf("expected wx_channel status fields to map, got %+v", result)
	}
	if !result.WXChannelCapabilitiesChecked || result.WXChannelCapabilitiesReady {
		t.Fatalf("expected wx_channel capability flags to map, got %+v", result)
	}
	if len(result.WXChannelMissingCapabilities) != 2 || result.WXChannelMissingCapabilities[0] != "feedSearch" {
		t.Fatalf("expected missing capabilities to map, got %+v", result.WXChannelMissingCapabilities)
	}
	if len(result.WXChannelCapabilityRoutes) != 2 || result.WXChannelCapabilityRoutes[1].Name != "feedList" || !result.WXChannelCapabilityRoutes[1].RoutePresent {
		t.Fatalf("expected capability routes to map, got %+v", result.WXChannelCapabilityRoutes)
	}
	if len(result.Notes) != 2 {
		t.Fatalf("expected notes to map, got %+v", result.Notes)
	}
}
