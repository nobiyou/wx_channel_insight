package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimecfg "github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestFileMigrationDryRunDoesNotConnectOrWrite(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{"ingest-state": `{"items":{}}`, "workspace-state": `{}`} {
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	err := runFileStoreMigration([]string{"--source-dir", dir}, runtimecfg.Config{Postgres: runtimecfg.PostgresConfig{DSN: "invalid"}}, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("dry run changed source: entries=%v err=%v", entries, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "accounts.json"), []byte(`{"items":`), 0600); err != nil {
		t.Fatal(err)
	}
	err = runFileStoreMigration([]string{"--source-dir", dir, "--apply"}, runtimecfg.Config{}, func(string, ...any) {})
	if err == nil || !strings.Contains(err.Error(), "invalid JSON in accounts") {
		t.Fatalf("must reject corrupt source before connecting: %v", err)
	}
}

func TestFileMigrationRequiresEncryptionBeforeConnecting(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{"ingest-state": `{"items":{}}`, "workspace-state": `{}`, "ai-service-settings": `{"owner":{"apiKey":"test-not-a-real-secret"}}`} {
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	err := runFileStoreMigration([]string{"--source-dir", dir, "--apply"}, runtimecfg.Config{}, func(string, ...any) {})
	if err == nil || !strings.Contains(err.Error(), "encryption key is required before migrating") {
		t.Fatalf("must reject unencrypted migration before opening destination: %v", err)
	}
	if strings.Contains(err.Error(), "test-not-a-real-secret") {
		t.Fatal("migration error exposed a source credential")
	}
}
