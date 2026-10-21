package ingestapi

import "github.com/nobiyou/wx_channel_insight/internal/ingest"

type WXFeedAccountResponse struct {
	Username  string   `json:"username"`
	Nickname  string   `json:"nickname"`
	Signature string   `json:"signature"`
	HeadURL   string   `json:"headUrl"`
	Region    string   `json:"region,omitempty"`
	Industry  string   `json:"industry,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

type WXFeedAccountsResponse struct {
	Items []WXFeedAccountResponse `json:"items"`
}

func NewWXFeedAccountsResponse(items []ingest.WXFeedAccount) WXFeedAccountsResponse {
	results := make([]WXFeedAccountResponse, 0, len(items))
	for _, item := range items {
		results = append(results, WXFeedAccountResponse{
			Username:  item.Username,
			Nickname:  item.Nickname,
			Signature: item.Signature,
			HeadURL:   item.HeadURL,
			Region:    item.Region,
			Industry:  item.Industry,
			Tags:      append([]string(nil), item.Tags...),
		})
	}
	return WXFeedAccountsResponse{Items: results}
}
