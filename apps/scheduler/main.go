package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/nobiyou/wx_channel_insight/internal/scheduler"
)

const (
	monitoringTickPath = "/api/v1/monitoring/scheduler/tick"
	// operatorCyclePath remains an internal test/compatibility name; the
	// scheduler now always resolves it to the monitoring tick endpoint.
	operatorCyclePath  = monitoringTickPath
	maxResponseBytes   = 1 << 20
	maxResponseSummary = 512
)

type httpCycleRunner struct {
	client         *http.Client
	baseURL        string
	operatorUserID string
	deviceCount    int
	bridgeToken    string
	strict         bool
}

type tickResponse struct {
	Status string `json:"status"`
	Run    struct {
		Status string `json:"status"`
	} `json:"run"`
	Error string `json:"error"`
}

func main() {
	cfg, err := scheduler.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, log.Default()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cfg scheduler.Config, logger *log.Logger) error {
	if logger == nil {
		logger = log.Default()
	}
	client := &http.Client{Timeout: cfg.RequestTimeout}
	runner := newHTTPCycleRunner(cfg, client)
	logger.Printf(
		"scheduler starting api=%s poll_interval=%s request_timeout=%s operator=%s device_count=%d run_on_start=%t",
		cfg.APIBaseURL,
		cfg.Interval,
		cfg.RequestTimeout,
		cfg.OperatorUserID,
		cfg.DeviceCount,
		cfg.RunOnStart,
	)

	service := scheduler.NewService(runner, cfg, scheduler.WithLogger(logger))
	if err := service.Run(ctx); err != nil {
		return fmt.Errorf("scheduler stopped with error: %w", err)
	}
	logger.Printf("scheduler stopped")
	return nil
}

func newHTTPCycleRunner(cfg scheduler.Config, client *http.Client) *httpCycleRunner {
	if client == nil {
		client = &http.Client{Timeout: cfg.RequestTimeout}
	}
	return &httpCycleRunner{
		client:         client,
		baseURL:        cfg.APIBaseURL,
		operatorUserID: cfg.OperatorUserID,
		deviceCount:    cfg.DeviceCount,
		bridgeToken:    cfg.BridgeToken,
		strict:         cfg.Strict,
	}
}

func (r *httpCycleRunner) RunCycle(ctx context.Context) error {
	if r == nil {
		return errors.New("scheduler HTTP tick runner is nil")
	}
	if ctx == nil {
		return errors.New("scheduler HTTP tick context is nil")
	}
	if r.client == nil {
		return errors.New("scheduler HTTP client is nil")
	}

	endpoint, err := cycleEndpoint(r.baseURL)
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct{}{})
	if err != nil {
		return fmt.Errorf("marshal monitoring tick request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create monitoring tick request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if operatorUserID := strings.TrimSpace(r.operatorUserID); operatorUserID != "" {
		req.Header.Set("X-Hub-User-ID", operatorUserID)
	}
	if r.deviceCount >= 0 {
		req.Header.Set("X-Hub-Device-Count", strconv.Itoa(r.deviceCount))
	}
	if bridgeToken := strings.TrimSpace(r.bridgeToken); bridgeToken != "" {
		req.Header.Set("X-Insight-Bridge-Token", bridgeToken)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("monitoring tick request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := readResponseBody(resp.Body)
	if err != nil {
		return fmt.Errorf("read monitoring tick response (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("monitoring tick returned HTTP %d: %s", resp.StatusCode, summarizeResponse(responseBody))
	}

	var payload tickResponse
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return fmt.Errorf("decode monitoring tick response: %w", err)
	}
	switch payload.Status {
	case "idle":
		return nil
	case "completed":
		if payload.Error != "" {
			return fmt.Errorf("monitoring tick completed with task error: %s", payload.Error)
		}
		return nil
	default:
		return fmt.Errorf("monitoring tick returned status=%q: %s", payload.Status, summarizeResponse(responseBody))
	}
}

func cycleEndpoint(baseURL string) (string, error) {
	trimmed := strings.TrimSpace(baseURL)
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("scheduler api base URL %q: %w", baseURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("scheduler api base URL %q must include a scheme and host", baseURL)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + operatorCyclePath
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func readResponseBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxResponseBytes)
	}
	return body, nil
}

func summarizeResponse(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "<empty response>"
	}
	if len(text) > maxResponseSummary {
		return text[:maxResponseSummary] + "..."
	}
	return text
}
