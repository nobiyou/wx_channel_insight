package ingest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestServiceResolvesHostLocalBindingToConfiguredContainerEndpoint(t *testing.T) {
	service := NewService(nil, ServiceConfig{
		WXFeedListClient: WXFeedListClient{BaseURL: "http://host.docker.internal:2026"},
	})

	resolved := service.resolveRuntimeSourceBinding(SourceBinding{
		Source:  RunSourceWXVideoSearch,
		BaseURL: "http://127.0.0.1:2026",
		Keyword: "codex",
	})
	if resolved.BaseURL != "http://host.docker.internal:2026" {
		t.Fatalf("expected container endpoint, got %q", resolved.BaseURL)
	}
	if persisted := service.persistedWXFeedBaseURL(resolved.BaseURL); persisted != "" {
		t.Fatalf("expected local endpoint to remain environment-independent, got %q", persisted)
	}
}

func TestResolveWXCommentExportPathWithRootMapsHostCommentDataPath(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "2026-08-07", "comment.json")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir comment data root: %v", err)
	}
	payload, err := json.Marshal(WXCommentExportFile{
		ObjectID:      "object-1",
		ObjectNonceID: "nonce-1",
		Title:         "真实视频",
		SavedAt:       "2026-08-07T01:00:00+08:00",
		Source:        "finderGetCommentList",
	})
	if err != nil {
		t.Fatalf("marshal comment export: %v", err)
	}
	if err := os.WriteFile(filePath, payload, 0o644); err != nil {
		t.Fatalf("write comment export: %v", err)
	}

	resolution, err := resolveWXCommentExportPathWithRoot(
		`E:\netdisk\wx_channel\downloads\comment_data\2026-08-07\comment.json`,
		root,
	)
	if err != nil {
		t.Fatalf("resolve mapped comment export: %v", err)
	}
	if resolution.ResolvedPath != filepath.Clean(filePath) {
		t.Fatalf("expected mapped path %q, got %q", filePath, resolution.ResolvedPath)
	}
}
