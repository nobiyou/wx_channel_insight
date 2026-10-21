//go:build windows

package windows_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type emitOnlyScriptResult struct {
	RepoRoot       string `json:"repoRoot"`
	ResultJSONPath string `json:"resultJsonPath"`
}

func TestStartInsightTrustedWebDevEmitOnlyIncludesDefaultResultPath(t *testing.T) {
	payload := runEmitOnlyScript(t, "start-insight-trusted-web-dev.ps1")
	expectedRoot := expectedRepoRoot(t)
	expectedPath := filepath.Join(expectedRoot, ".tmp-codex", "trusted-web", "trusted-web-latest.json")
	assertPathEqual(t, payload.RepoRoot, expectedRoot, "repoRoot")
	assertPathEqual(t, payload.ResultJSONPath, expectedPath, "resultJsonPath")
}

func TestRestoreWXChannelPageClientEmitOnlyShowsReadOnlyDataPlaneDiagnostic(t *testing.T) {
	host, ok := resolvePowerShellHost()
	if !ok {
		t.Skip("PowerShell host is not available on PATH")
	}

	scriptPath := filepath.Join(scriptDir(t), "restore-wx-channel-page-client.ps1")
	cmd := exec.Command(host, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-EmitOnly")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("emit-only script restore-wx-channel-page-client.ps1 failed: %v\n%s", err, string(output))
	}

	var payload struct {
		RepoRoot                    string            `json:"repoRoot"`
		Mode                        string            `json:"mode"`
		BaseURL                     string            `json:"baseURL"`
		WSHealthURL                 string            `json:"wsHealthURL"`
		StatusURL                   string            `json:"statusURL"`
		RequiredCapabilityURLs      map[string]string `json:"requiredCapabilityURLs"`
		RequiredCapabilityAdminHint string            `json:"requiredCapabilityAdminHint"`
		ExpectedProxyAddress        string            `json:"expectedProxyAddress"`
		LaunchesBrowser             bool              `json:"launchesBrowser"`
		MutatesProxySettings        bool              `json:"mutatesProxySettings"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("emit-only script restore-wx-channel-page-client.ps1 returned invalid JSON: %v\n%s", err, string(output))
	}

	expectedRoot := expectedRepoRoot(t)
	assertPathEqual(t, payload.RepoRoot, expectedRoot, "repoRoot")
	if payload.Mode != "read-only-data-plane-diagnostic" {
		t.Fatalf("expected read-only diagnostic mode, got %q", payload.Mode)
	}
	if payload.BaseURL != "http://127.0.0.1:2026" {
		t.Fatalf("expected default baseURL, got %q", payload.BaseURL)
	}
	if payload.WSHealthURL != "http://127.0.0.1:2026/ws/health" {
		t.Fatalf("expected default wsHealthURL, got %q", payload.WSHealthURL)
	}
	if payload.StatusURL != "http://127.0.0.1:2026/api/channels/status" {
		t.Fatalf("expected default statusURL, got %q", payload.StatusURL)
	}
	for name, expectedURL := range map[string]string{
		"feedSearch":    "http://127.0.0.1:2026/api/channels/feed/search?keyword=__insight_capability_probe__",
		"feedList":      "http://127.0.0.1:2026/api/channels/contact/feed/list?username=__insight_capability_probe__",
		"commentExport": "http://127.0.0.1:2026/api/channels/feed/comment/export",
	} {
		if payload.RequiredCapabilityURLs[name] != expectedURL {
			t.Fatalf("expected capability %s URL %q, got %+v", name, expectedURL, payload.RequiredCapabilityURLs)
		}
	}
	if !strings.Contains(payload.RequiredCapabilityAdminHint, "switch-insight-data-plane-admin.cmd") {
		t.Fatalf("expected admin switch hint, got %q", payload.RequiredCapabilityAdminHint)
	}
	if payload.ExpectedProxyAddress != "127.0.0.1:2025" {
		t.Fatalf("expected default proxy address, got %q", payload.ExpectedProxyAddress)
	}
	if payload.LaunchesBrowser {
		t.Fatalf("restore script must not launch a browser for wx_channel data capture")
	}
	if payload.MutatesProxySettings {
		t.Fatalf("restore script must not mutate proxy settings")
	}
}

func TestRestoreWXChannelPageClientEmitOnlyPassesUnderWindowsPowerShell(t *testing.T) {
	host, ok := resolveWindowsPowerShellHost()
	if !ok {
		t.Skip("Windows PowerShell host is not available on PATH")
	}

	scriptPath := filepath.Join(scriptDir(t), "restore-wx-channel-page-client.ps1")
	cmd := exec.Command(host, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-EmitOnly")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("emit-only script restore-wx-channel-page-client.ps1 failed under Windows PowerShell: %v\n%s", err, string(output))
	}

	var payload map[string]any
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("emit-only script restore-wx-channel-page-client.ps1 returned invalid JSON under Windows PowerShell: %v\n%s", err, string(output))
	}
}

func TestRestoreWXChannelPageClientScriptDoesNotLaunchBrowser(t *testing.T) {
	scriptPath := filepath.Join(scriptDir(t), "restore-wx-channel-page-client.ps1")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read restore-wx-channel-page-client.ps1 failed: %v", err)
	}
	text := string(content)
	for _, forbidden := range []string{"Start-Process", "--proxy-server", "channels.weixin.qq.com", "OpenURL"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("restore script should be a read-only wx_channel data-plane diagnostic and must not contain %q", forbidden)
		}
	}
}

func TestRestoreWXChannelPageClientScriptDetectsOldDataPlaneRoutes(t *testing.T) {
	scriptPath := filepath.Join(scriptDir(t), "restore-wx-channel-page-client.ps1")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read restore-wx-channel-page-client.ps1 failed: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		"Get-WXChannelCapabilityStatus",
		"/api/channels/feed/search?keyword=__insight_capability_probe__",
		"/api/channels/contact/feed/list?username=__insight_capability_probe__",
		"/api/channels/feed/comment/export",
		"statusCode -eq 404",
		"wx_channel_data_plane_incompatible",
		"switch-insight-data-plane-admin.cmd",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("restore script should detect old/incompatible wx_channel data plane with %q", required)
		}
	}
}

func TestStartInsightDockerStackEmitOnlyShowsHublessWXChannelSource(t *testing.T) {
	host, ok := resolvePowerShellHost()
	if !ok {
		t.Skip("PowerShell host is not available on PATH")
	}

	scriptPath := filepath.Join(scriptDir(t), "start-insight-docker-stack.ps1")
	cmd := exec.Command(host, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-WxChannelBaseUrl", "http://host.docker.internal:2026", "-HubUserId", "insight:operator:local", "-DeviceCount", "1", "-EmitOnly")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("emit-only script start-insight-docker-stack.ps1 failed: %v\n%s", err, string(output))
	}

	var payload struct {
		RepoRoot         string            `json:"repoRoot"`
		ComposeFile      string            `json:"composeFile"`
		WXChannelBaseURL string            `json:"wxChannelBaseUrl"`
		HubUserID        string            `json:"hubUserId"`
		DeviceCount      int               `json:"deviceCount"`
		ComposeCommand   string            `json:"composeCommand"`
		ComposeArgs      []string          `json:"composeArguments"`
		Env              map[string]string `json:"env"`
		Checks           struct {
			APIHealthURL string `json:"apiHealthURL"`
			EdgeWSURL    string `json:"edgeWSURL"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("emit-only script start-insight-docker-stack.ps1 returned invalid JSON: %v\n%s", err, string(output))
	}

	expectedRoot := expectedRepoRoot(t)
	assertPathEqual(t, payload.RepoRoot, expectedRoot, "repoRoot")
	assertPathEqual(t, payload.ComposeFile, filepath.Join(expectedRoot, "deploy", "stack", "docker-compose.yml"), "composeFile")
	if payload.WXChannelBaseURL != "http://host.docker.internal:2026" {
		t.Fatalf("expected Docker stack to target host wx_channel :2026, got %q", payload.WXChannelBaseURL)
	}
	if payload.Env["INSIGHT_WX_CHANNEL_BASE_URL"] != payload.WXChannelBaseURL {
		t.Fatalf("expected env wx_channel base to match payload, got env=%q payload=%q", payload.Env["INSIGHT_WX_CHANNEL_BASE_URL"], payload.WXChannelBaseURL)
	}
	if payload.Env["INSIGHT_DEFAULT_HUB_USER_ID"] != "insight:operator:local" || payload.HubUserID != "insight:operator:local" {
		t.Fatalf("expected local operator identity, got payload=%q env=%q", payload.HubUserID, payload.Env["INSIGHT_DEFAULT_HUB_USER_ID"])
	}
	if _, found := payload.Env["INSIGHT_HUB_BASE_URL"]; found {
		t.Fatalf("Docker stack should not set INSIGHT_HUB_BASE_URL in hub-less default: %+v", payload.Env)
	}
	if payload.ComposeCommand != "docker" || !containsAllArgs(payload.ComposeArgs, "compose", "up", "-d") {
		t.Fatalf("expected docker compose up -d plan, command=%q args=%v", payload.ComposeCommand, payload.ComposeArgs)
	}
	if payload.Checks.APIHealthURL != "http://127.0.0.1:18081/api/healthz" || payload.Checks.EdgeWSURL != "ws://127.0.0.1:18081/ws/client" {
		t.Fatalf("expected default stack checks, got %+v", payload.Checks)
	}
}

func TestDiagnoseInsightRuntimeEmitOnlyShowsReadOnlyPlan(t *testing.T) {
	host, ok := resolvePowerShellHost()
	if !ok {
		t.Skip("PowerShell host is not available on PATH")
	}

	scriptPath := filepath.Join(scriptDir(t), "diagnose-insight-runtime.ps1")
	cmd := exec.Command(host, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-EmitOnly")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("emit-only script diagnose-insight-runtime.ps1 failed: %v\n%s", err, string(output))
	}

	var payload struct {
		RepoRoot         string `json:"repoRoot"`
		APIBaseURL       string `json:"apiBaseURL"`
		WebBaseURL       string `json:"webBaseURL"`
		WXChannelBaseURL string `json:"wxChannelBaseURL"`
		Mode             string `json:"mode"`
		PlannedURLs      struct {
			WXChannelStatus        string `json:"wxChannelStatus"`
			InsightAPIHealth       string `json:"insightAPIHealth"`
			InsightRuntimeStatus   string `json:"insightRuntimeStatus"`
			InsightWebRuntimeProxy string `json:"insightWebRuntimeProxy"`
		} `json:"plannedURLs"`
		PlannedDependencyChecks struct {
			Docker        string   `json:"docker"`
			PostgresTools []string `json:"postgresTools"`
			RedisTools    []string `json:"redisTools"`
		} `json:"plannedDependencyChecks"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("emit-only script diagnose-insight-runtime.ps1 returned invalid JSON: %v\n%s", err, string(output))
	}

	assertPathEqual(t, payload.RepoRoot, expectedRepoRoot(t), "repoRoot")
	if payload.Mode != "read-only-diagnostic" {
		t.Fatalf("expected read-only diagnostic mode, got %q", payload.Mode)
	}
	if payload.APIBaseURL != "http://127.0.0.1:18081" || payload.WebBaseURL != "http://127.0.0.1:5176" || payload.WXChannelBaseURL != "http://127.0.0.1:2026" {
		t.Fatalf("unexpected default base URLs: %+v", payload)
	}
	if payload.PlannedURLs.WXChannelStatus != "http://127.0.0.1:2026/api/channels/status" ||
		payload.PlannedURLs.InsightAPIHealth != "http://127.0.0.1:18081/api/healthz" ||
		payload.PlannedURLs.InsightRuntimeStatus != "http://127.0.0.1:18081/api/v1/runtime/status" ||
		payload.PlannedURLs.InsightWebRuntimeProxy != "http://127.0.0.1:5176/api/v1/runtime/status" {
		t.Fatalf("unexpected diagnostic planned URLs: %+v", payload.PlannedURLs)
	}
	if payload.PlannedDependencyChecks.Docker != "docker info" {
		t.Fatalf("expected diagnostic plan to include docker info, got %+v", payload.PlannedDependencyChecks)
	}
	if !containsAllArgs(payload.PlannedDependencyChecks.PostgresTools, "pg_ctl", "initdb", "psql") {
		t.Fatalf("expected postgres tool checks in diagnostic plan, got %+v", payload.PlannedDependencyChecks.PostgresTools)
	}
	if !containsAllArgs(payload.PlannedDependencyChecks.RedisTools, "redis-server", "redis-cli") {
		t.Fatalf("expected redis tool checks in diagnostic plan, got %+v", payload.PlannedDependencyChecks.RedisTools)
	}
}

func TestDiagnoseInsightRuntimeScriptDoesNotStartProcesses(t *testing.T) {
	scriptPath := filepath.Join(scriptDir(t), "diagnose-insight-runtime.ps1")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read diagnose-insight-runtime.ps1 failed: %v", err)
	}
	text := string(content)
	for _, forbidden := range []string{"Start-Process", "docker compose", "docker @", "go run", "npm run"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("diagnostic script should be read-only and must not contain %q", forbidden)
		}
	}
}

func runEmitOnlyScript(t *testing.T, scriptName string) emitOnlyScriptResult {
	t.Helper()
	host, ok := resolvePowerShellHost()
	if !ok {
		t.Skip("PowerShell host is not available on PATH")
	}

	scriptPath := filepath.Join(scriptDir(t), scriptName)
	cmd := exec.Command(host, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-EmitOnly")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("emit-only script %s failed: %v\n%s", scriptName, err, string(output))
	}

	var payload emitOnlyScriptResult
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("emit-only script %s returned invalid JSON: %v\n%s", scriptName, err, string(output))
	}
	if payload.RepoRoot == "" || payload.ResultJSONPath == "" {
		t.Fatalf("emit-only script %s did not include repoRoot/resultJsonPath: %+v", scriptName, payload)
	}
	return payload
}

func resolvePowerShellHost() (string, bool) {
	for _, candidate := range []string{"pwsh.exe", "pwsh", "powershell.exe", "powershell"} {
		if path, err := exec.LookPath(candidate); err == nil && path != "" {
			return path, true
		}
	}
	return "", false
}

func resolveWindowsPowerShellHost() (string, bool) {
	for _, candidate := range []string{"powershell.exe", "powershell"} {
		if path, err := exec.LookPath(candidate); err == nil && path != "" {
			return path, true
		}
	}
	return "", false
}

func scriptDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "scripts", "windows"))
	if err != nil {
		t.Fatalf("resolve script dir failed: %v", err)
	}
	return path
}

func expectedRepoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Clean(filepath.Join(scriptDir(t), "..", ".."))
}

func assertPathEqual(t *testing.T, actual, expected, label string) {
	t.Helper()
	if !strings.EqualFold(filepath.Clean(actual), filepath.Clean(expected)) {
		t.Fatalf("expected %s %q, got %q", label, expected, actual)
	}
}

func containsAllArgs(args []string, required ...string) bool {
	for _, item := range required {
		found := false
		for _, arg := range args {
			if arg == item {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
