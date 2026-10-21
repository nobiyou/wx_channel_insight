package ingest_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
)

func TestAssessSourceBindingResolvesLatestCommentExportDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "comment_data")
	oldPath := writeWXCommentExportFixture(t, filepath.Join(root, "2026-07-03", "旧样本.json"), "oid-100", "2026-07-03T19:08:38+08:00")
	newPath := writeWXCommentExportFixture(t, filepath.Join(root, "2026-07-04", "新样本.json"), "oid-200", "2026-07-04T23:43:21+08:00")
	_ = oldPath

	assessment := ingest.AssessSourceBinding(ingest.RunSourceWXCommentExport, ingest.SourceBinding{
		Source: ingest.RunSourceWXCommentExport,
		Path:   root,
	})
	if !assessment.Runnable {
		t.Fatalf("expected directory binding runnable, got %+v", assessment)
	}
	if assessment.ResolvedPath != newPath {
		t.Fatalf("expected latest comment export path %s, got %+v", newPath, assessment)
	}
	if assessment.PersistPath != root {
		t.Fatalf("expected directory binding to persist root %s, got %+v", root, assessment)
	}
}

func TestAssessSourceBindingBlocksProofPlaceholderCommentExport(t *testing.T) {
	path := writeWXCommentExportFixture(
		t,
		filepath.Join(t.TempDir(), ".tmp-codex", "proof-inputs", "wx-channel-insight-comment-export-hub_user_team-runtime-local.json"),
		"team-runtime-proof-001",
		"2026-07-03T19:08:38+08:00",
	)

	assessment := ingest.AssessSourceBinding(ingest.RunSourceWXCommentExport, ingest.SourceBinding{
		Source: ingest.RunSourceWXCommentExport,
		Path:   path,
	})
	if assessment.Runnable {
		t.Fatalf("expected proof placeholder comment export to be blocked, got %+v", assessment)
	}
	if !strings.Contains(assessment.Reason, "team runtime proof 占位评论导出") {
		t.Fatalf("expected proof placeholder reason, got %+v", assessment)
	}
}

func TestServiceGetSummaryExplainsHowToReplaceProofPlaceholderCommentExport(t *testing.T) {
	repo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 7, 15, 30, 0, 0, time.FixedZone("CST", 8*3600))
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	path := writeWXCommentExportFixture(
		t,
		filepath.Join(t.TempDir(), ".tmp-codex", "proof-inputs", "wx-channel-insight-comment-export-hub_user_team-runtime-local.json"),
		"team-runtime-proof-001",
		"2026-07-03T19:08:38+08:00",
	)
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        path,
		DeviceCount: 2,
		UpdatedAt:   now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}

	summary, err := service.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("get summary failed: %v", err)
	}
	if summary.OperatorReadiness.BlockedSources != 1 || len(summary.OperatorReadiness.Blocked) != 1 || summary.OperatorReadiness.MissingSources != 1 {
		t.Fatalf("expected one blocked placeholder source and one missing source, got %+v", summary.OperatorReadiness)
	}
	if !strings.Contains(summary.OperatorReadiness.Blocked[0].Summary, "team runtime proof 占位评论导出") {
		t.Fatalf("expected blocked summary to call out proof placeholder, got %+v", summary.OperatorReadiness.Blocked[0])
	}
	if !strings.Contains(summary.OperatorReadiness.Blocked[0].Action, "重新导出评论 JSON") {
		t.Fatalf("expected blocked action to ask for a real comment export, got %+v", summary.OperatorReadiness.Blocked[0])
	}
}

func TestServiceImportWXCommentExportSelfHealsToNewerSameObjectFile(t *testing.T) {
	repo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 4, 23, 45, 0, 0, time.FixedZone("CST", 8*3600))
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	root := filepath.Join(t.TempDir(), "comment_data")
	oldPath := writeWXCommentExportFixture(t, filepath.Join(root, "2026-07-03", "同一视频旧导出.json"), "oid-same", "2026-07-03T19:08:38+08:00")
	newPath := writeWXCommentExportFixture(t, filepath.Join(root, "2026-07-04", "同一视频新导出.json"), "oid-same", "2026-07-04T23:43:21+08:00")
	_ = writeWXCommentExportFixture(t, filepath.Join(root, "2026-07-04", "别的视频更新导出.json"), "oid-other", "2026-07-04T23:50:21+08:00")

	summary, err := service.ImportWXCommentExport(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, oldPath)
	if err != nil {
		t.Fatalf("expected import success, got %v", err)
	}
	if len(summary.SourceStatuses) != 1 {
		t.Fatalf("expected one source status, got %+v", summary.SourceStatuses)
	}
	if !summary.SourceStatuses[0].LatestEventAt.Equal(time.Date(2026, 7, 4, 23, 43, 21, 0, time.FixedZone("CST", 8*3600))) {
		t.Fatalf("expected latest event time from newer same-object export, got %+v", summary.SourceStatuses[0])
	}

	bindings, err := service.ListBindings(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected one saved binding, got %+v", bindings)
	}
	if bindings[0].Path != newPath {
		t.Fatalf("expected binding to self-heal to newer same-object file %s, got %+v", newPath, bindings[0])
	}
}

func TestServiceRetryLastFailedImportRecoversFromMissingCommentExportFileViaCommentDataRoot(t *testing.T) {
	repo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600))
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	root := filepath.Join(t.TempDir(), "comment_data")
	missingPath := filepath.Join(root, "2026-07-03", "已经失效的旧导出.json")
	latestPath := writeWXCommentExportFixture(t, filepath.Join(root, "2026-07-05", "最新评论导出.json"), "oid-new", "2026-07-05T09:28:00+08:00")

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        missingPath,
		DeviceCount: 2,
		UpdatedAt:   now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}
	if err := repo.AppendRun(context.Background(), "hub:user:alice", ingest.Run{
		ID:         "wx_comment_export-failed",
		Source:     ingest.RunSourceWXCommentExport,
		Status:     "failed",
		StartedAt:  now.Add(-5 * time.Minute),
		FinishedAt: now.Add(-4 * time.Minute),
		Message:    "open path failed",
	}); err != nil {
		t.Fatalf("append failed run failed: %v", err)
	}

	summary, err := service.RetryLastFailedImport(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	}, ingest.RunSourceWXCommentExport)
	if err != nil {
		t.Fatalf("expected retry success after discovering latest comment export, got %v", err)
	}
	if len(summary.BindingStates) != 1 {
		t.Fatalf("expected one binding state, got %+v", summary.BindingStates)
	}
	if summary.BindingStates[0].Path != root {
		t.Fatalf("expected binding state to expose healed comment_data root %s, got %+v", root, summary.BindingStates[0])
	}

	bindings, err := service.ListBindings(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected one saved binding, got %+v", bindings)
	}
	if bindings[0].Path != root {
		t.Fatalf("expected saved binding to persist comment_data root %s, got %+v", root, bindings[0])
	}

	retrySummary, err := service.ImportWXCommentExport(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, bindings[0].Path)
	if err != nil {
		t.Fatalf("expected follow-up import from healed root path to succeed, got %v", err)
	}
	if len(retrySummary.SourceStatuses) != 1 || retrySummary.SourceStatuses[0].LatestEventAt.IsZero() {
		t.Fatalf("expected usable source status after healed import, got %+v", retrySummary.SourceStatuses)
	}
	_ = latestPath
}

func TestServiceListBindingStatesExposesHealedCommentDataRoot(t *testing.T) {
	repo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 5, 9, 40, 0, 0, time.FixedZone("CST", 8*3600))
	service := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	root := filepath.Join(t.TempDir(), "comment_data")
	missingPath := filepath.Join(root, "2026-07-03", "已经失效的旧导出.json")
	_ = writeWXCommentExportFixture(t, filepath.Join(root, "2026-07-05", "最新评论导出.json"), "oid-new", "2026-07-05T09:38:00+08:00")

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        missingPath,
		DeviceCount: 2,
		UpdatedAt:   now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}

	states, err := service.ListBindingStates(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list binding states failed: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected one binding state, got %+v", states)
	}
	if states[0].Path != root {
		t.Fatalf("expected binding state to expose healed comment_data root %s, got %+v", root, states[0])
	}
	if !states[0].Runnable {
		t.Fatalf("expected healed comment_data root binding runnable, got %+v", states[0])
	}
}

func writeWXCommentExportFixture(t *testing.T, path, objectID, savedAt string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir failed: %v", err)
	}
	payload := `{
  "objectId": "` + objectID + `",
  "objectNonceId": "nid-` + objectID + `",
  "title": "评论导出样本 ` + objectID + `",
  "author": "Insight Runtime",
  "commentInfo": [
    {
      "commentId": "c1",
      "content": "这条评论导出用于团队运营版测试。",
      "nickname": "用户A",
      "username": "user_a",
      "createtime": "1719828000",
      "likeCount": 12,
      "expandCommentCount": 0,
      "levelTwoComment": []
    }
  ],
  "countInfo": {
    "commentCount": 1
  },
  "originalCommentCount": 1,
  "savedAt": "` + savedAt + `",
  "source": "finderGetCommentList"
}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}
	return path
}
