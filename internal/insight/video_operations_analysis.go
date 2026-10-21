package insight

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type VideoOperationsAnalysis struct {
	RecentHot             VideoAssessmentStatus  `json:"recentHot"`
	Readiness             VideoAnalysisReadiness `json:"readiness"`
	RisingTop             []DataCenterItem       `json:"risingTop"`
	StandoutVideos        int                    `json:"standoutVideos"`
	UnderperformingVideos int                    `json:"underperformingVideos"`
	CompleteVideos        int                    `json:"completeVideos"`
	StaleVideos           int                    `json:"staleVideos"`
	RisingVideos          int                    `json:"risingVideos"`
	SelectedVideos        int                    `json:"selectedVideos"`
	LatestObservedAt      time.Time              `json:"latestObservedAt"`
	DiscoveryTrend        []VideoDiscoveryDay    `json:"discoveryTrend"`
	From                  time.Time              `json:"from"`
	To                    time.Time              `json:"to"`
	ObservedVideos        int                    `json:"observedVideos"`
	NewVideos             *int                   `json:"newVideos"`
	ComparableVideos      int                    `json:"comparableVideos"`
	GroupBy               string                 `json:"groupBy"`
	Groups                []VideoComparisonGroup `json:"groups"`
}

type VideoDiscoveryDay struct {
	Date           string `json:"date"`
	NewVideos      int    `json:"newVideos"`
	ObservedVideos int    `json:"observedVideos"`
}

type VideoComparisonGroup struct {
	Readiness           VideoAnalysisReadiness `json:"readiness"`
	evidence            videoAssessmentEvidence
	CompleteVideos      int       `json:"completeVideos"`
	KnownInteractions   int       `json:"knownInteractions"`
	AverageInteractions *float64  `json:"averageInteractions"`
	StandoutVideos      int       `json:"standoutVideos"`
	SelectedVideos      int       `json:"selectedVideos"`
	RisingVideos        int       `json:"risingVideos"`
	LatestObservedAt    time.Time `json:"latestObservedAt"`
	Key                 string    `json:"key"`
	Name                string    `json:"name"`
	Videos              int       `json:"videos"`
	NewVideos           *int      `json:"newVideos"`
	Likes               *int      `json:"likes"`
	Comments            *int      `json:"comments"`
	Favorites           *int      `json:"favorites"`
	Forwards            *int      `json:"forwards"`
	Interactions        *int      `json:"interactions"`
}

// Counts alone cannot distinguish a genuine zero from an unavailable assessment.
// Availability is produced here from the canonical per-video assessments; UI
// consumers must not infer eligibility from counts or completeness percentages.
type VideoAssessmentStatus struct {
	State          string `json:"state"`
	EligibleVideos int    `json:"eligibleVideos"`
	Reason         string `json:"reason"`
}

type VideoAnalysisReadiness struct {
	Growth   VideoAssessmentStatus `json:"growth"`
	Standout VideoAssessmentStatus `json:"standout"`
	Review   VideoAssessmentStatus `json:"review"`
}

type videoAssessmentEvidence struct {
	total, stale, growth, peers, freshPeers, review int
}

func (e *videoAssessmentEvidence) add(row DataCenterItem) {
	e.total++
	if row.Performance.Freshness == "stale" {
		e.stale++
	}
	if row.Performance.GrowthPerHour != nil && row.Performance.Freshness == "fresh" {
		e.growth++ // Includes qualified observations with genuinely zero growth.
	}
	if row.Performance.RelativeToMedian != nil {
		e.peers++
		if row.Performance.Freshness == "fresh" {
			e.freshPeers++
		}
	}
	if canReviewVideoPerformance(row) {
		e.review++
	}
}

func (e videoAssessmentEvidence) readiness() VideoAnalysisReadiness {
	if e.total == 0 {
		empty := VideoAssessmentStatus{State: "empty", Reason: "当前范围没有视频样本，调整筛选或采集后再评估"}
		return VideoAnalysisReadiness{Growth: empty, Standout: empty, Review: empty}
	}
	result := VideoAnalysisReadiness{
		Growth:   VideoAssessmentStatus{State: "insufficient_data", Reason: "缺少新鲜的两次合格观察，暂不能判断增长"},
		Standout: VideoAssessmentStatus{State: "insufficient_data", Reason: "缺少有效同组对比，需至少 5 条同账号同发布龄的完整样本"},
		Review:   VideoAssessmentStatus{State: "insufficient_data", Reason: "需有效同组对比、发布满 3 天、可比历史及新鲜指标"},
	}
	if e.stale == e.total {
		result.Growth = VideoAssessmentStatus{State: "stale", Reason: "当前范围视频均超过 24 小时未更新，需更新指标后判断增长"}
		result.Review = VideoAssessmentStatus{State: "stale", Reason: "当前范围视频均超过 24 小时未更新，暂不能判断低表现"}
	}
	if e.growth > 0 {
		result.Growth = VideoAssessmentStatus{State: "available", EligibleVideos: e.growth, Reason: fmt.Sprintf("%d 条新鲜视频具备合格的两次观察，可判断每小时增长", e.growth)}
	}
	if e.peers > 0 {
		result.Standout = VideoAssessmentStatus{State: "available", EligibleVideos: e.peers, Reason: fmt.Sprintf("%d 条视频具备同账号同发布龄的有效对比", e.peers)}
		if e.freshPeers < e.peers {
			result.Standout.State = "historical"
			result.Standout.Reason = fmt.Sprintf("%d 条可分层，仅 %d 条指标新鲜；包含历史结果，供内容复盘参考", e.peers, e.freshPeers)
		}
	}
	if e.review > 0 {
		result.Review = VideoAssessmentStatus{State: "available", EligibleVideos: e.review, Reason: fmt.Sprintf("%d 条视频满足发布满 3 天、可比历史和新鲜指标，可评估低表现", e.review)}
	}
	return result
}

// Analysis consumes the complete filtered set before pagination. New means
// first observed in the selected collection window, rather than first published.
func buildVideoOperationsAnalysis(rows []DataCenterItem, histories map[string][]time.Time, query DataCenterQuery) *VideoOperationsAnalysis {
	if query.Entity != DataCenterEntityVideos {
		return nil
	}
	result := &VideoOperationsAnalysis{From: query.From, To: query.To, ObservedVideos: len(rows), GroupBy: query.CompareBy, Groups: []VideoComparisonGroup{}}
	if result.GroupBy == "" {
		result.GroupBy = "account"
	}
	bounded := !query.From.IsZero() || !query.To.IsZero()
	if bounded {
		result.NewVideos = intPtr(0)
	}
	groups := map[string]*VideoComparisonGroup{}
	var evidence videoAssessmentEvidence
	days := map[string]*VideoDiscoveryDay{}
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	for _, row := range rows {
		evidence.add(row)
		interactions := completeInteractions(row)
		if interactions != nil {
			result.CompleteVideos++
		}
		if row.Performance.Tier == "standout" {
			result.StandoutVideos++
		}
		if row.Performance.Tier == "underperforming" {
			result.UnderperformingVideos++
		}
		if row.Performance.Freshness == "stale" {
			result.StaleVideos++
		}
		if row.Curation.Status == "selected" {
			result.SelectedVideos++
		}
		rising := row.Performance.GrowthPerHour != nil && *row.Performance.GrowthPerHour > 0 && row.Performance.Freshness == "fresh"
		if rising {
			result.RisingVideos++
		}
		if row.ObservedAt.After(result.LatestObservedAt) {
			result.LatestObservedAt = row.ObservedAt
		}
		if hasComparableInteractionCounts(row.GrowthStatus) {
			result.ComparableVideos++
		}
		var first time.Time
		for _, at := range histories[row.ID] {
			if !at.IsZero() && (first.IsZero() || at.Before(first)) {
				first = at
			}
		}
		observedDays := map[string]bool{}
		for _, at := range histories[row.ID] {
			if at.IsZero() || !query.From.IsZero() && at.Before(query.From) || !query.To.IsZero() && at.After(query.To) {
				continue
			}
			day := at.In(zone).Format("2006-01-02")
			if observedDays[day] {
				continue
			}
			observedDays[day] = true
			if days[day] == nil {
				days[day] = &VideoDiscoveryDay{Date: day}
			}
			days[day].ObservedVideos++
		}
		if !first.IsZero() && (query.From.IsZero() || !first.Before(query.From)) && (query.To.IsZero() || !first.After(query.To)) {
			day := first.In(zone).Format("2006-01-02")
			if days[day] == nil {
				days[day] = &VideoDiscoveryDay{Date: day}
			}
			days[day].NewVideos++
		}
		isNew := bounded && !first.IsZero() && (query.From.IsZero() || !first.Before(query.From)) && (query.To.IsZero() || !first.After(query.To))
		if isNew {
			*result.NewVideos++
		}
		keys := []string{row.ChannelUsername}
		names := []string{row.ChannelName}
		if result.GroupBy == "keyword" {
			keys = exactStrings(row.SourceKeywords)
			names = append([]string(nil), keys...)
		}
		if len(keys) == 0 {
			keys = []string{""}
			names = []string{""}
		}
		for i, key := range keys {
			name := names[i]
			if key == "" {
				key = strings.TrimSpace(name)
			}
			if key == "" {
				key = "__unknown"
				name = "来源未记录"
			}
			if name == "" {
				name = key
			}
			group := groups[key]
			if group == nil {
				group = &VideoComparisonGroup{Key: key, Name: name, Likes: intPtr(0), Comments: intPtr(0), Favorites: intPtr(0), Forwards: intPtr(0), Interactions: intPtr(0)}
				if bounded {
					group.NewVideos = intPtr(0)
				}
				groups[key] = group
			}
			group.Videos++
			group.evidence.add(row)
			if interactions != nil {
				group.CompleteVideos++
				group.KnownInteractions += *interactions
			}
			if row.Performance.Tier == "standout" {
				group.StandoutVideos++
			}
			if row.Curation.Status == "selected" {
				group.SelectedVideos++
			}
			if rising {
				group.RisingVideos++
			}
			if row.ObservedAt.After(group.LatestObservedAt) {
				group.LatestObservedAt = row.ObservedAt
			}
			if isNew {
				*group.NewVideos++
			}
			addCompleteMetric(&group.Likes, row.Likes)
			addCompleteMetric(&group.Comments, row.Comments)
			addCompleteMetric(&group.Favorites, row.Favorites)
			addCompleteMetric(&group.Forwards, row.Forwards)
			if group.Likes == nil || group.Comments == nil || group.Favorites == nil || group.Forwards == nil {
				group.Interactions = nil
			} else {
				total := *group.Likes + *group.Comments + *group.Favorites + *group.Forwards
				group.Interactions = &total
			}
		}
	}
	for _, group := range groups {
		group.Readiness = group.evidence.readiness()
		if group.CompleteVideos > 0 {
			average := float64(group.KnownInteractions) / float64(group.CompleteVideos)
			group.AverageInteractions = &average
		}
		result.Groups = append(result.Groups, *group)
	}
	result.Readiness = evidence.readiness()
	result.DiscoveryTrend = []VideoDiscoveryDay{}
	for _, day := range days {
		result.DiscoveryTrend = append(result.DiscoveryTrend, *day)
	}
	sort.Slice(result.DiscoveryTrend, func(i, j int) bool { return result.DiscoveryTrend[i].Date < result.DiscoveryTrend[j].Date })
	result.RisingTop = []DataCenterItem{}
	for _, row := range rows {
		if row.Performance.GrowthPerHour != nil && *row.Performance.GrowthPerHour > 0 && row.Performance.Freshness == "fresh" {
			result.RisingTop = append(result.RisingTop, row)
		}
	}
	sortDataCenterItems(result.RisingTop, "growth_per_hour", "desc")
	if len(result.RisingTop) > 4 {
		result.RisingTop = result.RisingTop[:4]
	}
	sort.Slice(result.Groups, func(i, j int) bool {
		a, b := result.Groups[i], result.Groups[j]
		if a.Interactions == nil && b.Interactions != nil {
			return false
		}
		if a.Interactions != nil && b.Interactions == nil {
			return true
		}
		if a.Interactions != nil && b.Interactions != nil && *a.Interactions != *b.Interactions {
			return *a.Interactions > *b.Interactions
		}
		return a.Key < b.Key
	})
	return result
}

func addCompleteMetric(total **int, value *int) {
	if *total == nil {
		return
	}
	if value == nil {
		*total = nil
		return
	}
	**total += *value
}
