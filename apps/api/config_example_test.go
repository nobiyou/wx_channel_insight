package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	runtimecfg "github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestLocalConfigExampleAcceptsDefaultWorkspaceOnLoopback(t *testing.T) {
	cfg, err := runtimecfg.LoadConfigFile(filepath.Join("..", "..", "config.example.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIAddr != "127.0.0.1:18081" || cfg.DashboardStore.Mode != runtimecfg.StoreModeSQLite {
		t.Fatal("local example must use loopback and SQLite")
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	ctx, err := buildHubRequestContext(request, cfg.Bridge, nil)
	if err != nil || ctx.ContextSource != "local_operator" {
		t.Fatalf("local example default workspace is inaccessible: %v", err)
	}
}

func TestPostgresConfigExampleRequiresTrustedIdentity(t *testing.T) {
	cfg, err := runtimecfg.LoadConfigFile(filepath.Join("..", "..", "config.postgres.example.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	if _, err := buildHubRequestContext(request, cfg.Bridge, nil); err == nil {
		t.Fatal("postgres example must require a configured trusted bridge")
	}
}
