package insightai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxVerificationResponseBytes = 1024 * 1024

type VerificationResult struct {
	OK         bool   `json:"ok"`
	Status     string `json:"status"`
	ErrorCode  string `json:"errorCode,omitempty"`
	Message    string `json:"message"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	LatencyMS  int64  `json:"latencyMs"`
	VerifiedAt string `json:"verifiedAt"`
}

func VerifyConnection(ctx context.Context, settings ServiceSettings, client *http.Client) VerificationResult {
	startedAt := time.Now()
	result := VerificationResult{
		Status:     VerificationFailed,
		Provider:   ProviderOpenAI,
		Model:      DefaultModel,
		VerifiedAt: startedAt.UTC().Format(time.RFC3339),
	}
	settings, err := NormalizeServiceSettings(settings)
	if err != nil {
		result.ErrorCode = "invalid_config"
		result.Message = err.Error()
		return result
	}
	result.Provider = settings.Provider
	result.Model = settings.Model
	if settings.APIKey == "" {
		result.ErrorCode = "not_configured"
		result.Message = ErrNotConfigured.Error()
		return result
	}
	payload, _ := json.Marshal(map[string]any{
		"model":             settings.Model,
		"input":             "Reply with OK.",
		"max_output_tokens": 16,
	})
	requestCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, settings.BaseURL+"/responses", bytes.NewReader(payload))
	if err != nil {
		result.ErrorCode = "invalid_request"
		result.Message = "无法创建 OpenAI 验证请求"
		return result
	}
	request.Header.Set("Authorization", "Bearer "+settings.APIKey)
	request.Header.Set("Content-Type", "application/json")
	if client == nil {
		client = &http.Client{}
	}
	response, err := client.Do(request)
	result.LatencyMS = time.Since(startedAt).Milliseconds()
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(requestCtx.Err(), context.DeadlineExceeded):
			result.ErrorCode = "timeout"
			result.Message = "OpenAI 连接验证超时"
		default:
			result.ErrorCode = "network"
			result.Message = "无法连接 OpenAI 服务"
		}
		return result
	}
	defer response.Body.Close()
	result.HTTPStatus = response.StatusCode
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxVerificationResponseBytes))
	if readErr != nil {
		result.ErrorCode = "invalid_response"
		result.Message = "无法读取 OpenAI 验证响应"
		return result
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			result.ErrorCode = "authentication"
			result.Message = "API Key 无效或没有访问权限"
		case http.StatusTooManyRequests:
			result.ErrorCode = "rate_limit"
			result.Message = "OpenAI 请求受到限流或账户额度不足"
		default:
			result.ErrorCode = "upstream"
			result.Message = "OpenAI 服务返回异常状态"
		}
		return result
	}
	var decoded struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &decoded) != nil || strings.TrimSpace(decoded.ID) == "" {
		result.ErrorCode = "invalid_response"
		result.Message = "OpenAI 返回了无法识别的响应"
		return result
	}
	if strings.TrimSpace(result.Model) == "" {
		result.Model = DefaultModel
	}
	result.OK = true
	result.Status = VerificationVerified
	result.Message = "OpenAI API 连接正常"
	return result
}
