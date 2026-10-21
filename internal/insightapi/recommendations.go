package insightapi

import "github.com/nobiyou/wx_channel_insight/internal/insight"

type RecommendationMetricsResponse struct {
	Views     int `json:"views"`
	Likes     int `json:"likes"`
	Comments  int `json:"comments"`
	Favorites int `json:"favorites"`
	Forwards  int `json:"forwards"`
}

type RecommendationDeltasResponse struct {
	Likes     int `json:"likes"`
	Comments  int `json:"comments"`
	Favorites int `json:"favorites"`
	Forwards  int `json:"forwards"`
	Total     int `json:"total"`
}

type AccountRecommendationResponse struct {
	ID                  string                        `json:"id"`
	Username            string                        `json:"username"`
	ChannelName         string                        `json:"channelName"`
	Source              string                        `json:"source"`
	Sources             []string                      `json:"sources"`
	VideoCount          int                           `json:"videoCount"`
	Metrics             RecommendationMetricsResponse `json:"metrics"`
	PositiveDeltas      RecommendationDeltasResponse  `json:"positiveDeltas"`
	InteractionScore    float64                       `json:"interactionScore"`
	GrowthScore         float64                       `json:"growthScore"`
	RecommendationScore float64                       `json:"recommendationScore"`
	GrowthRate          float64                       `json:"growthRate"`
	ObservationCount    int                           `json:"observationCount"`
	LastObservedAt      string                        `json:"lastObservedAt"`
	HasGrowthBaseline   bool                          `json:"hasGrowthBaseline"`
	InsufficientHistory bool                          `json:"insufficientHistory"`
	DataBasis           string                        `json:"dataBasis"`
	Reason              string                        `json:"reason"`
	CanMonitor          bool                          `json:"canMonitor"`
}

type VideoRecommendationResponse struct {
	ID                  string                        `json:"id"`
	ObjectID            string                        `json:"objectId,omitempty"`
	ObjectNonceID       string                        `json:"objectNonceId,omitempty"`
	CoverURL            string                        `json:"coverUrl,omitempty"`
	Title               string                        `json:"title"`
	ChannelUsername     string                        `json:"channelUsername,omitempty"`
	ChannelName         string                        `json:"channelName"`
	Keyword             string                        `json:"keyword,omitempty"`
	Source              string                        `json:"source"`
	Tags                []string                      `json:"tags,omitempty"`
	PublishedAt         string                        `json:"publishedAt"`
	Metrics             RecommendationMetricsResponse `json:"metrics"`
	PositiveDeltas      RecommendationDeltasResponse  `json:"positiveDeltas"`
	InteractionScore    float64                       `json:"interactionScore"`
	GrowthScore         float64                       `json:"growthScore"`
	RecommendationScore float64                       `json:"recommendationScore"`
	GrowthRate          float64                       `json:"growthRate"`
	ObservationCount    int                           `json:"observationCount"`
	LastObservedAt      string                        `json:"lastObservedAt"`
	HasGrowthBaseline   bool                          `json:"hasGrowthBaseline"`
	InsufficientHistory bool                          `json:"insufficientHistory"`
	DataBasis           string                        `json:"dataBasis"`
	Reason              string                        `json:"reason"`
}

type AccountRecommendationsResponse struct {
	Items         []AccountRecommendationResponse `json:"items"`
	WorkspaceID   string                          `json:"workspaceId"`
	HubUserID     string                          `json:"hubUserId"`
	LatestBatchID string                          `json:"latestBatchId"`
	GeneratedAt   string                          `json:"generatedAt"`
}

type VideoRecommendationsResponse struct {
	Items         []VideoRecommendationResponse `json:"items"`
	WorkspaceID   string                        `json:"workspaceId"`
	HubUserID     string                        `json:"hubUserId"`
	LatestBatchID string                        `json:"latestBatchId"`
	GeneratedAt   string                        `json:"generatedAt"`
}

func NewAccountRecommendationsResponse(items []insight.AccountRecommendation, snapshot insight.RecommendationSnapshot) AccountRecommendationsResponse {
	return AccountRecommendationsResponse{
		Items:         mapAccountRecommendations(items),
		WorkspaceID:   snapshot.WorkspaceID,
		HubUserID:     snapshot.HubUserID,
		LatestBatchID: snapshot.LatestBatchID,
		GeneratedAt:   formatTime(snapshot.GeneratedAt),
	}
}

func NewVideoRecommendationsResponse(items []insight.VideoRecommendation, snapshot insight.RecommendationSnapshot) VideoRecommendationsResponse {
	return VideoRecommendationsResponse{
		Items:         mapVideoRecommendations(items),
		WorkspaceID:   snapshot.WorkspaceID,
		HubUserID:     snapshot.HubUserID,
		LatestBatchID: snapshot.LatestBatchID,
		GeneratedAt:   formatTime(snapshot.GeneratedAt),
	}
}

func mapRecommendationMetrics(item insight.RecommendationMetrics) RecommendationMetricsResponse {
	return RecommendationMetricsResponse{
		Views:     item.Views,
		Likes:     item.Likes,
		Comments:  item.Comments,
		Favorites: item.Favorites,
		Forwards:  item.Forwards,
	}
}

func mapRecommendationDeltas(item insight.RecommendationDeltas) RecommendationDeltasResponse {
	return RecommendationDeltasResponse{
		Likes:     item.Likes,
		Comments:  item.Comments,
		Favorites: item.Favorites,
		Forwards:  item.Forwards,
		Total:     item.Total,
	}
}

func mapAccountRecommendations(items []insight.AccountRecommendation) []AccountRecommendationResponse {
	results := make([]AccountRecommendationResponse, 0, len(items))
	for _, item := range items {
		results = append(results, AccountRecommendationResponse{
			ID:                  item.ID,
			Username:            item.Username,
			ChannelName:         item.ChannelName,
			Source:              item.Source,
			Sources:             append([]string(nil), item.Sources...),
			VideoCount:          item.VideoCount,
			Metrics:             mapRecommendationMetrics(item.Metrics),
			PositiveDeltas:      mapRecommendationDeltas(item.PositiveDeltas),
			InteractionScore:    item.InteractionScore,
			GrowthScore:         item.GrowthScore,
			RecommendationScore: item.RecommendationScore,
			GrowthRate:          item.GrowthRate,
			ObservationCount:    item.ObservationCount,
			LastObservedAt:      formatTime(item.LastObservedAt),
			HasGrowthBaseline:   item.HasGrowthBaseline,
			InsufficientHistory: item.InsufficientHistory,
			DataBasis:           item.DataBasis,
			Reason:              item.Reason,
			CanMonitor:          item.CanMonitor,
		})
	}
	return results
}

func mapVideoRecommendations(items []insight.VideoRecommendation) []VideoRecommendationResponse {
	results := make([]VideoRecommendationResponse, 0, len(items))
	for _, item := range items {
		results = append(results, VideoRecommendationResponse{
			ID:                  item.ID,
			ObjectID:            item.ObjectID,
			ObjectNonceID:       item.ObjectNonceID,
			CoverURL:            item.CoverURL,
			Title:               item.Title,
			ChannelUsername:     item.ChannelUsername,
			ChannelName:         item.ChannelName,
			Keyword:             item.Keyword,
			Source:              item.Source,
			Tags:                append([]string(nil), item.Tags...),
			PublishedAt:         formatTime(item.PublishedAt),
			Metrics:             mapRecommendationMetrics(item.Metrics),
			PositiveDeltas:      mapRecommendationDeltas(item.PositiveDeltas),
			InteractionScore:    item.InteractionScore,
			GrowthScore:         item.GrowthScore,
			RecommendationScore: item.RecommendationScore,
			GrowthRate:          item.GrowthRate,
			ObservationCount:    item.ObservationCount,
			LastObservedAt:      formatTime(item.LastObservedAt),
			HasGrowthBaseline:   item.HasGrowthBaseline,
			InsufficientHistory: item.InsufficientHistory,
			DataBasis:           item.DataBasis,
			Reason:              item.Reason,
		})
	}
	return results
}
