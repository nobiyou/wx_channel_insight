package aggregate

import "time"

type Summary struct {
	WorkspaceID        string    `json:"workspaceId"`
	HubUserID          string    `json:"hubUserId"`
	State              string    `json:"state"`
	LatestBatchID      string    `json:"latestBatchId"`
	GeneratedAt        time.Time `json:"generatedAt"`
	LatestObservedAt   time.Time `json:"latestObservedAt"`
	VideoMetricCount   int       `json:"videoMetricCount"`
	TopicMetricCount   int       `json:"topicMetricCount"`
	CommentFactCount   int       `json:"commentFactCount"`
	SourceAccountCount int       `json:"sourceAccountCount"`
}

type VideoMetric struct {
	ID              string    `json:"id"`
	EventID         string    `json:"eventId"`
	BatchID         string    `json:"batchId"`
	Source          string    `json:"source"`
	ObservedAt      time.Time `json:"observedAt"`
	PublishedAt     time.Time `json:"publishedAt,omitempty"`
	ObjectID        string    `json:"objectId,omitempty"`
	ObjectNonceID   string    `json:"objectNonceId,omitempty"`
	CoverURL        string    `json:"coverUrl,omitempty"`
	Title           string    `json:"title"`
	ChannelName     string    `json:"channelName"`
	Keyword         string    `json:"keyword"`
	Signal          string    `json:"signal"`
	PromptAngle     string    `json:"promptAngle"`
	TargetAudience  string    `json:"targetAudience"`
	Tags            []string  `json:"tags,omitempty"`
	Keywords        []string  `json:"keywords,omitempty"`
	Views           int       `json:"views"`
	Likes           int       `json:"likes"`
	Comments        int       `json:"comments"`
	Favorites       int       `json:"favorites"`
	Forwards        int       `json:"forwards"`
	GrowthRate      float64   `json:"growthRate"`
	EngagementScore int       `json:"engagementScore"`
}

type TopicMetric struct {
	ID             string    `json:"id"`
	EventID        string    `json:"eventId"`
	BatchID        string    `json:"batchId"`
	Source         string    `json:"source"`
	ObservedAt     time.Time `json:"observedAt"`
	Keyword        string    `json:"keyword"`
	Cluster        string    `json:"cluster"`
	Signal         string    `json:"signal"`
	PromptAngle    string    `json:"promptAngle"`
	TargetAudience string    `json:"targetAudience"`
	Keywords       []string  `json:"keywords,omitempty"`
	Mentions       int       `json:"mentions"`
	GrowthRate     float64   `json:"growthRate"`
	RelatedVideos  int       `json:"relatedVideos"`
}

type CommentInsightFact struct {
	ID               string    `json:"id"`
	EventID          string    `json:"eventId"`
	BatchID          string    `json:"batchId"`
	Source           string    `json:"source"`
	ObservedAt       time.Time `json:"observedAt"`
	ObjectID         string    `json:"objectId,omitempty"`
	ObjectNonceID    string    `json:"objectNonceId,omitempty"`
	Theme            string    `json:"theme"`
	Sentiment        string    `json:"sentiment"`
	Representative   string    `json:"representative"`
	ActionSuggestion string    `json:"actionSuggestion"`
	TargetAudience   string    `json:"targetAudience"`
	RelatedKeywords  []string  `json:"relatedKeywords,omitempty"`
	QuestionRate     int       `json:"questionRate"`
	CommentCount     int       `json:"commentCount"`
}

type Aggregate struct {
	Summary             Summary              `json:"summary"`
	VideoMetrics        []VideoMetric        `json:"videoMetrics"`
	TopicMetrics        []TopicMetric        `json:"topicMetrics"`
	CommentInsightFacts []CommentInsightFact `json:"commentInsightFacts"`
}
