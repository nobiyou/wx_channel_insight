package insightfile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
)

func TestRankingRepositoryPersistsWorkspaceScopedHotRankings(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "rankings.json")
	repo, err := insightfile.NewRankingRepository(
		storePath,
	)
	if err != nil {
		t.Fatalf("new ranking repository failed: %v", err)
	}

	if err := repo.ReplaceHotVideos(context.Background(), "ws-alpha", []insight.HotVideo{{
		ID:          "video-alpha",
		Title:       "alpha",
		ChannelName: "Insight Test",
	}}); err != nil {
		t.Fatalf("replace hot videos failed: %v", err)
	}
	if err := repo.ReplaceHotVideoRankings(context.Background(), "ws-alpha", insight.HotVideoRankings{
		insight.HotVideoRankingHot:   {{ID: "video-alpha", Title: "alpha"}},
		insight.HotVideoRankingLikes: {{ID: "video-likes", Title: "likes"}},
	}); err != nil {
		t.Fatalf("replace hot video rankings failed: %v", err)
	}
	if err := repo.ReplaceHotTopics(context.Background(), "ws-alpha", []insight.HotTopic{{
		ID:      "topic-alpha",
		Keyword: "alpha",
		Cluster: "Insight Test",
	}}); err != nil {
		t.Fatalf("replace hot topics failed: %v", err)
	}

	reloaded, err := insightfile.NewRankingRepository(
		storePath,
	)
	if err != nil {
		t.Fatalf("reload ranking repository failed: %v", err)
	}

	videos, found, err := reloaded.GetHotVideos(context.Background(), "ws-alpha")
	if err != nil {
		t.Fatalf("get hot videos failed: %v", err)
	}
	if !found || len(videos) == 0 || videos[0].Title != "alpha" {
		t.Fatalf("expected persisted hot videos, got found=%t items=%+v", found, videos)
	}
	rankings, found, err := reloaded.GetHotVideoRankings(context.Background(), "ws-alpha")
	if err != nil {
		t.Fatalf("get hot video rankings failed: %v", err)
	}
	if !found || len(rankings[insight.HotVideoRankingLikes]) != 1 || rankings[insight.HotVideoRankingLikes][0].Title != "likes" {
		t.Fatalf("expected persisted hot video rankings, got found=%t rankings=%+v", found, rankings)
	}

	topics, found, err := reloaded.GetHotTopics(context.Background(), "ws-alpha")
	if err != nil {
		t.Fatalf("get hot topics failed: %v", err)
	}
	if !found || len(topics) == 0 || topics[0].Keyword != "alpha" {
		t.Fatalf("expected persisted hot topics, got found=%t items=%+v", found, topics)
	}

	meta := insight.ReadModelArtifactMeta{
		LatestBatchID:  "batch-alpha",
		MaterializedAt: time.Date(2026, 7, 3, 10, 14, 0, 0, time.FixedZone("CST", 8*3600)),
	}
	if err := reloaded.ReplaceHotVideoMeta(context.Background(), "ws-alpha", meta); err != nil {
		t.Fatalf("replace hot video meta failed: %v", err)
	}
	if err := reloaded.ReplaceHotTopicMeta(context.Background(), "ws-alpha", meta); err != nil {
		t.Fatalf("replace hot topic meta failed: %v", err)
	}

	verified, err := insightfile.NewRankingRepository(
		storePath,
	)
	if err != nil {
		t.Fatalf("verify ranking repository reload failed: %v", err)
	}
	videoMeta, found, err := verified.GetHotVideoMeta(context.Background(), "ws-alpha")
	if err != nil {
		t.Fatalf("get hot video meta failed: %v", err)
	}
	if !found || videoMeta.LatestBatchID != "batch-alpha" {
		t.Fatalf("expected persisted hot video meta, got found=%t meta=%+v", found, videoMeta)
	}
}

func TestRankingRepositoryRefreshesAcrossLiveInstances(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "rankings.json")
	writer, err := insightfile.NewRankingRepository(
		storePath,
	)
	if err != nil {
		t.Fatalf("new writer repository failed: %v", err)
	}
	reader, err := insightfile.NewRankingRepository(
		storePath,
	)
	if err != nil {
		t.Fatalf("new reader repository failed: %v", err)
	}

	if err := writer.ReplaceHotVideos(context.Background(), "ws-beta", []insight.HotVideo{{
		ID:          "video-beta",
		Title:       "beta",
		ChannelName: "Insight Test",
	}}); err != nil {
		t.Fatalf("replace hot videos failed: %v", err)
	}
	if err := writer.ReplaceHotTopics(context.Background(), "ws-beta", []insight.HotTopic{{
		ID:      "topic-beta",
		Keyword: "beta",
		Cluster: "Insight Test",
	}}); err != nil {
		t.Fatalf("replace hot topics failed: %v", err)
	}

	videos, found, err := reader.GetHotVideos(context.Background(), "ws-beta")
	if err != nil {
		t.Fatalf("get hot videos failed: %v", err)
	}
	if !found || len(videos) == 0 || videos[0].Title != "beta" {
		t.Fatalf("expected live hot videos refresh, got found=%t items=%+v", found, videos)
	}

	topics, found, err := reader.GetHotTopics(context.Background(), "ws-beta")
	if err != nil {
		t.Fatalf("get hot topics failed: %v", err)
	}
	if !found || len(topics) == 0 || topics[0].Keyword != "beta" {
		t.Fatalf("expected live hot topics refresh, got found=%t items=%+v", found, topics)
	}
}

func TestRankingRepositoryDoesNotSeedUnknownWorkspace(t *testing.T) {
	repo, err := insightfile.NewRankingRepository(filepath.Join(t.TempDir(), "rankings.json"))
	if err != nil {
		t.Fatalf("new ranking repository failed: %v", err)
	}

	videos, found, err := repo.GetHotVideos(context.Background(), "ws-without-events")
	if err != nil {
		t.Fatalf("get hot videos failed: %v", err)
	}
	if found || len(videos) != 0 {
		t.Fatalf("expected no seeded hot videos, got found=%t items=%+v", found, videos)
	}

	topics, found, err := repo.GetHotTopics(context.Background(), "ws-without-events")
	if err != nil {
		t.Fatalf("get hot topics failed: %v", err)
	}
	if found || len(topics) != 0 {
		t.Fatalf("expected no seeded hot topics, got found=%t items=%+v", found, topics)
	}
}
