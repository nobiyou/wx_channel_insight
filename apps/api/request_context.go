package main

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/hubbridge"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

const (
	headerHubUserID      = "X-Hub-User-ID"
	headerHubDeviceCount = "X-Hub-Device-Count"
	headerBridgeToken    = "X-Insight-Bridge-Token"
	headerAuthorization  = "Authorization"
)

type requestContextKey string

const hubContextKey requestContextKey = "hub-context"

type hubRequestContext struct {
	HubUserID     string
	DeviceCount   int
	ContextSource string
}

type hubSessionResolver interface {
	Resolve(ctx context.Context, bearerToken string) (hubbridge.Identity, error)
}

func withHubContext(next http.Handler, bridgeConfig runtime.BridgeConfig) http.Handler {
	return withHubContextAndSession(next, bridgeConfig, nil)
}

func withHubContextAndSession(next http.Handler, bridgeConfig runtime.BridgeConfig, sessionResolver hubSessionResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqContext, err := buildHubRequestContext(r, bridgeConfig, sessionResolver)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": err.Error(),
			})
			return
		}
		ctx := context.WithValue(r.Context(), hubContextKey, reqContext)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func buildHubRequestContext(r *http.Request, bridgeConfig runtime.BridgeConfig, sessionResolver hubSessionResolver) (hubRequestContext, error) {
	config := normalizeBridgeConfig(bridgeConfig)
	if shouldBypassTrustedBridge(r.URL.Path) {
		return buildDemoHubRequestContext(r), nil
	}
	strictTrustedLive := requiresStrictTrustedLive(config, r.URL.Path)

	// Prefer real Hub JWT session when configured and Authorization is present.
	if config.HubSessionEnabled() && sessionResolver != nil {
		if token, ok := bearerTokenFromRequest(r); ok {
			identity, err := sessionResolver.Resolve(r.Context(), token)
			if err != nil {
				return hubRequestContext{}, errHubSessionInvalid
			}
			deviceCount := identity.DeviceCount
			if headerDeviceCount := parseIntOrZero(r.Header.Get(headerHubDeviceCount)); headerDeviceCount > 0 {
				deviceCount = headerDeviceCount
			}
			return hubRequestContext{
				HubUserID:     identity.HubUserID,
				DeviceCount:   deviceCount,
				ContextSource: "hub_session",
			}, nil
		}
	}

	if strictTrustedLive && !config.TrustedModeEnabled() {
		return hubRequestContext{}, errTrustedBridgeNotConfigured
	}

	// Hub-less team edition: local operator may use operator id headers without shared secret / Hub JWT.
	if config.LocalOperatorEnabled() && requiresTrustedContext(r.URL.Path) {
		headerUserID := strings.TrimSpace(r.Header.Get(headerHubUserID))
		headerDeviceCount := parseIntOrZero(r.Header.Get(headerHubDeviceCount))
		if headerUserID != "" {
			// Once a shared secret exists, every local-operator request must prove it.
			// Accepting a missing token here would let callers spoof any workspace ID.
			if strings.TrimSpace(config.TrustedToken) != "" {
				providedToken := strings.TrimSpace(r.Header.Get(headerBridgeToken))
				if !secureCompare(providedToken, config.TrustedToken) {
					return hubRequestContext{}, errTrustedBridgeRequired
				}
				return hubRequestContext{HubUserID: headerUserID, DeviceCount: headerDeviceCount, ContextSource: "trusted_header"}, nil
			}
			return hubRequestContext{
				HubUserID:     headerUserID,
				DeviceCount:   headerDeviceCount,
				ContextSource: "local_operator",
			}, nil
		}
		// no header: allow empty id; handlers apply DefaultHubUserID / insight:operator:local
		if strictTrustedLive {
			return hubRequestContext{ContextSource: "local_operator"}, nil
		}
	}

	headerUserID := strings.TrimSpace(r.Header.Get(headerHubUserID))
	headerDeviceCount := parseIntOrZero(r.Header.Get(headerHubDeviceCount))
	if headerUserID != "" {
		if strictTrustedLive {
			// Shared-secret trusted header remains valid for machine-to-machine / proxy paths.
			if strings.TrimSpace(config.TrustedToken) != "" {
				providedToken := strings.TrimSpace(r.Header.Get(headerBridgeToken))
				if secureCompare(providedToken, config.TrustedToken) {
					return hubRequestContext{
						HubUserID:     headerUserID,
						DeviceCount:   headerDeviceCount,
						ContextSource: "trusted_header",
					}, nil
				}
			}
			// Hub session mode without Bearer still cannot invent identity from untrusted headers.
			if config.HubSessionEnabled() {
				return hubRequestContext{}, errHubSessionRequired
			}
			return hubRequestContext{}, errTrustedBridgeRequired
		}
		if config.TrustedModeEnabled() {
			if strings.TrimSpace(config.TrustedToken) != "" {
				providedToken := strings.TrimSpace(r.Header.Get(headerBridgeToken))
				if secureCompare(providedToken, config.TrustedToken) {
					return hubRequestContext{
						HubUserID:     headerUserID,
						DeviceCount:   headerDeviceCount,
						ContextSource: "trusted_header",
					}, nil
				}
			}
			if !config.DemoContextEnabled {
				if config.HubSessionEnabled() {
					return hubRequestContext{}, errHubSessionRequired
				}
				return hubRequestContext{}, errTrustedBridgeRequired
			}
		}
		return hubRequestContext{
			HubUserID:     headerUserID,
			DeviceCount:   headerDeviceCount,
			ContextSource: "header",
		}, nil
	}

	queryContext := buildQueryHubRequestContext(r)
	if queryContext.ContextSource == "query" {
		if strictTrustedLive {
			return hubRequestContext{}, errDemoContextDisabled
		}
		if !config.DemoContextEnabled && config.TrustedModeEnabled() {
			return hubRequestContext{}, errDemoContextDisabled
		}
		return queryContext, nil
	}

	if strictTrustedLive {
		if config.HubSessionEnabled() {
			return hubRequestContext{}, errHubSessionRequired
		}
		return hubRequestContext{}, errTrustedBridgeRequired
	}
	if config.TrustedModeEnabled() && !config.DemoContextEnabled && requiresTrustedContext(r.URL.Path) {
		if config.HubSessionEnabled() {
			return hubRequestContext{}, errHubSessionRequired
		}
		return hubRequestContext{}, errTrustedBridgeRequired
	}

	return queryContext, nil
}

func bearerTokenFromRequest(r *http.Request) (string, bool) {
	authHeader := strings.TrimSpace(r.Header.Get(headerAuthorization))
	if authHeader == "" {
		return "", false
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}
	return token, true
}

func buildDemoHubRequestContext(r *http.Request) hubRequestContext {
	headerUserID := strings.TrimSpace(r.Header.Get(headerHubUserID))
	headerDeviceCount := parseIntOrZero(r.Header.Get(headerHubDeviceCount))
	if headerUserID != "" {
		return hubRequestContext{
			HubUserID:     headerUserID,
			DeviceCount:   headerDeviceCount,
			ContextSource: "header",
		}
	}

	return buildQueryHubRequestContext(r)
}

func buildQueryHubRequestContext(r *http.Request) hubRequestContext {
	query := r.URL.Query()
	queryUserID := strings.TrimSpace(query.Get("hub_user_id"))
	queryDeviceCount := parseIntOrZero(query.Get("device_count"))
	if queryUserID != "" {
		return hubRequestContext{
			HubUserID:     queryUserID,
			DeviceCount:   queryDeviceCount,
			ContextSource: "query",
		}
	}

	return hubRequestContext{
		ContextSource: "default",
	}
}

func hubContextFromRequest(r *http.Request) hubRequestContext {
	if value, ok := r.Context().Value(hubContextKey).(hubRequestContext); ok {
		return value
	}
	reqContext, _ := buildHubRequestContext(r, runtime.BridgeConfig{}, nil)
	return reqContext
}

// In local operator mode an absent identity means the configured default
// workspace. Resolve it before using identity as a lock or heartbeat key.
func (s *server) resolveMonitoringContext(r *http.Request) (hubRequestContext, error) {
	hub := hubContextFromRequest(r)
	if hub.HubUserID != "" || s.contextResolver == nil {
		return hub, nil
	}
	resolved, err := s.contextResolver.Resolve(r.Context(), workspace.ContextQuery{
		DeviceCount: hub.DeviceCount, ContextSource: hub.ContextSource,
	})
	if err != nil {
		return hubRequestContext{}, err
	}
	hub.HubUserID = resolved.Workspace.OwnerUserID
	hub.DeviceCount = resolved.Workspace.DeviceCount
	return hub, nil
}

func parseIntOrZero(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func normalizeBridgeConfig(config runtime.BridgeConfig) runtime.BridgeConfig {
	if !config.TrustedModeEnabled() && !config.DemoContextEnabled && !config.RequireTrustedForLive {
		config.DemoContextEnabled = true
	}
	return config
}

func requiresStrictTrustedLive(config runtime.BridgeConfig, path string) bool {
	return config.RequireTrustedForLive && !config.DemoContextEnabled && requiresTrustedContext(path)
}

func secureCompare(left string, right string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func shouldBypassTrustedBridge(path string) bool {
	return strings.HasPrefix(path, "/api/demo/")
}

func requiresTrustedContext(path string) bool {
	return strings.HasPrefix(path, "/api/v1/")
}

var (
	errTrustedBridgeRequired      = unauthorizedContextError("trusted Hub bridge token is required for /api/v1/*")
	errTrustedBridgeNotConfigured = unauthorizedContextError("trusted Hub bridge is required for this runtime profile, but INSIGHT_TRUSTED_BRIDGE_TOKEN / INSIGHT_HUB_BASE_URL is not configured")
	errDemoContextDisabled        = unauthorizedContextError("demo query context is disabled for /api/v1/*")
	errHubSessionRequired         = unauthorizedContextError("Hub session is required for /api/v1/* (send Authorization: Bearer <hub-jwt>)")
	errHubSessionInvalid          = unauthorizedContextError("Hub session is invalid or expired")
)

type unauthorizedContextError string

func (e unauthorizedContextError) Error() string {
	return string(e)
}
