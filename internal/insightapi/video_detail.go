package insightapi

import (
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type VideoMetricSnapshotResponse struct {
	Likes     *int `json:"likes"`
	Comments  *int `json:"comments"`
	Favorites *int `json:"favorites"`
	Forwards  *int `json:"forwards"`
}

type VideoMetricHistoryPointResponse struct {
	ObservedAt string `json:"observedAt"`
	Likes      *int   `json:"likes"`
	Comments   *int   `json:"comments"`
	Favorites  *int   `json:"favorites"`
	Forwards   *int   `json:"forwards"`
}

type VideoPlaybackResponse struct {
	Status    string `json:"status"`
	URL       string `json:"url,omitempty"`
	Message   string `json:"message,omitempty"`
	FetchedAt string `json:"fetchedAt,omitempty"`
}

type VideoGrowthWindowResponse struct {
	From               string `json:"from,omitempty"`
	To                 string `json:"to,omitempty"`
	BaselineObservedAt string `json:"baselineObservedAt,omitempty"`
	LatestObservedAt   string `json:"latestObservedAt,omitempty"`
}

func mapVideoGrowthWindow(window insight.VideoGrowthWindow) VideoGrowthWindowResponse {
	return VideoGrowthWindowResponse{From: formatTime(window.From), To: formatTime(window.To), BaselineObservedAt: formatTime(window.BaselineObservedAt), LatestObservedAt: formatTime(window.LatestObservedAt)}
}

type VideoDetailResponse struct {
	ID                        string                            `json:"id"`
	ObjectID                  string                            `json:"objectId"`
	ObjectNonceID             string                            `json:"objectNonceId,omitempty"`
	Title                     string                            `json:"title"`
	ChannelUsername           string                            `json:"channelUsername,omitempty"`
	ChannelName               string                            `json:"channelName"`
	Source                    string                            `json:"source"`
	Sources                   []string                          `json:"sources"`
	SourceKeywords            []string                          `json:"sourceKeywords"`
	CoverURL                  string                            `json:"coverUrl,omitempty"`
	PublishedAt               string                            `json:"publishedAt"`
	Tags                      []string                          `json:"tags,omitempty"`
	Metrics                   VideoMetricSnapshotResponse       `json:"metrics"`
	PositiveDeltas            RecommendationDeltasResponse      `json:"positiveDeltas"`
	GrowthRate                *float64                          `json:"growthRate"`
	GrowthStatus              string                            `json:"growthStatus"`
	GrowthBasis               string                            `json:"growthBasis"`
	GrowthWindow              VideoGrowthWindowResponse         `json:"growthWindow"`
	ObservationCount          int                               `json:"observationCount"`
	HasGrowthBaseline         bool                              `json:"hasGrowthBaseline"`
	HasComparableInteractions bool                              `json:"hasComparableInteractions"`
	InsufficientHistory       bool                              `json:"insufficientHistory"`
	LastObservedAt            string                            `json:"lastObservedAt"`
	DataBasis                 string                            `json:"dataBasis"`
	History                   []VideoMetricHistoryPointResponse `json:"history"`
	Playback                  VideoPlaybackResponse             `json:"playback"`
}

func NewVideoDetailResponse(item insight.VideoDetail) VideoDetailResponse {
	result := VideoDetailResponse{
		ID:              item.ID,
		ObjectID:        item.ObjectID,
		ObjectNonceID:   item.ObjectNonceID,
		Title:           item.Title,
		ChannelUsername: item.ChannelUsername,
		ChannelName:     item.ChannelName,
		Source:          item.Source,
		Sources:         append([]string(nil), item.Sources...),
		SourceKeywords:  append([]string(nil), item.SourceKeywords...),
		CoverURL:        item.CoverURL,
		PublishedAt:     formatTime(item.PublishedAt),
		Tags:            append([]string(nil), item.Tags...),
		Metrics: VideoMetricSnapshotResponse{
			Likes:     ingest.MetricValue(item.Metrics.Likes, item.Metrics.MissingMetrics, ingest.MetricLike),
			Comments:  ingest.MetricValue(item.Metrics.Comments, item.Metrics.MissingMetrics, ingest.MetricComment),
			Favorites: ingest.MetricValue(item.Metrics.Favorites, item.Metrics.MissingMetrics, ingest.MetricFavorite),
			Forwards:  ingest.MetricValue(item.Metrics.Forwards, item.Metrics.MissingMetrics, ingest.MetricForward),
		},
		PositiveDeltas: mapRecommendationDeltas(item.PositiveDeltas),
		GrowthRate:     item.GrowthRate, GrowthStatus: item.GrowthStatus, GrowthBasis: item.GrowthBasis, GrowthWindow: mapVideoGrowthWindow(item.GrowthWindow),
		ObservationCount:          item.ObservationCount,
		HasGrowthBaseline:         item.HasGrowthBaseline,
		HasComparableInteractions: item.HasComparableInteractions,
		InsufficientHistory:       item.InsufficientHistory,
		LastObservedAt:            formatTime(item.LastObservedAt),
		DataBasis:                 item.DataBasis,
		Playback: VideoPlaybackResponse{
			Status:    item.Playback.Status,
			URL:       item.Playback.URL,
			Message:   item.Playback.Message,
			FetchedAt: formatTime(item.Playback.FetchedAt),
		},
		History: make([]VideoMetricHistoryPointResponse, 0, len(item.History)),
	}
	for _, point := range item.History {
		result.History = append(result.History, VideoMetricHistoryPointResponse{
			ObservedAt: formatTime(point.ObservedAt),
			Likes:      ingest.MetricValue(point.Likes, point.MissingMetrics, ingest.MetricLike),
			Comments:   ingest.MetricValue(point.Comments, point.MissingMetrics, ingest.MetricComment),
			Favorites:  ingest.MetricValue(point.Favorites, point.MissingMetrics, ingest.MetricFavorite),
			Forwards:   ingest.MetricValue(point.Forwards, point.MissingMetrics, ingest.MetricForward),
		})
	}
	return result
}
