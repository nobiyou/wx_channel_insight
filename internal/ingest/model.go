package ingest

import (
	"strings"
	"time"
)

const (
	RunSourceWXCommentExport = "wx_comment_export"
	RunSourceWXFeedList      = "wx_feed_list"
	RunSourceWXVideoSearch   = "wx_video_search"
	RunSourceWXManualShare   = "wx_manual_share"
	RunSourceWXVideoProfile  = "wx_video_profile"
	BindingOriginManual      = "manual"
	BindingOriginMonitoring  = "monitoring"
	BindingOriginImported    = "imported"
	OperatorTriggerCycle     = "operator-cycle"
	OperatorTriggerLoop      = "operator-loop"
	OperatorTriggerScheduled = "monitoring-scheduled"
)

type WXFeedAccount struct {
	Username  string   `json:"username"`
	Nickname  string   `json:"nickname"`
	Signature string   `json:"signature,omitempty"`
	HeadURL   string   `json:"headUrl,omitempty"`
	Region    string   `json:"region,omitempty"`
	Industry  string   `json:"industry,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

type Event struct {
	MissingMetrics        MetricMask `json:"missingMetrics,omitempty"`
	ID                    string     `json:"id"`
	BatchID               string     `json:"batchId"`
	HubUserID             string     `json:"hubUserId"`
	MachineID             string     `json:"machineId"`
	EventType             string     `json:"eventType"`
	Source                string     `json:"source"`
	CollectedAt           time.Time  `json:"collectedAt"`
	PublishedAt           time.Time  `json:"publishedAt,omitempty"`
	ObjectID              string     `json:"objectId,omitempty"`
	ObjectNonceID         string     `json:"objectNonceId,omitempty"`
	ChannelUsername       string     `json:"channelUsername,omitempty"`
	CoverURL              string     `json:"coverUrl,omitempty"`
	VideoCount            int        `json:"videoCount"`
	PlayCount             int        `json:"playCount"`
	LikeCount             int        `json:"likeCount"`
	CommentCount          int        `json:"commentCount"`
	FavCount              int        `json:"favCount"`
	ForwardCount          int        `json:"forwardCount"`
	TopicCount            int        `json:"topicCount"`
	Title                 string     `json:"title,omitempty"`
	ChannelName           string     `json:"channelName,omitempty"`
	Keyword               string     `json:"keyword,omitempty"`
	Cluster               string     `json:"cluster,omitempty"`
	Signal                string     `json:"signal,omitempty"`
	PromptAngle           string     `json:"promptAngle,omitempty"`
	RepresentativeComment string     `json:"representativeComment,omitempty"`
	TargetAudience        string     `json:"targetAudience,omitempty"`
	Tags                  []string   `json:"tags,omitempty"`
	Keywords              []string   `json:"keywords,omitempty"`
}

// VideoMetricObservation is an append-only observation captured from a real
// wx_channel feed/search response. Event rows remain the latest ingest state;
// these rows preserve each collection for change analysis.
type VideoMetricObservation struct {
	MissingMetrics  MetricMask `json:"missingMetrics,omitempty"`
	ID              string     `json:"id"`
	HubUserID       string     `json:"hubUserId"`
	EventID         string     `json:"eventId"`
	BatchID         string     `json:"batchId"`
	Source          string     `json:"source"`
	ObservedAt      time.Time  `json:"observedAt"`
	PublishedAt     time.Time  `json:"publishedAt,omitempty"`
	ObjectID        string     `json:"objectId"`
	ObjectNonceID   string     `json:"objectNonceId,omitempty"`
	ChannelUsername string     `json:"channelUsername,omitempty"`
	CoverURL        string     `json:"coverUrl,omitempty"`
	Title           string     `json:"title"`
	ChannelName     string     `json:"channelName"`
	Keyword         string     `json:"keyword,omitempty"`
	Tags            []string   `json:"tags,omitempty"`
	PlayCount       int        `json:"playCount"`
	LikeCount       int        `json:"likeCount"`
	CommentCount    int        `json:"commentCount"`
	FavCount        int        `json:"favCount"`
	ForwardCount    int        `json:"forwardCount"`
}

// AccountMetricRow contains only the derived metrics needed by the account
// library for the currently visible account page.
type AccountMetricRow struct {
	MissingMetrics   MetricMask `json:"missingMetrics,omitempty"`
	ChannelUsername  string     `json:"channelUsername"`
	ChannelName      string     `json:"channelName"`
	VideoCount       int        `json:"videoCount"`
	Likes            int        `json:"likes"`
	Comments         int        `json:"comments"`
	Favorites        int        `json:"favorites"`
	Forwards         int        `json:"forwards"`
	RecentObservedAt time.Time  `json:"recentObservedAt"`
}

type Run struct {
	Collection    *CollectionProgress `json:"collection,omitempty"`
	ID            string              `json:"id"`
	Source        string              `json:"source"`
	Status        string              `json:"status"`
	StartedAt     time.Time           `json:"startedAt"`
	FinishedAt    time.Time           `json:"finishedAt"`
	BatchID       string              `json:"batchId,omitempty"`
	EventCount    int                 `json:"eventCount"`
	LatestEventAt time.Time           `json:"latestEventAt"`
	LastEventType string              `json:"lastEventType,omitempty"`
	Message       string              `json:"message,omitempty"`
}

type SourceBinding struct {
	Source        string    `json:"source"`
	Origin        string    `json:"origin,omitempty"`
	TargetKey     string    `json:"targetKey,omitempty"`
	Path          string    `json:"path,omitempty"`
	BaseURL       string    `json:"baseURL,omitempty"`
	ShareURL      string    `json:"shareUrl,omitempty"`
	ObjectID      string    `json:"objectId,omitempty"`
	ObjectNonceID string    `json:"objectNonceId,omitempty"`
	Username      string    `json:"username,omitempty"`
	Keyword       string    `json:"keyword,omitempty"`
	DeviceCount   int       `json:"deviceCount,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// SourceBindingTarget is the business-facing identity of a saved source.
// Execution fields remain available on SourceBinding; this projection keeps
// source-management views independent from internal ids and file paths.
type SourceBindingTarget struct {
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

// BindingTargetKey returns the stable identity of one saved ingest target.
// It keeps old callers compatible by deriving the key from the source payload.
func BindingTargetKey(binding SourceBinding) string {
	if value := strings.TrimSpace(binding.TargetKey); value != "" {
		return value
	}
	switch strings.TrimSpace(binding.Source) {
	case RunSourceWXFeedList, "wx_channel_feed_list":
		return strings.TrimSpace(binding.Username)
	case RunSourceWXVideoSearch, "wx_channel_video_search":
		return strings.TrimSpace(binding.Keyword)
	case RunSourceWXCommentExport, "wx_channel_comment_export", "finderGetCommentList", "finderGetCommentList.partial":
		return strings.TrimSpace(binding.Path)
	case RunSourceWXManualShare:
		if value := strings.TrimSpace(binding.ObjectID); value != "" {
			return value
		}
		if value := strings.TrimSpace(binding.ShareURL); value != "" {
			return value
		}
		return strings.TrimSpace(binding.Source)
	default:
		return strings.TrimSpace(binding.Source)
	}
}

func NormalizeSourceBinding(binding SourceBinding) SourceBinding {
	binding.Origin = NormalizeBindingOrigin(binding.Origin)
	binding.ShareURL = strings.TrimSpace(binding.ShareURL)
	binding.ObjectID = strings.TrimSpace(binding.ObjectID)
	binding.ObjectNonceID = strings.TrimSpace(binding.ObjectNonceID)
	binding.TargetKey = BindingTargetKey(binding)
	return binding
}

func NormalizeBindingOrigin(origin string) string {
	switch strings.ToLower(strings.TrimSpace(origin)) {
	case BindingOriginMonitoring:
		return BindingOriginMonitoring
	case BindingOriginImported:
		return BindingOriginImported
	default:
		return BindingOriginManual
	}
}

// NormalizeSourceBindingForUpsert keeps provenance stable when a scheduled
// refresh sends only the mutable source fields.
func NormalizeSourceBindingForUpsert(binding, existing SourceBinding) SourceBinding {
	if strings.TrimSpace(binding.Origin) == "" {
		binding.Origin = existing.Origin
	}
	return NormalizeSourceBinding(binding)
}

type SourceBindingDiagnostic struct {
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

type OperatorRun struct {
	ID               string               `json:"id"`
	Status           string               `json:"status"`
	TriggerMode      string               `json:"triggerMode,omitempty"`
	StartedAt        time.Time            `json:"startedAt"`
	FinishedAt       time.Time            `json:"finishedAt"`
	WorkspaceID      string               `json:"workspaceId,omitempty"`
	LatestBatchID    string               `json:"latestBatchId,omitempty"`
	AttemptedSources int                  `json:"attemptedSources"`
	SucceededSources int                  `json:"succeededSources"`
	FailedSources    int                  `json:"failedSources"`
	SkippedSources   int                  `json:"skippedSources"`
	FreshnessState   string               `json:"freshnessState,omitempty"`
	ReportHeadline   string               `json:"reportHeadline,omitempty"`
	TopVideoTitle    string               `json:"topVideoTitle,omitempty"`
	TopTopicKeyword  string               `json:"topTopicKeyword,omitempty"`
	Message          string               `json:"message,omitempty"`
	Outcomes         []OperatorRunOutcome `json:"outcomes,omitempty"`
}

type OperatorRunOutcome struct {
	Source  string `json:"source"`
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
}

type SourceBindingState struct {
	Source            string                   `json:"source"`
	Origin            string                   `json:"origin"`
	TargetKey         string                   `json:"targetKey,omitempty"`
	Path              string                   `json:"path,omitempty"`
	BaseURL           string                   `json:"baseURL,omitempty"`
	ShareURL          string                   `json:"shareUrl,omitempty"`
	ObjectID          string                   `json:"objectId,omitempty"`
	ObjectNonceID     string                   `json:"objectNonceId,omitempty"`
	Username          string                   `json:"username,omitempty"`
	Keyword           string                   `json:"keyword,omitempty"`
	DeviceCount       int                      `json:"deviceCount,omitempty"`
	UpdatedAt         time.Time                `json:"updatedAt"`
	LastRunStatus     string                   `json:"lastRunStatus,omitempty"`
	LastRunAt         time.Time                `json:"lastRunAt"`
	LastSuccessAt     time.Time                `json:"lastSuccessAt"`
	LastFailureAt     time.Time                `json:"lastFailureAt"`
	LastError         string                   `json:"lastError,omitempty"`
	LatestBatchID     string                   `json:"latestBatchId,omitempty"`
	LastAutoRunStatus string                   `json:"lastAutoRunStatus,omitempty"`
	LastAutoRunAt     time.Time                `json:"lastAutoRunAt"`
	LastAutoSuccessAt time.Time                `json:"lastAutoSuccessAt"`
	LastAutoBatchID   string                   `json:"lastAutoBatchId,omitempty"`
	Runnable          bool                     `json:"runnable"`
	CanRetry          bool                     `json:"canRetry"`
	RetryReason       string                   `json:"retryReason,omitempty"`
	Target            *SourceBindingTarget     `json:"target,omitempty"`
	Diagnostic        *SourceBindingDiagnostic `json:"diagnostic,omitempty"`
}

type OperatorReadinessSource struct {
	Source            string                   `json:"source"`
	TargetKey         string                   `json:"targetKey,omitempty"`
	Label             string                   `json:"label"`
	State             string                   `json:"state"`
	Summary           string                   `json:"summary"`
	Action            string                   `json:"action"`
	Path              string                   `json:"path,omitempty"`
	BaseURL           string                   `json:"baseURL,omitempty"`
	ShareURL          string                   `json:"shareUrl,omitempty"`
	ObjectID          string                   `json:"objectId,omitempty"`
	ObjectNonceID     string                   `json:"objectNonceId,omitempty"`
	Username          string                   `json:"username,omitempty"`
	Keyword           string                   `json:"keyword,omitempty"`
	DeviceCount       int                      `json:"deviceCount,omitempty"`
	UpdatedAt         time.Time                `json:"updatedAt"`
	LastRunStatus     string                   `json:"lastRunStatus,omitempty"`
	LastSuccessAt     time.Time                `json:"lastSuccessAt"`
	LastFailureAt     time.Time                `json:"lastFailureAt"`
	LatestBatchID     string                   `json:"latestBatchId,omitempty"`
	LastAutoRunStatus string                   `json:"lastAutoRunStatus,omitempty"`
	LastAutoRunAt     time.Time                `json:"lastAutoRunAt"`
	LastAutoSuccessAt time.Time                `json:"lastAutoSuccessAt"`
	LastAutoBatchID   string                   `json:"lastAutoBatchId,omitempty"`
	Runnable          bool                     `json:"runnable"`
	CanRetry          bool                     `json:"canRetry"`
	RetryReason       string                   `json:"retryReason,omitempty"`
	Diagnostic        *SourceBindingDiagnostic `json:"diagnostic,omitempty"`
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

type SourceStatus struct {
	Source             string               `json:"source"`
	State              string               `json:"state"`
	FreshnessState     string               `json:"freshnessState"`
	SyncFreshnessState string               `json:"syncFreshnessState"`
	LastRunAt          time.Time            `json:"lastRunAt"`
	LastSuccessAt      time.Time            `json:"lastSuccessAt"`
	LastFailureAt      time.Time            `json:"lastFailureAt"`
	LastError          string               `json:"lastError,omitempty"`
	LatestBatchID      string               `json:"latestBatchId,omitempty"`
	LastEventType      string               `json:"lastEventType,omitempty"`
	LatestEventAt      time.Time            `json:"latestEventAt"`
	EventCount         int                  `json:"eventCount"`
	Target             *SourceBindingTarget `json:"target,omitempty"`
}

type Summary struct {
	Collection         *CollectionProgress  `json:"collection,omitempty"`
	HubUserID          string               `json:"hubUserId"`
	State              string               `json:"state"`
	FreshnessState     string               `json:"freshnessState"`
	TotalEvents        int                  `json:"totalEvents"`
	SourceCount        int                  `json:"sourceCount"`
	LatestEventAt      time.Time            `json:"latestEventAt"`
	LastSuccessAt      time.Time            `json:"lastSuccessAt"`
	LastFailureAt      time.Time            `json:"lastFailureAt"`
	LastFailureMessage string               `json:"lastFailureMessage,omitempty"`
	LatestBatchID      string               `json:"latestBatchId"`
	LastEventType      string               `json:"lastEventType"`
	AggregatedVideos   int                  `json:"aggregatedVideos"`
	AggregatedComments int                  `json:"aggregatedComments"`
	AggregatedTopics   int                  `json:"aggregatedTopics"`
	RecentEvents       []Event              `json:"recentEvents"`
	SourceStatuses     []SourceStatus       `json:"sourceStatuses"`
	RecentRuns         []Run                `json:"recentRuns"`
	BindingStates      []SourceBindingState `json:"bindingStates"`
	OperatorReadiness  OperatorReadiness    `json:"operatorReadiness"`
}
