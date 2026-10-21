package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "insight.local.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigFileStorageAndEnvironmentPrecedence(t *testing.T) {
	path := writeConfig(t, `{"server":{"addr":"127.0.0.1:19081"},"storage":{"driver":"sqlite","sqlite_path":"data/insight.db"},"bridge":{"local_operator":false},"redis":{"db":2}}`)
	env := map[string]string{"INSIGHT_API_ADDR": "127.0.0.1:19082", "INSIGHT_REDIS_DB": "0", "INSIGHT_LOCAL_OPERATOR": "true"}
	cfg, err := LoadConfigFile(path, func(key string) (string, bool) { v, ok := env[key]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIAddr != "127.0.0.1:19082" || cfg.Redis.DB != 0 || !cfg.Bridge.LocalOperator {
		t.Fatalf("environment overrides were not applied")
	}
	for _, store := range configuredStores(cfg) {
		if store.Mode != StoreModeSQLite {
			t.Fatalf("global driver did not reach every store: %+v", store)
		}
	}
	if cfg.SQLite.Path != filepath.Join(filepath.Dir(path), "data", "insight.db") {
		t.Fatalf("SQLite path must be relative to config: %s", cfg.SQLite.Path)
	}
}

func TestConfigFileEmptyEnvironmentClearsFileSecret(t *testing.T) {
	path := writeConfig(t, `{"openai":{"api_key":"file-only-test-value"},"redis":{"password":"file-secret"}}`)
	cfg, err := LoadConfigFile(path, func(key string) (string, bool) { return "", key == "OPENAI_API_KEY" || key == "INSIGHT_REDIS_PASSWORD" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenAI.Enabled() || cfg.Redis.Password != "" {
		t.Fatal("explicit empty environment must override file secrets")
	}
}

func TestConfigFileRejectsInvalidInputWithoutEchoingSecrets(t *testing.T) {
	for name, body := range map[string]string{
		"unknown field":        `{"storage":{"drivre":"sqlite"}}`,
		"invalid driver":       `{"storage":{"driver":"sqilte"}}`,
		"missing postgres dsn": `{"storage":{"driver":"postgres"}}`,
		"invalid bool":         `{"bridge":{"local_operator":"false"}}`,
		"duplicate key":        `{"storage":{"driver":"sqlite","driver":"file"}}`,
		"trailing document":    `{} {}`,
		"null root":            `null`,
		"wrong secret type":    `{"openai":{"api_key":{"private-test-secret":true}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadConfigFile(writeConfig(t, body), func(string) (string, bool) { return "", false })
			if err == nil {
				t.Fatal("expected configuration error")
			}
			if strings.Contains(err.Error(), "private-test-secret") {
				t.Fatal("configuration error leaked a secret")
			}
		})
	}
}

func TestConfigFileRejectsInvalidEnvironmentDriver(t *testing.T) {
	_, err := LoadConfigFile("", func(key string) (string, bool) { return "sqilte", key == "INSIGHT_STORE" })
	if err == nil {
		t.Fatal("invalid environment driver must not silently select file storage")
	}
}

func TestConfigFileKeepsExistingDefaultsWithoutFile(t *testing.T) {
	cfg, err := LoadConfigFile("", func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DashboardStore.Mode != StoreModeFile || cfg.Bridge.LocalOperator {
		t.Fatal("legacy defaults changed")
	}
	if _, err := LoadConfigFile(filepath.Join(t.TempDir(), "missing.json"), nil); err == nil {
		t.Fatal("an explicit missing file must fail")
	}
}

func TestConfigFilePostgresAndPerStoreOverrides(t *testing.T) {
	path := writeConfig(t, `{"storage":{"driver":"postgres","postgres_dsn":"postgres://example.invalid/insight"},"stores":{"prompt":{"driver":"memory"}},"server":{"cors_origins":["http://localhost:5176"]}}`)
	cfg, err := LoadConfigFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PromptStore.Mode != StoreModeMemory || cfg.IngestStore.Mode != StoreModePostgres || len(cfg.CORSOrigins) != 1 {
		t.Fatalf("per-store settings not honored")
	}
}
