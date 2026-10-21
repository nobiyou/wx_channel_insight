package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspacedemo"
)

type aiAnalysisRunnerStub struct {
	execution insightai.ModuleExecution
	err       error
	called    int
}

func (r *aiAnalysisRunnerStub) AnalyzeModule(_ context.Context, input insightai.ModuleInput) (insightai.ModuleExecution, error) {
	r.called++
	if r.err != nil {
		return insightai.ModuleExecution{}, r.err
	}
	execution := r.execution
	if execution.Module == "" {
		execution.Module = input.Module
	}
	if execution.ScopeKey == "" {
		execution.ScopeKey = input.ScopeKey
	}
	if execution.Window == "" {
		execution.Window = input.Window
	}
	return execution, nil
}

type failingAIAnalysisRepository struct{}

func (failingAIAnalysisRepository) Save(context.Context, insight.AIAnalysisRun) error {
	return errors.New("storage unavailable")
}
func (failingAIAnalysisRepository) List(context.Context, string, string, string, string, int) ([]insight.AIAnalysisRun, error) {
	return nil, errors.New("storage unavailable")
}
func (failingAIAnalysisRepository) Get(context.Context, string, string) (insight.AIAnalysisRun, bool, error) {
	return insight.AIAnalysisRun{}, false, errors.New("storage unavailable")
}

func newAIAnalysisTestServer(t *testing.T, runner moduleAnalysisRunner, repo insight.AIAnalysisRepository) *server {
	t.Helper()
	workspaceService := workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository())
	resolver := workspace.NewContextResolver(workspaceService, workspace.ContextResolverConfig{
		DefaultHubUserID: "hub:user:default", DefaultDeviceCount: 2,
	})
	srv := newServer(nil, nil, nil, resolver, runtimeStatusForAIAnalysisTest())
	srv.aiAnalysisRunner = runner
	srv.aiAnalysisRepository = repo
	srv.bridgeConfig.DemoContextEnabled = true
	return srv
}

func runtimeStatusForAIAnalysisTest() runtime.Status {
	return runtime.Status{OperatorIdentity: "insight:operator:test"}
}

func aiAnalysisRequest(t *testing.T, method, path, user string, body any) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal test request failed: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path+"&hub_user_id="+user, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestAIAnalysisRouteRunsAndPersistsWorkspaceScopedExecution(t *testing.T) {
	runner := &aiAnalysisRunnerStub{execution: insightai.ModuleExecution{
		Status: insightai.ModuleStatusCompleted, RunID: "run-api-1", InputFingerprint: "sha256:api",
		ModelProvider: insightai.ProviderOpenAI, ModelName: "gpt-5.6", ModelAttempted: true,
		PromptVersion: "content-v1", RunAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
		Result: json.RawMessage(`{"summary":"ok","strengths":[],"weaknesses":[],"audience":"operators","recommendations":[],"risks":[],"evidenceRefs":[]}`),
	}}
	repo := insightdemo.NewAIAnalysisRepository()
	srv := newAIAnalysisTestServer(t, runner, repo)
	body := map[string]any{
		"scopeKey": "dashboard", "window": "24h",
		"variables": map[string]any{"data_json": map[string]any{"workspace": "workspace-a"}, "video_json": []any{"video-1"}},
	}
	req := aiAnalysisRequest(t, http.MethodPost, "/api/v1/ai-analysis/content-analysis?", "hub:user:alice", body)
	response := httptest.NewRecorder()
	srv.routes().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("expected successful AI analysis response, got %d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload["status"] != insightai.ModuleStatusCompleted || payload["workspaceId"] != "ws-default-hub-user-alice" {
		t.Fatalf("unexpected AI analysis payload: %+v", payload)
	}
	items, err := repo.List(context.Background(), "ws-default-hub-user-alice", "content-analysis", "dashboard", "24h", 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("expected persisted API run, items=%+v err=%v", items, err)
	}

	detail := httptest.NewRecorder()
	srv.routes().ServeHTTP(detail, aiAnalysisRequest(t, http.MethodGet, "/api/v1/ai-analysis/runs/run-api-1?", "hub:user:bob", nil))
	if detail.Code != http.StatusNotFound {
		t.Fatalf("expected other workspace to be isolated, got %d body=%s", detail.Code, detail.Body.String())
	}
}

func TestAIAnalysisRouteMapsFallbackAndValidation(t *testing.T) {
	runner := &aiAnalysisRunnerStub{execution: insightai.ModuleExecution{
		Module: "content-analysis", ScopeKey: "dashboard", Window: "all", Status: insightai.ModuleStatusFallback,
		RunID: "run-fallback", InputFingerprint: "sha256:fallback", FallbackReason: "not configured",
		RunAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}}
	repo := insightdemo.NewAIAnalysisRepository()
	srv := newAIAnalysisTestServer(t, runner, repo)
	validBody := map[string]any{
		"scopeKey": "dashboard", "variables": map[string]any{"data_json": map[string]any{"count": 1}, "video_json": []any{"video-1"}},
	}
	response := httptest.NewRecorder()
	srv.routes().ServeHTTP(response, aiAnalysisRequest(t, http.MethodPost, "/api/v1/ai-analysis/content-analysis?", "hub:user:alice", validBody))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"status":"fallback"`)) {
		t.Fatalf("expected visible fallback response, got %d body=%s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte(`"result"`)) {
		t.Fatalf("fallback response should not contain result: %s", response.Body.String())
	}

	invalid := httptest.NewRecorder()
	invalidBody := map[string]any{
		"scopeKey": "dashboard", "variables": map[string]any{"data_json": map[string]any{}, "video_json": []any{"video-1"}, "not_allowed": true},
	}
	srv.routes().ServeHTTP(invalid, aiAnalysisRequest(t, http.MethodPost, "/api/v1/ai-analysis/content-analysis?", "hub:user:alice", invalidBody))
	if invalid.Code != http.StatusBadRequest || runner.called != 1 {
		t.Fatalf("expected validation to stop provider call, status=%d calls=%d body=%s", invalid.Code, runner.called, invalid.Body.String())
	}
}

func TestAIAnalysisRouteMapsDisabledInsufficientEvidenceAndHistoryErrors(t *testing.T) {
	runner := &aiAnalysisRunnerStub{execution: insightai.ModuleExecution{
		Module: "content-analysis", ScopeKey: "dashboard", Window: "all", Status: insightai.ModuleStatusDisabled,
		RunID: "run-disabled", InputFingerprint: "sha256:disabled", FallbackReason: "prompt disabled",
		RunAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}}
	srv := newAIAnalysisTestServer(t, runner, insightdemo.NewAIAnalysisRepository())
	validBody := map[string]any{
		"scopeKey": "dashboard", "variables": map[string]any{"data_json": map[string]any{"count": 1}, "video_json": []any{"video-1"}},
	}
	response := httptest.NewRecorder()
	srv.routes().ServeHTTP(response, aiAnalysisRequest(t, http.MethodPost, "/api/v1/ai-analysis/content-analysis?", "hub:user:alice", validBody))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"status":"disabled"`)) {
		t.Fatalf("expected disabled status, got %d body=%s", response.Code, response.Body.String())
	}

	insufficient := httptest.NewRecorder()
	insufficientBody := map[string]any{
		"scopeKey": "dashboard", "variables": map[string]any{"data_json": map[string]any{}, "video_json": []any{}},
	}
	srv.routes().ServeHTTP(insufficient, aiAnalysisRequest(t, http.MethodPost, "/api/v1/ai-analysis/content-analysis?", "hub:user:alice", insufficientBody))
	if insufficient.Code != http.StatusBadRequest || !bytes.Contains(insufficient.Body.Bytes(), []byte(`"code":"insufficient_evidence"`)) {
		t.Fatalf("expected insufficient evidence error, got %d body=%s", insufficient.Code, insufficient.Body.String())
	}

	unknown := httptest.NewRecorder()
	srv.routes().ServeHTTP(unknown, aiAnalysisRequest(t, http.MethodGet, "/api/v1/ai-analysis/unknown-module?", "hub:user:alice", nil))
	if unknown.Code != http.StatusBadRequest || runner.called != 1 {
		t.Fatalf("expected unknown module rejection, status=%d calls=%d body=%s", unknown.Code, runner.called, unknown.Body.String())
	}

	missing := httptest.NewRecorder()
	srv.routes().ServeHTTP(missing, aiAnalysisRequest(t, http.MethodGet, "/api/v1/ai-analysis/runs/missing?", "hub:user:alice", nil))
	if missing.Code != http.StatusNotFound || !bytes.Contains(missing.Body.Bytes(), []byte(`"code":"run_not_found"`)) {
		t.Fatalf("expected missing run response, got %d body=%s", missing.Code, missing.Body.String())
	}
}

func TestAIAnalysisRouteListsSavedRuns(t *testing.T) {
	repo := insightdemo.NewAIAnalysisRepository()
	when := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	if err := repo.Save(context.Background(), insight.AIAnalysisRun{
		ID: "run-list", WorkspaceID: "ws-default-hub-user-alice", Module: "content-analysis", ScopeKey: "dashboard",
		Window: "24h", InputFingerprint: "sha256:list", Status: insight.AIAnalysisStatusFallback, RunAt: when,
	}); err != nil {
		t.Fatalf("seed AI analysis run failed: %v", err)
	}
	srv := newAIAnalysisTestServer(t, &aiAnalysisRunnerStub{}, repo)
	response := httptest.NewRecorder()
	srv.routes().ServeHTTP(response, aiAnalysisRequest(t, http.MethodGet, "/api/v1/ai-analysis/content-analysis?scopeKey=dashboard&window=24h", "hub:user:alice", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"run-list"`)) {
		t.Fatalf("expected saved run list, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestAIAnalysisRouteMapsProviderAndStorageFailures(t *testing.T) {
	providerServer := newAIAnalysisTestServer(t, &aiAnalysisRunnerStub{err: errors.New("upstream")}, insightdemo.NewAIAnalysisRepository())
	body := map[string]any{"scopeKey": "dashboard", "variables": map[string]any{"data_json": map[string]any{"count": 1}, "video_json": []any{"video-1"}}}
	providerResponse := httptest.NewRecorder()
	providerServer.routes().ServeHTTP(providerResponse, aiAnalysisRequest(t, http.MethodPost, "/api/v1/ai-analysis/content-analysis?", "hub:user:alice", body))
	if providerResponse.Code != http.StatusBadGateway || bytes.Contains(providerResponse.Body.Bytes(), []byte("upstream")) {
		t.Fatalf("expected sanitized provider failure, got %d body=%s", providerResponse.Code, providerResponse.Body.String())
	}

	storageServer := newAIAnalysisTestServer(t, &aiAnalysisRunnerStub{execution: insightai.ModuleExecution{
		Module: "content-analysis", ScopeKey: "dashboard", Window: "all", Status: insightai.ModuleStatusFallback,
		RunID: "run-storage", InputFingerprint: "sha256:storage", RunAt: time.Now().UTC(),
	}}, failingAIAnalysisRepository{})
	storageResponse := httptest.NewRecorder()
	storageServer.routes().ServeHTTP(storageResponse, aiAnalysisRequest(t, http.MethodPost, "/api/v1/ai-analysis/content-analysis?", "hub:user:alice", body))
	if storageResponse.Code != http.StatusInternalServerError || bytes.Contains(storageResponse.Body.Bytes(), []byte("storage unavailable")) {
		t.Fatalf("expected sanitized storage failure, got %d body=%s", storageResponse.Code, storageResponse.Body.String())
	}
}
