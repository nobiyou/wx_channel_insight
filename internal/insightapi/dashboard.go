package insightapi

import (
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type DashboardResponse struct {
	GeneratedAt            string                                   `json:"generatedAt"`
	ReadModel              ReadModelStateResponse                   `json:"readModel"`
	Workspace              WorkspaceSummaryResponse                 `json:"workspace"`
	WorkspaceBridge        WorkspaceBridgeResponse                  `json:"workspaceBridge"`
	IngestSummary          IngestSummaryResponse                    `json:"ingestSummary"`
	Overview               OverviewMetricsResponse                  `json:"overview"`
	HotVideos              []HotVideoResponse                       `json:"hotVideos"`
	HotVideoRankings       map[string][]HotVideoResponse            `json:"hotVideoRankings,omitempty"`
	HotVideoWindowRankings map[string]map[string][]HotVideoResponse `json:"hotVideoWindowRankings,omitempty"`
	HotTopics              []HotTopicResponse                       `json:"hotTopics"`
	AccountRecommendations []AccountRecommendationResponse          `json:"accountRecommendations"`
	VideoRecommendations   []VideoRecommendationResponse            `json:"videoRecommendations"`
	WatchlistAlerts        []WatchlistAlertResponse                 `json:"watchlistAlerts"`
	CommentInsights        []CommentInsightResponse                 `json:"commentInsights"`
	ReportSummary          ReportSummaryResponse                    `json:"reportSummary"`
}

const (
	dashboardRecommendationLimit = insight.DashboardPageRecommendationLimit
	dashboardHotVideoLimit       = insight.DashboardPageHotVideoLimit
	dashboardHotTopicLimit       = insight.DashboardPageHotTopicLimit
	dashboardNestedDetailLimit   = insight.DashboardPageNestedDetailLimit
)

type ReadModelStateResponse struct {
	Mode           string                        `json:"mode"`
	MaterializedAt string                        `json:"materializedAt"`
	Source         string                        `json:"source"`
	LatestBatchID  string                        `json:"latestBatchId"`
	ViewStatuses   []ReadModelViewStatusResponse `json:"viewStatuses"`
}

type ReadModelViewStatusResponse struct {
	Key            string `json:"key"`
	Label          string `json:"label"`
	FreshnessState string `json:"freshnessState"`
	Summary        string `json:"summary"`
	SourceNote     string `json:"sourceNote"`
	LatestBatchID  string `json:"latestBatchId"`
	MaterializedAt string `json:"materializedAt"`
}

type WorkspaceSummaryResponse struct {
	Name        string `json:"name"`
	Owner       string `json:"owner"`
	DeviceCount int    `json:"deviceCount"`
}

type WorkspaceBridgeResponse struct {
	WorkspaceID   string `json:"workspaceId"`
	HubUserID     string `json:"hubUserId"`
	ContextSource string `json:"contextSource"`
	BootstrapMode string `json:"bootstrapMode"`
	Source        string `json:"source"`
	State         string `json:"state"`
	Resolution    string `json:"resolution"`
	LastSyncedAt  string `json:"lastSyncedAt"`
}

type IngestSummaryResponse struct {
	State              string                       `json:"state"`
	FreshnessState     string                       `json:"freshnessState"`
	TotalEvents        int                          `json:"totalEvents"`
	SourceCount        int                          `json:"sourceCount"`
	LatestEventAt      string                       `json:"latestEventAt"`
	LastSuccessAt      string                       `json:"lastSuccessAt"`
	LastFailureAt      string                       `json:"lastFailureAt"`
	LastFailureMessage string                       `json:"lastFailureMessage"`
	LatestBatchID      string                       `json:"latestBatchId"`
	LastEventType      string                       `json:"lastEventType"`
	AggregatedVideos   int                          `json:"aggregatedVideos"`
	AggregatedComments int                          `json:"aggregatedComments"`
	AggregatedTopics   int                          `json:"aggregatedTopics"`
	RecentEvents       []IngestEventResponse        `json:"recentEvents"`
	SourceStatuses     []IngestSourceStatusResponse `json:"sourceStatuses"`
	RecentRuns         []IngestRunResponse          `json:"recentRuns"`
	BindingStates      []IngestBindingStateResponse `json:"bindingStates"`
	OperatorReadiness  OperatorReadinessResponse    `json:"operatorReadiness"`
}

type IngestEventResponse struct {
	ID                    string   `json:"id"`
	BatchID               string   `json:"batchId"`
	MachineID             string   `json:"machineId"`
	EventType             string   `json:"eventType"`
	Source                string   `json:"source"`
	CollectedAt           string   `json:"collectedAt"`
	VideoCount            int      `json:"videoCount"`
	CommentCount          int      `json:"commentCount"`
	TopicCount            int      `json:"topicCount"`
	Title                 string   `json:"title"`
	ChannelName           string   `json:"channelName"`
	Keyword               string   `json:"keyword"`
	Cluster               string   `json:"cluster"`
	Signal                string   `json:"signal"`
	PromptAngle           string   `json:"promptAngle"`
	RepresentativeComment string   `json:"representativeComment"`
	TargetAudience        string   `json:"targetAudience"`
	Tags                  []string `json:"tags"`
	Keywords              []string `json:"keywords"`
}

type IngestSourceStatusResponse struct {
	Source         string                       `json:"source"`
	State          string                       `json:"state"`
	FreshnessState string                       `json:"freshnessState"`
	LastRunAt      string                       `json:"lastRunAt"`
	LastSuccessAt  string                       `json:"lastSuccessAt"`
	LastFailureAt  string                       `json:"lastFailureAt"`
	LastError      string                       `json:"lastError"`
	LatestBatchID  string                       `json:"latestBatchId"`
	LastEventType  string                       `json:"lastEventType"`
	EventCount     int                          `json:"eventCount"`
	Target         *IngestBindingTargetResponse `json:"target,omitempty"`
}

type IngestRunResponse struct {
	ID            string `json:"id"`
	Source        string `json:"source"`
	Status        string `json:"status"`
	StartedAt     string `json:"startedAt"`
	FinishedAt    string `json:"finishedAt"`
	BatchID       string `json:"batchId"`
	EventCount    int    `json:"eventCount"`
	LatestEventAt string `json:"latestEventAt"`
	LastEventType string `json:"lastEventType"`
	Message       string `json:"message"`
}

type IngestBindingStateResponse struct {
	Source            string                       `json:"source"`
	Origin            string                       `json:"origin"`
	TargetKey         string                       `json:"targetKey,omitempty"`
	Path              string                       `json:"path"`
	BaseURL           string                       `json:"baseURL"`
	Username          string                       `json:"username"`
	DeviceCount       int                          `json:"deviceCount"`
	UpdatedAt         string                       `json:"updatedAt"`
	LastRunStatus     string                       `json:"lastRunStatus"`
	LastRunAt         string                       `json:"lastRunAt"`
	LastSuccessAt     string                       `json:"lastSuccessAt"`
	LastFailureAt     string                       `json:"lastFailureAt"`
	LastError         string                       `json:"lastError"`
	LatestBatchID     string                       `json:"latestBatchId"`
	LastAutoRunStatus string                       `json:"lastAutoRunStatus"`
	LastAutoRunAt     string                       `json:"lastAutoRunAt"`
	LastAutoSuccessAt string                       `json:"lastAutoSuccessAt"`
	LastAutoBatchID   string                       `json:"lastAutoBatchId"`
	Runnable          bool                         `json:"runnable"`
	CanRetry          bool                         `json:"canRetry"`
	RetryReason       string                       `json:"retryReason"`
	Target            *IngestBindingTargetResponse `json:"target,omitempty"`
	Diagnostic        *BindingDiagnosticResponse   `json:"diagnostic,omitempty"`
}

type IngestBindingTargetResponse struct {
	Title           string `json:"title,omitempty"`
	ChannelName     string `json:"channelName,omitempty"`
	ChannelUsername string `json:"channelUsername,omitempty"`
	Keyword         string `json:"keyword,omitempty"`
	Comment         string `json:"comment,omitempty"`
	CommentCount    int    `json:"commentCount,omitempty"`
	ObjectID        string `json:"objectId,omitempty"`
	ObjectNonceID   string `json:"objectNonceId,omitempty"`
	PublishedAt     string `json:"publishedAt,omitempty"`
}

type OperatorReadinessSourceResponse struct {
	Source            string                     `json:"source"`
	TargetKey         string                     `json:"targetKey,omitempty"`
	Label             string                     `json:"label"`
	State             string                     `json:"state"`
	Summary           string                     `json:"summary"`
	Action            string                     `json:"action"`
	Path              string                     `json:"path"`
	BaseURL           string                     `json:"baseURL"`
	Username          string                     `json:"username"`
	DeviceCount       int                        `json:"deviceCount"`
	UpdatedAt         string                     `json:"updatedAt"`
	LastRunStatus     string                     `json:"lastRunStatus"`
	LastSuccessAt     string                     `json:"lastSuccessAt"`
	LastFailureAt     string                     `json:"lastFailureAt"`
	LatestBatchID     string                     `json:"latestBatchId"`
	LastAutoRunStatus string                     `json:"lastAutoRunStatus"`
	LastAutoRunAt     string                     `json:"lastAutoRunAt"`
	LastAutoSuccessAt string                     `json:"lastAutoSuccessAt"`
	LastAutoBatchID   string                     `json:"lastAutoBatchId"`
	Runnable          bool                       `json:"runnable"`
	CanRetry          bool                       `json:"canRetry"`
	RetryReason       string                     `json:"retryReason"`
	Diagnostic        *BindingDiagnosticResponse `json:"diagnostic,omitempty"`
}

type BindingDiagnosticResponse struct {
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

type OperatorReadinessResponse struct {
	State           string                            `json:"state"`
	ExpectedStatus  string                            `json:"expectedStatus"`
	RunnableSources int                               `json:"runnableSources"`
	BlockedSources  int                               `json:"blockedSources"`
	MissingSources  int                               `json:"missingSources"`
	Headline        string                            `json:"headline"`
	Summary         string                            `json:"summary"`
	NextAction      string                            `json:"nextAction"`
	Runnable        []OperatorReadinessSourceResponse `json:"runnable"`
	Blocked         []OperatorReadinessSourceResponse `json:"blocked"`
	Missing         []OperatorReadinessSourceResponse `json:"missing"`
}

type OverviewMetricsResponse struct {
	MonitoredAccounts int     `json:"monitoredAccounts"`
	TrackedVideos     int     `json:"trackedVideos"`
	EmergingTopics    int     `json:"emergingTopics"`
	NewCommentBursts  int     `json:"newCommentBursts"`
	DailyCoverageRate float64 `json:"dailyCoverageRate"`
	ReportFreshness   string  `json:"reportFreshness"`
}

type HotVideoResponse struct {
	ID                  string                       `json:"id"`
	Title               string                       `json:"title"`
	ChannelUsername     string                       `json:"channelUsername,omitempty"`
	ChannelName         string                       `json:"channelName"`
	Source              string                       `json:"source"`
	ObjectID            string                       `json:"objectId,omitempty"`
	ObjectNonceID       string                       `json:"objectNonceId,omitempty"`
	CoverURL            string                       `json:"coverUrl,omitempty"`
	Tags                []string                     `json:"tags"`
	PublishedAt         string                       `json:"publishedAt"`
	ObservedAt          string                       `json:"observedAt"`
	PublishedAgo        string                       `json:"publishedAgo"`
	Views               int                          `json:"views"`
	Likes               int                          `json:"likes"`
	Comments            int                          `json:"comments"`
	Favorites           int                          `json:"favorites"`
	Forwards            int                          `json:"forwards"`
	GrowthRate          float64                      `json:"growthRate,omitempty"`
	InteractionScore    float64                      `json:"interactionScore,omitempty"`
	GrowthScore         float64                      `json:"growthScore,omitempty"`
	RecommendationScore float64                      `json:"recommendationScore,omitempty"`
	PositiveDeltas      RecommendationDeltasResponse `json:"positiveDeltas,omitempty"`
	ObservationCount    int                          `json:"observationCount,omitempty"`
	HasGrowthBaseline   bool                         `json:"hasGrowthBaseline"`
	InsufficientHistory bool                         `json:"insufficientHistory"`
	DataBasis           string                       `json:"dataBasis,omitempty"`
	Reason              string                       `json:"reason,omitempty"`
	EngagementScore     int                          `json:"engagementScore,omitempty"`
	Signal              string                       `json:"signal"`
}

type HotTopicResponse struct {
	ID                  string                       `json:"id"`
	Keyword             string                       `json:"keyword"`
	Cluster             string                       `json:"cluster"`
	Source              string                       `json:"source"`
	Sources             []string                     `json:"sources,omitempty"`
	ObservedAt          string                       `json:"observedAt"`
	SourceAccounts      []string                     `json:"sourceAccounts"`
	SourceAccountCount  int                          `json:"sourceAccountCount"`
	Mentions            int                          `json:"mentions"`
	Comments            int                          `json:"comments"`
	GrowthRate          float64                      `json:"growthRate,omitempty"`
	RelatedVideos       int                          `json:"relatedVideos"`
	InteractionScore    float64                      `json:"interactionScore,omitempty"`
	GrowthScore         float64                      `json:"growthScore,omitempty"`
	RecommendationScore float64                      `json:"recommendationScore,omitempty"`
	PositiveDeltas      RecommendationDeltasResponse `json:"positiveDeltas,omitempty"`
	ObservationCount    int                          `json:"observationCount,omitempty"`
	HasGrowthBaseline   bool                         `json:"hasGrowthBaseline"`
	InsufficientHistory bool                         `json:"insufficientHistory"`
	DataBasis           string                       `json:"dataBasis,omitempty"`
	Reason              string                       `json:"reason,omitempty"`
	PromptAngle         string                       `json:"promptAngle"`
	Keywords            []string                     `json:"keywords"`
}

type WatchlistAlertResponse struct {
	ID           string `json:"id"`
	AccountName  string `json:"accountName"`
	AlertType    string `json:"alertType"`
	Source       string `json:"source"`
	Severity     string `json:"severity"`
	Summary      string `json:"summary"`
	OccurredAt   string `json:"occurredAt"`
	HappenedAt   string `json:"happenedAt"`
	Recommended  string `json:"recommended"`
	VideoCount   int    `json:"videoCount"`
	CommentDelta int    `json:"commentDelta"`
}

type CommentInsightResponse struct {
	ID               string   `json:"id"`
	Theme            string   `json:"theme"`
	Source           string   `json:"source"`
	ObservedAt       string   `json:"observedAt"`
	SourceAccounts   []string `json:"sourceAccounts"`
	QuestionRate     int      `json:"questionRate"`
	Sentiment        string   `json:"sentiment"`
	Representative   string   `json:"representative"`
	ActionSuggestion string   `json:"actionSuggestion"`
	RelatedKeywords  []string `json:"relatedKeywords"`
}

type ReportSummaryResponse struct {
	Period            string `json:"period"`
	Headline          string `json:"headline"`
	KeyFinding        string `json:"keyFinding"`
	RecommendedAction string `json:"recommendedAction"`
	Watchout          string `json:"watchout"`
	GeneratedAt       string `json:"generatedAt"`
	AutoGeneratedAt   string `json:"autoGeneratedAt"`
}

func NewDashboardResponse(payload insight.Dashboard) DashboardResponse {
	return newDashboardResponse(payload, 0)
}

// NewDashboardPageResponse keeps the dashboard transport bounded while the
// dedicated ranking and recommendation endpoints expose their requested limit.
func NewDashboardPageResponse(payload insight.Dashboard) DashboardResponse {
	response := newDashboardResponse(payload, dashboardRecommendationLimit)
	response.HotVideos = limitHotVideoResponses(response.HotVideos, dashboardHotVideoLimit)
	response.HotVideoRankings = limitHotVideoRankingResponses(response.HotVideoRankings, dashboardHotVideoLimit)
	response.HotVideoWindowRankings = limitHotVideoWindowRankingResponses(response.HotVideoWindowRankings, dashboardHotVideoLimit)
	response.HotTopics = limitHotTopicResponses(response.HotTopics, dashboardHotTopicLimit)
	response.IngestSummary.BindingStates = limitIngestBindingStateResponses(response.IngestSummary.BindingStates, dashboardNestedDetailLimit)
	response.IngestSummary.OperatorReadiness.Runnable = limitOperatorReadinessSourceResponses(response.IngestSummary.OperatorReadiness.Runnable, dashboardNestedDetailLimit)
	response.IngestSummary.OperatorReadiness.Blocked = limitOperatorReadinessSourceResponses(response.IngestSummary.OperatorReadiness.Blocked, dashboardNestedDetailLimit)
	response.IngestSummary.OperatorReadiness.Missing = limitOperatorReadinessSourceResponses(response.IngestSummary.OperatorReadiness.Missing, dashboardNestedDetailLimit)
	for index := range response.HotTopics {
		response.HotTopics[index].SourceAccounts = limitStrings(response.HotTopics[index].SourceAccounts, dashboardNestedDetailLimit)
	}
	return response
}

func newDashboardResponse(payload insight.Dashboard, recommendationLimit int) DashboardResponse {
	return DashboardResponse{
		GeneratedAt: formatTime(payload.GeneratedAt),
		ReadModel: ReadModelStateResponse{
			Mode:           payload.ReadModel.Mode,
			MaterializedAt: formatTime(payload.ReadModel.MaterializedAt),
			Source:         payload.ReadModel.Source,
			LatestBatchID:  payload.ReadModel.LatestBatchID,
			ViewStatuses:   mapReadModelViewStatuses(payload.ReadModel.ViewStatuses),
		},
		Workspace: WorkspaceSummaryResponse{
			Name:        payload.Workspace.Name,
			Owner:       payload.Workspace.Owner,
			DeviceCount: payload.Workspace.DeviceCount,
		},
		WorkspaceBridge: WorkspaceBridgeResponse{
			WorkspaceID:   payload.WorkspaceBridge.WorkspaceID,
			HubUserID:     payload.WorkspaceBridge.HubUserID,
			ContextSource: payload.WorkspaceBridge.ContextSource,
			BootstrapMode: payload.WorkspaceBridge.BootstrapMode,
			Source:        payload.WorkspaceBridge.Source,
			State:         payload.WorkspaceBridge.State,
			Resolution:    payload.WorkspaceBridge.Resolution,
			LastSyncedAt:  formatTime(payload.WorkspaceBridge.LastSyncedAt),
		},
		IngestSummary: IngestSummaryResponse{
			State:              payload.IngestSummary.State,
			FreshnessState:     payload.IngestSummary.FreshnessState,
			TotalEvents:        payload.IngestSummary.TotalEvents,
			SourceCount:        payload.IngestSummary.SourceCount,
			LatestEventAt:      formatTime(payload.IngestSummary.LatestEventAt),
			LastSuccessAt:      formatTime(payload.IngestSummary.LastSuccessAt),
			LastFailureAt:      formatTime(payload.IngestSummary.LastFailureAt),
			LastFailureMessage: payload.IngestSummary.LastFailureMessage,
			LatestBatchID:      payload.IngestSummary.LatestBatchID,
			LastEventType:      payload.IngestSummary.LastEventType,
			AggregatedVideos:   payload.IngestSummary.AggregatedVideos,
			AggregatedComments: payload.IngestSummary.AggregatedComments,
			AggregatedTopics:   payload.IngestSummary.AggregatedTopics,
			RecentEvents:       mapIngestEvents(payload.IngestSummary.RecentEvents),
			SourceStatuses:     mapIngestSourceStatuses(payload.IngestSummary.SourceStatuses),
			RecentRuns:         mapIngestRuns(payload.IngestSummary.RecentRuns),
			BindingStates:      mapIngestBindingStates(payload.IngestSummary.BindingStates),
			OperatorReadiness:  mapOperatorReadiness(payload.IngestSummary.OperatorReadiness),
		},
		Overview: OverviewMetricsResponse{
			MonitoredAccounts: payload.Overview.MonitoredAccounts,
			TrackedVideos:     payload.Overview.TrackedVideos,
			EmergingTopics:    payload.Overview.EmergingTopics,
			NewCommentBursts:  payload.Overview.NewCommentBursts,
			DailyCoverageRate: payload.Overview.DailyCoverageRate,
			ReportFreshness:   payload.Overview.ReportFreshness,
		},
		HotVideos:              mapHotVideos(payload.HotVideos),
		HotVideoRankings:       mapHotVideoRankings(payload.HotVideoRankings),
		HotVideoWindowRankings: mapHotVideoWindowRankings(payload.HotVideoWindowRankings),
		HotTopics:              mapHotTopics(payload.HotTopics),
		AccountRecommendations: mapAccountRecommendations(limitAccountRecommendations(payload.AccountRecommendations, recommendationLimit)),
		VideoRecommendations:   mapVideoRecommendations(limitVideoRecommendations(payload.VideoRecommendations, recommendationLimit)),
		WatchlistAlerts:        mapWatchlistAlerts(payload.WatchlistAlerts),
		CommentInsights:        mapCommentInsights(payload.CommentInsights),
		ReportSummary: ReportSummaryResponse{
			Period:            payload.ReportSummary.Period,
			Headline:          payload.ReportSummary.Headline,
			KeyFinding:        payload.ReportSummary.KeyFinding,
			RecommendedAction: payload.ReportSummary.RecommendedAction,
			Watchout:          payload.ReportSummary.Watchout,
			GeneratedAt:       formatTime(payload.ReportSummary.GeneratedAt),
			AutoGeneratedAt:   payload.ReportSummary.AutoGeneratedAt,
		},
	}
}

func limitAccountRecommendations(items []insight.AccountRecommendation, limit int) []insight.AccountRecommendation {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func limitVideoRecommendations(items []insight.VideoRecommendation, limit int) []insight.VideoRecommendation {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func limitHotVideoResponses(items []HotVideoResponse, limit int) []HotVideoResponse {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func limitHotTopicResponses(items []HotTopicResponse, limit int) []HotTopicResponse {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func limitIngestBindingStateResponses(items []IngestBindingStateResponse, limit int) []IngestBindingStateResponse {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func limitOperatorReadinessSourceResponses(items []OperatorReadinessSourceResponse, limit int) []OperatorReadinessSourceResponse {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func limitStrings(items []string, limit int) []string {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func limitHotVideoRankingResponses(source map[string][]HotVideoResponse, limit int) map[string][]HotVideoResponse {
	if source == nil {
		return nil
	}
	result := make(map[string][]HotVideoResponse, len(source))
	for key, items := range source {
		result[key] = limitHotVideoResponses(items, limit)
	}
	return result
}

func limitHotVideoWindowRankingResponses(source map[string]map[string][]HotVideoResponse, limit int) map[string]map[string][]HotVideoResponse {
	if source == nil {
		return nil
	}
	result := make(map[string]map[string][]HotVideoResponse, len(source))
	for window, rankings := range source {
		result[window] = limitHotVideoRankingResponses(rankings, limit)
	}
	return result
}

func mapReadModelViewStatuses(items []insight.ReadModelViewStatus) []ReadModelViewStatusResponse {
	results := make([]ReadModelViewStatusResponse, 0, len(items))
	for _, item := range items {
		results = append(results, ReadModelViewStatusResponse{
			Key:            item.Key,
			Label:          item.Label,
			FreshnessState: item.FreshnessState,
			Summary:        item.Summary,
			SourceNote:     item.SourceNote,
			LatestBatchID:  item.LatestBatchID,
			MaterializedAt: formatTime(item.MaterializedAt),
		})
	}
	return results
}

func mapIngestEvents(items []insight.IngestEvent) []IngestEventResponse {
	results := make([]IngestEventResponse, 0, len(items))
	for _, item := range items {
		results = append(results, IngestEventResponse{
			ID:                    item.ID,
			BatchID:               item.BatchID,
			MachineID:             item.MachineID,
			EventType:             item.EventType,
			Source:                item.Source,
			CollectedAt:           formatTime(item.CollectedAt),
			VideoCount:            item.VideoCount,
			CommentCount:          item.CommentCount,
			TopicCount:            item.TopicCount,
			Title:                 item.Title,
			ChannelName:           item.ChannelName,
			Keyword:               item.Keyword,
			Cluster:               item.Cluster,
			Signal:                item.Signal,
			PromptAngle:           item.PromptAngle,
			RepresentativeComment: item.RepresentativeComment,
			TargetAudience:        item.TargetAudience,
			Tags:                  append([]string(nil), item.Tags...),
			Keywords:              append([]string(nil), item.Keywords...),
		})
	}
	return results
}

func mapIngestSourceStatuses(items []insight.IngestSourceStatus) []IngestSourceStatusResponse {
	results := make([]IngestSourceStatusResponse, 0, len(items))
	for _, item := range items {
		results = append(results, IngestSourceStatusResponse{
			Source:         item.Source,
			State:          item.State,
			FreshnessState: item.FreshnessState,
			LastRunAt:      formatTime(item.LastRunAt),
			LastSuccessAt:  formatTime(item.LastSuccessAt),
			LastFailureAt:  formatTime(item.LastFailureAt),
			LastError:      item.LastError,
			LatestBatchID:  item.LatestBatchID,
			LastEventType:  item.LastEventType,
			EventCount:     item.EventCount,
			Target:         mapIngestBindingTarget(item.Target),
		})
	}
	return results
}

func mapIngestRuns(items []insight.IngestRun) []IngestRunResponse {
	results := make([]IngestRunResponse, 0, len(items))
	for _, item := range items {
		results = append(results, IngestRunResponse{
			ID:            item.ID,
			Source:        item.Source,
			Status:        item.Status,
			StartedAt:     formatTime(item.StartedAt),
			FinishedAt:    formatTime(item.FinishedAt),
			BatchID:       item.BatchID,
			EventCount:    item.EventCount,
			LatestEventAt: formatTime(item.LatestEventAt),
			LastEventType: item.LastEventType,
			Message:       item.Message,
		})
	}
	return results
}

func mapIngestBindingStates(items []insight.IngestBindingState) []IngestBindingStateResponse {
	results := make([]IngestBindingStateResponse, 0, len(items))
	for _, item := range items {
		results = append(results, IngestBindingStateResponse{
			Source:            item.Source,
			Origin:            item.Origin,
			TargetKey:         item.TargetKey,
			Path:              item.Path,
			BaseURL:           item.BaseURL,
			Username:          item.Username,
			DeviceCount:       item.DeviceCount,
			UpdatedAt:         formatTime(item.UpdatedAt),
			LastRunStatus:     item.LastRunStatus,
			LastRunAt:         formatTime(item.LastRunAt),
			LastSuccessAt:     formatTime(item.LastSuccessAt),
			LastFailureAt:     formatTime(item.LastFailureAt),
			LastError:         item.LastError,
			LatestBatchID:     item.LatestBatchID,
			LastAutoRunStatus: item.LastAutoRunStatus,
			LastAutoRunAt:     formatTime(item.LastAutoRunAt),
			LastAutoSuccessAt: formatTime(item.LastAutoSuccessAt),
			LastAutoBatchID:   item.LastAutoBatchID,
			Runnable:          item.Runnable,
			CanRetry:          item.CanRetry,
			RetryReason:       item.RetryReason,
			Target:            mapIngestBindingTarget(item.Target),
			Diagnostic:        mapBindingDiagnostic(item.Diagnostic),
		})
	}
	return results
}

func mapIngestBindingTarget(item *insight.IngestBindingTarget) *IngestBindingTargetResponse {
	if item == nil {
		return nil
	}
	return &IngestBindingTargetResponse{
		Title:           item.Title,
		ChannelName:     item.ChannelName,
		ChannelUsername: item.ChannelUsername,
		Keyword:         item.Keyword,
		Comment:         item.Comment,
		CommentCount:    item.CommentCount,
		ObjectID:        item.ObjectID,
		ObjectNonceID:   item.ObjectNonceID,
		PublishedAt:     formatTime(item.PublishedAt),
	}
}

func mapOperatorReadiness(item insight.OperatorReadiness) OperatorReadinessResponse {
	return OperatorReadinessResponse{
		State:           item.State,
		ExpectedStatus:  item.ExpectedStatus,
		RunnableSources: item.RunnableSources,
		BlockedSources:  item.BlockedSources,
		MissingSources:  item.MissingSources,
		Headline:        item.Headline,
		Summary:         item.Summary,
		NextAction:      item.NextAction,
		Runnable:        mapOperatorReadinessSources(item.Runnable),
		Blocked:         mapOperatorReadinessSources(item.Blocked),
		Missing:         mapOperatorReadinessSources(item.Missing),
	}
}

func mapOperatorReadinessSources(items []insight.OperatorReadinessSource) []OperatorReadinessSourceResponse {
	results := make([]OperatorReadinessSourceResponse, 0, len(items))
	for _, item := range items {
		results = append(results, OperatorReadinessSourceResponse{
			Source:            item.Source,
			TargetKey:         item.TargetKey,
			Label:             item.Label,
			State:             item.State,
			Summary:           item.Summary,
			Action:            item.Action,
			Path:              item.Path,
			BaseURL:           item.BaseURL,
			Username:          item.Username,
			DeviceCount:       item.DeviceCount,
			UpdatedAt:         formatTime(item.UpdatedAt),
			LastRunStatus:     item.LastRunStatus,
			LastSuccessAt:     formatTime(item.LastSuccessAt),
			LastFailureAt:     formatTime(item.LastFailureAt),
			LatestBatchID:     item.LatestBatchID,
			LastAutoRunStatus: item.LastAutoRunStatus,
			LastAutoRunAt:     formatTime(item.LastAutoRunAt),
			LastAutoSuccessAt: formatTime(item.LastAutoSuccessAt),
			LastAutoBatchID:   item.LastAutoBatchID,
			Runnable:          item.Runnable,
			CanRetry:          item.CanRetry,
			RetryReason:       item.RetryReason,
			Diagnostic:        mapBindingDiagnostic(item.Diagnostic),
		})
	}
	return results
}

func mapBindingDiagnostic(item *insight.BindingDiagnostic) *BindingDiagnosticResponse {
	if item == nil {
		return nil
	}
	result := &BindingDiagnosticResponse{
		Code:                       item.Code,
		ResolvedBaseURL:            item.ResolvedBaseURL,
		StatusURL:                  item.StatusURL,
		WSHealthURL:                item.WSHealthURL,
		WXChannelStatusError:       item.WXChannelStatusError,
		HealthStatus:               item.HealthStatus,
		CurrentBrowserProxyAddress: item.CurrentBrowserProxyAddress,
		ExpectedProxyAddress:       item.ExpectedProxyAddress,
		WXChannelProxyMode:         item.WXChannelProxyMode,
		WXChannelProxyAddress:      item.WXChannelProxyAddress,
		WXChannelInjectionError:    item.WXChannelInjectionError,
		WXChannelCertificateError:  item.WXChannelCertificateError,
		CurrentPagePath:            item.CurrentPagePath,
		CurrentPageHref:            item.CurrentPageHref,
	}
	if item.ConnectedClientCount != nil {
		value := *item.ConnectedClientCount
		result.ConnectedClientCount = &value
	}
	if item.ReadyClientCount != nil {
		value := *item.ReadyClientCount
		result.ReadyClientCount = &value
	}
	if item.FeedReadyClientCount != nil {
		value := *item.FeedReadyClientCount
		result.FeedReadyClientCount = &value
	}
	if item.WXChannelInjectionChecked != nil {
		value := *item.WXChannelInjectionChecked
		result.WXChannelInjectionChecked = &value
	}
	if item.WXChannelInjectionStarted != nil {
		value := *item.WXChannelInjectionStarted
		result.WXChannelInjectionStarted = &value
	}
	if item.WXChannelCertificateInstalled != nil {
		value := *item.WXChannelCertificateInstalled
		result.WXChannelCertificateInstalled = &value
	}
	return result
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func mapHotVideos(items []insight.HotVideo) []HotVideoResponse {
	results := make([]HotVideoResponse, 0, len(items))
	for _, item := range items {
		results = append(results, HotVideoResponse{
			ID:                  item.ID,
			Title:               item.Title,
			ChannelUsername:     item.ChannelUsername,
			ChannelName:         item.ChannelName,
			Source:              item.Source,
			ObjectID:            item.ObjectID,
			ObjectNonceID:       item.ObjectNonceID,
			CoverURL:            item.CoverURL,
			Tags:                append([]string(nil), item.Tags...),
			PublishedAt:         formatTime(item.PublishedAt),
			ObservedAt:          formatTime(item.ObservedAt),
			PublishedAgo:        item.PublishedAgo,
			Views:               item.Views,
			Likes:               item.Likes,
			Comments:            item.Comments,
			Favorites:           item.Favorites,
			Forwards:            item.Forwards,
			GrowthRate:          item.GrowthRate,
			InteractionScore:    item.InteractionScore,
			GrowthScore:         item.GrowthScore,
			RecommendationScore: item.RecommendationScore,
			PositiveDeltas:      mapRecommendationDeltas(item.PositiveDeltas),
			ObservationCount:    item.ObservationCount,
			HasGrowthBaseline:   item.HasGrowthBaseline,
			InsufficientHistory: item.InsufficientHistory,
			DataBasis:           item.DataBasis,
			Reason:              item.Reason,
			EngagementScore:     item.EngagementScore,
			Signal:              item.Signal,
		})
	}
	return results
}

func mapHotVideoRankings(source insight.HotVideoRankings) map[string][]HotVideoResponse {
	if source == nil {
		return nil
	}
	result := make(map[string][]HotVideoResponse, len(source))
	for kind, items := range source {
		result[kind] = mapHotVideos(items)
	}
	return result
}

func mapHotVideoWindowRankings(source insight.HotVideoWindowRankings) map[string]map[string][]HotVideoResponse {
	if source == nil {
		return nil
	}
	result := make(map[string]map[string][]HotVideoResponse, len(source))
	for window, rankings := range source {
		result[window] = mapHotVideoRankings(rankings)
	}
	return result
}

func mapHotTopics(items []insight.HotTopic) []HotTopicResponse {
	results := make([]HotTopicResponse, 0, len(items))
	for _, item := range items {
		results = append(results, HotTopicResponse{
			ID:                  item.ID,
			Keyword:             item.Keyword,
			Cluster:             item.Cluster,
			Source:              item.Source,
			Sources:             append([]string(nil), item.Sources...),
			ObservedAt:          formatTime(item.ObservedAt),
			SourceAccounts:      append([]string(nil), item.SourceAccounts...),
			SourceAccountCount:  len(item.SourceAccounts),
			Mentions:            item.Mentions,
			Comments:            item.Comments,
			GrowthRate:          item.GrowthRate,
			RelatedVideos:       item.RelatedVideos,
			InteractionScore:    item.InteractionScore,
			GrowthScore:         item.GrowthScore,
			RecommendationScore: item.RecommendationScore,
			PositiveDeltas:      mapRecommendationDeltas(item.PositiveDeltas),
			ObservationCount:    item.ObservationCount,
			HasGrowthBaseline:   item.HasGrowthBaseline,
			InsufficientHistory: item.InsufficientHistory,
			DataBasis:           item.DataBasis,
			Reason:              item.Reason,
			PromptAngle:         item.PromptAngle,
			Keywords:            append([]string(nil), item.Keywords...),
		})
	}
	return results
}

func mapWatchlistAlerts(items []insight.WatchlistAlert) []WatchlistAlertResponse {
	results := make([]WatchlistAlertResponse, 0, len(items))
	for _, item := range items {
		results = append(results, WatchlistAlertResponse{
			ID:           item.ID,
			AccountName:  item.AccountName,
			AlertType:    item.AlertType,
			Source:       item.Source,
			Severity:     item.Severity,
			Summary:      item.Summary,
			OccurredAt:   formatTime(item.OccurredAt),
			HappenedAt:   item.HappenedAt,
			Recommended:  item.Recommended,
			VideoCount:   item.VideoCount,
			CommentDelta: item.CommentDelta,
		})
	}
	return results
}

func mapCommentInsights(items []insight.CommentInsight) []CommentInsightResponse {
	results := make([]CommentInsightResponse, 0, len(items))
	for _, item := range items {
		results = append(results, CommentInsightResponse{
			ID:               item.ID,
			Theme:            item.Theme,
			Source:           item.Source,
			ObservedAt:       formatTime(item.ObservedAt),
			SourceAccounts:   append([]string(nil), item.SourceAccounts...),
			QuestionRate:     item.QuestionRate,
			Sentiment:        item.Sentiment,
			Representative:   item.Representative,
			ActionSuggestion: item.ActionSuggestion,
			RelatedKeywords:  append([]string(nil), item.RelatedKeywords...),
		})
	}
	return results
}
