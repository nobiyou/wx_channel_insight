package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/scheduler"
)

func TestHTTPCycleRunnerSendsCycleRequestAndHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != operatorCyclePath {
			t.Errorf("expected path %q, got %q", operatorCyclePath, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected JSON content type, got %q", got)
		}
		if got := r.Header.Get("X-Hub-User-ID"); got != "insight:operator:test" {
			t.Errorf("expected operator header, got %q", got)
		}
		if got := r.Header.Get("X-Hub-Device-Count"); got != "3" {
			t.Errorf("expected device count header, got %q", got)
		}
		if got := r.Header.Get("X-Insight-Bridge-Token"); got != "bridge-token" {
			t.Errorf("expected bridge token header, got %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if len(payload) != 0 {
			t.Errorf("expected empty monitoring tick request, got %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"completed","run":{"status":"success"}}`)
	}))
	defer server.Close()

	runner := newHTTPCycleRunner(scheduler.Config{
		APIBaseURL:     server.URL + "/",
		OperatorUserID: "insight:operator:test",
		DeviceCount:    3,
		BridgeToken:    "bridge-token",
		Strict:         true,
	}, server.Client())
	if err := runner.RunCycle(context.Background()); err != nil {
		t.Fatalf("RunCycle returned error: %v", err)
	}
}

func TestHTTPCycleRunnerOmitsUnsetOptionalHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Hub-User-ID"); got != "" {
			t.Errorf("expected no operator header, got %q", got)
		}
		if got := r.Header.Get("X-Hub-Device-Count"); got != "" {
			t.Errorf("expected no device header, got %q", got)
		}
		if got := r.Header.Get("X-Insight-Bridge-Token"); got != "" {
			t.Errorf("expected no bridge token header, got %q", got)
		}
		io.WriteString(w, `{"status":"idle"}`)
	}))
	defer server.Close()

	runner := newHTTPCycleRunner(scheduler.Config{APIBaseURL: server.URL, DeviceCount: -1}, server.Client())
	if err := runner.RunCycle(context.Background()); err != nil {
		t.Fatalf("RunCycle returned error: %v", err)
	}
}

func TestHTTPCycleRunnerRejectsNonSuccessCycleResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
	}{
		{name: "unknown tick status", statusCode: http.StatusOK, body: `{"status":"partial"}`, want: `monitoring tick returned status="partial"`},
		{name: "task error", statusCode: http.StatusOK, body: `{"status":"completed","error":"task failed"}`, want: `task error`},
		{name: "conflict", statusCode: http.StatusConflict, body: `{"status":"skipped","error":"cycle already running"}`, want: "HTTP 409"},
		{name: "strict failure", statusCode: http.StatusUnprocessableEntity, body: `{"status":"failed"}`, want: "HTTP 422"},
		{name: "malformed JSON", statusCode: http.StatusOK, body: `{not-json`, want: "decode monitoring tick response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				io.WriteString(w, tt.body)
			}))
			defer server.Close()

			runner := newHTTPCycleRunner(scheduler.Config{APIBaseURL: server.URL}, server.Client())
			err := runner.RunCycle(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestHTTPCycleRunnerReportsTransportErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	runner := newHTTPCycleRunner(scheduler.Config{APIBaseURL: baseURL}, http.DefaultClient)
	err := runner.RunCycle(context.Background())
	if err == nil || !strings.Contains(err.Error(), "monitoring tick request") {
		t.Fatalf("expected contextual transport error, got %v", err)
	}
}

func TestHTTPCycleRunnerRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", maxResponseBytes+1))
	}))
	defer server.Close()

	runner := newHTTPCycleRunner(scheduler.Config{APIBaseURL: server.URL}, server.Client())
	err := runner.RunCycle(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected response size error, got %v", err)
	}
}

func TestCycleEndpointRejectsInvalidBaseURL(t *testing.T) {
	_, err := cycleEndpoint("relative")
	if err == nil || !strings.Contains(err.Error(), "scheme and host") {
		t.Fatalf("expected base URL validation error, got %v", err)
	}
}

func TestHTTPCycleRunnerPropagatesCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := newHTTPCycleRunner(scheduler.Config{APIBaseURL: server.URL}, server.Client())
	err := runner.RunCycle(ctx)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
