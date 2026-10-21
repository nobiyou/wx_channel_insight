package wxchannel

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

// PageClientStatus is the subset of /api/channels/status needed by Insight
// when Hub is not in the path and wx_channel :2026 is the data plane.
type PageClientStatus struct {
	BaseURL             string
	Connected           bool
	Clients             int
	ReadyClients        int
	SearchReadyClients  int
	FeedReadyClients    int
	ProfileReadyClients int
	CommentReadyClients int
	Runtime             RuntimeDiagnostics
	RawOK               bool
	Message             string
}

type RuntimeDiagnostics struct {
	Proxy       RuntimeProxyStatus       `json:"proxy"`
	Certificate RuntimeCertificateStatus `json:"certificate"`
	Injection   RuntimeInjectionStatus   `json:"injection"`
	Features    RuntimeFeatureStatus     `json:"features"`
}

type RuntimeProxyStatus struct {
	Mode    string `json:"mode"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Address string `json:"address"`
	APIPort int    `json:"api_port"`
}

type RuntimeCertificateStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Error     string `json:"error"`
}

type RuntimeInjectionStatus struct {
	Enabled       bool   `json:"enabled"`
	TargetProcess string `json:"target_process"`
	Checked       bool   `json:"checked"`
	Started       bool   `json:"started"`
	LastError     string `json:"last_error"`
	UpdatedAt     string `json:"updated_at"`
}

type RuntimeFeatureStatus struct {
	RadarEnabled   bool `json:"radar_enabled"`
	CloudEnabled   bool `json:"cloud_enabled"`
	MetricsEnabled bool `json:"metrics_enabled"`
}

type CapabilityStatus struct {
	BaseURL string
	Checked bool
	Ready   bool
	Missing []string
	Routes  []CapabilityRoute
}

type CapabilityRoute struct {
	Name         string
	Method       string
	URL          string
	StatusCode   int
	RoutePresent bool
	Error        string
}

type capabilityProbe struct {
	Name   string
	Method string
	Path   string
	Body   string
}

var insightCapabilityProbes = []capabilityProbe{
	{
		Name:   "feedSearch",
		Method: http.MethodGet,
		Path:   "/api/channels/feed/search",
	},
	{
		Name:   "feedList",
		Method: http.MethodGet,
		Path:   "/api/channels/contact/feed/list",
	},
	{
		Name:   "commentExport",
		Method: http.MethodPost,
		Path:   "/api/channels/feed/comment/export",
	},
}

// StatusClient probes a local wx_channel API base (default http://127.0.0.1:2026).
type StatusClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewStatusClient(baseURL string, httpClient *http.Client) *StatusClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}
	return &StatusClient{
		BaseURL:    normalizeBaseURL(baseURL),
		HTTPClient: httpClient,
	}
}

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "http://127.0.0.1:2026"
	}
	return baseURL
}

func (c *StatusClient) Fetch(ctx context.Context) (PageClientStatus, error) {
	base := normalizeBaseURL(c.BaseURL)
	endpoint := base + "/api/channels/status"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return PageClientStatus{BaseURL: base}, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return PageClientStatus{BaseURL: base, Message: err.Error()}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PageClientStatus{
			BaseURL: base,
			Message: fmt.Sprintf("wx_channel status HTTP %d", resp.StatusCode),
		}, fmt.Errorf("wx_channel status HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Connected           bool               `json:"connected"`
			Clients             int                `json:"clients"`
			ReadyClients        int                `json:"ready_clients"`
			SearchReadyClients  int                `json:"search_ready_clients"`
			FeedReadyClients    int                `json:"feed_ready_clients"`
			ProfileReadyClients int                `json:"profile_ready_clients"`
			CommentReadyClients int                `json:"comment_ready_clients"`
			Runtime             RuntimeDiagnostics `json:"runtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return PageClientStatus{BaseURL: base, Message: "invalid status JSON"}, err
	}
	if payload.Code != 0 {
		msg := strings.TrimSpace(payload.Message)
		if msg == "" {
			msg = fmt.Sprintf("wx_channel status code %d", payload.Code)
		}
		return PageClientStatus{BaseURL: base, Message: msg}, fmt.Errorf("%s", msg)
	}

	return PageClientStatus{
		BaseURL:             base,
		Connected:           payload.Data.Connected || payload.Data.ReadyClients > 0,
		Clients:             payload.Data.Clients,
		ReadyClients:        payload.Data.ReadyClients,
		SearchReadyClients:  payload.Data.SearchReadyClients,
		FeedReadyClients:    payload.Data.FeedReadyClients,
		ProfileReadyClients: payload.Data.ProfileReadyClients,
		CommentReadyClients: payload.Data.CommentReadyClients,
		Runtime:             payload.Data.Runtime,
		RawOK:               true,
		Message:             strings.TrimSpace(payload.Message),
	}, nil
}

// DeviceCount maps page-client readiness into the old Hub device_count field
// used by Insight workspace bootstrap.
func (s PageClientStatus) DeviceCount() int {
	if s.ReadyClients > 0 {
		return s.ReadyClients
	}
	if s.Clients > 0 {
		return s.Clients
	}
	if s.Connected {
		return 1
	}
	return 0
}

func (s CapabilityStatus) Route(name string) *CapabilityRoute {
	for i := range s.Routes {
		if s.Routes[i].Name == name {
			return &s.Routes[i]
		}
	}
	return nil
}

func (c *StatusClient) FetchCapabilities(ctx context.Context) CapabilityStatus {
	base := normalizeBaseURL(c.BaseURL)
	status := CapabilityStatus{
		BaseURL: base,
		Checked: true,
	}
	for _, probe := range insightCapabilityProbes {
		route := c.probeCapabilityRoute(ctx, base, probe)
		status.Routes = append(status.Routes, route)
		if !route.RoutePresent {
			status.Missing = append(status.Missing, probe.Name)
		}
	}
	status.Ready = len(status.Missing) == 0
	return status
}

func (c *StatusClient) probeCapabilityRoute(ctx context.Context, base string, probe capabilityProbe) CapabilityRoute {
	endpoint := base + probe.Path
	route := CapabilityRoute{
		Name:   probe.Name,
		Method: probe.Method,
		URL:    endpoint,
	}
	var body io.Reader
	if probe.Body != "" {
		body = bytes.NewBufferString(probe.Body)
	}
	req, err := http.NewRequestWithContext(ctx, probe.Method, endpoint, body)
	if err != nil {
		route.Error = err.Error()
		return route
	}
	if probe.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		route.Error = err.Error()
		return route
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	route.StatusCode = resp.StatusCode
	route.RoutePresent = capabilityStatusCodeMeansRoutePresent(resp.StatusCode)
	return route
}

func capabilityStatusCodeMeansRoutePresent(statusCode int) bool {
	return statusCode != 0 && statusCode != http.StatusNotFound && statusCode != http.StatusMethodNotAllowed
}

func (s PageClientStatus) ReadyForFeed() bool {
	return s.RawOK && s.FeedReadyClients > 0
}

func (s PageClientStatus) ReadyForComment() bool {
	return s.RawOK && s.CommentReadyClients > 0
}
