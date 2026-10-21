package insightfile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
)

func TestCommentInsightRepositoryPersistsByWorkspace(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "comment-insights.json")
	repo, err := insightfile.NewCommentInsightRepository(storePath, []insight.CommentInsight{{
		ID:             "seed-ci",
		Theme:          "seed",
		Representative: "seed",
	}})
	if err != nil {
		t.Fatalf("new repository failed: %v", err)
	}

	if err := repo.ReplaceCommentInsights(context.Background(), "ws-alpha", []insight.CommentInsight{{
		ID:               "ci-alpha",
		Theme:            "alpha",
		Representative:   "alpha rep",
		RelatedKeywords:  []string{"alpha"},
		ActionSuggestion: "follow up",
	}}); err != nil {
		t.Fatalf("replace comment insights failed: %v", err)
	}

	reloaded, err := insightfile.NewCommentInsightRepository(storePath, []insight.CommentInsight{{
		ID:             "seed-ci",
		Theme:          "seed",
		Representative: "seed",
	}})
	if err != nil {
		t.Fatalf("reload repository failed: %v", err)
	}

	items, found, err := reloaded.GetCommentInsights(context.Background(), "ws-alpha")
	if err != nil {
		t.Fatalf("get comment insights failed: %v", err)
	}
	if !found || len(items) == 0 || items[0].Theme != "alpha" {
		t.Fatalf("expected persisted comment insights, got found=%t items=%+v", found, items)
	}

	meta := insight.ReadModelArtifactMeta{
		LatestBatchID:  "batch-alpha",
		MaterializedAt: time.Date(2026, 7, 3, 10, 14, 0, 0, time.FixedZone("CST", 8*3600)),
	}
	if err := reloaded.ReplaceCommentInsightMeta(context.Background(), "ws-alpha", meta); err != nil {
		t.Fatalf("replace comment insight meta failed: %v", err)
	}
	verified, err := insightfile.NewCommentInsightRepository(storePath, []insight.CommentInsight{{
		ID:             "seed-ci",
		Theme:          "seed",
		Representative: "seed",
	}})
	if err != nil {
		t.Fatalf("verify repository reload failed: %v", err)
	}
	gotMeta, found, err := verified.GetMeta(context.Background(), "ws-alpha")
	if err != nil {
		t.Fatalf("get comment insight meta failed: %v", err)
	}
	if !found || gotMeta.LatestBatchID != "batch-alpha" {
		t.Fatalf("expected persisted comment meta, got found=%t meta=%+v", found, gotMeta)
	}
}

func TestCommentInsightRepositoryRefreshesAcrossLiveInstances(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "comment-insights.json")
	writer, err := insightfile.NewCommentInsightRepository(storePath, []insight.CommentInsight{{
		ID:             "seed-ci",
		Theme:          "seed",
		Representative: "seed",
	}})
	if err != nil {
		t.Fatalf("new writer repository failed: %v", err)
	}
	reader, err := insightfile.NewCommentInsightRepository(storePath, []insight.CommentInsight{{
		ID:             "seed-ci",
		Theme:          "seed",
		Representative: "seed",
	}})
	if err != nil {
		t.Fatalf("new reader repository failed: %v", err)
	}

	if err := writer.ReplaceCommentInsights(context.Background(), "ws-beta", []insight.CommentInsight{{
		ID:               "ci-beta",
		Theme:            "beta",
		Representative:   "beta rep",
		RelatedKeywords:  []string{"beta"},
		ActionSuggestion: "follow up beta",
	}}); err != nil {
		t.Fatalf("replace comment insights failed: %v", err)
	}

	items, found, err := reader.GetCommentInsights(context.Background(), "ws-beta")
	if err != nil {
		t.Fatalf("get comment insights failed: %v", err)
	}
	if !found || len(items) == 0 || items[0].Theme != "beta" {
		t.Fatalf("expected live comment insights refresh, got found=%t items=%+v", found, items)
	}
}
