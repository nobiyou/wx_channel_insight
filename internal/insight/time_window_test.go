package insight

import (
	"testing"
	"time"
)

func TestFilterDashboardByTimeWindowUsesOneReferenceForAllAnalyticalData(t *testing.T) {
	referenceAt := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	recent := referenceAt.Add(-6 * time.Hour)
	medium := referenceAt.Add(-48 * time.Hour)
	old := referenceAt.Add(-96 * time.Hour)
	newDashboard := func() Dashboard {
		return Dashboard{
			GeneratedAt: referenceAt,
			ReportSummary: ReportSummary{
				Period:            "2026-08-23 日报",
				Headline:          "累计数据摘要",
				KeyFinding:        "累计数据发现",
				RecommendedAction: "累计数据动作",
				Watchout:          "累计数据风险",
			},
			HotVideos: []HotVideo{
				{ID: "video-recent", PublishedAt: recent},
				{ID: "video-medium", PublishedAt: medium},
				{ID: "video-old", PublishedAt: old},
			},
			HotVideoRankings: HotVideoRankings{
				HotVideoRankingHot:   {{ID: "video-recent", PublishedAt: recent}, {ID: "video-old", PublishedAt: old}},
				HotVideoRankingLikes: {{ID: "video-recent", PublishedAt: recent}},
			},
			HotVideoWindowRankings: HotVideoWindowRankings{
				TimeWindow24h: {HotVideoRankingHot: {{ID: "video-recent", PublishedAt: recent}, {ID: "stale-window-item", PublishedAt: old}}},
				TimeWindow72h: {HotVideoRankingHot: {{ID: "video-recent", PublishedAt: recent}, {ID: "video-medium", PublishedAt: medium}}},
			},
			HotTopics: []HotTopic{
				{ID: "topic-recent", ObservedAt: recent},
				{ID: "topic-medium", ObservedAt: medium},
				{ID: "topic-old", ObservedAt: old},
			},
			CommentInsights: []CommentInsight{
				{ID: "comment-recent", ObservedAt: recent},
				{ID: "comment-medium", ObservedAt: medium},
				{ID: "comment-old", ObservedAt: old},
			},
			WatchlistAlerts: []WatchlistAlert{
				{ID: "alert-recent", OccurredAt: recent},
				{ID: "alert-old", OccurredAt: old},
			},
			AccountRecommendations: []AccountRecommendation{
				{ID: "account-recent", LastObservedAt: recent},
				{ID: "account-old", LastObservedAt: old},
			},
			VideoRecommendations: []VideoRecommendation{
				{ID: "recommendation-recent", PublishedAt: recent},
				{ID: "recommendation-old", PublishedAt: old},
			},
		}
	}

	all := newDashboard()
	if err := FilterDashboardByTimeWindow(&all, TimeWindowAll, referenceAt); err != nil {
		t.Fatalf("filter all-time dashboard: %v", err)
	}
	if len(all.HotVideos) != 3 || len(all.HotTopics) != 3 || len(all.CommentInsights) != 3 {
		t.Fatalf("all-time filter changed data: videos=%d topics=%d comments=%d", len(all.HotVideos), len(all.HotTopics), len(all.CommentInsights))
	}

	within24h := newDashboard()
	if err := FilterDashboardByTimeWindow(&within24h, TimeWindow24h, referenceAt); err != nil {
		t.Fatalf("filter 24h dashboard: %v", err)
	}
	if len(within24h.HotVideos) != 1 || within24h.HotVideos[0].ID != "video-recent" {
		t.Fatalf("24h video ranking mismatch: %+v", within24h.HotVideos)
	}
	if len(within24h.HotTopics) != 1 || within24h.HotTopics[0].ID != "topic-recent" {
		t.Fatalf("24h topic filter mismatch: %+v", within24h.HotTopics)
	}
	if len(within24h.CommentInsights) != 1 || within24h.CommentInsights[0].ID != "comment-recent" {
		t.Fatalf("24h comment filter mismatch: %+v", within24h.CommentInsights)
	}
	if len(within24h.WatchlistAlerts) != 1 || len(within24h.AccountRecommendations) != 1 || len(within24h.VideoRecommendations) != 1 {
		t.Fatalf("24h auxiliary filters mismatch: alerts=%d accounts=%d videos=%d", len(within24h.WatchlistAlerts), len(within24h.AccountRecommendations), len(within24h.VideoRecommendations))
	}
	if within24h.Overview.TrackedVideos != 1 || within24h.Overview.EmergingTopics != 1 || within24h.Overview.NewCommentBursts != 1 {
		t.Fatalf("24h overview counts mismatch: %+v", within24h.Overview)
	}
	if within24h.ReportSummary.Period != "近 24 小时" || within24h.ReportSummary.Headline == "累计数据摘要" || within24h.ReportSummary.GeneratedAt.IsZero() {
		t.Fatalf("24h report summary must be rebuilt for the bounded window: %+v", within24h.ReportSummary)
	}

	within72h := newDashboard()
	if err := FilterDashboardByTimeWindow(&within72h, TimeWindow72h, referenceAt); err != nil {
		t.Fatalf("filter 72h dashboard: %v", err)
	}
	if len(within72h.HotVideos) != 2 || len(within72h.HotTopics) != 2 || len(within72h.CommentInsights) != 2 {
		t.Fatalf("72h filter mismatch: videos=%d topics=%d comments=%d", len(within72h.HotVideos), len(within72h.HotTopics), len(within72h.CommentInsights))
	}
	if within72h.ReportSummary.Period != "近 72 小时" || within72h.ReportSummary.Headline == "累计数据摘要" {
		t.Fatalf("72h report summary must be rebuilt for the bounded window: %+v", within72h.ReportSummary)
	}
}

func TestNormalizeTimeWindowRejectsUnsupportedValues(t *testing.T) {
	if normalized, ok := NormalizeTimeWindow(""); !ok || normalized != TimeWindowAll {
		t.Fatalf("empty time window should mean all, got %q, %v", normalized, ok)
	}
	if normalized, ok := NormalizeTimeWindow("24H"); !ok || normalized != TimeWindow24h {
		t.Fatalf("24H should normalize to 24h, got %q, %v", normalized, ok)
	}
	if _, ok := NormalizeTimeWindow("7d"); ok {
		t.Fatal("unsupported time window should be rejected")
	}
}

func TestDashboardTimeWindowReferenceUsesLatestOverlayObservation(t *testing.T) {
	payload := Dashboard{
		GeneratedAt: time.Date(2026, 8, 23, 6, 23, 57, 0, time.UTC),
		IngestSummary: IngestSummary{
			LatestEventAt: time.Date(2026, 8, 23, 8, 5, 45, 0, time.UTC),
		},
		CommentInsights: []CommentInsight{{
			ObservedAt: time.Date(2026, 8, 23, 8, 5, 0, 0, time.UTC),
		}},
	}

	got := dashboardTimeWindowReference(payload)
	want := time.Date(2026, 8, 23, 8, 5, 45, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected latest overlay observation as reference, got %v", got)
	}

	if err := FilterDashboardByTimeWindow(&payload, TimeWindow24h, time.Time{}); err != nil {
		t.Fatalf("filter dashboard by 24h: %v", err)
	}
}
