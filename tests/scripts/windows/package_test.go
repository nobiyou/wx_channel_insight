//go:build windows

package windows_test

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func packageFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range map[string]string{
		"go.mod": "module example.com/packagefixture\n\ngo 1.25.0\n",
		"go.sum": "", ".dockerignore": "**/.env\n", "LICENSE": "Apache License\n", "README.md": "# Fixture\n",
		"config.example.json": `{ "storage": {"driver":"sqlite"}}`, "config.postgres.example.json": `{}`,
		"apps/api/main.go": "package main\nfunc main() {}\n", "apps/jobs/main.go": "package main\nfunc main() {}\n",
		"apps/scheduler/main.go": "package main\nfunc main() {}\n", "internal/sample/model.go": "package sample\n",
		"migrations/postgres/001_init.sql": "SELECT 1;",
		"apps/web/package.json":            `{}`, "apps/web/package-lock.json": `{}`, "apps/web/index.html": "<html></html>",
		"apps/web/vite.config.js": "export default {};", "apps/web/src/App.vue": "<template>ok</template>",
		"deploy/stack/docker-compose.yml":         "services:\n  web:\n    build:\n      target: prebuilt\n",
		"deploy/stack/docker-compose.offline.yml": "services:\n  api:\n    image: insight-preview/api:preview\n  web:\n    image: insight-preview/web:preview\n  postgres:\n    image: insight-preview/postgres:16-alpine\n  redis:\n    image: insight-preview/redis:7-alpine\n",
		"deploy/stack/Dockerfile.api":             "FROM scratch\n", "deploy/stack/Dockerfile.web": "FROM scratch\n",
		"deploy/stack/nginx.conf": "# nginx\n", "deploy/stack/.env.example": "INSIGHT_POSTGRES_PASSWORD=CHANGE_ME\n",
		"deploy/stack/README.md": "# Deploy\n", "deploy/stack/DEPLOYMENT.md": "# Deploy\n",
		"deploy/stack/DEPLOYMENT-OFFLINE.md": "# Offline\n",
		"deploy/stack/.env":                  "SECRET_SENTINEL", "deploy/stack/.launch-eval.compose.yml": "SECRET_SENTINEL",
		"apps/api/var/private.json": "SECRET_SENTINEL", "internal/runtime/var/private.json": "SECRET_SENTINEL",
		"internal/sample/private.db": "SECRET_SENTINEL", "apps/web/.env.local": "SECRET_SENTINEL",
		"apps/web/node_modules/private.js": "SECRET_SENTINEL", "apps/web/dist/private.js": "SECRET_SENTINEL",
		"docs/private.md": "SECRET_SENTINEL", "insight.local.json": "SECRET_SENTINEL",
	} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runPackagingScript(t *testing.T, script string, args ...string) ([]byte, error) {
	t.Helper()
	host, ok := resolvePowerShellHost()
	if !ok {
		t.Skip("PowerShell is unavailable")
	}
	cmdArgs := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(scriptDir(t), script)}
	return exec.Command(host, append(cmdArgs, args...)...).CombinedOutput()
}

func TestSourcePackageIncludesPublicFilesAndExcludesLocalState(t *testing.T) {
	root := packageFixture(t)
	output := filepath.Join(root, "release", "source.zip")
	data, err := runPackagingScript(t, "build-insight-docker-package.ps1", "-RepoRoot", root, "-OutputPath", output)
	if err != nil {
		t.Fatalf("package failed: %v\n%s", err, data)
	}
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	entries := map[string]bool{}
	for _, file := range archive.File {
		name := strings.SplitN(file.Name, "/", 2)
		if len(name) == 2 {
			entries[name[1]] = true
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buf strings.Builder
		_, err = io.Copy(&buf, reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), "SECRET_SENTINEL") {
			t.Errorf("local state leaked into %s", file.Name)
		}
	}
	for _, path := range []string{"LICENSE", ".dockerignore", "config.example.json", "config.postgres.example.json", "deploy/stack/.env.example", "apps/api/main.go", "internal/sample/model.go", "migrations/postgres/001_init.sql", "scripts/windows/start-insight-docker-stack.ps1", "manifest.json"} {
		if !entries[path] {
			t.Errorf("missing public package input %s", path)
		}
	}
}

func TestPackageForceRefusesSourceDirectoryAndPreservesItsContents(t *testing.T) {
	root := packageFixture(t)
	output := filepath.Join(root, "internal")
	data, err := runPackagingScript(t, "build-insight-preview.ps1", "-RepoRoot", root, "-OutputPath", output, "-Force")
	if err == nil {
		t.Fatalf("must reject source directory as output: %s", data)
	}
	if _, err := os.Stat(filepath.Join(output, "sample", "model.go")); err != nil {
		t.Fatalf("source was changed: %v\n%s", err, data)
	}
}

func TestWindowsLauncherUsesPackageConfigWithoutCreatingStateInEmitOnly(t *testing.T) {
	root := t.TempDir()
	cfg := `{"server":{"addr":"127.0.0.1:23456"},"storage":{"driver":"sqlite","data_dir":"data","sqlite_path":"data/insight.db"},"bridge":{"local_operator":true},"runtime":{"default_hub_user_id":"test-owner"}}`
	if err := os.WriteFile(filepath.Join(root, "config.example.json"), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := runPackagingScript(t, "start-insight-preview.ps1", "-PackageRoot", root, "-EmitOnly")
	if err != nil {
		t.Fatalf("emit failed: %v\n%s", err, data)
	}
	var payload struct {
		APIURL     string            `json:"apiURL"`
		ConfigFile string            `json:"configFile"`
		Env        map[string]string `json:"env"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	if payload.APIURL != "http://127.0.0.1:23456" || payload.ConfigFile != filepath.Join(root, "insight.local.json") {
		t.Fatalf("config not respected: %+v", payload)
	}
	for _, key := range []string{"INSIGHT_STORE", "INSIGHT_DASHBOARD_STORE", "INSIGHT_LOCAL_OPERATOR", "OPENAI_API_KEY"} {
		if _, exists := payload.Env[key]; exists {
			t.Errorf("launcher must not override or expose %s", key)
		}
	}
	for _, path := range []string{"insight.local.json", "data"} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Errorf("emit-only created %s", path)
		}
	}
}

func runPackageCode(t *testing.T, script, root, code string) ([]byte, error) {
	t.Helper()
	host, ok := resolvePowerShellHost()
	if !ok {
		t.Skip("PowerShell unavailable")
	}
	cmd := exec.Command(host, "-NoProfile", "-NonInteractive", "-Command", code)
	cmd.Env = append(os.Environ(), "INSIGHT_TEST_SCRIPT="+filepath.Join(scriptDir(t), script), "INSIGHT_TEST_ROOT="+root)
	return cmd.CombinedOutput()
}

func TestOfflinePackagePreservesImageRefsAndRecordsActualImageIdentity(t *testing.T) {
	root := packageFixture(t)
	data, err := runPackageCode(t, "build-insight-docker-offline-package.ps1", root, `
function docker {
    $global:LASTEXITCODE = 0
    if ($args[0] -eq 'info') { return }
    if ($args[0] -eq 'image') { return '{"id":"sha256:fixture","os":"linux","architecture":"amd64","revision":"image-revision"}' }
    if ($args[0] -eq 'save') { [IO.File]::WriteAllText($args[2], ($args -join ' ')); return }
    throw "Unexpected Docker mutation: $args"
}
& $env:INSIGHT_TEST_SCRIPT -RepoRoot $env:INSIGHT_TEST_ROOT -OutputPath (Join-Path $env:INSIGHT_TEST_ROOT 'release/offline.zip') -ApiImageSource 'example/api:tested' -WebImageSource 'example/web:tested'
`)
	if err != nil {
		t.Fatalf("offline package failed: %v\n%s", err, data)
	}
	archive, err := zip.OpenReader(filepath.Join(root, "release", "offline.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	entries := map[string]string{}
	for _, file := range archive.File {
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.SplitN(file.Name, "/", 2)
		if len(parts) == 2 {
			entries[parts[1]] = string(body)
		}
	}
	for _, name := range []string{"LICENSE", ".env.example", "manifest.json", "images/insight-runtime-images.tar"} {
		if entries[name] == "" {
			t.Errorf("missing %s", name)
		}
	}
	for _, ref := range []string{"example/api:tested", "example/web:tested"} {
		if !strings.Contains(entries["docker-compose.yml"], ref) || !strings.Contains(entries["images/insight-runtime-images.tar"], ref) {
			t.Errorf("image ref lost: %s", ref)
		}
	}
	if !strings.Contains(entries["manifest.json"], "sha256:fixture") || !strings.Contains(entries["manifest.json"], "image-revision") {
		t.Error("actual image provenance not recorded")
	}
}

func TestJobsBuildFailureDoesNotReportStaleBinaryAsSuccess(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "bin", "wx_channel_insight_jobs.exe")
	if err := os.WriteFile(output, []byte("old executable"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := runPackageCode(t, "build-insight-jobs.ps1", root, `
function go { $global:LASTEXITCODE = 23 }
& $env:INSIGHT_TEST_SCRIPT -RepoRoot $env:INSIGHT_TEST_ROOT
`)
	if err == nil {
		t.Fatalf("failed build reported success: %s", data)
	}
	contents, err := os.ReadFile(output)
	if err != nil || string(contents) != "old executable" {
		t.Fatalf("old binary changed: %s %v", contents, err)
	}
}

func TestPackageForceProtectsRuntimeData(t *testing.T) {
	root := packageFixture(t)
	output := filepath.Join(root, "release", "used-package")
	if err := os.MkdirAll(filepath.Join(output, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"manifest.json": "{}", "data/insight.db": "live data"} {
		if err := os.WriteFile(filepath.Join(output, filepath.FromSlash(name)), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := runPackagingScript(t, "build-insight-preview.ps1", "-RepoRoot", root, "-OutputPath", output, "-Force")
	if err == nil {
		t.Fatalf("must reject runtime package: %s", data)
	}
	contents, err := os.ReadFile(filepath.Join(output, "data", "insight.db"))
	if err != nil || string(contents) != "live data" {
		t.Fatalf("runtime data changed: %s %v", contents, err)
	}
}

func TestSourceBuildFailurePreservesPreviousArchive(t *testing.T) {
	root := packageFixture(t)
	if err := os.Remove(filepath.Join(root, "LICENSE")); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "release", "source.zip")
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("previous archive"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := runPackagingScript(t, "build-insight-docker-package.ps1", "-RepoRoot", root, "-OutputPath", output, "-Force")
	if err == nil {
		t.Fatalf("missing required input must fail: %s", data)
	}
	contents, err := os.ReadFile(output)
	if err != nil || string(contents) != "previous archive" {
		t.Fatalf("previous package changed: %s %v", contents, err)
	}
	stages, err := filepath.Glob(filepath.Join(root, "release", ".tmp-package-*"))
	if err != nil || len(stages) != 0 {
		t.Fatalf("staging directory leaked: %v %v", stages, err)
	}
}

func TestOfflinePackageRequiresExplicitApplicationImages(t *testing.T) {
	root := packageFixture(t)
	data, err := runPackageCode(t, "build-insight-docker-offline-package.ps1", root, `
function docker { throw 'Docker must not be called for unspecified application images' }
& $env:INSIGHT_TEST_SCRIPT -RepoRoot $env:INSIGHT_TEST_ROOT
`)
	if err == nil || !strings.Contains(string(data), "explicit tagged image") {
		t.Fatalf("must validate image references first: %v\n%s", err, data)
	}
}

func TestSourcePackagingAndLauncherWorkInWindowsPowerShell(t *testing.T) {
	host, ok := resolveWindowsPowerShellHost()
	if !ok {
		t.Skip("Windows PowerShell unavailable")
	}
	root := packageFixture(t)
	output := filepath.Join(root, "release", "ps51.zip")
	cmd := exec.Command(host, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(scriptDir(t), "build-insight-docker-package.ps1"), "-RepoRoot", root, "-OutputPath", output)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PS 5.1 packaging: %v\n%s", err, data)
	}
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	found := false
	for _, file := range archive.File {
		if file.Name == "ps51/deploy/stack/.env.example" {
			found = true
		}
	}
	if !found {
		t.Fatal("PS 5.1 ZIP lost hidden template or portable paths")
	}
	// The source-package fixture deliberately contains an invalid private config.
	// Remove it here so this separate launcher check exercises first-run behavior.
	if err := os.Remove(filepath.Join(root, "insight.local.json")); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(host, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(scriptDir(t), "start-insight-preview.ps1"), "-PackageRoot", root, "-EmitOnly")
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PS 5.1 launcher: %v\n%s", err, data)
	}
	var payload struct {
		ConfigFile string `json:"configFile"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.ConfigFile != filepath.Join(root, "insight.local.json") {
		t.Fatalf("PS 5.1 invalid plan: %v\n%s", err, data)
	}
}

func TestDockerLauncherDoesNotOverrideEnvFileDefaultsOrWriteInEmitOnly(t *testing.T) {
	root := packageFixture(t)
	commentPath := filepath.Join(root, "new-comments")
	data, err := runPackagingScript(t, "start-insight-docker-stack.ps1", "-RepoRoot", root, "-WxChannelCommentDataDir", commentPath, "-EmitOnly")
	if err != nil {
		t.Fatalf("docker emit failed: %v\n%s", err, data)
	}
	var payload struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"INSIGHT_WX_CHANNEL_BASE_URL", "INSIGHT_DEFAULT_HUB_USER_ID", "INSIGHT_DEFAULT_DEVICE_COUNT", "INSIGHT_SCHEDULER_HUB_USER_ID"} {
		if _, ok := payload.Env[key]; ok {
			t.Errorf("implicit override of .env setting: %s", key)
		}
	}
	if _, err := os.Stat(commentPath); !os.IsNotExist(err) {
		t.Error("EmitOnly created comment directory")
	}
}

func TestStopPackageVerifiesRecordedProcessTimeWithParsedJSONDates(t *testing.T) {
	root := t.TempDir()
	data, err := runPackageCode(t, "stop-insight-preview.ps1", root, `
$expected = Join-Path $env:INSIGHT_TEST_ROOT 'bin/insight-api.exe'
$started = [DateTime]::Parse('2026-10-20T10:00:00.1234567Z').ToUniversalTime()
New-Item -ItemType Directory -Path (Join-Path $env:INSIGHT_TEST_ROOT 'data') | Out-Null
$state = @{ apiProcessId=1234; apiExecutable=$expected; apiStartedAt=$started.ToString('o'); schedulerProcessId=0 }
[IO.File]::WriteAllText((Join-Path $env:INSIGHT_TEST_ROOT 'data/preview-runtime.json'), ($state | ConvertTo-Json))
$global:stopped = 0
function Get-Process { [CmdletBinding()]param([int]$Id); [pscustomobject]@{Path=$expected; StartTime=$started; Id=$Id} }
function Stop-Process { [CmdletBinding()]param([int]$Id,[switch]$Force); $global:stopped++ }
& $env:INSIGHT_TEST_SCRIPT -PackageRoot $env:INSIGHT_TEST_ROOT
if ($global:stopped -ne 1) { throw 'Expected the verified package process to be stopped' }
`)
	if err != nil {
		t.Fatalf("recorded time mismatch: %v\n%s", err, data)
	}
}
