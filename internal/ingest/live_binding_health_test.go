package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeWXFeedListHealthFallsBackToWSHealthWhenStatusEndpointUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			http.NotFound(w, r)
		case "/ws/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":1,"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	assessment := probeWXFeedListHealth(context.Background(), server.URL, server.Client())
	if !assessment.Runnable {
		t.Fatalf("expected ws health fallback to keep binding runnable, got %+v", assessment)
	}
}

func TestProbeWXFeedListHealthBuildsStructuredStatusDiagnosticWhenFeedPageIsNotReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"message": "success",
				"data": {
					"clients": 1,
					"ready_clients": 1,
					"feed_ready_clients": 0,
					"client_list": [{
						"page_path": "/web/pages/search",
						"href": "https://channels.weixin.qq.com/web/pages/search",
						"api_ready": true,
						"supports_feed": false
					}]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	assessment := probeWXFeedListHealth(context.Background(), server.URL, server.Client())
	if assessment.Runnable {
		t.Fatalf("expected no-ready-feed-page assessment, got %+v", assessment)
	}
	if assessment.Reason != wxFeedListNoReadyClientReason {
		t.Fatalf("expected no-ready reason, got %+v", assessment)
	}
	if !strings.Contains(assessment.Detail, "/web/pages/search") {
		t.Fatalf("expected current page in detail, got %+v", assessment)
	}
	if assessment.Diagnostic == nil {
		t.Fatal("expected structured diagnostic")
	}
	if assessment.Diagnostic.StatusURL != server.URL+"/api/channels/status" {
		t.Fatalf("expected status URL in diagnostic, got %+v", assessment.Diagnostic)
	}
	if assessment.Diagnostic.ConnectedClientCount == nil || *assessment.Diagnostic.ConnectedClientCount != 1 {
		t.Fatalf("expected connected client count 1, got %+v", assessment.Diagnostic)
	}
	if assessment.Diagnostic.ReadyClientCount == nil || *assessment.Diagnostic.ReadyClientCount != 1 {
		t.Fatalf("expected ready client count 1, got %+v", assessment.Diagnostic)
	}
	if assessment.Diagnostic.FeedReadyClientCount == nil || *assessment.Diagnostic.FeedReadyClientCount != 0 {
		t.Fatalf("expected feed-ready client count 0, got %+v", assessment.Diagnostic)
	}
	if assessment.Diagnostic.CurrentPagePath != "/web/pages/search" {
		t.Fatalf("expected current page path in diagnostic, got %+v", assessment.Diagnostic)
	}
}

func TestAssessWXCommentExportBindingHealthBuildsStructuredStatusDiagnosticWhenPageIsNotReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"message": "success",
				"data": {
					"clients": 1,
					"ready_clients": 0,
					"feed_ready_clients": 0,
					"client_list": [{
						"page_path": "/web/pages/video/detail",
						"href": "https://channels.weixin.qq.com/web/pages/video/detail",
						"api_ready": false,
						"supports_feed": false
					}]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	assessment := assessWXCommentExportBindingHealth(context.Background(), server.URL, server.Client(), SourceBindingAssessment{
		Runnable:    true,
		PersistPath: `C:\temp\comments.json`,
	})
	if assessment.Runnable {
		t.Fatalf("expected no-ready comment export assessment, got %+v", assessment)
	}
	if assessment.Reason != wxCommentExportNoReadyClientReason {
		t.Fatalf("expected no-ready comment export reason, got %+v", assessment)
	}
	if !strings.Contains(assessment.Detail, "/web/pages/video/detail") {
		t.Fatalf("expected current page in detail, got %+v", assessment)
	}
	if assessment.Diagnostic == nil || assessment.Diagnostic.Code != "wx_comment_export_no_ready_page_client" {
		t.Fatalf("expected structured comment export diagnostic, got %+v", assessment)
	}
	if assessment.Diagnostic.ReadyClientCount == nil || *assessment.Diagnostic.ReadyClientCount != 0 {
		t.Fatalf("expected ready client count 0, got %+v", assessment.Diagnostic)
	}
	if assessment.Diagnostic.ConnectedClientCount == nil || *assessment.Diagnostic.ConnectedClientCount != 1 {
		t.Fatalf("expected connected client count 1, got %+v", assessment.Diagnostic)
	}
	if assessment.PersistPath != `C:\temp\comments.json` {
		t.Fatalf("expected persist path passthrough, got %+v", assessment)
	}
}

func TestProbeWXFeedListHealthUsesWXChannelProcessProxyRuntimeDiagnostic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"message": "success",
				"data": {
					"clients": 0,
					"ready_clients": 0,
					"feed_ready_clients": 0,
					"runtime": {
						"proxy": {"mode": "process_proxy", "address": "127.0.0.1:2025", "port": 2025},
						"injection": {"checked": true, "started": false, "last_error": "administrator permission may be required"},
						"certificate": {"installed": false, "error": "Cert drive unavailable"}
					}
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	assessment := probeWXFeedListHealth(context.Background(), server.URL, server.Client())
	if assessment.Runnable {
		t.Fatalf("expected process-proxy no-client assessment, got %+v", assessment)
	}
	if assessment.Diagnostic == nil || assessment.Diagnostic.Code != "wx_feed_list_process_proxy_no_client" {
		t.Fatalf("expected process proxy diagnostic, got %+v", assessment.Diagnostic)
	}
	if assessment.Diagnostic.WXChannelProxyMode != "process_proxy" || assessment.Diagnostic.WXChannelProxyAddress != "127.0.0.1:2025" {
		t.Fatalf("expected wx_channel proxy details, got %+v", assessment.Diagnostic)
	}
	if assessment.Diagnostic.WXChannelInjectionChecked == nil || !*assessment.Diagnostic.WXChannelInjectionChecked {
		t.Fatalf("expected injection checked diagnostic, got %+v", assessment.Diagnostic)
	}
	if assessment.Diagnostic.WXChannelInjectionStarted == nil || *assessment.Diagnostic.WXChannelInjectionStarted {
		t.Fatalf("expected failed injection diagnostic, got %+v", assessment.Diagnostic)
	}
	if assessment.Diagnostic.WXChannelCertificateInstalled == nil || *assessment.Diagnostic.WXChannelCertificateInstalled {
		t.Fatalf("expected certificate diagnostic, got %+v", assessment.Diagnostic)
	}
	if !strings.Contains(assessment.Detail, "进程代理") || !strings.Contains(assessment.Detail, "管理员权限") {
		t.Fatalf("expected process proxy/admin guidance, got %q", assessment.Detail)
	}
}
