package insight

import (
	"context"
	"log"
	"time"
)

const (
	DashboardPageRecommendationLimit = 50
	DashboardPageHotVideoLimit       = 10
	DashboardPageHotTopicLimit       = 5
	DashboardPageNestedDetailLimit   = 20
)

func persistDashboardSnapshot(
	ctx context.Context,
	workspaceID string,
	snapshot Dashboard,
	repo DashboardRepository,
	rankingRepo RankingRepository,
	commentInsightRepo CommentInsightRepository,
	reportSummaryRepo ReportSummaryRepository,
	dailyReportRepo DailyReportRepository,
	readCache *ReadCache,
) (Dashboard, error) {
	snapshotToPersist := snapshot

	if repo != nil {
		if err := repo.ReplaceDashboard(ctx, workspaceID, dashboardSnapshotForPrimaryStore(snapshotToPersist)); err != nil {
			return Dashboard{}, err
		}
	}
	if rankingRepo != nil {
		if err := rankingRepo.ReplaceHotVideos(ctx, workspaceID, snapshotToPersist.HotVideos); err != nil {
			return Dashboard{}, err
		}
		if snapshotToPersist.HotVideoRankings != nil {
			if err := rankingRepo.ReplaceHotVideoRankings(ctx, workspaceID, snapshotToPersist.HotVideoRankings); err != nil {
				return Dashboard{}, err
			}
		}
		if err := rankingRepo.ReplaceHotTopics(ctx, workspaceID, snapshotToPersist.HotTopics); err != nil {
			return Dashboard{}, err
		}
	}
	if commentInsightRepo != nil {
		if err := commentInsightRepo.ReplaceCommentInsights(ctx, workspaceID, snapshotToPersist.CommentInsights); err != nil {
			return Dashboard{}, err
		}
	}
	if reportSummaryRepo != nil {
		if err := reportSummaryRepo.ReplaceReportSummary(ctx, workspaceID, snapshotToPersist.ReportSummary); err != nil {
			return Dashboard{}, err
		}
	}
	if err := persistReadModelArtifactMeta(ctx, workspaceID, snapshotToPersist.ReadModel, rankingRepo, commentInsightRepo, reportSummaryRepo); err != nil {
		return Dashboard{}, err
	}
	if dailyReportRepo != nil {
		dailyReportSnapshot := dashboardSnapshotForPrimaryStore(snapshotToPersist)
		if err := FilterDashboardByTimeWindow(&dailyReportSnapshot, TimeWindow72h, time.Time{}); err != nil {
			log.Printf("daily report archive skipped workspace=%s window=%s: %v", workspaceID, TimeWindow72h, err)
		} else {
			report, err := NewDailyReport(workspaceID, DailyReportDate(dailyReportSnapshot.GeneratedAt), TimeWindow72h, dailyReportSnapshot.GeneratedAt, dailyReportSnapshot)
			if err == nil {
				err = dailyReportRepo.Upsert(ctx, report)
			}
			if err != nil {
				log.Printf("daily report archive skipped workspace=%s window=%s: %v", workspaceID, TimeWindow72h, err)
			}
		}
	}
	if readCache != nil {
		_ = readCache.SaveDashboard(ctx, workspaceID, dashboardSnapshotForPrimaryStore(snapshotToPersist))
		_ = readCache.SaveHotVideos(ctx, workspaceID, snapshotToPersist.HotVideos)
		_ = readCache.SaveHotTopics(ctx, workspaceID, snapshotToPersist.HotTopics)
		_ = readCache.SaveCommentInsights(ctx, workspaceID, snapshotToPersist.CommentInsights)
		_ = readCache.SaveReportSummary(ctx, workspaceID, snapshotToPersist.ReportSummary)
	}
	return snapshotToPersist, nil
}

// The dashboard snapshot is a bounded analysis summary. Dedicated read-model
// repositories retain the complete recommendation sets for deeper queries.
func dashboardSnapshotForPrimaryStore(snapshot Dashboard) Dashboard {
	projection := snapshot
	projection.AccountRecommendations = limitAccountRecommendationsForSnapshot(snapshot.AccountRecommendations)
	projection.VideoRecommendations = limitVideoRecommendationsForSnapshot(snapshot.VideoRecommendations)
	projection.HotVideos = limitHotVideosForSnapshot(snapshot.HotVideos)
	projection.HotVideoRankings = limitHotVideoRankingsForSnapshot(snapshot.HotVideoRankings)
	projection.HotVideoWindowRankings = limitHotVideoWindowRankingsForSnapshot(snapshot.HotVideoWindowRankings)
	projection.HotTopics = limitHotTopicsForSnapshot(snapshot.HotTopics)
	return projection
}

func limitAccountRecommendationsForSnapshot(items []AccountRecommendation) []AccountRecommendation {
	if len(items) > DashboardPageRecommendationLimit {
		items = items[:DashboardPageRecommendationLimit]
	}
	return append([]AccountRecommendation(nil), items...)
}

func limitVideoRecommendationsForSnapshot(items []VideoRecommendation) []VideoRecommendation {
	if len(items) > DashboardPageRecommendationLimit {
		items = items[:DashboardPageRecommendationLimit]
	}
	return append([]VideoRecommendation(nil), items...)
}

func limitHotVideosForSnapshot(items []HotVideo) []HotVideo {
	if len(items) > DashboardPageHotVideoLimit {
		items = items[:DashboardPageHotVideoLimit]
	}
	return cloneHotVideos(items)
}

func limitHotTopicsForSnapshot(items []HotTopic) []HotTopic {
	if len(items) > DashboardPageHotTopicLimit {
		items = items[:DashboardPageHotTopicLimit]
	}
	return cloneHotTopics(items)
}

func limitHotVideoRankingsForSnapshot(source HotVideoRankings) HotVideoRankings {
	if source == nil {
		return nil
	}
	result := make(HotVideoRankings, len(source))
	for kind, items := range source {
		result[kind] = limitHotVideosForSnapshot(items)
	}
	return result
}

func limitHotVideoWindowRankingsForSnapshot(source HotVideoWindowRankings) HotVideoWindowRankings {
	if source == nil {
		return nil
	}
	result := make(HotVideoWindowRankings, len(source))
	for window, rankings := range source {
		result[window] = limitHotVideoRankingsForSnapshot(rankings)
	}
	return result
}
