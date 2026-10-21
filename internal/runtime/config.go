package runtime

import (
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/sqlite"
)

type StoreMode string

const (
	StoreModeMemory                    StoreMode = "memory"
	StoreModeFile                      StoreMode = "file"
	StoreModePostgres                  StoreMode = "postgres"
	StoreModeSQLite                    StoreMode = "sqlite"
	RuntimeProfileTeamPreview                    = "team-preview"
	defaultTeamPreviewWXChannelBaseURL           = "http://127.0.0.1:2026"
)

type StoreConfig struct {
	Mode StoreMode
	Path string
}

type PostgresConfig struct {
	DSN string
}

type SQLiteConfig struct {
	Path string
}

func (c PostgresConfig) Enabled() bool {
	return strings.TrimSpace(c.DSN) != ""
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

func (c RedisConfig) Enabled() bool {
	return strings.TrimSpace(c.Addr) != ""
}

type BridgeConfig struct {
	TrustedToken          string
	DemoContextEnabled    bool
	RequireTrustedForLive bool
	// HubBaseURL is optional legacy Hub JWT resolution. Team edition hub-less mode leaves this empty.
	HubBaseURL string
	// LocalOperator enables Insight-owned operator identity without Hub (default for team-preview).
	LocalOperator bool
}

// EdgeConfig is the multi-LAN-client gateway hosted by Insight (replaces Hub /ws/client role).
type EdgeConfig struct {
	// Enabled accepts LAN wx_channel connectors on /ws/client. Default true for team-preview.
	Enabled bool
	// Secret optional shared secret checked as X-Cloud-Secret (same as wx_channel cloud_secret).
	Secret string
}

type OpenAIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

func (c OpenAIConfig) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != ""
}

func (c BridgeConfig) TrustedModeEnabled() bool {
	return strings.TrimSpace(c.TrustedToken) != "" || strings.TrimSpace(c.HubBaseURL) != "" || c.LocalOperator
}

func (c BridgeConfig) HubSessionEnabled() bool {
	return strings.TrimSpace(c.HubBaseURL) != ""
}

func (c BridgeConfig) LocalOperatorEnabled() bool {
	return c.LocalOperator
}

type Config struct {
	APIAddr                  string
	WebStaticRoot            string
	RuntimeProfile           string
	DefaultHubUserID         string
	DefaultDeviceCount       int
	WXChannelBaseURL         string
	WXCommentDataRoot        string
	DashboardStore           StoreConfig
	RankingStore             StoreConfig
	RecommendationStore      StoreConfig
	CommentInsightStore      StoreConfig
	DataCenterExclusionStore StoreConfig
	ReportSummaryStore       StoreConfig
	WorkspaceStore           StoreConfig
	IngestStore              StoreConfig
	MonitoringStore          StoreConfig
	PromptStore              StoreConfig
	AIServiceStore           StoreConfig
	AIAnalysisStore          StoreConfig
	JobsStatePath            string
	Postgres                 PostgresConfig
	SQLite                   SQLiteConfig
	sqliteClient             *sqlite.Client
	Redis                    RedisConfig
	OpenAI                   OpenAIConfig
	Bridge                   BridgeConfig
	Edge                     EdgeConfig
	CORSOrigins              []string
	AIConfigEncryptionKey    string
}

func LoadConfigFromLookup(lookup func(string) string) Config {
	if lookup == nil {
		lookup = func(string) string { return "" }
	}
	runtimeProfile := normalizeRuntimeProfile(lookup("INSIGHT_RUNTIME_PROFILE"))
	trustedBridgeToken := strings.TrimSpace(lookup("INSIGHT_TRUSTED_BRIDGE_TOKEN"))
	hubBaseURL := strings.TrimRight(strings.TrimSpace(lookup("INSIGHT_HUB_BASE_URL")), "/")
	// A live team runtime must use a shared bridge token or Hub session. Local
	// operator identity remains available only when explicitly enabled for a
	// development instance; it must never be an implicit production fallback.
	localOperatorDefault := false
	requireTrustedForLiveDefault := runtimeProfile == RuntimeProfileTeamPreview
	demoContextEnabledDefault := true
	if requireTrustedForLiveDefault || trustedBridgeToken != "" || localOperatorDefault {
		demoContextEnabledDefault = false
	}
	defaultOperatorID := "hub:user:demo"
	if runtimeProfile == RuntimeProfileTeamPreview {
		defaultOperatorID = "insight:operator:local"
	}
	postgresCfg := PostgresConfig{
		DSN: strings.TrimSpace(lookup("INSIGHT_POSTGRES_DSN")),
	}
	redisCfg := RedisConfig{
		Addr:     strings.TrimSpace(lookup("INSIGHT_REDIS_ADDR")),
		Password: lookup("INSIGHT_REDIS_PASSWORD"),
		DB:       envIntOrDefault(lookup, "INSIGHT_REDIS_DB", 0),
	}
	openAITimeoutSeconds := envIntOrDefault(lookup, "OPENAI_TIMEOUT_SECONDS", 45)
	if openAITimeoutSeconds <= 0 {
		openAITimeoutSeconds = 45
	}
	defaultStoreMode := envOrDefault(lookup, "INSIGHT_STORE", defaultStoreModeForProfile(runtimeProfile, postgresCfg.Enabled()))
	dataDir := envOrDefault(lookup, "INSIGHT_DATA_DIR", "var")

	return Config{
		APIAddr:            envOrDefault(lookup, "INSIGHT_API_ADDR", ":18081"),
		WebStaticRoot:      strings.TrimSpace(lookup("INSIGHT_WEB_STATIC_ROOT")),
		RuntimeProfile:     runtimeProfile,
		DefaultHubUserID:   envOrDefault(lookup, "INSIGHT_DEFAULT_HUB_USER_ID", defaultOperatorID),
		DefaultDeviceCount: envIntOrDefault(lookup, "INSIGHT_DEFAULT_DEVICE_COUNT", 6),
		WXChannelBaseURL:   resolveWXChannelBaseURL(runtimeProfile, lookup("INSIGHT_WX_CHANNEL_BASE_URL")),
		WXCommentDataRoot:  strings.TrimSpace(lookup("INSIGHT_WX_COMMENT_DATA_ROOT")),
		DashboardStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_DASHBOARD_STORE", defaultStoreMode)),
			Path: envOrDefault(lookup, "INSIGHT_DASHBOARD_STORE_PATH", filepath.Join(dataDir, "dashboard-snapshot.json")),
		},
		RankingStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_RANKING_STORE", envOrDefault(lookup, "INSIGHT_DASHBOARD_STORE", defaultStoreMode))),
			Path: envOrDefault(lookup, "INSIGHT_RANKING_STORE_PATH", filepath.Join(dataDir, "rankings.json")),
		},
		RecommendationStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_RECOMMENDATION_STORE", envOrDefault(lookup, "INSIGHT_DASHBOARD_STORE", defaultStoreMode))),
			Path: envOrDefault(lookup, "INSIGHT_RECOMMENDATION_STORE_PATH", filepath.Join(dataDir, "recommendations.json")),
		},
		CommentInsightStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_COMMENT_INSIGHT_STORE", envOrDefault(lookup, "INSIGHT_DASHBOARD_STORE", defaultStoreMode))),
			Path: envOrDefault(lookup, "INSIGHT_COMMENT_INSIGHT_STORE_PATH", filepath.Join(dataDir, "comment-insights.json")),
		},
		DataCenterExclusionStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_DATA_CENTER_EXCLUSION_STORE", envOrDefault(lookup, "INSIGHT_DASHBOARD_STORE", defaultStoreMode))),
			Path: envOrDefault(lookup, "INSIGHT_DATA_CENTER_EXCLUSION_STORE_PATH", filepath.Join(dataDir, "data-center-exclusions.json")),
		},
		ReportSummaryStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_REPORT_SUMMARY_STORE", envOrDefault(lookup, "INSIGHT_DASHBOARD_STORE", defaultStoreMode))),
			Path: envOrDefault(lookup, "INSIGHT_REPORT_SUMMARY_STORE_PATH", filepath.Join(dataDir, "report-summary.json")),
		},
		WorkspaceStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_WORKSPACE_STORE", defaultStoreMode)),
			Path: envOrDefault(lookup, "INSIGHT_WORKSPACE_STORE_PATH", filepath.Join(dataDir, "workspace-state.json")),
		},
		IngestStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_INGEST_STORE", defaultStoreMode)),
			Path: envOrDefault(lookup, "INSIGHT_INGEST_STORE_PATH", filepath.Join(dataDir, "ingest-state.json")),
		},
		MonitoringStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_MONITORING_STORE", defaultStoreMode)),
			Path: envOrDefault(lookup, "INSIGHT_MONITORING_STORE_PATH", filepath.Join(dataDir, "monitoring-tasks.json")),
		},
		PromptStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_PROMPT_STORE", envOrDefault(lookup, "INSIGHT_DASHBOARD_STORE", defaultStoreMode))),
			Path: envOrDefault(lookup, "INSIGHT_PROMPT_STORE_PATH", filepath.Join(dataDir, "prompt-settings.json")),
		},
		AIServiceStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_AI_CONFIG_STORE", envOrDefault(lookup, "INSIGHT_DASHBOARD_STORE", defaultStoreMode))),
			Path: envOrDefault(lookup, "INSIGHT_AI_CONFIG_STORE_PATH", filepath.Join(dataDir, "ai-service-settings.json")),
		},
		AIAnalysisStore: StoreConfig{
			Mode: normalizeStoreMode(envOrDefault(lookup, "INSIGHT_AI_ANALYSIS_STORE", envOrDefault(lookup, "INSIGHT_DASHBOARD_STORE", defaultStoreMode))),
			Path: envOrDefault(lookup, "INSIGHT_AI_ANALYSIS_STORE_PATH", filepath.Join(dataDir, "ai-analysis-runs.json")),
		},
		JobsStatePath: envOrDefault(lookup, "INSIGHT_JOBS_STATE_PATH", filepath.Join(dataDir, "jobs-runtime-state.json")),
		Postgres:      postgresCfg,
		SQLite:        SQLiteConfig{Path: envOrDefault(lookup, "INSIGHT_SQLITE_PATH", filepath.Join(dataDir, "insight.db"))},
		Redis:         redisCfg,
		OpenAI: OpenAIConfig{
			APIKey:  strings.TrimSpace(lookup("OPENAI_API_KEY")),
			BaseURL: strings.TrimRight(strings.TrimSpace(lookup("OPENAI_BASE_URL")), "/"),
			Model:   envOrDefault(lookup, "OPENAI_MODEL", "gpt-5.6"),
			Timeout: time.Duration(openAITimeoutSeconds) * time.Second,
		},
		Bridge: BridgeConfig{
			TrustedToken:          trustedBridgeToken,
			DemoContextEnabled:    envBoolOrDefault(lookup, "INSIGHT_DEMO_CONTEXT_ENABLED", demoContextEnabledDefault),
			RequireTrustedForLive: envBoolOrDefault(lookup, "INSIGHT_TRUSTED_BRIDGE_REQUIRED", requireTrustedForLiveDefault),
			HubBaseURL:            hubBaseURL,
			LocalOperator:         envBoolOrDefault(lookup, "INSIGHT_LOCAL_OPERATOR", localOperatorDefault),
		},
		Edge: EdgeConfig{
			Enabled: envBoolOrDefault(lookup, "INSIGHT_EDGE_WS_ENABLED", true),
			Secret:  strings.TrimSpace(lookup("INSIGHT_EDGE_WS_SECRET")),
		},
		CORSOrigins:           parseCORSOrigins(lookup("INSIGHT_CORS_ORIGINS")),
		AIConfigEncryptionKey: strings.TrimSpace(lookup("INSIGHT_AI_CONFIG_ENCRYPTION_KEY")),
	}
}

func parseCSV(raw string) []string {
	items := make([]string, 0)
	seen := make(map[string]struct{})
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}
	return items
}

func parseCORSOrigins(raw string) []string {
	origins := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimRight(strings.TrimSpace(item), "/")
		if origin == "" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			continue
		}
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins
}

func envOrDefault(lookup func(string) string, key, fallback string) string {
	if value := strings.TrimSpace(lookup(key)); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(lookup func(string) string, key string, fallback int) int {
	value := strings.TrimSpace(lookup(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBoolOrDefault(lookup func(string) string, key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(lookup(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func normalizeStoreMode(value string) StoreMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(StoreModeMemory):
		return StoreModeMemory
	case string(StoreModePostgres):
		return StoreModePostgres
	case string(StoreModeSQLite):
		return StoreModeSQLite
	default:
		return StoreModeFile
	}
}

func normalizeRuntimeProfile(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case RuntimeProfileTeamPreview:
		return RuntimeProfileTeamPreview
	default:
		return ""
	}
}

func defaultStoreModeForProfile(profile string, postgresEnabled bool) string {
	if profile == RuntimeProfileTeamPreview && postgresEnabled {
		return string(StoreModePostgres)
	}
	return string(StoreModeFile)
}

func resolveWXChannelBaseURL(profile, value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	if profile == RuntimeProfileTeamPreview {
		return defaultTeamPreviewWXChannelBaseURL
	}
	return ""
}
