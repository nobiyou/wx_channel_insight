package ingest

import (
	"strings"
	"testing"
)

func TestBuildNoReadyWXFeedListDetailIncludesWindowsProxyMismatchHint(t *testing.T) {
	original := readWindowsInternetProxySettings
	readWindowsInternetProxySettings = func() (internetProxySettings, bool) {
		return internetProxySettings{
			Enabled: true,
			Server:  "127.0.0.1:10808",
		}, true
	}
	defer func() {
		readWindowsInternetProxySettings = original
	}()

	zero := 0
	detail, diagnostic := buildNoReadyWXFeedListDetail(
		"http://127.0.0.1:2026",
		"http://127.0.0.1:2026/ws/health",
		&zero,
		"ok",
	)
	if detail == "" {
		t.Fatalf("expected proxy mismatch detail")
	}
	if !containsAll(detail, "10808", "2025", "feed list has no ready page client") {
		t.Fatalf("expected proxy mismatch detail to mention current and expected proxy, got %q", detail)
	}
	if diagnostic == nil || diagnostic.Code != "wx_feed_list_proxy_mismatch" {
		t.Fatalf("expected proxy mismatch diagnostic, got %+v", diagnostic)
	}
	if diagnostic.ReadyClientCount == nil || *diagnostic.ReadyClientCount != 0 {
		t.Fatalf("expected ready client count 0, got %+v", diagnostic)
	}
	if diagnostic.CurrentBrowserProxyAddress != "127.0.0.1:10808" || diagnostic.ExpectedProxyAddress != "127.0.0.1:2025" {
		t.Fatalf("expected current and expected proxy addresses, got %+v", diagnostic)
	}
}

func TestBuildNoReadyWXCommentExportDetailIncludesWindowsProxyMismatchHint(t *testing.T) {
	original := readWindowsInternetProxySettings
	readWindowsInternetProxySettings = func() (internetProxySettings, bool) {
		return internetProxySettings{
			Enabled: true,
			Server:  "127.0.0.1:10808",
		}, true
	}
	defer func() {
		readWindowsInternetProxySettings = original
	}()

	zero := 0
	detail, diagnostic := buildNoReadyWXCommentExportDetail(
		"http://127.0.0.1:2026",
		"http://127.0.0.1:2026/ws/health",
		&zero,
		"ok",
	)
	if detail == "" {
		t.Fatalf("expected proxy mismatch detail")
	}
	if !containsAll(detail, "10808", "2025", "comment export has no ready page client") {
		t.Fatalf("expected proxy mismatch detail to mention current and expected proxy, got %q", detail)
	}
	if diagnostic == nil || diagnostic.Code != "wx_comment_export_proxy_mismatch" {
		t.Fatalf("expected comment export proxy mismatch diagnostic, got %+v", diagnostic)
	}
	if diagnostic.CurrentBrowserProxyAddress != "127.0.0.1:10808" || diagnostic.ExpectedProxyAddress != "127.0.0.1:2025" {
		t.Fatalf("expected current and expected proxy addresses, got %+v", diagnostic)
	}
}

func TestBuildNoReadyWXCommentExportStatusDetailIncludesWindowsProxyMismatchHint(t *testing.T) {
	original := readWindowsInternetProxySettings
	readWindowsInternetProxySettings = func() (internetProxySettings, bool) {
		return internetProxySettings{
			Enabled: true,
			Server:  "127.0.0.1:10808",
		}, true
	}
	defer func() {
		readWindowsInternetProxySettings = original
	}()

	detail, diagnostic := buildNoReadyWXCommentExportStatusDetail(
		"http://127.0.0.1:2026",
		"http://127.0.0.1:2026/api/channels/status",
		wxFeedListStatusPayload{},
		0,
		0,
	)
	if detail == "" {
		t.Fatalf("expected proxy mismatch detail")
	}
	if !containsAll(detail, "10808", "2025", "comment export has no ready page client", "clients=0", "ready_clients=0") {
		t.Fatalf("expected proxy mismatch detail to mention current and expected proxy, got %q", detail)
	}
	if diagnostic == nil || diagnostic.Code != "wx_comment_export_proxy_mismatch" {
		t.Fatalf("expected comment export proxy mismatch diagnostic, got %+v", diagnostic)
	}
	if diagnostic.CurrentBrowserProxyAddress != "127.0.0.1:10808" || diagnostic.ExpectedProxyAddress != "127.0.0.1:2025" {
		t.Fatalf("expected current and expected proxy addresses, got %+v", diagnostic)
	}
}

func TestBuildNoReadyWXFeedListStatusDetailIncludesZeroClientEvidenceForProxyMismatch(t *testing.T) {
	original := readWindowsInternetProxySettings
	readWindowsInternetProxySettings = func() (internetProxySettings, bool) {
		return internetProxySettings{
			Enabled: true,
			Server:  "127.0.0.1:10808",
		}, true
	}
	defer func() {
		readWindowsInternetProxySettings = original
	}()

	detail, diagnostic := buildNoReadyWXFeedListStatusDetail(
		"http://127.0.0.1:2026",
		"http://127.0.0.1:2026/api/channels/status",
		wxFeedListStatusPayload{},
		0,
		0,
		0,
	)
	if detail == "" {
		t.Fatalf("expected proxy mismatch detail")
	}
	if !containsAll(detail, "10808", "2025", "feed list has no ready page client", "clients=0", "ready_clients=0") {
		t.Fatalf("expected proxy mismatch detail to mention proxy mismatch and zero client evidence, got %q", detail)
	}
	if diagnostic == nil || diagnostic.Code != "wx_feed_list_proxy_mismatch" {
		t.Fatalf("expected feed list proxy mismatch diagnostic, got %+v", diagnostic)
	}
	if diagnostic.CurrentBrowserProxyAddress != "127.0.0.1:10808" || diagnostic.ExpectedProxyAddress != "127.0.0.1:2025" {
		t.Fatalf("expected current and expected proxy addresses, got %+v", diagnostic)
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
