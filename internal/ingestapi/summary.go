package ingestapi

import (
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

type SummaryResponse struct {
	Collection         *ingest.CollectionProgress `json:"collection,omitempty"`
	HubUserID          string                     `json:"hubUserId"`
	State              string                     `json:"state"`
	FreshnessState     string                     `json:"freshnessState"`
	TotalEvents        int                        `json:"totalEvents"`
	SourceCount        int                        `json:"sourceCount"`
	LatestEventAt      string                     `json:"latestEventAt"`
	LastSuccessAt      string                     `json:"lastSuccessAt"`
	LastFailureAt      string                     `json:"lastFailureAt"`
	LastFailureMessage string                     `json:"lastFailureMessage"`
	LatestBatchID      string                     `json:"latestBatchId"`
	LastEventType      string                     `json:"lastEventType"`
	AggregatedVideos   int                        `json:"aggregatedVideos"`
	AggregatedComments int                        `json:"aggregatedComments"`
	AggregatedTopics   int                        `json:"aggregatedTopics"`
	RecentEvents       []EventResponse            `json:"recentEvents"`
	SourceStatuses     []SourceStatusResponse     `json:"sourceStatuses"`
	RecentRuns         []RunResponse              `json:"recentRuns"`
	BindingStates      []BindingStateResponse     `json:"bindingStates"`
	OperatorReadiness  OperatorReadinessResponse  `json:"operatorReadiness"`
}

type EventResponse struct {
	ID                    string   `json:"id"`
	BatchID               string   `json:"batchId"`
	MachineID             string   `json:"machineId"`
	EventType             string   `json:"eventType"`
	Source                string   `json:"source"`
	CollectedAt           string   `json:"collectedAt"`
	PublishedAt           string   `json:"publishedAt,omitempty"`
	ChannelUsername       string   `json:"channelUsername,omitempty"`
	VideoCount            int      `json:"videoCount"`
	PlayCount             int      `json:"playCount"`
	LikeCount             int      `json:"likeCount"`
	CommentCount          int      `json:"commentCount"`
	FavCount              int      `json:"favCount"`
	ForwardCount          int      `json:"forwardCount"`
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

type SourceStatusResponse struct {
	Source             string                 `json:"source"`
	State              string                 `json:"state"`
	FreshnessState     string                 `json:"freshnessState"`
	SyncFreshnessState string                 `json:"syncFreshnessState"`
	LastRunAt          string                 `json:"lastRunAt"`
	LastSuccessAt      string                 `json:"lastSuccessAt"`
	LastFailureAt      string                 `json:"lastFailureAt"`
	LastError          string                 `json:"lastError"`
	LatestBatchID      string                 `json:"latestBatchId"`
	LastEventType      string                 `json:"lastEventType"`
	LatestEventAt      string                 `json:"latestEventAt"`
	EventCount         int                    `json:"eventCount"`
	Target             *BindingTargetResponse `json:"target,omitempty"`
}

type RunResponse struct {
	Collection    *ingest.CollectionProgress `json:"collection,omitempty"`
	ID            string                     `json:"id"`
	Source        string                     `json:"source"`
	Status        string                     `json:"status"`
	StartedAt     string                     `json:"startedAt"`
	FinishedAt    string                     `json:"finishedAt"`
	BatchID       string                     `json:"batchId"`
	EventCount    int                        `json:"eventCount"`
	LatestEventAt string                     `json:"latestEventAt"`
	LastEventType string                     `json:"lastEventType"`
	Message       string                     `json:"message"`
}

type BindingStateResponse struct {
	Source            string                     `json:"source"`
	Origin            string                     `json:"origin"`
	TargetKey         string                     `json:"targetKey"`
	Path              string                     `json:"path"`
	BaseURL           string                     `json:"baseURL"`
	Username          string                     `json:"username"`
	Keyword           string                     `json:"keyword"`
	DeviceCount       int                        `json:"deviceCount"`
	UpdatedAt         string                     `json:"updatedAt"`
	LastRunStatus     string                     `json:"lastRunStatus"`
	LastRunAt         string                     `json:"lastRunAt"`
	LastSuccessAt     string                     `json:"lastSuccessAt"`
	LastFailureAt     string                     `json:"lastFailureAt"`
	LastError         string                     `json:"lastError"`
	LatestBatchID     string                     `json:"latestBatchId"`
	LastAutoRunStatus string                     `json:"lastAutoRunStatus"`
	LastAutoRunAt     string                     `json:"lastAutoRunAt"`
	LastAutoSuccessAt string                     `json:"lastAutoSuccessAt"`
	LastAutoBatchID   string                     `json:"lastAutoBatchId"`
	Runnable          bool                       `json:"runnable"`
	CanRetry          bool                       `json:"canRetry"`
	RetryReason       string                     `json:"retryReason"`
	Target            *BindingTargetResponse     `json:"target,omitempty"`
	Diagnostic        *BindingDiagnosticResponse `json:"diagnostic,omitempty"`
}

type BindingTargetResponse struct {
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
	TargetKey         string                     `json:"targetKey"`
	Label             string                     `json:"label"`
	State             string                     `json:"state"`
	Summary           string                     `json:"summary"`
	Action            string                     `json:"action"`
	Path              string                     `json:"path"`
	BaseURL           string                     `json:"baseURL"`
	Username          string                     `json:"username"`
	Keyword           string                     `json:"keyword"`
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
	ResolvedBaseURL               string `json:"resolvedBaseUrl"`
	StatusURL                     string `json:"statusUrl"`
	WSHealthURL                   string `json:"wsHealthUrl"`
	WXChannelStatusError          string `json:"wxChannelStatusError"`
	ConnectedClientCount          *int   `json:"connectedClientCount,omitempty"`
	ReadyClientCount              *int   `json:"readyClientCount,omitempty"`
	FeedReadyClientCount          *int   `json:"feedReadyClientCount,omitempty"`
	HealthStatus                  string `json:"healthStatus"`
	CurrentBrowserProxyAddress    string `json:"currentBrowserProxyAddress"`
	ExpectedProxyAddress          string `json:"expectedProxyAddress"`
	WXChannelProxyMode            string `json:"wxChannelProxyMode"`
	WXChannelProxyAddress         string `json:"wxChannelProxyAddress"`
	WXChannelInjectionChecked     *bool  `json:"wxChannelInjectionChecked,omitempty"`
	WXChannelInjectionStarted     *bool  `json:"wxChannelInjectionStarted,omitempty"`
	WXChannelInjectionError       string `json:"wxChannelInjectionError"`
	WXChannelCertificateInstalled *bool  `json:"wxChannelCertificateInstalled,omitempty"`
	WXChannelCertificateError     string `json:"wxChannelCertificateError"`
	CurrentPagePath               string `json:"currentPagePath"`
	CurrentPageHref               string `json:"currentPageHref"`
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

type BindingStatesResponse struct {
	Items []BindingStateResponse `json:"items"`
}

type OperatorRunResponse struct {
	ID               string                       `json:"id"`
	Status           string                       `json:"status"`
	TriggerMode      string                       `json:"triggerMode"`
	StartedAt        string                       `json:"startedAt"`
	FinishedAt       string                       `json:"finishedAt"`
	WorkspaceID      string                       `json:"workspaceId"`
	LatestBatchID    string                       `json:"latestBatchId"`
	AttemptedSources int                          `json:"attemptedSources"`
	SucceededSources int                          `json:"succeededSources"`
	FailedSources    int                          `json:"failedSources"`
	SkippedSources   int                          `json:"skippedSources"`
	FreshnessState   string                       `json:"freshnessState"`
	ReportHeadline   string                       `json:"reportHeadline"`
	TopVideoTitle    string                       `json:"topVideoTitle"`
	TopTopicKeyword  string                       `json:"topTopicKeyword"`
	Message          string                       `json:"message"`
	Outcomes         []OperatorRunOutcomeResponse `json:"outcomes"`
}

type OperatorRunsResponse struct {
	Items []OperatorRunResponse `json:"items"`
}

type OperatorRunOutcomeResponse struct {
	Source  string `json:"source"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

func NewSummaryResponse(summary ingest.Summary) SummaryResponse {
	return SummaryResponse{
		Collection:         summary.Collection,
		HubUserID:          summary.HubUserID,
		State:              summary.State,
		FreshnessState:     summary.FreshnessState,
		TotalEvents:        summary.TotalEvents,
		SourceCount:        summary.SourceCount,
		LatestEventAt:      formatTime(summary.LatestEventAt),
		LastSuccessAt:      formatTime(summary.LastSuccessAt),
		LastFailureAt:      formatTime(summary.LastFailureAt),
		LastFailureMessage: summary.LastFailureMessage,
		LatestBatchID:      summary.LatestBatchID,
		LastEventType:      summary.LastEventType,
		AggregatedVideos:   summary.AggregatedVideos,
		AggregatedComments: summary.AggregatedComments,
		AggregatedTopics:   summary.AggregatedTopics,
		RecentEvents:       mapEvents(summary.RecentEvents),
		SourceStatuses:     mapSourceStatuses(summary.SourceStatuses),
		RecentRuns:         mapRuns(summary.RecentRuns),
		BindingStates:      mapBindingStates(summary.BindingStates),
		OperatorReadiness:  mapOperatorReadiness(summary.OperatorReadiness),
	}
}

func NewBindingStatesResponse(items []ingest.SourceBindingState) BindingStatesResponse {
	return BindingStatesResponse{
		Items: mapBindingStates(items),
	}
}

func NewOperatorRunsResponse(items []ingest.OperatorRun) OperatorRunsResponse {
	return OperatorRunsResponse{
		Items: mapOperatorRuns(items),
	}
}

func mapEvents(items []ingest.Event) []EventResponse {
	results := make([]EventResponse, 0, len(items))
	for _, item := range items {
		results = append(results, EventResponse{
			ID:                    item.ID,
			BatchID:               item.BatchID,
			MachineID:             item.MachineID,
			EventType:             item.EventType,
			Source:                item.Source,
			CollectedAt:           formatTime(item.CollectedAt),
			PublishedAt:           formatTime(item.PublishedAt),
			ChannelUsername:       item.ChannelUsername,
			VideoCount:            item.VideoCount,
			PlayCount:             item.PlayCount,
			LikeCount:             item.LikeCount,
			CommentCount:          item.CommentCount,
			FavCount:              item.FavCount,
			ForwardCount:          item.ForwardCount,
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

func mapSourceStatuses(items []ingest.SourceStatus) []SourceStatusResponse {
	results := make([]SourceStatusResponse, 0, len(items))
	for _, item := range items {
		results = append(results, SourceStatusResponse{
			Source:             item.Source,
			State:              item.State,
			FreshnessState:     item.FreshnessState,
			SyncFreshnessState: item.SyncFreshnessState,
			LastRunAt:          formatTime(item.LastRunAt),
			LastSuccessAt:      formatTime(item.LastSuccessAt),
			LastFailureAt:      formatTime(item.LastFailureAt),
			LastError:          item.LastError,
			LatestBatchID:      item.LatestBatchID,
			LastEventType:      item.LastEventType,
			LatestEventAt:      formatTime(item.LatestEventAt),
			EventCount:         item.EventCount,
			Target:             mapBindingTarget(item.Target),
		})
	}
	return results
}

func mapRuns(items []ingest.Run) []RunResponse {
	results := make([]RunResponse, 0, len(items))
	for _, item := range items {
		results = append(results, RunResponse{
			ID:            item.ID,
			Collection:    item.Collection,
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

func mapBindingStates(items []ingest.SourceBindingState) []BindingStateResponse {
	results := make([]BindingStateResponse, 0, len(items))
	for _, item := range items {
		results = append(results, BindingStateResponse{
			Source:            item.Source,
			Origin:            item.Origin,
			TargetKey:         item.TargetKey,
			Path:              item.Path,
			BaseURL:           item.BaseURL,
			Username:          item.Username,
			Keyword:           item.Keyword,
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
			Target:            mapBindingTarget(item.Target),
			Diagnostic:        mapBindingDiagnostic(item.Diagnostic),
		})
	}
	return results
}

func mapBindingTarget(item *ingest.SourceBindingTarget) *BindingTargetResponse {
	if item == nil {
		return nil
	}
	return &BindingTargetResponse{
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

func mapOperatorReadiness(item ingest.OperatorReadiness) OperatorReadinessResponse {
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

func mapOperatorReadinessSources(items []ingest.OperatorReadinessSource) []OperatorReadinessSourceResponse {
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
			Keyword:           item.Keyword,
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

func mapBindingDiagnostic(item *ingest.SourceBindingDiagnostic) *BindingDiagnosticResponse {
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

func mapOperatorRuns(items []ingest.OperatorRun) []OperatorRunResponse {
	results := make([]OperatorRunResponse, 0, len(items))
	for _, item := range items {
		results = append(results, OperatorRunResponse{
			ID:               item.ID,
			Status:           item.Status,
			TriggerMode:      item.TriggerMode,
			StartedAt:        formatTime(item.StartedAt),
			FinishedAt:       formatTime(item.FinishedAt),
			WorkspaceID:      item.WorkspaceID,
			LatestBatchID:    item.LatestBatchID,
			AttemptedSources: item.AttemptedSources,
			SucceededSources: item.SucceededSources,
			FailedSources:    item.FailedSources,
			SkippedSources:   item.SkippedSources,
			FreshnessState:   item.FreshnessState,
			ReportHeadline:   item.ReportHeadline,
			TopVideoTitle:    item.TopVideoTitle,
			TopTopicKeyword:  item.TopTopicKeyword,
			Message:          item.Message,
			Outcomes:         mapOperatorRunOutcomes(item.Outcomes),
		})
	}
	return results
}

func mapOperatorRunOutcomes(items []ingest.OperatorRunOutcome) []OperatorRunOutcomeResponse {
	results := make([]OperatorRunOutcomeResponse, 0, len(items))
	for _, item := range items {
		results = append(results, OperatorRunOutcomeResponse{
			Source:  item.Source,
			Status:  item.Status,
			Summary: item.Summary,
		})
	}
	return results
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
