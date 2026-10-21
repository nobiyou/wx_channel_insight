package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigSharesFileAndBridgeWithAPI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "insight.local.json")
	body := `{"runtime":{"default_hub_user_id":"file-operator","default_device_count":3},"bridge":{"trusted_token":"file-token"},"scheduler":{"api_base_url":"http://127.0.0.1:19083","poll_interval":"30s","run_on_start":false}}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INSIGHT_CONFIG_FILE", path)
	t.Setenv("INSIGHT_SCHEDULER_REQUEST_TIMEOUT", "2m")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIBaseURL != "http://127.0.0.1:19083" || cfg.Interval != 30*time.Second || cfg.RequestTimeout != 2*time.Minute || cfg.OperatorUserID != "file-operator" || cfg.DeviceCount != 3 || cfg.BridgeToken != "file-token" || cfg.RunOnStart {
		t.Fatal("scheduler did not share configuration file/env precedence")
	}
}
