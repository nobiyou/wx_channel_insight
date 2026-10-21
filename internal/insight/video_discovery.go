package insight

import (
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	recentHeatVersion     = "log-half-life-v1"
	recentHeatMaxAge      = 7 * 24 * time.Hour
	recentHeatHalfLife    = 72 * time.Hour
	keywordRankMinSamples = 5
)

type VideoRecentHeat struct {
	Score     *float64 `json:"score"`
	State     string   `json:"state"`
	Reason    string   `json:"reason"`
	Algorithm string   `json:"algorithm"`
}

type VideoKeywordRank struct {
	Keyword     string   `json:"keyword"`
	Rank        *int     `json:"rank"`
	Percentile  *float64 `json:"percentile"`
	SampleCount int      `json:"sampleCount"`
	State       string   `json:"state"`
	Reason      string   `json:"reason"`
}

func assessRecentHeat(row DataCenterItem, reference time.Time) VideoRecentHeat {
	result := VideoRecentHeat{State: "insufficient_data", Reason: "互动指标未采全，暂不计算近期热度", Algorithm: recentHeatVersion}
	if row.Performance.Interactions == nil {
		return result
	}
	if row.Performance.Freshness == "stale" {
		result.State, result.Reason = "stale", "指标超过 24 小时未更新，不进入近期热门"
		return result
	}
	if row.Performance.Freshness != "fresh" {
		result.Reason = "缺少可信采集时间，暂不计算近期热度"
		return result
	}
	if row.PublishedAt.IsZero() || row.PublishedAt.After(row.ObservedAt) || row.PublishedAt.After(reference) {
		result.Reason = "缺少可信发布时间，暂不计算近期热度"
		return result
	}
	age := reference.Sub(row.PublishedAt)
	if age > recentHeatMaxAge {
		result.State, result.Reason = "outside_window", "发布已超过 7 天，可在累计热门中复盘"
		return result
	}
	score := math.Log1p(float64(*row.Performance.Interactions)) * math.Exp2(-float64(age)/float64(recentHeatHalfLife))
	result.Score, result.State = &score, "available"
	result.Reason = "时间衰减热度 v1：ln(1 + 总互动) × 2^(-发布小时数 / 72)，用于排序，不代表播放量或内容质量"
	return result
}

// Keyword ranks use exact collection provenance, never title matches. Build the
// cohort once before text, account, publication, curation and paging filters.
// The collection-time window still defines which observations can be compared.
func assessKeywordRanks(rows []DataCenterItem) {
	cohorts := map[string][]int{}
	historical := map[string]bool{}
	for i := range rows {
		row := &rows[i]
		row.Performance.KeywordRanks = []VideoKeywordRank{}
		if row.Performance.Interactions == nil || row.Performance.Freshness == "unknown" {
			continue
		}
		for _, keyword := range exactStrings(row.SourceKeywords) {
			cohorts[keyword] = append(cohorts[keyword], *row.Performance.Interactions)
			if row.Performance.Freshness != "fresh" {
				historical[keyword] = true
			}
		}
	}
	for _, values := range cohorts {
		sort.Ints(values)
	}
	for i := range rows {
		row := &rows[i]
		for _, keyword := range exactStrings(row.SourceKeywords) {
			values := cohorts[keyword]
			rank := VideoKeywordRank{Keyword: keyword, SampleCount: len(values), State: "insufficient_data"}
			switch {
			case row.Performance.Interactions == nil:
				rank.Reason = "该视频互动指标未采全，不参与关键词排名"
			case row.Performance.Freshness == "unknown":
				rank.Reason = "该视频缺少可信采集时间，不参与关键词排名"
			case len(values) < keywordRankMinSamples:
				rank.Reason = fmt.Sprintf("关键词仅 %d 条完整样本，至少 %d 条才排名", len(values), keywordRankMinSamples)
			default:
				value := *row.Performance.Interactions
				lower := sort.SearchInts(values, value)
				upper := sort.Search(len(values), func(j int) bool { return values[j] > value })
				position, percentile := len(values)-upper+1, float64(lower)/float64(len(values))*100
				rank.Rank, rank.Percentile, rank.State = &position, &percentile, "available"
				rank.Reason = fmt.Sprintf("按累计互动排名，超过 %.1f%% 已采集完整样本；并列同名次，未校正账号规模和发布龄", percentile)
				if historical[keyword] {
					rank.State = "historical"
					rank.Reason += "；样本含过期数据，仅供历史参考"
				}
			}
			row.Performance.KeywordRanks = append(row.Performance.KeywordRanks, rank)
		}
	}
}

// This describes the matching source scope BEFORE quality/library filters, so
// an empty live ranking can still explain stale evidence instead of showing 0.
func recentHotReadiness(rows []DataCenterItem) VideoAssessmentStatus {
	status := VideoAssessmentStatus{State: "empty", Reason: "当前来源范围没有采集样本"}
	stale := 0
	for _, row := range rows {
		if row.Performance.RecentHeat.Score != nil {
			status.EligibleVideos++
		}
		if row.Performance.Freshness == "stale" {
			stale++
		}
	}
	switch {
	case status.EligibleVideos > 0:
		status.State, status.Reason = "available", fmt.Sprintf("当前来源范围有 %d 条可计算近期热度的样本；榜单只展示正互动视频", status.EligibleVideos)
	case len(rows) > 0 && stale == len(rows):
		status.State, status.Reason = "stale", "当前来源范围的指标均超过 24 小时未更新，近期热门待更新；可先查看累计热门"
	case len(rows) > 0:
		status.State, status.Reason = "insufficient_data", "当前来源范围暂无满足近 7 天发布、24 小时内更新且互动完整的样本"
	}
	return status
}
