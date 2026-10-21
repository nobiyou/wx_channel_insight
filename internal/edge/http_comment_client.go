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

// HTTPCommentClient fetches comment pages through Insight /api/v1/edge/call (jobs process).
type HTTPCommentClient struct {
	InsightBaseURL  string
	OperatorID      string
	BridgeToken     string
	HTTPClient      *http.Client
	PreferClientID  string
	RequestInterval time.Duration
	Wait            func(context.Context, time.Duration) error
}

func (c *HTTPCommentClient) SetPreferClientID(clientID string) {
	if c == nil {
		return
	}
	c.PreferClientID = strings.TrimSpace(clientID)
}

func (c HTTPCommentClient) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 100 * time.Second}
}

func (c HTTPCommentClient) FetchCommentListPage(ctx context.Context, objectID, nonceID, commentID, nextMarker string) (json.RawMessage, error) {
	body := map[string]any{
		"key": "key:channels:fetch_feed_comment_list",
		"body": map[string]any{
			"object_id":   objectID,
			"nonce_id":    nonceID,
			"comment_id":  commentID,
			"next_marker": nextMarker,
		},
	}
	if c.PreferClientID != "" {
		body["clientId"] = c.PreferClientID
	}
	return c.postCall(ctx, body)
}

func (c HTTPCommentClient) CollectAllComments(ctx context.Context, objectID, nonceID string) ([]map[string]any, int, error) {
	pacer := newCommentRequestPacer(c.RequestInterval, c.Wait)
	topLevel, reported, err := fetchCommentPagesVia(ctx, c, objectID, nonceID, "", pacer)
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
		replies, _, rerr := fetchCommentPagesVia(ctx, c, objectID, "", commentID, pacer)
		if rerr != nil {
			return nil, 0, rerr
		}
		topLevel[i]["levelTwoComment"] = replies
	}
	return topLevel, reported, nil
}

type commentPageFetcher interface {
	FetchCommentListPage(ctx context.Context, objectID, nonceID, commentID, nextMarker string) (json.RawMessage, error)
}

func fetchCommentPagesVia(ctx context.Context, c commentPageFetcher, objectID, nonceID, commentID string, pacer *commentRequestPacer) ([]map[string]any, int, error) {
	items := make([]map[string]any, 0, 32)
	seen := map[string]struct{}{}
	nextMarker := ""
	reported := 0
	for page := 0; page < 50; page++ {
		if err := pacer.beforeRequest(ctx); err != nil {
			return nil, 0, err
		}
		raw, err := c.FetchCommentListPage(ctx, objectID, nonceID, commentID, nextMarker)
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

func (c HTTPCommentClient) postCall(ctx context.Context, payload map[string]any) (json.RawMessage, error) {
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
		return nil, fmt.Errorf("edge comment call invalid response HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	if resp.StatusCode >= 300 || !out.Success {
		msg := strings.TrimSpace(out.Error)
		if msg == "" {
			msg = fmt.Sprintf("edge comment call HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return out.Data, nil
}
