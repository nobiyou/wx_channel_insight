package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	operatorflow "github.com/nobiyou/wx_channel_insight/internal/operator"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	runtimecfg "github.com/nobiyou/wx_channel_insight/internal/runtime"
)

type syncSavedBindingsOptions struct {
	hubUserID        string
	deviceCount      int
	requestedSources map[string]struct{}
}

type operatorCycleOptions struct {
	syncSavedBindingsOptions
	strict      bool
	triggerMode string
}

type operatorLoopOptions struct {
	operatorCycleOptions
	interval   time.Duration
	iterations int
}

type operatorCommandRunner struct {
	runtimePack     *runtimecfg.DashboardRuntime
	operatorService *operatorflow.Service
	cfg             runtimecfg.Config
}

var operatorLoopSleep = func(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func newOperatorCommandRunner(cfg runtimecfg.Config) (*operatorCommandRunner, error) {
	runtimePack, err := runtimecfg.BuildDashboardRuntime(cfg)
	if err != nil {
		return nil, err
	}
	attachJobsEdgeFeedTransport(cfg, runtimePack.IngestService)
	if runtimePack.IngestService == nil {
		runtimePack.Close()
		return nil, fmt.Errorf("ingest service is not configured")
	}
	if runtimePack.ReadModelService == nil {
		runtimePack.Close()
		return nil, fmt.Errorf("read model refresh service is not configured")
	}
	operatorService, err := operatorflow.NewService(runtimePack.IngestService, runtimePack.ReadModelService)
	if err != nil {
		runtimePack.Close()
		return nil, err
	}
	accountMonitor, err := operatorflow.NewAccountMonitor(runtimePack.IngestService, runtimePack.AccountService)
	if err != nil {
		runtimePack.Close()
		return nil, err
	}
	operatorService.SetAccountMonitor(accountMonitor)
	if runtimePack.PostgresClient != nil {
		cycleLocker, err := postgres.NewAdvisoryCycleLocker(runtimePack.PostgresClient)
		if err != nil {
			runtimePack.Close()
			return nil, err
		}
		operatorService.SetCycleLocker(cycleLocker)
	}
	return &operatorCommandRunner{
		runtimePack:     runtimePack,
		operatorService: operatorService,
		cfg:             cfg,
	}, nil
}

func (runner *operatorCommandRunner) Close() {
	if runner != nil && runner.runtimePack != nil {
		runner.runtimePack.Close()
	}
}

func (runner *operatorCommandRunner) RunCycle(ctx context.Context, options operatorCycleOptions) (map[string]any, error) {
	triggerMode := strings.TrimSpace(options.triggerMode)
	if triggerMode == "" {
		triggerMode = ingest.OperatorTriggerCycle
	}
	result, cycleErr := runner.operatorService.RunCycle(ctx, operatorflow.Options{
		HubUserID:        options.hubUserID,
		DeviceCount:      options.deviceCount,
		RequestedSources: options.requestedSources,
		Strict:           options.strict,
		TriggerMode:      triggerMode,
	})
	var executionErr operatorflow.ExecutionError
	if cycleErr != nil && !errors.As(cycleErr, &executionErr) {
		return nil, cycleErr
	}

	payload := operatorflow.BuildRunPayload(result, currentJobsRuntimeStatus(runner.runtimePack))
	if cycleErr != nil {
		payload["error"] = cycleErr.Error()
	}
	if cycleErr != nil && options.strict {
		return payload, silentExitError{err: cycleErr}
	}
	return payload, nil
}

func runSyncSavedBindings(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseSyncSavedBindingsArgs(args)
	if err != nil {
		return err
	}
	runner, err := newOperatorCommandRunner(cfg)
	if err != nil {
		return err
	}
	defer runner.Close()

	result, err := runner.operatorService.SyncSavedBindings(context.Background(), operatorflow.Options{
		HubUserID:        options.hubUserID,
		DeviceCount:      options.deviceCount,
		RequestedSources: options.requestedSources,
	})
	if err != nil {
		return err
	}
	return writeJobsJSON(logf, operatorflow.BuildSyncSavedBindingsLogResult(result))
}

func runOperatorCycle(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseOperatorCycleArgs(args)
	if err != nil {
		return err
	}
	return runOperatorCycleWithOptions(options, cfg, logf)
}

func runOperatorCycleWithOptions(options operatorCycleOptions, cfg runtimecfg.Config, logf func(string, ...any)) error {
	runner, err := newOperatorCommandRunner(cfg)
	if err != nil {
		return err
	}
	defer runner.Close()

	payload, err := runner.RunCycle(context.Background(), options)
	_ = saveOperatorJobsState(runner.cfg, options, 1, payload, err)
	if err != nil {
		if payload != nil {
			if writeErr := writeJobsJSON(logf, payload); writeErr != nil {
				return writeErr
			}
		}
		return err
	}
	return writeJobsJSON(logf, payload)
}

func runOperatorLoop(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseOperatorLoopArgs(args)
	if err != nil {
		return err
	}
	runner, err := newOperatorCommandRunner(cfg)
	if err != nil {
		return err
	}
	defer runner.Close()

	ctx := context.Background()
	iteration := 0
	for {
		iteration++
		startedAt := time.Now().UTC()
		loopOptions := options.operatorCycleOptions
		loopOptions.triggerMode = ingest.OperatorTriggerLoop
		payload, cycleErr := runner.RunCycle(ctx, loopOptions)
		payload = annotateLoopPayload(payload, options, iteration, startedAt)
		_ = saveOperatorJobsState(runner.cfg, options.operatorCycleOptions, iteration, payload, cycleErr)
		if err := writeJobsJSON(logf, payload); err != nil {
			return err
		}
		if cycleErr != nil {
			return cycleErr
		}
		if options.iterations > 0 && iteration >= options.iterations {
			return nil
		}
		if err := operatorLoopSleep(ctx, options.interval); err != nil {
			return err
		}
	}
}

func runShowOperatorRuns(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	options, err := parseSummaryArgs(args)
	if err != nil {
		return err
	}
	service, cleanup, err := buildIngestService(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	runs, err := service.ListOperatorRuns(context.Background(), ingest.SummaryQuery{
		HubUserID: options.hubUserID,
	})
	if err != nil {
		return err
	}
	return writeJobsJSON(logf, runs)
}

func parseSyncSavedBindingsArgs(args []string) (syncSavedBindingsOptions, error) {
	options := syncSavedBindingsOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--sources":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--sources requires a value")
			}
			requested, err := parseRequestedSources(args[i])
			if err != nil {
				return options, err
			}
			options.requestedSources = requested
		case "--hub-user-id":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--hub-user-id requires a value")
			}
			options.hubUserID = strings.TrimSpace(args[i])
		case "--device-count":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--device-count requires a value")
			}
			value, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return options, fmt.Errorf("invalid --device-count: %w", err)
			}
			options.deviceCount = value
		default:
			return options, fmt.Errorf("unknown sync-saved-bindings arg: %s", args[i])
		}
	}
	return options, nil
}

func parseOperatorCycleArgs(args []string) (operatorCycleOptions, error) {
	options := operatorCycleOptions{}
	remaining := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "--strict" {
			options.strict = true
			continue
		}
		remaining = append(remaining, arg)
	}
	parsed, err := parseSyncSavedBindingsArgs(remaining)
	if err != nil {
		return options, err
	}
	options.syncSavedBindingsOptions = parsed
	return options, nil
}

func parseOperatorLoopArgs(args []string) (operatorLoopOptions, error) {
	options := operatorLoopOptions{
		interval: 5 * time.Minute,
	}
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--interval":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--interval requires a value")
			}
			value, err := time.ParseDuration(strings.TrimSpace(args[i]))
			if err != nil {
				return options, fmt.Errorf("invalid --interval: %w", err)
			}
			if value <= 0 {
				return options, fmt.Errorf("--interval must be greater than 0")
			}
			options.interval = value
		case "--iterations":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("--iterations requires a value")
			}
			value, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return options, fmt.Errorf("invalid --iterations: %w", err)
			}
			if value < 0 {
				return options, fmt.Errorf("--iterations must be 0 or greater")
			}
			options.iterations = value
		default:
			remaining = append(remaining, args[i])
		}
	}
	parsed, err := parseOperatorCycleArgs(remaining)
	if err != nil {
		return options, err
	}
	options.operatorCycleOptions = parsed
	return options, nil
}

func parseRequestedSources(value string) (map[string]struct{}, error) {
	return operatorflow.NormalizeRequestedSourcesCSV(value)
}

func currentJobsRuntimeStatus(runtimePack *runtimecfg.DashboardRuntime) runtimecfg.Status {
	status := runtimePack.RuntimeStatus
	if runtimePack.ReadCache != nil {
		status = operatorflow.WithRuntimeCacheSnapshot(status, runtimePack.ReadCache)
	}
	return status
}

func annotateLoopPayload(payload map[string]any, options operatorLoopOptions, iteration int, startedAt time.Time) map[string]any {
	if payload == nil {
		payload = map[string]any{}
	}
	loop := map[string]any{
		"iteration":  iteration,
		"iterations": options.iterations,
		"interval":   options.interval.String(),
		"startedAt":  startedAt,
	}
	if options.iterations == 0 || iteration < options.iterations {
		loop["nextRunAt"] = startedAt.Add(options.interval)
	}
	if options.iterations > 0 {
		loop["remainingIterations"] = options.iterations - iteration
	}
	payload["loop"] = loop
	return payload
}

func saveOperatorJobsState(cfg runtimecfg.Config, options operatorCycleOptions, iteration int, payload map[string]any, runErr error) error {
	var syncPayload map[string]any
	if payload != nil {
		if rawSyncPayload, ok := payload["sync"].(map[string]any); ok {
			syncPayload = rawSyncPayload
		}
	}
	requestedSources := operatorflow.RuntimeRequestedSourcesFromSyncPayload(options.requestedSources, syncPayload)
	state := runtimecfg.JobsRuntimeState{
		LastMode:             "operator-cycle",
		LastStatus:           payloadStatus(payload, runErr),
		LastHubUserID:        strings.TrimSpace(options.hubUserID),
		LastStartedAt:        payloadTime(payload, "startedAt"),
		LastFinishedAt:       payloadTime(payload, "finishedAt"),
		LastIteration:        iteration,
		LastIterations:       1,
		LastError:            payloadError(payload, runErr),
		LastRequestedCount:   len(requestedSources),
		LastRequestedSources: requestedSources,
	}
	if payload != nil {
		if loopPayload, ok := payload["loop"].(map[string]any); ok {
			state.LastMode = "operator-loop"
			state.LastIterations = payloadInt(loopPayload, "iterations")
			state.LastInterval = payloadString(loopPayload, "interval")
		}
	}
	return runtimecfg.SaveJobsRuntimeState(cfg.JobsStatePath, state)
}

func payloadStatus(payload map[string]any, runErr error) string {
	if value := payloadString(payload, "status"); value != "" {
		return value
	}
	if runErr != nil {
		return "failed"
	}
	return "ok"
}

func payloadError(payload map[string]any, runErr error) string {
	if value := payloadString(payload, "error"); value != "" {
		return value
	}
	if runErr != nil {
		return runErr.Error()
	}
	return ""
}

func payloadTime(payload map[string]any, key string) time.Time {
	syncPayload, ok := payload["sync"].(map[string]any)
	if !ok {
		return time.Time{}
	}
	value := payloadString(syncPayload, key)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

func payloadInt(payload map[string]any, key string) int {
	if payload == nil {
		return 0
	}
	value, ok := payload[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func writeJobsJSON(logf func(string, ...any), payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	logf("%s", data)
	return nil
}
