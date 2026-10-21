//go:build windows

package windows_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustClosedTCPPorts(t *testing.T, count int) []int {
	t.Helper()
	ports := make([]int, 0, count)
	listeners := make([]net.Listener, 0, count)
	for i := 0; i < count; i++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve local TCP port failed: %v", err)
		}
		listeners = append(listeners, listener)
		ports = append(ports, listener.Addr().(*net.TCPAddr).Port)
	}
	for _, listener := range listeners {
		if err := listener.Close(); err != nil {
			t.Fatalf("release local TCP port failed: %v", err)
		}
	}
	return ports
}

// Opt in after building a Windows package; normal unit runs need no release artifacts.
func TestBuiltWindowsPackageLifecycle(t *testing.T) {
	source := os.Getenv("INSIGHT_PACKAGE_TO_TEST")
	if source == "" {
		t.Skip("set INSIGHT_PACKAGE_TO_TEST to a freshly built Windows package")
	}
	root := filepath.Join(t.TempDir(), "package with spaces")
	if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(root, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.example.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	scheduler := config["scheduler"].(map[string]any)
	scheduler["run_on_start"] = false
	scheduler["poll_interval"] = "1h"
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	host, ok := resolveWindowsPowerShellHost()
	if !ok {
		t.Skip("PowerShell unavailable")
	}
	if requested := os.Getenv("INSIGHT_PACKAGE_TEST_HOST"); requested != "" {
		host = requested
	}
	env := []string{}
	for _, value := range os.Environ() {
		upper := strings.ToUpper(value)
		if !strings.HasPrefix(upper, "INSIGHT_") && !strings.HasPrefix(upper, "OPENAI_") {
			env = append(env, value)
		}
	}
	commandIndex := 0
	run := func(script string, args ...string) ([]byte, error) {
		cmdArgs := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(root, "scripts", "windows", script), "-PackageRoot", root}
		cmd := exec.Command(host, append(cmdArgs, args...)...)
		cmd.Env = env
		// Windows descendants may inherit pipe handles even when their own logs
		// are redirected. File capture lets us wait for the launcher itself.
		commandIndex++
		logPath := filepath.Join(filepath.Dir(root), fmt.Sprintf("command-%d.log", commandIndex))
		logFile, err := os.Create(logPath)
		if err != nil {
			return nil, err
		}
		cmd.Stdout, cmd.Stderr = logFile, logFile
		err = cmd.Run()
		_ = logFile.Close()
		output, readErr := os.ReadFile(logPath)
		if err != nil {
			return output, err
		}
		return output, readErr
	}
	port := mustClosedTCPPorts(t, 1)[0]
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	baseURL := "http://" + addr
	client := &http.Client{Timeout: 10 * time.Second}
	stop := func() {
		out, err := run("stop-insight-preview.ps1")
		if err != nil {
			t.Errorf("stop package: %v\n%s", err, out)
		}
	}
	t.Cleanup(stop)
	start := func() {
		t.Helper()
		out, err := run("start-insight-preview.ps1", "-APIAddr", addr, "-NoBrowser")
		if err != nil {
			t.Fatalf("start package: %v\n%s", err, out)
		}
		var state struct {
			Status             string `json:"status"`
			APIProcessID       int    `json:"apiProcessId"`
			SchedulerProcessID int    `json:"schedulerProcessId"`
		}
		if err := json.Unmarshal(out, &state); err != nil || state.Status != "ready" {
			t.Fatalf("invalid runtime state: %v %s", err, out)
		}
		// Retain handles to our own child processes for cleanup even if stop regresses.
		for _, id := range []int{state.APIProcessID, state.SchedulerProcessID} {
			process, err := os.FindProcess(id)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = process.Kill(); _ = process.Release() })
		}
	}
	start()
	for _, path := range []string{"/api/healthz", "/", "/api/v1/dashboard", "/api/v1/runtime/status"} {
		response, err := client.Get(baseURL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("GET %s: %d %s", path, response.StatusCode, body)
		}
	}
	if out, err := run("start-insight-preview.ps1", "-APIAddr", addr, "-NoBrowser"); err == nil || !strings.Contains(string(out), "already in use") {
		t.Fatalf("duplicate start must refuse occupied port: %v %s", err, out)
	}
	payload := `{"name":"package persistence","scopeMode":"all_saved_bindings","status":"paused","interval":"1h"}`
	response, err := client.Post(baseURL+"/api/v1/monitoring/tasks", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatalf("create persistence fixture: %d %s", response.StatusCode, body)
	}
	var task struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &task); err != nil || task.ID == "" {
		t.Fatalf("invalid created task: %s %v", body, err)
	}
	cmd := exec.Command(filepath.Join(root, "bin", "insight-jobs.exe"), "status")
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "dashboard=sqlite") {
		t.Fatalf("jobs did not load package SQLite config: %v %s", err, out)
	}
	stop()
	if _, err := os.Stat(filepath.Join(root, "data", "insight.db")); err != nil {
		t.Fatal(err)
	}
	start()
	response, err = client.Get(baseURL + "/api/v1/monitoring/tasks/" + task.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(string(body), "package persistence") {
		t.Fatalf("task lost on restart: %d %s", response.StatusCode, body)
	}
	stop()
	if response, err := client.Get(baseURL + "/api/healthz"); err == nil {
		response.Body.Close()
		t.Fatal("API still running after package stop")
	}
}
