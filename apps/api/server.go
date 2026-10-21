package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/edge"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestapi"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	"github.com/nobiyou/wx_channel_insight/internal/monitoringapi"
	operatorflow "github.com/nobiyou/wx_channel_insight/internal/operator"
	"github.com/nobiyou/wx_channel_insight/internal/prompt"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
	"github.com/nobiyou/wx_channel_insight/internal/runtimeapi"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspaceapi"
	"github.com/nobiyou/wx_channel_insight/internal/wxchannel"
)

type server struct {
	schedulerMu                sync.Mutex
	schedulerPolls             map[string]schedulerPoll
	dashboardService           *insight.DashboardService
	readModelRefreshService    readModelRefreshService
	readModelService           *insight.ReadModelService
	dataCenterService          *insight.DataCenterService
	ingestService              ingestService
	operatorRunner             operatorRunner
	monitoringService          *monitoring.Service
	accountService             accountService
	accountMonitor             accountMonitorRunner
	accountBackfillMu          sync.Mutex
	accountBackfills           map[string]bool
	metricRefreshMu            sync.Mutex
	metricRefreshes            map[string]metricRefreshState
	metricRefreshLocker        operatorflow.CycleLocker
	contextResolver            workspaceContextResolver
	readCache                  *insight.ReadCache
	runtimeStatus              runtime.Status
	jobsStatePath              string
	webStaticRoot              string
	trustedRuntimeEvidenceRoot string
	bridgeConfig               runtime.BridgeConfig
	corsOrigins                []string
	hubSessionResolver         hubSessionResolver
	wxChannelBaseURL           string
	edgeHub                    *edge.Hub
	videoPlaybackProfiles      *videoPlaybackProfileCache
	promptRepository           prompt.Repository
	aiServiceManager           *insightai.ServiceSettingsManager
	dailyReportRepo            insight.DailyReportRepository
	aiAnalysisRunner           moduleAnalysisRunner
	aiAnalysisRepository       insight.AIAnalysisRepository
}

type workspaceContextResolver interface {
	Resolve(ctx context.Context, query workspace.ContextQuery) (workspace.ResolvedContext, error)
}

type ingestService interface {
	GetSummary(ctx context.Context, query ingest.SummaryQuery) (ingest.Summary, error)
	ListEvents(ctx context.Context, query ingest.SummaryQuery) ([]ingest.Event, error)
	ListVideoMetricObservations(ctx context.Context, query ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error)
	ListLatestVideoMetricObservations(ctx context.Context, query ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error)
	ListRawComments(ctx context.Context, query ingest.SummaryQuery, rawQuery ingest.RawCommentQuery) (ingest.RawCommentPage, error)
	ExportRawComments(ctx context.Context, query ingest.SummaryQuery, rawQuery ingest.RawCommentQuery, dst io.Writer) error
	GetVideoCommentSync(ctx context.Context, query ingest.SummaryQuery, accountID, videoObjectID string) (ingest.VideoCommentSyncState, bool, error)
	ListBindings(ctx context.Context, query ingest.SummaryQuery) ([]ingest.SourceBinding, error)
	DeleteBinding(ctx context.Context, query ingest.SummaryQuery, source, targetKey string) error
	ListOperatorRuns(ctx context.Context, query ingest.SummaryQuery) ([]ingest.OperatorRun, error)
	AppendMockBatch(ctx context.Context, query ingest.SummaryQuery) (ingest.Summary, error)
	ImportEvents(ctx context.Context, query ingest.SummaryQuery, events []ingest.Event) (ingest.Summary, error)
	ImportWXCommentExport(ctx context.Context, query ingest.SummaryQuery, path string) (ingest.Summary, error)
	ImportWXCommentExportByAPI(ctx context.Context, query ingest.SummaryQuery, baseURL, objectID, nonceID, title, author, clientID string) (ingest.Summary, error)
	ImportWXCommentExportByAPIForAccount(ctx context.Context, query ingest.SummaryQuery, accountID, baseURL, objectID, nonceID, title, author, clientID string) (ingest.Summary, error)
	SaveWXCommentExportBinding(ctx context.Context, query ingest.SummaryQuery, path string) (ingest.Summary, error)
	SearchWXFeedAccounts(ctx context.Context, query ingest.SummaryQuery, baseURL, keyword, clientID string) ([]ingest.WXFeedAccount, error)
	ImportWXFeedList(ctx context.Context, query ingest.SummaryQuery, baseURL, username, clientID string) (ingest.Summary, error)
	ImportWXFeedListForAccount(ctx context.Context, query ingest.SummaryQuery, username, clientID string) (ingest.WXFeedListResult, ingest.Summary, error)
	SaveWXFeedListBinding(ctx context.Context, query ingest.SummaryQuery, baseURL, username string) (ingest.Summary, error)
	SearchWXVideos(ctx context.Context, query ingest.SummaryQuery, baseURL, keyword, clientID string) (ingest.WXFeedListResult, error)
	ImportWXVideoSearch(ctx context.Context, query ingest.SummaryQuery, baseURL, keyword, clientID string) (ingest.Summary, error)
	SaveWXVideoSearchBinding(ctx context.Context, query ingest.SummaryQuery, baseURL, keyword string) (ingest.Summary, error)
	ImportWXVideoShare(ctx context.Context, query ingest.SummaryQuery, baseURL, shareURL, clientID string) (ingest.WXFeedListItem, ingest.Summary, error)
	FetchWXVideoProfile(ctx context.Context, objectID, nonceID string) (ingest.WXFeedListItem, error)
	RetryLastFailedImport(ctx context.Context, query ingest.SummaryQuery, source string) (ingest.Summary, error)
	Reset(ctx context.Context, query ingest.SummaryQuery) (ingest.Summary, error)
}

type readOnlyIngestService interface {
	GetSummaryReadOnly(ctx context.Context, query ingest.SummaryQuery) (ingest.Summary, error)
}

type readModelRefreshService interface {
	RefreshDashboard(ctx context.Context, query insight.DashboardQuery, materializedBy string) (insight.Dashboard, error)
}

type accountService interface {
	List(context.Context, insight.AccountQuery) (insight.AccountPage, error)
	Get(context.Context, insight.AccountQuery, string) (insight.Account, error)
	FindByChannelUsername(context.Context, insight.AccountQuery, string) (insight.Account, error)
	UpsertObservedProfile(context.Context, insight.AccountQuery, insight.AccountObservedProfile) (insight.Account, error)
	UpdateManualProfile(context.Context, insight.AccountQuery, string, insight.AccountProfilePatch) (insight.Account, error)
	SetPriority(context.Context, insight.AccountQuery, string, bool) (insight.Account, error)
}

type accountMonitorRunner interface {
	Run(context.Context, ingest.SummaryQuery, monitoring.TaskTarget) (operatorflow.AccountMonitorResult, error)
	RunTask(context.Context, ingest.SummaryQuery, monitoring.TaskTarget, string) (operatorflow.AccountMonitorResult, error)
	RefreshVideoComments(context.Context, ingest.SummaryQuery, string, ingest.WXFeedListItem) (operatorflow.AccountMonitorResult, error)
}

type operatorRunner interface {
	RunCycle(ctx context.Context, options operatorflow.Options) (operatorflow.Result, error)
}

const (
	apiOperatorLoopDefaultIterations = 1
	apiOperatorLoopMaxIterations     = 3
	apiOperatorLoopDefaultInterval   = 5 * time.Second
	apiOperatorLoopMaxInterval       = time.Minute
)

type operatorRunRequest struct {
	Sources []string                `json:"sources"`
	Targets []monitoring.BindingRef `json:"targets,omitempty"`
	Strict  bool                    `json:"strict"`
}

type operatorLoopRequest struct {
	Sources    []string                `json:"sources"`
	Targets    []monitoring.BindingRef `json:"targets,omitempty"`
	Strict     bool                    `json:"strict"`
	Iterations *int                    `json:"iterations"`
	Interval   string                  `json:"interval"`
}

type operatorLoopConfig struct {
	Iterations int
	Interval   time.Duration
}

func newServer(dashboardService *insight.DashboardService, readModelService *insight.ReadModelService, ingestService ingestService, contextResolver workspaceContextResolver, runtimeStatus runtime.Status) *server {
	return &server{
		dashboardService:      dashboardService,
		readModelService:      readModelService,
		ingestService:         ingestService,
		contextResolver:       contextResolver,
		runtimeStatus:         runtimeStatus,
		accountBackfills:      map[string]bool{},
		videoPlaybackProfiles: newVideoPlaybackProfileCache(),
	}
}

func newServerWithRefresh(
	dashboardService *insight.DashboardService,
	readModelRefreshService readModelRefreshService,
	readModelService *insight.ReadModelService,
	ingestService ingestService,
	contextResolver workspaceContextResolver,
	runtimeStatus runtime.Status,
) *server {
	return &server{
		dashboardService:        dashboardService,
		readModelRefreshService: readModelRefreshService,
		readModelService:        readModelService,
		ingestService:           ingestService,
		contextResolver:         contextResolver,
		runtimeStatus:           runtimeStatus,
		accountBackfills:        map[string]bool{},
		videoPlaybackProfiles:   newVideoPlaybackProfileCache(),
	}
}

func newServerWithOperator(
	dashboardService *insight.DashboardService,
	readModelRefreshService readModelRefreshService,
	readModelService *insight.ReadModelService,
	ingestService ingestService,
	operatorRunner operatorRunner,
	contextResolver workspaceContextResolver,
	runtimeStatus runtime.Status,
) *server {
	return &server{
		dashboardService:        dashboardService,
		readModelRefreshService: readModelRefreshService,
		readModelService:        readModelService,
		ingestService:           ingestService,
		operatorRunner:          operatorRunner,
		contextResolver:         contextResolver,
		runtimeStatus:           runtimeStatus,
		accountBackfills:        map[string]bool{},
		videoPlaybackProfiles:   newVideoPlaybackProfileCache(),
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", s.handleHealth)
	// Always register edge HTTP routes so disabled mode returns a structured JSON body
	// instead of a bare mux 404 that confuses the web console.
	mux.HandleFunc("/api/v1/edge/nodes", s.handleEdgeNodes)
	mux.HandleFunc("/api/v1/edge/call", s.handleEdgeCall)
	mux.HandleFunc("/api/v1/dashboard", s.handleDashboard)
	mux.HandleFunc(aiAnalysisRoutePrefix, s.handleAIAnalysis)
	mux.HandleFunc(aiAnalysisRoutePrefix+"/", s.handleAIAnalysis)
	mux.HandleFunc("/api/v1/settings/prompts", s.handlePromptSettings)
	mux.HandleFunc("/api/v1/settings/prompts/", s.handlePromptSettings)
	mux.HandleFunc(aiServiceSettingsRoute, s.handleAIServiceSettings)
	mux.HandleFunc(aiServiceSettingsRoute+"/", s.handleAIServiceSettings)
	mux.HandleFunc("/api/v1/read-models/hot-videos", s.handleHotVideos)
	mux.HandleFunc(dataCenterRoutePrefix, s.handleDataCenter)
	mux.HandleFunc(operationsAgentRoute, s.handleOperationsAgent)
	mux.HandleFunc(operationsAgentRoute+"/", s.handleOperationsAgent)
	mux.HandleFunc("/api/v1/accounts", s.handleAccounts)
	mux.HandleFunc("/api/v1/accounts/", s.handleAccounts)
	mux.HandleFunc("/api/v1/read-models/videos/play/", s.handleVideoPlayback)
	mux.HandleFunc("/api/v1/read-models/videos/", s.handleVideoDetail)
	mux.HandleFunc("/api/v1/read-models/comments/export", s.handleRawCommentExport)
	mux.HandleFunc("/api/v1/read-models/hot-topics", s.handleHotTopics)
	mux.HandleFunc("/api/v1/read-models/account-recommendations", s.handleAccountRecommendations)
	mux.HandleFunc("/api/v1/read-models/video-recommendations", s.handleVideoRecommendations)
	mux.HandleFunc("/api/v1/read-models/comment-insights", s.handleCommentInsights)
	mux.HandleFunc("/api/v1/read-models/report-summary", s.handleReportSummary)
	mux.HandleFunc("/api/v1/reports", s.handleDailyReports)
	mux.HandleFunc("/api/v1/reports/", s.handleDailyReports)
	mux.HandleFunc("/api/v1/dashboard/replace", s.handleReplaceDashboard)
	mux.HandleFunc("/api/v1/dashboard/reset", s.handleResetDashboard)
	mux.HandleFunc("/api/v1/workspace/context", s.handleWorkspaceContext)
	mux.HandleFunc("/api/v1/runtime/status", s.handleRuntimeStatus)
	mux.HandleFunc("/api/v1/ingest/summary", s.handleIngestSummary)
	mux.HandleFunc("/api/v1/ingest/bindings", s.handleIngestBindings)
	mux.HandleFunc("/api/v1/ingest/operator-runs", s.handleOperatorRuns)
	mux.HandleFunc("/api/v1/operator/run-cycle", s.handleRunOperatorCycle)
	mux.HandleFunc("/api/v1/operator/run-loop", s.handleRunOperatorLoop)
	mux.HandleFunc("/api/v1/monitoring/tasks", s.handleMonitoringTasks)
	mux.HandleFunc("/api/v1/monitoring/tasks/", s.handleMonitoringTasks)
	mux.HandleFunc("/api/v1/monitoring/scheduler/tick", s.handleMonitoringSchedulerTick)
	mux.HandleFunc("/api/v1/monitoring/health", s.handleMonitoringHealth)
	mux.HandleFunc("/api/v1/monitoring/refresh-metrics", s.handleMetricRefresh)
	mux.HandleFunc("/api/v1/ingest/mock-batch", s.handleAppendMockBatch)
	mux.HandleFunc("/api/v1/ingest/events/import", s.handleImportEvents)
	mux.HandleFunc("/api/v1/ingest/wx-channel/comment-export/import", s.handleImportWXCommentExport)
	mux.HandleFunc("/api/v1/ingest/wx-channel/comment-export/sync", s.handleSyncWXCommentExport)
	mux.HandleFunc("/api/v1/ingest/wx-channel/comment-export/save", s.handleSaveWXCommentExportBinding)
	mux.HandleFunc("/api/v1/ingest/wx-channel/feed-list/search", s.handleSearchWXFeedAccounts)
	mux.HandleFunc("/api/v1/ingest/wx-channel/feed-list/import", s.handleImportWXFeedList)
	mux.HandleFunc("/api/v1/ingest/wx-channel/feed-list/save", s.handleSaveWXFeedListBinding)
	mux.HandleFunc("/api/v1/ingest/wx-channel/video-search/search", s.handleSearchWXVideos)
	mux.HandleFunc("/api/v1/ingest/wx-channel/video-search/import", s.handleImportWXVideoSearch)
	mux.HandleFunc("/api/v1/ingest/wx-channel/video-search/save", s.handleSaveWXVideoSearchBinding)
	mux.HandleFunc("/api/v1/ingest/wx-channel/video-share/import", s.handleImportWXVideoShare)
	mux.HandleFunc("/api/v1/ingest/retry", s.handleRetryIngest)
	mux.HandleFunc("/api/v1/ingest/reset", s.handleResetIngest)

	// Edge WS must not go through trusted-bridge auth middleware.
	root := http.NewServeMux()
	if s.edgeHub != nil {
		root.HandleFunc("/ws/client", s.edgeHub.ServeWS)
	}
	root.HandleFunc("/api/healthz", s.handleHealth)
	apiHandler := withHubContextAndSession(mux, s.bridgeConfig, s.hubSessionResolver)
	if strings.TrimSpace(s.webStaticRoot) == "" {
		root.Handle("/", apiHandler)
	} else {
		staticHandler := http.FileServer(http.Dir(s.webStaticRoot))
		root.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
				apiHandler.ServeHTTP(w, r)
				return
			}
			staticHandler.ServeHTTP(w, r)
		}))
	}
	return withCORS(root, s.corsOrigins)
}

func (s *server) handleEdgeNodes(w http.ResponseWriter, r *http.Request) {
	if s.edgeHub == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "edge gateway disabled"})
		return
	}
	s.edgeHub.HandleListNodes(w, r)
}

func (s *server) handleEdgeCall(w http.ResponseWriter, r *http.Request) {
	if s.edgeHub == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "edge gateway disabled"})
		return
	}
	s.edgeHub.HandleRemoteCall(w, r)
}

func (s *server) liveRuntimeStatus() runtime.Status {
	status := s.runtimeStatus
	if s.edgeHub != nil {
		sum := s.edgeHub.Summary()
		status = runtime.WithEdgeSummary(status, sum.Enabled, sum.OnlineNodes, sum.ReadyNodes, sum.FeedNodes)
	}
	return status
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "wx_channel_insight-api",
	})
}

func dashboardQueryFromRequest(r *http.Request) (insight.DashboardQuery, error) {
	reqContext := hubContextFromRequest(r)
	window := ""
	if r != nil && r.URL != nil {
		window = r.URL.Query().Get("window")
	}
	if normalized, ok := insight.NormalizeTimeWindow(window); ok {
		window = normalized
	} else {
		return insight.DashboardQuery{}, insight.ErrInvalidTimeWindow
	}
	return insight.DashboardQuery{
		HubUserID:     reqContext.HubUserID,
		DeviceCount:   reqContext.DeviceCount,
		ContextSource: reqContext.ContextSource,
		TimeWindow:    window,
	}, nil
}

func validateTimeWindowRequest(r *http.Request) error {
	_, err := dashboardQueryFromRequest(r)
	return err
}

func (s *server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	query, err := dashboardQueryFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	payload, err := s.dashboardService.GetReadOnlyDashboard(r.Context(), query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewDashboardPageResponse(payload))
}

func (s *server) handleHotVideos(w http.ResponseWriter, r *http.Request) {
	if err := validateTimeWindowRequest(r); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ranking, ok := insight.NormalizeHotVideoRanking(r.URL.Query().Get("ranking"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported hot video ranking"})
		return
	}
	limit, err := parseRecommendationLimit(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	items, err := s.loadHotVideosByRanking(r, ranking)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	viewStatus, err := s.loadReadModelViewStatus(r, "videos")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewRankedHotVideosResponse(items, ranking, viewStatus))
}

func (s *server) handleHotTopics(w http.ResponseWriter, r *http.Request) {
	if err := validateTimeWindowRequest(r); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	limit, err := parseRecommendationLimit(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	items, err := s.loadHotTopics(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	viewStatus, err := s.loadReadModelViewStatus(r, "topics")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewHotTopicsResponse(items, viewStatus))
}

func (s *server) handleAccountRecommendations(w http.ResponseWriter, r *http.Request) {
	if err := validateTimeWindowRequest(r); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	items, snapshot, limit, err := s.loadAccountRecommendations(r)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errInvalidRecommendationLimit) {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, insightapi.NewAccountRecommendationsResponse(items, snapshot))
}

func (s *server) handleVideoRecommendations(w http.ResponseWriter, r *http.Request) {
	if err := validateTimeWindowRequest(r); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	items, snapshot, limit, err := s.loadVideoRecommendations(r)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errInvalidRecommendationLimit) {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, insightapi.NewVideoRecommendationsResponse(items, snapshot))
}

func (s *server) handleCommentInsights(w http.ResponseWriter, r *http.Request) {
	if err := validateTimeWindowRequest(r); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	limit, err := parseRecommendationLimit(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	items, err := s.loadCommentInsights(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	viewStatus, err := s.loadReadModelViewStatus(r, "comments")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewCommentInsightsResponse(items, viewStatus))
}

func (s *server) handleReportSummary(w http.ResponseWriter, r *http.Request) {
	if err := validateTimeWindowRequest(r); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	item, err := s.loadReportSummary(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	viewStatus, err := s.loadReadModelViewStatus(r, "reports")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewReportSummaryOnlyResponse(item, viewStatus))
}

func (s *server) handleWorkspaceContext(w http.ResponseWriter, r *http.Request) {
	if s.contextResolver == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "workspace context resolver is not configured",
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	resolved, err := s.contextResolver.Resolve(r.Context(), workspace.ContextQuery{
		HubUserID:     reqContext.HubUserID,
		DeviceCount:   reqContext.DeviceCount,
		ContextSource: reqContext.ContextSource,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, workspaceapi.NewContextResponse(resolved))
}

func (s *server) loadHotVideos(r *http.Request) ([]insight.HotVideo, error) {
	return s.loadHotVideosByRanking(r, insight.HotVideoRankingHot)
}

func (s *server) loadHotVideosByRanking(r *http.Request, ranking string) ([]insight.HotVideo, error) {
	if s.readModelService == nil {
		return nil, errors.New("read model service is not configured")
	}
	query, err := dashboardQueryFromRequest(r)
	if err != nil {
		return nil, err
	}
	return s.readModelService.GetHotVideosByRanking(r.Context(), query, ranking)
}

func (s *server) loadHotTopics(r *http.Request) ([]insight.HotTopic, error) {
	if s.readModelService == nil {
		return nil, errors.New("read model service is not configured")
	}
	query, err := dashboardQueryFromRequest(r)
	if err != nil {
		return nil, err
	}
	return s.readModelService.GetHotTopics(r.Context(), query)
}

var (
	errInvalidRecommendationLimit = errors.New("limit must be between 1 and 100")
)

func (s *server) loadAccountRecommendations(r *http.Request) ([]insight.AccountRecommendation, insight.RecommendationSnapshot, int, error) {
	if s.readModelService == nil {
		return nil, insight.RecommendationSnapshot{}, 0, errors.New("read model service is not configured")
	}
	limit, err := parseRecommendationLimit(r)
	if err != nil {
		return nil, insight.RecommendationSnapshot{}, 0, err
	}
	query, err := dashboardQueryFromRequest(r)
	if err != nil {
		return nil, insight.RecommendationSnapshot{}, 0, err
	}
	items, snapshot, err := s.readModelService.GetAccountRecommendations(r.Context(), query)
	return items, snapshot, limit, err
}

func (s *server) loadVideoRecommendations(r *http.Request) ([]insight.VideoRecommendation, insight.RecommendationSnapshot, int, error) {
	if s.readModelService == nil {
		return nil, insight.RecommendationSnapshot{}, 0, errors.New("read model service is not configured")
	}
	limit, err := parseRecommendationLimit(r)
	if err != nil {
		return nil, insight.RecommendationSnapshot{}, 0, err
	}
	query, err := dashboardQueryFromRequest(r)
	if err != nil {
		return nil, insight.RecommendationSnapshot{}, 0, err
	}
	items, snapshot, err := s.readModelService.GetVideoRecommendations(r.Context(), query)
	return items, snapshot, limit, err
}

func parseRecommendationLimit(r *http.Request) (int, error) {
	if r == nil || r.URL == nil {
		return 0, nil
	}
	value := strings.TrimSpace(r.URL.Query().Get("limit"))
	if value == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 100 {
		return 0, errInvalidRecommendationLimit
	}
	return limit, nil
}

func (s *server) loadCommentInsights(r *http.Request) ([]insight.CommentInsight, error) {
	if s.readModelService == nil {
		return nil, errors.New("read model service is not configured")
	}
	query, err := dashboardQueryFromRequest(r)
	if err != nil {
		return nil, err
	}
	return s.readModelService.GetCommentInsights(r.Context(), query)
}

func (s *server) loadReportSummary(r *http.Request) (insight.ReportSummary, error) {
	if s.readModelService == nil {
		return insight.ReportSummary{}, errors.New("read model service is not configured")
	}
	query, err := dashboardQueryFromRequest(r)
	if err != nil {
		return insight.ReportSummary{}, err
	}
	return s.readModelService.GetReportSummary(r.Context(), query)
}

func (s *server) loadReadModelViewStatus(r *http.Request, key string) (*insight.ReadModelViewStatus, error) {
	if s.readModelService == nil {
		return nil, errors.New("read model service is not configured")
	}
	query, err := dashboardQueryFromRequest(r)
	if err != nil {
		return nil, err
	}
	item, found, err := s.readModelService.GetViewStatus(r.Context(), query, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &item, nil
}

func (s *server) handleRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	status := s.currentRuntimeStatusForRequest(r)
	response := runtimeapi.NewStatusResponse(status)
	writeJSON(w, http.StatusOK, response)
}

func (s *server) currentRuntimeStatus(ctx context.Context) runtime.Status {
	status := s.liveRuntimeStatus()
	if s.wxChannelBaseURL != "" {
		status.WXChannelBaseURL = s.wxChannelBaseURL
	}
	if status.WXChannelBaseURL != "" {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		client := wxchannel.NewStatusClient(status.WXChannelBaseURL, nil)
		if pageStatus, err := client.Fetch(probeCtx); err == nil {
			status.WXChannelConnected = pageStatus.Connected
			status.WXChannelReadyClients = pageStatus.ReadyClients
			status.WXChannelFeedReady = pageStatus.FeedReadyClients
			status.WXChannelCommentReady = pageStatus.CommentReadyClients
			status = runtime.WithWXChannelCapabilities(status, mapWXChannelCapabilities(client.FetchCapabilities(probeCtx)))
			if status.OperatorIdentity == "" {
				status.OperatorIdentity = status.JobsLastHubUserID
			}
		}
		cancel()
	}
	if s.readCache != nil {
		status = runtime.WithReadCacheSnapshot(status, s.readCache.Snapshot())
	}
	if s.jobsStatePath != "" {
		if jobsState, err := runtime.LoadJobsRuntimeState(s.jobsStatePath); err == nil {
			status = runtime.WithJobsRuntimeState(status, jobsState)
		}
	}
	if s.trustedRuntimeEvidenceRoot != "" {
		status = runtime.WithTrustedRuntimeEvidence(status, runtime.LoadTrustedRuntimeEvidence(s.trustedRuntimeEvidenceRoot))
	}
	return status
}

func mapWXChannelCapabilities(capabilities wxchannel.CapabilityStatus) runtime.WXChannelCapabilityStatus {
	routes := make([]runtime.WXChannelCapabilityRoute, 0, len(capabilities.Routes))
	for _, route := range capabilities.Routes {
		routes = append(routes, runtime.WXChannelCapabilityRoute{
			Name:         route.Name,
			Method:       route.Method,
			URL:          route.URL,
			StatusCode:   route.StatusCode,
			RoutePresent: route.RoutePresent,
			Error:        route.Error,
		})
	}
	return runtime.WXChannelCapabilityStatus{
		BaseURL: capabilities.BaseURL,
		Checked: capabilities.Checked,
		Ready:   capabilities.Ready,
		Missing: append([]string(nil), capabilities.Missing...),
		Routes:  routes,
	}
}

func (s *server) currentRuntimeStatusForRequest(r *http.Request) runtime.Status {
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	status := s.currentRuntimeStatus(ctx)
	if s.dashboardService == nil || r == nil {
		return status
	}
	reqContext := hubContextFromRequest(r)
	dashboard, err := s.dashboardService.GetLaunchReadinessDashboard(r.Context(), insight.DashboardQuery{
		HubUserID:     reqContext.HubUserID,
		DeviceCount:   reqContext.DeviceCount,
		ContextSource: reqContext.ContextSource,
	})
	if err != nil {
		return status
	}
	if s.monitoringService == nil {
		return runtime.WithLaunchReadiness(status, dashboard)
	}
	monitoringHealth, healthErr := s.monitoringService.GetAutomationHealth(r.Context(), reqContext.HubUserID)
	if healthErr != nil {
		monitoringHealth = monitoring.AutomationHealth{Checked: true, Error: healthErr.Error()}
	}
	return runtime.WithLaunchReadinessMonitoring(status, dashboard, monitoringHealth)
}

func (s *server) handleReplaceDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dashboardService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "dashboard service is not configured",
		})
		return
	}

	var payload insightapi.ReplaceDashboardRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	reqContext := hubContextFromRequest(r)
	if payload.Workspace.Owner == "" {
		payload.Workspace.Owner = reqContext.HubUserID
	}
	if payload.WorkspaceBridge.WorkspaceID == "" && s.contextResolver != nil {
		resolved, err := s.contextResolver.Resolve(r.Context(), workspace.ContextQuery{
			HubUserID:     reqContext.HubUserID,
			DeviceCount:   reqContext.DeviceCount,
			ContextSource: reqContext.ContextSource,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}
		payload.WorkspaceBridge.WorkspaceID = resolved.Workspace.ID
		payload.WorkspaceBridge.HubUserID = resolved.Workspace.OwnerUserID
		payload.WorkspaceBridge.ContextSource = resolved.ContextSource
	}

	if err := s.dashboardService.ReplaceDashboard(r.Context(), insight.Dashboard(payload)); err != nil {
		writeDashboardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *server) handleResetDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dashboardService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "dashboard service is not configured",
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	if err := s.dashboardService.ResetDashboardForQuery(r.Context(), insight.DashboardQuery{
		HubUserID:     reqContext.HubUserID,
		DeviceCount:   reqContext.DeviceCount,
		ContextSource: reqContext.ContextSource,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *server) handleIngestSummary(w http.ResponseWriter, r *http.Request) {
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	query := ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}
	var summary ingest.Summary
	var err error
	if readOnly, ok := s.ingestService.(readOnlyIngestService); ok {
		summary, err = readOnly.GetSummaryReadOnly(r.Context(), query)
	} else {
		summary, err = s.ingestService.GetSummary(r.Context(), query)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleIngestBindings(w http.ResponseWriter, r *http.Request) {
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	if r.Method == http.MethodDelete {
		source := firstQuery(r, "source")
		targetKey := firstQuery(r, "target_key", "targetKey", "target")
		if err := s.ingestService.DeleteBinding(r.Context(), ingest.SummaryQuery{
			HubUserID:   reqContext.HubUserID,
			DeviceCount: reqContext.DeviceCount,
		}, source, targetKey); err != nil {
			if errors.Is(err, ingest.ErrSourceBindingNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "required") {
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":           "deleted",
			"source":           source,
			"targetKey":        targetKey,
			"historyPreserved": true,
		})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	summary, err := s.ingestService.GetSummary(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewBindingStatesResponse(summary.BindingStates))
}

func (s *server) handleOperatorRuns(w http.ResponseWriter, r *http.Request) {
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}
	limit, err := parseOperatorRunsLimit(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	items, err := s.ingestService.ListOperatorRuns(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	writeJSON(w, http.StatusOK, ingestapi.NewOperatorRunsResponse(items))
}

func parseOperatorRunsLimit(r *http.Request) (int, error) {
	if r == nil || r.URL == nil {
		return 0, nil
	}
	value := strings.TrimSpace(r.URL.Query().Get("limit"))
	if value == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 100 {
		return 0, errors.New("limit must be between 1 and 100")
	}
	return limit, nil
}

func (s *server) handleRunOperatorCycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.operatorRunner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "operator runner is not configured",
		})
		return
	}

	var payload operatorRunRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	requestedSources, err := operatorflow.NormalizeRequestedSources(payload.Sources)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	targetRefs, err := operatorflow.NormalizeTargetRefs(payload.Targets)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	options := operatorflow.Options{
		HubUserID:        reqContext.HubUserID,
		DeviceCount:      reqContext.DeviceCount,
		RequestedSources: requestedSources,
		TargetRefs:       targetRefs,
		Strict:           payload.Strict,
		TriggerMode:      ingest.OperatorTriggerCycle,
	}
	result, runErr := s.operatorRunner.RunCycle(r.Context(), options)
	if runErr != nil {
		if errors.Is(runErr, operatorflow.ErrCycleAlreadyRunning) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"status": "skipped",
				"error":  runErr.Error(),
			})
			return
		}
		var executionErr operatorflow.ExecutionError
		if !errors.As(runErr, &executionErr) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": runErr.Error(),
			})
			return
		}
	}

	s.saveOperatorJobsState(result, options, runErr, operatorflow.JobsStateOptions{
		Mode:       "operator-cycle",
		Iteration:  1,
		Iterations: 1,
	})

	status := s.currentRuntimeStatus(r.Context())
	payloadMap := operatorflow.BuildRunPayload(result, status)
	if runErr != nil {
		if payload.Strict {
			writeJSON(w, http.StatusUnprocessableEntity, payloadMap)
			return
		}
		writeJSON(w, http.StatusOK, payloadMap)
		return
	}
	writeJSON(w, http.StatusOK, payloadMap)
}

func (s *server) handleRunOperatorLoop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.operatorRunner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "operator runner is not configured",
		})
		return
	}

	var payload operatorLoopRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	requestedSources, err := operatorflow.NormalizeRequestedSources(payload.Sources)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	targetRefs, err := operatorflow.NormalizeTargetRefs(payload.Targets)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	loopCfg, err := normalizeOperatorLoopConfig(payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	options := operatorflow.Options{
		HubUserID:        reqContext.HubUserID,
		DeviceCount:      reqContext.DeviceCount,
		RequestedSources: requestedSources,
		TargetRefs:       targetRefs,
		Strict:           payload.Strict,
		TriggerMode:      ingest.OperatorTriggerLoop,
	}
	intervalText := operatorLoopIntervalText(loopCfg.Interval)

	for iteration := 1; iteration <= loopCfg.Iterations; iteration++ {
		result, runErr := s.operatorRunner.RunCycle(r.Context(), options)
		if runErr != nil {
			if errors.Is(runErr, operatorflow.ErrCycleAlreadyRunning) {
				writeJSON(w, http.StatusConflict, map[string]string{
					"status": "skipped",
					"error":  runErr.Error(),
				})
				return
			}
			var executionErr operatorflow.ExecutionError
			if !errors.As(runErr, &executionErr) {
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": runErr.Error(),
				})
				return
			}
		}

		s.saveOperatorJobsState(result, options, runErr, operatorflow.JobsStateOptions{
			Mode:       "operator-loop",
			Iteration:  iteration,
			Iterations: loopCfg.Iterations,
			Interval:   intervalText,
		})

		payloadMap := operatorflow.BuildRunPayload(result, s.currentRuntimeStatus(r.Context()))
		payloadMap = annotateOperatorLoopPayload(payloadMap, iteration, loopCfg.Iterations, loopCfg.Interval, result.StartedAt)
		if runErr != nil && payload.Strict {
			writeJSON(w, http.StatusUnprocessableEntity, payloadMap)
			return
		}
		if iteration >= loopCfg.Iterations {
			writeJSON(w, http.StatusOK, payloadMap)
			return
		}
		if err := sleepOperatorLoop(r.Context(), loopCfg.Interval); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}
	}
}

func normalizeOperatorLoopConfig(payload operatorLoopRequest) (operatorLoopConfig, error) {
	config := operatorLoopConfig{
		Iterations: apiOperatorLoopDefaultIterations,
	}

	if payload.Iterations != nil {
		value := *payload.Iterations
		if value < 1 {
			return config, errors.New("iterations must be 1 or greater")
		}
		if value > apiOperatorLoopMaxIterations {
			return config, errors.New("iterations must be 3 or less")
		}
		config.Iterations = value
	}

	interval := strings.TrimSpace(payload.Interval)
	if interval == "" {
		if config.Iterations > 1 {
			config.Interval = apiOperatorLoopDefaultInterval
		}
		return config, nil
	}

	value, err := time.ParseDuration(interval)
	if err != nil {
		return config, err
	}
	if value <= 0 {
		return config, errors.New("interval must be greater than 0")
	}
	if value > apiOperatorLoopMaxInterval {
		return config, errors.New("interval must be 1m or less")
	}
	config.Interval = value
	return config, nil
}

func annotateOperatorLoopPayload(payload map[string]any, iteration, iterations int, interval time.Duration, startedAt time.Time) map[string]any {
	if payload == nil {
		payload = map[string]any{}
	}

	loop := map[string]any{
		"iteration":  iteration,
		"iterations": iterations,
	}
	if !startedAt.IsZero() {
		loop["startedAt"] = startedAt.UTC()
	}
	if intervalText := operatorLoopIntervalText(interval); intervalText != "" {
		loop["interval"] = intervalText
	}
	if iterations > 0 {
		loop["remainingIterations"] = iterations - iteration
	}
	payload["loop"] = loop
	return payload
}

func operatorLoopIntervalText(interval time.Duration) string {
	if interval <= 0 {
		return ""
	}
	return interval.String()
}

func sleepOperatorLoop(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return nil
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *server) saveOperatorJobsState(result operatorflow.Result, options operatorflow.Options, runErr error, stateOptions operatorflow.JobsStateOptions) {
	if s.jobsStatePath == "" {
		return
	}
	state := operatorflow.BuildJobsRuntimeState(result, options, runErr, stateOptions)
	_ = runtime.SaveJobsRuntimeState(s.jobsStatePath, state)
}

func (s *server) handleAppendMockBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.AppendMockBatch(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if err := s.refreshDashboardAfterIngest(r.Context(), reqContext); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleImportEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	var payload struct {
		Events []ingest.Event `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.ImportEvents(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, payload.Events)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if err := s.refreshDashboardAfterIngest(r.Context(), reqContext); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleImportWXCommentExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	var payload struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.ImportWXCommentExport(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, payload.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if err := s.refreshDashboardAfterIngest(r.Context(), reqContext); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleSyncWXCommentExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	var payload struct {
		BaseURL  string `json:"baseUrl"`
		ObjectID string `json:"objectId"`
		NonceID  string `json:"nonceId"`
		Title    string `json:"title"`
		Author   string `json:"author"`
		ClientID string `json:"clientId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.ImportWXCommentExportByAPI(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, payload.BaseURL, payload.ObjectID, payload.NonceID, payload.Title, payload.Author, payload.ClientID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if err := s.refreshDashboardAfterIngest(r.Context(), reqContext); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleImportWXFeedList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	var payload struct {
		BaseURL  string `json:"baseUrl"`
		Username string `json:"username"`
		ClientID string `json:"clientId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.ImportWXFeedList(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, payload.BaseURL, payload.Username, payload.ClientID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if err := s.refreshDashboardAfterIngest(r.Context(), reqContext); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleSearchWXVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	keyword := r.URL.Query().Get("keyword")
	result, err := s.ingestService.SearchWXVideos(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, r.URL.Query().Get("base_url"), keyword, firstQuery(r, "client_id", "clientId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewWXVideoSearchResponse(keyword, result))
}

func (s *server) handleImportWXVideoSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	var payload struct {
		BaseURL  string `json:"baseUrl"`
		Keyword  string `json:"keyword"`
		ClientID string `json:"clientId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.ImportWXVideoSearch(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, payload.BaseURL, payload.Keyword, payload.ClientID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if err := s.refreshDashboardAfterIngest(r.Context(), reqContext); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleImportWXVideoShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	var payload struct {
		BaseURL  string `json:"baseUrl"`
		ShareURL string `json:"shareUrl"`
		ClientID string `json:"clientId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(payload.ShareURL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "shareUrl is required"})
		return
	}

	reqContext := hubContextFromRequest(r)
	item, summary, err := s.ingestService.ImportWXVideoShare(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, payload.BaseURL, payload.ShareURL, payload.ClientID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.refreshDashboardAfterIngest(r.Context(), reqContext); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewWXManualVideoResponse(payload.ShareURL, item, summary))
}

func (s *server) handleSaveWXCommentExportBinding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	var payload struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.SaveWXCommentExportBinding(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, payload.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleSearchWXFeedAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	items, err := s.ingestService.SearchWXFeedAccounts(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, r.URL.Query().Get("base_url"), r.URL.Query().Get("keyword"), firstQuery(r, "client_id", "clientId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if s.accountService != nil {
		for _, item := range items {
			if strings.TrimSpace(item.Username) == "" {
				continue
			}
			if _, saveErr := s.accountService.UpsertObservedProfile(r.Context(), insight.AccountQuery{
				HubUserID: reqContext.HubUserID, DeviceCount: reqContext.DeviceCount, ContextSource: "account_search",
			}, insight.AccountObservedProfile{
				Platform:        insight.AccountPlatformWXChannels,
				ChannelUsername: item.Username,
				ChannelName:     item.Nickname,
				Signature:       item.Signature,
				AvatarURL:       item.HeadURL,
				Region:          item.Region,
				Industry:        item.Industry,
				Tags:            append([]string(nil), item.Tags...),
				ObservedAt:      time.Now().UTC(),
			}); saveErr != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": "保存账号画像失败: " + saveErr.Error(),
				})
				return
			}
		}
	}

	writeJSON(w, http.StatusOK, ingestapi.NewWXFeedAccountsResponse(items))
}

func (s *server) handleSaveWXFeedListBinding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	var payload struct {
		BaseURL  string `json:"baseUrl"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.SaveWXFeedListBinding(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, payload.BaseURL, payload.Username)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleSaveWXVideoSearchBinding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	var payload struct {
		BaseURL string `json:"baseUrl"`
		Keyword string `json:"keyword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.SaveWXVideoSearchBinding(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, payload.BaseURL, payload.Keyword)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleRetryIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	var payload struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.RetryLastFailedImport(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, payload.Source)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if err := s.refreshDashboardAfterIngest(r.Context(), reqContext); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) handleResetIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ingest service is not configured",
		})
		return
	}

	reqContext := hubContextFromRequest(r)
	summary, err := s.ingestService.Reset(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}
	if err := s.refreshDashboardAfterIngest(r.Context(), reqContext); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ingestapi.NewSummaryResponse(summary))
}

func (s *server) refreshDashboardAfterIngest(ctx context.Context, reqContext hubRequestContext) error {
	if s.readModelRefreshService == nil {
		return nil
	}
	_, err := s.readModelRefreshService.RefreshDashboard(ctx, insight.DashboardQuery{
		HubUserID:     reqContext.HubUserID,
		DeviceCount:   reqContext.DeviceCount,
		ContextSource: reqContext.ContextSource,
	}, "api")
	return err
}

func writeDashboardError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if isInvalidDashboardSnapshot(err) {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}

func isInvalidDashboardSnapshot(err error) bool {
	return errors.Is(err, insight.ErrInvalidDashboardSnapshot)
}

func (s *server) handleMonitoringTasks(w http.ResponseWriter, r *http.Request) {
	if s.monitoringService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "monitoring service is not configured"})
		return
	}
	const prefix = "/api/v1/monitoring/tasks"
	relative := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(strings.Trim(relative, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		s.handleMonitoringTaskCollection(w, r)
		return
	}
	if len(parts) < 1 || strings.TrimSpace(parts[0]) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "monitoring task route not found"})
		return
	}
	taskID := parts[0]
	hub := hubContextFromRequest(r)
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		task, err := s.monitoringService.GetTask(r.Context(), hub.HubUserID, taskID)
		if err != nil {
			writeMonitoringError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, monitoringapi.NewTaskResponse(task))
	case len(parts) == 1 && r.Method == http.MethodPatch:
		var payload monitoringapi.TaskRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		task, err := payload.Apply(monitoring.Task{ID: taskID})
		if err != nil {
			writeMonitoringError(w, err)
			return
		}
		updated, err := s.monitoringService.UpdateTask(r.Context(), hub.HubUserID, task)
		if err != nil {
			writeMonitoringError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, monitoringapi.NewTaskResponse(updated))
	case len(parts) == 1 && r.Method == http.MethodDelete:
		if err := s.monitoringService.ArchiveTask(r.Context(), hub.HubUserID, taskID); err != nil {
			writeMonitoringError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": monitoring.StatusArchived, "id": taskID})
	case len(parts) == 2 && parts[1] == "runs" && r.Method == http.MethodGet:
		limit := parseIntOrZero(r.URL.Query().Get("limit"))
		runs, err := s.monitoringService.ListTaskRuns(r.Context(), hub.HubUserID, taskID, limit)
		if err != nil {
			writeMonitoringError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": monitoringapi.NewTaskRunsResponse(runs)})
	case len(parts) == 2 && parts[1] == "run" && r.Method == http.MethodPost:
		run, err := s.monitoringService.RunManualTask(r.Context(), hub.HubUserID, taskID)
		writeMonitoringRunResult(w, run, err)
	case len(parts) == 2 && parts[1] == "retry" && r.Method == http.MethodPost:
		run, err := s.monitoringService.RetryTask(r.Context(), hub.HubUserID, taskID)
		writeMonitoringRunResult(w, run, err)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method or monitoring task route not allowed"})
	}
}

func (s *server) handleMonitoringTaskCollection(w http.ResponseWriter, r *http.Request) {
	hub := hubContextFromRequest(r)
	switch r.Method {
	case http.MethodGet:
		includeArchived := strings.EqualFold(firstQuery(r, "includeArchived", "include_archived"), "true")
		tasks, err := s.monitoringService.ListTasks(r.Context(), hub.HubUserID, includeArchived)
		if err != nil {
			writeMonitoringError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, monitoringapi.NewTasksResponse(tasks))
	case http.MethodPost:
		var payload monitoringapi.TaskRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		task, err := payload.Apply(monitoring.Task{})
		if err != nil {
			writeMonitoringError(w, err)
			return
		}
		created, err := s.monitoringService.CreateTask(r.Context(), hub.HubUserID, task)
		if err != nil {
			writeMonitoringError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, monitoringapi.NewTaskResponse(created))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *server) handleMonitoringSchedulerTick(w http.ResponseWriter, r *http.Request) {
	if s.monitoringService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "monitoring service is not configured"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	hub, err := s.resolveMonitoringContext(r)
	if err != nil {
		writeMonitoringError(w, err)
		return
	}
	finish := s.beginSchedulerPoll(hub.HubUserID)
	defer finish()
	run, err := s.monitoringService.RunDueTask(r.Context(), hub.HubUserID)
	if errors.Is(err, monitoring.ErrNoDueTask) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "idle"})
		return
	}
	if err != nil && run.ID == "" {
		writeMonitoringError(w, err)
		return
	}
	payload := map[string]any{
		"status": "completed",
		"run":    monitoringapi.NewTaskRunResponse(run),
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, payload)
}

func writeMonitoringRunResult(w http.ResponseWriter, run monitoring.TaskRun, err error) {
	if err != nil && run.ID == "" {
		writeMonitoringError(w, err)
		return
	}
	payload := map[string]any{"run": monitoringapi.NewTaskRunResponse(run)}
	if err != nil {
		payload["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, payload)
}

func writeMonitoringError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, monitoring.ErrTaskNotFound):
		status = http.StatusNotFound
	case errors.Is(err, monitoring.ErrTaskArchived), errors.Is(err, monitoring.ErrTaskRunInProgress), errors.Is(err, monitoring.ErrNoRetryableRun):
		status = http.StatusConflict
	case errors.Is(err, monitoring.ErrSystemTaskReadOnly):
		status = http.StatusForbidden
	case errors.Is(err, monitoring.ErrOperatorNotReady):
		status = http.StatusServiceUnavailable
	case strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), "unsupported"), strings.Contains(err.Error(), "duplicate"), strings.Contains(err.Error(), "interval"):
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func firstQuery(r *http.Request, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(r.URL.Query().Get(key)); v != "" {
			return v
		}
	}
	return ""
}
