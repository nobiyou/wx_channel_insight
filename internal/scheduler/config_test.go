package scheduler

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigFromLookupUsesDefaults(t *testing.T) {
	cfg, err := LoadConfigFromLookup(func(string) string { return "" })
	if err != nil {
		t.Fatalf("LoadConfigFromLookup returned error: %v", err)
	}

	if cfg.APIBaseURL != defaultAPIBaseURL {
		t.Fatalf("expected default API base URL %q, got %q", defaultAPIBaseURL, cfg.APIBaseURL)
	}
	if cfg.Interval != 15*time.Second {
		t.Fatalf("expected fifteen-second poll interval, got %s", cfg.Interval)
	}
	if cfg.RequestTimeout != 10*time.Minute {
		t.Fatalf("expected ten-minute request timeout, got %s", cfg.RequestTimeout)
	}
	if cfg.OperatorUserID != defaultOperatorUserID {
		t.Fatalf("expected default operator user ID %q, got %q", defaultOperatorUserID, cfg.OperatorUserID)
	}
	if cfg.DeviceCount != 1 || cfg.BridgeToken != "" || !cfg.Strict || !cfg.RunOnStart {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadConfigFromLookupUsesSchedulerPrecedenceAndParsesValues(t *testing.T) {
	values := map[string]string{
		"INSIGHT_API_BASE_URL":              "http://fallback.example",
		"INSIGHT_SCHEDULER_API_BASE_URL":    "https://scheduler.example/api/",
		"INSIGHT_SCHEDULER_POLL_INTERVAL":   "30s",
		"INSIGHT_SCHEDULER_REQUEST_TIMEOUT": "2m",
		"INSIGHT_SCHEDULER_HUB_USER_ID":     "insight:operator:test",
		"INSIGHT_DEFAULT_HUB_USER_ID":       "insight:operator:default",
		"INSIGHT_SCHEDULER_DEVICE_COUNT":    "4",
		"INSIGHT_DEFAULT_DEVICE_COUNT":      "8",
		"INSIGHT_SCHEDULER_BRIDGE_TOKEN":    "scheduler-token",
		"INSIGHT_TRUSTED_BRIDGE_TOKEN":      "trusted-token",
		"INSIGHT_SCHEDULER_STRICT":          "false",
		"INSIGHT_SCHEDULER_RUN_ON_START":    "0",
	}
	cfg, err := LoadConfigFromLookup(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("LoadConfigFromLookup returned error: %v", err)
	}

	if cfg.APIBaseURL != values["INSIGHT_SCHEDULER_API_BASE_URL"] || cfg.Interval != 30*time.Second || cfg.RequestTimeout != 2*time.Minute {
		t.Fatalf("unexpected scheduler timing configuration: %+v", cfg)
	}
	if cfg.OperatorUserID != values["INSIGHT_SCHEDULER_HUB_USER_ID"] || cfg.DeviceCount != 4 || cfg.BridgeToken != values["INSIGHT_SCHEDULER_BRIDGE_TOKEN"] || cfg.Strict || cfg.RunOnStart {
		t.Fatalf("unexpected configured values: %+v", cfg)
	}
}

func TestLoadConfigFromLookupUsesFallbackPrecedence(t *testing.T) {
	values := map[string]string{
		"INSIGHT_DEFAULT_HUB_USER_ID":  "insight:operator:fallback",
		"INSIGHT_DEFAULT_DEVICE_COUNT": "3",
		"INSIGHT_TRUSTED_BRIDGE_TOKEN": "trusted-token",
	}
	cfg, err := LoadConfigFromLookup(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("LoadConfigFromLookup returned error: %v", err)
	}
	if cfg.OperatorUserID != values["INSIGHT_DEFAULT_HUB_USER_ID"] || cfg.DeviceCount != 3 || cfg.BridgeToken != values["INSIGHT_TRUSTED_BRIDGE_TOKEN"] {
		t.Fatalf("unexpected fallback values: %+v", cfg)
	}
}

func TestLoadConfigFromLookupRejectsInvalidValuesWithContext(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "url", key: "INSIGHT_SCHEDULER_API_BASE_URL", value: "/relative", want: "scheme and host"},
		{name: "poll interval", key: "INSIGHT_SCHEDULER_POLL_INTERVAL", value: "not-a-duration", want: "INSIGHT_SCHEDULER_POLL_INTERVAL"},
		{name: "timeout", key: "INSIGHT_SCHEDULER_REQUEST_TIMEOUT", value: "0s", want: "scheduler request timeout"},
		{name: "device count", key: "INSIGHT_SCHEDULER_DEVICE_COUNT", value: "-1", want: "scheduler device count"},
		{name: "strict", key: "INSIGHT_SCHEDULER_STRICT", value: "sometimes", want: "INSIGHT_SCHEDULER_STRICT"},
		{name: "run on start", key: "INSIGHT_SCHEDULER_RUN_ON_START", value: "sometimes", want: "INSIGHT_SCHEDULER_RUN_ON_START"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadConfigFromLookup(func(key string) string {
				if key == tt.key {
					return tt.value
				}
				return ""
			})
			if err == nil {
				t.Fatalf("expected error, got config %+v", cfg)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error to contain %q, got %q", tt.want, err)
			}
		})
	}
}

func TestConfigValidateRejectsMissingURLPartsAndNonPositiveValues(t *testing.T) {
	base := Config{
		APIBaseURL:     "http://127.0.0.1:18081",
		Interval:       time.Second,
		RequestTimeout: time.Second,
		DeviceCount:    0,
	}
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "missing scheme", cfg: func() Config { c := base; c.APIBaseURL = "127.0.0.1:18081"; return c }(), want: "scheme and host"},
		{name: "missing host", cfg: func() Config { c := base; c.APIBaseURL = "http://"; return c }(), want: "scheme and host"},
		{name: "zero interval", cfg: func() Config { c := base; c.Interval = 0; return c }(), want: "interval must be positive"},
		{name: "negative timeout", cfg: func() Config { c := base; c.RequestTimeout = -time.Second; return c }(), want: "request timeout must be positive"},
		{name: "negative device count", cfg: func() Config { c := base; c.DeviceCount = -1; return c }(), want: "device count must be non-negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected validation error containing %q, got %v", tt.want, err)
			}
		})
	}
}
