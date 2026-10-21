package insightai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/prompt"
)

const (
	ModuleStatusCompleted            = "completed"
	ModuleStatusFallback             = "fallback"
	ModuleStatusFailed               = "failed"
	ModuleStatusDisabled             = "disabled"
	ModuleStatusInsufficientEvidence = "insufficient_evidence"
	maxModuleInputBytes              = maxInputBytes
	maxModuleOutputBytes             = maxResponseBytes
)

var (
	ErrModuleRequired       = errors.New("AI analysis module is required")
	ErrScopeKeyRequired     = errors.New("AI analysis scope key is required")
	ErrInvalidModuleWindow  = errors.New("AI analysis window must be all, 24h or 72h")
	ErrModuleInputRequired  = errors.New("AI analysis input is required")
	ErrModuleUnknown        = errors.New("AI analysis module is not supported")
	ErrModuleVariable       = errors.New("AI analysis variable is not allowed")
	ErrModuleEvidence       = errors.New("AI analysis evidence is insufficient")
	errModulePromptDisabled = errors.New("AI analysis prompt is disabled")
)

type structuredRequestError struct {
	operation string
	err       error
}

func (e structuredRequestError) Error() string {
	return requestFailureReason(e.operation, e.err)
}

func (e structuredRequestError) Unwrap() error {
	return e.err
}

type ModuleInput struct {
	Module    string
	ScopeKey  string
	Window    string
	Variables map[string]json.RawMessage
}

type ModuleExecution struct {
	Module                   string
	ScopeKey                 string
	Window                   string
	Status                   string
	Result                   json.RawMessage
	FallbackReason           string
	InputFingerprint         string
	RunID                    string
	ModelProvider            string
	ModelName                string
	ModelAttempted           bool
	PromptVersion            string
	ConfigOwnerID            string
	ConfigSource             string
	ConfigVerificationStatus string
	AnalysisEngine           string
	FallbackPolicy           string
	DurationMS               int64
	RunAt                    time.Time
}

type ModuleDescriptor struct {
	Key               string
	Label             string
	Variables         []string
	RequiredVariables []string
	SchemaName        string
	Schema            map[string]any
}

func ModuleDescriptorFor(key string) (ModuleDescriptor, bool) {
	key = strings.TrimSpace(key)
	profile, ok := prompt.Descriptor(key)
	if !ok {
		return ModuleDescriptor{}, false
	}
	return ModuleDescriptor{
		Key:               key,
		Label:             profile.Label,
		Variables:         append([]string(nil), profile.Variables...),
		RequiredVariables: append([]string(nil), moduleRequiredVariables[key]...),
		SchemaName:        moduleSchemaNames[key],
		Schema:            moduleResultSchemas[key],
	}, true
}

func ModuleDescriptors() []ModuleDescriptor {
	result := make([]ModuleDescriptor, 0, len(prompt.Descriptors()))
	for _, profile := range prompt.Descriptors() {
		descriptor, ok := ModuleDescriptorFor(profile.Key)
		if ok {
			result = append(result, descriptor)
		}
	}
	return result
}

func NormalizeModuleInput(input ModuleInput) (ModuleInput, error) {
	input.Module = strings.TrimSpace(input.Module)
	if input.Module == "" {
		return ModuleInput{}, ErrModuleRequired
	}
	if _, ok := ModuleDescriptorFor(input.Module); !ok {
		return ModuleInput{}, fmt.Errorf("%w: %s", ErrModuleUnknown, input.Module)
	}
	input.ScopeKey = strings.TrimSpace(input.ScopeKey)
	if input.ScopeKey == "" {
		return ModuleInput{}, ErrScopeKeyRequired
	}
	input.Window = strings.ToLower(strings.TrimSpace(input.Window))
	if input.Window == "" {
		input.Window = "all"
	}
	switch input.Window {
	case "all", "24h", "72h":
	default:
		return ModuleInput{}, ErrInvalidModuleWindow
	}
	if input.Variables == nil {
		input.Variables = map[string]json.RawMessage{}
	}
	for key, raw := range input.Variables {
		if _, ok := promptVariableAllowed(input.Module, key); !ok {
			return ModuleInput{}, fmt.Errorf("%w %q for %s", ErrModuleVariable, key, input.Module)
		}
		if len(raw) == 0 || !json.Valid(raw) {
			return ModuleInput{}, fmt.Errorf("%w %q must contain valid JSON", ErrModuleInputRequired, key)
		}
	}
	descriptor, _ := ModuleDescriptorFor(input.Module)
	for _, key := range descriptor.RequiredVariables {
		raw, ok := input.Variables[key]
		if !ok || !hasModuleEvidence(raw) {
			return ModuleInput{}, fmt.Errorf("%w: %s", ErrModuleEvidence, key)
		}
	}
	return input, nil
}

func promptVariableAllowed(module, variable string) (string, bool) {
	descriptor, ok := ModuleDescriptorFor(module)
	if !ok {
		return "", false
	}
	for _, candidate := range descriptor.Variables {
		if candidate == variable {
			return candidate, true
		}
	}
	return "", false
}

func hasModuleEvidence(raw json.RawMessage) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return false
	}
	switch typed := value.(type) {
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return true
	}
}

func moduleInputFingerprint(input ModuleInput) string {
	payload, _ := json.Marshal(struct {
		Module    string
		ScopeKey  string
		Window    string
		Variables map[string]json.RawMessage
	}{input.Module, input.ScopeKey, input.Window, input.Variables})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func moduleRunID(input ModuleInput, startedAt time.Time) string {
	return "openai-" + sanitizeID(input.Module) + "-" + sanitizeID(input.ScopeKey) + "-" + startedAt.UTC().Format("20060102T150405.000Z")
}

func moduleFallbackExecution(settings ServiceSettings, input ModuleInput, reason string, startedAt time.Time, attempted bool) ModuleExecution {
	status := ModuleStatusFallback
	if settings.AnalysisEngine != AnalysisEngineRule && settings.FallbackPolicy == FallbackPolicyError {
		status = ModuleStatusFailed
	}
	execution := ModuleExecution{
		Module:                   input.Module,
		ScopeKey:                 input.ScopeKey,
		Window:                   input.Window,
		Status:                   status,
		FallbackReason:           strings.TrimSpace(reason),
		InputFingerprint:         moduleInputFingerprint(input),
		RunID:                    moduleRunID(input, startedAt),
		ModelProvider:            ProviderOpenAI,
		ModelName:                settings.Model,
		ModelAttempted:           attempted,
		PromptVersion:            modulePromptVersions[input.Module],
		ConfigOwnerID:            settings.OwnerID,
		ConfigSource:             settings.Source,
		ConfigVerificationStatus: settings.Verification,
		AnalysisEngine:           settings.AnalysisEngine,
		FallbackPolicy:           settings.FallbackPolicy,
		RunAt:                    startedAt.UTC(),
	}
	if !startedAt.IsZero() {
		execution.DurationMS = time.Since(startedAt).Milliseconds()
	}
	return execution
}

func (c *Client) AnalyzeModule(ctx context.Context, input ModuleInput) (ModuleExecution, error) {
	input, err := NormalizeModuleInput(input)
	if err != nil {
		return ModuleExecution{}, err
	}
	descriptor, _ := ModuleDescriptorFor(input.Module)
	settings, err := c.resolveServiceSettings(ctx)
	if err != nil {
		fallbackSettings := DefaultServiceSettings()
		fallbackSettings.OwnerID = c.serviceOwnerID(ctx)
		return moduleFallbackExecution(fallbackSettings, input, "AI service settings could not be loaded: "+err.Error(), time.Now(), false), nil
	}
	startedAt := time.Now()
	if settings.AnalysisEngine == AnalysisEngineRule {
		return moduleFallbackExecution(settings, input, "analysis engine is configured to rule-engine", startedAt, false), nil
	}
	if settings.APIKey == "" {
		return moduleFallbackExecution(settings, input, ErrNotConfigured.Error(), startedAt, false), nil
	}
	systemPrompt, userPrompt, promptVersion, err := c.resolveModulePrompt(ctx, descriptor, input.Variables)
	if err != nil {
		if errors.Is(err, errModulePromptDisabled) {
			execution := moduleFallbackExecution(settings, input, err.Error(), startedAt, false)
			execution.Status = ModuleStatusDisabled
			return execution, nil
		}
		return moduleFallbackExecution(settings, input, err.Error(), startedAt, false), nil
	}
	executionBase := moduleFallbackExecution(settings, input, "", startedAt, true)
	executionBase.PromptVersion = promptVersion
	payload, err := json.Marshal(input.Variables)
	if err != nil {
		return ModuleExecution{}, fmt.Errorf("marshal module analysis variables: %w", err)
	}
	if len(payload) > maxModuleInputBytes {
		return moduleFallbackExecution(settings, input, "AI analysis input is too large", startedAt, false), nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()
	statusCode, responseBody, err := c.requestStructuredOutput(requestCtx, settings, systemPrompt, userPrompt, descriptor.SchemaName, descriptor.Schema)
	if err != nil {
		return moduleFallbackExecution(settings, input, requestFailureReason("openai request", err), startedAt, true), nil
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return moduleFallbackExecution(settings, input, fmt.Sprintf("openai request returned HTTP %d", statusCode), startedAt, true), nil
	}
	textOutput, err := responseOutputText(responseBody)
	if err != nil {
		return moduleFallbackExecution(settings, input, err.Error(), startedAt, true), nil
	}
	if len(textOutput) > maxModuleOutputBytes {
		return moduleFallbackExecution(settings, input, "openai analysis result is too large", startedAt, true), nil
	}
	if err := validateModuleResult(input.Module, []byte(textOutput)); err != nil {
		return moduleFallbackExecution(settings, input, "openai analysis result was invalid: "+err.Error(), startedAt, true), nil
	}
	executionBase.Status = ModuleStatusCompleted
	executionBase.Result = json.RawMessage(append([]byte(nil), []byte(textOutput)...))
	executionBase.FallbackReason = ""
	executionBase.DurationMS = time.Since(startedAt).Milliseconds()
	executionBase.RunID = moduleRunID(input, startedAt)
	return executionBase, nil
}

func (c *Client) resolveModulePrompt(ctx context.Context, descriptor ModuleDescriptor, variables map[string]json.RawMessage) (string, string, string, error) {
	profile, ok := prompt.DefaultProfile(descriptor.Key)
	if !ok {
		return "", "", modulePromptVersions[descriptor.Key], fmt.Errorf("%w: %s", ErrModuleUnknown, descriptor.Key)
	}
	version := modulePromptVersions[descriptor.Key]
	if c.config.PromptRepository != nil {
		ownerID := ownerIDFromContext(ctx)
		if ownerID == "" {
			ownerID = strings.TrimSpace(c.config.PromptOwnerID)
		}
		if ownerID == "" {
			return "", "", version, errors.New("prompt owner id is not configured")
		}
		settings, err := c.config.PromptRepository.Load(ctx, ownerID)
		if err != nil {
			return "", "", version, fmt.Errorf("prompt settings could not be loaded: %w", err)
		}
		if candidate, exists := settings.Profiles[descriptor.Key]; exists {
			profile = candidate
		}
		if !profile.Enabled {
			return "", "", version, errModulePromptDisabled
		}
	}
	if err := prompt.ValidateProfile(profile); err != nil {
		return "", "", version, fmt.Errorf("%s prompt is invalid: %w", descriptor.Key, err)
	}
	values := make(map[string]string, len(variables))
	for key, raw := range variables {
		values[key] = string(raw)
	}
	rendered, err := prompt.Render(profile.UserPromptTemplate, values)
	if err != nil {
		return "", "", version, fmt.Errorf("%s prompt cannot be rendered: %w", descriptor.Key, err)
	}
	if strings.TrimSpace(profile.Version) != "" {
		version = profile.Version
	}
	return profile.SystemPrompt, rendered, version, nil
}

func responseOutputText(responseBody []byte) (string, error) {
	var decoded responsesResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", errors.New("openai response was not valid JSON")
	}
	textOutput := strings.TrimSpace(decoded.OutputText)
	if textOutput == "" {
		textOutput = strings.TrimSpace(decoded.firstOutputText())
	}
	if textOutput == "" {
		return "", errors.New("openai returned no usable structured analysis")
	}
	return textOutput, nil
}

func validateModuleResult(module string, raw []byte) error {
	descriptor, ok := ModuleDescriptorFor(module)
	if !ok {
		return ErrModuleUnknown
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return errors.New("result must be a JSON object")
	}
	schemaProperties, _ := descriptor.Schema["properties"].(map[string]any)
	for key, value := range object {
		if _, ok := schemaProperties[key]; !ok {
			return fmt.Errorf("unknown result field %q", key)
		}
		if err := validateModuleResultField(key, value, schemaProperties[key]); err != nil {
			return err
		}
	}
	required, _ := descriptor.Schema["required"].([]string)
	for _, key := range required {
		value, ok := object[key]
		if !ok {
			return fmt.Errorf("missing result field %q", key)
		}
		if string(value) == "null" {
			return fmt.Errorf("result field %q cannot be null", key)
		}
	}
	return nil
}

func validateModuleResultField(key string, raw json.RawMessage, schema any) error {
	properties, ok := schema.(map[string]any)
	if !ok {
		return fmt.Errorf("result field %q has invalid schema", key)
	}
	expected, _ := properties["type"].(string)
	switch expected {
	case "string":
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return fmt.Errorf("result field %q must be a string", key)
		}
	case "array":
		var value []string
		if json.Unmarshal(raw, &value) != nil {
			return fmt.Errorf("result field %q must be an array of strings", key)
		}
	default:
		return fmt.Errorf("result field %q has unsupported schema type %q", key, expected)
	}
	return nil
}

func (c *Client) requestStructuredOutput(ctx context.Context, settings ServiceSettings, systemPrompt, userPrompt, schemaName string, schema map[string]any) (int, []byte, error) {
	requestPayload := responsesRequest{
		Model:           settings.Model,
		MaxOutputTokens: 1024,
		Instructions:    systemPrompt,
		Input:           userPrompt,
		Text: responsesTextConfig{Format: responsesFormatConfig{
			Type:       "json_schema",
			Name:       schemaName,
			Strict:     true,
			JSONSchema: schema,
		}},
	}
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal openai request: %w", err)
	}
	statusCode, responseBody, err := c.postResponses(ctx, settings, body)
	if err != nil {
		return 0, nil, structuredRequestError{operation: "openai request", err: err}
	}
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		return statusCode, responseBody, nil
	}
	if !shouldRetryStructuredOutput(statusCode) {
		return statusCode, responseBody, nil
	}
	compatPayload := responsesRequest{
		Model:           settings.Model,
		MaxOutputTokens: 1024,
		Instructions:    systemPrompt,
		Input:           userPrompt + "\n\n请只返回一个可解析的 JSON 对象，不要使用 Markdown 代码块，也不要添加 JSON 以外的文字。",
		Text: responsesTextConfig{Format: responsesFormatConfig{
			Type: "json_object",
		}},
	}
	compatBody, err := json.Marshal(compatPayload)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal compatible openai request: %w", err)
	}
	statusCode, responseBody, err = c.postResponses(ctx, settings, compatBody)
	if err != nil {
		return 0, nil, structuredRequestError{operation: "openai compatible request", err: err}
	}
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		return statusCode, responseBody, nil
	}
	plainPayload := plainResponsesRequest{
		Model:           settings.Model,
		MaxOutputTokens: 1024,
		Instructions:    systemPrompt,
		Input:           compatPayload.Input,
	}
	if plainBody, marshalErr := json.Marshal(plainPayload); marshalErr == nil {
		if plainStatus, plainResponseBody, plainErr := c.postResponses(ctx, settings, plainBody); plainErr == nil {
			statusCode, responseBody = plainStatus, plainResponseBody
		}
	}
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		return statusCode, responseBody, nil
	}
	chatStatus, chatResponseBody, chatErr := c.postChatCompletions(ctx, settings, systemPrompt, compatPayload.Input)
	if chatErr == nil {
		statusCode, responseBody = chatStatus, chatResponseBody
	}
	return statusCode, responseBody, nil
}
