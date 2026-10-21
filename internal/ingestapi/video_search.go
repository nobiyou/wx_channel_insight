package ingestapi

import (
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

type WXVideoSearchItemResponse struct {
	ID              string   `json:"id"`
	ObjectNonceID   string   `json:"objectNonceId"`
	Title           string   `json:"title"`
	ChannelName     string   `json:"channelName"`
	ChannelUsername string   `json:"channelUsername,omitempty"`
	PublishedAt     string   `json:"publishedAt,omitempty"`
	CoverURL        string   `json:"coverUrl,omitempty"`
	Tags            []string `json:"tags"`
	PlayCount       int      `json:"playCount"`
	LikeCount       int      `json:"likeCount"`
	CommentCount    int      `json:"commentCount"`
	FavCount        int      `json:"favCount"`
	ForwardCount    int      `json:"forwardCount"`
	Signal          string   `json:"signal"`
}

type WXVideoSearchResponse struct {
	Keyword    string                      `json:"keyword"`
	NextMarker string                      `json:"nextMarker"`
	Items      []WXVideoSearchItemResponse `json:"items"`
}

func NewWXVideoSearchResponse(keyword string, result ingest.WXFeedListResult) WXVideoSearchResponse {
	items := make([]WXVideoSearchItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, WXVideoSearchItemResponse{
			ID:              item.ID,
			ObjectNonceID:   item.ObjectNonceID,
			Title:           item.Title,
			ChannelName:     item.ChannelName,
			ChannelUsername: item.ChannelUsername,
			PublishedAt:     formatPublishedAt(item.PublishedAt),
			CoverURL:        item.CoverURL,
			Tags:            append([]string(nil), item.Tags...),
			PlayCount:       item.PlayCount,
			LikeCount:       item.LikeCount,
			CommentCount:    item.CommentCount,
			FavCount:        item.FavCount,
			ForwardCount:    item.ForwardCount,
			Signal:          item.Signal,
		})
	}
	return WXVideoSearchResponse{
		Keyword:    keyword,
		NextMarker: result.NextMarker,
		Items:      items,
	}
}

func formatPublishedAt(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
