package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestBuildStatusReflectsConfig(t *testing.T) {
	cfg := Config{
		RuntimeProfile:      RuntimeProfileTeamPreview,
		Bridge:              BridgeConfig{TrustedToken: "bridge-secret", DemoContextEnabled: false, RequireTrustedForLive: true},
		DashboardStore:      StoreConfig{Mode: StoreModePostgres},
		RankingStore:        StoreConfig{Mode: StoreModeMemory},
		CommentInsightStore: StoreConfig{Mode: StoreModeFile},
		ReportSummaryStore:  StoreConfig{Mode: StoreModePostgres},
		WorkspaceStore:      StoreConfig{Mode: StoreModeFile},
		IngestStore:         StoreConfig{Mode: StoreModeMemory},
		Postgres:            PostgresConfig{DSN: "postgres://demo"},
		Redis:               RedisConfig{Addr: "127.0.0.1:6379"},
	}

	status := BuildStatus(cfg)
	if status.Phase != "team-edition-phase-a" {
		t.Fatalf("expected phase-a status, got %q", status.Phase)
	}
	if status.RuntimeProfile != RuntimeProfileTeamPreview {
		t.Fatalf("expected runtime profile team-preview, got %q", status.RuntimeProfile)
	}
	if status.HubBridgeMode != "trusted-header" || status.DemoContextEnabled {
		t.Fatalf("expected trusted bridge mode, got %+v", status)
	}
	if status.LiveRuntimeClass != "transition" {
		t.Fatalf("expected transition live runtime class, got %+v", status)
	}
	if status.LiveRuntimeProofState != "missing" {
		t.Fatalf("expected missing live proof state before evidence is loaded, got %+v", status)
	}
	if status.DashboardStoreMode != "postgres" {
		t.Fatalf("expected postgres dashboard mode, got %q", status.DashboardStoreMode)
	}
	if status.RankingStoreMode != "memory" {
		t.Fatalf("expected memory ranking mode, got %q", status.RankingStoreMode)
	}
	if status.CommentInsightStoreMode != "file" {
		t.Fatalf("expected file comment insight mode, got %q", status.CommentInsightStoreMode)
	}
	if status.ReportSummaryStoreMode != "postgres" {
		t.Fatalf("expected postgres report summary mode, got %q", status.ReportSummaryStoreMode)
	}
	if !status.PostgresConfigured || !status.RedisConfigured {
		t.Fatalf("expected postgres and redis to be configured, got %+v", status)
	}
	if status.PostgresReady || status.RedisReady {
		t.Fatalf("expected config-only status to stay unproven before runtime evidence, got %+v", status)
	}
	if !status.ReadCacheConfigured || status.ReadCacheActive {
		t.Fatalf("expected read cache to be configured but not yet active before runtime evidence, got %+v", status)
	}
	if len(status.ReadCacheScope) < 5 {
		t.Fatalf("expected read cache scope entries, got %+v", status.ReadCacheScope)
	}
	if status.OwnerBoundaryState != "met" {
		t.Fatalf("expected owner boundary state met, got %+v", status)
	}
	if status.OwnerBoundaryEvidence == "" || status.OwnerBoundarySummary == "" || status.OwnerBoundaryAction == "" {
		t.Fatalf("expected owner boundary evidence fields, got %+v", status)
	}
	if status.JobsEntrypointState != "available" || status.JobsRuntimeState != "ingest-and-read-model-ops-available" {
		t.Fatalf("unexpected jobs status: %+v", status)
	}
	if len(status.Notes) < 3 {
		t.Fatalf("expected explanatory notes, got %+v", status.Notes)
	}
	if status.LiveRuntimeSummary == "" || status.LiveRuntimeAction == "" {
		t.Fatalf("expected live runtime summary/action, got %+v", status)
	}
	if len(status.LiveRuntimeRequirements) != 4 {
		t.Fatalf("expected four live runtime requirements, got %+v", status.LiveRuntimeRequirements)
	}
}

func TestBuildStatusMarksTrustedBridgeAsRequiredWhenTeamPreviewLacksToken(t *testing.T) {
	cfg := Config{
		RuntimeProfile:      RuntimeProfileTeamPreview,
		Bridge:              BridgeConfig{DemoContextEnabled: false, RequireTrustedForLive: true},
		DashboardStore:      StoreConfig{Mode: StoreModePostgres},
		RankingStore:        StoreConfig{Mode: StoreModePostgres},
		CommentInsightStore: StoreConfig{Mode: StoreModePostgres},
		ReportSummaryStore:  StoreConfig{Mode: StoreModePostgres},
		WorkspaceStore:      StoreConfig{Mode: StoreModePostgres},
		IngestStore:         StoreConfig{Mode: StoreModePostgres},
		Postgres:            PostgresConfig{DSN: "postgres://demo"},
	}

	status := BuildStatus(cfg)
	if status.HubBridgeMode != "trusted-required" {
		t.Fatalf("expected trusted-required bridge mode, got %+v", status)
	}
	if status.LiveRuntimeClass != "transition" {
		t.Fatalf("expected transition live runtime class, got %+v", status)
	}
	if status.LiveRuntimeSummary == "" || status.LiveRuntimeAction == "" {
		t.Fatalf("expected live runtime summary/action, got %+v", status)
	}
	if len(status.LiveRuntimeRequirements) == 0 || status.LiveRuntimeRequirements[0].Key != "trusted-bridge" {
		t.Fatalf("expected trusted-bridge requirement first, got %+v", status.LiveRuntimeRequirements)
	}
	if status.LiveRuntimeRequirements[0].Status != "blocked" {
		t.Fatalf("expected trusted bridge to stay blocked without token, got %+v", status.LiveRuntimeRequirements[0])
	}
}

func TestBuildStatusExplainsMissingFormalInfraForTeamPreview(t *testing.T) {
	cfg := Config{
		RuntimeProfile:      RuntimeProfileTeamPreview,
		Bridge:              BridgeConfig{DemoContextEnabled: false, RequireTrustedForLive: true},
		DashboardStore:      StoreConfig{Mode: StoreModeFile},
		RankingStore:        StoreConfig{Mode: StoreModeFile},
		CommentInsightStore: StoreConfig{Mode: StoreModeFile},
		ReportSummaryStore:  StoreConfig{Mode: StoreModeFile},
		WorkspaceStore:      StoreConfig{Mode: StoreModeFile},
		IngestStore:         StoreConfig{Mode: StoreModeFile},
	}

	status := BuildStatus(cfg)
	var storeRequirement LiveRuntimeRequirement
	var cacheRequirement LiveRuntimeRequirement
	for _, item := range status.LiveRuntimeRequirements {
		if item.Key == "formal-stores" {
			storeRequirement = item
		}
		if item.Key == "redis-read-cache" {
			cacheRequirement = item
		}
	}
	if storeRequirement.Status != "blocked" || storeRequirement.Summary == "" {
		t.Fatalf("expected blocked formal stores requirement, got %+v", storeRequirement)
	}
	if cacheRequirement.Status != "blocked" || cacheRequirement.Summary == "" {
		t.Fatalf("expected blocked redis cache requirement, got %+v", cacheRequirement)
	}
	if want := "Postgres"; !containsText(storeRequirement.Summary, want) || !containsText(storeRequirement.Action, "INSIGHT_POSTGRES_DSN") {
		t.Fatalf("expected team-preview store requirement to mention Postgres setup, got %+v", storeRequirement)
	}
	if want := "Redis"; !containsText(cacheRequirement.Summary, want) || !containsText(cacheRequirement.Action, "INSIGHT_REDIS_ADDR") {
		t.Fatalf("expected team-preview cache requirement to mention Redis setup, got %+v", cacheRequirement)
	}
}

func TestBuildStatusKeepsFormalStoresBlockedWhenTeamPreviewUsesPostgresModesWithoutDSN(t *testing.T) {
	cfg := Config{
		RuntimeProfile:      RuntimeProfileTeamPreview,
		Bridge:              BridgeConfig{DemoContextEnabled: false, RequireTrustedForLive: true},
		DashboardStore:      StoreConfig{Mode: StoreModePostgres},
		RankingStore:        StoreConfig{Mode: StoreModePostgres},
		CommentInsightStore: StoreConfig{Mode: StoreModePostgres},
		ReportSummaryStore:  StoreConfig{Mode: StoreModePostgres},
		WorkspaceStore:      StoreConfig{Mode: StoreModePostgres},
		IngestStore:         StoreConfig{Mode: StoreModePostgres},
	}

	status := BuildStatus(cfg)
	var storeRequirement LiveRuntimeRequirement
	for _, item := range status.LiveRuntimeRequirements {
		if item.Key == "formal-stores" {
			storeRequirement = item
			break
		}
	}
	if storeRequirement.Status != "blocked" {
		t.Fatalf("expected blocked formal stores requirement without DSN, got %+v", storeRequirement)
	}
	if !containsText(storeRequirement.Summary, "Postgres 还没有配置") {
		t.Fatalf("expected summary to mention missing postgres config, got %+v", storeRequirement)
	}
	if !containsText(storeRequirement.Action, "INSIGHT_POSTGRES_DSN") {
		t.Fatalf("expected action to mention INSIGHT_POSTGRES_DSN, got %+v", storeRequirement)
	}
}

func TestBuildStatusKeepsFormalStoresBlockedWhenPostgresModesLackConfigOutsideTeamPreview(t *testing.T) {
	cfg := Config{
		DashboardStore:      StoreConfig{Mode: StoreModePostgres},
		RankingStore:        StoreConfig{Mode: StoreModePostgres},
		CommentInsightStore: StoreConfig{Mode: StoreModePostgres},
		ReportSummaryStore:  StoreConfig{Mode: StoreModePostgres},
		WorkspaceStore:      StoreConfig{Mode: StoreModePostgres},
		IngestStore:         StoreConfig{Mode: StoreModePostgres},
	}

	status := BuildStatus(cfg)
	var storeRequirement LiveRuntimeRequirement
	for _, item := range status.LiveRuntimeRequirements {
		if item.Key == "formal-stores" {
			storeRequirement = item
			break
		}
	}
	if storeRequirement.Status != "blocked" {
		t.Fatalf("expected blocked formal stores requirement without postgres config, got %+v", storeRequirement)
	}
	if !containsText(storeRequirement.Summary, "还没有 Postgres 配置") {
		t.Fatalf("expected summary to mention missing postgres config, got %+v", storeRequirement)
	}
	if !containsText(storeRequirement.Action, "INSIGHT_POSTGRES_DSN") {
		t.Fatalf("expected action to mention INSIGHT_POSTGRES_DSN, got %+v", storeRequirement)
	}
}

func containsText(haystack string, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestWithReadCacheSnapshotAppliesDynamicCounters(t *testing.T) {
	base := Status{
		HubBridgeMode:           "trusted-header",
		DemoContextEnabled:      false,
		DashboardStoreMode:      "postgres",
		RankingStoreMode:        "postgres",
		CommentInsightStoreMode: "postgres",
		ReportSummaryStoreMode:  "postgres",
		WorkspaceStoreMode:      "postgres",
		IngestStoreMode:         "postgres",
		PostgresConfigured:      true,
		PostgresReady:           true,
		RedisConfigured:         true,
		ReadCacheConfigured:     true,
		RedisReady:              true,
		ReadCacheScope:          []string{"dashboard", "hot_videos", "hot_topics", "report_summary"},
	}
	snapshot := insight.ReadCacheSnapshot{
		Enabled:          true,
		HitCount:         5,
		MissCount:        2,
		FallbackCount:    2,
		WriteCount:       3,
		InvalidateCount:  1,
		LastHitAt:        time.Date(2026, 7, 3, 14, 0, 0, 0, time.UTC),
		LastMissAt:       time.Date(2026, 7, 3, 14, 1, 0, 0, time.UTC),
		LastFallbackAt:   time.Date(2026, 7, 3, 14, 2, 0, 0, time.UTC),
		LastWriteAt:      time.Date(2026, 7, 3, 14, 3, 0, 0, time.UTC),
		LastInvalidateAt: time.Date(2026, 7, 3, 14, 4, 0, 0, time.UTC),
		ScopeStats: map[string]insight.ReadCacheScopeSnapshot{
			"dashboard": {
				HitCount:       3,
				MissCount:      1,
				FallbackCount:  1,
				WriteCount:     2,
				LastHitAt:      time.Date(2026, 7, 3, 14, 0, 0, 0, time.UTC),
				LastMissAt:     time.Date(2026, 7, 3, 14, 1, 0, 0, time.UTC),
				LastFallbackAt: time.Date(2026, 7, 3, 14, 2, 0, 0, time.UTC),
				LastWriteAt:    time.Date(2026, 7, 3, 14, 3, 0, 0, time.UTC),
			},
			"hot_videos": {
				HitCount:    2,
				WriteCount:  1,
				LastHitAt:   time.Date(2026, 7, 3, 14, 0, 30, 0, time.UTC),
				LastWriteAt: time.Date(2026, 7, 3, 14, 3, 30, 0, time.UTC),
			},
			"hot_topics": {
				MissCount:   1,
				WriteCount:  1,
				LastMissAt:  time.Date(2026, 7, 3, 14, 1, 30, 0, time.UTC),
				LastWriteAt: time.Date(2026, 7, 3, 14, 3, 45, 0, time.UTC),
			},
			"report_summary": {
				HitCount:    1,
				LastHitAt:   time.Date(2026, 7, 3, 14, 2, 30, 0, time.UTC),
				WriteCount:  1,
				LastWriteAt: time.Date(2026, 7, 3, 14, 3, 50, 0, time.UTC),
			},
		},
	}

	status := WithReadCacheSnapshot(base, snapshot)
	if !status.ReadCacheActive {
		t.Fatalf("expected read cache to stay active, got %+v", status)
	}
	if status.ReadCacheHitCount != 5 || status.ReadCacheMissCount != 2 {
		t.Fatalf("expected hit/miss counts to map, got %+v", status)
	}
	if status.ReadCacheFallbackCount != 2 || status.ReadCacheWriteCount != 3 {
		t.Fatalf("expected fallback/write counts to map, got %+v", status)
	}
	if status.ReadCacheInvalidateCount != 1 {
		t.Fatalf("expected invalidate count to map, got %+v", status)
	}
	if !status.ReadCacheLastFallbackAt.Equal(snapshot.LastFallbackAt) {
		t.Fatalf("expected fallback timestamp to map, got %+v", status)
	}
	if len(status.ReadCacheScopeStats) != 4 {
		t.Fatalf("expected scoped cache stats to map, got %+v", status.ReadCacheScopeStats)
	}
	if scope := status.ReadCacheScopeStats["dashboard"]; scope.HitCount != 3 || scope.FallbackCount != 1 {
		t.Fatalf("expected dashboard scoped stats to map, got %+v", scope)
	}
	if scope := status.ReadCacheScopeStats["hot_videos"]; scope.WriteCount != 1 {
		t.Fatalf("expected hot_videos scoped stats to map, got %+v", scope)
	}
	if scope := status.ReadCacheScopeStats["hot_topics"]; scope.MissCount != 1 {
		t.Fatalf("expected hot_topics scoped stats to map, got %+v", scope)
	}
	if scope := status.ReadCacheScopeStats["report_summary"]; scope.HitCount != 1 {
		t.Fatalf("expected report_summary scoped stats to map, got %+v", scope)
	}
	if status.LiveRuntimeClass != "formal-ready" {
		t.Fatalf("expected formal-ready live runtime class after scoped cache proof, got %+v", status)
	}
	if len(status.LiveRuntimeRequirements) != 4 {
		t.Fatalf("expected live runtime requirements after read cache snapshot, got %+v", status.LiveRuntimeRequirements)
	}
}

func TestWithReadCacheSnapshotKeepsCacheRequirementUnknownWithoutCoreScopeProof(t *testing.T) {
	base := Status{
		HubBridgeMode:           "trusted-header",
		DemoContextEnabled:      false,
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
		ReadCacheScope:          []string{"dashboard", "hot_videos", "hot_topics", "report_summary"},
	}
	snapshot := insight.ReadCacheSnapshot{
		Enabled:   true,
		HitCount:  2,
		MissCount: 1,
		ScopeStats: map[string]insight.ReadCacheScopeSnapshot{
			"dashboard": {
				HitCount:  2,
				LastHitAt: time.Date(2026, 7, 3, 14, 0, 0, 0, time.UTC),
			},
			"hot_videos": {
				WriteCount:  1,
				LastWriteAt: time.Date(2026, 7, 3, 14, 3, 30, 0, time.UTC),
			},
		},
	}

	status := WithReadCacheSnapshot(base, snapshot)
	if status.LiveRuntimeClass != "transition" {
		t.Fatalf("expected transition live runtime class without complete scoped proof, got %+v", status)
	}
	var cacheRequirement LiveRuntimeRequirement
	for _, item := range status.LiveRuntimeRequirements {
		if item.Key == "redis-read-cache" {
			cacheRequirement = item
			break
		}
	}
	if cacheRequirement.Status != "unknown" {
		t.Fatalf("expected unknown redis cache requirement without complete scoped proof, got %+v", cacheRequirement)
	}
	if !containsText(cacheRequirement.Summary, "hot_topics") || !containsText(cacheRequirement.Summary, "report_summary") {
		t.Fatalf("expected missing scopes in cache summary, got %+v", cacheRequirement)
	}
}

func TestWithWXChannelCapabilitiesBlocksFormalReadyWhenRequiredRoutesAreMissing(t *testing.T) {
	base := Status{
		HubBridgeMode:           "local-operator",
		DemoContextEnabled:      false,
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
		ReadCacheScope:          []string{"dashboard", "hot_videos", "hot_topics", "report_summary"},
		ReadCacheScopeStats: map[string]insight.ReadCacheScopeSnapshot{
			"dashboard":      {HitCount: 3},
			"hot_videos":     {WriteCount: 1},
			"hot_topics":     {MissCount: 1},
			"report_summary": {HitCount: 1},
		},
	}

	status := WithWXChannelCapabilities(base, WXChannelCapabilityStatus{
		BaseURL: "http://127.0.0.1:2026",
		Checked: true,
		Ready:   false,
		Missing: []string{"feedSearch", "commentExport"},
		Routes: []WXChannelCapabilityRoute{
			{Name: "feedSearch", URL: "http://127.0.0.1:2026/api/channels/feed/search?keyword=__insight_capability_probe__", StatusCode: 404, RoutePresent: false},
			{Name: "commentExport", URL: "http://127.0.0.1:2026/api/channels/feed/comment/export", StatusCode: 404, RoutePresent: false},
		},
	})

	if status.LiveRuntimeClass == "formal-ready" {
		t.Fatalf("expected incompatible wx_channel data plane to block formal-ready, got %+v", status)
	}
	var dataPlaneRequirement LiveRuntimeRequirement
	for _, item := range status.LiveRuntimeRequirements {
		if item.Key == "wx-channel-data-plane" {
			dataPlaneRequirement = item
			break
		}
	}
	if dataPlaneRequirement.Status != "blocked" {
		t.Fatalf("expected blocked wx_channel data plane requirement, got %+v", status.LiveRuntimeRequirements)
	}
	if !strings.Contains(dataPlaneRequirement.Summary, "feedSearch") || !strings.Contains(dataPlaneRequirement.Summary, "commentExport") {
		t.Fatalf("expected missing capability names in summary, got %+v", dataPlaneRequirement)
	}
	if !strings.Contains(dataPlaneRequirement.Action, "switch-insight-data-plane-admin.cmd") {
		t.Fatalf("expected admin switch action, got %+v", dataPlaneRequirement)
	}
}

func TestWithInfrastructureAvailabilityMapsLiveInfraEvidence(t *testing.T) {
	base := Status{
		PostgresConfigured: true,
		RedisConfigured:    true,
	}
	updated := WithInfrastructureAvailability(base, true, false)
	if !updated.PostgresReady || updated.RedisReady {
		t.Fatalf("expected postgres ready only, got %+v", updated)
	}
	if updated.LiveRuntimeClass != "transition" {
		t.Fatalf("expected transition live runtime class after infra evidence, got %+v", updated)
	}
}

func TestWithJobsRuntimeStateAppliesLatestOperatorStatus(t *testing.T) {
	base := Status{
		JobsRuntimeState: "ingest-and-read-model-ops-available",
	}
	updated := WithJobsRuntimeState(base, JobsRuntimeState{
		LastMode:           "operator-loop",
		LastStatus:         "failed",
		LastHubUserID:      "hub:user:alice",
		LastFinishedAt:     time.Date(2026, 7, 3, 15, 0, 0, 0, time.UTC),
		LastIteration:      1,
		LastIterations:     3,
		LastInterval:       "5m0s",
		LastError:          "wx_feed_list timeout",
		LastRequestedCount: 2,
		LastRequestedSources: []string{
			"wx_comment_export",
			"wx_feed_list",
		},
		UpdatedAt: time.Date(2026, 7, 3, 15, 0, 1, 0, time.UTC),
	})

	if updated.JobsRuntimeState != "recent-failed" {
		t.Fatalf("expected recent-failed jobs runtime state, got %+v", updated)
	}
	if updated.JobsLastMode != "operator-loop" || updated.JobsLastError != "wx_feed_list timeout" {
		t.Fatalf("expected jobs runtime fields to map, got %+v", updated)
	}
	if len(updated.JobsLastRequestedSources) != 2 || updated.JobsLastRequestedSources[0] != "wx_comment_export" || updated.JobsLastRequestedSources[1] != "wx_feed_list" {
		t.Fatalf("expected requested sources to map, got %+v", updated)
	}
}

func TestWithTrustedRuntimeEvidenceKeepsHistoricalProofSeparatedFromCurrentDemoLive(t *testing.T) {
	status := Status{
		HubBridgeMode:           "demo-context",
		DemoContextEnabled:      true,
		DashboardStoreMode:      "file",
		RankingStoreMode:        "file",
		CommentInsightStoreMode: "file",
		ReportSummaryStoreMode:  "file",
		WorkspaceStoreMode:      "file",
		IngestStoreMode:         "file",
	}

	status = WithTrustedRuntimeEvidence(status, TrustedRuntimeEvidence{
		State:                  "verified",
		VerifiedAt:             time.Date(2026, 7, 3, 14, 55, 7, 0, time.UTC),
		Source:                 "verify-insight-docker-stack",
		HubUserID:              "hub:user:proof",
		APIURL:                 "http://127.0.0.1:18161",
		WebURL:                 "http://127.0.0.1:5261",
		TrustedBoundaryMode:    "trusted-only",
		APITrustedContextState: "verified",
		APIBrowserQueryState:   "blocked",
		WebTrustedContextState: "verified",
		WebBrowserQueryState:   "isolated",
	})

	if status.LiveRuntimeClass != "prototype-compatible" {
		t.Fatalf("expected prototype-compatible live runtime, got %+v", status)
	}
	if status.LiveRuntimeProofState != "historical" {
		t.Fatalf("expected historical proof state, got %+v", status)
	}
	if len(status.LiveRuntimeRequirements) != 4 || status.LiveRuntimeRequirements[3].Status != "unknown" {
		t.Fatalf("expected proof requirement to stay unresolved, got %+v", status.LiveRuntimeRequirements)
	}
}

func TestWithTrustedRuntimeEvidenceDowngradesIncompleteVerifiedProof(t *testing.T) {
	status := Status{
		HubBridgeMode:           "trusted-header",
		DemoContextEnabled:      false,
		DashboardStoreMode:      "postgres",
		RankingStoreMode:        "postgres",
		CommentInsightStoreMode: "postgres",
		ReportSummaryStoreMode:  "postgres",
		WorkspaceStoreMode:      "postgres",
		IngestStoreMode:         "postgres",
	}

	status = WithTrustedRuntimeEvidence(status, TrustedRuntimeEvidence{
		State:                  "verified",
		TrustedBoundaryMode:    "trusted-only",
		APITrustedContextState: "verified",
		APIBrowserQueryState:   "blocked",
		WebTrustedContextState: "verified",
		WebBrowserQueryState:   "isolated",
	})

	if status.TrustedRuntimeEvidence.State != "partial" {
		t.Fatalf("expected partial trusted runtime evidence, got %+v", status.TrustedRuntimeEvidence)
	}
	if status.LiveRuntimeProofState != "partial" {
		t.Fatalf("expected partial live runtime proof state, got %+v", status)
	}
	if !containsText(status.TrustedRuntimeEvidence.Summary, "verifiedAt 缺失") ||
		!containsText(status.TrustedRuntimeEvidence.Summary, "source 缺失") ||
		!containsText(status.TrustedRuntimeEvidence.Summary, "hubUserId 缺失") {
		t.Fatalf("expected summary to mention missing verified proof metadata, got %+v", status.TrustedRuntimeEvidence)
	}
	if len(status.LiveRuntimeRequirements) != 4 || status.LiveRuntimeRequirements[3].Status != "unknown" {
		t.Fatalf("expected proof requirement to stay unresolved, got %+v", status.LiveRuntimeRequirements)
	}
}
