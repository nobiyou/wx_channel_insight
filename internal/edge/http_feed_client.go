package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPFeedClient calls Insight edge API from the jobs process.
type HTTPFeedClient struct {
	InsightBaseURL string
	OperatorID     string
	BridgeToken    string
	HTTPClient     *http.Client
	PreferClientID string
}

func (c HTTPFeedClient) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 100 * time.Second}
}

func (c HTTPFeedClient) SearchAccounts(ctx context.Context, keyword string) (json.RawMessage, error) {
	body := map[string]any{
		"key":  "key:channels:contact_list",
		"body": map[string]any{"keyword": keyword},
	}
	if c.PreferClientID != "" {
		body["clientId"] = c.PreferClientID
	}
	return c.postCall(ctx, body)
}

func (c HTTPFeedClient) SearchVideos(ctx context.Context, keyword string) (json.RawMessage, error) {
	return c.SearchVideosPage(ctx, keyword, "")
}

func (c HTTPFeedClient) SearchVideosPage(ctx context.Context, keyword, marker string) (json.RawMessage, error) {
	body := map[string]any{
		"key":  "key:channels:contact_list",
		"body": map[string]any{"next_marker": marker, "keyword": keyword, "type": 3},
	}
	if c.PreferClientID != "" {
		body["clientId"] = c.PreferClientID
	}
	return c.postCall(ctx, body)
}

func (c HTTPFeedClient) FetchFeedList(ctx context.Context, username string) (json.RawMessage, error) {
	return c.FetchFeedListPage(ctx, username, "")
}

func (c HTTPFeedClient) FetchFeedListPage(ctx context.Context, username, marker string) (json.RawMessage, error) {
	body := map[string]any{
		"key":  "key:channels:feed_list",
		"body": map[string]any{"next_marker": marker, "username": username},
	}
	if c.PreferClientID != "" {
		body["clientId"] = c.PreferClientID
	}
	return c.postCall(ctx, body)
}

func (c HTTPFeedClient) FetchVideoProfile(ctx context.Context, objectID, nonceID string) (json.RawMessage, error) {
	body := map[string]any{
		"key": "key:channels:feed_profile",
		"body": map[string]any{
			"object_id": objectID,
			"nonce_id":  nonceID,
		},
	}
	if c.PreferClientID != "" {
		body["clientId"] = c.PreferClientID
	}
	return c.postCall(ctx, body)
}

func (c HTTPFeedClient) ResolveVideoShare(ctx context.Context, shareURL string) (json.RawMessage, error) {
	shareURL = strings.TrimSpace(shareURL)
	if shareURL == "" {
		return nil, fmt.Errorf("share url is required")
	}
	body := map[string]any{
		"key": "key:channels:shared_feed_profile",
		"body": map[string]any{
			"url": shareURL,
		},
	}
	if c.PreferClientID != "" {
		body["clientId"] = c.PreferClientID
	}
	return c.postCall(ctx, body)
}

func (c HTTPFeedClient) postCall(ctx context.Context, payload map[string]any) (json.RawMessage, error) {
	base := strings.TrimRight(strings.TrimSpace(c.InsightBaseURL), "/")
	if base == "" {
		base = "http://127.0.0.1:18081"
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/edge/call", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	op := strings.TrimSpace(c.OperatorID)
	if op == "" {
		op = "insight:operator:local"
	}
	req.Header.Set("X-Hub-User-ID", op)
	req.Header.Set("X-Hub-Device-Count", "1")
	if tok := strings.TrimSpace(c.BridgeToken); tok != "" {
		req.Header.Set("X-Insight-Bridge-Token", tok)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var out struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("edge call invalid response HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	if resp.StatusCode >= 300 || !out.Success {
		msg := strings.TrimSpace(out.Error)
		if msg == "" {
			msg = fmt.Sprintf("edge call HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return out.Data, nil
}

func (c *HTTPFeedClient) SetPreferClientID(clientID string) {
	if c == nil {
		return
	}
	c.PreferClientID = strings.TrimSpace(clientID)
}
