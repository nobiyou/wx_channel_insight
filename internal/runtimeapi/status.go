package runtimeapi

import (
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

type StatusResponse struct {
	Phase                        string                                 `json:"phase"`
	RuntimeProfile               string                                 `json:"runtimeProfile"`
	HubBridgeMode                string                                 `json:"hubBridgeMode"`
	DemoContextEnabled           bool                                   `json:"demoContextEnabled"`
	LaunchReadiness              *LaunchReadinessResponse               `json:"launchReadiness,omitempty"`
	OwnerBoundaryState           string                                 `json:"ownerBoundaryState"`
	OwnerBoundaryEvidence        string                                 `json:"ownerBoundaryEvidence"`
	OwnerBoundarySummary         string                                 `json:"ownerBoundarySummary"`
	OwnerBoundaryAction          string                                 `json:"ownerBoundaryAction"`
	LiveRuntimeClass             string                                 `json:"liveRuntimeClass"`
	LiveRuntimeProofState        string                                 `json:"liveRuntimeProofState"`
	LiveRuntimeSummary           string                                 `json:"liveRuntimeSummary"`
	LiveRuntimeAction            string                                 `json:"liveRuntimeAction"`
	LiveRuntimeRequirements      []LiveRuntimeRequirementResponse       `json:"liveRuntimeRequirements"`
	TrustedRuntimeEvidence       TrustedRuntimeEvidenceResponse         `json:"trustedRuntimeEvidence"`
	DashboardStoreMode           string                                 `json:"dashboardStoreMode"`
	RankingStoreMode             string                                 `json:"rankingStoreMode"`
	CommentInsightStoreMode      string                                 `json:"commentInsightStoreMode"`
	ReportSummaryStoreMode       string                                 `json:"reportSummaryStoreMode"`
	WorkspaceStoreMode           string                                 `json:"workspaceStoreMode"`
	IngestStoreMode              string                                 `json:"ingestStoreMode"`
	PostgresConfigured           bool                                   `json:"postgresConfigured"`
	PostgresReady                bool                                   `json:"postgresReady"`
	RedisConfigured              bool                                   `json:"redisConfigured"`
	RedisReady                   bool                                   `json:"redisReady"`
	ReadCacheConfigured          bool                                   `json:"readCacheConfigured"`
	ReadCacheActive              bool                                   `json:"readCacheActive"`
	ReadCacheScope               []string                               `json:"readCacheScope"`
	ReadCacheHitCount            int64                                  `json:"readCacheHitCount"`
	ReadCacheMissCount           int64                                  `json:"readCacheMissCount"`
	ReadCacheFallbackCount       int64                                  `json:"readCacheFallbackCount"`
	ReadCacheWriteCount          int64                                  `json:"readCacheWriteCount"`
	ReadCacheInvalidateCount     int64                                  `json:"readCacheInvalidateCount"`
	ReadCacheLastHitAt           string                                 `json:"readCacheLastHitAt"`
	ReadCacheLastMissAt          string                                 `json:"readCacheLastMissAt"`
	ReadCacheLastFallbackAt      string                                 `json:"readCacheLastFallbackAt"`
	ReadCacheLastWriteAt         string                                 `json:"readCacheLastWriteAt"`
	ReadCacheLastInvalidateAt    string                                 `json:"readCacheLastInvalidateAt"`
	ReadCacheScopeStats          map[string]ReadCacheScopeStatsResponse `json:"readCacheScopeStats,omitempty"`
	JobsEntrypointState          string                                 `json:"jobsEntrypointState"`
	JobsRuntimeState             string                                 `json:"jobsRuntimeState"`
	JobsLastMode                 string                                 `json:"jobsLastMode"`
	JobsLastStatus               string                                 `json:"jobsLastStatus"`
	JobsLastHubUserID            string                                 `json:"jobsLastHubUserId"`
	JobsLastStartedAt            string                                 `json:"jobsLastStartedAt"`
	JobsLastFinishedAt           string                                 `json:"jobsLastFinishedAt"`
	JobsLastIteration            int                                    `json:"jobsLastIteration"`
	JobsLastIterations           int                                    `json:"jobsLastIterations"`
	JobsLastInterval             string                                 `json:"jobsLastInterval"`
	JobsLastError                string                                 `json:"jobsLastError"`
	JobsLastRequestedCount       int                                    `json:"jobsLastRequestedCount"`
	JobsLastRequestedSources     []string                               `json:"jobsLastRequestedSources,omitempty"`
	JobsLastUpdatedAt            string                                 `json:"jobsLastUpdatedAt"`
	WXChannelBaseURL             string                                 `json:"wxChannelBaseUrl"`
	WXChannelConnected           bool                                   `json:"wxChannelConnected"`
	WXChannelReadyClients        int                                    `json:"wxChannelReadyClients"`
	WXChannelFeedReady           int                                    `json:"wxChannelFeedReadyClients"`
	WXChannelCommentReady        int                                    `json:"wxChannelCommentReadyClients"`
	WXChannelCapabilitiesChecked bool                                   `json:"wxChannelCapabilitiesChecked"`
	WXChannelCapabilitiesReady   bool                                   `json:"wxChannelCapabilitiesReady"`
	WXChannelMissingCapabilities []string                               `json:"wxChannelMissingCapabilities,omitempty"`
	WXChannelCapabilityRoutes    []WXChannelCapabilityRouteResponse     `json:"wxChannelCapabilityRoutes,omitempty"`
	OperatorIdentity             string                                 `json:"operatorIdentity"`
	Notes                        []string                               `json:"notes"`
}

type TrustedRuntimeEvidenceResponse struct {
	State                     string   `json:"state"`
	VerifiedAt                string   `json:"verifiedAt"`
	Source                    string   `json:"source"`
	HubUserID                 string   `json:"hubUserId"`
	APIURL                    string   `json:"apiUrl"`
	WebURL                    string   `json:"webUrl"`
	TrustedBoundaryMode       string   `json:"trustedBoundaryMode"`
	APITrustedContextState    string   `json:"apiTrustedContextState"`
	APIBrowserQueryState      string   `json:"apiBrowserQueryState"`
	APIBrowserQueryStatusCode int      `json:"apiBrowserQueryStatusCode"`
	WebTrustedContextState    string   `json:"webTrustedContextState"`
	WebBrowserQueryState      string   `json:"webBrowserQueryState"`
	Summary                   string   `json:"summary"`
	Notes                     []string `json:"notes"`
}

type LiveRuntimeRequirementResponse struct {
	Label   string `json:"label"`
	Key     string `json:"key"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Action  string `json:"action"`
}

type WXChannelCapabilityRouteResponse struct {
	Name         string `json:"name"`
	Method       string `json:"method"`
	URL          string `json:"url"`
	StatusCode   int    `json:"statusCode"`
	RoutePresent bool   `json:"routePresent"`
	Error        string `json:"error,omitempty"`
}

type LaunchReadinessResponse struct {
	Gates        []LaunchGateResponse `json:"gates"`
	MetCount     int                  `json:"metCount"`
	BlockedCount int                  `json:"blockedCount"`
	UnknownCount int                  `json:"unknownCount"`
	TotalCount   int                  `json:"totalCount"`
	Tone         string               `json:"tone"`
	Headline     string               `json:"headline"`
	Summary      string               `json:"summary"`
}

type LaunchGateResponse struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
	Action   string `json:"action"`
}

type ReadCacheScopeStatsResponse struct {
	HitCount       int64  `json:"hitCount"`
	MissCount      int64  `json:"missCount"`
	FallbackCount  int64  `json:"fallbackCount"`
	WriteCount     int64  `json:"writeCount"`
	LastHitAt      string `json:"lastHitAt"`
	LastMissAt     string `json:"lastMissAt"`
	LastFallbackAt string `json:"lastFallbackAt"`
	LastWriteAt    string `json:"lastWriteAt"`
}

func NewStatusResponse(status runtime.Status) StatusResponse {
	return StatusResponse{
		Phase:                        status.Phase,
		RuntimeProfile:               status.RuntimeProfile,
		HubBridgeMode:                status.HubBridgeMode,
		DemoContextEnabled:           status.DemoContextEnabled,
		LaunchReadiness:              newLaunchReadinessResponse(status.LaunchReadiness),
		OwnerBoundaryState:           status.OwnerBoundaryState,
		OwnerBoundaryEvidence:        status.OwnerBoundaryEvidence,
		OwnerBoundarySummary:         status.OwnerBoundarySummary,
		OwnerBoundaryAction:          status.OwnerBoundaryAction,
		LiveRuntimeClass:             status.LiveRuntimeClass,
		LiveRuntimeProofState:        status.LiveRuntimeProofState,
		LiveRuntimeSummary:           status.LiveRuntimeSummary,
		LiveRuntimeAction:            status.LiveRuntimeAction,
		LiveRuntimeRequirements:      newLiveRuntimeRequirementResponses(status.LiveRuntimeRequirements),
		TrustedRuntimeEvidence:       newTrustedRuntimeEvidenceResponse(status.TrustedRuntimeEvidence),
		DashboardStoreMode:           status.DashboardStoreMode,
		RankingStoreMode:             status.RankingStoreMode,
		CommentInsightStoreMode:      status.CommentInsightStoreMode,
		ReportSummaryStoreMode:       status.ReportSummaryStoreMode,
		WorkspaceStoreMode:           status.WorkspaceStoreMode,
		IngestStoreMode:              status.IngestStoreMode,
		PostgresConfigured:           status.PostgresConfigured,
		PostgresReady:                status.PostgresReady,
		RedisConfigured:              status.RedisConfigured,
		RedisReady:                   status.RedisReady,
		ReadCacheConfigured:          status.ReadCacheConfigured,
		ReadCacheActive:              status.ReadCacheActive,
		ReadCacheScope:               append([]string(nil), status.ReadCacheScope...),
		ReadCacheHitCount:            status.ReadCacheHitCount,
		ReadCacheMissCount:           status.ReadCacheMissCount,
		ReadCacheFallbackCount:       status.ReadCacheFallbackCount,
		ReadCacheWriteCount:          status.ReadCacheWriteCount,
		ReadCacheInvalidateCount:     status.ReadCacheInvalidateCount,
		ReadCacheLastHitAt:           formatTime(status.ReadCacheLastHitAt),
		ReadCacheLastMissAt:          formatTime(status.ReadCacheLastMissAt),
		ReadCacheLastFallbackAt:      formatTime(status.ReadCacheLastFallbackAt),
		ReadCacheLastWriteAt:         formatTime(status.ReadCacheLastWriteAt),
		ReadCacheLastInvalidateAt:    formatTime(status.ReadCacheLastInvalidateAt),
		ReadCacheScopeStats:          newReadCacheScopeStatsResponses(status.ReadCacheScopeStats),
		JobsEntrypointState:          status.JobsEntrypointState,
		JobsRuntimeState:             status.JobsRuntimeState,
		JobsLastMode:                 status.JobsLastMode,
		JobsLastStatus:               status.JobsLastStatus,
		JobsLastHubUserID:            status.JobsLastHubUserID,
		JobsLastStartedAt:            formatTime(status.JobsLastStartedAt),
		JobsLastFinishedAt:           formatTime(status.JobsLastFinishedAt),
		JobsLastIteration:            status.JobsLastIteration,
		JobsLastIterations:           status.JobsLastIterations,
		JobsLastInterval:             status.JobsLastInterval,
		JobsLastError:                status.JobsLastError,
		JobsLastRequestedCount:       status.JobsLastRequestedCount,
		JobsLastRequestedSources:     append([]string(nil), status.JobsLastRequestedSources...),
		JobsLastUpdatedAt:            formatTime(status.JobsLastUpdatedAt),
		WXChannelBaseURL:             status.WXChannelBaseURL,
		WXChannelConnected:           status.WXChannelConnected,
		WXChannelReadyClients:        status.WXChannelReadyClients,
		WXChannelFeedReady:           status.WXChannelFeedReady,
		WXChannelCommentReady:        status.WXChannelCommentReady,
		WXChannelCapabilitiesChecked: status.WXChannelCapabilitiesChecked,
		WXChannelCapabilitiesReady:   status.WXChannelCapabilitiesReady,
		WXChannelMissingCapabilities: append([]string(nil), status.WXChannelMissingCapabilities...),
		WXChannelCapabilityRoutes:    newWXChannelCapabilityRouteResponses(status.WXChannelCapabilityRoutes),
		OperatorIdentity:             status.OperatorIdentity,
		Notes:                        append([]string(nil), status.Notes...),
	}
}

func newLaunchReadinessResponse(readiness *runtime.LaunchReadiness) *LaunchReadinessResponse {
	if readiness == nil {
		return nil
	}
	result := &LaunchReadinessResponse{
		MetCount:     readiness.MetCount,
		BlockedCount: readiness.BlockedCount,
		UnknownCount: readiness.UnknownCount,
		TotalCount:   readiness.TotalCount,
		Tone:         readiness.Tone,
		Headline:     readiness.Headline,
		Summary:      readiness.Summary,
	}
	if len(readiness.Gates) > 0 {
		result.Gates = make([]LaunchGateResponse, 0, len(readiness.Gates))
		for _, item := range readiness.Gates {
			result.Gates = append(result.Gates, LaunchGateResponse{
				Key:      item.Key,
				Label:    item.Label,
				Status:   item.Status,
				Evidence: item.Evidence,
				Action:   item.Action,
			})
		}
	}
	return result
}

func newReadCacheScopeStatsResponses(items map[string]insight.ReadCacheScopeSnapshot) map[string]ReadCacheScopeStatsResponse {
	if len(items) == 0 {
		return nil
	}
	result := make(map[string]ReadCacheScopeStatsResponse, len(items))
	for key, item := range items {
		result[key] = ReadCacheScopeStatsResponse{
			HitCount:       item.HitCount,
			MissCount:      item.MissCount,
			FallbackCount:  item.FallbackCount,
			WriteCount:     item.WriteCount,
			LastHitAt:      formatTime(item.LastHitAt),
			LastMissAt:     formatTime(item.LastMissAt),
			LastFallbackAt: formatTime(item.LastFallbackAt),
			LastWriteAt:    formatTime(item.LastWriteAt),
		}
	}
	return result
}

func newLiveRuntimeRequirementResponses(items []runtime.LiveRuntimeRequirement) []LiveRuntimeRequirementResponse {
	if len(items) == 0 {
		return nil
	}

	result := make([]LiveRuntimeRequirementResponse, 0, len(items))
	for _, item := range items {
		result = append(result, LiveRuntimeRequirementResponse{
			Label:   item.Label,
			Key:     item.Key,
			Status:  item.Status,
			Summary: item.Summary,
			Action:  item.Action,
		})
	}
	return result
}

func newWXChannelCapabilityRouteResponses(items []runtime.WXChannelCapabilityRoute) []WXChannelCapabilityRouteResponse {
	if len(items) == 0 {
		return nil
	}
	result := make([]WXChannelCapabilityRouteResponse, 0, len(items))
	for _, item := range items {
		result = append(result, WXChannelCapabilityRouteResponse{
			Name:         item.Name,
			Method:       item.Method,
			URL:          item.URL,
			StatusCode:   item.StatusCode,
			RoutePresent: item.RoutePresent,
			Error:        item.Error,
		})
	}
	return result
}

func newTrustedRuntimeEvidenceResponse(evidence runtime.TrustedRuntimeEvidence) TrustedRuntimeEvidenceResponse {
	return TrustedRuntimeEvidenceResponse{
		State:                     evidence.State,
		VerifiedAt:                formatTime(evidence.VerifiedAt),
		Source:                    evidence.Source,
		HubUserID:                 evidence.HubUserID,
		APIURL:                    evidence.APIURL,
		WebURL:                    evidence.WebURL,
		TrustedBoundaryMode:       evidence.TrustedBoundaryMode,
		APITrustedContextState:    evidence.APITrustedContextState,
		APIBrowserQueryState:      evidence.APIBrowserQueryState,
		APIBrowserQueryStatusCode: evidence.APIBrowserQueryStatusCode,
		WebTrustedContextState:    evidence.WebTrustedContextState,
		WebBrowserQueryState:      evidence.WebBrowserQueryState,
		Summary:                   evidence.Summary,
		Notes:                     append([]string(nil), evidence.Notes...),
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
