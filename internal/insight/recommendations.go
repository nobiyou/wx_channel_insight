package insight

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

const (
	interactionWeight = 0.4
	growthWeight      = 0.6
)

// RecommendationMetrics contains the values shown to an operator. Views are
// displayed as source facts and excluded from recommendation and ranking calculations.
type RecommendationMetrics struct {
	Views     int `json:"views"`
	Likes     int `json:"likes"`
	Comments  int `json:"comments"`
	Favorites int `json:"favorites"`
	Forwards  int `json:"forwards"`
}

type RecommendationDeltas struct {
	Likes     int `json:"likes"`
	Comments  int `json:"comments"`
	Favorites int `json:"favorites"`
	Forwards  int `json:"forwards"`
	Total     int `json:"total"`
}

// VideoGrowthWindow identifies the actual pair behind the displayed rate.
// Request bounds select observations; the latest two distinct times supply
// the comparison, which never includes views.
type VideoGrowthWindow struct {
	From               time.Time `json:"from,omitempty"`
	To                 time.Time `json:"to,omitempty"`
	BaselineObservedAt time.Time `json:"baselineObservedAt,omitempty"`
	LatestObservedAt   time.Time `json:"latestObservedAt,omitempty"`
}

type videoGrowthEvidence struct {
	Rate             *float64
	Status           string
	Basis            string
	Window           VideoGrowthWindow
	Deltas           RecommendationDeltas
	Previous         *ingest.VideoMetricObservation
	Latest           *ingest.VideoMetricObservation
	ObservationCount int
}

// A zero baseline prevents division for a percentage, but still supplies a
// valid absolute increment. Missing/corrected/single observations do not.
func hasComparableInteractionCounts(status string) bool {
	return status == "available" || status == "zero_baseline"
}

// qualifyVideoGrowth is the common factual growth owner for recommendations,
// video lists and detail. Equal-time copies are one comparison point; original
// observations remain untouched for historical display.
func qualifyVideoGrowth(history []ingest.VideoMetricObservation, from, to time.Time) videoGrowthEvidence {
	result := videoGrowthEvidence{
		Status: "insufficient_history",
		Basis:  "窗口内最近两个不同观察时刻的点赞、评论、收藏、转发总量增幅；不含播放量。",
		Window: VideoGrowthWindow{From: from, To: to},
	}
	observations := make([]ingest.VideoMetricObservation, 0, len(history))
	for _, item := range history {
		if item.ObservedAt.IsZero() || (!from.IsZero() && item.ObservedAt.Before(from)) || (!to.IsZero() && item.ObservedAt.After(to)) {
			continue
		}
		observations = append(observations, item)
	}
	sort.SliceStable(observations, func(i, j int) bool {
		if observations[i].ObservedAt.Equal(observations[j].ObservedAt) {
			return observations[i].ID < observations[j].ID
		}
		return observations[i].ObservedAt.Before(observations[j].ObservedAt)
	})
	distinct := make([]ingest.VideoMetricObservation, 0, len(observations))
	for _, item := range observations {
		if len(distinct) > 0 && distinct[len(distinct)-1].ObservedAt.Equal(item.ObservedAt) {
			distinct[len(distinct)-1] = item
		} else {
			distinct = append(distinct, item)
		}
	}
	result.ObservationCount = len(distinct)
	if len(distinct) == 0 {
		return result
	}
	latest := distinct[len(distinct)-1]
	result.Latest, result.Window.LatestObservedAt = &latest, latest.ObservedAt
	if len(distinct) < 2 {
		return result
	}
	previous := distinct[len(distinct)-2]
	result.Window.BaselineObservedAt = previous.ObservedAt
	if (latest.MissingMetrics|previous.MissingMetrics)&ingest.InteractionMetrics != 0 {
		result.Status = "metrics_unavailable"
		result.Basis = "可比观察缺少互动指标，无法计算可靠增长；未获得的指标不作为零值。"
		return result
	}
	if latest.LikeCount < previous.LikeCount || latest.CommentCount < previous.CommentCount || latest.FavCount < previous.FavCount || latest.ForwardCount < previous.ForwardCount {
		result.Status = "counter_correction"
		return result
	}
	previousMetrics := metricsFromObservation(previous)
	result.Deltas = positiveDeltas(latest, previous)
	if previousMetrics.Likes+previousMetrics.Comments+previousMetrics.Favorites+previousMetrics.Forwards == 0 {
		result.Status = "zero_baseline"
		result.Basis = "基线互动为 0，可计算两次观察的新增数量；增长率分母为 0，百分比不可用。"
		return result
	}
	result.Status, result.Previous = "available", &previous
	rate := growthRate(metricsFromObservation(latest), previousMetrics, result.Deltas)
	result.Rate = &rate
	return result
}

type RecommendationEvidence struct {
	ObservationCount    int                  `json:"observationCount"`
	HasGrowthBaseline   bool                 `json:"hasGrowthBaseline"`
	InsufficientHistory bool                 `json:"insufficientHistory"`
	LastObservedAt      time.Time            `json:"lastObservedAt,omitempty"`
	GrowthRate          float64              `json:"growthRate"`
	PositiveDeltas      RecommendationDeltas `json:"positiveDeltas"`
	Source              string               `json:"source"`
	DataBasis           string               `json:"dataBasis"`
}

type AccountRecommendation struct {
	ID                  string                `json:"id"`
	Username            string                `json:"username"`
	ChannelName         string                `json:"channelName"`
	Source              string                `json:"source"`
	Sources             []string              `json:"sources"`
	VideoCount          int                   `json:"videoCount"`
	Metrics             RecommendationMetrics `json:"metrics"`
	PositiveDeltas      RecommendationDeltas  `json:"positiveDeltas"`
	InteractionScore    float64               `json:"interactionScore"`
	GrowthScore         float64               `json:"growthScore"`
	RecommendationScore float64               `json:"recommendationScore"`
	GrowthRate          float64               `json:"growthRate"`
	ObservationCount    int                   `json:"observationCount"`
	LastObservedAt      time.Time             `json:"lastObservedAt,omitempty"`
	HasGrowthBaseline   bool                  `json:"hasGrowthBaseline"`
	InsufficientHistory bool                  `json:"insufficientHistory"`
	DataBasis           string                `json:"dataBasis"`
	Reason              string                `json:"reason"`
	CanMonitor          bool                  `json:"canMonitor"`
}

type VideoRecommendation struct {
	ID                  string                `json:"id"`
	ObjectID            string                `json:"objectId,omitempty"`
	ObjectNonceID       string                `json:"objectNonceId,omitempty"`
	CoverURL            string                `json:"coverUrl,omitempty"`
	Title               string                `json:"title"`
	ChannelUsername     string                `json:"channelUsername,omitempty"`
	ChannelName         string                `json:"channelName"`
	Keyword             string                `json:"keyword,omitempty"`
	Source              string                `json:"source"`
	Tags                []string              `json:"tags,omitempty"`
	PublishedAt         time.Time             `json:"publishedAt,omitempty"`
	Metrics             RecommendationMetrics `json:"metrics"`
	PositiveDeltas      RecommendationDeltas  `json:"positiveDeltas"`
	InteractionScore    float64               `json:"interactionScore"`
	GrowthScore         float64               `json:"growthScore"`
	RecommendationScore float64               `json:"recommendationScore"`
	GrowthRate          float64               `json:"growthRate"`
	ObservationCount    int                   `json:"observationCount"`
	LastObservedAt      time.Time             `json:"lastObservedAt,omitempty"`
	HasGrowthBaseline   bool                  `json:"hasGrowthBaseline"`
	InsufficientHistory bool                  `json:"insufficientHistory"`
	DataBasis           string                `json:"dataBasis"`
	Reason              string                `json:"reason"`
}

type TopicRecommendation struct {
	ID                  string                `json:"id"`
	Keyword             string                `json:"keyword"`
	Cluster             string                `json:"cluster"`
	Source              string                `json:"source"`
	Sources             []string              `json:"sources"`
	SourceAccounts      []string              `json:"sourceAccounts"`
	RelatedVideos       int                   `json:"relatedVideos"`
	Metrics             RecommendationMetrics `json:"metrics"`
	Mentions            int                   `json:"mentions"`
	PositiveDeltas      RecommendationDeltas  `json:"positiveDeltas"`
	InteractionScore    float64               `json:"interactionScore"`
	GrowthScore         float64               `json:"growthScore"`
	RecommendationScore float64               `json:"recommendationScore"`
	GrowthRate          float64               `json:"growthRate"`
	ObservationCount    int                   `json:"observationCount"`
	LastObservedAt      time.Time             `json:"lastObservedAt,omitempty"`
	HasGrowthBaseline   bool                  `json:"hasGrowthBaseline"`
	InsufficientHistory bool                  `json:"insufficientHistory"`
	DataBasis           string                `json:"dataBasis"`
	Reason              string                `json:"reason"`
	ReferenceVideos     []VideoRecommendation `json:"referenceVideos,omitempty"`
	CommentEvidence     []CommentInsight      `json:"commentEvidence,omitempty"`
	EvidenceStatus      string                `json:"evidenceStatus"`
}

type RecommendationSnapshot struct {
	WorkspaceID            string                  `json:"workspaceId"`
	HubUserID              string                  `json:"hubUserId"`
	LatestBatchID          string                  `json:"latestBatchId"`
	GeneratedAt            time.Time               `json:"generatedAt"`
	AccountRecommendations []AccountRecommendation `json:"accountRecommendations"`
	VideoRecommendations   []VideoRecommendation   `json:"videoRecommendations"`
	TopicRecommendations   []TopicRecommendation   `json:"topicRecommendations"`
}

type videoHistoryRow struct {
	identity         string
	history          []ingest.VideoMetricObservation
	latest           ingest.VideoMetricObservation
	previous         *ingest.VideoMetricObservation
	metrics          RecommendationMetrics
	deltas           RecommendationDeltas
	rawScore         float64
	growth           float64
	growthScoreRaw   float64
	interactionScore float64
	growthScore      float64
}

type topicHistoryRow struct {
	key              string
	keyword          string
	cluster          string
	source           string
	sources          map[string]struct{}
	sourceAccounts   map[string]struct{}
	metrics          RecommendationMetrics
	previousMetrics  RecommendationMetrics
	deltas           RecommendationDeltas
	relatedVideos    int
	observationCount int
	lastObservedAt   time.Time
	hasBaseline      bool
	rawScore         float64
	growthScoreRaw   float64
	growthRate       float64
	mentions         int
	referenceVideos  []VideoRecommendation
}

// BuildRecommendationSnapshot derives all recommendation and hot-topic data
// from raw ingest events and append-only metric observations.
func BuildRecommendationSnapshot(workspaceID, hubUserID string, events []ingest.Event, observations []ingest.VideoMetricObservation, bindings []ingest.SourceBinding, now time.Time) RecommendationSnapshot {
	if now.IsZero() {
		now = time.Now()
	}
	observations = mergeEventObservations(events, observations)
	histories := groupVideoHistories(observations)
	videoRows := make([]videoHistoryRow, 0, len(histories))
	for identity, history := range histories {
		if len(history) == 0 {
			continue
		}
		row := videoHistoryRow{identity: identity, history: history, latest: history[len(history)-1]}
		growth := qualifyVideoGrowth(history, time.Time{}, time.Time{})
		row.metrics = metricsFromObservation(row.latest)
		row.rawScore = interactionRawScore(row.metrics)
		if growth.Rate != nil {
			// This recommendation score requires a relative-growth baseline.
			row.previous, row.deltas = growth.Previous, growth.Deltas
			row.growth = *growth.Rate
		}
		videoRows = append(videoRows, row)
	}

	scoreRows(videoRows)
	monitored := monitoredFeedUsernames(bindings)
	accounts := buildAccountRecommendations(videoRows, monitored)
	videos := buildVideoRecommendations(videoRows)
	topics := buildTopicRecommendations(videoRows, events)

	latestBatchID := ""
	for _, event := range events {
		if strings.TrimSpace(event.BatchID) != "" && event.CollectedAt.After(now.Add(-100*365*24*time.Hour)) {
			if latestBatchID == "" || event.CollectedAt.After(latestEventAt(events, latestBatchID)) {
				latestBatchID = strings.TrimSpace(event.BatchID)
			}
		}
	}
	return RecommendationSnapshot{
		WorkspaceID:            workspaceID,
		HubUserID:              hubUserID,
		LatestBatchID:          latestBatchID,
		GeneratedAt:            now,
		AccountRecommendations: accounts,
		VideoRecommendations:   videos,
		TopicRecommendations:   topics,
	}
}

func mergeEventObservations(events []ingest.Event, observations []ingest.VideoMetricObservation) []ingest.VideoMetricObservation {
	result := append([]ingest.VideoMetricObservation(nil), observations...)
	eventIndexes := map[string][]int{}
	for index, observation := range result {
		if eventID := strings.TrimSpace(observation.EventID); eventID != "" {
			eventIndexes[eventID] = append(eventIndexes[eventID], index)
		}
	}
	for _, event := range events {
		if event.EventType != "video_snapshot" || !isRecommendationVideoSource(event.Source) || strings.TrimSpace(event.ID) == "" {
			continue
		}
		if indexes, exists := eventIndexes[event.ID]; exists {
			// The event row is the latest ingest state. Preserve its known
			// immutable publication time when a later append-only observation
			// omitted that field.
			if !event.PublishedAt.IsZero() {
				for _, index := range indexes {
					if result[index].PublishedAt.IsZero() {
						result[index].PublishedAt = event.PublishedAt
					}
				}
			}
			continue
		}
		result = append(result, ingest.VideoMetricObservation{
			ID:              event.ID,
			HubUserID:       event.HubUserID,
			EventID:         event.ID,
			BatchID:         event.BatchID,
			Source:          event.Source,
			ObservedAt:      event.CollectedAt,
			PublishedAt:     event.PublishedAt,
			ObjectID:        event.ObjectID,
			ObjectNonceID:   event.ObjectNonceID,
			CoverURL:        event.CoverURL,
			ChannelUsername: event.ChannelUsername,
			Title:           event.Title,
			ChannelName:     event.ChannelName,
			Keyword:         event.Keyword,
			Tags:            append([]string(nil), event.Tags...),
			PlayCount:       event.PlayCount,
			LikeCount:       event.LikeCount,
			CommentCount:    event.CommentCount,
			FavCount:        event.FavCount,
			ForwardCount:    event.ForwardCount,
		})
		eventIndexes[event.ID] = []int{len(result) - 1}
	}
	return result
}

func groupVideoHistories(observations []ingest.VideoMetricObservation) map[string][]ingest.VideoMetricObservation {
	result := map[string][]ingest.VideoMetricObservation{}
	for _, observation := range observations {
		if !isRecommendationVideoSource(observation.Source) {
			continue
		}
		identity := videoIdentity(observation)
		if identity == "" {
			continue
		}
		result[identity] = append(result[identity], observation)
	}
	for identity := range result {
		sort.SliceStable(result[identity], func(i, j int) bool {
			left, right := result[identity][i], result[identity][j]
			if left.ObservedAt.Equal(right.ObservedAt) {
				return left.ID < right.ID
			}
			return left.ObservedAt.Before(right.ObservedAt)
		})
	}
	return result
}

func videoIdentity(observation ingest.VideoMetricObservation) string {
	for _, value := range []string{observation.ObjectID, observation.ObjectNonceID, observation.EventID, observation.ID} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func scoreRows(rows []videoHistoryRow) {
	maxInteraction, maxGrowth := 0.0, 0.0
	for i := range rows {
		if rows[i].rawScore > maxInteraction {
			maxInteraction = rows[i].rawScore
		}
		rows[i].growthScoreRaw = growthRawScore(rows[i].deltas)
		if rows[i].growthScoreRaw > maxGrowth {
			maxGrowth = rows[i].growthScoreRaw
		}
	}
	for i := range rows {
		rows[i].interactionScore = 0
		rows[i].growthScore = 0
		if maxInteraction > 0 {
			rows[i].interactionScore = rows[i].rawScore / maxInteraction
		}
		if maxGrowth > 0 {
			rows[i].growthScore = rows[i].growthScoreRaw / maxGrowth
		}
	}
}

func buildAccountRecommendations(rows []videoHistoryRow, monitored map[string]struct{}) []AccountRecommendation {
	type accountRow struct {
		username, channelName, source string
		sources                       map[string]struct{}
		videoIdentities               map[string]struct{}
		metrics                       RecommendationMetrics
		previousMetrics               RecommendationMetrics
		deltas                        RecommendationDeltas
		observationCount              int
		lastObservedAt                time.Time
		hasBaseline                   bool
		rawScore, growthRaw           float64
	}
	grouped := map[string]*accountRow{}
	for _, row := range rows {
		username := strings.TrimSpace(row.latest.ChannelUsername)
		if username == "" || isMonitoredUsername(username, monitored) {
			continue
		}
		item := grouped[strings.ToLower(username)]
		if item == nil {
			item = &accountRow{
				username:        username,
				channelName:     strings.TrimSpace(row.latest.ChannelName),
				sources:         map[string]struct{}{},
				videoIdentities: map[string]struct{}{},
			}
			grouped[strings.ToLower(username)] = item
		}
		if item.channelName == "" {
			item.channelName = strings.TrimSpace(row.latest.ChannelName)
		}
		item.sources[row.latest.Source] = struct{}{}
		item.videoIdentities[row.identity] = struct{}{}
		item.metrics = addMetrics(item.metrics, row.metrics)
		if row.previous != nil {
			item.previousMetrics = addMetrics(item.previousMetrics, metricsFromObservation(*row.previous))
		}
		item.deltas = addDeltas(item.deltas, row.deltas)
		item.observationCount += len(row.history)
		item.hasBaseline = item.hasBaseline || row.previous != nil
		if row.latest.ObservedAt.After(item.lastObservedAt) {
			item.lastObservedAt = row.latest.ObservedAt
			item.source = row.latest.Source
		}
		item.rawScore += row.rawScore
		item.growthRaw += growthRawScore(row.deltas)
	}
	rowsForScore := make([]*accountRow, 0, len(grouped))
	for _, row := range grouped {
		rowsForScore = append(rowsForScore, row)
	}
	maxInteraction, maxGrowth := 0.0, 0.0
	for _, row := range rowsForScore {
		maxInteraction = math.Max(maxInteraction, row.rawScore)
		maxGrowth = math.Max(maxGrowth, row.growthRaw)
	}
	results := make([]AccountRecommendation, 0, len(rowsForScore))
	for _, row := range rowsForScore {
		interaction, growth := normalized(row.rawScore, maxInteraction), normalized(row.growthRaw, maxGrowth)
		growthRate := aggregateGrowthRate(row.previousMetrics, row.deltas, row.hasBaseline)
		sources := sortedKeys(row.sources)
		recommendationScore := interaction*interactionWeight + growth*growthWeight
		results = append(results, AccountRecommendation{
			ID:                  recommendationID("account", row.username),
			Username:            row.username,
			ChannelName:         row.channelName,
			Source:              row.source,
			Sources:             sources,
			VideoCount:          len(row.videoIdentities),
			Metrics:             row.metrics,
			PositiveDeltas:      row.deltas,
			InteractionScore:    interaction,
			GrowthScore:         growth,
			RecommendationScore: recommendationScore,
			GrowthRate:          growthRate,
			ObservationCount:    row.observationCount,
			LastObservedAt:      row.lastObservedAt,
			HasGrowthBaseline:   row.hasBaseline,
			InsufficientHistory: !row.hasBaseline,
			DataBasis:           recommendationDataBasis(),
			Reason:              recommendationReason(recommendationScore, row.deltas, row.hasBaseline),
			CanMonitor:          true,
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		return recommendationBefore(results[i].RecommendationScore, results[i].LastObservedAt, results[i].ID, results[j].RecommendationScore, results[j].LastObservedAt, results[j].ID)
	})
	return results
}

func buildVideoRecommendations(rows []videoHistoryRow) []VideoRecommendation {
	results := make([]VideoRecommendation, 0, len(rows))
	for _, row := range rows {
		results = append(results, videoRecommendationFromRow(row))
	}
	sort.SliceStable(results, func(i, j int) bool {
		return recommendationBefore(results[i].RecommendationScore, results[i].LastObservedAt, results[i].ID, results[j].RecommendationScore, results[j].LastObservedAt, results[j].ID)
	})
	return results
}

func buildTopicRecommendations(rows []videoHistoryRow, events []ingest.Event) []TopicRecommendation {
	grouped := map[string]*topicHistoryRow{}
	for _, row := range rows {
		for _, topic := range observationTopics(row.latest) {
			key := canonicalTopicKey(topic)
			if key == "" {
				continue
			}
			item := grouped[key]
			if item == nil {
				item = &topicHistoryRow{key: key, keyword: strings.TrimSpace(topic), sources: map[string]struct{}{}, sourceAccounts: map[string]struct{}{}}
				grouped[key] = item
			}
			if source := strings.TrimSpace(row.latest.Source); source != "" {
				item.sources[source] = struct{}{}
			}
			item.metrics = addMetrics(item.metrics, row.metrics)
			if row.previous != nil {
				item.previousMetrics = addMetrics(item.previousMetrics, metricsFromObservation(*row.previous))
			}
			item.deltas = addDeltas(item.deltas, row.deltas)
			item.relatedVideos++
			item.observationCount += len(row.history)
			item.hasBaseline = item.hasBaseline || row.previous != nil
			if row.latest.ObservedAt.After(item.lastObservedAt) {
				item.lastObservedAt = row.latest.ObservedAt
				item.source = row.latest.Source
			}
			if account := strings.TrimSpace(row.latest.ChannelUsername); account != "" {
				item.sourceAccounts[account] = struct{}{}
			} else if account := strings.TrimSpace(row.latest.ChannelName); account != "" {
				item.sourceAccounts[account] = struct{}{}
			}
			item.rawScore += interactionRawScore(row.metrics)
			item.growthScoreRaw += growthRawScore(row.deltas)
			item.referenceVideos = append(item.referenceVideos, videoRecommendationFromRow(row))
		}
	}
	for _, event := range events {
		if event.EventType == "topic_cluster" {
			addTopicClusterRecommendation(grouped, event)
			continue
		}
		if !isRealCommentEvent(event) || strings.TrimSpace(event.Keyword) == "" {
			continue
		}
		for _, keyword := range normalizeTopicCandidates(event.Keyword, append(append([]string(nil), event.Keywords...), event.Tags...)...) {
			addCommentTopicRecommendation(grouped, event, keyword)
		}
	}
	rowsForScore := make([]*topicHistoryRow, 0, len(grouped))
	for _, row := range grouped {
		rowsForScore = append(rowsForScore, row)
	}
	maxInteraction, maxGrowth := 0.0, 0.0
	for _, row := range rowsForScore {
		maxInteraction = math.Max(maxInteraction, interactionRawScore(row.metrics))
		maxGrowth = math.Max(maxGrowth, row.growthScoreRaw)
	}
	results := make([]TopicRecommendation, 0, len(rowsForScore))
	for _, row := range rowsForScore {
		interaction := normalized(interactionRawScore(row.metrics), maxInteraction)
		growth := normalized(row.growthScoreRaw, maxGrowth)
		mentions := maxInt(row.metrics.Comments, row.mentions)
		referenceVideos := topicReferenceVideos(row.referenceVideos)
		results = append(results, TopicRecommendation{
			ID:                  recommendationID("topic", row.key),
			Keyword:             row.keyword,
			Cluster:             row.cluster,
			Source:              row.source,
			Sources:             sortedKeys(row.sources),
			SourceAccounts:      sortedKeys(row.sourceAccounts),
			RelatedVideos:       row.relatedVideos,
			Metrics:             row.metrics,
			Mentions:            mentions,
			PositiveDeltas:      row.deltas,
			InteractionScore:    interaction,
			GrowthScore:         growth,
			RecommendationScore: interaction*interactionWeight + growth*growthWeight,
			ObservationCount:    row.observationCount,
			LastObservedAt:      row.lastObservedAt,
			HasGrowthBaseline:   row.hasBaseline,
			InsufficientHistory: !row.hasBaseline,
			DataBasis:           topicRecommendationDataBasis(row.observationCount),
			Reason:              recommendationReason(interaction*interactionWeight+growth*growthWeight, row.deltas, row.hasBaseline),
			GrowthRate:          aggregateGrowthRate(row.previousMetrics, row.deltas, row.hasBaseline),
			ReferenceVideos:     referenceVideos,
			EvidenceStatus:      topicEvidenceStatus(referenceVideos),
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		return recommendationBefore(results[i].RecommendationScore, results[i].LastObservedAt, results[i].ID, results[j].RecommendationScore, results[j].LastObservedAt, results[j].ID)
	})
	return results
}

func addTopicClusterRecommendation(grouped map[string]*topicHistoryRow, event ingest.Event) {
	for _, keyword := range normalizeTopicCandidates(event.Keyword, append(append([]string(nil), event.Keywords...), event.Tags...)...) {
		addTopicClusterRecommendationForKeyword(grouped, event, keyword)
	}
}

func addTopicClusterRecommendationForKeyword(grouped map[string]*topicHistoryRow, event ingest.Event, keyword string) {
	key := canonicalTopicKey(keyword)
	if key == "" {
		return
	}
	item := grouped[key]
	if item == nil {
		item = &topicHistoryRow{key: key, keyword: keyword, sources: map[string]struct{}{}, sourceAccounts: map[string]struct{}{}}
		grouped[key] = item
	}
	if source := strings.TrimSpace(event.Source); source != "" {
		item.sources[source] = struct{}{}
		if item.source == "" || event.CollectedAt.After(item.lastObservedAt) {
			item.source = source
		}
	}
	item.metrics.Comments += nonNegative(event.CommentCount)
	item.mentions += nonNegative(event.TopicCount)
	item.relatedVideos += nonNegative(event.VideoCount)
	item.observationCount++
	if event.CollectedAt.After(item.lastObservedAt) {
		item.lastObservedAt = event.CollectedAt
	}
	if item.cluster == "" {
		item.cluster = strings.TrimSpace(event.Cluster)
	}
	if account := strings.TrimSpace(event.ChannelUsername); account != "" {
		item.sourceAccounts[account] = struct{}{}
	}
	item.rawScore = interactionRawScore(item.metrics)
}

func observationTopics(item ingest.VideoMetricObservation) []string {
	values := append([]string(nil), item.Tags...)
	if containsTopicHashtag(item.Title) {
		values = append(values, item.Title)
	}
	return normalizeTopicCandidates(item.Keyword, values...)
}

func addCommentTopicRecommendation(grouped map[string]*topicHistoryRow, event ingest.Event, keyword string) {
	key := canonicalTopicKey(keyword)
	if key == "" {
		return
	}
	item := grouped[key]
	if item == nil {
		item = &topicHistoryRow{key: key, keyword: keyword, sources: map[string]struct{}{}, sourceAccounts: map[string]struct{}{}}
		grouped[key] = item
	}
	if source := strings.TrimSpace(event.Source); source != "" {
		item.sources[source] = struct{}{}
	}
	item.metrics.Comments += nonNegative(event.CommentCount)
	item.mentions += nonNegative(event.CommentCount)
	item.observationCount++
	if event.CollectedAt.After(item.lastObservedAt) {
		item.lastObservedAt = event.CollectedAt
		item.source = event.Source
	}
	if item.cluster == "" {
		item.cluster = strings.TrimSpace(event.Cluster)
	}
	if account := strings.TrimSpace(event.ChannelUsername); account != "" {
		item.sourceAccounts[account] = struct{}{}
	}
}

func metricsFromObservation(item ingest.VideoMetricObservation) RecommendationMetrics {
	return RecommendationMetrics{
		Views:     nonNegative(item.PlayCount),
		Likes:     nonNegative(item.LikeCount),
		Comments:  nonNegative(item.CommentCount),
		Favorites: nonNegative(item.FavCount),
		Forwards:  nonNegative(item.ForwardCount),
	}
}

func addMetrics(left, right RecommendationMetrics) RecommendationMetrics {
	return RecommendationMetrics{
		Views:     left.Views + right.Views,
		Likes:     left.Likes + right.Likes,
		Comments:  left.Comments + right.Comments,
		Favorites: left.Favorites + right.Favorites,
		Forwards:  left.Forwards + right.Forwards,
	}
}

func positiveDeltas(current, previous ingest.VideoMetricObservation) RecommendationDeltas {
	result := RecommendationDeltas{
		Likes:     positiveDelta(current.LikeCount, previous.LikeCount),
		Comments:  positiveDelta(current.CommentCount, previous.CommentCount),
		Favorites: positiveDelta(current.FavCount, previous.FavCount),
		Forwards:  positiveDelta(current.ForwardCount, previous.ForwardCount),
	}
	result.Total = result.Likes + result.Comments + result.Favorites + result.Forwards
	return result
}

func addDeltas(left, right RecommendationDeltas) RecommendationDeltas {
	result := RecommendationDeltas{
		Likes:     left.Likes + right.Likes,
		Comments:  left.Comments + right.Comments,
		Favorites: left.Favorites + right.Favorites,
		Forwards:  left.Forwards + right.Forwards,
	}
	result.Total = result.Likes + result.Comments + result.Favorites + result.Forwards
	return result
}

func positiveDelta(current, previous int) int {
	if current <= previous {
		return 0
	}
	return current - previous
}

func interactionRawScore(metrics RecommendationMetrics) float64 {
	return math.Log1p(float64(metrics.Likes)) + math.Log1p(float64(metrics.Comments)) + math.Log1p(float64(metrics.Favorites)) + math.Log1p(float64(metrics.Forwards))
}

func growthRawScore(deltas RecommendationDeltas) float64 {
	return math.Log1p(float64(deltas.Likes)) + math.Log1p(float64(deltas.Comments)) + math.Log1p(float64(deltas.Favorites)) + math.Log1p(float64(deltas.Forwards))
}

func growthRate(current, previous RecommendationMetrics, deltas RecommendationDeltas) float64 {
	if previous.Likes+previous.Comments+previous.Favorites+previous.Forwards <= 0 {
		return 0
	}
	return float64(deltas.Total) / float64(previous.Likes+previous.Comments+previous.Favorites+previous.Forwards) * 100
}

func aggregateGrowthRate(previous RecommendationMetrics, deltas RecommendationDeltas, baseline bool) float64 {
	if !baseline {
		return 0
	}
	previousTotal := previous.Likes + previous.Comments + previous.Favorites + previous.Forwards
	if deltas.Total == 0 || previousTotal == 0 {
		return 0
	}
	return float64(deltas.Total) / float64(previousTotal) * 100
}

func normalized(value, maximum float64) float64 {
	if maximum <= 0 || value <= 0 {
		return 0
	}
	return value / maximum
}

func monitoredFeedUsernames(bindings []ingest.SourceBinding) map[string]struct{} {
	result := map[string]struct{}{}
	for _, binding := range bindings {
		source := strings.TrimSpace(binding.Source)
		if source != ingest.RunSourceWXFeedList && source != "wx_channel_feed_list" {
			continue
		}
		if username := strings.ToLower(strings.TrimSpace(binding.Username)); username != "" {
			result[username] = struct{}{}
		}
	}
	return result
}

func isMonitoredUsername(username string, monitored map[string]struct{}) bool {
	_, exists := monitored[strings.ToLower(strings.TrimSpace(username))]
	return exists
}

func isRecommendationVideoSource(source string) bool {
	switch strings.TrimSpace(source) {
	case ingest.RunSourceWXFeedList, "wx_channel_feed_list", ingest.RunSourceWXVideoSearch, "wx_channel_video_search", ingest.RunSourceWXManualShare, ingest.RunSourceWXVideoProfile:
		return true
	default:
		return false
	}
}

func isRealCommentEvent(event ingest.Event) bool {
	return event.EventType == "comment_snapshot" && strings.TrimSpace(event.Source) != "" && strings.Contains(strings.ToLower(event.Source), "comment")
}

func recommendationDataBasis() string {
	return "真实 ingest 视频、话题或评论观察；点赞、评论、收藏、转发及历史正向增量；播放/阅读数未参与评分"
}

func topicRecommendationDataBasis(observationCount int) string {
	if observationCount == 0 {
		return "尚无真实采集证据，暂不参与趋势榜评分"
	}
	return recommendationDataBasis()
}

func recommendationReason(score float64, deltas RecommendationDeltas, hasBaseline bool) string {
	if score == 0 && deltas.Total == 0 && !hasBaseline {
		return "尚无足够互动或增长证据，暂不参与趋势评分。"
	}
	if !hasBaseline {
		return "有真实互动指标，但暂无上一条历史观察，增长分暂为 0。"
	}
	if deltas.Total <= 0 {
		return "有历史观察，但四项互动指标没有正向增量。"
	}
	return "四项互动指标存在正向增量，增长因素按 60% 权重参与推荐。"
}

func recommendationID(kind, value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		value = "unknown"
	}
	value = strings.NewReplacer(" ", "-", "\t", "-", "\n", "-", "/", "-", "\\", "-").Replace(value)
	return kind + "-" + value
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func recommendationBefore(leftScore float64, leftAt time.Time, leftID string, rightScore float64, rightAt time.Time, rightID string) bool {
	if leftScore != rightScore {
		return leftScore > rightScore
	}
	if !leftAt.Equal(rightAt) {
		return leftAt.After(rightAt)
	}
	return leftID < rightID
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func latestEventAt(events []ingest.Event, batchID string) time.Time {
	latest := time.Time{}
	for _, event := range events {
		if strings.TrimSpace(event.BatchID) == batchID && event.CollectedAt.After(latest) {
			latest = event.CollectedAt
		}
	}
	return latest
}
