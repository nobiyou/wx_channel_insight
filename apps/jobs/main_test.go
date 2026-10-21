package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimecfg "github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestRunStatusLogsCurrentMode(t *testing.T) {
	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}

	err := run(nil, runtimecfg.Config{
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(t.TempDir(), "comment-insights-status.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeMemory},
	}, logf)
	if err != nil {
		t.Fatalf("expected status command success, got %v", err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "jobs status") {
		t.Fatalf("expected status log, got %+v", lines)
	}
	if !strings.Contains(lines[0], "rankings=") || !strings.Contains(lines[0], "comment_insights=") || !strings.Contains(lines[0], "report_summary=") {
		t.Fatalf("expected expanded status output, got %+v", lines)
	}
}

func TestRunBootstrapSchemaRequiresPostgresDSN(t *testing.T) {
	err := run([]string{"bootstrap-schema"}, runtimecfg.Config{}, func(string, ...any) {})
	if err == nil || !strings.Contains(err.Error(), "INSIGHT_POSTGRES_DSN") {
		t.Fatalf("expected missing dsn error, got %v", err)
	}
}

func TestRunCheckMigrationsSucceeds(t *testing.T) {
	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}

	err := run([]string{"check-migrations"}, runtimecfg.Config{}, logf)
	if err != nil {
		t.Fatalf("expected check-migrations success, got %v", err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "migrations directory is present") {
		t.Fatalf("expected migrations check log, got %+v", lines)
	}
}

func TestParseCommentExportArgsRequiresPath(t *testing.T) {
	_, err := parseCommentExportArgs(nil)
	if err == nil || !strings.Contains(err.Error(), "--path") {
		t.Fatalf("expected missing path error, got %v", err)
	}
}

func TestParseFeedListArgsRequiresUsername(t *testing.T) {
	_, err := parseFeedListArgs(nil)
	if err == nil || !strings.Contains(err.Error(), "--username") {
		t.Fatalf("expected missing username error, got %v", err)
	}
}

func TestParseFeedListSearchArgsRequiresKeyword(t *testing.T) {
	_, err := parseFeedListSearchArgs(nil)
	if err == nil || !strings.Contains(err.Error(), "--keyword") {
		t.Fatalf("expected missing keyword error, got %v", err)
	}
}

func TestParseBackfillWXVideoCoversArgsAcceptsObjectIDs(t *testing.T) {
	options, err := parseBackfillWXVideoCoversArgs([]string{
		"--hub-user-id", "insight:operator:local",
		"--device-count", "1",
		"--object-id", "video-1",
		"--object-id", "video-2",
	})
	if err != nil {
		t.Fatalf("parse backfill args: %v", err)
	}
	if options.hubUserID != "insight:operator:local" || options.deviceCount != 1 || len(options.objectIDs) != 2 || options.objectIDs[1] != "video-2" {
		t.Fatalf("unexpected backfill options: %+v", options)
	}
}

func TestParseRetryIngestArgsRequiresSource(t *testing.T) {
	_, err := parseRetryIngestArgs(nil)
	if err == nil || !strings.Contains(err.Error(), "--source") {
		t.Fatalf("expected missing source error, got %v", err)
	}
}

func TestParseSaveTrustedRuntimeEvidenceArgsRequiresProofPath(t *testing.T) {
	_, err := parseSaveTrustedRuntimeEvidenceArgs(nil)
	if err == nil || !strings.Contains(err.Error(), "--proof-path") {
		t.Fatalf("expected missing proof-path error, got %v", err)
	}
}

func TestParseSyncSavedBindingsArgsRejectsUnsupportedSource(t *testing.T) {
	_, err := parseSyncSavedBindingsArgs([]string{"--sources", "unknown_source"})
	if err == nil || !strings.Contains(err.Error(), "unsupported source") {
		t.Fatalf("expected unsupported source error, got %v", err)
	}
}

func TestParseOperatorLoopArgsRejectsZeroInterval(t *testing.T) {
	_, err := parseOperatorLoopArgs([]string{"--interval", "0s"})
	if err == nil || !strings.Contains(err.Error(), "greater than 0") {
		t.Fatalf("expected invalid interval error, got %v", err)
	}
}

func TestParseOperatorLoopArgsRejectsNegativeIterations(t *testing.T) {
	_, err := parseOperatorLoopArgs([]string{"--iterations", "-1"})
	if err == nil || !strings.Contains(err.Error(), "0 or greater") {
		t.Fatalf("expected invalid iterations error, got %v", err)
	}
}

func TestRunImportWXCommentExportWorksWithFileStore(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		CommentInsightStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "comment-insights-state.json"),
		},
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "ingest-state.json"),
		},
	}
	payload := `{
  "objectId": "oid-200",
  "objectNonceId": "nid-200",
  "title": "评论导出测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先说说怎么落地",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}

	err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf)
	if err != nil {
		t.Fatalf("expected import-wx-comment-export success, got %v", err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "import-wx-comment-export success") {
		t.Fatalf("expected success log, got %+v", lines)
	}
}

func TestRunSearchWXFeedAccountsReturnsJSONCandidates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/contact/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("keyword") != "teamlight" {
			t.Fatalf("unexpected keyword: %s", r.URL.Query().Get("keyword"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"data": {
					"BaseResponse": {
						"ErrMsg": { "String": "" },
						"Ret": 0
					},
					"infoList": [{
						"contact": {
							"nickname": "TEAMLIGHT-",
							"username": "v2_060000231003b20faec8c5e28b10c6d6ce01ea36b077f6a0e58226dc63fe48550812f2bdbca9@finder",
							"signature": "",
							"headUrl": "https://example.com/avatar.png"
						}
					}]
				}
			}
		}`))
	}))
	defer server.Close()

	cfg := runtimecfg.Config{
		DefaultHubUserID: "hub:user:demo",
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeMemory,
		},
	}
	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}

	if err := run([]string{"search-wx-feed-accounts", "--base-url", server.URL, "--keyword", "teamlight"}, cfg, logf); err != nil {
		t.Fatalf("expected search-wx-feed-accounts success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one search result line, got %+v", lines)
	}

	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadMap); err != nil {
		t.Fatalf("expected JSON search result, got %v", err)
	}
	if payloadMap["status"] != "ok" || payloadMap["keyword"] != "teamlight" {
		t.Fatalf("unexpected search metadata: %+v", payloadMap)
	}
	items, ok := payloadMap["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one search item, got %+v", payloadMap["items"])
	}
}

func TestRunSaveTrustedRuntimeEvidenceWritesFormalStore(t *testing.T) {
	tempDir := t.TempDir()
	proofDir := filepath.Join(tempDir, ".tmp-codex")
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		t.Fatalf("mkdir proof dir failed: %v", err)
	}

	proofPath := filepath.Join(proofDir, "verify-docker-stack-proof-latest.json")
	proofPayload := `{
  "status": "success",
  "hubUserId": "hub:user:proof",
  "apiPort": 61288,
  "webPort": 61289,
  "results": [
    {
      "name": "check-runtime-status",
      "exitCode": 0,
      "output": "{\"hubBridgeMode\":\"trusted-header\",\"demoContextEnabled\":false}"
    },
    {
      "name": "check-runtime-trusted-context",
      "exitCode": 0,
      "output": "{\"hubUserId\":\"hub:user:proof\",\"contextSource\":\"trusted_header\"}"
    },
    {
      "name": "check-runtime-browser-query-blocked",
      "exitCode": 0,
      "output": "{\"statusCode\":401,\"json\":null}"
    },
    {
      "name": "check-trusted-web-context",
      "exitCode": 0,
      "output": "{\"hubUserId\":\"hub:user:proof\",\"contextSource\":\"trusted_header\"}"
    },
    {
      "name": "check-trusted-web-browser-query-isolated",
      "exitCode": 0,
      "output": "{\"hubUserId\":\"hub:user:proof\",\"contextSource\":\"trusted_header\"}"
    }
  ]
}`
	if err := os.WriteFile(proofPath, []byte(proofPayload), 0o644); err != nil {
		t.Fatalf("write proof file failed: %v", err)
	}

	outputPath := filepath.Join(tempDir, "var", "trusted-runtime-evidence.json")
	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}

	if err := run([]string{"save-trusted-runtime-evidence", "--proof-path", proofPath, "--output-path", outputPath}, runtimecfg.Config{}, logf); err != nil {
		t.Fatalf("expected save-trusted-runtime-evidence success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one output line, got %+v", lines)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if payload["status"] != "ok" || payload["source"] != "verify-insight-docker-stack" {
		t.Fatalf("expected success output payload, got %+v", payload)
	}
	if payload["outputPath"] != outputPath {
		t.Fatalf("expected resolved output path, got %+v", payload)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("expected formal evidence file to exist, got %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("expected valid stored evidence JSON, got %v", err)
	}
	if stored["source"] != "verify-insight-docker-stack" || stored["hubUserId"] != "hub:user:proof" {
		t.Fatalf("unexpected stored evidence payload: %+v", stored)
	}
}

func TestRunShowIngestSummaryReturnsJSON(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		CommentInsightStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "comment-insights-state.json"),
		},
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "ingest-state.json"),
		},
	}
	payload := `{
  "objectId": "oid-201",
  "objectNonceId": "nid-201",
  "title": "show summary 测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先说说怎么落地",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}

	if err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}

	if err := run([]string{"show-ingest-summary", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected show-ingest-summary success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one summary line, got %+v", lines)
	}

	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadMap); err != nil {
		t.Fatalf("expected JSON summary, got %v line=%q", err, lines[0])
	}
	if payloadMap["hubUserID"] != "hub:user:alice" {
		t.Fatalf("expected hub user id, got %v", payloadMap["hubUserID"])
	}
	if payloadMap["freshnessState"] == "" {
		t.Fatalf("expected freshness state, got %+v", payloadMap)
	}
}

func TestRunShowIngestRunsReturnsJSON(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		CommentInsightStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "comment-insights-state.json"),
		},
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "ingest-state.json"),
		},
	}
	payload := `{
  "objectId": "oid-202",
  "objectNonceId": "nid-202",
  "title": "show runs 测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先说说怎么落地",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"show-ingest-runs", "--hub-user-id", "hub:user:alice"}, cfg, logf); err != nil {
		t.Fatalf("expected show-ingest-runs success, got %v", err)
	}
	var payloadArray []map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadArray); err != nil {
		t.Fatalf("expected JSON runs, got %v", err)
	}
	if len(payloadArray) == 0 {
		t.Fatalf("expected at least one run, got %+v", payloadArray)
	}
}

func TestRunShowIngestSourcesReturnsJSON(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		CommentInsightStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "comment-insights-state.json"),
		},
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "ingest-state.json"),
		},
	}
	payload := `{
  "objectId": "oid-203",
  "objectNonceId": "nid-203",
  "title": "show sources 测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先说说怎么落地",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"show-ingest-sources", "--hub-user-id", "hub:user:alice"}, cfg, logf); err != nil {
		t.Fatalf("expected show-ingest-sources success, got %v", err)
	}
	var payloadArray []map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadArray); err != nil {
		t.Fatalf("expected JSON source statuses, got %v", err)
	}
	if len(payloadArray) == 0 {
		t.Fatalf("expected at least one source status, got %+v", payloadArray)
	}
}

func TestRunShowIngestBindingsReturnsJSON(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		CommentInsightStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "comment-insights-state.json"),
		},
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "ingest-state.json"),
		},
	}
	payload := `{
  "objectId": "oid-206",
  "objectNonceId": "nid-206",
  "title": "show bindings 测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先说说怎么落地",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"show-ingest-bindings", "--hub-user-id", "hub:user:alice"}, cfg, logf); err != nil {
		t.Fatalf("expected show-ingest-bindings success, got %v", err)
	}

	var payloadArray []map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadArray); err != nil {
		t.Fatalf("expected JSON bindings, got %v", err)
	}
	if len(payloadArray) != 1 {
		t.Fatalf("expected one binding, got %+v", payloadArray)
	}
	if payloadArray[0]["source"] != "wx_comment_export" {
		t.Fatalf("expected comment export binding, got %+v", payloadArray[0])
	}
	if payloadArray[0]["path"] == "" {
		t.Fatalf("expected persisted path, got %+v", payloadArray[0])
	}
}

func TestRunRetryLastFailedIngestRefreshesReadModel(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		DashboardStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "dashboard-state.json"),
		},
		RankingStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "rankings-state.json"),
		},
		CommentInsightStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "comment-insights-state.json"),
		},
		ReportSummaryStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "report-summary-state.json"),
		},
		WorkspaceStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "workspace-state.json"),
		},
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "ingest-state.json"),
		},
	}

	payload := `{
  "objectId": "oid-207",
  "objectNonceId": "nid-207",
  "title": "retry comment export 测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先说说怎么落地",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"sync-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected sync success, got %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(tempDir, "ingest-state.json"))
	if err != nil {
		t.Fatalf("read ingest state failed: %v", err)
	}
	updated := strings.Replace(string(raw), `"status": "success"`, `"status": "failed"`, 1)
	updated = strings.Replace(updated, `"message": ""`, `"message": "manual failure for retry"`, 1)
	if err := os.WriteFile(filepath.Join(tempDir, "ingest-state.json"), []byte(updated), 0o644); err != nil {
		t.Fatalf("overwrite ingest state failed: %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"retry-last-failed-ingest", "--source", "wx_comment_export", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected retry-last-failed-ingest success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one retry result line, got %+v", lines)
	}

	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadMap); err != nil {
		t.Fatalf("expected JSON retry result, got %v", err)
	}
	if payloadMap["status"] != "ok" {
		t.Fatalf("expected ok retry status, got %+v", payloadMap)
	}
	readModel, ok := payloadMap["readModel"].(map[string]any)
	if !ok || readModel["mode"] == "" {
		t.Fatalf("expected refreshed read model payload, got %+v", payloadMap)
	}
}

func TestRunSaveWXCommentExportBindingPersistsBindingWithoutImportRun(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		DashboardStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "dashboard-state.json"),
		},
		RankingStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "rankings-state.json"),
		},
		CommentInsightStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "comment-insights-state.json"),
		},
		ReportSummaryStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "report-summary-state.json"),
		},
		WorkspaceStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "workspace-state.json"),
		},
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "ingest-state.json"),
		},
	}
	path := filepath.Join(tempDir, "comment-export.json")
	payload := `{
  "objectId": "oid-save-jobs-1",
  "objectNonceId": "nid-save-jobs-1",
  "title": "jobs 保存评论导出绑定",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"先保存来源","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-05T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{
		"save-wx-comment-export-binding",
		"--path", path,
		"--hub-user-id", "hub:user:alice",
		"--device-count", "2",
	}, cfg, logf); err != nil {
		t.Fatalf("expected save-wx-comment-export-binding success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one save binding line, got %+v", lines)
	}

	var savedPayload map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &savedPayload); err != nil {
		t.Fatalf("expected JSON save binding payload, got %v", err)
	}
	if savedPayload["status"] != "ok" || savedPayload["source"] != "wx_comment_export" {
		t.Fatalf("unexpected save binding payload: %+v", savedPayload)
	}
	bindingStates, ok := savedPayload["bindingStates"].([]any)
	if !ok || len(bindingStates) != 1 {
		t.Fatalf("expected one binding state, got %+v", savedPayload)
	}
	binding, ok := bindingStates[0].(map[string]any)
	if !ok {
		t.Fatalf("expected binding state object, got %+v", bindingStates[0])
	}
	if binding["source"] != "wx_comment_export" || binding["path"] != path {
		t.Fatalf("unexpected binding state: %+v", binding)
	}
	if binding["lastRunStatus"] != nil && binding["lastRunStatus"] != "" {
		t.Fatalf("expected save-wx-comment-export-binding to avoid creating an import run, got %+v", binding)
	}
	if binding["latestBatchId"] != nil && binding["latestBatchId"] != "" {
		t.Fatalf("expected no latest batch after saving binding only, got %+v", binding)
	}

	lines = nil
	if err := run([]string{"show-ingest-bindings", "--hub-user-id", "hub:user:alice"}, cfg, logf); err != nil {
		t.Fatalf("expected show-ingest-bindings success, got %v", err)
	}
	var savedBindings []map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &savedBindings); err != nil {
		t.Fatalf("expected JSON bindings payload, got %v", err)
	}
	if len(savedBindings) != 1 || savedBindings[0]["path"] != path {
		t.Fatalf("expected persisted comment-export binding, got %+v", savedBindings)
	}
}

func TestRunSaveWXFeedListBindingPersistsBlockedBinding(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		DashboardStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "dashboard-state.json"),
		},
		RankingStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "rankings-state.json"),
		},
		CommentInsightStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "comment-insights-state.json"),
		},
		ReportSummaryStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "report-summary-state.json"),
		},
		WorkspaceStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "workspace-state.json"),
		},
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "ingest-state.json"),
		},
	}

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{
		"save-wx-feed-list-binding",
		"--base-url", "http://127.0.0.1:65530",
		"--username", "gh_team_saved",
		"--hub-user-id", "hub:user:alice",
		"--device-count", "2",
	}, cfg, logf); err != nil {
		t.Fatalf("expected save-wx-feed-list-binding success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one save binding line, got %+v", lines)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("expected JSON save binding payload, got %v", err)
	}
	if payload["status"] != "ok" || payload["source"] != "wx_feed_list" {
		t.Fatalf("unexpected save binding payload: %+v", payload)
	}
	bindingStates, ok := payload["bindingStates"].([]any)
	if !ok || len(bindingStates) != 1 {
		t.Fatalf("expected one binding state, got %+v", payload)
	}
	binding, ok := bindingStates[0].(map[string]any)
	if !ok {
		t.Fatalf("expected binding state object, got %+v", bindingStates[0])
	}
	if binding["source"] != "wx_feed_list" || binding["username"] != "gh_team_saved" {
		t.Fatalf("unexpected binding state: %+v", binding)
	}
	if binding["lastRunStatus"] != nil && binding["lastRunStatus"] != "" {
		t.Fatalf("expected save-wx-feed-list-binding to avoid creating an import run, got %+v", binding)
	}
	if binding["latestBatchId"] != nil && binding["latestBatchId"] != "" {
		t.Fatalf("expected no latest batch after saving binding only, got %+v", binding)
	}

	lines = nil
	if err := run([]string{"show-ingest-bindings", "--hub-user-id", "hub:user:alice"}, cfg, logf); err != nil {
		t.Fatalf("expected show-ingest-bindings success, got %v", err)
	}
	var savedBindings []map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &savedBindings); err != nil {
		t.Fatalf("expected JSON bindings payload, got %v", err)
	}
	if len(savedBindings) != 1 || savedBindings[0]["username"] != "gh_team_saved" {
		t.Fatalf("expected persisted feed-list binding, got %+v", savedBindings)
	}
}

func TestRunShowDashboardReflectsImportedIngestSignals(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		DashboardStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "dashboard-state.json"),
		},
		RankingStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "rankings-state.json"),
		},
		CommentInsightStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "comment-insights-state.json"),
		},
		ReportSummaryStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "report-summary-state.json"),
		},
		WorkspaceStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "workspace-state.json"),
		},
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "ingest-state.json"),
		},
	}
	payload := `{
  "objectId": "oid-204",
  "objectNonceId": "nid-204",
  "title": "show dashboard 测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先说说怎么落地",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"show-dashboard", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected show-dashboard success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one dashboard line, got %+v", lines)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadMap); err != nil {
		t.Fatalf("expected dashboard JSON, got %v", err)
	}
	ingestSummary, ok := payloadMap["ingestSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected ingestSummary object, got %+v", payloadMap)
	}
	if ingestSummary["state"] != "ready" {
		t.Fatalf("expected ready ingest state, got %+v", ingestSummary)
	}
	if ingestSummary["freshnessState"] == "" {
		t.Fatalf("expected freshness state, got %+v", ingestSummary)
	}
}

func TestRunRefreshDashboardMaterializesReadModelWithoutDuplicateOverlay(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
		DashboardStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "dashboard-state.json"),
		},
		RankingStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "rankings-state.json"),
		},
		CommentInsightStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "comment-insights-state.json"),
		},
		ReportSummaryStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "report-summary-state.json"),
		},
		WorkspaceStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "workspace-state.json"),
		},
		IngestStore: runtimecfg.StoreConfig{
			Mode: runtimecfg.StoreModeFile,
			Path: filepath.Join(tempDir, "ingest-state.json"),
		},
	}
	payload := `{
  "objectId": "oid-205",
  "objectNonceId": "nid-205",
  "title": "refresh dashboard 测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先说说怎么落地",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	var refreshLines []string
	refreshLogf := func(format string, args ...any) {
		refreshLines = append(refreshLines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"refresh-dashboard", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, refreshLogf); err != nil {
		t.Fatalf("expected refresh-dashboard success, got %v", err)
	}
	if len(refreshLines) != 1 {
		t.Fatalf("expected one refresh line, got %+v", refreshLines)
	}
	var refreshPayload map[string]any
	if err := json.Unmarshal([]byte(refreshLines[0]), &refreshPayload); err != nil {
		t.Fatalf("expected refresh JSON, got %v", err)
	}
	if refreshPayload["mode"] != "materialized" {
		t.Fatalf("expected materialized mode, got %+v", refreshPayload)
	}
	normalized, ok := refreshPayload["normalized"].(map[string]any)
	if !ok {
		t.Fatalf("expected normalized summary in refresh payload, got %+v", refreshPayload)
	}
	if normalized["contentCount"] == nil || normalized["commentCount"] == nil {
		t.Fatalf("expected normalized counts, got %+v", normalized)
	}
	aggregated, ok := refreshPayload["aggregate"].(map[string]any)
	if !ok {
		t.Fatalf("expected aggregate summary in refresh payload, got %+v", refreshPayload)
	}
	if aggregated["videoMetricCount"] == nil || aggregated["commentFactCount"] == nil {
		t.Fatalf("expected aggregate counts, got %+v", aggregated)
	}

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"show-dashboard", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected show-dashboard success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one dashboard line, got %+v", lines)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadMap); err != nil {
		t.Fatalf("expected dashboard JSON, got %v", err)
	}
	readModel, ok := payloadMap["readModel"].(map[string]any)
	if !ok {
		t.Fatalf("expected readModel object, got %+v", payloadMap)
	}
	if readModel["mode"] != "materialized" {
		t.Fatalf("expected materialized read model, got %+v", readModel)
	}
	workspaceBridge, ok := payloadMap["workspaceBridge"].(map[string]any)
	if !ok {
		t.Fatalf("expected workspaceBridge object, got %+v", payloadMap)
	}
	if workspaceBridge["contextSource"] != "jobs" {
		t.Fatalf("expected jobs context source after refresh, got %+v", workspaceBridge)
	}
}

func TestRunNormalizeIngestAndShowSummary(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
		JobsStatePath:       filepath.Join(tempDir, "jobs-runtime-state.json"),
	}
	payload := `{
  "objectId": "oid-205-normalize",
  "objectNonceId": "nid-205-normalize",
  "title": "normalize dashboard 测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先把 raw event 和 read model 中间层补出来",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	var normalizeLines []string
	normalizeLogf := func(format string, args ...any) {
		normalizeLines = append(normalizeLines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"normalize-ingest", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, normalizeLogf); err != nil {
		t.Fatalf("expected normalize-ingest success, got %v", err)
	}
	if len(normalizeLines) != 1 {
		t.Fatalf("expected one normalize line, got %+v", normalizeLines)
	}
	var normalizePayload map[string]any
	if err := json.Unmarshal([]byte(normalizeLines[0]), &normalizePayload); err != nil {
		t.Fatalf("expected normalize JSON, got %v", err)
	}
	if normalizePayload["status"] != "ok" || normalizePayload["workspaceId"] == "" {
		t.Fatalf("unexpected normalize payload: %+v", normalizePayload)
	}
	if normalizePayload["contentCount"] == nil || normalizePayload["commentCount"] == nil {
		t.Fatalf("expected normalize counts, got %+v", normalizePayload)
	}

	var summaryLines []string
	summaryLogf := func(format string, args ...any) {
		summaryLines = append(summaryLines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"show-normalized-summary", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, summaryLogf); err != nil {
		t.Fatalf("expected show-normalized-summary success, got %v", err)
	}
	if len(summaryLines) != 1 {
		t.Fatalf("expected one normalized summary line, got %+v", summaryLines)
	}
	var summaryPayload map[string]any
	if err := json.Unmarshal([]byte(summaryLines[0]), &summaryPayload); err != nil {
		t.Fatalf("expected normalized summary JSON, got %v", err)
	}
	if summaryPayload["status"] != "ok" || summaryPayload["latestBatchId"] == "" {
		t.Fatalf("unexpected normalized summary payload: %+v", summaryPayload)
	}
}

func TestRunAggregateMetricsAndShowSummary(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
		JobsStatePath:       filepath.Join(tempDir, "jobs-runtime-state.json"),
	}
	payload := `{
  "objectId": "oid-agg-205",
  "objectNonceId": "nid-agg-205",
  "title": "aggregate metrics 测试标题",
  "author": "Insight Test",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "先把 aggregate owner 补上",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	var aggregateLines []string
	aggregateLogf := func(format string, args ...any) {
		aggregateLines = append(aggregateLines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"aggregate-metrics", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, aggregateLogf); err != nil {
		t.Fatalf("expected aggregate-metrics success, got %v", err)
	}
	if len(aggregateLines) != 1 {
		t.Fatalf("expected one aggregate line, got %+v", aggregateLines)
	}
	var aggregatePayload map[string]any
	if err := json.Unmarshal([]byte(aggregateLines[0]), &aggregatePayload); err != nil {
		t.Fatalf("expected aggregate JSON, got %v", err)
	}
	if aggregatePayload["status"] != "ok" || aggregatePayload["latestBatchId"] == "" {
		t.Fatalf("unexpected aggregate payload: %+v", aggregatePayload)
	}
	if aggregatePayload["videoMetricCount"] == nil || aggregatePayload["commentFactCount"] == nil {
		t.Fatalf("expected aggregate counts, got %+v", aggregatePayload)
	}

	var summaryLines []string
	summaryLogf := func(format string, args ...any) {
		summaryLines = append(summaryLines, fmt.Sprintf(format, args...))
	}
	if err := run([]string{"show-aggregate-summary", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, summaryLogf); err != nil {
		t.Fatalf("expected show-aggregate-summary success, got %v", err)
	}
	if len(summaryLines) != 1 {
		t.Fatalf("expected one aggregate summary line, got %+v", summaryLines)
	}
	var summaryPayload map[string]any
	if err := json.Unmarshal([]byte(summaryLines[0]), &summaryPayload); err != nil {
		t.Fatalf("expected aggregate summary JSON, got %v", err)
	}
	if summaryPayload["status"] != "ok" || summaryPayload["videoMetricCount"] == nil {
		t.Fatalf("unexpected aggregate summary payload: %+v", summaryPayload)
	}
}

func TestRunShowHotVideosReturnsJSON(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
		JobsStatePath:       filepath.Join(tempDir, "jobs-runtime-state.json"),
	}
	payload := `{
  "objectId": "oid-206",
  "objectNonceId": "nid-206",
  "title": "show hot videos 标题",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"先说说怎么落地","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}
	if err := run([]string{"refresh-dashboard", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected refresh success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	if err := run([]string{"show-hot-videos", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected show-hot-videos success, got %v", err)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadMap); err != nil {
		t.Fatalf("expected JSON hot videos, got %v", err)
	}
	items, exists := payloadMap["items"]
	if !exists {
		t.Fatalf("expected hot videos items wrapper, got %+v", payloadMap)
	}
	if items != nil {
		payloadArray, ok := items.([]any)
		if !ok {
			t.Fatalf("expected hot videos items to be an array or null, got %+v", payloadMap)
		}
		if len(payloadArray) > 0 {
			first, ok := payloadArray[0].(map[string]any)
			if !ok || first["title"] == "" {
				t.Fatalf("expected hot videos payload, got %+v", payloadMap)
			}
		}
	}
	viewStatus, ok := payloadMap["viewStatus"].(map[string]any)
	if !ok || viewStatus["key"] != "videos" {
		t.Fatalf("expected hot videos view status, got %+v", payloadMap)
	}
}

func TestRunShowReportSummaryReturnsJSON(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
		JobsStatePath:       filepath.Join(tempDir, "jobs-runtime-state.json"),
	}
	payload := `{
  "objectId": "oid-207",
  "objectNonceId": "nid-207",
  "title": "show report summary 标题",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"先说说怎么落地","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected import success, got %v", err)
	}
	if err := run([]string{"refresh-dashboard", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected refresh success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	if err := run([]string{"show-report-summary", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected show-report-summary success, got %v", err)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadMap); err != nil {
		t.Fatalf("expected JSON report summary, got %v", err)
	}
	item, ok := payloadMap["item"].(map[string]any)
	if !ok || item["headline"] == "" {
		t.Fatalf("expected report summary headline, got %+v", payloadMap)
	}
	viewStatus, ok := payloadMap["viewStatus"].(map[string]any)
	if !ok || viewStatus["key"] != "reports" {
		t.Fatalf("expected report summary view status, got %+v", payloadMap)
	}
}

func TestRunSyncWXCommentExportMaterializesReadModel(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
		JobsStatePath:       filepath.Join(tempDir, "jobs-runtime-state.json"),
	}
	payload := `{
  "objectId": "oid-208",
  "objectNonceId": "nid-208",
  "title": "sync comment export 标题",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"先给一个能每天看的主视图","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	path := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	if err := run([]string{"sync-wx-comment-export", "--path", path, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected sync-wx-comment-export success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one sync line, got %+v", lines)
	}

	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadMap); err != nil {
		t.Fatalf("expected sync JSON, got %v", err)
	}
	if payloadMap["status"] != "ok" || payloadMap["source"] != "wx_comment_export" {
		t.Fatalf("expected sync result metadata, got %+v", payloadMap)
	}
	readModel, ok := payloadMap["readModel"].(map[string]any)
	if !ok || readModel["mode"] != "materialized" {
		t.Fatalf("expected materialized read model result, got %+v", payloadMap)
	}
}

func TestRunSyncSavedBindingsReplaysStoredBindingsAndRefreshesReadModel(t *testing.T) {
	tempDir := t.TempDir()
	serverCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalls++
		switch r.URL.Path {
		case "/api/channels/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"message": "success",
				"data": {
					"clients": 1,
					"ready_clients": 1,
					"feed_ready_clients": 1,
					"client_list": [{
						"page_path": "/channels/home",
						"href": "https://channels.weixin.qq.com/platform",
						"api_ready": true,
						"supports_feed": true
					}]
				}
			}`))
			return
		case "/ws/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":1}`))
			return
		case "/api/channels/contact/feed/list":
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("username") != "gh_team_operator" {
			t.Fatalf("unexpected username: %s", r.URL.Query().Get("username"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"object": [{
					"id": "feed-team-1",
					"objectNonceId": "nonce-team-1",
					"contact": { "nickname": "团队运营作者" },
					"objectDesc": {
						"description": "团队运营版今天先把一键主链路跑通",
						"media": [{ "commentCount": 73 }]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
		JobsStatePath:       filepath.Join(tempDir, "jobs-runtime-state.json"),
	}
	commentPayload := `{
  "objectId": "oid-209",
  "objectNonceId": "nid-209",
  "title": "一键同步保存 binding 测试标题",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"把每天固定动作尽量收成一条 operator 路径","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	commentPath := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(commentPayload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}

	if err := run([]string{"import-wx-comment-export", "--path", commentPath, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected comment export import success, got %v", err)
	}
	if err := run([]string{"import-wx-feed-list", "--base-url", server.URL, "--username", "gh_team_operator", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected feed list import success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	if err := run([]string{"sync-saved-bindings", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected sync-saved-bindings success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one sync line, got %+v", lines)
	}
	if serverCalls < 2 {
		t.Fatalf("expected saved wx_feed_list binding to call upstream again, got %d calls", serverCalls)
	}

	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadMap); err != nil {
		t.Fatalf("expected sync-saved-bindings JSON, got %v", err)
	}
	if payloadMap["status"] != "ok" {
		t.Fatalf("expected ok status, got %+v", payloadMap)
	}
	if payloadMap["attemptedSources"] != float64(2) || payloadMap["succeededSources"] != float64(2) {
		t.Fatalf("expected two attempted and succeeded sources, got %+v", payloadMap)
	}
	imports, ok := payloadMap["imports"].([]any)
	if !ok || len(imports) != 2 {
		t.Fatalf("expected two import payloads, got %+v", payloadMap["imports"])
	}
	readModel, ok := payloadMap["readModel"].(map[string]any)
	if !ok || readModel["mode"] != "materialized" {
		t.Fatalf("expected materialized read model result, got %+v", payloadMap)
	}
}

func TestRunSyncSavedBindingsRefreshesCommentExportWhenWXChannelBaseURLConfigured(t *testing.T) {
	tempDir := t.TempDir()
	feedCalls := 0
	commentCalls := 0
	refreshedPath := filepath.Join(tempDir, "comment-export-refreshed.json")
	if err := os.WriteFile(refreshedPath, []byte(`{
  "objectId": "oid-ops-refresh-301",
  "objectNonceId": "nid-ops-refresh-301",
  "title": "jobs refreshed comment export",
  "author": "Insight Test",
  "commentInfo": [
    {"commentId":"c1","content":"新评论一","nickname":"用户A","username":"user_a","createtime":"1719914400","likeCount":3,"expandCommentCount":0,"levelTwoComment":[]},
    {"commentId":"c2","content":"新评论二","nickname":"用户B","username":"user_b","createtime":"1719918000","likeCount":2,"expandCommentCount":0,"levelTwoComment":[]}
  ],
  "countInfo": { "commentCount": 2 },
  "originalCommentCount": 2,
  "savedAt": "2026-07-05T10:05:00+08:00",
  "source": "finderGetCommentList"
}`), 0o644); err != nil {
		t.Fatalf("write refreshed export file failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":1}`))
		case "/api/channels/feed/comment/export":
			commentCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"message":"success","data":{"object_id":"oid-ops-refresh-301","saved_path":%q,"relative_path":"comment_data/2026-07-05/comment-export-refreshed.json","title":"jobs refreshed comment export","author":"Insight Test","source":"finderGetCommentList"}}`, refreshedPath)))
		default:
			feedCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"message": "success",
				"data": {
					"object": [{
						"id": "feed-sync-refresh-1",
						"objectNonceId": "nonce-sync-refresh-1",
						"contact": { "nickname": "Ops Refresh Feed" },
						"objectDesc": {
							"description": "jobs refresh path",
							"media": [{ "commentCount": 34 }]
						}
					}]
				}
			}`))
		}
	}))
	defer server.Close()

	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		WXChannelBaseURL:    server.URL,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
		JobsStatePath:       filepath.Join(tempDir, "jobs-runtime-state.json"),
	}
	commentPath := filepath.Join(tempDir, "comment-export-stale.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-ops-refresh-301",
  "objectNonceId": "nid-ops-refresh-301",
  "title": "jobs stale comment export",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"旧评论","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write stale export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", commentPath, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected comment export import success, got %v", err)
	}
	if err := run([]string{"import-wx-feed-list", "--base-url", server.URL, "--username", "gh_refresh_jobs", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected feed list import success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	if err := run([]string{"sync-saved-bindings", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected sync-saved-bindings success, got %v", err)
	}
	if commentCalls == 0 {
		t.Fatalf("expected comment export refresh endpoint to be called")
	}
	if feedCalls < 2 {
		t.Fatalf("expected feed list upstream to be called at least twice, got %d", feedCalls)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one sync line, got %+v", lines)
	}
}

func TestRunSyncSavedBindingsShortCircuitsCommentExportWhenNoReadyClient(t *testing.T) {
	tempDir := t.TempDir()
	commentCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"clients":1,"ready_clients":0,"feed_ready_clients":0,"client_list":[{"page_path":"/web/pages/video/detail","href":"https://channels.weixin.qq.com/web/pages/video/detail","api_ready":false,"supports_feed":false}]}}`))
		case "/api/channels/feed/comment/export":
			commentCalls++
			t.Fatalf("comment export endpoint should not be called when no ready client is reported")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		WXChannelBaseURL:    server.URL,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
		JobsStatePath:       filepath.Join(tempDir, "jobs-runtime-state.json"),
	}
	commentPath := filepath.Join(tempDir, "comment-export-stale.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-jobs-blocked-1",
  "objectNonceId": "nid-jobs-blocked-1",
  "title": "jobs stale comment export",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"需要先等页面 ready","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write stale export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", commentPath, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected comment export import success, got %v", err)
	}

	err := run([]string{"sync-saved-bindings", "--sources", "wx_comment_export", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {})
	if err == nil {
		t.Fatal("expected sync-saved-bindings failure when comment export has no ready client")
	}
	if !strings.Contains(err.Error(), "comment export has no ready page client") {
		t.Fatalf("expected ready-client guidance error, got %v", err)
	}
	if commentCalls != 0 {
		t.Fatalf("expected comment export endpoint to be skipped, got %d calls", commentCalls)
	}
}

func TestRunOperatorCycleReturnsOpsBrief(t *testing.T) {
	tempDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws/health" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":1}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"object": [{
					"id": "feed-ops-1",
					"objectNonceId": "nonce-ops-1",
					"contact": { "nickname": "运营观察账号" },
					"objectDesc": {
						"description": "今天先看热视频和热话题有没有一起抬头",
						"media": [{ "commentCount": 66 }]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
	}
	commentPayload := `{
  "objectId": "oid-210",
  "objectNonceId": "nid-210",
  "title": "operator cycle 评论标题",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"团队最需要每天能跑通的一条运营主链路","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	commentPath := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(commentPayload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}

	if err := run([]string{"import-wx-comment-export", "--path", commentPath, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected comment import success, got %v", err)
	}
	if err := run([]string{"import-wx-feed-list", "--base-url", server.URL, "--username", "gh_ops_runner", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected feed list import success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	if err := run([]string{"run-operator-cycle", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected run-operator-cycle success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one operator cycle line, got %+v", lines)
	}

	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadMap); err != nil {
		t.Fatalf("expected operator cycle JSON, got %v", err)
	}
	if payloadMap["status"] != "ok" {
		t.Fatalf("expected ok operator cycle status, got %+v", payloadMap)
	}
	syncResult, ok := payloadMap["sync"].(map[string]any)
	if !ok || syncResult["status"] != "ok" {
		t.Fatalf("expected sync result in operator cycle, got %+v", payloadMap)
	}
	brief, ok := payloadMap["brief"].(map[string]any)
	if !ok {
		t.Fatalf("expected brief payload, got %+v", payloadMap)
	}
	runtimeBrief, ok := brief["runtime"].(map[string]any)
	if !ok || runtimeBrief["phase"] != "team-edition-phase-a" {
		t.Fatalf("expected runtime phase in brief, got %+v", brief)
	}
	readModelBrief, ok := brief["readModel"].(map[string]any)
	if !ok || readModelBrief["mode"] != "materialized" {
		t.Fatalf("expected materialized read model in brief, got %+v", brief)
	}
	signals, ok := brief["signals"].(map[string]any)
	if !ok {
		t.Fatalf("expected signals payload, got %+v", brief)
	}
	topVideo, ok := signals["topVideo"].(map[string]any)
	if !ok || topVideo["title"] == "" {
		t.Fatalf("expected top video in operator brief, got %+v", signals)
	}
	report, ok := signals["report"].(map[string]any)
	if !ok || report["headline"] == "" {
		t.Fatalf("expected report summary in operator brief, got %+v", signals)
	}
}

func TestRunShowOperatorRunsReturnsRecordedCycle(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
	}
	commentPayload := `{
  "objectId": "oid-211",
  "objectNonceId": "nid-211",
  "title": "show operator runs 评论标题",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"需要把 operator cycle 留成可追踪批次历史","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	commentPath := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(commentPayload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", commentPath, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected comment import success, got %v", err)
	}
	if err := run([]string{"run-operator-cycle", "--sources", "wx_comment_export", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected operator cycle success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	if err := run([]string{"show-operator-runs", "--hub-user-id", "hub:user:alice"}, cfg, logf); err != nil {
		t.Fatalf("expected show-operator-runs success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one operator runs line, got %+v", lines)
	}
	var payloadArray []map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadArray); err != nil {
		t.Fatalf("expected operator runs JSON, got %v", err)
	}
	if len(payloadArray) == 0 {
		t.Fatalf("expected at least one operator run, got %+v", payloadArray)
	}
	if payloadArray[0]["status"] == "" || payloadArray[0]["reportHeadline"] == "" {
		t.Fatalf("expected operator run summary fields, got %+v", payloadArray[0])
	}
	outcomes, ok := payloadArray[0]["outcomes"].([]any)
	if !ok || len(outcomes) == 0 {
		t.Fatalf("expected operator run outcomes in jobs output, got %+v", payloadArray[0])
	}
}

func TestRunOperatorCycleStrictReturnsErrorAndStillRecordsFailure(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
	}
	commentPayload := `{
  "objectId": "oid-212",
  "objectNonceId": "nid-212",
  "title": "strict operator cycle 评论标题",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"失败也要落批次历史","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	commentPath := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(commentPayload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", commentPath, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected comment import success, got %v", err)
	}
	if err := os.Remove(commentPath); err != nil {
		t.Fatalf("remove export file failed: %v", err)
	}

	var cycleLines []string
	logf := func(format string, args ...any) { cycleLines = append(cycleLines, fmt.Sprintf(format, args...)) }
	err := run([]string{"run-operator-cycle", "--sources", "wx_comment_export", "--strict", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf)
	if err == nil {
		t.Fatal("expected strict operator cycle error, got nil")
	}
	var silentErr silentExitError
	if !errors.As(err, &silentErr) {
		t.Fatalf("expected silentExitError, got %T", err)
	}
	if len(cycleLines) != 1 {
		t.Fatalf("expected one operator cycle output line, got %+v", cycleLines)
	}
	var cyclePayload map[string]any
	if err := json.Unmarshal([]byte(cycleLines[0]), &cyclePayload); err != nil {
		t.Fatalf("expected operator cycle JSON, got %v", err)
	}
	if cyclePayload["status"] != "failed" {
		t.Fatalf("expected failed operator cycle payload, got %+v", cyclePayload)
	}

	var lines []string
	showLogf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	if err := run([]string{"show-operator-runs", "--hub-user-id", "hub:user:alice"}, cfg, showLogf); err != nil {
		t.Fatalf("expected show-operator-runs success, got %v", err)
	}
	var payloadArray []map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payloadArray); err != nil {
		t.Fatalf("expected operator runs JSON, got %v", err)
	}
	if len(payloadArray) == 0 || payloadArray[0]["status"] != "failed" {
		t.Fatalf("expected failed operator run to be recorded, got %+v", payloadArray)
	}
	if payloadArray[0]["message"] == "" {
		t.Fatalf("expected failed operator run message, got %+v", payloadArray[0])
	}
}

func TestRunOperatorCycleWithoutStrictReturnsPartialWhenDefaultSourceIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
	}
	commentPayload := `{
  "objectId": "oid-213",
  "objectNonceId": "nid-213",
  "title": "partial operator cycle 评论标题",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"只有评论来源时默认应提示部分完成","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	commentPath := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(commentPayload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", commentPath, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected comment import success, got %v", err)
	}

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	if err := run([]string{"run-operator-cycle", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected non-strict operator cycle success, got %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one operator cycle output line, got %+v", lines)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("expected operator cycle JSON, got %v", err)
	}
	if payload["status"] != "partial" {
		t.Fatalf("expected partial operator cycle payload, got %+v", payload)
	}
	syncResult, ok := payload["sync"].(map[string]any)
	if !ok || syncResult["status"] != "partial" || syncResult["finalStatus"] != "partial" {
		t.Fatalf("expected partial sync result, got %+v", payload)
	}
	skipped, ok := syncResult["skipped"].([]any)
	if !ok || len(skipped) == 0 {
		t.Fatalf("expected skipped sources for missing default source, got %+v", syncResult)
	}
}

func TestRunOperatorLoopStopsAfterIterationsAndLogsEachRun(t *testing.T) {
	tempDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws/health" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":1}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"object": [{
					"id": "feed-loop-1",
					"objectNonceId": "nonce-loop-1",
					"contact": { "nickname": "Loop 账号" },
					"objectDesc": {
						"description": "按轮次跑 operator loop",
						"media": [{ "commentCount": 88 }]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
		JobsStatePath:       filepath.Join(tempDir, "jobs-runtime-state.json"),
	}
	commentPayload := `{
  "objectId": "oid-214",
  "objectNonceId": "nid-214",
  "title": "operator loop 评论标题",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"loop 需要多轮跑并逐轮输出","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	commentPath := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(commentPayload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", commentPath, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected comment import success, got %v", err)
	}
	if err := run([]string{"import-wx-feed-list", "--base-url", server.URL, "--username", "gh_ops_loop", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected feed list import success, got %v", err)
	}

	originalSleep := operatorLoopSleep
	operatorLoopSleep = func(context.Context, time.Duration) error { return nil }
	defer func() { operatorLoopSleep = originalSleep }()

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	if err := run([]string{"run-operator-loop", "--interval", "1ms", "--iterations", "2", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf); err != nil {
		t.Fatalf("expected run-operator-loop success, got %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected two operator loop lines, got %+v", lines)
	}

	for index, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("expected operator loop JSON on line %d, got %v", index, err)
		}
		if payload["status"] != "ok" {
			t.Fatalf("expected ok operator loop payload, got %+v", payload)
		}
		loopPayload, ok := payload["loop"].(map[string]any)
		if !ok {
			t.Fatalf("expected loop payload, got %+v", payload)
		}
		if loopPayload["iteration"] != float64(index+1) {
			t.Fatalf("expected iteration %d, got %+v", index+1, loopPayload)
		}
		if loopPayload["iterations"] != float64(2) {
			t.Fatalf("expected total iterations in loop payload, got %+v", loopPayload)
		}
	}

	jobsState, err := runtimecfg.LoadJobsRuntimeState(cfg.JobsStatePath)
	if err != nil {
		t.Fatalf("expected jobs runtime state load success, got %v", err)
	}
	if jobsState.LastMode != "operator-loop" || jobsState.LastStatus != "ok" {
		t.Fatalf("expected operator loop runtime state, got %+v", jobsState)
	}
	if jobsState.LastIteration != 2 || jobsState.LastIterations != 2 {
		t.Fatalf("expected operator loop iteration state, got %+v", jobsState)
	}
	if jobsState.LastRequestedCount != 2 {
		t.Fatalf("expected default operator loop to request two primary sources, got %+v", jobsState)
	}
	if len(jobsState.LastRequestedSources) != 2 || jobsState.LastRequestedSources[0] != "wx_comment_export" || jobsState.LastRequestedSources[1] != "wx_feed_list" {
		t.Fatalf("expected default operator loop requested sources, got %+v", jobsState)
	}
}

func TestRunOperatorLoopStrictStopsOnFirstFailure(t *testing.T) {
	tempDir := t.TempDir()
	cfg := runtimecfg.Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "dashboard-state.json")},
		RankingStore:        runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "rankings-state.json")},
		CommentInsightStore: runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "comment-insights-state.json")},
		ReportSummaryStore:  runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "report-summary-state.json")},
		WorkspaceStore:      runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "workspace-state.json")},
		IngestStore:         runtimecfg.StoreConfig{Mode: runtimecfg.StoreModeFile, Path: filepath.Join(tempDir, "ingest-state.json")},
		JobsStatePath:       filepath.Join(tempDir, "jobs-runtime-state.json"),
	}
	commentPayload := `{
  "objectId": "oid-215",
  "objectNonceId": "nid-215",
  "title": "strict operator loop 评论标题",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"strict loop 失败后要立刻停","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":12,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`
	commentPath := filepath.Join(tempDir, "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(commentPayload), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := run([]string{"import-wx-comment-export", "--path", commentPath, "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("expected comment import success, got %v", err)
	}
	if err := os.Remove(commentPath); err != nil {
		t.Fatalf("remove export file failed: %v", err)
	}

	originalSleep := operatorLoopSleep
	operatorLoopSleep = func(context.Context, time.Duration) error { return nil }
	defer func() { operatorLoopSleep = originalSleep }()

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	err := run([]string{"run-operator-loop", "--interval", "1ms", "--iterations", "3", "--sources", "wx_comment_export", "--strict", "--hub-user-id", "hub:user:alice", "--device-count", "2"}, cfg, logf)
	if err == nil {
		t.Fatal("expected strict operator loop error, got nil")
	}
	var silentErr silentExitError
	if !errors.As(err, &silentErr) {
		t.Fatalf("expected silentExitError, got %T", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected strict operator loop to stop after first line, got %+v", lines)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("expected operator loop JSON, got %v", err)
	}
	if payload["status"] != "failed" {
		t.Fatalf("expected failed operator loop payload, got %+v", payload)
	}
	loopPayload, ok := payload["loop"].(map[string]any)
	if !ok || loopPayload["iteration"] != float64(1) {
		t.Fatalf("expected first iteration loop payload, got %+v", payload)
	}

	jobsState, loadErr := runtimecfg.LoadJobsRuntimeState(cfg.JobsStatePath)
	if loadErr != nil {
		t.Fatalf("expected jobs runtime state load success, got %v", loadErr)
	}
	if jobsState.LastStatus != "failed" || jobsState.LastError == "" {
		t.Fatalf("expected failed operator loop runtime state, got %+v", jobsState)
	}
	if jobsState.LastRequestedCount != 1 {
		t.Fatalf("expected strict operator loop to persist one requested source, got %+v", jobsState)
	}
	if len(jobsState.LastRequestedSources) != 1 || jobsState.LastRequestedSources[0] != "wx_comment_export" {
		t.Fatalf("expected strict operator loop requested source list, got %+v", jobsState)
	}
}
