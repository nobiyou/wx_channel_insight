package insight_test

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/rediscache"
)

func TestReadCacheRoundTripsDashboardAndReadModels(t *testing.T) {
	client, err := rediscache.NewClientWithKV("127.0.0.1:6379", "", 0, &stubKV{})
	if err != nil {
		t.Fatalf("build redis stub client failed: %v", err)
	}
	cache := insight.NewReadCache(client, time.Minute)

	dashboard := insight.Dashboard{
		ReadModel: insight.ReadModelState{
			Mode:          "materialized",
			LatestBatchID: "batch-001",
		},
		HotVideos: []insight.HotVideo{{ID: "video-1", Title: "视频 1"}},
	}
	if err := cache.SaveDashboard(context.Background(), "ws-1", dashboard); err != nil {
		t.Fatalf("save dashboard failed: %v", err)
	}
	gotDashboard, found, err := cache.LoadDashboard(context.Background(), "ws-1")
	if err != nil || !found {
		t.Fatalf("load dashboard failed: found=%v err=%v", found, err)
	}
	if gotDashboard.ReadModel.LatestBatchID != "batch-001" {
		t.Fatalf("unexpected dashboard payload: %+v", gotDashboard)
	}

	videos := []insight.HotVideo{{ID: "video-1", Title: "视频 1"}}
	if err := cache.SaveHotVideos(context.Background(), "ws-1", videos); err != nil {
		t.Fatalf("save hot videos failed: %v", err)
	}
	gotVideos, found, err := cache.LoadHotVideos(context.Background(), "ws-1")
	if err != nil || !found || len(gotVideos) != 1 {
		t.Fatalf("load hot videos failed: found=%v err=%v items=%+v", found, err, gotVideos)
	}

	topics := []insight.HotTopic{{ID: "topic-1", Keyword: "团队运营版"}}
	if err := cache.SaveHotTopics(context.Background(), "ws-1", topics); err != nil {
		t.Fatalf("save hot topics failed: %v", err)
	}
	gotTopics, found, err := cache.LoadHotTopics(context.Background(), "ws-1")
	if err != nil || !found || len(gotTopics) != 1 {
		t.Fatalf("load hot topics failed: found=%v err=%v items=%+v", found, err, gotTopics)
	}

	comments := []insight.CommentInsight{{ID: "comment-1", Theme: "追问主题"}}
	if err := cache.SaveCommentInsights(context.Background(), "ws-1", comments); err != nil {
		t.Fatalf("save comment insights failed: %v", err)
	}
	gotComments, found, err := cache.LoadCommentInsights(context.Background(), "ws-1")
	if err != nil || !found || len(gotComments) != 1 {
		t.Fatalf("load comment insights failed: found=%v err=%v items=%+v", found, err, gotComments)
	}

	report := insight.ReportSummary{Headline: "日报标题"}
	if err := cache.SaveReportSummary(context.Background(), "ws-1", report); err != nil {
		t.Fatalf("save report summary failed: %v", err)
	}
	gotReport, found, err := cache.LoadReportSummary(context.Background(), "ws-1")
	if err != nil || !found || gotReport.Headline != "日报标题" {
		t.Fatalf("load report summary failed: found=%v err=%v payload=%+v", found, err, gotReport)
	}

	snapshot := cache.Snapshot()
	for _, scope := range []string{"dashboard", "hot_videos", "hot_topics", "comment_insights", "report_summary"} {
		stats, ok := snapshot.ScopeStats[scope]
		if !ok {
			t.Fatalf("expected scope stats for %q, got %+v", scope, snapshot.ScopeStats)
		}
		if stats.WriteCount < 1 || stats.HitCount < 1 {
			t.Fatalf("expected scope %q to record hit/write activity, got %+v", scope, stats)
		}
		if stats.LastWriteAt.IsZero() || stats.LastHitAt.IsZero() {
			t.Fatalf("expected scope %q timestamps, got %+v", scope, stats)
		}
	}
}

func TestReadCacheInvalidateWorkspaceClearsAllKeys(t *testing.T) {
	client, err := rediscache.NewClientWithKV("127.0.0.1:6379", "", 0, &stubKV{})
	if err != nil {
		t.Fatalf("build redis stub client failed: %v", err)
	}
	cache := insight.NewReadCache(client, time.Minute)
	if err := cache.SaveDashboard(context.Background(), "ws-2", insight.Dashboard{}); err != nil {
		t.Fatalf("seed dashboard failed: %v", err)
	}
	if err := cache.SaveHotVideos(context.Background(), "ws-2", []insight.HotVideo{{ID: "v"}}); err != nil {
		t.Fatalf("seed hot videos failed: %v", err)
	}
	if err := cache.SaveHotTopics(context.Background(), "ws-2", []insight.HotTopic{{ID: "t"}}); err != nil {
		t.Fatalf("seed hot topics failed: %v", err)
	}
	if err := cache.SaveCommentInsights(context.Background(), "ws-2", []insight.CommentInsight{{ID: "c"}}); err != nil {
		t.Fatalf("seed comment insights failed: %v", err)
	}
	if err := cache.SaveReportSummary(context.Background(), "ws-2", insight.ReportSummary{Headline: "h"}); err != nil {
		t.Fatalf("seed report summary failed: %v", err)
	}

	if err := cache.InvalidateWorkspace(context.Background(), "ws-2"); err != nil {
		t.Fatalf("invalidate workspace failed: %v", err)
	}

	if _, found, err := cache.LoadDashboard(context.Background(), "ws-2"); err != nil || found {
		t.Fatalf("expected dashboard cache miss after invalidation, found=%v err=%v", found, err)
	}
	if _, found, err := cache.LoadHotVideos(context.Background(), "ws-2"); err != nil || found {
		t.Fatalf("expected hot videos cache miss after invalidation, found=%v err=%v", found, err)
	}
	if _, found, err := cache.LoadHotTopics(context.Background(), "ws-2"); err != nil || found {
		t.Fatalf("expected hot topics cache miss after invalidation, found=%v err=%v", found, err)
	}
	if _, found, err := cache.LoadCommentInsights(context.Background(), "ws-2"); err != nil || found {
		t.Fatalf("expected comment insights cache miss after invalidation, found=%v err=%v", found, err)
	}
	if _, found, err := cache.LoadReportSummary(context.Background(), "ws-2"); err != nil || found {
		t.Fatalf("expected report summary cache miss after invalidation, found=%v err=%v", found, err)
	}
}

func TestReadCacheSnapshotTracksHitsMissesWritesAndInvalidations(t *testing.T) {
	client, err := rediscache.NewClientWithKV("127.0.0.1:6379", "", 0, &stubKV{})
	if err != nil {
		t.Fatalf("build redis stub client failed: %v", err)
	}
	cache := insight.NewReadCache(client, time.Minute)

	if _, found, err := cache.LoadDashboard(context.Background(), "ws-3"); err != nil || found {
		t.Fatalf("expected initial dashboard miss, found=%v err=%v", found, err)
	}
	if err := cache.SaveDashboard(context.Background(), "ws-3", insight.Dashboard{}); err != nil {
		t.Fatalf("save dashboard failed: %v", err)
	}
	if _, found, err := cache.LoadDashboard(context.Background(), "ws-3"); err != nil || !found {
		t.Fatalf("expected dashboard hit after save, found=%v err=%v", found, err)
	}
	cache.RecordScopeFallback("dashboard", time.Date(2026, 7, 3, 14, 5, 0, 0, time.UTC))
	if err := cache.InvalidateWorkspace(context.Background(), "ws-3"); err != nil {
		t.Fatalf("invalidate workspace failed: %v", err)
	}

	snapshot := cache.Snapshot()
	if !snapshot.Enabled {
		t.Fatalf("expected cache enabled snapshot, got %+v", snapshot)
	}
	if snapshot.MissCount < 1 {
		t.Fatalf("expected miss count >= 1, got %+v", snapshot)
	}
	if snapshot.HitCount < 1 {
		t.Fatalf("expected hit count >= 1, got %+v", snapshot)
	}
	if snapshot.WriteCount < 1 {
		t.Fatalf("expected write count >= 1, got %+v", snapshot)
	}
	if snapshot.FallbackCount < 1 {
		t.Fatalf("expected fallback count >= 1, got %+v", snapshot)
	}
	if snapshot.InvalidateCount < 1 {
		t.Fatalf("expected invalidate count >= 1, got %+v", snapshot)
	}
	if snapshot.LastHitAt.IsZero() || snapshot.LastMissAt.IsZero() || snapshot.LastFallbackAt.IsZero() || snapshot.LastWriteAt.IsZero() || snapshot.LastInvalidateAt.IsZero() {
		t.Fatalf("expected non-zero timestamps, got %+v", snapshot)
	}
	dashboardScope, ok := snapshot.ScopeStats["dashboard"]
	if !ok {
		t.Fatalf("expected dashboard scope stats, got %+v", snapshot.ScopeStats)
	}
	if dashboardScope.HitCount < 1 || dashboardScope.MissCount < 1 || dashboardScope.FallbackCount < 1 || dashboardScope.WriteCount < 1 {
		t.Fatalf("expected dashboard scope counters, got %+v", dashboardScope)
	}
	if dashboardScope.LastHitAt.IsZero() || dashboardScope.LastMissAt.IsZero() || dashboardScope.LastFallbackAt.IsZero() || dashboardScope.LastWriteAt.IsZero() {
		t.Fatalf("expected dashboard scope timestamps, got %+v", dashboardScope)
	}
}
