package insight

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

type ingestDerivedSignals struct {
	videos          []HotVideo
	topics          []HotTopic
	comment         *ingest.Event
	commentInsights []CommentInsight
	alerts          []WatchlistAlert
}

func applyIngestOverlay(payload *Dashboard, summary ingest.Summary) {
	if payload == nil || summary.TotalEvents == 0 {
		return
	}

	payload.Overview.MonitoredAccounts = deriveMonitoredAccountCount(summary)
	payload.Overview.DailyCoverageRate = deriveCoverageRate(summary.SourceCount, payload.Workspace.DeviceCount)
	if summary.AggregatedVideos > 0 {
		payload.Overview.TrackedVideos = summary.AggregatedVideos
	}
	if summary.AggregatedComments > 0 {
		payload.Overview.NewCommentBursts = maxInt(1, summary.AggregatedComments/60)
	}
	if summary.AggregatedTopics > 0 {
		payload.Overview.EmergingTopics = summary.AggregatedTopics
	}
	if !summary.LatestEventAt.IsZero() {
		payload.ReportSummary.Period = summary.LatestEventAt.Format("2006-01-02") + " 日报"
		payload.Overview.ReportFreshness = humanizeFreshness(summary.LatestEventAt, payload.GeneratedAt)
	}

	signals := deriveIngestSignals(summary, payload.GeneratedAt)
	applyIngestHotVideoOverlay(payload, signals)
	applyIngestHotTopicOverlay(payload, signals)
	applyIngestCommentInsightOverlay(payload, signals)
	applyIngestWatchlistOverlay(payload, summary, signals)
	applyIngestReportOverlay(payload, summary, signals)
}

func humanizeFreshness(latest, generatedAt time.Time) string {
	if latest.IsZero() {
		return ""
	}

	base := generatedAt
	if base.IsZero() || latest.After(base) {
		base = latest
	}

	diff := base.Sub(latest)
	if diff < time.Minute {
		return "刚刚"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%d 分钟前", int(diff.Minutes()))
	}
	return fmt.Sprintf("%d 小时前", int(diff.Hours()))
}

func publishedAgo(publishedAt, referenceAt time.Time) string {
	if publishedAt.IsZero() {
		return ""
	}
	return humanizeFreshness(publishedAt, referenceAt)
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func fallbackString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func sourceAccountsFromEvent(event ingest.Event) []string {
	account := strings.TrimSpace(event.ChannelName)
	if account == "" {
		return nil
	}
	return []string{account}
}

func deriveMonitoredAccountCount(summary ingest.Summary) int {
	seen := map[string]struct{}{}
	for _, event := range summary.RecentEvents {
		name := strings.TrimSpace(event.ChannelName)
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}
	if len(seen) > 0 {
		return len(seen)
	}
	return maxInt(0, summary.SourceCount)
}

func deriveCoverageRate(sourceCount, deviceCount int) float64 {
	if sourceCount <= 0 {
		return 0
	}
	if deviceCount <= 0 {
		return 100
	}
	covered := minInt(sourceCount, deviceCount)
	return math.Round((float64(covered)*1000)/float64(deviceCount)) / 10
}

func ingestScopedID(prefix string, event ingest.Event) string {
	switch {
	case event.ID == "" && event.BatchID == "":
		return prefix
	case event.ID == "":
		return fmt.Sprintf("%s-%s", prefix, event.BatchID)
	case event.BatchID == "":
		return fmt.Sprintf("%s-%s", prefix, event.ID)
	case strings.Contains(event.ID, event.BatchID):
		return fmt.Sprintf("%s-%s", prefix, event.ID)
	default:
		return fmt.Sprintf("%s-%s-%s", prefix, event.BatchID, event.ID)
	}
}

func applyIngestWatchlistOverlay(payload *Dashboard, summary ingest.Summary, signals ingestDerivedSignals) {
	if len(summary.RecentEvents) == 0 {
		return
	}

	alerts := signals.alerts
	if len(alerts) == 0 {
		latest := summary.RecentEvents[0]
		alerts = []WatchlistAlert{{
			ID:           "alert-ingest-latest",
			AccountName:  "Insight Ingest",
			AlertType:    "批次接入",
			Source:       latest.Source,
			Severity:     "medium",
			Summary:      fmt.Sprintf("最新 batch %s 已接入，最近事件类型为 %s。", summary.LatestBatchID, latest.EventType),
			OccurredAt:   summary.LatestEventAt,
			HappenedAt:   humanizeFreshness(summary.LatestEventAt, payload.GeneratedAt),
			Recommended:  "优先检查这批事件是否已经足够支撑下一轮 watchlist 和日报判断。",
			VideoCount:   summary.AggregatedVideos,
			CommentDelta: summary.AggregatedComments,
		}}
	}

	merged := make([]WatchlistAlert, 0, maxInt(len(alerts), len(payload.WatchlistAlerts)))
	seen := map[string]struct{}{}
	for _, alert := range alerts {
		if _, ok := seen[alert.ID]; ok {
			continue
		}
		seen[alert.ID] = struct{}{}
		merged = append(merged, alert)
		if len(merged) >= 5 {
			payload.WatchlistAlerts = merged
			return
		}
	}
	for _, existing := range payload.WatchlistAlerts {
		if _, ok := seen[existing.ID]; ok {
			continue
		}
		seen[existing.ID] = struct{}{}
		merged = append(merged, existing)
		if len(merged) >= 5 {
			payload.WatchlistAlerts = merged
			return
		}
	}
	payload.WatchlistAlerts = merged
}

func applyIngestReportOverlay(payload *Dashboard, summary ingest.Summary, signals ingestDerivedSignals) {
	if summary.TotalEvents == 0 {
		return
	}

	topicList := joinTopKeywords(signals.topics, 3)
	topicCount := minInt(3, len(signals.topics))
	switch {
	case topicList == "":
		payload.ReportSummary.Headline = fmt.Sprintf(
			"多类运营信号正在走高，最新 ingest 已覆盖 %d 个采集来源。",
			summary.SourceCount,
		)
	case topicCount <= 1:
		payload.ReportSummary.Headline = fmt.Sprintf(
			"%s 正在走高，最新 ingest 已覆盖 %d 个采集来源。",
			topicList,
			summary.SourceCount,
		)
	default:
		payload.ReportSummary.Headline = fmt.Sprintf(
			"%s 等 %d 组主题正在走高，最新 ingest 已覆盖 %d 个采集来源。",
			topicList,
			topicCount,
			summary.SourceCount,
		)
	}

	questionFocus := ""
	if len(signals.commentInsights) > 0 {
		questionFocus = signals.commentInsights[0].Theme
	}
	if questionFocus == "" {
		questionFocus = "高追问评论"
	}
	payload.ReportSummary.KeyFinding = fmt.Sprintf(
		"当前累计得到 %d 个视频信号、%d 条评论信号、%d 个话题信号；其中“%s”是最值得优先回答的评论入口。",
		summary.AggregatedVideos,
		summary.AggregatedComments,
		summary.AggregatedTopics,
		questionFocus,
	)
	payload.ReportSummary.RecommendedAction = "核对账号和关键词样本的指标变化、采集覆盖及评论证据。"
	if len(signals.alerts) > 0 {
		payload.ReportSummary.Watchout = fmt.Sprintf("当前仍是 P2-lite 骨架，但需要优先盯住：%s", signals.alerts[0].Summary)
	} else if signals.comment != nil && signals.comment.RepresentativeComment != "" {
		payload.ReportSummary.Watchout = fmt.Sprintf("当前仍是 P2-lite 骨架，代表性追问已经出现：%s", signals.comment.RepresentativeComment)
	} else {
		payload.ReportSummary.Watchout = "当前仍是 P2-lite 骨架，事件已接入，但热榜与评论正文尚未完全切换到正式 read model。"
	}
	payload.ReportSummary.AutoGeneratedAt = humanizeFreshness(payload.GeneratedAt, payload.GeneratedAt)
	payload.ReportSummary.GeneratedAt = payload.GeneratedAt
}

func deriveIngestSignals(summary ingest.Summary, referenceAt time.Time) ingestDerivedSignals {
	var signals ingestDerivedSignals
	topicStats := buildHotTopicMetricStats(summary.RecentEvents)
	seenVideoKeywords := map[string]struct{}{}
	seenTopicKeywords := map[string]struct{}{}
	seenCommentThemes := map[string]struct{}{}
	seenAlertIDs := map[string]struct{}{}
	var videoAlerts []WatchlistAlert
	var commentAlerts []WatchlistAlert
	var topicAlerts []WatchlistAlert
	videoEvents := make([]ingest.Event, 0, len(summary.RecentEvents))

	for _, event := range summary.RecentEvents {
		switch event.EventType {
		case "video_snapshot":
			videoEvents = append(videoEvents, event)
			if event.Title != "" {
				video := buildHotVideoFromIngestAt(summary, event, referenceAt)
				if video != nil {
					appendUniqueHotVideo(&signals.videos, seenVideoKeywords, *video)
				}
				if alert := buildWatchlistAlertFromVideoAt(summary, event, referenceAt); alert != nil {
					appendUniqueWatchlistAlert(&videoAlerts, seenAlertIDs, *alert)
				}
			}
		case "topic_cluster":
			if event.Keyword != "" {
				topic := buildHotTopicFromIngestWithStats(summary, event, topicStats)
				if topic != nil {
					appendUniqueHotTopic(&signals.topics, seenTopicKeywords, *topic)
				}
				if alert := buildWatchlistAlertFromTopicAt(summary, event, referenceAt); alert != nil {
					appendUniqueWatchlistAlert(&topicAlerts, seenAlertIDs, *alert)
				}
			}
		case "comment_snapshot":
			if signals.comment == nil {
				eventCopy := event
				signals.comment = &eventCopy
			}
			commentInsight := buildCommentInsightFromIngest(summary, event)
			if commentInsight != nil {
				appendUniqueCommentInsight(&signals.commentInsights, seenCommentThemes, *commentInsight)
			}
			if alert := buildWatchlistAlertFromCommentAt(summary, event, referenceAt); alert != nil {
				appendUniqueWatchlistAlert(&commentAlerts, seenAlertIDs, *alert)
			}
		}
	}

	if signals.comment != nil {
		if len(signals.videos) == 0 {
			if video := buildHotVideoFromComment(summary, *signals.comment, referenceAt); video != nil {
				appendUniqueHotVideo(&signals.videos, seenVideoKeywords, *video)
			}
		}
		if len(signals.topics) == 0 && signals.comment.Keyword != "" {
			if topic := buildHotTopicFromIngest(summary, *signals.comment); topic != nil {
				appendUniqueHotTopic(&signals.topics, seenTopicKeywords, *topic)
			}
		}
	}

	backfillSignalsFromVideos(summary, videoEvents, &signals, topicStats, seenTopicKeywords, seenCommentThemes)
	signals.alerts = interleaveWatchlistAlerts(videoAlerts, commentAlerts, topicAlerts)
	return signals
}

func buildHotVideoFromIngest(summary ingest.Summary, event ingest.Event) *HotVideo {
	return buildHotVideoFromIngestAt(summary, event, summary.LatestEventAt)
}

func buildHotVideoFromIngestAt(summary ingest.Summary, event ingest.Event, referenceAt time.Time) *HotVideo {
	if event.Title == "" {
		return nil
	}

	commentCount := maxInt(0, event.CommentCount)
	views := maxInt(0, event.PlayCount)
	likes := maxInt(0, event.LikeCount)
	tags := append([]string(nil), event.Tags...)
	if len(tags) == 0 && event.Keyword != "" {
		tags = append(tags, event.Keyword)
	}
	publishedAt := event.PublishedAt

	return &HotVideo{
		ID:            ingestScopedID("ingest-video", event),
		Title:         event.Title,
		ChannelName:   fallbackString(event.ChannelName, "Insight Ingest"),
		Source:        event.Source,
		ObjectID:      event.ObjectID,
		ObjectNonceID: event.ObjectNonceID,
		CoverURL:      event.CoverURL,
		Tags:          tags,
		PublishedAt:   publishedAt,
		PublishedAgo:  publishedAgo(publishedAt, referenceAt),
		Views:         views,
		Likes:         likes,
		Comments:      commentCount,
		Favorites:     maxInt(0, event.FavCount),
		Forwards:      maxInt(0, event.ForwardCount),
		Signal:        fallbackString(event.Signal, "ingest 派生"),
	}
}

func buildHotVideoFromComment(summary ingest.Summary, event ingest.Event, referenceAt time.Time) *HotVideo {
	if event.Title != "" {
		return buildHotVideoFromIngestAt(summary, event, referenceAt)
	}

	synthetic := event
	synthetic.Title = strings.TrimSpace(event.Keyword)
	if synthetic.Title == "" {
		synthetic.Title = compactIngestText(event.RepresentativeComment, 24)
	}
	if synthetic.Title == "" {
		return nil
	}
	if synthetic.Signal == "" {
		synthetic.Signal = "持续讨论"
	}
	if len(synthetic.Tags) == 0 {
		synthetic.Tags = append([]string(nil), event.Keywords...)
		if len(synthetic.Tags) == 0 && event.Keyword != "" {
			synthetic.Tags = []string{event.Keyword}
		}
	}
	return buildHotVideoFromIngestAt(summary, synthetic, referenceAt)
}

type hotTopicMetricStats struct {
	Mentions      int
	RelatedVideos int
}

func buildHotTopicMetricStats(events []ingest.Event) map[string]hotTopicMetricStats {
	stats := make(map[string]hotTopicMetricStats)
	seenVideos := make(map[string]map[string]struct{})
	for _, event := range events {
		if event.EventType != "video_snapshot" {
			continue
		}
		candidates := videoTopicCandidates(event)
		if len(candidates) == 0 {
			continue
		}

		videoKey := strings.TrimSpace(event.ObjectID)
		if videoKey == "" {
			videoKey = strings.TrimSpace(event.ID)
		}
		for _, keyword := range candidates {
			key := canonicalTopicKey(keyword)
			if key == "" {
				continue
			}
			metric := stats[key]
			metric.Mentions += maxInt(0, event.CommentCount)
			if videoKey != "" {
				if seenVideos[key] == nil {
					seenVideos[key] = make(map[string]struct{})
				}
				if _, seen := seenVideos[key][videoKey]; !seen {
					seenVideos[key][videoKey] = struct{}{}
					metric.RelatedVideos += maxInt(0, event.VideoCount)
				}
			}
			stats[key] = metric
		}
	}
	return stats
}

func buildHotTopicFromIngest(summary ingest.Summary, event ingest.Event) *HotTopic {
	return buildHotTopicFromIngestWithStats(summary, event, nil)
}

func buildHotTopicFromIngestWithStats(_ ingest.Summary, event ingest.Event, stats map[string]hotTopicMetricStats) *HotTopic {
	candidates := normalizeTopicCandidates(event.Keyword, append(append([]string(nil), event.Keywords...), event.Tags...)...)
	if len(candidates) == 0 {
		return nil
	}
	keyword := candidates[0]

	mentions := maxInt(0, event.CommentCount)
	relatedVideos := maxInt(0, event.VideoCount)
	if metric, ok := stats[canonicalTopicKey(keyword)]; ok {
		mentions = metric.Mentions
		relatedVideos = metric.RelatedVideos
	}
	keywords := normalizeTopicCandidates(keyword, append(append([]string(nil), event.Keywords...), event.Tags...)...)

	return &HotTopic{
		ID:             ingestScopedID("ingest-topic", event),
		Keyword:        keyword,
		Cluster:        fallbackString(event.Cluster, "Insight Ingest"),
		Source:         event.Source,
		ObservedAt:     event.CollectedAt,
		SourceAccounts: sourceAccountsFromEvent(event),
		Mentions:       mentions,
		RelatedVideos:  relatedVideos,
		PromptAngle:    fallbackString(event.PromptAngle, "先回答评论区实际追问，再做观点延展"),
		Keywords:       keywords,
	}
}

func buildHotTopicFromVideo(summary ingest.Summary, event ingest.Event) *HotTopic {
	return buildHotTopicFromVideoWithStats(summary, event, nil)
}

func buildHotTopicFromVideoWithStats(_ ingest.Summary, event ingest.Event, stats map[string]hotTopicMetricStats) *HotTopic {
	keyword := deriveVideoSignalKeyword(event)
	if keyword == "" {
		return nil
	}

	keywords := deriveVideoSignalKeywords(event, keyword)
	mentions := maxInt(0, event.CommentCount)
	relatedVideos := maxInt(0, event.VideoCount)
	if metric, ok := stats[canonicalTopicKey(keyword)]; ok {
		mentions = metric.Mentions
		relatedVideos = metric.RelatedVideos
	}
	return &HotTopic{
		ID:             ingestScopedID("ingest-video-topic", event),
		Keyword:        keyword,
		Cluster:        fallbackString(event.ChannelName, "视频热点"),
		Source:         event.Source,
		ObservedAt:     event.CollectedAt,
		SourceAccounts: sourceAccountsFromEvent(event),
		Mentions:       mentions,
		RelatedVideos:  relatedVideos,
		PromptAngle:    buildVideoPromptAngle(event, keyword),
		Keywords:       keywords,
	}
}

func buildCommentInsightFromIngest(summary ingest.Summary, event ingest.Event) *CommentInsight {
	if event.Keyword == "" && event.RepresentativeComment == "" {
		return nil
	}

	questionRate := minInt(88, maxInt(35, 20+event.CommentCount/4))
	theme := event.Keyword
	if theme == "" {
		theme = "评论区高追问主题"
	}

	representative := event.RepresentativeComment
	if representative == "" {
		representative = "评论区开始集中追问具体落地方式。"
	}

	actionSuggestion := "优先做一条直接回答型内容，再把高频问题沉淀成系列选题。"
	if event.Keyword != "" {
		actionSuggestion = fmt.Sprintf("围绕“%s”先做回答型内容，再继续追踪下一轮评论追问。", event.Keyword)
	}

	sentiment := "求解释"
	switch {
	case summary.AggregatedComments >= 180:
		sentiment = "追问密集"
	case event.Signal != "":
		sentiment = event.Signal
	}

	keywords := append([]string(nil), event.Keywords...)
	if len(keywords) == 0 && event.Keyword != "" {
		keywords = append(keywords, event.Keyword)
	}

	return &CommentInsight{
		ID:               fmt.Sprintf("ci-ingest-%s", event.BatchID),
		Theme:            theme,
		Source:           event.Source,
		ObservedAt:       event.CollectedAt,
		ObjectID:         event.ObjectID,
		ObjectNonceID:    event.ObjectNonceID,
		SourceAccounts:   sourceAccountsFromEvent(event),
		QuestionRate:     questionRate,
		Sentiment:        sentiment,
		Representative:   representative,
		ActionSuggestion: actionSuggestion,
		RelatedKeywords:  keywords,
	}
}

func buildCommentInsightFromVideo(summary ingest.Summary, event ingest.Event) *CommentInsight {
	keyword := deriveVideoSignalKeyword(event)
	if keyword == "" {
		return nil
	}

	representative := fmt.Sprintf("《%s》这条视频当前已积累 %d 条评论，优先检查高赞追问和复读关键词。", compactIngestText(event.Title, 24), maxInt(0, event.CommentCount))
	sentiment := fallbackString(event.Signal, "评论升温")
	if event.CommentCount >= 100 {
		sentiment = "高评论密度"
	}

	return &CommentInsight{
		ID:               ingestScopedID("ci-video", event),
		Theme:            keyword,
		Source:           event.Source,
		ObservedAt:       event.CollectedAt,
		ObjectID:         event.ObjectID,
		ObjectNonceID:    event.ObjectNonceID,
		SourceAccounts:   sourceAccountsFromEvent(event),
		QuestionRate:     minInt(82, maxInt(28, 18+event.CommentCount/3)),
		Sentiment:        sentiment,
		Representative:   representative,
		ActionSuggestion: fmt.Sprintf("围绕“%s”补一条回答型或复盘型内容，承接这条视频已经冒头的讨论。", keyword),
		RelatedKeywords:  deriveVideoSignalKeywords(event, keyword),
	}
}

func buildWatchlistAlertFromVideo(summary ingest.Summary, event ingest.Event) *WatchlistAlert {
	return buildWatchlistAlertFromVideoAt(summary, event, summary.LatestEventAt)
}

func buildWatchlistAlertFromVideoAt(summary ingest.Summary, event ingest.Event, referenceAt time.Time) *WatchlistAlert {
	if event.Title == "" {
		return nil
	}

	return &WatchlistAlert{
		ID:           ingestScopedID("alert-video", event),
		AccountName:  fallbackString(event.ChannelName, "Insight Ingest"),
		AlertType:    "新热视频",
		Source:       event.Source,
		Severity:     "high",
		Summary:      fmt.Sprintf("%s 进入高增速观察区，当前信号为 %s。", event.Title, fallbackString(event.Signal, "ingest 派生")),
		OccurredAt:   event.CollectedAt,
		HappenedAt:   humanizeFreshness(event.CollectedAt, referenceAt),
		Recommended:  "优先拆解标题、结构和评论区承接方式，判断是否值得跟进。",
		VideoCount:   maxInt(1, event.VideoCount),
		CommentDelta: event.CommentCount,
	}
}

func buildWatchlistAlertFromComment(summary ingest.Summary, event ingest.Event) *WatchlistAlert {
	return buildWatchlistAlertFromCommentAt(summary, event, summary.LatestEventAt)
}

func buildWatchlistAlertFromCommentAt(summary ingest.Summary, event ingest.Event, referenceAt time.Time) *WatchlistAlert {
	if event.Keyword == "" && event.RepresentativeComment == "" {
		return nil
	}

	summaryText := event.RepresentativeComment
	if summaryText == "" {
		summaryText = fmt.Sprintf("评论区开始集中追问 %s。", event.Keyword)
	}

	return &WatchlistAlert{
		ID:           fmt.Sprintf("alert-comment-%s", event.BatchID),
		AccountName:  fallbackString(event.TargetAudience, "Insight Ingest"),
		AlertType:    "评论爆发",
		Source:       event.Source,
		Severity:     "medium",
		Summary:      summaryText,
		OccurredAt:   event.CollectedAt,
		HappenedAt:   humanizeFreshness(event.CollectedAt, referenceAt),
		Recommended:  fmt.Sprintf("围绕“%s”优先做一条回答型内容。", fallbackString(event.Keyword, "高追问问题")),
		VideoCount:   maxInt(1, event.VideoCount),
		CommentDelta: event.CommentCount,
	}
}

func buildWatchlistAlertFromTopic(summary ingest.Summary, event ingest.Event) *WatchlistAlert {
	return buildWatchlistAlertFromTopicAt(summary, event, summary.LatestEventAt)
}

func buildWatchlistAlertFromTopicAt(summary ingest.Summary, event ingest.Event, referenceAt time.Time) *WatchlistAlert {
	if event.Keyword == "" {
		return nil
	}

	return &WatchlistAlert{
		ID:           fmt.Sprintf("alert-topic-%s", event.BatchID),
		AccountName:  fallbackString(event.Cluster, "Insight Ingest"),
		AlertType:    "主题切换",
		Source:       event.Source,
		Severity:     "low",
		Summary:      fmt.Sprintf("%s 相关讨论正在升温，适合继续观察跨账号共振。", event.Keyword),
		OccurredAt:   event.CollectedAt,
		HappenedAt:   humanizeFreshness(event.CollectedAt, referenceAt),
		Recommended:  fallbackString(event.PromptAngle, "继续观察是否值得扩成稳定栏目。"),
		VideoCount:   maxInt(1, event.VideoCount),
		CommentDelta: event.CommentCount,
	}
}

func backfillSignalsFromVideos(summary ingest.Summary, videoEvents []ingest.Event, signals *ingestDerivedSignals, topicStats map[string]hotTopicMetricStats, seenTopicKeywords, seenCommentThemes map[string]struct{}) {
	if len(videoEvents) == 0 || signals == nil {
		return
	}

	const targetCount = 5
	for _, event := range videoEvents {
		if len(signals.topics) < targetCount {
			if topic := buildHotTopicFromVideoWithStats(summary, event, topicStats); topic != nil {
				appendUniqueHotTopic(&signals.topics, seenTopicKeywords, *topic)
			}
		}
		if len(signals.commentInsights) < targetCount {
			if commentInsight := buildCommentInsightFromVideo(summary, event); commentInsight != nil {
				appendUniqueCommentInsight(&signals.commentInsights, seenCommentThemes, *commentInsight)
			}
		}
		if len(signals.topics) >= targetCount && len(signals.commentInsights) >= targetCount {
			return
		}
	}
}

func deriveVideoSignalKeyword(event ingest.Event) string {
	for _, candidate := range candidateVideoKeywords(event) {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func deriveVideoSignalKeywords(event ingest.Event, fallback string) []string {
	keywords := make([]string, 0, maxInt(1, len(event.Tags)))
	seen := map[string]struct{}{}
	appendKeyword := func(part string) {
		if part == "" {
			return
		}
		key := canonicalTopicKey(part)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		keywords = append(keywords, part)
	}
	for _, part := range candidateVideoKeywords(event) {
		appendKeyword(part)
		if len(keywords) >= 3 {
			return keywords
		}
	}
	appendKeyword(fallback)
	return keywords
}

func candidateVideoKeywords(event ingest.Event) []string {
	return videoTopicCandidates(event)
}

func videoTopicCandidates(event ingest.Event) []string {
	values := append([]string(nil), event.Tags...)
	values = append(values, event.Keywords...)
	if containsTopicHashtag(event.Title) {
		values = append(values, event.Title)
	}
	return normalizeTopicCandidates(event.Keyword, values...)
}

func buildVideoPromptAngle(event ingest.Event, keyword string) string {
	title := compactIngestText(event.Title, 24)
	if title == "" {
		return fmt.Sprintf("先解释为什么 %s 会突然冒头，再补内容结构和承接方式。", keyword)
	}
	return fmt.Sprintf("从《%s》的评论区切入，先解释为什么 %s 会突然冒头。", title, keyword)
}

func compactIngestText(value string, limit int) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 {
		return normalized
	}
	runes := []rune(normalized)
	if len(runes) <= limit {
		return normalized
	}
	return string(runes[:limit])
}

func splitVideoKeywordCandidates(value string) []string {
	return normalizeTopicCandidates(value)
}

const maxTopicCandidateRunes = 20

// normalizeTopicCandidates keeps the display value of the first valid form,
// while using canonicalTopicKey for matching and aggregation. Explicit
// hashtags take precedence over surrounding title fragments because imported
// tag arrays often contain both kinds of text.
func normalizeTopicCandidates(keyword string, values ...string) []string {
	inputs := make([]string, 0, 1+len(values))
	if strings.TrimSpace(keyword) != "" {
		inputs = append(inputs, keyword)
	}
	inputs = append(inputs, values...)
	if len(inputs) == 0 {
		return nil
	}

	hashtagCandidates := make([]string, 0, len(inputs))
	hasHashtag := false
	for _, value := range inputs {
		if containsTopicHashtag(value) {
			hasHashtag = true
			hashtagCandidates = append(hashtagCandidates, extractHashtagTokens(value)...)
		}
	}
	if hasHashtag {
		inputs = hashtagCandidates
	}

	results := make([]string, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, value := range inputs {
		normalized := normalizeTopicDisplay(value)
		if !isValidTopicCandidate(normalized) {
			continue
		}
		key := canonicalTopicKey(normalized)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		results = append(results, normalized)
	}
	return results
}

func containsTopicHashtag(value string) bool {
	return strings.ContainsRune(value, '#') || strings.ContainsRune(value, '＃')
}

func extractHashtagTokens(value string) []string {
	runes := []rune(normalizeTopicText(value))
	results := make([]string, 0, 3)
	for index := 0; index < len(runes); index++ {
		if runes[index] != '#' {
			continue
		}
		start := index + 1
		end := start
		for end < len(runes) && isTopicTokenRune(runes[end]) {
			end++
		}
		if end > start {
			results = append(results, string(runes[start:end]))
		}
		index = end - 1
	}
	return results
}

func normalizeTopicDisplay(value string) string {
	value = normalizeTopicText(value)
	value = strings.TrimSpace(value)
	value = strings.TrimFunc(value, func(r rune) bool {
		return r == '#' || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func normalizeTopicText(value string) string {
	runes := []rune(strings.TrimSpace(value))
	for index, r := range runes {
		switch {
		case r == '\u3000':
			runes[index] = ' '
		case r >= '\uff01' && r <= '\uff5e':
			runes[index] = r - ('\uff01' - '!')
		case r == '＃':
			runes[index] = '#'
		}
	}
	return string(runes)
}

func isTopicTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}

func canonicalTopicKey(value string) string {
	value = normalizeTopicText(value)
	var builder strings.Builder
	for _, r := range value {
		if isTopicTokenRune(r) {
			builder.WriteRune(unicode.ToLower(r))
		}
	}
	return builder.String()
}

func isValidTopicCandidate(value string) bool {
	if value == "" || len([]rune(value)) > maxTopicCandidateRunes {
		return false
	}
	for _, r := range value {
		if r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			continue
		}
		return false
	}
	key := canonicalTopicKey(value)
	if key == "" {
		return false
	}

	letters, digits, han := 0, 0, 0
	for _, r := range []rune(key) {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r):
			digits++
		}
		if unicode.Is(unicode.Han, r) {
			han++
		}
	}
	if letters == 0 && digits > 0 {
		return false
	}
	if letters+digits < 2 {
		return false
	}
	if digits > 0 && han == 0 && letters <= 1 {
		return false
	}
	if han >= 10 || (han >= 8 && len([]rune(strings.ReplaceAll(value, " ", ""))) >= 12) {
		return false
	}

	compact := strings.ReplaceAll(value, " ", "")
	if isNumericTopicNoise(compact) {
		return false
	}
	if strings.HasSuffix(compact, "步搞定") || strings.Contains(compact, "轻松上手") || strings.Contains(compact, "分镜和素材准备") || strings.Contains(compact, "一键搞定") {
		return false
	}
	if hasTopicSentencePrefix(compact) {
		return false
	}
	if len([]rune(compact)) >= 8 && containsAnyTopicPhrase(compact, "小白", "我用", "终于", "怎么", "如何", "为什么", "到底", "掌握", "接入", "上手", "准备", "究竟", "有什么用", "必须看懂", "引发热议", "保姆级", "教程") {
		return false
	}
	return true
}

func isNumericTopicNoise(value string) bool {
	if value == "" {
		return true
	}
	for _, r := range value {
		if unicode.IsDigit(r) {
			continue
		}
		if strings.ContainsRune("年月日时分秒万亿千百十MmkK%", r) {
			continue
		}
		return false
	}
	return true
}

func hasTopicSentencePrefix(value string) bool {
	for _, prefix := range []string{"我", "你", "他", "她", "这", "那", "从", "一个", "每", "还", "居然", "以后", "不是", "有", "为了", "以及", "再到"} {
		if strings.HasPrefix(value, prefix) && len([]rune(value)) > len([]rune(prefix))+1 {
			return true
		}
	}
	return false
}

func containsAnyTopicPhrase(value string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}

func applyIngestHotVideoOverlay(payload *Dashboard, signals ingestDerivedSignals) {
	if payload == nil || len(signals.videos) == 0 {
		return
	}
	payload.HotVideos = MergeHotVideos(signals.videos, payload.HotVideos, hotVideoRankingLimit)
}

func applyIngestHotTopicOverlay(payload *Dashboard, signals ingestDerivedSignals) {
	if payload == nil || len(signals.topics) == 0 {
		return
	}
	payload.HotTopics = MergeHotTopics(signals.topics, payload.HotTopics, hotTopicRankingLimit)
}

func applyIngestCommentInsightOverlay(payload *Dashboard, signals ingestDerivedSignals) {
	if payload == nil || len(signals.commentInsights) == 0 {
		return
	}
	payload.CommentInsights = MergeCommentInsights(signals.commentInsights, payload.CommentInsights, 5)
}

func appendUniqueHotVideo(items *[]HotVideo, seen map[string]struct{}, item HotVideo) bool {
	key := item.Title
	if key == "" {
		key = item.ID
	}
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	*items = append(*items, item)
	return true
}

func appendUniqueHotTopic(items *[]HotTopic, seen map[string]struct{}, item HotTopic) bool {
	normalized, ok := normalizeHotTopic(item)
	if !ok {
		return false
	}
	item = normalized
	key := canonicalTopicKey(item.Keyword)
	if key == "" {
		key = item.ID
	}
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	*items = append(*items, item)
	return true
}

func appendUniqueCommentInsight(items *[]CommentInsight, seen map[string]struct{}, item CommentInsight) bool {
	key := item.Theme
	if key == "" {
		key = item.ID
	}
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	*items = append(*items, item)
	return true
}

func appendUniqueWatchlistAlert(items *[]WatchlistAlert, seen map[string]struct{}, item WatchlistAlert) bool {
	key := item.ID
	if key == "" {
		key = item.Summary
	}
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	*items = append(*items, item)
	return true
}

func joinTopKeywords(items []HotTopic, limit int) string {
	if len(items) == 0 || limit <= 0 {
		return ""
	}

	values := make([]string, 0, limit)
	for _, item := range items {
		if item.Keyword == "" {
			continue
		}
		values = append(values, item.Keyword)
		if len(values) >= limit {
			break
		}
	}
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 {
		return values[0]
	}
	return fmt.Sprintf("%s、%s", values[0], values[1])
}

func interleaveWatchlistAlerts(videoAlerts, commentAlerts, topicAlerts []WatchlistAlert) []WatchlistAlert {
	maxLen := maxInt(len(videoAlerts), maxInt(len(commentAlerts), len(topicAlerts)))
	results := make([]WatchlistAlert, 0, len(videoAlerts)+len(commentAlerts)+len(topicAlerts))
	for i := 0; i < maxLen; i++ {
		if i < len(videoAlerts) {
			results = append(results, videoAlerts[i])
		}
		if i < len(commentAlerts) {
			results = append(results, commentAlerts[i])
		}
		if i < len(topicAlerts) {
			results = append(results, topicAlerts[i])
		}
	}
	return results
}
