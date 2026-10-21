package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

const (
	aiAnalysisRoutePrefix  = "/api/v1/ai-analysis"
	aiAnalysisRequestLimit = 256 * 1024
)

type moduleAnalysisRunner interface {
	AnalyzeModule(context.Context, insightai.ModuleInput) (insightai.ModuleExecution, error)
}

func (s *server) handleAIAnalysis(w http.ResponseWriter, r *http.Request) {
	if s.aiAnalysisRepository == nil {
		writeAIAnalysisError(w, http.StatusServiceUnavailable, "ai_analysis_unavailable", "AI analysis service is not configured")
		return
	}
	relative := strings.TrimPrefix(r.URL.Path, aiAnalysisRoutePrefix)
	relative = strings.Trim(relative, "/")
	if relative == "" {
		writeAIAnalysisError(w, http.StatusBadRequest, "module_required", insightai.ErrModuleRequired.Error())
		return
	}
	if strings.HasPrefix(relative, "runs/") {
		if r.Method != http.MethodGet {
			writeAIAnalysisError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		runID := strings.Trim(strings.TrimPrefix(relative, "runs/"), "/")
		if runID == "" || strings.Contains(runID, "/") {
			writeAIAnalysisError(w, http.StatusBadRequest, "run_id_required", "run id is required")
			return
		}
		s.handleAIAnalysisRun(w, r, runID)
		return
	}
	if strings.Contains(relative, "/") {
		writeAIAnalysisError(w, http.StatusBadRequest, "invalid_route", "AI analysis route is invalid")
		return
	}
	module := relative
	if r.Method == http.MethodPost {
		s.handleRunAIAnalysis(w, r, module)
		return
	}
	if r.Method == http.MethodGet {
		s.handleListAIAnalysis(w, r, module)
		return
	}
	writeAIAnalysisError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

func (s *server) handleRunAIAnalysis(w http.ResponseWriter, r *http.Request, module string) {
	if s.aiAnalysisRunner == nil {
		writeAIAnalysisError(w, http.StatusServiceUnavailable, "ai_analysis_unavailable", "AI analysis runner is not configured")
		return
	}
	var payload insightapi.AIAnalysisRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, aiAnalysisRequestLimit))
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			writeAIAnalysisError(w, http.StatusBadRequest, "request_required", "AI analysis request is required")
			return
		}
		writeAIAnalysisError(w, http.StatusBadRequest, "invalid_json", "AI analysis request is invalid")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeAIAnalysisError(w, http.StatusBadRequest, "invalid_json", "AI analysis request must contain one JSON object")
		return
	}
	input := insightai.ModuleInput{
		Module:    module,
		ScopeKey:  payload.ScopeKey,
		Window:    payload.Window,
		Variables: payload.Variables,
	}
	normalized, err := insightai.NormalizeModuleInput(input)
	if err != nil {
		writeAIAnalysisValidationError(w, err)
		return
	}
	resolved, err := s.resolveAIAnalysisWorkspace(r)
	if err != nil {
		writeAIAnalysisError(w, http.StatusInternalServerError, "workspace_resolution_failed", "workspace context could not be resolved")
		return
	}
	analysisContext := insightai.WithPromptOwner(r.Context(), s.promptOwnerID(r))
	execution, err := s.aiAnalysisRunner.AnalyzeModule(analysisContext, normalized)
	if err != nil {
		writeAIAnalysisError(w, http.StatusBadGateway, "provider_failure", "AI analysis provider request failed")
		return
	}
	run := AIAnalysisRunFromExecution(resolved.Workspace.ID, execution)
	if err := s.aiAnalysisRepository.Save(r.Context(), run); err != nil {
		writeAIAnalysisError(w, http.StatusInternalServerError, "run_persistence_failed", "AI analysis result could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewAIAnalysisRunResponse(run))
}

func (s *server) handleListAIAnalysis(w http.ResponseWriter, r *http.Request, module string) {
	descriptor, ok := insightai.ModuleDescriptorFor(module)
	if !ok {
		writeAIAnalysisError(w, http.StatusBadRequest, "module_unknown", "AI analysis module is not supported")
		return
	}
	resolved, err := s.resolveAIAnalysisWorkspace(r)
	if err != nil {
		writeAIAnalysisError(w, http.StatusInternalServerError, "workspace_resolution_failed", "workspace context could not be resolved")
		return
	}
	scopeKey := firstQuery(r, "scopeKey", "scope_key")
	window := firstQuery(r, "window")
	limit, err := parseAIAnalysisLimit(r)
	if err != nil {
		writeAIAnalysisError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}
	items, err := s.aiAnalysisRepository.List(r.Context(), resolved.Workspace.ID, descriptor.Key, scopeKey, window, limit)
	if err != nil {
		if errors.Is(err, insight.ErrInvalidTimeWindow) {
			writeAIAnalysisError(w, http.StatusBadRequest, "invalid_window", err.Error())
			return
		}
		writeAIAnalysisError(w, http.StatusInternalServerError, "run_query_failed", "AI analysis history could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewAIAnalysisRunsResponse(descriptor.Key, scopeKey, window, items))
}

func (s *server) handleAIAnalysisRun(w http.ResponseWriter, r *http.Request, runID string) {
	resolved, err := s.resolveAIAnalysisWorkspace(r)
	if err != nil {
		writeAIAnalysisError(w, http.StatusInternalServerError, "workspace_resolution_failed", "workspace context could not be resolved")
		return
	}
	run, found, err := s.aiAnalysisRepository.Get(r.Context(), resolved.Workspace.ID, runID)
	if err != nil {
		writeAIAnalysisError(w, http.StatusInternalServerError, "run_query_failed", "AI analysis run could not be loaded")
		return
	}
	if !found {
		writeAIAnalysisError(w, http.StatusNotFound, "run_not_found", insight.ErrAIAnalysisRunNotFound.Error())
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewAIAnalysisRunResponse(run))
}

func (s *server) resolveAIAnalysisWorkspace(r *http.Request) (workspace.ResolvedContext, error) {
	if s.contextResolver == nil {
		return workspace.ResolvedContext{}, errors.New("workspace context resolver is not configured")
	}
	hub := hubContextFromRequest(r)
	return s.contextResolver.Resolve(r.Context(), workspace.ContextQuery{
		HubUserID:     hub.HubUserID,
		DeviceCount:   hub.DeviceCount,
		ContextSource: hub.ContextSource,
	})
}

func AIAnalysisRunFromExecution(workspaceID string, execution insightai.ModuleExecution) insight.AIAnalysisRun {
	return insight.AIAnalysisRun{
		ID:                       execution.RunID,
		WorkspaceID:              workspaceID,
		Module:                   execution.Module,
		ScopeKey:                 execution.ScopeKey,
		Window:                   execution.Window,
		InputFingerprint:         execution.InputFingerprint,
		Status:                   execution.Status,
		Result:                   append(json.RawMessage(nil), execution.Result...),
		ModelProvider:            execution.ModelProvider,
		ModelName:                execution.ModelName,
		ModelAttempted:           execution.ModelAttempted,
		PromptVersion:            execution.PromptVersion,
		ConfigOwnerID:            execution.ConfigOwnerID,
		ConfigSource:             execution.ConfigSource,
		ConfigVerificationStatus: execution.ConfigVerificationStatus,
		AnalysisEngine:           execution.AnalysisEngine,
		FallbackPolicy:           execution.FallbackPolicy,
		FallbackReason:           execution.FallbackReason,
		DurationMS:               execution.DurationMS,
		RunAt:                    execution.RunAt,
	}
}

func parseAIAnalysisLimit(r *http.Request) (int, error) {
	value := firstQuery(r, "limit")
	if value == "" {
		return insight.MaxAIAnalysisHistoryLimit, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > insight.MaxAIAnalysisHistoryLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", insight.MaxAIAnalysisHistoryLimit)
	}
	return limit, nil
}

func writeAIAnalysisValidationError(w http.ResponseWriter, err error) {
	code := "invalid_request"
	switch {
	case errors.Is(err, insightai.ErrModuleUnknown):
		code = "module_unknown"
	case errors.Is(err, insightai.ErrModuleVariable):
		code = "variable_not_allowed"
	case errors.Is(err, insightai.ErrModuleEvidence):
		code = "insufficient_evidence"
	case errors.Is(err, insightai.ErrInvalidModuleWindow):
		code = "invalid_window"
	}
	writeAIAnalysisError(w, http.StatusBadRequest, code, err.Error())
}

func writeAIAnalysisError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, insightapi.AIAnalysisErrorResponse{Code: code, Error: strings.TrimSpace(message)})
}
