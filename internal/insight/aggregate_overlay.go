package insight

import (
	"fmt"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func applyAggregateOverlay(payload *Dashboard, sourceSummary ingest.Summary, aggregateData aggregate.Aggregate) {
	payload.Overview.MonitoredAccounts = maxInt(aggregateData.Summary.SourceAccountCount, payload.Overview.MonitoredAccounts)
	payload.Overview.TrackedVideos = maxInt(len(aggregateData.VideoMetrics), payload.Overview.TrackedVideos)
	payload.Overview.EmergingTopics = maxInt(len(aggregateData.TopicMetrics), payload.Overview.EmergingTopics)
	payload.Overview.NewCommentBursts = maxInt(1, len(aggregateData.CommentInsightFacts))
	payload.Overview.DailyCoverageRate = deriveCoverageRate(sourceSummary.SourceCount, payload.Workspace.DeviceCount)
	if !aggregateData.Summary.LatestObservedAt.IsZero() {
		payload.Overview.ReportFreshness = humanizeFreshness(aggregateData.Summary.LatestObservedAt, payload.GeneratedAt)
		payload.ReportSummary.Period = aggregateData.Summary.LatestObservedAt.Format("2006-01-02") + " 日报"
	}

	payload.HotVideos = MergeHotVideos(mapAggregateVideoMetrics(aggregateData.VideoMetrics, payload.GeneratedAt), payload.HotVideos, hotVideoRankingLimit)
	payload.HotTopics = MergeHotTopics(mapAggregateTopicMetrics(aggregateData.TopicMetrics), payload.HotTopics, hotTopicRankingLimit)
	payload.CommentInsights = MergeCommentInsights(mapAggregateCommentFacts(aggregateData.CommentInsightFacts), payload.CommentInsights, 5)
	payload.WatchlistAlerts = mergeAggregateAlerts(buildAggregateWatchlistAlerts(aggregateData, payload.GeneratedAt), payload.WatchlistAlerts, 5)
	applyAggregateReportSummary(payload, aggregateData, payload.GeneratedAt)
}

func mapAggregateVideoMetrics(items []aggregate.VideoMetric, referenceAt time.Time) []HotVideo {
	results := make([]HotVideo, 0, len(items))
	for _, item := range items {
		results = append(results, HotVideo{
			ID:              item.ID,
			Title:           item.Title,
			ChannelName:     item.ChannelName,
			Source:          item.Source,
			ObjectID:        item.ObjectID,
			ObjectNonceID:   item.ObjectNonceID,
			CoverURL:        item.CoverURL,
			Tags:            append([]string(nil), item.Tags...),
			PublishedAt:     item.PublishedAt,
			PublishedAgo:    publishedAgo(item.PublishedAt, referenceAt),
			Views:           item.Views,
			Likes:           item.Likes,
			Comments:        item.Comments,
			Favorites:       item.Favorites,
			Forwards:        item.Forwards,
			GrowthRate:      item.GrowthRate,
			EngagementScore: item.EngagementScore,
			Signal:          item.Signal,
		})
	}
	return results
}

func mapAggregateTopicMetrics(items []aggregate.TopicMetric) []HotTopic {
	results := make([]HotTopic, 0, len(items))
	for _, item := range items {
		topic, ok := normalizeHotTopic(HotTopic{
			ID:            item.ID,
			Keyword:       item.Keyword,
			Cluster:       item.Cluster,
			Source:        item.Source,
			ObservedAt:    item.ObservedAt,
			Mentions:      item.Mentions,
			GrowthRate:    item.GrowthRate,
			RelatedVideos: item.RelatedVideos,
			PromptAngle:   item.PromptAngle,
			Keywords:      append([]string(nil), item.Keywords...),
		})
		if ok {
			results = append(results, topic)
		}
	}
	return results
}

func mapAggregateCommentFacts(items []aggregate.CommentInsightFact) []CommentInsight {
	results := make([]CommentInsight, 0, len(items))
	for _, item := range items {
		results = append(results, CommentInsight{
			ID:               item.ID,
			Theme:            item.Theme,
			Source:           item.Source,
			ObservedAt:       item.ObservedAt,
			ObjectID:         item.ObjectID,
			ObjectNonceID:    item.ObjectNonceID,
			QuestionRate:     item.QuestionRate,
			Sentiment:        item.Sentiment,
			Representative:   item.Representative,
			ActionSuggestion: item.ActionSuggestion,
			RelatedKeywords:  append([]string(nil), item.RelatedKeywords...),
		})
	}
	return results
}

func buildAggregateWatchlistAlerts(input aggregate.Aggregate, referenceAt time.Time) []WatchlistAlert {
	results := make([]WatchlistAlert, 0, len(input.VideoMetrics)+len(input.TopicMetrics)+len(input.CommentInsightFacts))
	seen := map[string]struct{}{}

	for _, item := range input.VideoMetrics {
		alert := WatchlistAlert{
			ID:           strings.Replace(item.ID, "ingest-video", "alert-video", 1),
			AccountName:  fallbackString(item.ChannelName, "Insight Aggregate"),
			AlertType:    "新热视频",
			Source:       item.Source,
			Severity:     "high",
			Summary:      fmt.Sprintf("%s 进入高增速观察区，当前信号为 %s。", item.Title, fallbackString(item.Signal, "聚合派生")),
			OccurredAt:   item.ObservedAt,
			HappenedAt:   humanizeFreshness(item.ObservedAt, referenceAt),
			Recommended:  "优先拆解标题、结构和评论区承接方式，判断是否值得跟进。",
			VideoCount:   1,
			CommentDelta: item.Comments,
		}
		appendUniqueWatchlistAlert(&results, seen, alert)
	}
	for _, item := range input.CommentInsightFacts {
		alert := WatchlistAlert{
			ID:           fmt.Sprintf("alert-comment-%s", item.BatchID),
			AccountName:  fallbackString(item.TargetAudience, "Insight Aggregate"),
			AlertType:    "评论爆发",
			Source:       item.Source,
			Severity:     "medium",
			Summary:      fallbackString(item.Representative, fmt.Sprintf("评论区开始集中追问 %s。", item.Theme)),
			OccurredAt:   item.ObservedAt,
			HappenedAt:   humanizeFreshness(item.ObservedAt, referenceAt),
			Recommended:  fmt.Sprintf("围绕“%s”优先做一条回答型内容。", fallbackString(item.Theme, "高追问问题")),
			VideoCount:   1,
			CommentDelta: item.CommentCount,
		}
		appendUniqueWatchlistAlert(&results, seen, alert)
	}
	for _, item := range input.TopicMetrics {
		alert := WatchlistAlert{
			ID:           fmt.Sprintf("alert-topic-%s", item.BatchID),
			AccountName:  fallbackString(item.Cluster, "Insight Aggregate"),
			AlertType:    "主题切换",
			Source:       item.Source,
			Severity:     "low",
			Summary:      fmt.Sprintf("%s 相关讨论正在升温，适合继续观察跨账号共振。", item.Keyword),
			OccurredAt:   item.ObservedAt,
			HappenedAt:   humanizeFreshness(item.ObservedAt, referenceAt),
			Recommended:  fallbackString(item.PromptAngle, "继续观察是否值得扩成稳定栏目。"),
			VideoCount:   maxInt(1, item.RelatedVideos),
			CommentDelta: maxInt(0, item.Mentions),
		}
		appendUniqueWatchlistAlert(&results, seen, alert)
	}
	return results
}

func mergeAggregateAlerts(primary, fallback []WatchlistAlert, limit int) []WatchlistAlert {
	results := make([]WatchlistAlert, 0, maxInt(len(primary), len(fallback)))
	seen := map[string]struct{}{}
	for _, item := range primary {
		appendUniqueWatchlistAlert(&results, seen, item)
		if limit > 0 && len(results) >= limit {
			return results
		}
	}
	for _, item := range fallback {
		appendUniqueWatchlistAlert(&results, seen, item)
		if limit > 0 && len(results) >= limit {
			return results
		}
	}
	return results
}

func applyAggregateReportSummary(payload *Dashboard, input aggregate.Aggregate, referenceAt time.Time) {
	topics := mapAggregateTopicMetrics(input.TopicMetrics)
	topicList := joinTopKeywords(topics, 3)
	topicCount := minInt(3, len(topics))
	switch {
	case topicList == "":
		payload.ReportSummary.Headline = fmt.Sprintf(
			"多类运营信号正在走高，最新 aggregate 已覆盖 %d 个来源账号。",
			maxInt(1, input.Summary.SourceAccountCount),
		)
	case topicCount <= 1:
		payload.ReportSummary.Headline = fmt.Sprintf(
			"%s 正在走高，最新 aggregate 已覆盖 %d 个来源账号。",
			topicList,
			maxInt(1, input.Summary.SourceAccountCount),
		)
	default:
		payload.ReportSummary.Headline = fmt.Sprintf(
			"%s 等 %d 组主题正在走高，最新 aggregate 已覆盖 %d 个来源账号。",
			topicList,
			topicCount,
			maxInt(1, input.Summary.SourceAccountCount),
		)
	}

	questionFocus := ""
	if len(input.CommentInsightFacts) > 0 {
		questionFocus = input.CommentInsightFacts[0].Theme
	}
	if questionFocus == "" {
		questionFocus = "高追问评论"
	}
	payload.ReportSummary.KeyFinding = fmt.Sprintf(
		"当前累计得到 %d 个视频指标、%d 组评论事实、%d 组话题指标；其中“%s”是最值得优先回答的评论入口。",
		len(input.VideoMetrics),
		len(input.CommentInsightFacts),
		len(input.TopicMetrics),
		questionFocus,
	)
	payload.ReportSummary.RecommendedAction = "核对账号和关键词样本的指标变化、采集覆盖及评论证据。"
	if len(payload.WatchlistAlerts) > 0 {
		payload.ReportSummary.Watchout = fmt.Sprintf("当前仍是团队版过渡态，但需要优先盯住：%s", payload.WatchlistAlerts[0].Summary)
	} else if len(input.CommentInsightFacts) > 0 {
		payload.ReportSummary.Watchout = fmt.Sprintf("当前仍是团队版过渡态，代表性追问已经出现：%s", input.CommentInsightFacts[0].Representative)
	} else {
		payload.ReportSummary.Watchout = "当前仍是团队版过渡态，但 aggregate owner 已经开始承接热点指标。"
	}
	payload.ReportSummary.GeneratedAt = referenceAt
	payload.ReportSummary.AutoGeneratedAt = humanizeFreshness(referenceAt, referenceAt)
}
