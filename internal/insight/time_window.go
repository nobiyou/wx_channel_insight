package insight

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TimeWindowAll = "all"
	TimeWindow24h = "24h"
	TimeWindow72h = "72h"
)

var ErrInvalidTimeWindow = errors.New("time window must be all, 24h, or 72h")

// NormalizeTimeWindow is the single contract for the three supported data
// scopes. Empty input intentionally means all historical data.
func NormalizeTimeWindow(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return TimeWindowAll, true
	}
	switch value {
	case TimeWindowAll, TimeWindow24h, TimeWindow72h:
		return value, true
	default:
		return "", false
	}
}

func TimeWindowDuration(window string) time.Duration {
	switch window {
	case TimeWindow24h:
		return 24 * time.Hour
	case TimeWindow72h:
		return 72 * time.Hour
	default:
		return 0
	}
}

func timeWindowLabel(window string) string {
	switch window {
	case TimeWindow24h:
		return "近 24 小时"
	case TimeWindow72h:
		return "近 72 小时"
	default:
		return "全部"
	}
}

func matchesTimeWindow(value time.Time, window string, referenceAt time.Time) bool {
	if window == TimeWindowAll {
		return true
	}
	if referenceAt.IsZero() || value.IsZero() || value.After(referenceAt) {
		return false
	}
	return referenceAt.Sub(value) <= TimeWindowDuration(window)
}

// dashboardTimeWindowReference anchors bounded windows to the newest
// observation available in the payload. A persisted snapshot can be older
// than a freshly overlaid ingest batch, so GeneratedAt alone is insufficient.
func dashboardTimeWindowReference(payload Dashboard) time.Time {
	referenceAt := payload.GeneratedAt
	consider := func(value time.Time) {
		if !value.IsZero() && value.After(referenceAt) {
			referenceAt = value
		}
	}
	consider(payload.IngestSummary.LatestEventAt)
	consider(payload.ReportSummary.GeneratedAt)
	for _, item := range payload.HotVideos {
		consider(item.PublishedAt)
		consider(item.ObservedAt)
	}
	for _, item := range payload.HotTopics {
		consider(item.ObservedAt)
	}
	for _, item := range payload.CommentInsights {
		consider(item.ObservedAt)
	}
	for _, item := range payload.WatchlistAlerts {
		consider(item.OccurredAt)
	}
	for _, item := range payload.AccountRecommendations {
		consider(item.LastObservedAt)
	}
	for _, item := range payload.VideoRecommendations {
		consider(item.PublishedAt)
		consider(item.LastObservedAt)
	}
	return referenceAt
}

func filterHotVideosByTimeWindow(items []HotVideo, window string, referenceAt time.Time) []HotVideo {
	filtered := make([]HotVideo, 0, len(items))
	for _, item := range items {
		if matchesTimeWindow(item.PublishedAt, window, referenceAt) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterHotTopicsByTimeWindow(items []HotTopic, window string, referenceAt time.Time) []HotTopic {
	filtered := make([]HotTopic, 0, len(items))
	for _, item := range items {
		if matchesTimeWindow(item.ObservedAt, window, referenceAt) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterCommentInsightsByTimeWindow(items []CommentInsight, window string, referenceAt time.Time) []CommentInsight {
	filtered := make([]CommentInsight, 0, len(items))
	for _, item := range items {
		if matchesTimeWindow(item.ObservedAt, window, referenceAt) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterAccountRecommendationsByTimeWindow(items []AccountRecommendation, window string, referenceAt time.Time) []AccountRecommendation {
	filtered := make([]AccountRecommendation, 0, len(items))
	for _, item := range items {
		if matchesTimeWindow(item.LastObservedAt, window, referenceAt) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterVideoRecommendationsByTimeWindow(items []VideoRecommendation, window string, referenceAt time.Time) []VideoRecommendation {
	filtered := make([]VideoRecommendation, 0, len(items))
	for _, item := range items {
		value := item.PublishedAt
		if value.IsZero() {
			value = item.LastObservedAt
		}
		if matchesTimeWindow(value, window, referenceAt) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterTopicRecommendationsByTimeWindow(items []TopicRecommendation, window string, referenceAt time.Time) []TopicRecommendation {
	filtered := make([]TopicRecommendation, 0, len(items))
	for _, item := range items {
		if matchesTimeWindow(item.LastObservedAt, window, referenceAt) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// FilterDashboardByTimeWindow applies the same range semantics to every
// analytical collection returned by the dashboard. The all-time payload is
// preserved byte-for-byte in shape, while a bounded window also replaces
// overview counts with counts from the bounded sample.
func FilterDashboardByTimeWindow(payload *Dashboard, window string, referenceAt time.Time) error {
	if payload == nil {
		return nil
	}
	normalized, ok := NormalizeTimeWindow(window)
	if !ok {
		return ErrInvalidTimeWindow
	}
	if normalized == TimeWindowAll {
		return nil
	}
	if referenceAt.IsZero() {
		referenceAt = dashboardTimeWindowReference(*payload)
	}
	if referenceAt.IsZero() {
		referenceAt = time.Now()
	}

	if rankings, exists := payload.HotVideoWindowRankings[normalized]; exists {
		filteredRankings := make(HotVideoRankings, len(rankings))
		for kind, items := range rankings {
			filteredRankings[kind] = filterHotVideosByTimeWindow(items, normalized, referenceAt)
		}
		payload.HotVideoRankings = filteredRankings
		payload.HotVideos = cloneHotVideos(filteredRankings[HotVideoRankingHot])
	} else {
		payload.HotVideos = filterHotVideosByTimeWindow(payload.HotVideos, normalized, referenceAt)
		filteredRankings := make(HotVideoRankings, len(payload.HotVideoRankings))
		for kind, items := range payload.HotVideoRankings {
			filteredRankings[kind] = filterHotVideosByTimeWindow(items, normalized, referenceAt)
		}
		payload.HotVideoRankings = filteredRankings
	}
	payload.HotTopics = filterHotTopicsByTimeWindow(payload.HotTopics, normalized, referenceAt)
	payload.CommentInsights = filterCommentInsightsByTimeWindow(payload.CommentInsights, normalized, referenceAt)
	payload.WatchlistAlerts = filterWatchlistAlertsByTimeWindow(payload.WatchlistAlerts, normalized, referenceAt)
	payload.AccountRecommendations = filterAccountRecommendationsByTimeWindow(payload.AccountRecommendations, normalized, referenceAt)
	payload.VideoRecommendations = filterVideoRecommendationsByTimeWindow(payload.VideoRecommendations, normalized, referenceAt)
	payload.Overview.TrackedVideos = len(payload.HotVideos)
	payload.Overview.EmergingTopics = len(payload.HotTopics)
	payload.Overview.NewCommentBursts = len(payload.CommentInsights)
	payload.Overview.MonitoredAccounts = countDashboardAccounts(payload.HotVideos, payload.WatchlistAlerts)
	applyReportSummaryTimeWindow(payload, normalized, referenceAt)
	return nil
}

// applyReportSummaryTimeWindow keeps the report card truthful after the
// underlying dashboard collections have been narrowed to a bounded window.
// The all-time summary remains owned by the persisted report read model.
func applyReportSummaryTimeWindow(payload *Dashboard, window string, referenceAt time.Time) {
	if payload == nil || window == TimeWindowAll {
		return
	}
	if referenceAt.IsZero() {
		referenceAt = payload.GeneratedAt
	}
	if referenceAt.IsZero() {
		referenceAt = time.Now()
	}

	label := timeWindowLabel(window)
	payload.ReportSummary.Period = label
	payload.ReportSummary.GeneratedAt = referenceAt
	payload.ReportSummary.AutoGeneratedAt = humanizeFreshness(referenceAt, referenceAt)

	topicList := joinTopKeywords(payload.HotTopics, 3)
	if topicList == "" {
		payload.ReportSummary.Headline = fmt.Sprintf("%s内暂无可识别的活跃话题信号。", label)
	} else {
		payload.ReportSummary.Headline = fmt.Sprintf("%s内，%s等 %d 组话题进入当前分析窗口。", label, topicList, len(payload.HotTopics))
	}

	commentFocus := "高追问评论"
	if len(payload.CommentInsights) > 0 && strings.TrimSpace(payload.CommentInsights[0].Theme) != "" {
		commentFocus = payload.CommentInsights[0].Theme
	}
	payload.ReportSummary.KeyFinding = fmt.Sprintf(
		"当前窗口呈现 %d 条重点视频、%d 组评论洞察和 %d 组话题信号；优先关注“%s”。",
		len(payload.HotVideos), len(payload.CommentInsights), len(payload.HotTopics), commentFocus,
	)

	switch {
	case len(payload.CommentInsights) > 0 && strings.TrimSpace(payload.CommentInsights[0].Theme) != "":
		payload.ReportSummary.RecommendedAction = fmt.Sprintf("核对“%s”的评论样本及下一轮指标变化。", payload.CommentInsights[0].Theme)
	case len(payload.HotVideos) > 0 && strings.TrimSpace(payload.HotVideos[0].Title) != "":
		payload.ReportSummary.RecommendedAction = fmt.Sprintf("先拆解《%s》的标题、结构和评论承接。", payload.HotVideos[0].Title)
	default:
		payload.ReportSummary.RecommendedAction = fmt.Sprintf("%s内暂无足够信号，先继续积累真实数据。", label)
	}

	switch {
	case len(payload.WatchlistAlerts) > 0 && strings.TrimSpace(payload.WatchlistAlerts[0].Summary) != "":
		payload.ReportSummary.Watchout = payload.WatchlistAlerts[0].Summary
	case len(payload.CommentInsights) > 0 && strings.TrimSpace(payload.CommentInsights[0].Representative) != "":
		payload.ReportSummary.Watchout = payload.CommentInsights[0].Representative
	default:
		payload.ReportSummary.Watchout = fmt.Sprintf("%s内没有额外风险提示。", label)
	}
}

func filterWatchlistAlertsByTimeWindow(items []WatchlistAlert, window string, referenceAt time.Time) []WatchlistAlert {
	filtered := make([]WatchlistAlert, 0, len(items))
	for _, item := range items {
		if matchesTimeWindow(item.OccurredAt, window, referenceAt) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func countDashboardAccounts(videos []HotVideo, alerts []WatchlistAlert) int {
	accounts := map[string]struct{}{}
	for _, item := range videos {
		if name := strings.TrimSpace(item.ChannelName); name != "" {
			accounts[name] = struct{}{}
		} else if username := strings.TrimSpace(item.ChannelUsername); username != "" {
			accounts[username] = struct{}{}
		}
	}
	for _, item := range alerts {
		if name := strings.TrimSpace(item.AccountName); name != "" {
			accounts[name] = struct{}{}
		}
	}
	return len(accounts)
}
