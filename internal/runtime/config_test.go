package runtime

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigFromLookupUsesDefaults(t *testing.T) {
	cfg := LoadConfigFromLookup(func(string) string { return "" })

	if cfg.APIAddr != ":18081" {
		t.Fatalf("expected default api addr, got %q", cfg.APIAddr)
	}
	if cfg.WebStaticRoot != "" {
		t.Fatalf("expected empty default web static root, got %q", cfg.WebStaticRoot)
	}
	if cfg.RuntimeProfile != "" {
		t.Fatalf("expected default runtime profile empty, got %q", cfg.RuntimeProfile)
	}
	if cfg.DefaultHubUserID != "hub:user:demo" {
		t.Fatalf("expected default hub user id, got %q", cfg.DefaultHubUserID)
	}
	if cfg.DefaultDeviceCount != 6 {
		t.Fatalf("expected default device count 6, got %d", cfg.DefaultDeviceCount)
	}
	if cfg.WXChannelBaseURL != "" {
		t.Fatalf("expected empty default wx channel base url, got %q", cfg.WXChannelBaseURL)
	}
	if cfg.DashboardStore.Mode != StoreModeFile {
		t.Fatalf("expected default dashboard store file, got %q", cfg.DashboardStore.Mode)
	}
	if cfg.RankingStore.Mode != StoreModeFile {
		t.Fatalf("expected default ranking store file, got %q", cfg.RankingStore.Mode)
	}
	if cfg.CommentInsightStore.Mode != StoreModeFile {
		t.Fatalf("expected default comment insight store file, got %q", cfg.CommentInsightStore.Mode)
	}
	if cfg.ReportSummaryStore.Mode != StoreModeFile {
		t.Fatalf("expected default report summary store file, got %q", cfg.ReportSummaryStore.Mode)
	}
	if cfg.JobsStatePath == "" {
		t.Fatalf("expected default jobs state path, got empty value")
	}
	if cfg.PromptStore.Mode != StoreModeFile || cfg.PromptStore.Path != filepath.Join("var", "prompt-settings.json") {
		t.Fatalf("expected default prompt store, got %+v", cfg.PromptStore)
	}
	if cfg.AIServiceStore.Mode != StoreModeFile || cfg.AIServiceStore.Path != filepath.Join("var", "ai-service-settings.json") {
		t.Fatalf("expected default AI service store, got store=%+v", cfg.AIServiceStore)
	}
	if cfg.AIAnalysisStore.Mode != StoreModeFile || cfg.AIAnalysisStore.Path != filepath.Join("var", "ai-analysis-runs.json") {
		t.Fatalf("expected default AI analysis store, got store=%+v", cfg.AIAnalysisStore)
	}
	if cfg.Postgres.Enabled() {
		t.Fatalf("expected postgres disabled by default")
	}
	if cfg.Redis.Enabled() {
		t.Fatalf("expected redis disabled by default")
	}
	if cfg.OpenAI.Model != "gpt-5.6" || cfg.OpenAI.Enabled() {
		t.Fatalf("expected OpenAI to default to gpt-5.6 without a key, got %+v", cfg.OpenAI)
	}
	if cfg.Bridge.TrustedModeEnabled() {
		t.Fatalf("expected trusted bridge disabled by default")
	}
	if !cfg.Bridge.DemoContextEnabled {
		t.Fatalf("expected demo context enabled by default")
	}
	if cfg.Bridge.RequireTrustedForLive {
		t.Fatalf("expected trusted bridge not required by default")
	}
}

func TestLoadConfigFromLookupReadsConfiguredModes(t *testing.T) {
	values := map[string]string{
		"INSIGHT_API_ADDR":                   ":19081",
		"INSIGHT_WEB_STATIC_ROOT":            "C:\\Insight\\web",
		"INSIGHT_RUNTIME_PROFILE":            "team-preview",
		"INSIGHT_DEFAULT_HUB_USER_ID":        "hub:user:alice",
		"INSIGHT_DEFAULT_DEVICE_COUNT":       "12",
		"INSIGHT_WX_CHANNEL_BASE_URL":        "http://127.0.0.1:2026",
		"INSIGHT_WX_CHANNEL_TOKEN":           "local-secret",
		"INSIGHT_WX_CHANNEL_DOWNLOAD_ROOT":   "/mnt/wx_channel/downloads",
		"INSIGHT_DASHBOARD_STORE":            "postgres",
		"INSIGHT_RANKING_STORE":              "postgres",
		"INSIGHT_COMMENT_INSIGHT_STORE":      "memory",
		"INSIGHT_REPORT_SUMMARY_STORE":       "memory",
		"INSIGHT_WORKSPACE_STORE":            "memory",
		"INSIGHT_INGEST_STORE":               "postgres",
		"INSIGHT_DASHBOARD_STORE_PATH":       "custom-dashboard.json",
		"INSIGHT_RANKING_STORE_PATH":         "custom-rankings.json",
		"INSIGHT_COMMENT_INSIGHT_STORE_PATH": "custom-comment-insights.json",
		"INSIGHT_REPORT_SUMMARY_STORE_PATH":  "custom-report-summary.json",
		"INSIGHT_WORKSPACE_STORE_PATH":       "custom-workspace.json",
		"INSIGHT_INGEST_STORE_PATH":          "custom-ingest.json",
		"INSIGHT_JOBS_STATE_PATH":            "custom-jobs-state.json",
		"INSIGHT_AI_CONFIG_STORE":            "memory",
		"INSIGHT_AI_CONFIG_STORE_PATH":       "custom-ai-service.json",
		"INSIGHT_AI_ANALYSIS_STORE":          "file",
		"INSIGHT_AI_ANALYSIS_STORE_PATH":     "custom-ai-analysis.json",
		"INSIGHT_POSTGRES_DSN":               "postgres://demo",
		"INSIGHT_REDIS_ADDR":                 "127.0.0.1:6379",
		"INSIGHT_REDIS_PASSWORD":             "secret",
		"INSIGHT_REDIS_DB":                   "4",
		"INSIGHT_TRUSTED_BRIDGE_TOKEN":       "bridge-secret",
		"INSIGHT_DEMO_CONTEXT_ENABLED":       "false",
		"OPENAI_API_KEY":                     "openai-test-key",
		"OPENAI_BASE_URL":                    "http://127.0.0.1:9000/v1",
		"OPENAI_MODEL":                       "gpt-5.6",
		"OPENAI_TIMEOUT_SECONDS":             "12",
	}

	cfg := LoadConfigFromLookup(func(key string) string {
		return values[key]
	})

	if cfg.APIAddr != ":19081" {
		t.Fatalf("expected configured api addr, got %q", cfg.APIAddr)
	}
	if cfg.WebStaticRoot != "C:\\Insight\\web" {
		t.Fatalf("expected configured web static root, got %q", cfg.WebStaticRoot)
	}
	if cfg.RuntimeProfile != RuntimeProfileTeamPreview {
		t.Fatalf("expected team-preview runtime profile, got %q", cfg.RuntimeProfile)
	}
	if cfg.DefaultHubUserID != "hub:user:alice" {
		t.Fatalf("expected configured hub user id, got %q", cfg.DefaultHubUserID)
	}
	if cfg.DefaultDeviceCount != 12 {
		t.Fatalf("expected configured device count, got %d", cfg.DefaultDeviceCount)
	}
	if cfg.WXChannelBaseURL != "http://127.0.0.1:2026" {
		t.Fatalf("expected configured wx channel base url, got %q", cfg.WXChannelBaseURL)
	}
	if cfg.DashboardStore.Mode != StoreModePostgres {
		t.Fatalf("expected postgres dashboard mode, got %q", cfg.DashboardStore.Mode)
	}
	if cfg.RankingStore.Mode != StoreModePostgres {
		t.Fatalf("expected postgres ranking mode, got %q", cfg.RankingStore.Mode)
	}
	if cfg.CommentInsightStore.Mode != StoreModeMemory {
		t.Fatalf("expected memory comment insight mode, got %q", cfg.CommentInsightStore.Mode)
	}
	if cfg.ReportSummaryStore.Mode != StoreModeMemory {
		t.Fatalf("expected memory report summary mode, got %q", cfg.ReportSummaryStore.Mode)
	}
	if cfg.WorkspaceStore.Mode != StoreModeMemory {
		t.Fatalf("expected memory workspace mode, got %q", cfg.WorkspaceStore.Mode)
	}
	if cfg.IngestStore.Mode != StoreModePostgres {
		t.Fatalf("expected postgres ingest mode, got %q", cfg.IngestStore.Mode)
	}
	if cfg.JobsStatePath != "custom-jobs-state.json" {
		t.Fatalf("expected configured jobs state path, got %q", cfg.JobsStatePath)
	}
	if cfg.AIServiceStore.Mode != StoreModeMemory || cfg.AIServiceStore.Path != "custom-ai-service.json" {
		t.Fatalf("expected configured AI service store, got store=%+v", cfg.AIServiceStore)
	}
	if cfg.AIAnalysisStore.Mode != StoreModeFile || cfg.AIAnalysisStore.Path != "custom-ai-analysis.json" {
		t.Fatalf("expected configured AI analysis store, got store=%+v", cfg.AIAnalysisStore)
	}
	if !cfg.Postgres.Enabled() || cfg.Postgres.DSN != "postgres://demo" {
		t.Fatalf("expected configured postgres dsn, got %+v", cfg.Postgres)
	}
	if !cfg.Redis.Enabled() || cfg.Redis.Addr != "127.0.0.1:6379" || cfg.Redis.DB != 4 {
		t.Fatalf("expected configured redis values, got %+v", cfg.Redis)
	}
	if !cfg.OpenAI.Enabled() || cfg.OpenAI.APIKey != "openai-test-key" || cfg.OpenAI.BaseURL != "http://127.0.0.1:9000/v1" || cfg.OpenAI.Model != "gpt-5.6" || cfg.OpenAI.Timeout != 12*time.Second {
		t.Fatalf("expected configured OpenAI values, got %+v", cfg.OpenAI)
	}
	if !cfg.Bridge.TrustedModeEnabled() || cfg.Bridge.TrustedToken != "bridge-secret" {
		t.Fatalf("expected configured bridge token, got %+v", cfg.Bridge)
	}
	if cfg.Bridge.DemoContextEnabled {
		t.Fatalf("expected demo context disabled, got %+v", cfg.Bridge)
	}
	if !cfg.Bridge.RequireTrustedForLive {
		t.Fatalf("expected trusted bridge to be required for team-preview, got %+v", cfg.Bridge)
	}
}

func TestLoadConfigFromLookupDisablesDemoContextByDefaultWhenTrustedBridgeIsConfigured(t *testing.T) {
	cfg := LoadConfigFromLookup(func(key string) string {
		if key == "INSIGHT_TRUSTED_BRIDGE_TOKEN" {
			return "bridge-secret"
		}
		return ""
	})

	if !cfg.Bridge.TrustedModeEnabled() {
		t.Fatalf("expected trusted bridge enabled, got %+v", cfg.Bridge)
	}
	if cfg.Bridge.DemoContextEnabled {
		t.Fatalf("expected demo context disabled by derived default, got %+v", cfg.Bridge)
	}
	if cfg.Bridge.RequireTrustedForLive {
		t.Fatalf("expected trusted bridge requirement to stay off without team-preview profile, got %+v", cfg.Bridge)
	}
}

func TestLoadConfigFromLookupUsesTeamPreviewPostgresDefaultsWhenStoreModesAreOmitted(t *testing.T) {
	values := map[string]string{
		"INSIGHT_RUNTIME_PROFILE":      "team-preview",
		"INSIGHT_POSTGRES_DSN":         "postgres://team-preview",
		"INSIGHT_TRUSTED_BRIDGE_TOKEN": "bridge-secret",
	}

	cfg := LoadConfigFromLookup(func(key string) string {
		return values[key]
	})

	if cfg.RuntimeProfile != RuntimeProfileTeamPreview {
		t.Fatalf("expected team-preview runtime profile, got %q", cfg.RuntimeProfile)
	}
	if cfg.WXChannelBaseURL != defaultTeamPreviewWXChannelBaseURL {
		t.Fatalf("expected team-preview to default wx_channel base url, got %q", cfg.WXChannelBaseURL)
	}
	if cfg.DashboardStore.Mode != StoreModePostgres ||
		cfg.RankingStore.Mode != StoreModePostgres ||
		cfg.CommentInsightStore.Mode != StoreModePostgres ||
		cfg.ReportSummaryStore.Mode != StoreModePostgres ||
		cfg.WorkspaceStore.Mode != StoreModePostgres ||
		cfg.IngestStore.Mode != StoreModePostgres ||
		cfg.AIServiceStore.Mode != StoreModePostgres ||
		cfg.AIAnalysisStore.Mode != StoreModePostgres {
		t.Fatalf("expected team-preview profile to default stores to postgres, got %+v", cfg)
	}
	if cfg.Bridge.DemoContextEnabled {
		t.Fatalf("expected demo context disabled when trusted bridge token is configured, got %+v", cfg.Bridge)
	}
	if !cfg.Bridge.RequireTrustedForLive {
		t.Fatalf("expected team-preview to require trusted bridge, got %+v", cfg.Bridge)
	}
}

func TestLoadConfigFromLookupRequiresExplicitTrustedBridgeForTeamPreview(t *testing.T) {
	values := map[string]string{
		"INSIGHT_RUNTIME_PROFILE": "team-preview",
	}

	cfg := LoadConfigFromLookup(func(key string) string {
		return values[key]
	})

	if cfg.RuntimeProfile != RuntimeProfileTeamPreview {
		t.Fatalf("expected team-preview runtime profile, got %q", cfg.RuntimeProfile)
	}
	if strings.TrimSpace(cfg.Bridge.TrustedToken) != "" {
		t.Fatalf("expected trusted bridge token to stay empty, got %+v", cfg.Bridge)
	}
	if cfg.Bridge.LocalOperatorEnabled() {
		t.Fatalf("expected local operator disabled without explicit opt-in, got %+v", cfg.Bridge)
	}
	if cfg.Bridge.TrustedModeEnabled() {
		t.Fatalf("expected trusted mode disabled without token, Hub or explicit local operator, got %+v", cfg.Bridge)
	}
	if cfg.DefaultHubUserID != "insight:operator:local" {
		t.Fatalf("expected default operator id insight:operator:local, got %q", cfg.DefaultHubUserID)
	}
	if cfg.WXChannelBaseURL != "http://127.0.0.1:2026" {
		t.Fatalf("expected default wx_channel :2026, got %q", cfg.WXChannelBaseURL)
	}
	if cfg.Bridge.DemoContextEnabled {
		t.Fatalf("expected team-preview to disable demo context by default, got %+v", cfg.Bridge)
	}
	if !cfg.Bridge.RequireTrustedForLive {
		t.Fatalf("expected team-preview to require trusted live boundary, got %+v", cfg.Bridge)
	}
}

func TestLoadConfigFromLookupParsesOnlyAbsoluteCORSOrigins(t *testing.T) {
	cfg := LoadConfigFromLookup(func(key string) string {
		if key == "INSIGHT_CORS_ORIGINS" {
			return "http://127.0.0.1:5176/, https://insight.example, *, javascript:bad, https://insight.example/path"
		}
		return ""
	})
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[0] != "http://127.0.0.1:5176" || cfg.CORSOrigins[1] != "https://insight.example" {
		t.Fatalf("unexpected CORS origins: %#v", cfg.CORSOrigins)
	}
}

func TestLoadConfigFromLookupAllowsExplicitDemoOverrideForTeamPreview(t *testing.T) {
	values := map[string]string{
		"INSIGHT_RUNTIME_PROFILE":         "team-preview",
		"INSIGHT_DEMO_CONTEXT_ENABLED":    "true",
		"INSIGHT_TRUSTED_BRIDGE_REQUIRED": "false",
	}

	cfg := LoadConfigFromLookup(func(key string) string {
		return values[key]
	})

	if !cfg.Bridge.DemoContextEnabled {
		t.Fatalf("expected explicit demo override to be honored, got %+v", cfg.Bridge)
	}
	if cfg.Bridge.RequireTrustedForLive {
		t.Fatalf("expected explicit trusted bridge requirement override to be honored, got %+v", cfg.Bridge)
	}
}

func TestLoadConfigHubBaseURLEnablesSessionBridge(t *testing.T) {
	cfg := LoadConfigFromLookup(func(key string) string {
		if key == "INSIGHT_HUB_BASE_URL" {
			return "http://127.0.0.1:8080/"
		}
		return ""
	})
	if !cfg.Bridge.HubSessionEnabled() {
		t.Fatalf("expected hub session enabled")
	}
	if cfg.Bridge.HubBaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("expected trimmed hub base url, got %q", cfg.Bridge.HubBaseURL)
	}
	if !cfg.Bridge.TrustedModeEnabled() {
		t.Fatalf("hub base url alone should count as trusted mode")
	}
}
