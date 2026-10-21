package insight

import (
	"time"
)

type Dashboard struct {
	GeneratedAt            time.Time               `json:"generatedAt"`
	ReadModel              ReadModelState          `json:"readModel"`
	Workspace              WorkspaceSummary        `json:"workspace"`
	WorkspaceBridge        WorkspaceBridge         `json:"workspaceBridge"`
	IngestSummary          IngestSummary           `json:"ingestSummary"`
	Overview               OverviewMetrics         `json:"overview"`
	HotVideos              []HotVideo              `json:"hotVideos"`
	HotVideoRankings       HotVideoRankings        `json:"hotVideoRankings,omitempty"`
	HotVideoWindowRankings HotVideoWindowRankings  `json:"hotVideoWindowRankings,omitempty"`
	HotTopics              []HotTopic              `json:"hotTopics"`
	AccountRecommendations []AccountRecommendation `json:"accountRecommendations"`
	VideoRecommendations   []VideoRecommendation   `json:"videoRecommendations"`
	WatchlistAlerts        []WatchlistAlert        `json:"watchlistAlerts"`
	CommentInsights        []CommentInsight        `json:"commentInsights"`
	ReportSummary          ReportSummary           `json:"reportSummary"`
}

type ReadModelState struct {
	Mode           string                `json:"mode"`
	MaterializedAt time.Time             `json:"materializedAt"`
	Source         string                `json:"source"`
	LatestBatchID  string                `json:"latestBatchId"`
	ViewStatuses   []ReadModelViewStatus `json:"viewStatuses"`
}

type ReadModelViewStatus struct {
	Key            string    `json:"key"`
	Label          string    `json:"label"`
	FreshnessState string    `json:"freshnessState"`
	Summary        string    `json:"summary"`
	SourceNote     string    `json:"sourceNote"`
	LatestBatchID  string    `json:"latestBatchId"`
	MaterializedAt time.Time `json:"materializedAt"`
}

type ReadModelArtifactMeta struct {
	LatestBatchID  string    `json:"latestBatchId"`
	MaterializedAt time.Time `json:"materializedAt"`
}

type WorkspaceSummary struct {
	Name        string `json:"name"`
	Owner       string `json:"owner"`
	DeviceCount int    `json:"deviceCount"`
}

type WorkspaceBridge struct {
	WorkspaceID   string    `json:"workspaceId"`
	HubUserID     string    `json:"hubUserId"`
	ContextSource string    `json:"contextSource"`
	BootstrapMode string    `json:"bootstrapMode"`
	Source        string    `json:"source"`
	State         string    `json:"state"`
	Resolution    string    `json:"resolution"`
	LastSyncedAt  time.Time `json:"lastSyncedAt"`
}

type IngestSummary struct {
	State              string               `json:"state"`
	FreshnessState     string               `json:"freshnessState"`
	TotalEvents        int                  `json:"totalEvents"`
	SourceCount        int                  `json:"sourceCount"`
	LatestEventAt      time.Time            `json:"latestEventAt"`
	LastSuccessAt      time.Time            `json:"lastSuccessAt"`
	LastFailureAt      time.Time            `json:"lastFailureAt"`
	LastFailureMessage string               `json:"lastFailureMessage"`
	LatestBatchID      string               `json:"latestBatchId"`
	LastEventType      string               `json:"lastEventType"`
	AggregatedVideos   int                  `json:"aggregatedVideos"`
	AggregatedComments int                  `json:"aggregatedComments"`
	AggregatedTopics   int                  `json:"aggregatedTopics"`
	RecentEvents       []IngestEvent        `json:"recentEvents"`
	SourceStatuses     []IngestSourceStatus `json:"sourceStatuses"`
	RecentRuns         []IngestRun          `json:"recentRuns"`
	BindingStates      []IngestBindingState `json:"bindingStates"`
	OperatorReadiness  OperatorReadiness    `json:"operatorReadiness"`
}

type IngestEvent struct {
	ID                    string    `json:"id"`
	BatchID               string    `json:"batchId"`
	MachineID             string    `json:"machineId"`
	EventType             string    `json:"eventType"`
	Source                string    `json:"source"`
	CollectedAt           time.Time `json:"collectedAt"`
	VideoCount            int       `json:"videoCount"`
	CommentCount          int       `json:"commentCount"`
	TopicCount            int       `json:"topicCount"`
	Title                 string    `json:"title"`
	ChannelName           string    `json:"channelName"`
	Keyword               string    `json:"keyword"`
	Cluster               string    `json:"cluster"`
	Signal                string    `json:"signal"`
	PromptAngle           string    `json:"promptAngle"`
	RepresentativeComment string    `json:"representativeComment"`
	TargetAudience        string    `json:"targetAudience"`
	Tags                  []string  `json:"tags"`
	Keywords              []string  `json:"keywords"`
}

type IngestSourceStatus struct {
	Source         string               `json:"source"`
	State          string               `json:"state"`
	FreshnessState string               `json:"freshnessState"`
	LastRunAt      time.Time            `json:"lastRunAt"`
	LastSuccessAt  time.Time            `json:"lastSuccessAt"`
	LastFailureAt  time.Time            `json:"lastFailureAt"`
	LastError      string               `json:"lastError"`
	LatestBatchID  string               `json:"latestBatchId"`
	LastEventType  string               `json:"lastEventType"`
	EventCount     int                  `json:"eventCount"`
	Target         *IngestBindingTarget `json:"target,omitempty"`
}

type IngestBindingTarget struct {
	Title           string    `json:"title,omitempty"`
	ChannelName     string    `json:"channelName,omitempty"`
	ChannelUsername string    `json:"channelUsername,omitempty"`
	Keyword         string    `json:"keyword,omitempty"`
	Comment         string    `json:"comment,omitempty"`
	CommentCount    int       `json:"commentCount,omitempty"`
	ObjectID        string    `json:"objectId,omitempty"`
	ObjectNonceID   string    `json:"objectNonceId,omitempty"`
	PublishedAt     time.Time `json:"publishedAt,omitempty"`
}

type IngestRun struct {
	ID            string    `json:"id"`
	Source        string    `json:"source"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"startedAt"`
	FinishedAt    time.Time `json:"finishedAt"`
	BatchID       string    `json:"batchId"`
	EventCount    int       `json:"eventCount"`
	LatestEventAt time.Time `json:"latestEventAt"`
	LastEventType string    `json:"lastEventType"`
	Message       string    `json:"message"`
}

type IngestBindingState struct {
	Source            string               `json:"source"`
	Origin            string               `json:"origin"`
	TargetKey         string               `json:"targetKey,omitempty"`
	Path              string               `json:"path"`
	BaseURL           string               `json:"baseURL"`
	Username          string               `json:"username"`
	DeviceCount       int                  `json:"deviceCount"`
	UpdatedAt         time.Time            `json:"updatedAt"`
	LastRunStatus     string               `json:"lastRunStatus"`
	LastRunAt         time.Time            `json:"lastRunAt"`
	LastSuccessAt     time.Time            `json:"lastSuccessAt"`
	LastFailureAt     time.Time            `json:"lastFailureAt"`
	LastError         string               `json:"lastError"`
	LatestBatchID     string               `json:"latestBatchId"`
	LastAutoRunStatus string               `json:"lastAutoRunStatus"`
	LastAutoRunAt     time.Time            `json:"lastAutoRunAt"`
	LastAutoSuccessAt time.Time            `json:"lastAutoSuccessAt"`
	LastAutoBatchID   string               `json:"lastAutoBatchId"`
	Runnable          bool                 `json:"runnable"`
	CanRetry          bool                 `json:"canRetry"`
	RetryReason       string               `json:"retryReason"`
	Target            *IngestBindingTarget `json:"target,omitempty"`
	Diagnostic        *BindingDiagnostic   `json:"diagnostic,omitempty"`
}

type OperatorReadiness struct {
	State           string                    `json:"state"`
	ExpectedStatus  string                    `json:"expectedStatus"`
	RunnableSources int                       `json:"runnableSources"`
	BlockedSources  int                       `json:"blockedSources"`
	MissingSources  int                       `json:"missingSources"`
	Headline        string                    `json:"headline"`
	Summary         string                    `json:"summary"`
	NextAction      string                    `json:"nextAction"`
	Runnable        []OperatorReadinessSource `json:"runnable"`
	Blocked         []OperatorReadinessSource `json:"blocked"`
	Missing         []OperatorReadinessSource `json:"missing"`
}

type OperatorReadinessSource struct {
	Source            string             `json:"source"`
	TargetKey         string             `json:"targetKey,omitempty"`
	Label             string             `json:"label"`
	State             string             `json:"state"`
	Summary           string             `json:"summary"`
	Action            string             `json:"action"`
	Path              string             `json:"path"`
	BaseURL           string             `json:"baseURL"`
	Username          string             `json:"username"`
	DeviceCount       int                `json:"deviceCount"`
	UpdatedAt         time.Time          `json:"updatedAt"`
	LastRunStatus     string             `json:"lastRunStatus"`
	LastSuccessAt     time.Time          `json:"lastSuccessAt"`
	LastFailureAt     time.Time          `json:"lastFailureAt"`
	LatestBatchID     string             `json:"latestBatchId"`
	LastAutoRunStatus string             `json:"lastAutoRunStatus"`
	LastAutoRunAt     time.Time          `json:"lastAutoRunAt"`
	LastAutoSuccessAt time.Time          `json:"lastAutoSuccessAt"`
	LastAutoBatchID   string             `json:"lastAutoBatchId"`
	Runnable          bool               `json:"runnable"`
	CanRetry          bool               `json:"canRetry"`
	RetryReason       string             `json:"retryReason"`
	Diagnostic        *BindingDiagnostic `json:"diagnostic,omitempty"`
}

type BindingDiagnostic struct {
	Code                          string `json:"code"`
	ResolvedBaseURL               string `json:"resolvedBaseUrl,omitempty"`
	StatusURL                     string `json:"statusUrl,omitempty"`
	WSHealthURL                   string `json:"wsHealthUrl,omitempty"`
	WXChannelStatusError          string `json:"wxChannelStatusError,omitempty"`
	ConnectedClientCount          *int   `json:"connectedClientCount,omitempty"`
	ReadyClientCount              *int   `json:"readyClientCount,omitempty"`
	FeedReadyClientCount          *int   `json:"feedReadyClientCount,omitempty"`
	HealthStatus                  string `json:"healthStatus,omitempty"`
	CurrentBrowserProxyAddress    string `json:"currentBrowserProxyAddress,omitempty"`
	ExpectedProxyAddress          string `json:"expectedProxyAddress,omitempty"`
	WXChannelProxyMode            string `json:"wxChannelProxyMode,omitempty"`
	WXChannelProxyAddress         string `json:"wxChannelProxyAddress,omitempty"`
	WXChannelInjectionChecked     *bool  `json:"wxChannelInjectionChecked,omitempty"`
	WXChannelInjectionStarted     *bool  `json:"wxChannelInjectionStarted,omitempty"`
	WXChannelInjectionError       string `json:"wxChannelInjectionError,omitempty"`
	WXChannelCertificateInstalled *bool  `json:"wxChannelCertificateInstalled,omitempty"`
	WXChannelCertificateError     string `json:"wxChannelCertificateError,omitempty"`
	CurrentPagePath               string `json:"currentPagePath,omitempty"`
	CurrentPageHref               string `json:"currentPageHref,omitempty"`
}

type OverviewMetrics struct {
	MonitoredAccounts int     `json:"monitoredAccounts"`
	TrackedVideos     int     `json:"trackedVideos"`
	EmergingTopics    int     `json:"emergingTopics"`
	NewCommentBursts  int     `json:"newCommentBursts"`
	DailyCoverageRate float64 `json:"dailyCoverageRate"`
	ReportFreshness   string  `json:"reportFreshness"`
}

type HotVideo struct {
	ID                  string               `json:"id"`
	Title               string               `json:"title"`
	ChannelUsername     string               `json:"channelUsername,omitempty"`
	ChannelName         string               `json:"channelName"`
	Source              string               `json:"source"`
	ObjectID            string               `json:"objectId,omitempty"`
	ObjectNonceID       string               `json:"objectNonceId,omitempty"`
	CoverURL            string               `json:"coverUrl,omitempty"`
	Tags                []string             `json:"tags"`
	PublishedAt         time.Time            `json:"publishedAt,omitempty"`
	ObservedAt          time.Time            `json:"observedAt,omitempty"`
	PublishedAgo        string               `json:"publishedAgo"`
	Views               int                  `json:"views"`
	Likes               int                  `json:"likes"`
	Comments            int                  `json:"comments"`
	Favorites           int                  `json:"favorites"`
	Forwards            int                  `json:"forwards"`
	GrowthRate          float64              `json:"growthRate,omitempty"`
	InteractionScore    float64              `json:"interactionScore,omitempty"`
	GrowthScore         float64              `json:"growthScore,omitempty"`
	RecommendationScore float64              `json:"recommendationScore,omitempty"`
	PositiveDeltas      RecommendationDeltas `json:"positiveDeltas,omitempty"`
	ObservationCount    int                  `json:"observationCount,omitempty"`
	HasGrowthBaseline   bool                 `json:"hasGrowthBaseline"`
	InsufficientHistory bool                 `json:"insufficientHistory"`
	DataBasis           string               `json:"dataBasis,omitempty"`
	Reason              string               `json:"reason,omitempty"`
	EngagementScore     int                  `json:"engagementScore,omitempty"`
	Signal              string               `json:"signal"`
}

type HotTopic struct {
	ID                  string               `json:"id"`
	Keyword             string               `json:"keyword"`
	Cluster             string               `json:"cluster"`
	Source              string               `json:"source"`
	Sources             []string             `json:"sources,omitempty"`
	ObservedAt          time.Time            `json:"observedAt,omitempty"`
	SourceAccounts      []string             `json:"sourceAccounts"`
	Mentions            int                  `json:"mentions"`
	Comments            int                  `json:"comments"`
	GrowthRate          float64              `json:"growthRate,omitempty"`
	RelatedVideos       int                  `json:"relatedVideos"`
	InteractionScore    float64              `json:"interactionScore,omitempty"`
	GrowthScore         float64              `json:"growthScore,omitempty"`
	RecommendationScore float64              `json:"recommendationScore,omitempty"`
	PositiveDeltas      RecommendationDeltas `json:"positiveDeltas,omitempty"`
	ObservationCount    int                  `json:"observationCount,omitempty"`
	HasGrowthBaseline   bool                 `json:"hasGrowthBaseline"`
	InsufficientHistory bool                 `json:"insufficientHistory"`
	DataBasis           string               `json:"dataBasis,omitempty"`
	Reason              string               `json:"reason,omitempty"`
	PromptAngle         string               `json:"promptAngle"`
	Keywords            []string             `json:"keywords"`
}

type WatchlistAlert struct {
	ID           string    `json:"id"`
	AccountName  string    `json:"accountName"`
	AlertType    string    `json:"alertType"`
	Source       string    `json:"source"`
	Severity     string    `json:"severity"`
	Summary      string    `json:"summary"`
	OccurredAt   time.Time `json:"occurredAt,omitempty"`
	HappenedAt   string    `json:"happenedAt"`
	Recommended  string    `json:"recommended"`
	VideoCount   int       `json:"videoCount"`
	CommentDelta int       `json:"commentDelta"`
}

type CommentInsight struct {
	ID               string    `json:"id"`
	Theme            string    `json:"theme"`
	Source           string    `json:"source"`
	ObservedAt       time.Time `json:"observedAt,omitempty"`
	ObjectID         string    `json:"objectId,omitempty"`
	ObjectNonceID    string    `json:"objectNonceId,omitempty"`
	SourceAccounts   []string  `json:"sourceAccounts"`
	QuestionRate     int       `json:"questionRate"`
	Sentiment        string    `json:"sentiment"`
	Representative   string    `json:"representative"`
	ActionSuggestion string    `json:"actionSuggestion"`
	RelatedKeywords  []string  `json:"relatedKeywords"`
}

type ReportSummary struct {
	Period            string    `json:"period"`
	Headline          string    `json:"headline"`
	KeyFinding        string    `json:"keyFinding"`
	RecommendedAction string    `json:"recommendedAction"`
	Watchout          string    `json:"watchout"`
	GeneratedAt       time.Time `json:"generatedAt,omitempty"`
	AutoGeneratedAt   string    `json:"autoGeneratedAt"`
}
