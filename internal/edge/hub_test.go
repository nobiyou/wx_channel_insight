package edge

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func gzipTestMessage(t *testing.T, data []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("gzip edge message: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip edge message: %v", err)
	}
	return compressed.Bytes()
}

func TestHubRegisterListAndCallRoundTrip(t *testing.T) {
	hub := NewHub(WithLogger(func(string, ...any) {}))

	server := httptest.NewServer(http.HandlerFunc(hub.ServeWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"X-Client-ID": []string{"lan-pc-1"}},
	})
	if err != nil {
		t.Fatalf("dial edge: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// client read loop: answer api_call commands
	go func() {
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var msg CloudMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			if msg.Type != MsgTypeCommand {
				continue
			}
			respPayload, _ := json.Marshal(ResponsePayload{
				RequestID: msg.ID,
				Success:   true,
				Data:      json.RawMessage(`{"ok":true,"echo":"search"}`),
			})
			out, _ := json.Marshal(CloudMessage{
				ID:        msg.ID,
				Type:      MsgTypeResponse,
				ClientID:  "lan-pc-1",
				Payload:   respPayload,
				Timestamp: time.Now().Unix(),
			})
			_ = conn.Write(context.Background(), websocket.MessageText, out)
		}
	}()

	// heartbeat so SelectReady can see capabilities
	hb, _ := json.Marshal(HeartbeatPayload{
		Hostname:         "pc1",
		Status:           "running",
		APIReady:         true,
		ReadyClients:     1,
		FeedReadyClients: 1,
		SupportsFeed:     true,
		SupportsSearch:   true,
	})
	hbMsg, _ := json.Marshal(CloudMessage{
		ID:        "hb-1",
		Type:      MsgTypeHeartbeat,
		ClientID:  "lan-pc-1",
		Payload:   hb,
		Timestamp: time.Now().Unix(),
	})
	if err := conn.Write(ctx, websocket.MessageText, hbMsg); err != nil {
		t.Fatalf("send heartbeat: %v", err)
	}

	// wait briefly for heartbeat applied
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		list := hub.List()
		if len(list) == 1 && list[0].ReadyClients == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	list := hub.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 node, got %d", len(list))
	}
	if list[0].ID != "lan-pc-1" || list[0].ReadyClients != 1 {
		t.Fatalf("unexpected node info: %+v", list[0])
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer callCancel()
	data, err := hub.CallAPI(callCtx, "lan-pc-1", "key:channels:contact_list", map[string]any{"keyword": "test"}, time.Second)
	if err != nil {
		t.Fatalf("CallAPI: %v", err)
	}
	if !strings.Contains(string(data), `"ok":true`) {
		t.Fatalf("unexpected response data: %s", string(data))
	}

	summary := hub.Summary()
	if !summary.Enabled || summary.OnlineNodes != 1 || summary.ReadyNodes != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestServeWSRepliesToHeartbeat(t *testing.T) {
	hub := NewHub(WithLogger(func(string, ...any) {}))
	server := httptest.NewServer(http.HandlerFunc(hub.ServeWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"X-Client-ID": []string{"heartbeat-client"}},
	})
	if err != nil {
		t.Fatalf("dial edge: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	heartbeat, err := json.Marshal(CloudMessage{
		ID:        "heartbeat-1",
		Type:      MsgTypeHeartbeat,
		ClientID:  "heartbeat-client",
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("marshal heartbeat: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, heartbeat); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel()
	messageType, data, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("read heartbeat ACK: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("expected text heartbeat ACK, got %v", messageType)
	}
	var ack CloudMessage
	if err := json.Unmarshal(data, &ack); err != nil {
		t.Fatalf("decode heartbeat ACK: %v", err)
	}
	if ack.ID != "heartbeat-1" || ack.Type != MsgTypeHeartbeatAck || ack.ClientID != "heartbeat-client" {
		t.Fatalf("unexpected heartbeat ACK: %+v", ack)
	}
}

func TestServeWSDecodesLegacyGzipBinaryMessages(t *testing.T) {
	hub := NewHub(WithLogger(func(string, ...any) {}))
	server := httptest.NewServer(http.HandlerFunc(hub.ServeWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"X-Client-ID": []string{"legacy-gzip-client"}},
	})
	if err != nil {
		t.Fatalf("dial edge: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	payload, err := json.Marshal(HeartbeatPayload{
		Hostname: "legacy-client", Status: "running", APIReady: true,
		ReadyClients: 1, FeedReadyClients: 1, SupportsFeed: true,
	})
	if err != nil {
		t.Fatalf("marshal heartbeat payload: %v", err)
	}
	message, err := json.Marshal(CloudMessage{
		ID: "legacy-gzip-heartbeat", Type: MsgTypeHeartbeat,
		ClientID: "legacy-gzip-client", Payload: payload, Timestamp: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("marshal heartbeat: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, gzipTestMessage(t, message)); err != nil {
		t.Fatalf("write gzip heartbeat: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		items := hub.List()
		if len(items) == 1 && items[0].ReadyClients == 1 && items[0].FeedReadyClients == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected gzip heartbeat to update edge node, got %+v", hub.List())
}

func TestServeWSRequiresClientID(t *testing.T) {
	hub := NewHub(WithLogger(func(string, ...any) {}))
	req := httptest.NewRequest(http.MethodGet, "/ws/client", nil)
	rr := httptest.NewRecorder()
	hub.ServeWS(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestServeWSChecksSecret(t *testing.T) {
	hub := NewHub(WithSecret("s3cret"), WithLogger(func(string, ...any) {}))
	req := httptest.NewRequest(http.MethodGet, "/ws/client", nil)
	req.Header.Set("X-Client-ID", "n1")
	rr := httptest.NewRecorder()
	hub.ServeWS(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
