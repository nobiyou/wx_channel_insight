package edge

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const maxCloudMessageBytes = 10 * 1024 * 1024

// ServeWS upgrades /ws/client for wx_channel cloud connectors.
// Compatible headers: X-Client-ID, X-Cloud-Secret (optional).
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if secret := h.Secret(); secret != "" {
		provided := strings.TrimSpace(r.Header.Get("X-Cloud-Secret"))
		if provided == "" {
			provided = strings.TrimSpace(r.URL.Query().Get("secret"))
		}
		if provided != secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	clientID := strings.TrimSpace(r.Header.Get("X-Client-ID"))
	if clientID == "" {
		clientID = strings.TrimSpace(r.URL.Query().Get("client_id"))
	}
	if clientID == "" {
		http.Error(w, "X-Client-ID required", http.StatusBadRequest)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// LAN agents are non-browser; Origin may be empty.
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionContextTakeover,
	})
	if err != nil {
		log.Printf("edge ws accept failed: %v", err)
		return
	}
	conn.SetReadLimit(10 * 1024 * 1024)

	remoteIP := clientIP(r)
	node := newNode(clientID, remoteIP, conn, h.now())
	h.Register(node)
	defer h.Unregister(node)

	ctx := r.Context()
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		msg, err := parseCloudMessage(msgType, data)
		if err != nil {
			h.logf("edge invalid message from %s: %v", clientID, err)
			continue
		}
		if err := h.handleNodeMessage(ctx, node, msg); err != nil {
			h.logf("edge message handling failed for %s: %v", clientID, err)
			return
		}
	}
}

func parseCloudMessage(messageType websocket.MessageType, data []byte) (CloudMessage, error) {
	decoded, err := decodeCloudMessageData(messageType, data)
	if err != nil {
		return CloudMessage{}, err
	}
	var msg CloudMessage
	if err := json.Unmarshal(decoded, &msg); err != nil {
		return CloudMessage{}, err
	}
	return msg, nil
}

func decodeCloudMessageData(messageType websocket.MessageType, data []byte) ([]byte, error) {
	if messageType != websocket.MessageBinary || len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode gzip edge message: %w", err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(io.LimitReader(reader, maxCloudMessageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read gzip edge message: %w", err)
	}
	if len(decoded) > maxCloudMessageBytes {
		return nil, fmt.Errorf("decoded edge message exceeds %d bytes", maxCloudMessageBytes)
	}
	return decoded, nil
}

func (h *Hub) handleNodeMessage(ctx context.Context, node *Node, msg CloudMessage) error {
	switch msg.Type {
	case MsgTypeHeartbeat:
		var hb HeartbeatPayload
		if len(msg.Payload) > 0 {
			_ = json.Unmarshal(msg.Payload, &hb)
		}
		node.applyHeartbeat(hb, h.now())
		ack, err := json.Marshal(CloudMessage{
			ID:        msg.ID,
			Type:      MsgTypeHeartbeatAck,
			ClientID:  node.ID,
			Timestamp: h.now().Unix(),
		})
		if err != nil {
			return fmt.Errorf("marshal heartbeat ACK: %w", err)
		}
		writeCtx, cancelWrite := context.WithTimeout(ctx, 5*time.Second)
		defer cancelWrite()
		if err := node.write(writeCtx, websocket.MessageText, ack); err != nil {
			return fmt.Errorf("send heartbeat ACK: %w", err)
		}
	case MsgTypeResponse:
		var resp ResponsePayload
		if err := json.Unmarshal(msg.Payload, &resp); err != nil {
			h.logf("edge invalid response from %s: %v", node.ID, err)
			return nil
		}
		reqID := strings.TrimSpace(resp.RequestID)
		if reqID == "" {
			reqID = strings.TrimSpace(msg.ID)
			resp.RequestID = reqID
		}
		node.complete(reqID, resp)
	default:
		// Ignore unknown messages so older or future clients cannot break the edge loop.
	}
	return nil
}

func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if rip := strings.TrimSpace(r.Header.Get("X-Real-IP")); rip != "" {
		return rip
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// HandleListNodes is GET /api/v1/edge/nodes
func (h *Hub) HandleListNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": h.List(),
		"count": len(h.List()),
	})
}

type remoteCallRequest struct {
	ClientID string          `json:"clientId"`
	Action   string          `json:"action"`
	Data     json.RawMessage `json:"data"`
	Timeout  string          `json:"timeout"`
	// Convenience for api_call without wrapping action/data.
	Key  string          `json:"key"`
	Body json.RawMessage `json:"body"`
}

// HandleRemoteCall is POST /api/v1/edge/call  (also used as Hub-like remote entry for Insight itself)
func (h *Hub) HandleRemoteCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req remoteCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	action := strings.TrimSpace(req.Action)
	data := req.Data
	if action == "" && strings.TrimSpace(req.Key) != "" {
		action = "api_call"
		payload, err := json.Marshal(APICallData{Key: strings.TrimSpace(req.Key), Body: req.Body})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		data = payload
	}
	if action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action or key is required"})
		return
	}

	timeout := 2 * time.Minute
	if strings.TrimSpace(req.Timeout) != "" {
		if parsed, err := time.ParseDuration(req.Timeout); err == nil && parsed > 0 {
			timeout = parsed
		}
	}

	ctx := r.Context()
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout+5*time.Second)
		defer cancel()
	}

	resp, err := h.Call(ctx, req.ClientID, action, data, timeout)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	status := http.StatusOK
	if !resp.Success {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, map[string]any{
		"success":   resp.Success,
		"requestId": resp.RequestID,
		"data":      json.RawMessage(resp.Data),
		"error":     resp.Error,
		"clientId":  strings.TrimSpace(req.ClientID),
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// Summary is a compact view for runtime status embedding.
type Summary struct {
	Enabled     bool `json:"enabled"`
	OnlineNodes int  `json:"onlineNodes"`
	ReadyNodes  int  `json:"readyNodes"`
	FeedNodes   int  `json:"feedNodes"`
}

func (h *Hub) Summary() Summary {
	if h == nil {
		return Summary{Enabled: false}
	}
	list := h.List()
	ready, feed := 0, 0
	for _, n := range list {
		if n.Status == "stale" {
			continue
		}
		if n.ReadyClients > 0 || n.APIReady {
			ready++
		}
		if n.FeedReadyClients > 0 || n.SupportsFeed {
			feed++
		}
	}
	return Summary{
		Enabled:     true,
		OnlineNodes: len(list),
		ReadyNodes:  ready,
		FeedNodes:   feed,
	}
}

// Ensure compile-time helpers used by tests.
var _ = fmt.Sprintf
