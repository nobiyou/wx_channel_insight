package insightapi

import (
	"encoding/json"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type AIAnalysisRequest struct {
	ScopeKey  string                     `json:"scopeKey"`
	Window    string                     `json:"window"`
	Variables map[string]json.RawMessage `json:"variables"`
}

type AIAnalysisRunResponse struct {
	ID                       string          `json:"id"`
	WorkspaceID              string          `json:"workspaceId"`
	Module                   string          `json:"module"`
	ScopeKey                 string          `json:"scopeKey"`
	Window                   string          `json:"window"`
	Status                   string          `json:"status"`
	Result                   json.RawMessage `json:"result,omitempty"`
	ModelProvider            string          `json:"modelProvider"`
	ModelName                string          `json:"modelName"`
	ModelAttempted           bool            `json:"modelAttempted"`
	PromptVersion            string          `json:"promptVersion"`
	InputFingerprint         string          `json:"inputFingerprint"`
	ConfigOwnerID            string          `json:"configOwnerId,omitempty"`
	ConfigSource             string          `json:"configSource,omitempty"`
	ConfigVerificationStatus string          `json:"configVerificationStatus,omitempty"`
	AnalysisEngine           string          `json:"analysisEngine,omitempty"`
	FallbackPolicy           string          `json:"fallbackPolicy,omitempty"`
	FallbackReason           string          `json:"fallbackReason,omitempty"`
	DurationMS               int64           `json:"durationMs,omitempty"`
	RunAt                    time.Time       `json:"runAt"`
}

type AIAnalysisRunsResponse struct {
	Module   string                  `json:"module"`
	ScopeKey string                  `json:"scopeKey,omitempty"`
	Window   string                  `json:"window,omitempty"`
	Items    []AIAnalysisRunResponse `json:"items"`
}

type AIAnalysisErrorResponse struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

func NewAIAnalysisRunResponse(run insight.AIAnalysisRun) AIAnalysisRunResponse {
	return AIAnalysisRunResponse{
		ID:                       run.ID,
		WorkspaceID:              run.WorkspaceID,
		Module:                   run.Module,
		ScopeKey:                 run.ScopeKey,
		Window:                   run.Window,
		Status:                   run.Status,
		Result:                   append(json.RawMessage(nil), run.Result...),
		ModelProvider:            run.ModelProvider,
		ModelName:                run.ModelName,
		ModelAttempted:           run.ModelAttempted,
		PromptVersion:            run.PromptVersion,
		InputFingerprint:         run.InputFingerprint,
		ConfigOwnerID:            run.ConfigOwnerID,
		ConfigSource:             run.ConfigSource,
		ConfigVerificationStatus: run.ConfigVerificationStatus,
		AnalysisEngine:           run.AnalysisEngine,
		FallbackPolicy:           run.FallbackPolicy,
		FallbackReason:           run.FallbackReason,
		DurationMS:               run.DurationMS,
		RunAt:                    run.RunAt,
	}
}

func NewAIAnalysisRunsResponse(module, scopeKey, window string, items []insight.AIAnalysisRun) AIAnalysisRunsResponse {
	response := AIAnalysisRunsResponse{
		Module:   module,
		ScopeKey: scopeKey,
		Window:   window,
		Items:    make([]AIAnalysisRunResponse, 0, len(items)),
	}
	for _, item := range items {
		response.Items = append(response.Items, NewAIAnalysisRunResponse(item))
	}
	return response
}
