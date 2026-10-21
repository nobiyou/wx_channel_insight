package edge

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

// CommentTransport loads feed comment pages via edge api_call or local HTTP.
type CommentTransport struct {
	LocalBaseURL    string
	Hub             *Hub
	HTTPClient      *http.Client
	PreferClientID  string
	RequestInterval time.Duration
	Wait            func(context.Context, time.Duration) error
}

const defaultCommentRequestInterval = 1500 * time.Millisecond

type commentRequestPacer struct {
	interval time.Duration
	wait     func(context.Context, time.Duration) error
	started  bool
}

func newCommentRequestPacer(interval time.Duration, wait func(context.Context, time.Duration) error) *commentRequestPacer {
	if interval == 0 {
		interval = defaultCommentRequestInterval
	}
	if wait == nil {
		wait = waitForCommentRequest
	}
	return &commentRequestPacer{interval: interval, wait: wait}
}

func (p *commentRequestPacer) beforeRequest(ctx context.Context) error {
	if p == nil || p.interval < 0 {
		return nil
	}
	if !p.started {
		p.started = true
		return nil
	}
	return p.wait(ctx, p.interval)
}

func waitForCommentRequest(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (t *CommentTransport) SetPreferClientID(clientID string) {
	if t == nil {
		return
	}
	t.PreferClientID = strings.TrimSpace(clientID)
}

func (t CommentTransport) client() *http.Client {
	if t.HTTPClient != nil {
		return t.HTTPClient
	}
	return &http.Client{Timeout: 90 * time.Second}
}

// FetchCommentListPage returns the raw page payload for finder comment list.
func (t CommentTransport) FetchCommentListPage(ctx context.Context, objectID, nonceID, commentID, nextMarker string) (json.RawMessage, error) {
	objectID = strings.TrimSpace(objectID)
	if objectID == "" {
		return nil, fmt.Errorf("object_id is required")
	}
	body := map[string]any{
		"object_id":   objectID,
		"nonce_id":    strings.TrimSpace(nonceID),
		"comment_id":  strings.TrimSpace(commentID),
		"next_marker": strings.TrimSpace(nextMarker),
	}

	if t.Hub != nil {
		raw, err := t.Hub.CallAPI(ctx, t.PreferClientID, "key:channels:fetch_feed_comment_list", body, 90*time.Second)
		if err == nil {
			return raw, nil
		}
		if strings.TrimSpace(t.LocalBaseURL) == "" {
			return nil, err
		}
	}

	base := strings.TrimRight(strings.TrimSpace(t.LocalBaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("no edge hub and no local wx_channel base URL for comments")
	}
	q := url.Values{}
	q.Set("object_id", objectID)
	if v := strings.TrimSpace(nonceID); v != "" {
		q.Set("nonce_id", v)
	}
	if v := strings.TrimSpace(commentID); v != "" {
		q.Set("comment_id", v)
	}
	if v := strings.TrimSpace(nextMarker); v != "" {
		q.Set("next_marker", v)
	}
	return t.getLocal(ctx, base+"/api/channels/feed/comment/list?"+q.Encode())
}

func (t CommentTransport) getLocal(ctx context.Context, endpoint string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local comment list HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Data) > 0 {
		if envelope.Code != 0 {
			return nil, fmt.Errorf("local comment list error: %s", envelope.Message)
		}
		return envelope.Data, nil
	}
	return json.RawMessage(body), nil
}

// CollectAllComments paginates top-level comments and replies via transport.
func (t CommentTransport) CollectAllComments(ctx context.Context, objectID, nonceID string) (topLevel []map[string]any, reportedCount int, err error) {
	pacer := newCommentRequestPacer(t.RequestInterval, t.Wait)
	topLevel, reportedCount, err = t.fetchPages(ctx, objectID, nonceID, "", pacer)
	if err != nil {
		return nil, 0, err
	}
	for i := range topLevel {
		commentID := stringValue(topLevel[i]["commentId"])
		if commentID == "" || !commentHasReplies(topLevel[i]) {
			if _, ok := topLevel[i]["levelTwoComment"]; !ok {
				topLevel[i]["levelTwoComment"] = []map[string]any{}
			}
			continue
		}
		replies, _, rerr := t.fetchPages(ctx, objectID, "", commentID, pacer)
		if rerr != nil {
			return nil, 0, rerr
		}
		topLevel[i]["levelTwoComment"] = replies
	}
	return topLevel, reportedCount, nil
}

func (t CommentTransport) fetchPages(ctx context.Context, objectID, nonceID, commentID string, pacer *commentRequestPacer) ([]map[string]any, int, error) {
	items := make([]map[string]any, 0, 32)
	seen := map[string]struct{}{}
	nextMarker := ""
	reported := 0

	for page := 0; page < 50; page++ {
		if err := pacer.beforeRequest(ctx); err != nil {
			return nil, 0, err
		}
		raw, err := t.FetchCommentListPage(ctx, objectID, nonceID, commentID, nextMarker)
		if err != nil {
			return nil, 0, err
		}
		pageItems, pageReported, lastBuffer, err := parseCommentListPayload(raw)
		if err != nil {
			return nil, 0, err
		}
		if pageReported > 0 {
			reported = pageReported
		}
		pageNew := 0
		for _, item := range pageItems {
			key := stringValue(item["commentId"])
			if key == "" {
				key = fmt.Sprintf("idx-%d", len(items))
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if _, ok := item["levelTwoComment"]; !ok {
				item["levelTwoComment"] = []map[string]any{}
			}
			items = append(items, item)
			pageNew++
		}
		if lastBuffer == "" || pageNew == 0 {
			break
		}
		nextMarker = lastBuffer
	}
	return items, reported, nil
}

func parseCommentListPayload(raw json.RawMessage) (items []map[string]any, reported int, lastBuffer string, err error) {
	// Shape mirrors wx_channel feedCommentAPIResponse loosely.
	var resp struct {
		ErrCode int    `json:"errCode"`
		ErrMsg  string `json:"errMsg"`
		Data    struct {
			CommentInfo []map[string]any `json:"commentInfo"`
			LastBuffer  string           `json:"lastBuffer"`
			CountInfo   struct {
				CommentCount int `json:"commentCount"`
			} `json:"countInfo"`
		} `json:"data"`
		// sometimes nested again
		CommentInfo []map[string]any `json:"commentInfo"`
		LastBuffer  string           `json:"lastBuffer"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, 0, "", err
	}
	if resp.ErrCode != 0 {
		msg := strings.TrimSpace(resp.ErrMsg)
		if msg == "" {
			msg = fmt.Sprintf("comment list error code %d", resp.ErrCode)
		}
		return nil, 0, "", fmt.Errorf("%s", msg)
	}
	items = resp.Data.CommentInfo
	if len(items) == 0 {
		items = resp.CommentInfo
	}
	lastBuffer = resp.Data.LastBuffer
	if lastBuffer == "" {
		lastBuffer = resp.LastBuffer
	}
	reported = resp.Data.CountInfo.CommentCount
	if items == nil {
		items = []map[string]any{}
	}
	return items, reported, lastBuffer, nil
}

func stringValue(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case float64:
		return fmt.Sprintf("%.0f", value)
	case json.Number:
		return value.String()
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprintf("%v", value)
	}
}

func commentHasReplies(comment map[string]any) bool {
	if asInt(comment["expandCommentCount"]) > 0 {
		return true
	}
	if asInt(comment["continueFlag"]) > 0 {
		return true
	}
	if replies, ok := comment["levelTwoComment"].([]any); ok && len(replies) > 0 {
		return true
	}
	if replies, ok := comment["levelTwoComment"].([]map[string]any); ok && len(replies) > 0 {
		return true
	}
	return false
}

func asInt(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case float64:
		return int(value)
	case json.Number:
		i, _ := value.Int64()
		return int(i)
	case string:
		var n int
		fmt.Sscanf(value, "%d", &n)
		return n
	default:
		return 0
	}
}
