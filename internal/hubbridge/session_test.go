package hubbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFormatHubUserID(t *testing.T) {
	if got := FormatHubUserID(0); got != "" {
		t.Fatalf("expected empty for zero id, got %q", got)
	}
	if got := FormatHubUserID(42); got != "hub:user:42" {
		t.Fatalf("expected hub:user:42, got %q", got)
	}
}

func TestHTTPClientResolveUsesProfileAndDeviceList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/auth/profile":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    7,
				"email": "ops@example.com",
				"role":  "user",
			})
		case "/api/device/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"devices": []map[string]any{
					{"id": "machine-a"},
					{"id": "machine-b"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	client.SetCacheTTL(0)

	identity, err := client.Resolve(context.Background(), "good-token")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if identity.HubUserID != "hub:user:7" {
		t.Fatalf("unexpected hub user id: %+v", identity)
	}
	if identity.DeviceCount != 2 {
		t.Fatalf("expected device count 2, got %d", identity.DeviceCount)
	}
	if identity.Email != "ops@example.com" || identity.Source != "hub_session" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestHTTPClientResolveRejectsInvalidSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Resolve(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("expected invalid session error")
	}
}

func TestHTTPClientCachesIdentity(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    3,
			"email": "cache@example.com",
			"role":  "admin",
		})
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	client.SetNow(func() time.Time { return now })
	client.SetCacheTTL(time.Minute)

	if _, err := client.Resolve(context.Background(), "tok"); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if _, err := client.Resolve(context.Background(), "tok"); err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	// profile + optional device list on first call only; second should be cache hit
	if hits < 1 {
		t.Fatalf("expected at least one hub call, got %d", hits)
	}
	firstHits := hits
	if _, err := client.Resolve(context.Background(), "tok"); err != nil {
		t.Fatalf("third resolve: %v", err)
	}
	if hits != firstHits {
		t.Fatalf("expected cache hit with no extra hub calls, first=%d now=%d", firstHits, hits)
	}
}
