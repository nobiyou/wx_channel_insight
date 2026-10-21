package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultConfigFile = "insight.local.json"

// configField is the single mapping from the public JSON schema to the existing
// environment interface. Values are validated before the legacy default builder.
type configField struct{ env, kind string }

var configFields = map[string]configField{
	"server.addr":                     {"INSIGHT_API_ADDR", "string"},
	"server.web_static_root":          {"INSIGHT_WEB_STATIC_ROOT", "path"},
	"server.cors_origins":             {"INSIGHT_CORS_ORIGINS", "list"},
	"runtime.profile":                 {"INSIGHT_RUNTIME_PROFILE", "string"},
	"runtime.default_hub_user_id":     {"INSIGHT_DEFAULT_HUB_USER_ID", "string"},
	"runtime.default_device_count":    {"INSIGHT_DEFAULT_DEVICE_COUNT", "int"},
	"runtime.jobs_state_path":         {"INSIGHT_JOBS_STATE_PATH", "path"},
	"storage.driver":                  {"INSIGHT_STORE", "driver"},
	"storage.data_dir":                {"INSIGHT_DATA_DIR", "path"},
	"storage.sqlite_path":             {"INSIGHT_SQLITE_PATH", "path"},
	"storage.postgres_dsn":            {"INSIGHT_POSTGRES_DSN", "string"},
	"redis.addr":                      {"INSIGHT_REDIS_ADDR", "string"},
	"redis.password":                  {"INSIGHT_REDIS_PASSWORD", "string"},
	"redis.db":                        {"INSIGHT_REDIS_DB", "int"},
	"wx_channel.base_url":             {"INSIGHT_WX_CHANNEL_BASE_URL", "string"},
	"wx_channel.comment_data_root":    {"INSIGHT_WX_COMMENT_DATA_ROOT", "path"},
	"bridge.trusted_token":            {"INSIGHT_TRUSTED_BRIDGE_TOKEN", "string"},
	"bridge.hub_base_url":             {"INSIGHT_HUB_BASE_URL", "string"},
	"bridge.local_operator":           {"INSIGHT_LOCAL_OPERATOR", "bool"},
	"bridge.demo_context_enabled":     {"INSIGHT_DEMO_CONTEXT_ENABLED", "bool"},
	"bridge.require_trusted_for_live": {"INSIGHT_TRUSTED_BRIDGE_REQUIRED", "bool"},
	"edge.enabled":                    {"INSIGHT_EDGE_WS_ENABLED", "bool"},
	"edge.secret":                     {"INSIGHT_EDGE_WS_SECRET", "string"},
	"openai.api_key":                  {"OPENAI_API_KEY", "string"},
	"openai.base_url":                 {"OPENAI_BASE_URL", "string"},
	"openai.model":                    {"OPENAI_MODEL", "string"},
	"openai.timeout_seconds":          {"OPENAI_TIMEOUT_SECONDS", "int"},
	"ai_config_encryption_key":        {"INSIGHT_AI_CONFIG_ENCRYPTION_KEY", "string"},
	"scheduler.api_base_url":          {"INSIGHT_SCHEDULER_API_BASE_URL", "string"},
	"scheduler.poll_interval":         {"INSIGHT_SCHEDULER_POLL_INTERVAL", "string"},
	"scheduler.request_timeout":       {"INSIGHT_SCHEDULER_REQUEST_TIMEOUT", "string"},
	"scheduler.run_on_start":          {"INSIGHT_SCHEDULER_RUN_ON_START", "bool"},
	"scheduler.strict":                {"INSIGHT_SCHEDULER_STRICT", "bool"},
}

var storeEnvNames = map[string]string{
	"dashboard": "DASHBOARD", "ranking": "RANKING", "recommendation": "RECOMMENDATION",
	"comment_insight": "COMMENT_INSIGHT", "data_center_exclusion": "DATA_CENTER_EXCLUSION",
	"report_summary": "REPORT_SUMMARY", "workspace": "WORKSPACE", "ingest": "INGEST",
	"monitoring": "MONITORING", "prompt": "PROMPT", "ai_service": "AI_CONFIG", "ai_analysis": "AI_ANALYSIS",
}

func init() {
	for name, env := range storeEnvNames {
		configFields["stores."+name+".driver"] = configField{"INSIGHT_" + env + "_STORE", "driver"}
		configFields["stores."+name+".path"] = configField{"INSIGHT_" + env + "_STORE_PATH", "path"}
	}
}

func LoadConfig() (Config, error) {
	lookup, err := LoadConfigLookup()
	if err != nil {
		return Config{}, err
	}
	return configFromValidatedLookup(lookup)
}

// LoadConfigLookup shares file/env precedence with the HTTP scheduler without
// making the scheduler own database settings or opening any storage connections.
func LoadConfigLookup() (func(string) string, error) {
	path := strings.TrimSpace(os.Getenv("INSIGHT_CONFIG_FILE"))
	if path == "" {
		path = DefaultConfigFile
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			path = ""
		}
	}
	return configLookup(path, os.LookupEnv)
}

func LoadConfigFile(path string, lookup func(string) (string, bool)) (Config, error) {
	merged, err := configLookup(path, lookup)
	if err != nil {
		return Config{}, err
	}
	return configFromValidatedLookup(merged)
}

func configLookup(path string, lookup func(string) (string, bool)) (func(string) string, error) {
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}
	values := map[string]string{}
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open configuration %q: %w", path, err)
		}
		defer file.Close()
		const maxConfigBytes = 1 << 20
		limited := &io.LimitedReader{R: file, N: maxConfigBytes + 1}
		dec := json.NewDecoder(limited)
		if err := decodeConfigObject(dec, "", values); err != nil {
			return nil, fmt.Errorf("configuration %q: %w", path, err)
		}
		if _, err := dec.Token(); err != io.EOF {
			return nil, fmt.Errorf("configuration %q must contain one JSON object", path)
		}
		if limited.N <= 0 {
			return nil, errors.New("configuration exceeds 1 MiB")
		}
		base, err := filepath.Abs(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		for _, field := range configFields {
			if v := values[field.env]; field.kind == "path" && v != "" && !filepath.IsAbs(v) {
				values[field.env] = filepath.Join(base, v)
			}
		}
		if _, ok := values["INSIGHT_DATA_DIR"]; !ok {
			values["INSIGHT_DATA_DIR"] = filepath.Join(base, "var")
		}
	}
	return func(key string) string {
		if value, ok := lookup(key); ok {
			return value
		}
		return values[key]
	}, nil
}

func decodeConfigObject(dec *json.Decoder, prefix string, values map[string]string) error {
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("expected a JSON object")
	}
	seen := map[string]bool{}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return errors.New("invalid JSON configuration")
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("expected a configuration field")
		}
		path := prefix + key
		if seen[key] {
			return fmt.Errorf("duplicate configuration field %q", path)
		}
		seen[key] = true
		field, leaf := configFields[path]
		if !leaf {
			known := false
			for candidate := range configFields {
				if strings.HasPrefix(candidate, path+".") {
					known = true
					break
				}
			}
			if !known {
				return fmt.Errorf("unknown configuration field %q", path)
			}
			if err := decodeConfigObject(dec, path+".", values); err != nil {
				return err
			}
			continue
		}
		var raw json.RawMessage
		if dec.Decode(&raw) != nil {
			return fmt.Errorf("invalid JSON for %s", path)
		}
		if string(raw) == "null" {
			return fmt.Errorf("%s cannot be null", path)
		}
		var value string
		switch field.kind {
		case "bool":
			var parsed bool
			if json.Unmarshal(raw, &parsed) != nil {
				return fmt.Errorf("%s must be a boolean", path)
			}
			value = strconv.FormatBool(parsed)
		case "int":
			var parsed int
			if json.Unmarshal(raw, &parsed) != nil {
				return fmt.Errorf("%s must be an integer", path)
			}
			value = strconv.Itoa(parsed)
		case "list":
			var parsed []string
			if json.Unmarshal(raw, &parsed) != nil {
				return fmt.Errorf("%s must be an array of strings", path)
			}
			value = strings.Join(parsed, ",")
		default:
			if json.Unmarshal(raw, &value) != nil {
				return fmt.Errorf("%s must be a string", path)
			}
		}
		values[field.env] = value
	}
	if _, err := dec.Token(); err != nil {
		return errors.New("unterminated configuration object")
	}
	return nil
}

func configFromValidatedLookup(lookup func(string) string) (Config, error) {
	for _, field := range configFields {
		value := strings.TrimSpace(lookup(field.env))
		if value == "" {
			continue
		}
		switch field.kind {
		case "driver":
			switch StoreMode(strings.ToLower(value)) {
			case StoreModeFile, StoreModeMemory, StoreModeSQLite, StoreModePostgres:
			default:
				return Config{}, fmt.Errorf("%s must be file, memory, sqlite or postgres", field.env)
			}
		case "int":
			v, err := strconv.Atoi(value)
			if err != nil || v < 0 || field.env == "OPENAI_TIMEOUT_SECONDS" && v == 0 {
				return Config{}, fmt.Errorf("%s must be a valid non-negative integer (timeout must be positive)", field.env)
			}
		case "bool":
			switch strings.ToLower(value) {
			case "1", "0", "true", "false", "yes", "no", "on", "off":
			default:
				return Config{}, fmt.Errorf("%s must be a boolean", field.env)
			}
		}
	}
	if profile := strings.TrimSpace(lookup("INSIGHT_RUNTIME_PROFILE")); profile != "" && strings.ToLower(profile) != RuntimeProfileTeamPreview {
		return Config{}, errors.New("INSIGHT_RUNTIME_PROFILE must be empty or team-preview")
	}
	cfg := LoadConfigFromLookup(lookup)
	for _, store := range configuredStores(cfg) {
		if store.Mode == StoreModePostgres && !cfg.Postgres.Enabled() {
			return Config{}, errors.New("postgres storage requires INSIGHT_POSTGRES_DSN or storage.postgres_dsn")
		}
	}
	return cfg, nil
}

func configuredStores(cfg Config) []StoreConfig {
	return []StoreConfig{cfg.DashboardStore, cfg.RankingStore, cfg.RecommendationStore, cfg.CommentInsightStore, cfg.DataCenterExclusionStore, cfg.ReportSummaryStore, cfg.WorkspaceStore, cfg.IngestStore, cfg.MonitoringStore, cfg.PromptStore, cfg.AIServiceStore, cfg.AIAnalysisStore}
}
