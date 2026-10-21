package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultWXCommentExportPollInterval = time.Second
	defaultWXCommentExportPollTimeout  = 5 * time.Minute
)

// CommentListTransport fetches comment pages (local HTTP or edge api_call).
type CommentListTransport interface {
	CollectAllComments(ctx context.Context, objectID, nonceID string) (topLevel []map[string]any, reportedCount int, err error)
}

type WXCommentExportClient struct {
	BaseURL      string
	Client       *http.Client
	Transport    CommentListTransport
	PollInterval time.Duration
	PollTimeout  time.Duration
}

type WXCommentExportRequest struct {
	ObjectID string
	NonceID  string
	Title    string
	Author   string
}

type WXCommentExportResult struct {
	ObjectID     string
	SavedPath    string
	RelativePath string
	Title        string
	Author       string
	Source       string
}

func (c WXCommentExportClient) Export(ctx context.Context, req WXCommentExportRequest) (WXCommentExportResult, error) {
	req.ObjectID = strings.TrimSpace(req.ObjectID)
	req.NonceID = strings.TrimSpace(req.NonceID)
	req.Title = strings.TrimSpace(req.Title)
	req.Author = strings.TrimSpace(req.Author)
	if req.ObjectID == "" {
		return WXCommentExportResult{}, fmt.Errorf("object_id is required")
	}
	if req.NonceID == "" {
		return WXCommentExportResult{}, fmt.Errorf("nonce_id is required")
	}

	if c.Transport != nil {
		result, err := exportCommentsViaTransport(ctx, c.Transport, req)
		if err == nil {
			return result, nil
		}
		// fall back to collector-local HTTP export only when BaseURL set
		if strings.TrimSpace(c.BaseURL) == "" {
			return WXCommentExportResult{}, err
		}
	}

	baseURL := normalizeWXFeedListBaseURL(c.BaseURL)
	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	body, err := json.Marshal(map[string]string{
		"object_id": req.ObjectID,
		"nonce_id":  req.NonceID,
		"title":     req.Title,
		"author":    req.Author,
	})
	if err != nil {
		return WXCommentExportResult{}, err
	}

	endpoint := baseURL + "/api/channels/feed/comment/export"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return WXCommentExportResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return WXCommentExportResult{}, normalizeWXFeedListFetchError(baseURL, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return WXCommentExportResult{}, err
	}

	if resp.StatusCode == http.StatusAccepted {
		var queued wxCommentExportEnvelope
		if err := json.Unmarshal(responseBody, &queued); err != nil {
			return WXCommentExportResult{}, fmt.Errorf("wx_channel comment export returned invalid queued response: %w", err)
		}
		if queued.Code != 0 {
			return WXCommentExportResult{}, normalizeWXCommentExportMessage(baseURL, firstNonEmpty(queued.Message, "wx_channel comment export queue error"))
		}
		jobID := strings.TrimSpace(queued.Data.JobID)
		if jobID == "" {
			return WXCommentExportResult{}, fmt.Errorf("wx_channel comment export returned HTTP 202 without job_id")
		}
		return c.waitForCommentExport(ctx, baseURL, jobID, req)
	}

	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("wx_channel api returned HTTP %d", resp.StatusCode)
		var payload struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if len(responseBody) > 0 {
			if err := json.Unmarshal(responseBody, &payload); err == nil {
				switch {
				case strings.TrimSpace(payload.Message) != "":
					message = strings.TrimSpace(payload.Message)
				case strings.TrimSpace(payload.Error) != "":
					message = strings.TrimSpace(payload.Error)
				}
			}
		}
		return WXCommentExportResult{}, normalizeWXCommentExportMessage(baseURL, message)
	}

	var payload wxCommentExportEnvelope
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return WXCommentExportResult{}, err
	}
	if payload.Code != 0 {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = "wx_channel api error"
		}
		return WXCommentExportResult{}, normalizeWXCommentExportMessage(baseURL, message)
	}
	if strings.TrimSpace(payload.Data.SavedPath) == "" {
		return WXCommentExportResult{}, fmt.Errorf("wx_channel comment export did not return saved_path")
	}

	return buildWXCommentExportResult(payload.Data, req), nil
}

type wxCommentExportEnvelope struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    wxCommentExportJobData `json:"data"`
}

type wxCommentExportJobData struct {
	JobID        string                     `json:"job_id"`
	Status       string                     `json:"status"`
	Result       *wxCommentExportResultData `json:"result,omitempty"`
	Error        string                     `json:"error,omitempty"`
	ObjectID     string                     `json:"object_id"`
	SavedPath    string                     `json:"saved_path"`
	RelativePath string                     `json:"relative_path"`
	Title        string                     `json:"title"`
	Author       string                     `json:"author"`
	Source       string                     `json:"source"`
}

type wxCommentExportResultData struct {
	ObjectID     string `json:"object_id"`
	SavedPath    string `json:"saved_path"`
	RelativePath string `json:"relative_path"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	Source       string `json:"source"`
}

func (c WXCommentExportClient) waitForCommentExport(ctx context.Context, baseURL, jobID string, req WXCommentExportRequest) (WXCommentExportResult, error) {
	pollTimeout := c.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = defaultWXCommentExportPollTimeout
	}
	pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	interval := c.PollInterval
	if interval <= 0 {
		interval = defaultWXCommentExportPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	endpoint := strings.TrimRight(baseURL, "/") + "/api/channels/feed/comment/export/status?job_id=" + url.QueryEscape(jobID)
	for {
		select {
		case <-pollCtx.Done():
			return WXCommentExportResult{}, fmt.Errorf("wx_channel comment export job %s did not finish within %s: %w", jobID, pollTimeout, pollCtx.Err())
		case <-ticker.C:
			status, err := c.fetchCommentExportStatus(pollCtx, endpoint)
			if err != nil {
				return WXCommentExportResult{}, err
			}
			switch strings.ToLower(strings.TrimSpace(status.Status)) {
			case "succeeded", "success", "completed":
				if status.Result == nil && strings.TrimSpace(status.SavedPath) == "" {
					return WXCommentExportResult{}, fmt.Errorf("wx_channel comment export job %s succeeded without result", jobID)
				}
				result := buildWXCommentExportResult(status, req)
				if strings.TrimSpace(result.SavedPath) == "" {
					return WXCommentExportResult{}, fmt.Errorf("wx_channel comment export job %s succeeded without saved_path", jobID)
				}
				return result, nil
			case "failed", "error":
				message := firstNonEmpty(strings.TrimSpace(status.Error), "wx_channel comment export job failed")
				return WXCommentExportResult{}, normalizeWXCommentExportMessage(baseURL, message)
			}
		}
	}
}

func (c WXCommentExportClient) fetchCommentExportStatus(ctx context.Context, endpoint string) (wxCommentExportJobData, error) {
	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return wxCommentExportJobData{}, err
	}
	resp, err := httpClient.Do(request)
	if err != nil {
		return wxCommentExportJobData{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return wxCommentExportJobData{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return wxCommentExportJobData{}, fmt.Errorf("wx_channel comment export status returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload wxCommentExportEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return wxCommentExportJobData{}, err
	}
	if payload.Code != 0 {
		return wxCommentExportJobData{}, fmt.Errorf("wx_channel comment export status error: %s", firstNonEmpty(strings.TrimSpace(payload.Message), "code="+fmt.Sprint(payload.Code)))
	}
	if payload.Data.Result != nil {
		payload.Data.ObjectID = payload.Data.Result.ObjectID
		payload.Data.SavedPath = payload.Data.Result.SavedPath
		payload.Data.RelativePath = payload.Data.Result.RelativePath
		payload.Data.Title = payload.Data.Result.Title
		payload.Data.Author = payload.Data.Result.Author
		payload.Data.Source = payload.Data.Result.Source
	}
	return payload.Data, nil
}

func buildWXCommentExportResult(data wxCommentExportJobData, req WXCommentExportRequest) WXCommentExportResult {
	return WXCommentExportResult{
		ObjectID:     firstNonEmpty(strings.TrimSpace(data.ObjectID), req.ObjectID),
		SavedPath:    strings.TrimSpace(data.SavedPath),
		RelativePath: strings.TrimSpace(data.RelativePath),
		Title:        firstNonEmpty(strings.TrimSpace(data.Title), req.Title),
		Author:       firstNonEmpty(strings.TrimSpace(data.Author), req.Author),
		Source:       strings.TrimSpace(data.Source),
	}
}

func normalizeWXCommentExportMessage(baseURL, message string) error {
	normalized := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(normalized, "jsapi_jsonparse_failed"):
		return fmt.Errorf("当前客户端上下文与保存的评论导出视频不匹配。请先在视频号独立客户端中切到目标视频，等待当前视频信息完全加载后再重试；如果这是旧绑定，请重新导出评论或重新绑定当前视频")
	case strings.Contains(normalized, "no ready wechat page"),
		strings.Contains(normalized, "no available client"),
		strings.Contains(normalized, "no ready client"):
		return fmt.Errorf("wx_channel 已启动，但当前没有就绪的视频号客户端。请先在视频号独立客户端切到目标视频，等待 API 初始化完成后再重试评论导出")
	case strings.Contains(normalized, "http 503"):
		return fmt.Errorf("wx_channel API 当前不可用。请确认 %s 已启动，并检查当前视频号客户端采集链路是否在线", baseURL)
	default:
		return fmt.Errorf("%s", message)
	}
}

func exportCommentsViaTransport(ctx context.Context, transport CommentListTransport, req WXCommentExportRequest) (WXCommentExportResult, error) {
	topLevel, reported, err := transport.CollectAllComments(ctx, req.ObjectID, req.NonceID)
	if err != nil {
		return WXCommentExportResult{}, err
	}
	replyCount := 0
	for _, cmt := range topLevel {
		switch replies := cmt["levelTwoComment"].(type) {
		case []map[string]any:
			replyCount += len(replies)
		case []any:
			replyCount += len(replies)
		}
	}
	// Persist on Insight server so ImportWXCommentExport can read a local path.
	dir := filepath.Join("var", "edge-comment-exports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WXCommentExportResult{}, err
	}
	fileName := fmt.Sprintf("comment-%s-%d.json", sanitizeFilePart(req.ObjectID), time.Now().Unix())
	savedPath := filepath.Join(dir, fileName)
	doc := map[string]any{
		"objectId":             req.ObjectID,
		"objectNonceId":        req.NonceID,
		"title":                req.Title,
		"author":               req.Author,
		"commentInfo":          topLevel,
		"countInfo":            map[string]any{"commentCount": len(topLevel)},
		"originalCommentCount": reported,
		"savedAt":              time.Now().Format(time.RFC3339),
		"source":               "edge_fetch_feed_comment_list",
		"topLevelCount":        len(topLevel),
		"replyCount":           replyCount,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return WXCommentExportResult{}, err
	}
	if err := os.WriteFile(savedPath, raw, 0o644); err != nil {
		return WXCommentExportResult{}, err
	}
	abs, _ := filepath.Abs(savedPath)
	return WXCommentExportResult{
		ObjectID:     req.ObjectID,
		SavedPath:    abs,
		RelativePath: filepath.ToSlash(filepath.Join("var", "edge-comment-exports", fileName)),
		Title:        req.Title,
		Author:       req.Author,
		Source:       "edge_fetch_feed_comment_list",
	}, nil
}

func sanitizeFilePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}
