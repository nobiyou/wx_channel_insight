package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FeedTransport performs wx_channel feed/search either via local HTTP :2026
// or via a connected LAN edge node (remote api_call).
type FeedTransport struct {
	// LocalBaseURL is tried first when non-empty (e.g. http://127.0.0.1:2026 on a collector host).
	LocalBaseURL string
	// Hub is the multi-LAN edge gateway. When local HTTP is unavailable or empty, remote nodes are used.
	Hub *Hub
	// HTTPClient optional local HTTP client.
	HTTPClient *http.Client
	// PreferClientID optional sticky node id.
	PreferClientID string
}

func (t FeedTransport) httpClient() *http.Client {
	if t.HTTPClient != nil {
		return t.HTTPClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (t FeedTransport) SearchAccounts(ctx context.Context, keyword string) (json.RawMessage, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}
	// 1) local HTTP
	if base := strings.TrimRight(strings.TrimSpace(t.LocalBaseURL), "/"); base != "" {
		raw, err := t.getLocal(ctx, base+"/api/channels/contact/search?keyword="+url.QueryEscape(keyword))
		if err == nil {
			return raw, nil
		}
		// fall through to edge when local fails
		if t.Hub == nil {
			return nil, err
		}
	}
	if t.Hub == nil {
		return nil, fmt.Errorf("no local wx_channel base URL and edge gateway is disabled")
	}
	body := map[string]any{"keyword": keyword}
	return t.Hub.CallAPI(ctx, t.PreferClientID, "key:channels:contact_list", body, 90*time.Second)
}

func (t FeedTransport) SearchVideos(ctx context.Context, keyword string) (json.RawMessage, error) {
	return t.SearchVideosPage(ctx, keyword, "")
}

func (t FeedTransport) SearchVideosPage(ctx context.Context, keyword, marker string) (json.RawMessage, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}
	if base := strings.TrimRight(strings.TrimSpace(t.LocalBaseURL), "/"); base != "" {
		raw, err := t.getLocal(ctx, base+"/api/channels/feed/search?keyword="+url.QueryEscape(keyword)+"&next_marker="+url.QueryEscape(marker))
		if err == nil {
			return raw, nil
		}
		if t.Hub == nil {
			return nil, err
		}
	}
	if t.Hub == nil {
		return nil, fmt.Errorf("no local wx_channel base URL and edge gateway is disabled")
	}
	body := map[string]any{"next_marker": marker, "keyword": keyword, "type": 3}
	return t.Hub.CallAPI(ctx, t.PreferClientID, "key:channels:contact_list", body, 90*time.Second)
}

func (t FeedTransport) FetchFeedList(ctx context.Context, username string) (json.RawMessage, error) {
	return t.FetchFeedListPage(ctx, username, "")
}

func (t FeedTransport) FetchFeedListPage(ctx context.Context, username, marker string) (json.RawMessage, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if base := strings.TrimRight(strings.TrimSpace(t.LocalBaseURL), "/"); base != "" {
		raw, err := t.getLocal(ctx, base+"/api/channels/contact/feed/list?username="+url.QueryEscape(username)+"&next_marker="+url.QueryEscape(marker))
		if err == nil {
			return raw, nil
		}
		if t.Hub == nil {
			return nil, err
		}
	}
	if t.Hub == nil {
		return nil, fmt.Errorf("no local wx_channel base URL and edge gateway is disabled")
	}
	body := map[string]any{"next_marker": marker, "username": username}
	return t.Hub.CallAPI(ctx, t.PreferClientID, "key:channels:feed_list", body, 90*time.Second)
}

func (t FeedTransport) FetchVideoProfile(ctx context.Context, objectID, nonceID string) (json.RawMessage, error) {
	objectID = strings.TrimSpace(objectID)
	if objectID == "" {
		return nil, fmt.Errorf("object_id is required")
	}
	params := "?object_id=" + url.QueryEscape(objectID)
	if nonceID = strings.TrimSpace(nonceID); nonceID != "" {
		params += "&nonce_id=" + url.QueryEscape(nonceID)
	}
	if base := strings.TrimRight(strings.TrimSpace(t.LocalBaseURL), "/"); base != "" {
		raw, err := t.getLocal(ctx, base+"/api/channels/feed/profile"+params)
		if err == nil {
			return raw, nil
		}
		if t.Hub == nil {
			return nil, err
		}
	}
	if t.Hub == nil {
		return nil, fmt.Errorf("no local wx_channel base URL and edge gateway is disabled")
	}
	body := map[string]any{
		"object_id": objectID,
		"nonce_id":  nonceID,
	}
	return t.Hub.CallAPI(ctx, t.PreferClientID, "key:channels:feed_profile", body, 90*time.Second)
}

func (t FeedTransport) ResolveVideoShare(ctx context.Context, shareURL string) (json.RawMessage, error) {
	shareURL = strings.TrimSpace(shareURL)
	if shareURL == "" {
		return nil, fmt.Errorf("share url is required")
	}
	params := "?url=" + url.QueryEscape(shareURL)
	if base := strings.TrimRight(strings.TrimSpace(t.LocalBaseURL), "/"); base != "" {
		raw, err := t.getLocal(ctx, base+"/api/channels/feed/profile"+params)
		if err == nil {
			return raw, nil
		}
		if t.Hub == nil {
			return nil, err
		}
	}
	if t.Hub == nil {
		return nil, fmt.Errorf("no local wx_channel base URL and edge gateway is disabled")
	}
	return t.Hub.CallAPI(ctx, t.PreferClientID, "key:channels:shared_feed_profile", map[string]any{
		"url": shareURL,
	}, 90*time.Second)
}

func (t FeedTransport) getLocal(ctx context.Context, endpoint string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.httpClient().Do(req)
	if err != nil {
		if isConnectionError(err) {
			return nil, fmt.Errorf("local wx_channel unreachable: %w", err)
		}
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local wx_channel HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	// Normalize to the inner data payload when wrapped as {code,data}.
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Data) > 0 {
		if envelope.Code != 0 {
			return nil, fmt.Errorf("local wx_channel api error: %s", envelope.Message)
		}
		return envelope.Data, nil
	}
	return json.RawMessage(body), nil
}

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no connection") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "unreachable")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Decode helpers shared with ingest parsers: wrap raw edge/local payloads into
// the HTTP envelope shape expected by existing ingest unmarshallers when needed.
func WrapSuccessEnvelope(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return json.RawMessage(`{"code":0,"data":{}}`)
	}
	// If already an envelope with code, return as-is.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err == nil {
		if _, hasCode := probe["code"]; hasCode {
			return data
		}
	}
	var buf bytes.Buffer
	buf.WriteString(`{"code":0,"data":`)
	buf.Write(data)
	buf.WriteByte('}')
	return buf.Bytes()
}

func (t *FeedTransport) SetPreferClientID(clientID string) {
	if t == nil {
		return
	}
	t.PreferClientID = strings.TrimSpace(clientID)
}
