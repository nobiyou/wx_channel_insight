package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/hubbridge"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestBuildHubRequestContextPrefersHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?hub_user_id=hub:user:bob&device_count=4", nil)
	req.Header.Set(headerHubUserID, "hub:user:alice")
	req.Header.Set(headerHubDeviceCount, "9")

	ctx, err := buildHubRequestContext(req, runtime.BridgeConfig{}, nil)
	if err != nil {
		t.Fatalf("expected context build success, got %v", err)
	}

	if ctx.HubUserID != "hub:user:alice" {
		t.Fatalf("expected header hub user id, got %q", ctx.HubUserID)
	}
	if ctx.DeviceCount != 9 {
		t.Fatalf("expected header device count 9, got %d", ctx.DeviceCount)
	}
	if ctx.ContextSource != "header" {
		t.Fatalf("expected header context source, got %q", ctx.ContextSource)
	}
}

func TestBuildHubRequestContextFallsBackToQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?hub_user_id=hub:user:bob&device_count=4", nil)

	ctx, err := buildHubRequestContext(req, runtime.BridgeConfig{}, nil)
	if err != nil {
		t.Fatalf("expected context build success, got %v", err)
	}

	if ctx.HubUserID != "hub:user:bob" {
		t.Fatalf("expected query hub user id, got %q", ctx.HubUserID)
	}
	if ctx.DeviceCount != 4 {
		t.Fatalf("expected query device count 4, got %d", ctx.DeviceCount)
	}
	if ctx.ContextSource != "query" {
		t.Fatalf("expected query context source, got %q", ctx.ContextSource)
	}
}

func TestBuildHubRequestContextDefaultsWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)

	ctx, err := buildHubRequestContext(req, runtime.BridgeConfig{}, nil)
	if err != nil {
		t.Fatalf("expected context build success, got %v", err)
	}

	if ctx.HubUserID != "" {
		t.Fatalf("expected empty hub user id, got %q", ctx.HubUserID)
	}
	if ctx.DeviceCount != 0 {
		t.Fatalf("expected device count 0, got %d", ctx.DeviceCount)
	}
	if ctx.ContextSource != "default" {
		t.Fatalf("expected default context source, got %q", ctx.ContextSource)
	}
}

func TestBuildHubRequestContextAcceptsTrustedHeaderWhenTokenMatches(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.Header.Set(headerHubUserID, "hub:user:trusted")
	req.Header.Set(headerHubDeviceCount, "12")
	req.Header.Set(headerBridgeToken, "bridge-secret")

	ctx, err := buildHubRequestContext(req, runtime.BridgeConfig{
		TrustedToken:       "bridge-secret",
		DemoContextEnabled: false,
	}, nil)
	if err != nil {
		t.Fatalf("expected trusted header success, got %v", err)
	}
	if ctx.HubUserID != "hub:user:trusted" || ctx.DeviceCount != 12 {
		t.Fatalf("expected trusted header context, got %+v", ctx)
	}
	if ctx.ContextSource != "trusted_header" {
		t.Fatalf("expected trusted_header context source, got %q", ctx.ContextSource)
	}
}

func TestBuildHubRequestContextRejectsQueryWhenTrustedModeDisablesDemoContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?hub_user_id=hub:user:query&device_count=7", nil)

	_, err := buildHubRequestContext(req, runtime.BridgeConfig{
		TrustedToken:       "bridge-secret",
		DemoContextEnabled: false,
	}, nil)
	if err == nil {
		t.Fatal("expected trusted-only query rejection, got nil")
	}
	if err != errDemoContextDisabled {
		t.Fatalf("expected demo-context-disabled error, got %v", err)
	}
}

func TestBuildHubRequestContextRejectsHeaderWithoutTrustedTokenWhenDemoContextDisabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.Header.Set(headerHubUserID, "hub:user:header")
	req.Header.Set(headerHubDeviceCount, "3")

	_, err := buildHubRequestContext(req, runtime.BridgeConfig{
		TrustedToken:       "bridge-secret",
		DemoContextEnabled: false,
	}, nil)
	if err == nil {
		t.Fatal("expected missing trusted token rejection, got nil")
	}
	if err != errTrustedBridgeRequired {
		t.Fatalf("expected trusted-bridge-required error, got %v", err)
	}
}

func TestBuildHubRequestContextRejectsLiveRequestsWhenTeamPreviewRequiresTrustedBridgeButTokenIsMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?hub_user_id=hub:user:query&device_count=7", nil)

	_, err := buildHubRequestContext(req, runtime.BridgeConfig{
		RequireTrustedForLive: true,
		DemoContextEnabled:    false,
	}, nil)
	if err == nil {
		t.Fatal("expected missing trusted bridge configuration rejection, got nil")
	}
	if err != errTrustedBridgeNotConfigured {
		t.Fatalf("expected trusted-bridge-not-configured error, got %v", err)
	}
}

func TestBuildHubRequestContextAllowsTeamPreviewDemoOverrideWhenExplicitlyEnabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?hub_user_id=hub:user:query&device_count=7", nil)

	ctx, err := buildHubRequestContext(req, runtime.BridgeConfig{
		RequireTrustedForLive: true,
		DemoContextEnabled:    true,
	}, nil)
	if err != nil {
		t.Fatalf("expected explicit demo override to work, got %v", err)
	}
	if ctx.ContextSource != "query" || ctx.HubUserID != "hub:user:query" {
		t.Fatalf("expected query demo override context, got %+v", ctx)
	}
}

type stubHubSessionResolver struct {
	identity hubbridge.Identity
	err      error
	calls    int
}

func (s *stubHubSessionResolver) Resolve(ctx context.Context, bearerToken string) (hubbridge.Identity, error) {
	s.calls++
	if s.err != nil {
		return hubbridge.Identity{}, s.err
	}
	return s.identity, nil
}

func TestBuildHubRequestContextPrefersHubSessionBearer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.Header.Set("Authorization", "Bearer hub-jwt")
	req.Header.Set(headerHubUserID, "hub:user:should-not-win")
	req.Header.Set(headerHubDeviceCount, "9")

	resolver := &stubHubSessionResolver{identity: hubbridge.Identity{
		HubUserID:   "hub:user:7",
		DeviceCount: 2,
		Source:      "hub_session",
	}}
	ctx, err := buildHubRequestContext(req, runtime.BridgeConfig{
		HubBaseURL:            "http://127.0.0.1:8080",
		RequireTrustedForLive: true,
		DemoContextEnabled:    false,
	}, resolver)
	if err != nil {
		t.Fatalf("expected hub session success, got %v", err)
	}
	if ctx.ContextSource != "hub_session" || ctx.HubUserID != "hub:user:7" {
		t.Fatalf("unexpected context: %+v", ctx)
	}
	// header device count can override best-effort hub count
	if ctx.DeviceCount != 9 {
		t.Fatalf("expected header device count override 9, got %d", ctx.DeviceCount)
	}
	if resolver.calls != 1 {
		t.Fatalf("expected one resolve call, got %d", resolver.calls)
	}
}

func TestBuildHubRequestContextRejectsInvalidHubSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.Header.Set("Authorization", "Bearer bad")
	resolver := &stubHubSessionResolver{err: errHubSessionInvalid}
	_, err := buildHubRequestContext(req, runtime.BridgeConfig{
		HubBaseURL:            "http://127.0.0.1:8080",
		RequireTrustedForLive: true,
		DemoContextEnabled:    false,
	}, resolver)
	if err != errHubSessionInvalid {
		t.Fatalf("expected invalid hub session, got %v", err)
	}
}

func TestBuildHubRequestContextRequiresHubSessionWhenOnlyHubBaseURLConfigured(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	_, err := buildHubRequestContext(req, runtime.BridgeConfig{
		HubBaseURL:            "http://127.0.0.1:8080",
		RequireTrustedForLive: true,
		DemoContextEnabled:    false,
	}, &stubHubSessionResolver{})
	if err != errHubSessionRequired {
		t.Fatalf("expected hub session required, got %v", err)
	}
}

func TestBuildHubRequestContextAllowsLocalOperatorWithoutToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.Header.Set(headerHubUserID, "insight:operator:local")
	req.Header.Set(headerHubDeviceCount, "1")
	ctx, err := buildHubRequestContext(req, runtime.BridgeConfig{
		LocalOperator:         true,
		RequireTrustedForLive: true,
		DemoContextEnabled:    false,
	}, nil)
	if err != nil {
		t.Fatalf("expected local operator success, got %v", err)
	}
	if ctx.ContextSource != "local_operator" || ctx.HubUserID != "insight:operator:local" {
		t.Fatalf("unexpected context: %+v", ctx)
	}
}

func TestBuildHubRequestContextRejectsMissingLocalOperatorTokenWhenConfigured(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	r.Header.Set(headerHubUserID, "spoofed-owner")
	_, err := buildHubRequestContext(r, runtime.BridgeConfig{
		TrustedToken:          "bridge-secret",
		LocalOperator:         true,
		RequireTrustedForLive: true,
		DemoContextEnabled:    false,
	}, nil)
	if err == nil || err.Error() != errTrustedBridgeRequired.Error() {
		t.Fatalf("expected missing local-operator token to be rejected, got %v", err)
	}
}
