package operator

import (
	"sort"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

type JobsStateOptions struct {
	Mode       string
	Iteration  int
	Iterations int
	Interval   string
}

func BuildJobsRuntimeState(result Result, options Options, runErr error, stateOptions JobsStateOptions) runtime.JobsRuntimeState {
	mode := strings.TrimSpace(stateOptions.Mode)
	if mode == "" {
		mode = "operator-cycle"
	}

	iteration := stateOptions.Iteration
	if iteration <= 0 {
		iteration = 1
	}

	iterations := stateOptions.Iterations
	if iterations <= 0 {
		iterations = 1
	}

	hubUserID := strings.TrimSpace(options.HubUserID)
	if hubUserID == "" {
		hubUserID = strings.TrimSpace(result.ReadModel.WorkspaceBridge.HubUserID)
	}

	lastStatus := BuildCycleStatus(result)
	if lastStatus == "" {
		if runErr != nil {
			lastStatus = "failed"
		} else {
			lastStatus = "ok"
		}
	}

	lastError := ""
	if lastStatus == "failed" {
		lastError = strings.TrimSpace(result.Message)
	}
	if lastError == "" && runErr != nil {
		lastError = strings.TrimSpace(runErr.Error())
	}

	requestedSources := RuntimeRequestedSourcesForResult(options.RequestedSources, result)

	state := runtime.JobsRuntimeState{
		LastMode:             mode,
		LastStatus:           lastStatus,
		LastHubUserID:        hubUserID,
		LastStartedAt:        result.StartedAt,
		LastFinishedAt:       result.FinishedAt,
		LastIteration:        iteration,
		LastIterations:       iterations,
		LastInterval:         strings.TrimSpace(stateOptions.Interval),
		LastError:            lastError,
		LastRequestedCount:   len(requestedSources),
		LastRequestedSources: requestedSources,
	}

	if state.LastMode != "operator-loop" {
		state.LastInterval = ""
	}

	return state
}

func RuntimeRequestedSources(requested map[string]struct{}) []string {
	primary := []string{
		ingest.RunSourceWXCommentExport,
		ingest.RunSourceWXVideoSearch,
		ingest.RunSourceWXFeedList,
	}
	if len(requested) == 0 {
		return append([]string(nil), primary[:2]...)
	}

	result := make([]string, 0, len(requested))
	for _, source := range primary {
		if _, ok := requested[source]; ok {
			result = append(result, source)
		}
	}

	extras := make([]string, 0, len(requested))
	for source := range requested {
		if source == ingest.RunSourceWXCommentExport || source == ingest.RunSourceWXVideoSearch || source == ingest.RunSourceWXFeedList {
			continue
		}
		extras = append(extras, source)
	}
	sort.Strings(extras)
	result = append(result, extras...)
	return result
}

func RuntimeRequestedSourcesForResult(requested map[string]struct{}, result Result) []string {
	if len(requested) > 0 {
		return RuntimeRequestedSources(requested)
	}
	sources := runtimeSourcesFromResult(result)
	if len(sources) > 0 {
		return sources
	}
	return RuntimeRequestedSources(requested)
}

func RuntimeRequestedSourcesFromSyncPayload(requested map[string]struct{}, syncPayload map[string]any) []string {
	if len(requested) > 0 {
		return RuntimeRequestedSources(requested)
	}
	sources := runtimeSourcesFromSyncPayload(syncPayload)
	if len(sources) > 0 {
		return sources
	}
	return RuntimeRequestedSources(requested)
}

func runtimeSourcesFromResult(result Result) []string {
	seen := map[string]struct{}{}
	sources := make([]string, 0, len(result.Imports)+len(result.Failures)+len(result.Skipped))
	for _, item := range result.Imports {
		sources = appendRuntimeSource(sources, seen, item.Source)
	}
	for _, item := range result.Failures {
		sources = appendRuntimeSource(sources, seen, item.Source)
	}
	for _, item := range result.Skipped {
		sources = appendRuntimeSource(sources, seen, item.Source)
	}
	return sources
}

func runtimeSourcesFromSyncPayload(syncPayload map[string]any) []string {
	if syncPayload == nil {
		return nil
	}
	seen := map[string]struct{}{}
	sources := make([]string, 0, 3)
	sources = appendRuntimeSourcesFromPayloadItems(sources, seen, syncPayload["imports"])
	sources = appendRuntimeSourcesFromPayloadItems(sources, seen, syncPayload["failures"])
	sources = appendRuntimeSourcesFromPayloadItems(sources, seen, syncPayload["skipped"])
	return sources
}

func appendRuntimeSourcesFromPayloadItems(sources []string, seen map[string]struct{}, raw any) []string {
	switch items := raw.(type) {
	case []map[string]any:
		for _, payload := range items {
			source, _ := payload["source"].(string)
			sources = appendRuntimeSource(sources, seen, source)
		}
	case []any:
		for _, item := range items {
			payload, ok := item.(map[string]any)
			if !ok {
				continue
			}
			source, _ := payload["source"].(string)
			sources = appendRuntimeSource(sources, seen, source)
		}
	case []Failure:
		for _, item := range items {
			sources = appendRuntimeSource(sources, seen, item.Source)
		}
	case []Skipped:
		for _, item := range items {
			sources = appendRuntimeSource(sources, seen, item.Source)
		}
	}
	return sources
}

func appendRuntimeSource(sources []string, seen map[string]struct{}, source string) []string {
	source = strings.TrimSpace(source)
	switch source {
	case ingest.RunSourceWXCommentExport, ingest.RunSourceWXVideoSearch, ingest.RunSourceWXFeedList:
	default:
		return sources
	}
	if _, ok := seen[source]; ok {
		return sources
	}
	seen[source] = struct{}{}
	return append(sources, source)
}
