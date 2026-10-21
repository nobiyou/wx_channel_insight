package insight

import (
	"sort"
	"strings"
	"time"
)

const (
	HotVideoRankingHot       = "hot"
	HotVideoRankingLikes     = "likes"
	HotVideoRankingComments  = "comments"
	HotVideoRankingFavorites = "favorites"
	HotVideoRankingForwards  = "forwards"
)

// HotVideoRankings contains the independently sorted views of the same real
// video candidate set. The "hot" list remains the compatibility default.
type HotVideoRankings map[string][]HotVideo

// HotVideoWindowRankings contains the same boards restricted to a publication
// time window. The reference time is the dashboard materialization time.
type HotVideoWindowRankings map[string]HotVideoRankings

const (
	HotVideoWindow24h = "24h"
	HotVideoWindow72h = "72h"
)

var hotVideoRankingWindows = []struct {
	key   string
	hours time.Duration
}{
	{key: HotVideoWindow24h, hours: 24 * time.Hour},
	{key: HotVideoWindow72h, hours: 72 * time.Hour},
}

var hotVideoRankingKinds = []string{
	HotVideoRankingHot,
	HotVideoRankingLikes,
	HotVideoRankingComments,
	HotVideoRankingFavorites,
	HotVideoRankingForwards,
}

func NormalizeHotVideoRanking(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return HotVideoRankingHot, true
	}
	for _, kind := range hotVideoRankingKinds {
		if value == kind {
			return kind, true
		}
	}
	return "", false
}

func HotVideoRankingKinds() []string {
	return append([]string(nil), hotVideoRankingKinds...)
}

func HotVideoRankingLabel(kind string) string {
	switch kind {
	case HotVideoRankingLikes:
		return "点赞榜"
	case HotVideoRankingComments:
		return "评论榜"
	case HotVideoRankingFavorites:
		return "收藏榜"
	case HotVideoRankingForwards:
		return "转发榜"
	default:
		return "综合热门"
	}
}

// BuildHotVideoRankings derives every board from the complete candidate set.
// The caller must pass all candidates, rather than an already limited list.
func BuildHotVideoRankings(items []HotVideo) HotVideoRankings {
	result := make(HotVideoRankings, len(hotVideoRankingKinds))
	for _, kind := range hotVideoRankingKinds {
		ordered := cloneHotVideos(items)
		if kind != HotVideoRankingHot {
			sort.SliceStable(ordered, func(i, j int) bool {
				return hotVideoMetricBefore(ordered[i], ordered[j], kind)
			})
		}
		if len(ordered) > hotVideoRankingLimit {
			ordered = ordered[:hotVideoRankingLimit]
		}
		result[kind] = ordered
	}
	return result
}

// BuildHotVideoWindowRankings filters the complete candidate set by actual
// publication time before deriving each board. Videos without a publication
// timestamp are intentionally excluded because observation time is not a
// substitute for the video's release time.
func BuildHotVideoWindowRankings(items []HotVideo, referenceAt time.Time) HotVideoWindowRankings {
	result := make(HotVideoWindowRankings, len(hotVideoRankingWindows))
	for _, window := range hotVideoRankingWindows {
		candidates := make([]HotVideo, 0, len(items))
		if !referenceAt.IsZero() {
			for _, item := range items {
				if item.PublishedAt.IsZero() || item.PublishedAt.After(referenceAt) {
					continue
				}
				if referenceAt.Sub(item.PublishedAt) <= window.hours {
					candidates = append(candidates, item)
				}
			}
		}
		result[window.key] = BuildHotVideoRankings(candidates)
	}
	return result
}

func hotVideoMetricBefore(left, right HotVideo, kind string) bool {
	leftValue, leftDelta := hotVideoMetric(left, kind)
	rightValue, rightDelta := hotVideoMetric(right, kind)
	if leftValue != rightValue {
		return leftValue > rightValue
	}
	if leftDelta != rightDelta {
		return leftDelta > rightDelta
	}
	if !left.ObservedAt.Equal(right.ObservedAt) {
		return left.ObservedAt.After(right.ObservedAt)
	}
	return left.ID < right.ID
}

func hotVideoMetric(item HotVideo, kind string) (int, int) {
	switch kind {
	case HotVideoRankingLikes:
		return item.Likes, item.PositiveDeltas.Likes
	case HotVideoRankingComments:
		return item.Comments, item.PositiveDeltas.Comments
	case HotVideoRankingFavorites:
		return item.Favorites, item.PositiveDeltas.Favorites
	case HotVideoRankingForwards:
		return item.Forwards, item.PositiveDeltas.Forwards
	default:
		return 0, 0
	}
}

func cloneHotVideoRankings(source HotVideoRankings) HotVideoRankings {
	if source == nil {
		return nil
	}
	result := make(HotVideoRankings, len(source))
	for kind, items := range source {
		result[kind] = cloneHotVideos(items)
	}
	return result
}

func CloneHotVideoRankings(source HotVideoRankings) HotVideoRankings {
	return cloneHotVideoRankings(source)
}

func cloneHotVideoWindowRankings(source HotVideoWindowRankings) HotVideoWindowRankings {
	if source == nil {
		return nil
	}
	result := make(HotVideoWindowRankings, len(source))
	for window, rankings := range source {
		result[window] = cloneHotVideoRankings(rankings)
	}
	return result
}

func CloneHotVideoWindowRankings(source HotVideoWindowRankings) HotVideoWindowRankings {
	return cloneHotVideoWindowRankings(source)
}

// mergeHotVideoRankings keeps newer ranking keys while preserving boards that
// are absent from an older persisted snapshot.
func mergeHotVideoRankings(primary, fallback HotVideoRankings) HotVideoRankings {
	result := cloneHotVideoRankings(fallback)
	if result == nil && primary == nil {
		return nil
	}
	if result == nil {
		result = make(HotVideoRankings)
	}
	for kind, items := range primary {
		result[kind] = cloneHotVideos(items)
	}
	return result
}

func MergeHotVideos(primary, fallback []HotVideo, limit int) []HotVideo {
	results := make([]HotVideo, 0, maxInt(len(primary), len(fallback)))
	seen := map[string]struct{}{}
	for _, item := range primary {
		appendUniqueHotVideo(&results, seen, item)
		if limit > 0 && len(results) >= limit {
			return results
		}
	}
	for _, item := range fallback {
		appendUniqueHotVideo(&results, seen, item)
		if limit > 0 && len(results) >= limit {
			return results
		}
	}
	return results
}

func MergeHotTopics(primary, fallback []HotTopic, limit int) []HotTopic {
	results := make([]HotTopic, 0, maxInt(len(primary), len(fallback)))
	seen := map[string]struct{}{}
	for _, item := range primary {
		appendUniqueHotTopic(&results, seen, item)
		if limit > 0 && len(results) >= limit {
			return results
		}
	}
	for _, item := range fallback {
		appendUniqueHotTopic(&results, seen, item)
		if limit > 0 && len(results) >= limit {
			return results
		}
	}
	return results
}

func cloneHotVideos(items []HotVideo) []HotVideo {
	results := append([]HotVideo(nil), items...)
	for i := range results {
		results[i].Tags = append([]string(nil), results[i].Tags...)
	}
	return results
}

func cloneHotTopics(items []HotTopic) []HotTopic {
	results := make([]HotTopic, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if !appendUniqueHotTopic(&results, seen, item) {
			continue
		}
		index := len(results) - 1
		results[index].Keywords = append([]string(nil), results[index].Keywords...)
		results[index].Sources = append([]string(nil), results[index].Sources...)
		results[index].SourceAccounts = append([]string(nil), results[index].SourceAccounts...)
	}
	return results
}

func CloneHotVideos(items []HotVideo) []HotVideo {
	return cloneHotVideos(items)
}

func CloneHotTopics(items []HotTopic) []HotTopic {
	return cloneHotTopics(items)
}

// normalizeHotTopic applies the same topic candidate rules to persisted and
// derived ranking items. This keeps old snapshots from bypassing ingest-time
// normalization when they are read back into a dashboard.
func normalizeHotTopic(item HotTopic) (HotTopic, bool) {
	candidates := normalizeTopicCandidates(item.Keyword, item.Keywords...)
	if len(candidates) == 0 {
		return HotTopic{}, false
	}
	item.Keyword = candidates[0]
	item.Keywords = candidates
	return item, true
}
