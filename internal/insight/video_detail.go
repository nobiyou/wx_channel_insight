package insight

import (
	"sort"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

type VideoMetricSnapshot struct {
	MissingMetrics ingest.MetricMask `json:"-"`
	Likes          int               `json:"likes"`
	Comments       int               `json:"comments"`
	Favorites      int               `json:"favorites"`
	Forwards       int               `json:"forwards"`
}

type VideoMetricHistoryPoint struct {
	MissingMetrics ingest.MetricMask `json:"-"`
	ObservedAt     time.Time         `json:"observedAt"`
	Likes          int               `json:"likes"`
	Comments       int               `json:"comments"`
	Favorites      int               `json:"favorites"`
	Forwards       int               `json:"forwards"`
}

type VideoPlayback struct {
	Status    string    `json:"status"`
	URL       string    `json:"url,omitempty"`
	Message   string    `json:"message,omitempty"`
	FetchedAt time.Time `json:"fetchedAt,omitempty"`
}

type VideoDetail struct {
	ID                        string                    `json:"id"`
	ObjectID                  string                    `json:"objectId"`
	ObjectNonceID             string                    `json:"objectNonceId,omitempty"`
	Title                     string                    `json:"title"`
	ChannelUsername           string                    `json:"channelUsername,omitempty"`
	ChannelName               string                    `json:"channelName"`
	Source                    string                    `json:"source"`
	Sources                   []string                  `json:"sources"`
	SourceKeywords            []string                  `json:"sourceKeywords"`
	CoverURL                  string                    `json:"coverUrl,omitempty"`
	PublishedAt               time.Time                 `json:"publishedAt,omitempty"`
	Tags                      []string                  `json:"tags,omitempty"`
	Metrics                   VideoMetricSnapshot       `json:"metrics"`
	PositiveDeltas            RecommendationDeltas      `json:"positiveDeltas"`
	GrowthRate                *float64                  `json:"growthRate"`
	GrowthStatus              string                    `json:"growthStatus"`
	GrowthBasis               string                    `json:"growthBasis"`
	GrowthWindow              VideoGrowthWindow         `json:"growthWindow"`
	ObservationCount          int                       `json:"observationCount"`
	HasGrowthBaseline         bool                      `json:"hasGrowthBaseline"`
	HasComparableInteractions bool                      `json:"hasComparableInteractions"`
	InsufficientHistory       bool                      `json:"insufficientHistory"`
	LastObservedAt            time.Time                 `json:"lastObservedAt,omitempty"`
	DataBasis                 string                    `json:"dataBasis"`
	History                   []VideoMetricHistoryPoint `json:"history"`
	Playback                  VideoPlayback             `json:"playback"`
}

// BuildVideoDetail derives the stable video metadata and metric history from
// append-only observations. A profile may fill current metadata when an
// observation is incomplete, but it is never added to the persisted history.
func BuildVideoDetail(objectID string, observations []ingest.VideoMetricObservation, profile *ingest.WXFeedListItem) (VideoDetail, bool) {
	objectID = strings.TrimSpace(objectID)
	if objectID == "" {
		return VideoDetail{}, false
	}

	matches := make([]ingest.VideoMetricObservation, 0)
	for _, observation := range observations {
		if strings.TrimSpace(observation.ObjectID) == objectID ||
			(strings.TrimSpace(observation.ObjectID) == "" && strings.TrimSpace(observation.ID) == objectID) {
			matches = append(matches, observation)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].ObservedAt.Equal(matches[j].ObservedAt) {
			return matches[i].ID < matches[j].ID
		}
		return matches[i].ObservedAt.Before(matches[j].ObservedAt)
	})
	if len(matches) == 0 && profile == nil {
		return VideoDetail{}, false
	}

	detail := VideoDetail{
		ID:        objectID,
		ObjectID:  objectID,
		History:   make([]VideoMetricHistoryPoint, 0, len(matches)),
		DataBasis: "真实 wx_channel 视频观察；曲线仅展示已保存的点赞、评论、收藏、转发历史",
		Playback:  VideoPlayback{Status: "unavailable"},
	}
	growth := qualifyVideoGrowth(matches, time.Time{}, time.Time{})
	detail.GrowthRate, detail.GrowthStatus, detail.GrowthBasis, detail.GrowthWindow = growth.Rate, growth.Status, growth.Basis, growth.Window
	detail.PositiveDeltas = growth.Deltas
	detail.HasGrowthBaseline = growth.Status == "available"
	detail.HasComparableInteractions = hasComparableInteractionCounts(growth.Status)
	detail.InsufficientHistory = growth.Status == "insufficient_history"
	if len(matches) > 0 {
		sourceSet, keywordSet := map[string]struct{}{}, map[string]struct{}{}
		for _, match := range matches {
			if value := strings.TrimSpace(match.Source); value != "" {
				sourceSet[value] = struct{}{}
			}
			if value := strings.TrimSpace(match.Keyword); value != "" {
				keywordSet[value] = struct{}{}
			}
		}
		for value := range sourceSet {
			detail.Sources = append(detail.Sources, value)
		}
		for value := range keywordSet {
			detail.SourceKeywords = append(detail.SourceKeywords, value)
		}
		sort.Strings(detail.Sources)
		sort.Strings(detail.SourceKeywords)
		latest := matches[len(matches)-1]
		detail.ObjectNonceID = latest.ObjectNonceID
		detail.Title = latest.Title
		detail.ChannelUsername = latest.ChannelUsername
		detail.ChannelName = latest.ChannelName
		detail.Source = latest.Source
		detail.CoverURL = latest.CoverURL
		detail.PublishedAt = latest.PublishedAt
		detail.Tags = append([]string(nil), latest.Tags...)
		detail.Metrics = videoMetricSnapshot(latest)
		detail.LastObservedAt = latest.ObservedAt
		for index := len(matches) - 1; index >= 0; index-- {
			if detail.PublishedAt.IsZero() && !matches[index].PublishedAt.IsZero() {
				detail.PublishedAt = matches[index].PublishedAt
			}
			if detail.Title == "" {
				detail.Title = matches[index].Title
			}
			if detail.ChannelUsername == "" {
				detail.ChannelUsername = matches[index].ChannelUsername
			}
			if detail.ChannelName == "" {
				detail.ChannelName = matches[index].ChannelName
			}
			if detail.Source == "" {
				detail.Source = matches[index].Source
			}
			if detail.CoverURL == "" {
				detail.CoverURL = matches[index].CoverURL
			}
			if len(detail.Tags) == 0 && len(matches[index].Tags) > 0 {
				detail.Tags = append([]string(nil), matches[index].Tags...)
			}
		}
		for _, item := range matches {
			detail.History = append(detail.History, VideoMetricHistoryPoint{
				ObservedAt:     item.ObservedAt,
				MissingMetrics: item.MissingMetrics,
				Likes:          nonNegative(item.LikeCount),
				Comments:       nonNegative(item.CommentCount),
				Favorites:      nonNegative(item.FavCount),
				Forwards:       nonNegative(item.ForwardCount),
			})
		}
		detail.ObservationCount = len(matches)
	}

	if profile != nil {
		if profile.ObjectNonceID != "" {
			detail.ObjectNonceID = profile.ObjectNonceID
		}
		if profile.Title != "" {
			detail.Title = profile.Title
		}
		if profile.ChannelUsername != "" {
			detail.ChannelUsername = profile.ChannelUsername
		}
		if profile.ChannelName != "" {
			detail.ChannelName = profile.ChannelName
		}
		if profile.CoverURL != "" {
			detail.CoverURL = profile.CoverURL
		}
		if !profile.PublishedAt.IsZero() {
			detail.PublishedAt = profile.PublishedAt
		}
		if len(profile.Tags) > 0 {
			detail.Tags = append([]string(nil), profile.Tags...)
		}
		if len(matches) == 0 {
			detail.Metrics = VideoMetricSnapshot{
				MissingMetrics: profile.MissingMetrics,
				Likes:          nonNegative(profile.LikeCount),
				Comments:       nonNegative(profile.CommentCount),
				Favorites:      nonNegative(profile.FavCount),
				Forwards:       nonNegative(profile.ForwardCount),
			}
		}
	}

	return detail, true
}

func videoMetricSnapshot(item ingest.VideoMetricObservation) VideoMetricSnapshot {
	return VideoMetricSnapshot{
		MissingMetrics: item.MissingMetrics,
		Likes:          nonNegative(item.LikeCount),
		Comments:       nonNegative(item.CommentCount),
		Favorites:      nonNegative(item.FavCount),
		Forwards:       nonNegative(item.ForwardCount),
	}
}
