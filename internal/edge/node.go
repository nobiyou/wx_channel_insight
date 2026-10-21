package edge

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Node is one online wx_channel edge instance (often one LAN PC).
type Node struct {
	ID       string
	RemoteIP string
	conn     *websocket.Conn

	mu      sync.RWMutex
	writeMu sync.Mutex
	info    NodeInfo
	pending map[string]chan ResponsePayload
	closed  bool
}

type NodeInfo struct {
	ID                  string          `json:"id"`
	RemoteIP            string          `json:"remoteIp,omitempty"`
	Hostname            string          `json:"hostname,omitempty"`
	Version             string          `json:"version,omitempty"`
	Status              string          `json:"status,omitempty"`
	PagePath            string          `json:"pagePath,omitempty"`
	Href                string          `json:"href,omitempty"`
	APIReady            bool            `json:"apiReady"`
	WSClients           int             `json:"wsClients"`
	ReadyClients        int             `json:"readyClients"`
	SearchReadyClients  int             `json:"searchReadyClients"`
	FeedReadyClients    int             `json:"feedReadyClients"`
	ProfileReadyClients int             `json:"profileReadyClients"`
	SupportsSearch      bool            `json:"supportsSearch"`
	SupportsFeed        bool            `json:"supportsFeed"`
	SupportsProfile     bool            `json:"supportsProfile"`
	Methods             map[string]bool `json:"methods,omitempty"`
	ConnectedAt         time.Time       `json:"connectedAt"`
	LastSeenAt          time.Time       `json:"lastSeenAt"`
}

func newNode(id, remoteIP string, conn *websocket.Conn, now time.Time) *Node {
	return &Node{
		ID:       id,
		RemoteIP: remoteIP,
		conn:     conn,
		pending:  make(map[string]chan ResponsePayload),
		info: NodeInfo{
			ID:          id,
			RemoteIP:    remoteIP,
			Status:      "connected",
			ConnectedAt: now,
			LastSeenAt:  now,
		},
	}
}

func (n *Node) Info() NodeInfo {
	n.mu.RLock()
	defer n.mu.RUnlock()
	info := n.info
	if n.info.Methods != nil {
		methods := make(map[string]bool, len(n.info.Methods))
		for k, v := range n.info.Methods {
			methods[k] = v
		}
		info.Methods = methods
	}
	return info
}

func (n *Node) applyHeartbeat(hb HeartbeatPayload, at time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.info.Hostname = hb.Hostname
	n.info.Version = hb.Version
	if hb.Status != "" {
		n.info.Status = hb.Status
	} else {
		n.info.Status = "running"
	}
	n.info.PagePath = hb.PagePath
	n.info.Href = hb.Href
	n.info.APIReady = hb.APIReady
	n.info.WSClients = hb.WSClients
	n.info.ReadyClients = hb.ReadyClients
	n.info.SearchReadyClients = hb.SearchReadyClients
	n.info.FeedReadyClients = hb.FeedReadyClients
	n.info.ProfileReadyClients = hb.ProfileReadyClients
	n.info.SupportsSearch = hb.SupportsSearch || hb.SearchReadyClients > 0
	n.info.SupportsFeed = hb.SupportsFeed || hb.FeedReadyClients > 0
	n.info.SupportsProfile = hb.SupportsProfile || hb.ProfileReadyClients > 0
	if hb.Methods != nil {
		n.info.Methods = hb.Methods
	}
	n.info.LastSeenAt = at
}

func (n *Node) track(reqID string, ch chan ResponsePayload) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pending[reqID] = ch
}

func (n *Node) untrack(reqID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.pending, reqID)
}

func (n *Node) complete(reqID string, resp ResponsePayload) {
	n.mu.Lock()
	ch, ok := n.pending[reqID]
	if ok {
		delete(n.pending, reqID)
	}
	n.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- resp:
	default:
	}
}

// write serializes all data frames sent to an edge client. coder/websocket
// permits one concurrent reader and one concurrent writer per connection;
// heartbeat ACKs and remote commands can otherwise race on the writer.
func (n *Node) write(ctx context.Context, messageType websocket.MessageType, data []byte) error {
	n.writeMu.Lock()
	defer n.writeMu.Unlock()

	if n.conn == nil {
		return errors.New("edge node connection is nil")
	}
	return n.conn.Write(ctx, messageType, data)
}

func (n *Node) closeLocked(reason string) {
	if n.closed {
		return
	}
	n.closed = true
	for id, ch := range n.pending {
		select {
		case ch <- ResponsePayload{RequestID: id, Success: false, Error: reason}:
		default:
		}
		delete(n.pending, id)
	}
	if n.conn != nil {
		_ = n.conn.Close(websocket.StatusNormalClosure, reason)
	}
}
