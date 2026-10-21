package insight

import (
	"fmt"
	"sort"
	"time"
)

type VideoPerformance struct {
	RecentHeat         VideoRecentHeat    `json:"recentHeat"`
	KeywordRanks       []VideoKeywordRank `json:"keywordRanks"`
	Tier               string             `json:"tier"`
	Reason             string             `json:"reason"`
	Interactions       *int               `json:"interactions"`
	RelativeToMedian   *float64           `json:"relativeToMedian"`
	MedianInteractions *float64           `json:"medianInteractions"`
	PeerCount          int                `json:"peerCount"`
	AgeBand            string             `json:"ageBand"`
	GrowthPerHour      *float64           `json:"growthPerHour"`
	GrowthInteractions *int               `json:"growthInteractions"`
	Freshness          string             `json:"freshness"`
}

func completeInteractions(row DataCenterItem) *int {
	if row.Likes == nil || row.Comments == nil || row.Favorites == nil || row.Forwards == nil {
		return nil
	}
	value := *row.Likes + *row.Comments + *row.Favorites + *row.Forwards
	return &value
}

func videoAgeBand(row DataCenterItem) string {
	if row.PublishedAt.IsZero() || row.ObservedAt.IsZero() || row.PublishedAt.After(row.ObservedAt) {
		return ""
	}
	age := row.ObservedAt.Sub(row.PublishedAt)
	switch {
	case age < 24*time.Hour:
		return "发布 24 小时内"
	case age < 3*24*time.Hour:
		return "发布 1–3 天"
	case age < 7*24*time.Hour:
		return "发布 3–7 天"
	case age < 30*24*time.Hour:
		return "发布 7–30 天"
	default:
		return "发布 30 天以上"
	}
}

// The baseline is fixed before curation/quality filters and pagination. Compare
// only the same account and publication-age band; it is never a site-wide rank.
func assessVideoPerformance(rows []DataCenterItem, reference time.Time) {
	cohorts := map[string][]int{}
	for i := range rows {
		row := &rows[i]
		p := VideoPerformance{Tier: "insufficient_data", Interactions: completeInteractions(*row), AgeBand: videoAgeBand(*row), Freshness: "unknown", Reason: "缺少发布时间或账号身份，暂不分层"}
		if !row.ObservedAt.IsZero() && !row.ObservedAt.After(reference.Add(5*time.Minute)) {
			p.Freshness = "fresh"
			if reference.Sub(row.ObservedAt) > 24*time.Hour {
				p.Freshness = "stale"
			}
		}
		if p.Interactions == nil {
			p.Reason = "互动指标未采全，不按零值判定表现"
		}
		if p.Interactions != nil && p.AgeBand != "" && row.ChannelUsername != "" {
			key := row.ChannelUsername + "|" + p.AgeBand
			cohorts[key] = append(cohorts[key], *p.Interactions)
		}
		// Reuse the qualified two-observation window; normalize its actual
		// elapsed time, rather than treating differently spaced polls equally.
		span := row.GrowthWindow.LatestObservedAt.Sub(row.GrowthWindow.BaselineObservedAt)
		if hasComparableInteractionCounts(row.GrowthStatus) && row.growthInteractions != nil && span >= 10*time.Minute && span <= 72*time.Hour {
			speed := float64(*row.growthInteractions) / span.Hours()
			p.GrowthInteractions = cloneOptionalInt(row.growthInteractions)
			p.GrowthPerHour = &speed
		}
		row.Performance = p
		row.Performance.RecentHeat = assessRecentHeat(*row, reference)
	}
	medians := map[string]float64{}
	for key, values := range cohorts {
		sort.Ints(values)
		mid := len(values) / 2
		median := float64(values[mid])
		if len(values)%2 == 0 {
			median = (float64(values[mid-1]) + float64(values[mid])) / 2
		}
		medians[key] = median
	}
	for i := range rows {
		row := &rows[i]
		p := &row.Performance
		if p.Interactions == nil || p.AgeBand == "" || row.ChannelUsername == "" {
			continue
		}
		key := row.ChannelUsername + "|" + p.AgeBand
		p.PeerCount = len(cohorts[key])
		median := medians[key]
		p.MedianInteractions = &median
		if p.PeerCount < 5 {
			p.Reason = fmt.Sprintf("同账号同发布龄仅 %d 条完整样本，至少 5 条才分层", p.PeerCount)
			continue
		}
		if median <= 0 {
			p.Reason = "同组互动中位数为零，暂不计算表现倍数"
			continue
		}
		ratio := float64(*p.Interactions) / median
		p.RelativeToMedian = &ratio
		p.Tier = "typical"
		p.Reason = fmt.Sprintf("同账号 · %s · %d 条完整样本，互动为中位数 %.2f 倍", p.AgeBand, p.PeerCount, ratio)
		if ratio >= 2 && *p.Interactions >= 100 {
			p.Tier = "standout"
		}
		if ratio <= 0.3 && canReviewVideoPerformance(*row) {
			p.Tier = "underperforming"
		}
		if ratio <= 0.3 && p.Tier != "underperforming" {
			p.Tier = "insufficient_data"
			p.Reason += "；暂不列为低表现，需满足发布满 3 天、可比历史和新鲜指标"
		}
		if p.Freshness == "stale" {
			p.Reason += "；指标超过 24 小时未更新，结论仅供历史复盘"
		}
	}
	assessKeywordRanks(rows)
}

// Both classification and aggregate availability use the same prerequisites.
func canReviewVideoPerformance(row DataCenterItem) bool {
	return row.Performance.RelativeToMedian != nil && row.ObservedAt.Sub(row.PublishedAt) >= 72*time.Hour && hasComparableInteractionCounts(row.GrowthStatus) && row.Performance.Freshness == "fresh"
}
