package scheduler

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	runtimecfg "github.com/nobiyou/wx_channel_insight/internal/runtime"
)

const (
	defaultAPIBaseURL     = "http://127.0.0.1:18081"
	defaultInterval       = 15 * time.Second
	defaultRequestTimeout = 10 * time.Minute
	defaultOperatorUserID = "insight:operator:local"
	defaultDeviceCount    = 1
)

// Config contains the scheduler-owned settings. APIBaseURL and RequestTimeout
// are reserved for the API-backed CycleRunner assembled by the application.
type Config struct {
	APIBaseURL string
	// Interval is the API poll interval. Task cadence belongs to Insight task rows.
	Interval       time.Duration
	RequestTimeout time.Duration
	OperatorUserID string
	DeviceCount    int
	BridgeToken    string
	Strict         bool
	RunOnStart     bool
}

func LoadConfig() (Config, error) {
	lookup, err := runtimecfg.LoadConfigLookup()
	if err != nil {
		return Config{}, err
	}
	return LoadConfigFromLookup(lookup)
}

// LoadConfigFromLookup reads scheduler settings without falling back after a
// scheduler-specific value has been provided but cannot be parsed.
func LoadConfigFromLookup(lookup func(string) string) (Config, error) {
	if lookup == nil {
		lookup = func(string) string { return "" }
	}

	apiBaseURL := firstConfigured(lookup, "INSIGHT_SCHEDULER_API_BASE_URL", "INSIGHT_API_BASE_URL")
	if apiBaseURL == "" {
		apiBaseURL = defaultAPIBaseURL
	}

	intervalKey := "INSIGHT_SCHEDULER_POLL_INTERVAL"
	interval, err := durationValue(lookup, intervalKey, defaultInterval)
	if strings.TrimSpace(lookup(intervalKey)) == "" && strings.TrimSpace(lookup("INSIGHT_SCHEDULER_INTERVAL")) != "" {
		intervalKey = "INSIGHT_SCHEDULER_INTERVAL"
		interval, err = durationValue(lookup, intervalKey, defaultInterval)
	}
	if err != nil {
		return Config{}, err
	}
	requestTimeout, err := durationValue(lookup, "INSIGHT_SCHEDULER_REQUEST_TIMEOUT", defaultRequestTimeout)
	if err != nil {
		return Config{}, err
	}
	operatorUserID := firstConfigured(lookup, "INSIGHT_SCHEDULER_HUB_USER_ID", "INSIGHT_DEFAULT_HUB_USER_ID")
	if operatorUserID == "" {
		operatorUserID = defaultOperatorUserID
	}
	deviceCount, err := intValue(lookup, "INSIGHT_SCHEDULER_DEVICE_COUNT", "INSIGHT_DEFAULT_DEVICE_COUNT", defaultDeviceCount)
	if err != nil {
		return Config{}, err
	}
	bridgeToken := firstConfigured(lookup, "INSIGHT_SCHEDULER_BRIDGE_TOKEN", "INSIGHT_TRUSTED_BRIDGE_TOKEN")
	strict, err := boolValue(lookup, "INSIGHT_SCHEDULER_STRICT", true)
	if err != nil {
		return Config{}, err
	}
	runOnStart, err := boolValue(lookup, "INSIGHT_SCHEDULER_RUN_ON_START", true)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		APIBaseURL:     apiBaseURL,
		Interval:       interval,
		RequestTimeout: requestTimeout,
		OperatorUserID: operatorUserID,
		DeviceCount:    deviceCount,
		BridgeToken:    bridgeToken,
		Strict:         strict,
		RunOnStart:     runOnStart,
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	parsedURL, err := url.Parse(strings.TrimSpace(c.APIBaseURL))
	if err != nil {
		return fmt.Errorf("scheduler api base URL %q must include a scheme and host: %w", c.APIBaseURL, err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("scheduler api base URL %q must include a scheme and host", c.APIBaseURL)
	}
	if c.Interval <= 0 {
		return fmt.Errorf("scheduler interval must be positive, got %s", c.Interval)
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("scheduler request timeout must be positive, got %s", c.RequestTimeout)
	}
	if c.DeviceCount < 0 {
		return fmt.Errorf("scheduler device count must be non-negative, got %d", c.DeviceCount)
	}
	return nil
}

func firstConfigured(lookup func(string) string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(lookup(key)); value != "" {
			return value
		}
	}
	return ""
}

func durationValue(lookup func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(lookup(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: parse duration: %w", key, value, err)
	}
	return parsed, nil
}

func intValue(lookup func(string) string, primaryKey, fallbackKey string, fallback int) (int, error) {
	key := primaryKey
	value := strings.TrimSpace(lookup(primaryKey))
	if value == "" {
		key = fallbackKey
		value = strings.TrimSpace(lookup(fallbackKey))
	}
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: parse integer: %w", key, value, err)
	}
	return parsed, nil
}

func boolValue(lookup func(string) string, key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(lookup(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid %s %q: parse boolean: %w", key, value, err)
	}
	return parsed, nil
}
