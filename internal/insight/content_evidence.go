package insight

import (
	"sort"
	"strings"
)

const (
	TopicEvidenceStatusAvailable   = "available"
	TopicEvidenceStatusUnavailable = "unavailable"
	recommendationEvidenceLimit    = 5
)

// videoRecommendationFromRow is the single mapping used by both the video
// ranking and topic evidence. Keeping the shapes identical prevents the topic
// pool from presenting metrics that differ from the video ranking.
func videoRecommendationFromRow(row videoHistoryRow) VideoRecommendation {
	item := row.latest
	publishedAt := item.PublishedAt
	if publishedAt.IsZero() {
		for index := len(row.history) - 1; index >= 0; index-- {
			if !row.history[index].PublishedAt.IsZero() {
				publishedAt = row.history[index].PublishedAt
				break
			}
		}
	}
	recommendationScore := row.interactionScore*interactionWeight + row.growthScore*growthWeight
	return VideoRecommendation{
		ID:                  recommendationID("video", row.identity),
		ObjectID:            item.ObjectID,
		ObjectNonceID:       item.ObjectNonceID,
		CoverURL:            item.CoverURL,
		Title:               item.Title,
		ChannelUsername:     item.ChannelUsername,
		ChannelName:         item.ChannelName,
		Keyword:             item.Keyword,
		Source:              item.Source,
		Tags:                append([]string(nil), item.Tags...),
		PublishedAt:         publishedAt,
		Metrics:             row.metrics,
		PositiveDeltas:      row.deltas,
		InteractionScore:    row.interactionScore,
		GrowthScore:         row.growthScore,
		RecommendationScore: recommendationScore,
		GrowthRate:          row.growth,
		ObservationCount:    len(row.history),
		LastObservedAt:      item.ObservedAt,
		HasGrowthBaseline:   row.previous != nil,
		InsufficientHistory: row.previous == nil,
		DataBasis:           recommendationDataBasis(),
		Reason:              recommendationReason(recommendationScore, row.deltas, row.previous != nil),
	}
}

func isUsableTopicReferenceVideo(item VideoRecommendation) bool {
	return strings.TrimSpace(item.Title) != "" && (strings.TrimSpace(item.ObjectID) != "" || strings.TrimSpace(item.ObjectNonceID) != "")
}

func topicReferenceVideos(items []VideoRecommendation) []VideoRecommendation {
	filtered := make([]VideoRecommendation, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if !isUsableTopicReferenceVideo(item) {
			continue
		}
		identity := firstNonEmpty(item.ObjectID, item.ObjectNonceID, item.ID)
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return recommendationBefore(
			filtered[i].RecommendationScore, filtered[i].LastObservedAt, filtered[i].ID,
			filtered[j].RecommendationScore, filtered[j].LastObservedAt, filtered[j].ID,
		)
	})
	if len(filtered) > recommendationEvidenceLimit {
		filtered = filtered[:recommendationEvidenceLimit]
	}
	return filtered
}

func topicEvidenceStatus(referenceVideos []VideoRecommendation) string {
	if len(referenceVideos) == 0 {
		return TopicEvidenceStatusUnavailable
	}
	return TopicEvidenceStatusAvailable
}

func matchTopicCommentEvidence(topic TopicRecommendation, comments []CommentInsight) []CommentInsight {
	if len(topic.ReferenceVideos) == 0 || len(comments) == 0 {
		return nil
	}
	ordered := append([]CommentInsight(nil), comments...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if !left.ObservedAt.Equal(right.ObservedAt) {
			return left.ObservedAt.After(right.ObservedAt)
		}
		return left.ID < right.ID
	})
	videoIDs := map[string]struct{}{}
	keywords := map[string]struct{}{}
	for _, video := range topic.ReferenceVideos {
		for _, value := range []string{video.ObjectID, video.ObjectNonceID} {
			if value = normalizeEvidenceText(value); value != "" {
				videoIDs[value] = struct{}{}
			}
		}
		for _, value := range append([]string{video.Keyword}, video.Tags...) {
			if value = canonicalTopicKey(value); value != "" {
				keywords[value] = struct{}{}
			}
		}
	}
	for _, value := range []string{topic.Keyword, topic.Cluster} {
		if value = canonicalTopicKey(value); value != "" {
			keywords[value] = struct{}{}
		}
	}

	result := make([]CommentInsight, 0, recommendationEvidenceLimit)
	seen := map[string]struct{}{}
	appendMatch := func(item CommentInsight) {
		key := normalizeEvidenceText(item.ID)
		if key == "" {
			key = normalizeEvidenceText(item.Theme + "\x00" + item.Representative)
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}

	// Stable object IDs are authoritative. Keyword matching is only the
	// secondary association path for older insight snapshots without bindings.
	for _, item := range ordered {
		objectID := normalizeEvidenceText(item.ObjectID)
		nonceID := normalizeEvidenceText(item.ObjectNonceID)
		if _, ok := videoIDs[objectID]; ok && objectID != "" {
			appendMatch(item)
		} else if _, ok := videoIDs[nonceID]; ok && nonceID != "" {
			appendMatch(item)
		}
		if len(result) == recommendationEvidenceLimit {
			return result
		}
	}
	for _, item := range ordered {
		if commentRelatedToKeywords(item, keywords) {
			appendMatch(item)
		}
		if len(result) == recommendationEvidenceLimit {
			break
		}
	}
	return result
}

func commentRelatedToKeywords(item CommentInsight, keywords map[string]struct{}) bool {
	for _, value := range item.RelatedKeywords {
		value = canonicalTopicKey(value)
		if value == "" {
			continue
		}
		if _, ok := keywords[value]; ok {
			return true
		}
	}
	return false
}

func normalizeEvidenceText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
