package insightapi

import "github.com/nobiyou/wx_channel_insight/internal/insight"

type DataCenterItemResponse struct {
	Curation         insight.VideoCuration     `json:"curation"`
	Performance      insight.VideoPerformance  `json:"performance"`
	ID               string                    `json:"id"`
	Entity           string                    `json:"entity"`
	Status           string                    `json:"status"`
	HistoryAvailable bool                      `json:"historyAvailable"`
	DeletedAt        string                    `json:"deletedAt,omitempty"`
	DeletedBy        string                    `json:"deletedBy,omitempty"`
	Source           string                    `json:"source,omitempty"`
	Sources          []string                  `json:"sources,omitempty"`
	SourceKeywords   []string                  `json:"sourceKeywords"`
	ObservedAt       string                    `json:"observedAt,omitempty"`
	PublishedAt      string                    `json:"publishedAt,omitempty"`
	ObjectID         string                    `json:"objectId,omitempty"`
	ObjectNonceID    string                    `json:"objectNonceId,omitempty"`
	CoverURL         string                    `json:"coverUrl,omitempty"`
	Title            string                    `json:"title,omitempty"`
	ChannelName      string                    `json:"channelName,omitempty"`
	ChannelUsername  string                    `json:"channelUsername,omitempty"`
	AccountID        string                    `json:"accountId,omitempty"`
	IdentityStatus   string                    `json:"identityStatus,omitempty"`
	Region           string                    `json:"region,omitempty"`
	Industry         string                    `json:"industry,omitempty"`
	Note             string                    `json:"note,omitempty"`
	ProfileSource    string                    `json:"profileSource,omitempty"`
	IsPriority       bool                      `json:"isPriority,omitempty"`
	Keyword          string                    `json:"keyword,omitempty"`
	Cluster          string                    `json:"cluster,omitempty"`
	Theme            string                    `json:"theme,omitempty"`
	Representative   string                    `json:"representative,omitempty"`
	Sentiment        string                    `json:"sentiment,omitempty"`
	ActionSuggestion string                    `json:"actionSuggestion,omitempty"`
	Signal           string                    `json:"signal,omitempty"`
	SourceAccounts   []string                  `json:"sourceAccounts,omitempty"`
	RelatedKeywords  []string                  `json:"relatedKeywords,omitempty"`
	Tags             []string                  `json:"tags,omitempty"`
	Keywords         []string                  `json:"keywords,omitempty"`
	Views            *int                      `json:"views,omitempty"`
	Likes            *int                      `json:"likes,omitempty"`
	Comments         *int                      `json:"comments,omitempty"`
	Favorites        *int                      `json:"favorites,omitempty"`
	Forwards         *int                      `json:"forwards,omitempty"`
	VideoCount       *int                      `json:"videoCount,omitempty"`
	Mentions         *int                      `json:"mentions,omitempty"`
	RelatedVideos    *int                      `json:"relatedVideos,omitempty"`
	QuestionRate     *int                      `json:"questionRate,omitempty"`
	CommentID        string                    `json:"commentId,omitempty"`
	ParentCommentID  string                    `json:"parentCommentId,omitempty"`
	AuthorUsername   string                    `json:"authorUsername,omitempty"`
	AuthorNickname   string                    `json:"authorNickname,omitempty"`
	Content          string                    `json:"content,omitempty"`
	IPRegion         string                    `json:"ipRegion,omitempty"`
	CommentCreatedAt string                    `json:"commentCreatedAt,omitempty"`
	CommentLikeCount *int                      `json:"commentLikeCount,omitempty"`
	ReplyCount       *int                      `json:"replyCount,omitempty"`
	ObservationCount int                       `json:"observationCount,omitempty"`
	GrowthRate       *float64                  `json:"growthRate"`
	GrowthStatus     string                    `json:"growthStatus,omitempty"`
	GrowthBasis      string                    `json:"growthBasis,omitempty"`
	GrowthWindow     VideoGrowthWindowResponse `json:"growthWindow"`
	DataBasis        string                    `json:"dataBasis,omitempty"`
}

type DataCenterPageResponse struct {
	Management       *insight.VideoManagementSummary  `json:"management,omitempty"`
	Analysis         *insight.VideoOperationsAnalysis `json:"analysis,omitempty"`
	Entity           string                           `json:"entity"`
	Items            []DataCenterItemResponse         `json:"items"`
	Page             int                              `json:"page"`
	PageSize         int                              `json:"pageSize"`
	Total            int                              `json:"total"`
	TotalPages       int                              `json:"totalPages"`
	Summary          DataCenterSummaryResponse        `json:"summary"`
	AsOf             string                           `json:"asOf"`
	GeneratedAt      string                           `json:"generatedAt"`
	HistoryAvailable bool                             `json:"historyAvailable"`
	ViewStatus       DataCenterViewStatusResponse     `json:"viewStatus"`
}

type DataCenterSummaryResponse struct {
	Videos   int `json:"videos"`
	Topics   int `json:"topics"`
	Accounts int `json:"accounts"`
	Comments int `json:"comments"`
}

type DataCenterViewStatusResponse struct {
	State    string `json:"state"`
	Source   string `json:"source"`
	Evidence string `json:"evidence"`
	Notice   string `json:"notice,omitempty"`
}

type DataCenterHistoryItemResponse struct {
	ID              string `json:"id"`
	EntityID        string `json:"entityId"`
	ObservedAt      string `json:"observedAt"`
	Source          string `json:"source,omitempty"`
	EvidenceType    string `json:"evidenceType"`
	Title           string `json:"title,omitempty"`
	ChannelName     string `json:"channelName,omitempty"`
	ChannelUsername string `json:"channelUsername,omitempty"`
	Keyword         string `json:"keyword,omitempty"`
	Theme           string `json:"theme,omitempty"`
	Representative  string `json:"representative,omitempty"`
	Signal          string `json:"signal,omitempty"`
	Views           *int   `json:"views,omitempty"`
	Likes           *int   `json:"likes,omitempty"`
	Comments        *int   `json:"comments,omitempty"`
	Favorites       *int   `json:"favorites,omitempty"`
	Forwards        *int   `json:"forwards,omitempty"`
	VideoCount      *int   `json:"videoCount,omitempty"`
	Mentions        *int   `json:"mentions,omitempty"`
	RelatedVideos   *int   `json:"relatedVideos,omitempty"`
	QuestionRate    *int   `json:"questionRate,omitempty"`
}

type DataCenterHistoryResponse struct {
	Entity           string                          `json:"entity"`
	EntityID         string                          `json:"entityId"`
	Items            []DataCenterHistoryItemResponse `json:"items"`
	Page             int                             `json:"page"`
	PageSize         int                             `json:"pageSize"`
	Total            int                             `json:"total"`
	TotalPages       int                             `json:"totalPages"`
	HistoryAvailable bool                            `json:"historyAvailable"`
	Evidence         string                          `json:"evidence"`
}

type DataCenterMutationResponse struct {
	Entity         string   `json:"entity"`
	EntityID       string   `json:"entityId"`
	DeletedAt      string   `json:"deletedAt,omitempty"`
	DeletedBy      string   `json:"deletedBy,omitempty"`
	DeletionMode   string   `json:"deletionMode"`
	ImpactScope    []string `json:"impactScope"`
	ReadModelState string   `json:"readModelState"`
	Restored       bool     `json:"restored"`
}

func NewDataCenterPageResponse(payload insight.DataCenterPage) DataCenterPageResponse {
	items := make([]DataCenterItemResponse, 0, len(payload.Items))
	for _, item := range payload.Items {
		items = append(items, mapDataCenterItem(item))
	}
	return DataCenterPageResponse{
		Management: payload.Management,
		Analysis:   payload.Analysis,
		Entity:     string(payload.Entity), Items: items, Page: payload.Page, PageSize: payload.PageSize,
		Total: payload.Total, TotalPages: payload.TotalPages, AsOf: payload.AsOf,
		Summary: DataCenterSummaryResponse{
			Videos: payload.Summary.Videos, Topics: payload.Summary.Topics,
			Accounts: payload.Summary.Accounts, Comments: payload.Summary.Comments,
		},
		GeneratedAt: formatTime(payload.GeneratedAt), HistoryAvailable: payload.HistoryAvailable,
		ViewStatus: DataCenterViewStatusResponse{State: payload.ViewStatus.State, Source: payload.ViewStatus.Source, Evidence: payload.ViewStatus.Evidence, Notice: payload.ViewStatus.Notice},
	}
}

func NewDataCenterHistoryResponse(payload insight.DataCenterHistoryPage) DataCenterHistoryResponse {
	items := make([]DataCenterHistoryItemResponse, 0, len(payload.Items))
	for _, item := range payload.Items {
		items = append(items, DataCenterHistoryItemResponse{
			ID: item.ID, EntityID: item.EntityID, ObservedAt: formatTime(item.ObservedAt), Source: item.Source,
			EvidenceType: item.EvidenceType, Title: item.Title, ChannelName: item.ChannelName, ChannelUsername: item.ChannelUsername,
			Keyword: item.Keyword, Theme: item.Theme, Representative: item.Representative, Signal: item.Signal,
			Views: item.Views, Likes: item.Likes, Comments: item.Comments, Favorites: item.Favorites, Forwards: item.Forwards,
			VideoCount: item.VideoCount, Mentions: item.Mentions, RelatedVideos: item.RelatedVideos, QuestionRate: item.QuestionRate,
		})
	}
	return DataCenterHistoryResponse{
		Entity: string(payload.Entity), EntityID: payload.EntityID, Items: items, Page: payload.Page, PageSize: payload.PageSize,
		Total: payload.Total, TotalPages: payload.TotalPages, HistoryAvailable: payload.HistoryAvailable, Evidence: payload.Evidence,
	}
}

func NewDataCenterMutationResponse(payload insight.DataCenterMutationResult) DataCenterMutationResponse {
	return DataCenterMutationResponse{
		Entity: string(payload.Entity), EntityID: payload.EntityID, DeletedAt: formatTime(payload.DeletedAt), DeletedBy: payload.DeletedBy,
		DeletionMode: payload.DeletionMode, ImpactScope: append([]string(nil), payload.ImpactScope...), ReadModelState: payload.ReadModelState, Restored: payload.Restored,
	}
}

func mapDataCenterItem(item insight.DataCenterItem) DataCenterItemResponse {
	return DataCenterItemResponse{
		Curation: item.Curation, Performance: item.Performance,
		ID: item.ID, Entity: string(item.Entity), Status: item.Status, HistoryAvailable: item.HistoryAvailable,
		DeletedAt: formatTime(item.DeletedAt), DeletedBy: item.DeletedBy, Source: item.Source, Sources: append([]string(nil), item.Sources...), SourceKeywords: append([]string{}, item.SourceKeywords...),
		ObservedAt: formatTime(item.ObservedAt), PublishedAt: formatTime(item.PublishedAt), ObjectID: item.ObjectID, ObjectNonceID: item.ObjectNonceID, CoverURL: item.CoverURL,
		Title: item.Title, ChannelName: item.ChannelName, ChannelUsername: item.ChannelUsername, AccountID: item.AccountID, IdentityStatus: item.IdentityStatus,
		Region: item.Region, Industry: item.Industry, Note: item.Note, ProfileSource: item.ProfileSource, IsPriority: item.IsPriority,
		Keyword: item.Keyword, Cluster: item.Cluster, Theme: item.Theme,
		Representative: item.Representative, Sentiment: item.Sentiment, ActionSuggestion: item.ActionSuggestion, Signal: item.Signal,
		SourceAccounts: append([]string(nil), item.SourceAccounts...), RelatedKeywords: append([]string(nil), item.RelatedKeywords...), Tags: append([]string(nil), item.Tags...), Keywords: append([]string(nil), item.Keywords...),
		Views: item.Views, Likes: item.Likes, Comments: item.Comments, Favorites: item.Favorites, Forwards: item.Forwards, VideoCount: item.VideoCount,
		Mentions: item.Mentions, RelatedVideos: item.RelatedVideos, QuestionRate: item.QuestionRate, CommentID: item.CommentID,
		ParentCommentID: item.ParentCommentID, AuthorUsername: item.AuthorUsername, AuthorNickname: item.AuthorNickname, Content: item.Content,
		IPRegion: item.IPRegion, CommentCreatedAt: formatTime(item.CommentCreatedAt), CommentLikeCount: item.CommentLikeCount, ReplyCount: item.ReplyCount,
		ObservationCount: item.ObservationCount, GrowthRate: item.GrowthRate, GrowthStatus: item.GrowthStatus, GrowthBasis: item.GrowthBasis, GrowthWindow: mapVideoGrowthWindow(item.GrowthWindow), DataBasis: item.DataBasis,
	}
}
