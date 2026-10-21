package insightai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/prompt"
)

const (
	DefaultBaseURL   = "https://api.openai.com/v1"
	DefaultModel     = "gpt-5.6"
	maxInputBytes    = 128 * 1024
	maxResponseBytes = 2 * 1024 * 1024
)

var ErrNotConfigured = errors.New("openai api key is not configured")

type Config struct {
	APIKey                 string
	BaseURL                string
	Model                  string
	Timeout                time.Duration
	PromptRepository       prompt.Repository
	PromptOwnerID          string
	ServiceSettingsManager *ServiceSettingsManager
	ServiceOwnerID         string
}

func (c Config) normalized() Config {
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.Model == "" {
		c.Model = DefaultModel
	}
	if c.Timeout <= 0 {
		c.Timeout = 45 * time.Second
	}
	return c
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != ""
}

type Client struct {
	config     Config
	httpClient *http.Client
}

func NewClient(config Config, httpClient *http.Client) *Client {
	config = config.normalized()
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{config: config, httpClient: httpClient}
}

func (c *Client) Model() string {
	if c == nil {
		return DefaultModel
	}
	return c.config.Model
}

func (c *Client) postResponses(ctx context.Context, settings ServiceSettings, body []byte) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.BaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("create openai request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+settings.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("read openai response: %w", err)
	}
	return response.StatusCode, responseBody, nil
}

func shouldRetryStructuredOutput(statusCode int) bool {
	switch statusCode {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func requestFailureReason(operation string, err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return operation + " timed out"
	case errors.Is(err, context.Canceled):
		return operation + " canceled"
	default:
		return operation + " failed"
	}
}

func (c *Client) resolveServiceSettings(ctx context.Context) (ServiceSettings, error) {
	if c.config.ServiceSettingsManager != nil {
		return c.config.ServiceSettingsManager.Resolve(ctx, c.serviceOwnerID(ctx))
	}
	settings := ServiceSettings{
		Provider: ProviderOpenAI,
		BaseURL:  c.config.BaseURL,
		Model:    c.config.Model,
		Timeout:  c.config.Timeout,
		APIKey:   c.config.APIKey,
		Source:   ConfigSourceEnvironment,
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		settings.Source = ConfigSourceNone
	}
	settings.Verification = VerificationNever
	settings.OwnerID = c.serviceOwnerID(ctx)
	return settings, nil
}

func (c *Client) serviceOwnerID(ctx context.Context) string {
	ownerID := ownerIDFromContext(ctx)
	if ownerID == "" {
		ownerID = strings.TrimSpace(c.config.ServiceOwnerID)
	}
	if ownerID == "" {
		ownerID = strings.TrimSpace(c.config.PromptOwnerID)
	}
	return ownerID
}

func sanitizeID(value string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

type responsesRequest struct {
	Model           string              `json:"model"`
	MaxOutputTokens int                 `json:"max_output_tokens,omitempty"`
	Instructions    string              `json:"instructions"`
	Input           string              `json:"input"`
	Text            responsesTextConfig `json:"text"`
}

type responsesTextConfig struct {
	Format responsesFormatConfig `json:"format"`
}

type responsesFormatConfig struct {
	Type       string         `json:"type"`
	Name       string         `json:"name,omitempty"`
	Strict     bool           `json:"strict,omitempty"`
	JSONSchema map[string]any `json:"schema,omitempty"`
}

type responsesResponse struct {
	OutputText string          `json:"output_text"`
	Output     []responseBlock `json:"output"`
}

type responseBlock struct {
	Content []responseContent `json:"content"`
}

type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (r responsesResponse) firstOutputText() string {
	for _, block := range r.Output {
		for _, content := range block.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return content.Text
			}
		}
	}
	return ""
}

type promptOwnerContextKey struct{}

func WithPromptOwner(ctx context.Context, ownerID string) context.Context {
	return context.WithValue(ctx, promptOwnerContextKey{}, strings.TrimSpace(ownerID))
}

func ownerIDFromContext(ctx context.Context) string {
	ownerID, _ := ctx.Value(promptOwnerContextKey{}).(string)
	return strings.TrimSpace(ownerID)
}

func (c *Client) postChatCompletions(ctx context.Context, settings ServiceSettings, systemPrompt, userPrompt string) (int, []byte, error) {
	payload := chatCompletionsRequest{
		Model:          settings.Model,
		MaxTokens:      1024,
		Messages:       []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt + "\n\n请只返回一个可解析的 JSON 对象，不要使用 Markdown 代码块，也不要添加 JSON 以外的文字。"}},
		ResponseFormat: nil,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal chat completions request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("create chat completions request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+settings.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	rawBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("read chat completions response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response.StatusCode, rawBody, nil
	}
	var decoded chatCompletionsResponse
	if err := json.Unmarshal(rawBody, &decoded); err != nil || len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return response.StatusCode, rawBody, nil
	}
	normalized, err := json.Marshal(responsesResponse{OutputText: decoded.Choices[0].Message.Content})
	if err != nil {
		return 0, nil, fmt.Errorf("normalize chat completions response: %w", err)
	}
	return response.StatusCode, normalized, nil
}

type chatCompletionsRequest struct {
	Model          string              `json:"model"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	Messages       []chatMessage       `json:"messages"`
	ResponseFormat *chatResponseFormat `json:"response_format,omitempty"`
}
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type chatResponseFormat struct {
	Type string `json:"type"`
}
type chatCompletionsResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}
type plainResponsesRequest struct {
	Model           string `json:"model"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
	Instructions    string `json:"instructions"`
	Input           string `json:"input"`
}
