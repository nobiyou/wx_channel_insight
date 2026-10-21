package normalize

import "time"

type Summary struct {
	WorkspaceID   string    `json:"workspaceId"`
	HubUserID     string    `json:"hubUserId"`
	State         string    `json:"state"`
	LatestBatchID string    `json:"latestBatchId"`
	GeneratedAt   time.Time `json:"generatedAt"`
	LatestEventAt time.Time `json:"latestEventAt"`
	ContentCount  int       `json:"contentCount"`
	CommentCount  int       `json:"commentCount"`
	TopicCount    int       `json:"topicCount"`
	AccountCount  int       `json:"accountCount"`
}

type ContentSnapshot struct {
	ID              string    `json:"id"`
	BatchID         string    `json:"batchId"`
	EventID         string    `json:"eventId"`
	Source          string    `json:"source"`
	CollectedAt     time.Time `json:"collectedAt"`
	PublishedAt     time.Time `json:"publishedAt,omitempty"`
	ObjectID        string    `json:"objectId,omitempty"`
	ObjectNonceID   string    `json:"objectNonceId,omitempty"`
	ChannelUsername string    `json:"channelUsername,omitempty"`
	CoverURL        string    `json:"coverUrl,omitempty"`
	Title           string    `json:"title"`
	ChannelName     string    `json:"channelName"`
	Keyword         string    `json:"keyword"`
	Signal          string    `json:"signal"`
	PromptAngle     string    `json:"promptAngle"`
	TargetAudience  string    `json:"targetAudience"`
	VideoCount      int       `json:"videoCount"`
	PlayCount       int       `json:"playCount"`
	LikeCount       int       `json:"likeCount"`
	CommentCount    int       `json:"commentCount"`
	FavCount        int       `json:"favCount"`
	ForwardCount    int       `json:"forwardCount"`
	TopicCount      int       `json:"topicCount"`
	Tags            []string  `json:"tags,omitempty"`
	Keywords        []string  `json:"keywords,omitempty"`
}

type CommentSnapshot struct {
	ID                    string    `json:"id"`
	BatchID               string    `json:"batchId"`
	EventID               string    `json:"eventId"`
	Source                string    `json:"source"`
	CollectedAt           time.Time `json:"collectedAt"`
	ObjectID              string    `json:"objectId,omitempty"`
	ObjectNonceID         string    `json:"objectNonceId,omitempty"`
	Title                 string    `json:"title"`
	ChannelName           string    `json:"channelName"`
	Keyword               string    `json:"keyword"`
	Signal                string    `json:"signal"`
	RepresentativeComment string    `json:"representativeComment"`
	TargetAudience        string    `json:"targetAudience"`
	VideoCount            int       `json:"videoCount"`
	CommentCount          int       `json:"commentCount"`
	TopicCount            int       `json:"topicCount"`
	Keywords              []string  `json:"keywords,omitempty"`
}

type TopicObservation struct {
	ID             string    `json:"id"`
	BatchID        string    `json:"batchId"`
	EventID        string    `json:"eventId"`
	Source         string    `json:"source"`
	CollectedAt    time.Time `json:"collectedAt"`
	ObjectID       string    `json:"objectId,omitempty"`
	ObjectNonceID  string    `json:"objectNonceId,omitempty"`
	Keyword        string    `json:"keyword"`
	Cluster        string    `json:"cluster"`
	Signal         string    `json:"signal"`
	PromptAngle    string    `json:"promptAngle"`
	TargetAudience string    `json:"targetAudience"`
	VideoCount     int       `json:"videoCount"`
	CommentCount   int       `json:"commentCount"`
	TopicCount     int       `json:"topicCount"`
	Keywords       []string  `json:"keywords,omitempty"`
}

type AccountObservation struct {
	ID           string    `json:"id"`
	BatchID      string    `json:"batchId"`
	EventID      string    `json:"eventId"`
	Source       string    `json:"source"`
	CollectedAt  time.Time `json:"collectedAt"`
	Username     string    `json:"username,omitempty"`
	ChannelName  string    `json:"channelName"`
	Title        string    `json:"title"`
	Signal       string    `json:"signal"`
	VideoCount   int       `json:"videoCount"`
	CommentCount int       `json:"commentCount"`
	TopicCount   int       `json:"topicCount"`
	Tags         []string  `json:"tags,omitempty"`
}

type Snapshot struct {
	Summary             Summary              `json:"summary"`
	ContentSnapshots    []ContentSnapshot    `json:"contentSnapshots"`
	CommentSnapshots    []CommentSnapshot    `json:"commentSnapshots"`
	TopicObservations   []TopicObservation   `json:"topicObservations"`
	AccountObservations []AccountObservation `json:"accountObservations"`
}
