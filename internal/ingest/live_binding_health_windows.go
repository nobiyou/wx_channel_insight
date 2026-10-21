//go:build windows

package ingest

import (
	"net"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type internetProxySettings struct {
	Enabled bool
	Server  string
}

var readWindowsInternetProxySettings = defaultReadWindowsInternetProxySettings

func detectLocalWXFeedListProxyMismatch(baseURL string) *wxFeedListProxyMismatch {
	expected := expectedWXChannelProxyAddress(baseURL)
	if expected == "" || !isLoopbackWXFeedListBaseURL(baseURL) {
		return nil
	}

	settings, ok := readWindowsInternetProxySettings()
	if !ok || !settings.Enabled {
		return nil
	}

	configured := pickConfiguredProxyEndpoint(settings.Server)
	if configured == "" || sameProxyEndpoint(configured, expected) {
		return nil
	}

	return &wxFeedListProxyMismatch{
		CurrentProxyAddress:  configured,
		ExpectedProxyAddress: expected,
	}
}

func defaultReadWindowsInternetProxySettings() (internetProxySettings, bool) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return internetProxySettings{}, false
	}
	defer key.Close()

	proxyEnabledValue, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil {
		return internetProxySettings{}, false
	}
	proxyServerValue, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		proxyServerValue = ""
	}

	return internetProxySettings{
		Enabled: proxyEnabledValue != 0,
		Server:  strings.TrimSpace(proxyServerValue),
	}, true
}

func pickConfiguredProxyEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var firstFallback string
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "=") {
			pieces := strings.SplitN(part, "=", 2)
			scheme := strings.ToLower(strings.TrimSpace(pieces[0]))
			endpoint := strings.TrimSpace(pieces[1])
			if firstFallback == "" {
				firstFallback = endpoint
			}
			if scheme == "https" || scheme == "http" {
				return endpoint
			}
			continue
		}
		if firstFallback == "" {
			firstFallback = part
		}
	}
	return firstFallback
}

func sameProxyEndpoint(left, right string) bool {
	leftHost, leftPort := splitProxyEndpoint(left)
	rightHost, rightPort := splitProxyEndpoint(right)
	if leftPort == "" || rightPort == "" {
		return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
	}
	return strings.EqualFold(leftPort, rightPort) && sameLoopbackHost(leftHost, rightHost)
}

func splitProxyEndpoint(value string) (string, string) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(value))
	if err == nil {
		return normalizeLoopbackHost(host), port
	}
	if strings.Count(value, ":") == 1 {
		parts := strings.SplitN(strings.TrimSpace(value), ":", 2)
		return normalizeLoopbackHost(parts[0]), parts[1]
	}
	return normalizeLoopbackHost(strings.TrimSpace(value)), ""
}

func sameLoopbackHost(left, right string) bool {
	return normalizeLoopbackHost(left) == normalizeLoopbackHost(right)
}

func normalizeLoopbackHost(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "localhost", "::1":
		return "127.0.0.1"
	default:
		return value
	}
}
