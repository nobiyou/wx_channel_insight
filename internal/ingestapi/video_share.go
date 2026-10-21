package ingestapi

import "github.com/nobiyou/wx_channel_insight/internal/ingest"

type WXManualVideoItemResponse struct {
	ID                string   `json:"id"`
	ObjectID          string   `json:"objectId"`
	ObjectNonceID     string   `json:"objectNonceId,omitempty"`
	Title             string   `json:"title"`
	ChannelUsername   string   `json:"channelUsername,omitempty"`
	ChannelName       string   `json:"channelName"`
	Source            string   `json:"source"`
	CoverURL          string   `json:"coverUrl,omitempty"`
	PublishedAt       string   `json:"publishedAt,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	PlayCount         int      `json:"playCount"`
	LikeCount         int      `json:"likeCount"`
	CommentCount      int      `json:"commentCount"`
	FavCount          int      `json:"favCount"`
	ForwardCount      int      `json:"forwardCount"`
	VideoURLAvailable bool     `json:"videoUrlAvailable"`
	DataBasis         string   `json:"dataBasis"`
}

type WXManualVideoResponse struct {
	InputURL string                    `json:"inputUrl"`
	Video    WXManualVideoItemResponse `json:"video"`
	Summary  SummaryResponse           `json:"summary"`
}

func NewWXManualVideoResponse(inputURL string, item ingest.WXFeedListItem, summary ingest.Summary) WXManualVideoResponse {
	return WXManualVideoResponse{
		InputURL: inputURL,
		Video: WXManualVideoItemResponse{
			ID:                item.ID,
			ObjectID:          item.ID,
			ObjectNonceID:     item.ObjectNonceID,
			Title:             item.Title,
			ChannelUsername:   item.ChannelUsername,
			ChannelName:       item.ChannelName,
			Source:            ingest.RunSourceWXManualShare,
			CoverURL:          item.CoverURL,
			PublishedAt:       formatTime(item.PublishedAt),
			Tags:              append([]string(nil), item.Tags...),
			PlayCount:         item.PlayCount,
			LikeCount:         item.LikeCount,
			CommentCount:      item.CommentCount,
			FavCount:          item.FavCount,
			ForwardCount:      item.ForwardCount,
			VideoURLAvailable: item.VideoURL != "",
			DataBasis:         "真实 wx_channel 分享链接解析；已加入默认监控，默认任务启用时会继续采集，任务暂停时同步暂停",
		},
		Summary: NewSummaryResponse(summary),
	}
}
