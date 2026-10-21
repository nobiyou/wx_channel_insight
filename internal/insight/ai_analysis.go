package insight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	AIAnalysisStatusCompleted            = "completed"
	AIAnalysisStatusFallback             = "fallback"
	AIAnalysisStatusFailed               = "failed"
	AIAnalysisStatusDisabled             = "disabled"
	AIAnalysisStatusInsufficientEvidence = "insufficient_evidence"
	MaxAIAnalysisHistoryLimit            = 20
	MaxAIAnalysisResultBytes             = 2 * 1024 * 1024
	maxAIAnalysisIdentityLength          = 200
)

var (
	ErrInvalidAIAnalysisRun  = errors.New("invalid AI analysis run")
	ErrAIAnalysisRunNotFound = errors.New("AI analysis run was not found")
)

// AIAnalysisRun is the durable, workspace-scoped carrier for one advisory
// analysis attempt. It deliberately contains no source snapshot or secret.
type AIAnalysisRun struct {
	ID                       string          `json:"id"`
	WorkspaceID              string          `json:"workspaceId"`
	Module                   string          `json:"module"`
	ScopeKey                 string          `json:"scopeKey"`
	Window                   string          `json:"window"`
	InputFingerprint         string          `json:"inputFingerprint"`
	Status                   string          `json:"status"`
	Result                   json.RawMessage `json:"result,omitempty"`
	ModelProvider            string          `json:"modelProvider"`
	ModelName                string          `json:"modelName"`
	ModelAttempted           bool            `json:"modelAttempted"`
	PromptVersion            string          `json:"promptVersion"`
	ConfigOwnerID            string          `json:"configOwnerId"`
	ConfigSource             string          `json:"configSource"`
	ConfigVerificationStatus string          `json:"configVerificationStatus"`
	AnalysisEngine           string          `json:"analysisEngine"`
	FallbackPolicy           string          `json:"fallbackPolicy"`
	FallbackReason           string          `json:"fallbackReason,omitempty"`
	DurationMS               int64           `json:"durationMs"`
	RunAt                    time.Time       `json:"runAt"`
}

type AIAnalysisRepository interface {
	Save(context.Context, AIAnalysisRun) error
	List(context.Context, string, string, string, string, int) ([]AIAnalysisRun, error)
	Get(context.Context, string, string) (AIAnalysisRun, bool, error)
}

func NormalizeAIAnalysisRun(run AIAnalysisRun) (AIAnalysisRun, error) {
	run.ID = strings.TrimSpace(run.ID)
	run.WorkspaceID = strings.TrimSpace(run.WorkspaceID)
	run.Module = strings.ToLower(strings.TrimSpace(run.Module))
	run.ScopeKey = strings.TrimSpace(run.ScopeKey)
	run.Window = strings.ToLower(strings.TrimSpace(run.Window))
	run.InputFingerprint = strings.TrimSpace(run.InputFingerprint)
	run.Status = strings.ToLower(strings.TrimSpace(run.Status))
	run.ModelProvider = strings.TrimSpace(run.ModelProvider)
	run.ModelName = strings.TrimSpace(run.ModelName)
	run.PromptVersion = strings.TrimSpace(run.PromptVersion)
	run.ConfigOwnerID = strings.TrimSpace(run.ConfigOwnerID)
	run.ConfigSource = strings.TrimSpace(run.ConfigSource)
	run.ConfigVerificationStatus = strings.TrimSpace(run.ConfigVerificationStatus)
	run.AnalysisEngine = strings.TrimSpace(run.AnalysisEngine)
	run.FallbackPolicy = strings.TrimSpace(run.FallbackPolicy)
	run.FallbackReason = strings.TrimSpace(run.FallbackReason)
	if run.Window == "" {
		run.Window = TimeWindowAll
	}
	if normalized, ok := NormalizeTimeWindow(run.Window); ok {
		run.Window = normalized
	} else {
		return AIAnalysisRun{}, fmt.Errorf("%w: window must be all, 24h or 72h", ErrInvalidAIAnalysisRun)
	}
	if run.Status != AIAnalysisStatusCompleted && run.Status != AIAnalysisStatusFallback && run.Status != AIAnalysisStatusFailed && run.Status != AIAnalysisStatusDisabled && run.Status != AIAnalysisStatusInsufficientEvidence {
		return AIAnalysisRun{}, fmt.Errorf("%w: unsupported status %q", ErrInvalidAIAnalysisRun, run.Status)
	}
	if run.Status != AIAnalysisStatusCompleted {
		// Error and fallback records must never retain an unvalidated provider body.
		run.Result = nil
	} else {
		if len(run.Result) == 0 || len(run.Result) > MaxAIAnalysisResultBytes || !json.Valid(run.Result) {
			return AIAnalysisRun{}, fmt.Errorf("%w: completed result must be valid JSON within %d bytes", ErrInvalidAIAnalysisRun, MaxAIAnalysisResultBytes)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(run.Result, &object); err != nil || object == nil {
			return AIAnalysisRun{}, fmt.Errorf("%w: completed result must be a JSON object", ErrInvalidAIAnalysisRun)
		}
		run.Result = append(json.RawMessage(nil), run.Result...)
	}
	if run.DurationMS < 0 {
		return AIAnalysisRun{}, fmt.Errorf("%w: durationMs cannot be negative", ErrInvalidAIAnalysisRun)
	}
	run.RunAt = run.RunAt.UTC()
	if run.ID == "" || run.WorkspaceID == "" || run.Module == "" || run.ScopeKey == "" || run.InputFingerprint == "" || run.RunAt.IsZero() {
		return AIAnalysisRun{}, fmt.Errorf("%w: id, workspaceId, module, scopeKey, inputFingerprint and runAt are required", ErrInvalidAIAnalysisRun)
	}
	for label, value := range map[string]string{
		"id": run.ID, "workspaceId": run.WorkspaceID, "module": run.Module,
		"scopeKey": run.ScopeKey, "inputFingerprint": run.InputFingerprint,
	} {
		if len(value) > maxAIAnalysisIdentityLength {
			return AIAnalysisRun{}, fmt.Errorf("%w: %s is too long", ErrInvalidAIAnalysisRun, label)
		}
	}
	return run, nil
}

func ValidateAIAnalysisRun(run AIAnalysisRun) error {
	_, err := NormalizeAIAnalysisRun(run)
	return err
}

func CloneAIAnalysisRun(run AIAnalysisRun) AIAnalysisRun {
	run.Result = append(json.RawMessage(nil), run.Result...)
	return run
}
