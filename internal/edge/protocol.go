package edge

import "encoding/json"

// Wire protocol is the active wx_channel/internal/cloud Edge subset.
// Existing LAN clients can set cloud_hub_url to the Insight server without a code change.

type MessageType string

const (
	MsgTypeHeartbeat    MessageType = "heartbeat"
	MsgTypeHeartbeatAck MessageType = "heartbeat_ack"
	MsgTypeCommand      MessageType = "command"
	MsgTypeResponse     MessageType = "response"
)

type CloudMessage struct {
	ID         string          `json:"id"`
	Type       MessageType     `json:"type"`
	ClientID   string          `json:"client_id"`
	Payload    json.RawMessage `json:"payload"`
	Timestamp  int64           `json:"timestamp"`
	Compressed bool            `json:"compressed,omitempty"`
}

type HeartbeatPayload struct {
	Hostname            string          `json:"hostname"`
	Version             string          `json:"version"`
	Status              string          `json:"status"`
	HardwareFingerprint string          `json:"hardware_fingerprint,omitempty"`
	PagePath            string          `json:"page_path,omitempty"`
	Href                string          `json:"href,omitempty"`
	APIReady            bool            `json:"api_ready,omitempty"`
	WSClients           int             `json:"ws_clients,omitempty"`
	ReadyClients        int             `json:"ready_clients,omitempty"`
	SearchReadyClients  int             `json:"search_ready_clients,omitempty"`
	FeedReadyClients    int             `json:"feed_ready_clients,omitempty"`
	ProfileReadyClients int             `json:"profile_ready_clients,omitempty"`
	SupportsSearch      bool            `json:"supports_search,omitempty"`
	SupportsFeed        bool            `json:"supports_feed,omitempty"`
	SupportsProfile     bool            `json:"supports_profile,omitempty"`
	Methods             map[string]bool `json:"methods,omitempty"`
}

type CommandPayload struct {
	Action string          `json:"action"`
	Data   json.RawMessage `json:"data"`
}

type ResponsePayload struct {
	RequestID string          `json:"request_id"`
	Success   bool            `json:"success"`
	Data      json.RawMessage `json:"data"`
	Error     string          `json:"error"`
}

// APICallData is the body of action "api_call" (and mapped search/download actions on the client).
type APICallData struct {
	Key  string          `json:"key"`
	Body json.RawMessage `json:"body"`
}
