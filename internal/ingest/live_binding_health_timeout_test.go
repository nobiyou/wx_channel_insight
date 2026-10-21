package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchWXFeedListHealthBodyBoundsProbeTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"clients":1}`))
	}))
	defer server.Close()

	startedAt := time.Now()
	_, ok, failure := fetchWXFeedListHealthBody(context.Background(), server.URL, &http.Client{
		Timeout: 20 * time.Second,
	}, 1024)
	elapsed := time.Since(startedAt)

	if ok {
		t.Fatal("expected slow probe to time out instead of succeeding")
	}
	if failure == "" {
		t.Fatal("expected timeout failure detail")
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("expected health probe timeout to stay bounded, got %s", elapsed)
	}
}

func TestProbeWXVideoSearchHealthSurfacesStatusTimeoutDiagnostic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"data":{"clients":1,"ready_clients":1}}`))
	}))
	defer server.Close()

	assessment := probeWXVideoSearchHealth(context.Background(), server.URL, &http.Client{
		Timeout: 20 * time.Second,
	})

	if assessment.Runnable {
		t.Fatalf("expected status timeout to block video search, got %+v", assessment)
	}
	if assessment.Reason != wxVideoSearchHealthUnreachableReason {
		t.Fatalf("expected video-search health unreachable reason, got %+v", assessment)
	}
	if assessment.Diagnostic == nil || assessment.Diagnostic.Code != "wx_video_search_health_unreachable" {
		t.Fatalf("expected video-search health diagnostic, got %+v", assessment.Diagnostic)
	}
	if assessment.Diagnostic.StatusURL != server.URL+"/api/channels/status" {
		t.Fatalf("expected status URL in diagnostic, got %+v", assessment.Diagnostic)
	}
	if !strings.Contains(assessment.Diagnostic.WXChannelStatusError, "timeout") {
		t.Fatalf("expected status timeout diagnostic, got %+v", assessment.Diagnostic)
	}
	if !strings.Contains(assessment.Detail, "/api/channels/status") || !strings.Contains(assessment.Detail, "重启 wx_channel") {
		t.Fatalf("expected actionable status timeout detail, got %q", assessment.Detail)
	}
}

func TestBuildOperatorReadinessUsesWXChannelStatusRecoveryAction(t *testing.T) {
	readiness := buildOperatorReadiness([]SourceBindingState{{
		Source:      RunSourceWXVideoSearch,
		BaseURL:     "http://127.0.0.1:2026",
		Runnable:    false,
		CanRetry:    true,
		RetryReason: wxVideoSearchHealthUnreachableReason,
		Diagnostic: &SourceBindingDiagnostic{
			Code:                 "wx_video_search_health_unreachable",
			ResolvedBaseURL:      "http://127.0.0.1:2026",
			StatusURL:            "http://127.0.0.1:2026/api/channels/status",
			WXChannelStatusError: "timeout after 1s",
		},
	}})

	if readiness.BlockedSources != 1 || len(readiness.Blocked) != 1 {
		t.Fatalf("expected one blocked source, got %+v", readiness)
	}
	action := readiness.Blocked[0].Action
	if !strings.Contains(action, "wx_channel API 本身可响应") || !strings.Contains(action, "timeout after 1s") {
		t.Fatalf("expected wx_channel status recovery action, got %q", action)
	}
}
