package edge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Hub tracks online LAN edge nodes (wx_channel instances) connected via /ws/client.
type Hub struct {
	mu         sync.RWMutex
	nodes      map[string]*Node
	secret     string
	now        func() time.Time
	logf       func(string, ...any)
	staleAfter time.Duration
}

type HubOption func(*Hub)

func WithSecret(secret string) HubOption {
	return func(h *Hub) {
		h.secret = strings.TrimSpace(secret)
	}
}

func WithNow(now func() time.Time) HubOption {
	return func(h *Hub) {
		if now != nil {
			h.now = now
		}
	}
}

func WithLogger(logf func(string, ...any)) HubOption {
	return func(h *Hub) {
		if logf != nil {
			h.logf = logf
		}
	}
}

func NewHub(opts ...HubOption) *Hub {
	h := &Hub{
		nodes:      make(map[string]*Node),
		now:        time.Now,
		logf:       log.Printf,
		staleAfter: 90 * time.Second,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *Hub) Secret() string {
	return h.secret
}

func (h *Hub) Register(node *Node) {
	if node == nil || strings.TrimSpace(node.ID) == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if prev, ok := h.nodes[node.ID]; ok && prev != node {
		prev.closeLocked("replaced by new connection")
	}
	h.nodes[node.ID] = node
	h.logf("edge node online: id=%s ip=%s", node.ID, node.RemoteIP)
}

func (h *Hub) Unregister(node *Node) {
	if node == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	current, ok := h.nodes[node.ID]
	if !ok || current != node {
		return
	}
	delete(h.nodes, node.ID)
	node.closeLocked("disconnected")
	h.logf("edge node offline: id=%s", node.ID)
}

func (h *Hub) List() []NodeInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	now := h.now()
	out := make([]NodeInfo, 0, len(h.nodes))
	for _, node := range h.nodes {
		info := node.Info()
		if !info.LastSeenAt.IsZero() && now.Sub(info.LastSeenAt) > h.staleAfter {
			info.Status = "stale"
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReadyClients != out[j].ReadyClients {
			return out[i].ReadyClients > out[j].ReadyClients
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (h *Hub) Get(id string) (*Node, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	node, ok := h.nodes[strings.TrimSpace(id)]
	return node, ok
}

// SelectReady prefers a node that can serve feed/search with local intercepted clients.
func (h *Hub) SelectReady(preferFeed bool) (*Node, error) {
	return h.SelectForCapability(preferFeed, false)
}

// SelectForCapability picks an online node. preferFeed weights feed-ready nodes;
// preferSearch weights search-ready nodes.
func (h *Hub) SelectForCapability(preferFeed, preferSearch bool) (*Node, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var best *Node
	bestScore := -1
	now := h.now()
	for _, node := range h.nodes {
		info := node.Info()
		if !info.LastSeenAt.IsZero() && now.Sub(info.LastSeenAt) > h.staleAfter {
			continue
		}
		if info.ReadyClients <= 0 && !info.APIReady {
			continue
		}
		score := info.ReadyClients
		if preferFeed {
			if !info.SupportsFeed && info.FeedReadyClients == 0 {
				continue
			}
			score = info.FeedReadyClients*10 + info.ReadyClients
		}
		if preferSearch {
			if !info.SupportsSearch && info.SearchReadyClients == 0 {
				// still allow ready nodes; search capability may not be reported on older clients
				score += info.SearchReadyClients * 5
			} else {
				score += info.SearchReadyClients*10 + 5
			}
		}
		if score > bestScore {
			bestScore = score
			best = node
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no ready edge node online")
	}
	return best, nil
}

func (h *Hub) Call(ctx context.Context, clientID, action string, data json.RawMessage, timeout time.Duration) (ResponsePayload, error) {
	clientID = strings.TrimSpace(clientID)
	action = strings.TrimSpace(action)
	if action == "" {
		return ResponsePayload{}, fmt.Errorf("action is required")
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}

	var node *Node
	var err error
	if clientID != "" {
		var ok bool
		node, ok = h.Get(clientID)
		if !ok {
			return ResponsePayload{}, fmt.Errorf("edge node offline: %s", clientID)
		}
	} else {
		preferFeed := strings.Contains(action, "feed")
		preferSearch := strings.Contains(action, "search") || strings.Contains(action, "contact")
		// api_call without hints: any ready node
		if action == "api_call" && !preferFeed && !preferSearch {
			preferFeed = true
			preferSearch = true
		}
		node, err = h.SelectForCapability(preferFeed, preferSearch)
		if err != nil {
			return ResponsePayload{}, err
		}
	}

	reqID := fmt.Sprintf("insight-%d", h.now().UnixNano())
	cmdPayload, err := json.Marshal(CommandPayload{Action: action, Data: data})
	if err != nil {
		return ResponsePayload{}, err
	}
	msg := CloudMessage{
		ID:        reqID,
		Type:      MsgTypeCommand,
		ClientID:  "insight-server",
		Payload:   cmdPayload,
		Timestamp: h.now().Unix(),
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return ResponsePayload{}, err
	}

	respCh := make(chan ResponsePayload, 1)
	node.track(reqID, respCh)
	defer node.untrack(reqID)

	writeCtx, cancelWrite := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWrite()
	if err := node.write(writeCtx, websocket.MessageText, raw); err != nil {
		return ResponsePayload{}, fmt.Errorf("send to edge node failed: %w", err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ResponsePayload{}, ctx.Err()
	case <-timer.C:
		return ResponsePayload{}, fmt.Errorf("edge call timeout after %s", timeout)
	case resp := <-respCh:
		return resp, nil
	}
}

// CallAPI is a convenience wrapper for action=api_call with key/body.
func (h *Hub) CallAPI(ctx context.Context, clientID, key string, body any, timeout time.Duration) (json.RawMessage, error) {
	bodyRaw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(APICallData{Key: key, Body: bodyRaw})
	if err != nil {
		return nil, err
	}
	// When client not specified, pick by API key capability.
	if strings.TrimSpace(clientID) == "" {
		preferFeed := strings.Contains(key, "feed")
		preferSearch := strings.Contains(key, "contact") || strings.Contains(key, "search")
		if node, selErr := h.SelectForCapability(preferFeed, preferSearch); selErr == nil && node != nil {
			clientID = node.ID
		}
	}
	resp, err := h.Call(ctx, clientID, "api_call", data, timeout)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		if resp.Error != "" {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return nil, fmt.Errorf("edge api_call failed")
	}
	return resp.Data, nil
}
