package wxchannel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DownloadStatusCompleted = "completed"
	DownloadStatusFailed    = "failed"
)

type DownloadRequest struct {
	VideoURL     string
	VideoID      string
	Title        string
	Author       string
	Key          string
	SourceURL    string
	ForceSave    bool
	ExpectedSize int64
}

type DownloadRecord struct {
	ID           string    `json:"id"`
	VideoID      string    `json:"videoId"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	FileSize     int64     `json:"fileSize"`
	FilePath     string    `json:"filePath"`
	RelativePath string    `json:"relativePath"`
	Status       string    `json:"status"`
	DownloadTime time.Time `json:"downloadTime"`
	ErrorMessage string    `json:"errorMessage"`
}

type DownloadClient struct {
	BaseURL      string
	Token        string
	HTTPClient   *http.Client
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func NewDownloadClient(baseURL, token string, httpClient *http.Client) *DownloadClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &DownloadClient{
		BaseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Token:        strings.TrimSpace(token),
		HTTPClient:   httpClient,
		PollInterval: 500 * time.Millisecond,
		PollTimeout:  30 * time.Minute,
	}
}

func (c *DownloadClient) StartAndWait(ctx context.Context, input DownloadRequest) (DownloadRecord, error) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" {
		return DownloadRecord{}, fmt.Errorf("wx_channel download API is not configured")
	}
	if err := validateDownloadRequest(input); err != nil {
		return DownloadRecord{}, err
	}
	if err := c.start(ctx, input); err != nil {
		return DownloadRecord{}, err
	}
	return c.WaitForRecord(ctx, input.VideoID)
}

func (c *DownloadClient) WaitForRecord(ctx context.Context, videoID string) (DownloadRecord, error) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" {
		return DownloadRecord{}, fmt.Errorf("wx_channel download API is not configured")
	}
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return DownloadRecord{}, fmt.Errorf("video id is required")
	}

	pollCtx := ctx
	var cancel context.CancelFunc
	if c.PollTimeout > 0 {
		pollCtx, cancel = context.WithTimeout(ctx, c.PollTimeout)
		defer cancel()
	}
	interval := c.PollInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		record, found, err := c.fetch(pollCtx, videoID)
		if err != nil {
			return DownloadRecord{}, err
		}
		if found {
			switch record.Status {
			case DownloadStatusCompleted:
				if strings.TrimSpace(record.RelativePath) == "" && strings.TrimSpace(record.FilePath) == "" {
					return DownloadRecord{}, fmt.Errorf("wx_channel download %q completed without relativePath or filePath", videoID)
				}
				return record, nil
			case DownloadStatusFailed:
				message := strings.TrimSpace(record.ErrorMessage)
				if message == "" {
					message = "wx_channel download failed"
				}
				return DownloadRecord{}, fmt.Errorf("wx_channel download %q failed: %s", videoID, message)
			}
		}

		select {
		case <-pollCtx.Done():
			return DownloadRecord{}, fmt.Errorf("waiting for wx_channel download %q: %w", videoID, pollCtx.Err())
		case <-ticker.C:
		}
	}
}

func (c *DownloadClient) start(ctx context.Context, input DownloadRequest) error {
	payload := buildDownloadPayload(input)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode wx_channel download request: %w", err)
	}
	status, responseBody, err := c.do(ctx, http.MethodPost, c.BaseURL+"/__wx_channels_api/download_video", body)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("wx_channel download start HTTP %d: %s", status, compactBody(responseBody))
	}
	var response struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return fmt.Errorf("decode wx_channel download start response: %w", err)
	}
	if !response.Success {
		message := strings.TrimSpace(response.Error)
		if message == "" {
			message = strings.TrimSpace(response.Message)
		}
		if message == "" {
			message = "wx_channel rejected the download request"
		}
		return fmt.Errorf("start wx_channel download: %s", message)
	}
	return nil
}

func (c *DownloadClient) fetch(ctx context.Context, videoID string) (DownloadRecord, bool, error) {
	endpoint := c.BaseURL + "/api/downloads/" + url.PathEscape(videoID)
	status, body, err := c.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return DownloadRecord{}, false, err
	}
	if status == http.StatusNotFound {
		return DownloadRecord{}, false, nil
	}
	if status < 200 || status >= 300 {
		return DownloadRecord{}, false, fmt.Errorf("wx_channel download status HTTP %d: %s", status, compactBody(body))
	}
	var response struct {
		Success bool           `json:"success"`
		Error   string         `json:"error"`
		Data    DownloadRecord `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return DownloadRecord{}, false, fmt.Errorf("decode wx_channel download status: %w", err)
	}
	if !response.Success {
		message := strings.TrimSpace(response.Error)
		if message == "" {
			message = "wx_channel returned an unsuccessful download status"
		}
		return DownloadRecord{}, false, fmt.Errorf("read wx_channel download status: %s", message)
	}
	return response.Data, true, nil
}

func (c *DownloadClient) do(ctx context.Context, method, endpoint string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("create wx_channel request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("X-Local-Auth", c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request wx_channel: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read wx_channel response: %w", err)
	}
	return resp.StatusCode, responseBody, nil
}

func compactBody(body []byte) string {
	message := strings.TrimSpace(string(body))
	if len(message) > 300 {
		return message[:300]
	}
	return message
}
