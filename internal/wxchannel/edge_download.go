package wxchannel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const edgeDownloadAPIKey = "key:channels:download_video"

// APICaller is the existing Edge API-call owner. It keeps the download path
// on the connected wx_channel page instead of exposing temporary media URLs.
type APICaller interface {
	CallAPI(context.Context, string, string, any, time.Duration) (json.RawMessage, error)
}

type EdgeDownloadClient struct {
	caller       APICaller
	statusClient *DownloadClient
	clientID     string
	startTimeout time.Duration
}

func NewEdgeDownloadClient(caller APICaller, statusClient *DownloadClient, clientID string) *EdgeDownloadClient {
	return &EdgeDownloadClient{
		caller:       caller,
		statusClient: statusClient,
		clientID:     strings.TrimSpace(clientID),
		startTimeout: 10 * time.Minute,
	}
}

func (c *EdgeDownloadClient) StartAndWait(ctx context.Context, input DownloadRequest) (DownloadRecord, error) {
	if c == nil || c.caller == nil {
		return DownloadRecord{}, fmt.Errorf("wx_channel edge download API is not configured")
	}
	if c.statusClient == nil {
		return DownloadRecord{}, fmt.Errorf("wx_channel download status API is not configured")
	}
	if err := validateDownloadRequest(input); err != nil {
		return DownloadRecord{}, err
	}

	timeout := c.startTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	response, err := c.caller.CallAPI(ctx, c.clientID, edgeDownloadAPIKey, buildDownloadPayload(input), timeout)
	if err != nil {
		return DownloadRecord{}, fmt.Errorf("start wx_channel edge download: %w", err)
	}
	if err := validateEdgeDownloadResponse(response); err != nil {
		return DownloadRecord{}, err
	}
	return c.statusClient.WaitForRecord(ctx, input.VideoID)
}

func validateDownloadRequest(input DownloadRequest) error {
	if strings.TrimSpace(input.VideoID) == "" {
		return fmt.Errorf("video id is required")
	}
	if strings.TrimSpace(input.VideoURL) == "" {
		return fmt.Errorf("video URL is required")
	}
	return nil
}

func buildDownloadPayload(input DownloadRequest) map[string]interface{} {
	payload := map[string]interface{}{
		"videoUrl":  input.VideoURL,
		"videoId":   input.VideoID,
		"title":     input.Title,
		"author":    input.Author,
		"forceSave": input.ForceSave,
	}
	if strings.TrimSpace(input.Key) != "" {
		payload["key"] = input.Key
	}
	if strings.TrimSpace(input.SourceURL) != "" {
		payload["sourceUrl"] = input.SourceURL
	}
	if input.ExpectedSize > 0 {
		payload["expectedSize"] = input.ExpectedSize
	}
	return payload
}

func validateEdgeDownloadResponse(raw json.RawMessage) error {
	var response struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Message string `json:"message"`
		ErrCode int    `json:"errCode"`
		ErrMsg  string `json:"errMsg"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return fmt.Errorf("decode wx_channel edge download response: %w", err)
	}
	if response.ErrCode != 0 || !response.Success {
		message := strings.TrimSpace(response.ErrMsg)
		if message == "" {
			message = strings.TrimSpace(response.Error)
		}
		if message == "" {
			message = strings.TrimSpace(response.Message)
		}
		if message == "" {
			message = "wx_channel rejected the download request"
		}
		return fmt.Errorf("edge download rejected: %s", message)
	}
	return nil
}
