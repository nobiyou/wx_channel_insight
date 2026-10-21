package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORSAllowsConfiguredOriginAndRejectsArbitraryOrigin(t *testing.T) {
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), []string{"http://127.0.0.1:5176"})

	allowed := httptest.NewRecorder()
	allowedRequest := httptest.NewRequest(http.MethodOptions, "/api/healthz", nil)
	allowedRequest.Header.Set("Origin", "http://127.0.0.1:5176")
	handler.ServeHTTP(allowed, allowedRequest)
	if allowed.Code != http.StatusNoContent || allowed.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5176" {
		t.Fatalf("configured origin was not allowed: status=%d headers=%v", allowed.Code, allowed.Header())
	}

	blocked := httptest.NewRecorder()
	blockedRequest := httptest.NewRequest(http.MethodOptions, "/api/healthz", nil)
	blockedRequest.Header.Set("Origin", "https://evil.example")
	handler.ServeHTTP(blocked, blockedRequest)
	if blocked.Code != http.StatusForbidden || blocked.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("arbitrary origin was not rejected: status=%d headers=%v", blocked.Code, blocked.Header())
	}
}
