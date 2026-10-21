package wxchannel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusClientFetchParsesReadyClients(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/status" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "success",
			"data": map[string]any{
				"connected":             true,
				"clients":               1,
				"ready_clients":         1,
				"search_ready_clients":  1,
				"feed_ready_clients":    1,
				"profile_ready_clients": 1,
				"comment_ready_clients": 1,
				"runtime": map[string]any{
					"proxy": map[string]any{
						"mode":     "process_proxy",
						"address":  "127.0.0.1:2025",
						"port":     2025,
						"api_port": 2026,
					},
					"injection": map[string]any{
						"checked": true,
						"started": true,
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewStatusClient(server.URL, server.Client())
	status, err := client.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !status.Connected || status.DeviceCount() != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if !status.ReadyForFeed() || !status.ReadyForComment() {
		t.Fatalf("expected feed/comment ready, got %+v", status)
	}
	if status.Runtime.Proxy.Mode != "process_proxy" || status.Runtime.Proxy.Address != "127.0.0.1:2025" {
		t.Fatalf("expected runtime proxy diagnostics, got %+v", status.Runtime.Proxy)
	}
}

func TestStatusClientFetchCapabilitiesDetectsMissingInsightRoutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/feed/search":
			if r.URL.RawQuery != "" {
				t.Fatalf("feedSearch capability probe should not hit page-client search data path, got query %q", r.URL.RawQuery)
			}
			http.NotFound(w, r)
		case "/api/channels/contact/feed/list":
			if r.URL.RawQuery != "" {
				t.Fatalf("feedList capability probe should not hit page-client feed data path, got query %q", r.URL.RawQuery)
			}
			http.Error(w, "username is required", http.StatusBadRequest)
		case "/api/channels/feed/comment/export":
			if r.ContentLength > 0 {
				t.Fatalf("commentExport capability probe should use route validation only, got content length %d", r.ContentLength)
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewStatusClient(server.URL, server.Client())
	capabilities := client.FetchCapabilities(context.Background())

	if capabilities.Ready {
		t.Fatalf("expected missing routes to make capabilities not ready, got %+v", capabilities)
	}
	if len(capabilities.Missing) != 2 || capabilities.Missing[0] != "feedSearch" || capabilities.Missing[1] != "commentExport" {
		t.Fatalf("expected missing feedSearch/commentExport, got %+v", capabilities.Missing)
	}
	feedList := capabilities.Route("feedList")
	if feedList == nil || !feedList.RoutePresent || feedList.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected feedList route to be present on HTTP 400, got %+v", feedList)
	}
}

func TestNormalizeBaseURLDefaultsTo2026(t *testing.T) {
	if got := normalizeBaseURL(""); got != "http://127.0.0.1:2026" {
		t.Fatalf("got %q", got)
	}
}
