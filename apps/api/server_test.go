package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	operatorflow "github.com/nobiyou/wx_channel_insight/internal/operator"
	"github.com/nobiyou/wx_channel_insight/internal/rediscache"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspacedemo"
	redis "github.com/redis/go-redis/v9"
)

func TestRoutesServeConfiguredWebStaticRootAlongsideAPI(t *testing.T) {
	staticRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticRoot, "index.html"), []byte("<html>Insight internal test</html>"), 0o644); err != nil {
		t.Fatalf("write static index: %v", err)
	}

	srv := newServer(nil, nil, nil, nil, runtime.Status{})
	srv.webStaticRoot = staticRoot

	webRec := httptest.NewRecorder()
	srv.routes().ServeHTTP(webRec, httptest.NewRequest(http.MethodGet, "/", nil))
	if webRec.Code != http.StatusOK || !strings.Contains(webRec.Body.String(), "Insight internal test") {
		t.Fatalf("expected static web response, got status=%d body=%q", webRec.Code, webRec.Body.String())
	}

	healthRec := httptest.NewRecorder()
	srv.routes().ServeHTTP(healthRec, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected API health route alongside static web, got status=%d body=%q", healthRec.Code, healthRec.Body.String())
	}
}

type stubDashboardRepo struct {
	payload insight.Dashboard
	err     error
	keys    []string
}

type recordingDashboardRepo struct {
	payload       insight.Dashboard
	replaced      insight.Dashboard
	replacedKey   string
	replaceCalled bool
}

type stubIngestService struct {
	summary              ingest.Summary
	readOnlySummary      ingest.Summary
	hasReadOnlySummary   bool
	summaryCalls         *int
	readOnlySummaryCalls *int
	err                  error
	runs                 []ingest.OperatorRun
	observations         []ingest.VideoMetricObservation
	videoProfile         ingest.WXFeedListItem
	videoProfileErr      error
	accounts             []ingest.WXFeedAccount
	videoSearch          ingest.WXFeedListResult
	rawComments          ingest.RawCommentPage
	exportedJSON         string
	calledBaseURL        string
	calledKeyword        string
	calledClientID       string
}

type stubOperatorRunner struct {
	result          operatorflow.Result
	err             error
	results         []operatorflow.Result
	errs            []error
	recordedOptions *[]operatorflow.Options
	callCount       *int
}

func (r stubDashboardRepo) GetDashboard(context.Context, string) (insight.Dashboard, error) {
	return r.payload, r.err
}

func (r stubDashboardRepo) ReplaceDashboard(context.Context, string, insight.Dashboard) error {
	return r.err
}

func (r stubDashboardRepo) ResetDashboard(context.Context, string) error {
	return r.err
}

func (r *recordingDashboardRepo) GetDashboard(context.Context, string) (insight.Dashboard, error) {
	return r.payload, nil
}

func (r *recordingDashboardRepo) ReplaceDashboard(_ context.Context, workspaceID string, snapshot insight.Dashboard) error {
	r.replacedKey = workspaceID
	r.replaced = snapshot
	r.replaceCalled = true
	r.payload = snapshot
	return nil
}

func (r *recordingDashboardRepo) ResetDashboard(context.Context, string) error {
	return nil
}

func (s stubIngestService) GetSummary(context.Context, ingest.SummaryQuery) (ingest.Summary, error) {
	if s.summaryCalls != nil {
		*s.summaryCalls++
	}
	return s.summary, s.err
}

func (s stubIngestService) GetSummaryReadOnly(context.Context, ingest.SummaryQuery) (ingest.Summary, error) {
	if s.readOnlySummaryCalls != nil {
		*s.readOnlySummaryCalls++
	}
	if s.hasReadOnlySummary {
		return s.readOnlySummary, s.err
	}
	return s.summary, s.err
}

func (s stubIngestService) ListEvents(context.Context, ingest.SummaryQuery) ([]ingest.Event, error) {
	return nil, s.err
}

func (s stubIngestService) ListVideoMetricObservations(context.Context, ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	return append([]ingest.VideoMetricObservation(nil), s.observations...), s.err
}

func (s stubIngestService) ListLatestVideoMetricObservations(ctx context.Context, query ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	return s.ListVideoMetricObservations(ctx, query)
}

func (s stubIngestService) GetVideoCommentSync(context.Context, ingest.SummaryQuery, string, string) (ingest.VideoCommentSyncState, bool, error) {
	return ingest.VideoCommentSyncState{}, false, s.err
}

func (s stubIngestService) ListRawComments(_ context.Context, _ ingest.SummaryQuery, query ingest.RawCommentQuery) (ingest.RawCommentPage, error) {
	if s.err != nil {
		return ingest.RawCommentPage{}, s.err
	}
	return ingest.PageRawComments(s.rawComments.Items, query), nil
}

func (s stubIngestService) ExportRawComments(_ context.Context, _ ingest.SummaryQuery, _ ingest.RawCommentQuery, dst io.Writer) error {
	if s.err != nil {
		return s.err
	}
	payload := s.exportedJSON
	if payload == "" {
		payload = `{"items":[],"total":0}`
	}
	_, err := io.WriteString(dst, payload)
	return err
}

func (s stubIngestService) ListBindings(context.Context, ingest.SummaryQuery) ([]ingest.SourceBinding, error) {
	return nil, s.err
}

func (s stubIngestService) DeleteBinding(context.Context, ingest.SummaryQuery, string, string) error {
	return s.err
}

func (s stubIngestService) ListOperatorRuns(context.Context, ingest.SummaryQuery) ([]ingest.OperatorRun, error) {
	return append([]ingest.OperatorRun(nil), s.runs...), s.err
}

func (s stubIngestService) AppendMockBatch(context.Context, ingest.SummaryQuery) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) ImportEvents(context.Context, ingest.SummaryQuery, []ingest.Event) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) ImportWXCommentExport(context.Context, ingest.SummaryQuery, string) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) ImportWXCommentExportByAPI(context.Context, ingest.SummaryQuery, string, string, string, string, string, string) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) ImportWXCommentExportByAPIForAccount(context.Context, ingest.SummaryQuery, string, string, string, string, string, string, string) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) SaveWXCommentExportBinding(context.Context, ingest.SummaryQuery, string) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) SearchWXFeedAccounts(context.Context, ingest.SummaryQuery, string, string, string) ([]ingest.WXFeedAccount, error) {
	return append([]ingest.WXFeedAccount(nil), s.accounts...), s.err
}

func (s stubIngestService) ImportWXFeedList(context.Context, ingest.SummaryQuery, string, string, string) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) ImportWXFeedListForAccount(context.Context, ingest.SummaryQuery, string, string) (ingest.WXFeedListResult, ingest.Summary, error) {
	return ingest.WXFeedListResult{}, s.summary, s.err
}

func (s stubIngestService) SaveWXFeedListBinding(context.Context, ingest.SummaryQuery, string, string) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) SearchWXVideos(context.Context, ingest.SummaryQuery, string, string, string) (ingest.WXFeedListResult, error) {
	return s.videoSearch, s.err
}

func (s stubIngestService) ImportWXVideoSearch(context.Context, ingest.SummaryQuery, string, string, string) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) SaveWXVideoSearchBinding(context.Context, ingest.SummaryQuery, string, string) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) ImportWXVideoShare(context.Context, ingest.SummaryQuery, string, string, string) (ingest.WXFeedListItem, ingest.Summary, error) {
	return s.videoProfile, s.summary, s.err
}

func (s stubIngestService) FetchWXVideoProfile(context.Context, string, string) (ingest.WXFeedListItem, error) {
	return s.videoProfile, s.videoProfileErr
}

func (s stubIngestService) RetryLastFailedImport(context.Context, ingest.SummaryQuery, string) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubIngestService) Reset(context.Context, ingest.SummaryQuery) (ingest.Summary, error) {
	return s.summary, s.err
}

func (s stubOperatorRunner) RunCycle(_ context.Context, options operatorflow.Options) (operatorflow.Result, error) {
	if s.recordedOptions != nil {
		*s.recordedOptions = append(*s.recordedOptions, options)
	}

	index := 0
	if s.callCount != nil {
		index = *s.callCount
		*s.callCount = *s.callCount + 1
	}

	result := s.result
	if index < len(s.results) {
		result = s.results[index]
	}

	err := s.err
	if index < len(s.errs) {
		err = s.errs[index]
	}

	return result, err
}

func TestHandleOperatorCycleReturnsConflictWhenCycleIsAlreadyRunning(t *testing.T) {
	srv := newServerWithOperator(nil, nil, nil, stubIngestService{}, stubOperatorRunner{err: operatorflow.ErrCycleAlreadyRunning}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/run-cycle", strings.NewReader(`{"strict":true}`))
	srv.handleRunOperatorCycle(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected HTTP 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode conflict payload: %v", err)
	}
	if payload["status"] != "skipped" || payload["error"] != operatorflow.ErrCycleAlreadyRunning.Error() {
		t.Fatalf("expected skipped cycle conflict payload, got %+v", payload)
	}
}

func TestHandleOperatorLoopReturnsConflictWhenCycleIsAlreadyRunning(t *testing.T) {
	srv := newServerWithOperator(nil, nil, nil, stubIngestService{}, stubOperatorRunner{err: operatorflow.ErrCycleAlreadyRunning}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/run-loop", strings.NewReader(`{"iterations":1,"strict":true}`))
	srv.handleRunOperatorLoop(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected HTTP 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode conflict payload: %v", err)
	}
	if payload["status"] != "skipped" || payload["error"] != operatorflow.ErrCycleAlreadyRunning.Error() {
		t.Fatalf("expected skipped loop conflict payload, got %+v", payload)
	}
}

func TestHandleOperatorLoopStrictFailureReturnsUnprocessableEntity(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/run-loop", strings.NewReader(`{"iterations":1,"strict":true}`))

	runner := stubOperatorRunner{
		result: operatorflow.Result{Status: "failed", FinalStatus: "failed"},
		err:    operatorflow.ExecutionError{Err: errors.New("source failed")},
	}
	newServerWithOperator(nil, nil, nil, nil, runner, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d body=%s", rec.Code, rec.Body.String())
	}
}

type stubKV struct {
	values map[string]string
}

func (s *stubKV) Get(_ context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(context.Background(), "get", key)
	if value, ok := s.values[key]; ok {
		cmd.SetVal(value)
	} else {
		cmd.SetErr(redis.Nil)
	}
	return cmd
}

func (s *stubKV) Set(_ context.Context, key string, value interface{}, _ time.Duration) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(context.Background(), "set", key)
	if s.values == nil {
		s.values = map[string]string{}
	}
	text, _ := value.(string)
	s.values[key] = text
	cmd.SetVal("OK")
	return cmd
}

func (s *stubKV) Del(_ context.Context, keys ...string) *redis.IntCmd {
	cmd := redis.NewIntCmd(context.Background(), "del")
	var deleted int64
	for _, key := range keys {
		if _, ok := s.values[key]; ok {
			delete(s.values, key)
			deleted++
		}
	}
	cmd.SetVal(deleted)
	return cmd
}

func (s *stubKV) Close() error {
	return nil
}

func TestHandleDashboardReturnsTransportResponse(t *testing.T) {
	repo := stubDashboardRepo{
		payload: insight.Dashboard{
			GeneratedAt: time.Date(2026, 6, 30, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			Workspace: insight.WorkspaceSummary{
				Name:        "默认运营工作区",
				Owner:       "hub:user:demo",
				DeviceCount: 6,
			},
			WorkspaceBridge: insight.WorkspaceBridge{
				WorkspaceID:   "ws-default-hub-user-demo",
				HubUserID:     "hub:user:demo",
				ContextSource: "default",
				BootstrapMode: "lazy_create",
				Source:        "insight_default_workspace",
				State:         "ready",
				Resolution:    "created",
				LastSyncedAt:  time.Date(2026, 6, 30, 10, 35, 0, 0, time.FixedZone("CST", 8*3600)),
			},
			Overview: insight.OverviewMetrics{
				MonitoredAccounts: 18,
				TrackedVideos:     326,
				EmergingTopics:    12,
				NewCommentBursts:  9,
				DailyCoverageRate: 93.4,
				ReportFreshness:   "8 分钟前",
			},
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)

	newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected JSON content type, got %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("expected no-store cache control, got %q", got)
	}

	var payload insightapi.DashboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload.GeneratedAt != "2026-06-30T10:30:00+08:00" {
		t.Fatalf("expected mapped generatedAt, got %q", payload.GeneratedAt)
	}
	if payload.Workspace.Owner != "hub:user:demo" {
		t.Fatalf("expected mapped workspace owner, got %q", payload.Workspace.Owner)
	}
	if payload.WorkspaceBridge.WorkspaceID != "ws-default-hub-user-demo" {
		t.Fatalf("expected workspace bridge workspace id, got %q", payload.WorkspaceBridge.WorkspaceID)
	}
}

func TestHandleHotRankingsReturnEmptyWithoutPersistedEvents(t *testing.T) {
	rankingRepo, err := insightfile.NewRankingRepository(filepath.Join(t.TempDir(), "rankings.json"))
	if err != nil {
		t.Fatalf("new ranking repository failed: %v", err)
	}
	contextResolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	dashboardService := insight.NewDashboardService(insightdemo.NewDashboardRepository(), insight.DashboardServiceConfig{
		WorkspaceContextResolver: contextResolver,
		RankingRepository:        rankingRepo,
	})
	readModelService := insight.NewReadModelService(
		dashboardService,
		insight.WithRankingRepository(rankingRepo),
		insight.WithWorkspaceContextResolver(contextResolver),
	)
	srv := newServer(dashboardService, readModelService, nil, contextResolver, runtime.Status{})

	for _, path := range []string{
		"/api/v1/read-models/hot-videos",
		"/api/v1/read-models/hot-topics",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected %s status 200, got %d body=%s", path, rec.Code, rec.Body.String())
		}

		var payload struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("expected %s valid JSON, got %v", path, err)
		}
		if len(payload.Items) != 0 {
			t.Fatalf("expected %s to return no seeded items, got %s", path, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected dashboard status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var dashboardPayload insightapi.DashboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &dashboardPayload); err != nil {
		t.Fatalf("expected dashboard valid JSON, got %v", err)
	}
	if len(dashboardPayload.HotVideos) != 0 || len(dashboardPayload.HotTopics) != 0 {
		t.Fatalf("expected dashboard rankings to be empty without events, got videos=%+v topics=%+v", dashboardPayload.HotVideos, dashboardPayload.HotTopics)
	}
}

func TestHandleDashboardPrefersHeaderContextOverQueryFallback(t *testing.T) {
	contextResolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 6,
		},
	)
	repo := stubDashboardRepo{
		payload: insight.Dashboard{
			GeneratedAt: time.Date(2026, 6, 30, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			Workspace: insight.WorkspaceSummary{
				Name:        "默认运营工作区",
				DeviceCount: 6,
			},
			Overview: insight.OverviewMetrics{
				MonitoredAccounts: 18,
				TrackedVideos:     326,
				EmergingTopics:    12,
				NewCommentBursts:  9,
				DailyCoverageRate: 93.4,
				ReportFreshness:   "8 分钟前",
			},
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: contextResolver,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?hub_user_id=hub:user:bob&device_count=4", nil)
	req.Header.Set(headerHubUserID, "hub:user:alice")
	req.Header.Set(headerHubDeviceCount, "9")

	newServer(svc, insight.NewReadModelService(svc), nil, contextResolver, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload insightapi.DashboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload.Workspace.Owner != "hub:user:alice" {
		t.Fatalf("expected header owner, got %q", payload.Workspace.Owner)
	}
	if payload.Workspace.DeviceCount != 9 {
		t.Fatalf("expected header device count 9, got %d", payload.Workspace.DeviceCount)
	}
	if payload.WorkspaceBridge.ContextSource != "header" {
		t.Fatalf("expected header context source, got %q", payload.WorkspaceBridge.ContextSource)
	}
}

func TestHandleDashboardAcceptsTrustedBridgeHeader(t *testing.T) {
	contextResolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 6,
		},
	)
	repo := stubDashboardRepo{
		payload: insight.Dashboard{
			GeneratedAt: time.Date(2026, 6, 30, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			Workspace: insight.WorkspaceSummary{
				Name:        "默认运营工作区",
				DeviceCount: 6,
			},
			Overview: insight.OverviewMetrics{
				ReportFreshness: "8 分钟前",
			},
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: contextResolver,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?hub_user_id=hub:user:bob&device_count=4", nil)
	req.Header.Set(headerHubUserID, "hub:user:alice")
	req.Header.Set(headerHubDeviceCount, "9")
	req.Header.Set(headerBridgeToken, "bridge-secret")

	srv := newServer(svc, insight.NewReadModelService(svc), nil, contextResolver, runtime.Status{})
	srv.bridgeConfig = runtime.BridgeConfig{TrustedToken: "bridge-secret", DemoContextEnabled: false}
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload insightapi.DashboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload.Workspace.Owner != "hub:user:alice" {
		t.Fatalf("expected trusted bridge owner, got %q", payload.Workspace.Owner)
	}
	if payload.WorkspaceBridge.ContextSource != "trusted_header" {
		t.Fatalf("expected trusted header context source, got %q", payload.WorkspaceBridge.ContextSource)
	}
}

func TestHandleDashboardRejectsPlainHeaderWhenTrustedBridgeIsRequired(t *testing.T) {
	repo := stubDashboardRepo{
		payload: insight.Dashboard{
			GeneratedAt: time.Date(2026, 6, 30, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.Header.Set(headerHubUserID, "hub:user:alice")

	srv := newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{})
	srv.bridgeConfig = runtime.BridgeConfig{TrustedToken: "bridge-secret", DemoContextEnabled: false}
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["error"] == nil {
		t.Fatalf("expected trusted bridge error payload, got %+v", payload)
	}
}

func TestHandleDashboardRejectsDemoQueryWhenTrustedBridgeDisablesIt(t *testing.T) {
	repo := stubDashboardRepo{
		payload: insight.Dashboard{
			GeneratedAt: time.Date(2026, 6, 30, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?hub_user_id=hub:user:bob&device_count=4", nil)

	srv := newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{})
	srv.bridgeConfig = runtime.BridgeConfig{TrustedToken: "bridge-secret", DemoContextEnabled: false}
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleDashboardRejectsTeamPreviewLiveRequestWhenTrustedBridgeIsNotConfigured(t *testing.T) {
	repo := stubDashboardRepo{
		payload: insight.Dashboard{
			GeneratedAt: time.Date(2026, 6, 30, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?hub_user_id=hub:user:bob&device_count=4", nil)

	srv := newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{})
	srv.bridgeConfig = runtime.BridgeConfig{RequireTrustedForLive: true, DemoContextEnabled: false}
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["error"] != errTrustedBridgeNotConfigured.Error() {
		t.Fatalf("expected trusted bridge not configured error, got %+v", payload)
	}
}

func TestHandleDashboardReturnsServerErrorOnServiceFailure(t *testing.T) {
	repo := stubDashboardRepo{
		err: errors.New("repo unavailable"),
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)

	newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestHandleWorkspaceContextReturnsResolvedBridgeState(t *testing.T) {
	contextResolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 6,
		},
	)
	repo := stubDashboardRepo{}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: contextResolver,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspace/context?hub_user_id=hub:user:bob&device_count=4", nil)
	req.Header.Set(headerHubUserID, "hub:user:carol")
	req.Header.Set(headerHubDeviceCount, "11")

	newServer(svc, insight.NewReadModelService(svc), nil, contextResolver, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["hubUserId"] != "hub:user:carol" {
		t.Fatalf("expected header hub user id, got %v", payload["hubUserId"])
	}
	if payload["contextSource"] != "header" {
		t.Fatalf("expected header context source, got %v", payload["contextSource"])
	}
}

func TestHandleReplaceDashboardPersistsNewSnapshot(t *testing.T) {
	repo := stubDashboardRepo{}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"generatedAt":"2026-06-30T18:00:00+08:00","workspace":{"name":"默认运营工作区","deviceCount":6},"overview":{"monitoredAccounts":27,"trackedVideos":326,"emergingTopics":12,"newCommentBursts":9,"dailyCoverageRate":93.4,"reportFreshness":"刚刚"},"hotVideos":[{"id":"vid-1","title":"A","channelName":"C","tags":["t"],"publishedAgo":"1 小时前","views":1,"likes":1,"comments":1,"growthRate":1.1,"engagementScore":1,"signal":"up"}],"hotTopics":[{"id":"topic-1","keyword":"K","cluster":"C","mentions":1,"growthRate":1.2,"relatedVideos":1,"promptAngle":"P","keywords":["k"]}],"watchlistAlerts":[{"id":"alert-1","accountName":"A","alertType":"new","severity":"high","summary":"S","happenedAt":"今天","recommended":"R","videoCount":1,"commentDelta":1}],"commentInsights":[{"id":"ci-1","theme":"T","questionRate":1,"sentiment":"high","representative":"R","actionSuggestion":"A","relatedKeywords":["k"]}],"reportSummary":{"period":"2026-06-30","headline":"H","keyFinding":"K","recommendedAction":"R","watchout":"W","autoGeneratedAt":"今天"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/replace", body)

	newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestHandleReplaceDashboardRejectsSparseSnapshot(t *testing.T) {
	repo := stubDashboardRepo{}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"generatedAt":"2026-06-30T18:00:00+08:00","overview":{"monitoredAccounts":27}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/replace", body)

	newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestHandleResetDashboardReturnsOK(t *testing.T) {
	repo := stubDashboardRepo{}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/reset", nil)

	newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestHandleHotVideosReturnsReadModelSlice(t *testing.T) {
	repo := stubDashboardRepo{
		payload: insight.Dashboard{
			GeneratedAt: time.Date(2026, 6, 30, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			Workspace:   insight.WorkspaceSummary{Name: "默认运营工作区", DeviceCount: 6},
			Overview:    insight.OverviewMetrics{ReportFreshness: "刚刚"},
			ReadModel: insight.ReadModelState{ViewStatuses: []insight.ReadModelViewStatus{{
				Key:            "videos",
				Label:          "Hot Videos",
				FreshnessState: "fresh",
				Summary:        "当前热视频榜已经跟到最新物化批次，可直接支撑今天的样本判断。",
				SourceNote:     "wx_feed_list fresh",
				LatestBatchID:  "batch-1",
				MaterializedAt: time.Date(2026, 6, 30, 10, 31, 0, 0, time.FixedZone("CST", 8*3600)),
			}}},
			HotVideos: []insight.HotVideo{{ID: "vid-1", Title: "A", ChannelName: "C", ObjectID: "oid-hot-1", ObjectNonceID: "nid-hot-1"}},
			HotVideoRankings: insight.HotVideoRankings{
				insight.HotVideoRankingLikes: {{ID: "vid-likes", Title: "点赞榜 A"}},
			},
			HotTopics:       []insight.HotTopic{{ID: "topic-1", Keyword: "K"}},
			WatchlistAlerts: []insight.WatchlistAlert{{ID: "alert-1"}},
			CommentInsights: []insight.CommentInsight{{ID: "ci-1"}},
			ReportSummary: insight.ReportSummary{
				Period: "2026-06-30", Headline: "H", KeyFinding: "K", RecommendedAction: "R", Watchout: "W", AutoGeneratedAt: "今天",
			},
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/hot-videos", nil)

	newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload insightapi.HotVideosResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].Title != "A" {
		t.Fatalf("expected hot videos payload, got %+v", payload)
	}
	if payload.Ranking != insight.HotVideoRankingHot {
		t.Fatalf("expected default hot ranking, got %+v", payload)
	}
	if payload.Items[0].ObjectID != "oid-hot-1" || payload.Items[0].ObjectNonceID != "nid-hot-1" {
		t.Fatalf("expected hot video identifiers in payload, got %+v", payload.Items[0])
	}
	if payload.ViewStatus == nil || payload.ViewStatus.Key != "videos" || payload.ViewStatus.LatestBatchID != "batch-1" {
		t.Fatalf("expected view status metadata, got %+v", payload.ViewStatus)
	}

	likesRec := httptest.NewRecorder()
	likesReq := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/hot-videos?ranking=likes", nil)
	newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{}).routes().ServeHTTP(likesRec, likesReq)

	if likesRec.Code != http.StatusOK {
		t.Fatalf("expected likes ranking status 200, got %d", likesRec.Code)
	}
	var likesPayload insightapi.HotVideosResponse
	if err := json.Unmarshal(likesRec.Body.Bytes(), &likesPayload); err != nil {
		t.Fatalf("expected valid likes ranking JSON, got %v", err)
	}
	if likesPayload.Ranking != insight.HotVideoRankingLikes || len(likesPayload.Items) != 1 || likesPayload.Items[0].Title != "点赞榜 A" {
		t.Fatalf("expected likes ranking payload, got %+v", likesPayload)
	}
}

func TestHandleDashboardAppliesRequestedTimeWindow(t *testing.T) {
	referenceAt := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	repo := stubDashboardRepo{payload: insight.Dashboard{
		GeneratedAt: referenceAt,
		HotVideos: []insight.HotVideo{
			{ID: "recent-video", Title: "最近视频", PublishedAt: referenceAt.Add(-6 * time.Hour)},
			{ID: "old-video", Title: "历史视频", PublishedAt: referenceAt.Add(-96 * time.Hour)},
		},
		HotTopics: []insight.HotTopic{
			{ID: "recent-topic", Keyword: "最近话题", ObservedAt: referenceAt.Add(-6 * time.Hour)},
			{ID: "old-topic", Keyword: "历史话题", ObservedAt: referenceAt.Add(-96 * time.Hour)},
		},
		CommentInsights: []insight.CommentInsight{
			{ID: "recent-comment", Theme: "最近需求", ObservedAt: referenceAt.Add(-6 * time.Hour)},
			{ID: "old-comment", Theme: "历史需求", ObservedAt: referenceAt.Add(-96 * time.Hour)},
		},
	}}
	service := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	server := newServer(service, insight.NewReadModelService(service), nil, nil, runtime.Status{}).routes()

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?window=24h", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 24h dashboard status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload insightapi.DashboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode 24h dashboard: %v", err)
	}
	if len(payload.HotVideos) != 1 || payload.HotVideos[0].ID != "recent-video" {
		t.Fatalf("expected only recent video, got %+v", payload.HotVideos)
	}
	if len(payload.HotTopics) != 1 || payload.HotTopics[0].ID != "recent-topic" {
		t.Fatalf("expected only recent topic, got %+v", payload.HotTopics)
	}
	if len(payload.CommentInsights) != 1 || payload.CommentInsights[0].ID != "recent-comment" {
		t.Fatalf("expected only recent comment, got %+v", payload.CommentInsights)
	}
	if payload.Overview.TrackedVideos != 1 || payload.Overview.EmergingTopics != 1 || payload.Overview.NewCommentBursts != 1 {
		t.Fatalf("expected bounded overview counts, got %+v", payload.Overview)
	}

	invalid := httptest.NewRecorder()
	server.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?window=7d", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("expected unsupported window to return 400, got %d", invalid.Code)
	}
}

func TestReadModelRankingEndpointsRespectLimit(t *testing.T) {
	repo := stubDashboardRepo{
		payload: insight.Dashboard{
			HotVideos: []insight.HotVideo{
				{ID: "video-1", Title: "视频一"},
				{ID: "video-2", Title: "视频二"},
			},
			HotVideoRankings: insight.HotVideoRankings{
				insight.HotVideoRankingComments: {
					{ID: "video-comments-1", Title: "评论榜一"},
					{ID: "video-comments-2", Title: "评论榜二"},
				},
			},
			HotTopics: []insight.HotTopic{
				{ID: "topic-1", Keyword: "话题一"},
				{ID: "topic-2", Keyword: "话题二"},
			},
			CommentInsights: []insight.CommentInsight{
				{ID: "comment-1", Theme: "需求一"},
				{ID: "comment-2", Theme: "需求二"},
			},
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	server := newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{}).routes()

	videoRec := httptest.NewRecorder()
	server.ServeHTTP(videoRec, httptest.NewRequest(http.MethodGet, "/api/v1/read-models/hot-videos?ranking=comments&limit=1", nil))
	if videoRec.Code != http.StatusOK {
		t.Fatalf("expected limited video ranking 200, got %d: %s", videoRec.Code, videoRec.Body.String())
	}
	var videos insightapi.HotVideosResponse
	if err := json.Unmarshal(videoRec.Body.Bytes(), &videos); err != nil {
		t.Fatalf("decode limited video ranking: %v", err)
	}
	if len(videos.Items) != 1 || videos.Items[0].Title != "评论榜一" {
		t.Fatalf("expected one limited video ranking item, got %+v", videos.Items)
	}

	topicRec := httptest.NewRecorder()
	server.ServeHTTP(topicRec, httptest.NewRequest(http.MethodGet, "/api/v1/read-models/hot-topics?limit=1", nil))
	if topicRec.Code != http.StatusOK {
		t.Fatalf("expected limited topic ranking 200, got %d: %s", topicRec.Code, topicRec.Body.String())
	}
	var topics insightapi.HotTopicsResponse
	if err := json.Unmarshal(topicRec.Body.Bytes(), &topics); err != nil {
		t.Fatalf("decode limited topic ranking: %v", err)
	}
	if len(topics.Items) != 1 || topics.Items[0].Keyword != "话题一" {
		t.Fatalf("expected one limited topic ranking item, got %+v", topics.Items)
	}

	commentRec := httptest.NewRecorder()
	server.ServeHTTP(commentRec, httptest.NewRequest(http.MethodGet, "/api/v1/read-models/comment-insights?limit=1", nil))
	if commentRec.Code != http.StatusOK {
		t.Fatalf("expected limited comment ranking 200, got %d: %s", commentRec.Code, commentRec.Body.String())
	}
	var comments insightapi.CommentInsightsResponse
	if err := json.Unmarshal(commentRec.Body.Bytes(), &comments); err != nil {
		t.Fatalf("decode limited comment ranking: %v", err)
	}
	if len(comments.Items) != 1 || comments.Items[0].Theme != "需求一" {
		t.Fatalf("expected one limited comment ranking item, got %+v", comments.Items)
	}
}

func TestHandleReportSummaryReturnsReadModelSlice(t *testing.T) {
	repo := stubDashboardRepo{
		payload: insight.Dashboard{
			GeneratedAt: time.Date(2026, 6, 30, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			Workspace:   insight.WorkspaceSummary{Name: "默认运营工作区", DeviceCount: 6},
			Overview:    insight.OverviewMetrics{ReportFreshness: "刚刚"},
			ReadModel: insight.ReadModelState{ViewStatuses: []insight.ReadModelViewStatus{{
				Key:            "reports",
				Label:          "Reports",
				FreshnessState: "lagging",
				Summary:        "日报摘要还没完全对齐最新物化结果。",
				SourceNote:     "wx_comment_export lagging",
				LatestBatchID:  "batch-2",
				MaterializedAt: time.Date(2026, 6, 30, 10, 29, 0, 0, time.FixedZone("CST", 8*3600)),
			}}},
			HotVideos:       []insight.HotVideo{{ID: "vid-1"}},
			HotTopics:       []insight.HotTopic{{ID: "topic-1"}},
			WatchlistAlerts: []insight.WatchlistAlert{{ID: "alert-1"}},
			CommentInsights: []insight.CommentInsight{{ID: "ci-1"}},
			ReportSummary: insight.ReportSummary{
				Period: "2026-06-30", Headline: "团队日报", KeyFinding: "今日重点", RecommendedAction: "跟进", Watchout: "注意节奏", AutoGeneratedAt: "今天",
			},
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/report-summary", nil)

	newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload insightapi.ReportSummaryOnlyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload.Item.Headline != "团队日报" {
		t.Fatalf("expected report summary payload, got %+v", payload)
	}
	if payload.ViewStatus == nil || payload.ViewStatus.Key != "reports" || payload.ViewStatus.FreshnessState != "lagging" {
		t.Fatalf("expected report view status metadata, got %+v", payload.ViewStatus)
	}
}

func TestHandleRuntimeStatusReturnsDynamicReadCacheTelemetry(t *testing.T) {
	client, err := rediscache.NewClientWithKV("127.0.0.1:6379", "", 0, &stubKV{})
	if err != nil {
		t.Fatalf("build redis stub client failed: %v", err)
	}
	cache := insight.NewReadCache(client, time.Minute)
	if _, found, err := cache.LoadDashboard(context.Background(), "ws-runtime"); err != nil || found {
		t.Fatalf("expected initial cache miss, found=%v err=%v", found, err)
	}
	if err := cache.SaveDashboard(context.Background(), "ws-runtime", insight.Dashboard{}); err != nil {
		t.Fatalf("save dashboard failed: %v", err)
	}
	if _, found, err := cache.LoadDashboard(context.Background(), "ws-runtime"); err != nil || !found {
		t.Fatalf("expected cache hit after save, found=%v err=%v", found, err)
	}
	cache.RecordScopeFallback("dashboard", time.Date(2026, 7, 3, 15, 0, 0, 0, time.UTC))
	if err := cache.InvalidateWorkspace(context.Background(), "ws-runtime"); err != nil {
		t.Fatalf("invalidate workspace failed: %v", err)
	}

	srv := newServer(nil, nil, nil, nil, runtime.Status{
		Phase:               "team-edition-phase-a",
		RuntimeProfile:      runtime.RuntimeProfileTeamPreview,
		RedisConfigured:     true,
		ReadCacheActive:     true,
		ReadCacheScope:      []string{"dashboard"},
		JobsEntrypointState: "available",
		JobsRuntimeState:    "ingest-and-read-model-ops-available",
	})
	srv.readCache = cache

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["readCacheHitCount"] != float64(1) {
		t.Fatalf("expected readCacheHitCount 1, got %v", payload["readCacheHitCount"])
	}
	if payload["readCacheMissCount"] != float64(1) {
		t.Fatalf("expected readCacheMissCount 1, got %v", payload["readCacheMissCount"])
	}
	if payload["readCacheFallbackCount"] != float64(1) {
		t.Fatalf("expected readCacheFallbackCount 1, got %v", payload["readCacheFallbackCount"])
	}
	if payload["readCacheWriteCount"] != float64(1) {
		t.Fatalf("expected readCacheWriteCount 1, got %v", payload["readCacheWriteCount"])
	}
	if payload["readCacheInvalidateCount"] != float64(1) {
		t.Fatalf("expected readCacheInvalidateCount 1, got %v", payload["readCacheInvalidateCount"])
	}
	if payload["readCacheLastFallbackAt"] == "" {
		t.Fatalf("expected fallback timestamp, got %v", payload["readCacheLastFallbackAt"])
	}
	scopeStats, ok := payload["readCacheScopeStats"].(map[string]any)
	if !ok {
		t.Fatalf("expected readCacheScopeStats payload, got %+v", payload["readCacheScopeStats"])
	}
	dashboardScope, ok := scopeStats["dashboard"].(map[string]any)
	if !ok {
		t.Fatalf("expected dashboard scope payload, got %+v", scopeStats)
	}
	if dashboardScope["hitCount"] != float64(1) || dashboardScope["missCount"] != float64(1) {
		t.Fatalf("expected dashboard scope hit/miss counts, got %+v", dashboardScope)
	}
	if dashboardScope["fallbackCount"] != float64(1) || dashboardScope["writeCount"] != float64(1) {
		t.Fatalf("expected dashboard scope fallback/write counts, got %+v", dashboardScope)
	}
	if payload["liveRuntimeClass"] != "transition" {
		t.Fatalf("expected transition live runtime class, got %+v", payload)
	}
	if payload["runtimeProfile"] != runtime.RuntimeProfileTeamPreview {
		t.Fatalf("expected runtime profile team-preview, got %+v", payload)
	}
	requirements, ok := payload["liveRuntimeRequirements"].([]any)
	if !ok || len(requirements) != 4 {
		t.Fatalf("expected four live runtime requirements, got %+v", payload["liveRuntimeRequirements"])
	}
}

func TestHandleRuntimeStatusSurfacesMissingWXChannelCapabilities(t *testing.T) {
	wxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"message": "success",
				"data": map[string]any{
					"connected":             true,
					"clients":               2,
					"ready_clients":         2,
					"feed_ready_clients":    1,
					"comment_ready_clients": 1,
				},
			})
		case "/api/channels/contact/feed/list":
			http.Error(w, "username is required", http.StatusBadRequest)
		case "/api/channels/feed/search", "/api/channels/feed/comment/export":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer wxServer.Close()

	srv := newServer(nil, nil, nil, nil, runtime.Status{
		HubBridgeMode:           "local-operator",
		DemoContextEnabled:      false,
		DashboardStoreMode:      "postgres",
		RankingStoreMode:        "postgres",
		CommentInsightStoreMode: "postgres",
		ReportSummaryStoreMode:  "postgres",
		WorkspaceStoreMode:      "postgres",
		IngestStoreMode:         "postgres",
		PostgresConfigured:      true,
		PostgresReady:           true,
		RedisConfigured:         true,
		RedisReady:              true,
		ReadCacheConfigured:     true,
		ReadCacheActive:         true,
		ReadCacheScope:          []string{"dashboard", "hot_videos", "hot_topics", "report_summary"},
		ReadCacheScopeStats: map[string]insight.ReadCacheScopeSnapshot{
			"dashboard":      {HitCount: 1},
			"hot_videos":     {WriteCount: 1},
			"hot_topics":     {MissCount: 1},
			"report_summary": {HitCount: 1},
		},
	})
	srv.wxChannelBaseURL = wxServer.URL

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["liveRuntimeClass"] == "formal-ready" {
		t.Fatalf("expected incompatible wx_channel data plane to block formal-ready, got %+v", payload)
	}
	if payload["wxChannelCapabilitiesChecked"] != true || payload["wxChannelCapabilitiesReady"] != false {
		t.Fatalf("expected wx_channel capability flags, got %+v", payload)
	}
	missing, ok := payload["wxChannelMissingCapabilities"].([]any)
	if !ok || len(missing) != 2 || missing[0] != "feedSearch" || missing[1] != "commentExport" {
		t.Fatalf("expected missing feedSearch/commentExport, got %+v", payload["wxChannelMissingCapabilities"])
	}
	routes, ok := payload["wxChannelCapabilityRoutes"].([]any)
	if !ok || len(routes) != 3 {
		t.Fatalf("expected capability route probes, got %+v", payload["wxChannelCapabilityRoutes"])
	}
	requirements, ok := payload["liveRuntimeRequirements"].([]any)
	if !ok {
		t.Fatalf("expected live runtime requirements, got %+v", payload["liveRuntimeRequirements"])
	}
	var dataPlane map[string]any
	for _, item := range requirements {
		candidate, _ := item.(map[string]any)
		if candidate["key"] == "wx-channel-data-plane" {
			dataPlane = candidate
			break
		}
	}
	if dataPlane == nil || dataPlane["status"] != "blocked" {
		t.Fatalf("expected blocked wx-channel-data-plane requirement, got %+v", requirements)
	}
}

func TestHandleRuntimeStatusBuildsLaunchReadinessFromCurrentDashboard(t *testing.T) {
	materializedAt := time.Date(2026, 7, 6, 10, 57, 1, 0, time.UTC)
	repo := stubDashboardRepo{
		payload: insight.Dashboard{
			ReadModel: insight.ReadModelState{
				Mode:           "materialized",
				LatestBatchID:  "batch-launch-1",
				MaterializedAt: materializedAt,
				ViewStatuses: []insight.ReadModelViewStatus{
					{Key: "videos", LatestBatchID: "batch-launch-1", MaterializedAt: materializedAt},
					{Key: "topics", LatestBatchID: "batch-launch-1", MaterializedAt: materializedAt},
					{Key: "comments", LatestBatchID: "batch-launch-1", MaterializedAt: materializedAt},
					{Key: "reports", LatestBatchID: "batch-launch-1", MaterializedAt: materializedAt},
				},
			},
			WorkspaceBridge: insight.WorkspaceBridge{
				State:         "ready",
				WorkspaceID:   "ws-default-hub-user-team-runtime-20260706185558",
				HubUserID:     "hub:user:team-runtime-20260706185558",
				ContextSource: "trusted_header",
				BootstrapMode: "lazy_create",
				Source:        "insight_default_workspace",
				Resolution:    "reused",
				LastSyncedAt:  time.Date(2026, 7, 6, 10, 56, 37, 0, time.UTC),
			},
			IngestSummary: insight.IngestSummary{
				State:          "ready",
				FreshnessState: "fresh",
				LatestBatchID:  "batch-launch-1",
				LastSuccessAt:  materializedAt,
				BindingStates: []insight.IngestBindingState{
					{
						Source:            "wx_comment_export",
						Runnable:          true,
						LastRunStatus:     "success",
						LastAutoRunStatus: "success",
						LastAutoRunAt:     materializedAt,
						LastAutoSuccessAt: materializedAt,
						LastAutoBatchID:   "batch-launch-1",
					},
					{
						Source:            "wx_feed_list",
						Runnable:          true,
						LastRunStatus:     "success",
						LastAutoRunStatus: "success",
						LastAutoRunAt:     materializedAt,
						LastAutoSuccessAt: materializedAt,
						LastAutoBatchID:   "batch-launch-1",
					},
				},
			},
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})
	srv := newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{
		Phase:                    "team-edition-phase-a",
		RuntimeProfile:           runtime.RuntimeProfileTeamPreview,
		HubBridgeMode:            "trusted-header",
		DemoContextEnabled:       false,
		OwnerBoundaryState:       "met",
		OwnerBoundaryEvidence:    "analytics=insight-postgres · stores=Dashboard=postgres · Rankings=postgres · Comment Insights=postgres · Report Summary=postgres · Workspace=postgres · Ingest=postgres",
		OwnerBoundarySummary:     "当前 analytics 热路径仍留在 Insight 自己的 store owner 内。",
		OwnerBoundaryAction:      "继续保持当前 owner 边界。",
		DashboardStoreMode:       "postgres",
		RankingStoreMode:         "postgres",
		CommentInsightStoreMode:  "postgres",
		ReportSummaryStoreMode:   "postgres",
		WorkspaceStoreMode:       "postgres",
		IngestStoreMode:          "postgres",
		PostgresConfigured:       true,
		PostgresReady:            true,
		RedisConfigured:          true,
		RedisReady:               true,
		ReadCacheConfigured:      true,
		ReadCacheActive:          true,
		ReadCacheScope:           []string{"dashboard", "hot_videos", "hot_topics", "comment_insights", "report_summary"},
		ReadCacheScopeStats:      map[string]insight.ReadCacheScopeSnapshot{"dashboard": {HitCount: 4}, "hot_videos": {HitCount: 1}, "hot_topics": {HitCount: 1}, "report_summary": {HitCount: 1}},
		JobsEntrypointState:      "available",
		JobsRuntimeState:         "recent-ok",
		JobsLastMode:             "operator-loop",
		JobsLastStatus:           "ok",
		JobsLastFinishedAt:       materializedAt,
		JobsLastRequestedCount:   2,
		JobsLastRequestedSources: []string{"wx_comment_export", "wx_video_search"},
		TrustedRuntimeEvidence:   runtime.TrustedRuntimeEvidence{State: "verified", Source: "verify-insight-docker-stack", VerifiedAt: time.Date(2026, 7, 6, 10, 58, 44, 0, time.UTC), HubUserID: "hub:user:team-runtime-20260706185558", TrustedBoundaryMode: "trusted-only", APITrustedContextState: "verified", APIBrowserQueryState: "blocked", WebTrustedContextState: "verified", WebBrowserQueryState: "isolated"},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	readiness, ok := payload["launchReadiness"].(map[string]any)
	if !ok {
		t.Fatalf("expected launchReadiness payload, got %+v", payload)
	}
	if readiness["metCount"] != float64(8) || readiness["blockedCount"] != float64(0) || readiness["unknownCount"] != float64(0) {
		t.Fatalf("expected all launch gates met, got %+v", readiness)
	}
	gates, ok := readiness["gates"].([]any)
	if !ok || len(gates) != 8 {
		t.Fatalf("expected eight launch gates, got %+v", readiness["gates"])
	}

	monitoringRepo := monitoring.NewMemoryRepository()
	if err := monitoringRepo.CreateTask(context.Background(), monitoring.Task{
		ID: "runtime-abandoned-task", HubUserID: "runtime-health", Name: "abandoned runtime task",
		Status: monitoring.StatusEnabled, ScopeMode: monitoring.ScopeAllSavedBindings,
		Interval: 30 * time.Minute, NextRunAt: materializedAt.Add(time.Hour),
		LastRunAt: materializedAt.Add(time.Minute), LastFailureAt: materializedAt.Add(time.Minute),
		LastStatus: monitoring.RunStatusAbandoned, LastError: "monitoring task lease expired",
		CreatedAt: materializedAt.Add(-time.Hour), UpdatedAt: materializedAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create monitoring task: %v", err)
	}
	monitoringService, err := monitoring.NewService(monitoringRepo, monitoring.ServiceConfig{DefaultHubUserID: "runtime-health"})
	if err != nil {
		t.Fatalf("new monitoring service: %v", err)
	}
	srv.monitoringService = monitoringService

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil)
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected monitoring-aware status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	payload = map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode monitoring-aware runtime status: %v", err)
	}
	readiness, ok = payload["launchReadiness"].(map[string]any)
	if !ok || readiness["metCount"] == float64(8) || readiness["unknownCount"] != float64(1) {
		t.Fatalf("expected abandoned monitoring run to invalidate 8/8 readiness, got %+v", readiness)
	}
}

func TestHandleRuntimeStatusSkipsDashboardAutoMaterialization(t *testing.T) {
	now := time.Date(2026, 7, 8, 1, 10, 0, 0, time.UTC)
	baseRepo := insightdemo.NewDashboardRepository()
	seed, err := baseRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("load seed failed: %v", err)
	}
	seed.ReadModel = insight.ReadModelState{
		Mode:           "materialized",
		Source:         "jobs",
		LatestBatchID:  "batch-status-stale",
		MaterializedAt: now.Add(-30 * time.Minute),
	}
	repo := &recordingDashboardRepo{payload: seed}
	resolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	summary := ingest.Summary{
		State:          "ready",
		FreshnessState: "fresh",
		TotalEvents:    1,
		LatestBatchID:  "batch-status-fresh",
		LatestEventAt:  now,
		LastSuccessAt:  now,
		SourceStatuses: []ingest.SourceStatus{
			{Source: "wx_comment_export", FreshnessState: "fresh", LatestBatchID: "batch-status-fresh"},
			{Source: "wx_video_search", FreshnessState: "fresh", LatestBatchID: "batch-status-fresh"},
		},
		BindingStates: []ingest.SourceBindingState{
			{
				Source:            "wx_comment_export",
				Runnable:          true,
				LastRunStatus:     "success",
				LastAutoRunStatus: "success",
				LastAutoRunAt:     now,
				LastAutoSuccessAt: now,
				LastAutoBatchID:   "batch-status-fresh",
			},
			{
				Source:            "wx_video_search",
				Runnable:          true,
				LastRunStatus:     "success",
				LastAutoRunStatus: "success",
				LastAutoRunAt:     now,
				LastAutoSuccessAt: now,
				LastAutoBatchID:   "batch-status-fresh",
			},
		},
	}
	svc := insight.NewDashboardService(repo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: resolver,
		IngestService:            stubIngestService{summary: summary},
		Materializer:             insight.NewMaterializer(seed),
	})
	srv := newServer(svc, insight.NewReadModelService(svc), nil, nil, runtime.Status{
		Phase:                    "team-edition-phase-a",
		RuntimeProfile:           runtime.RuntimeProfileTeamPreview,
		HubBridgeMode:            "trusted-header",
		DemoContextEnabled:       false,
		OwnerBoundaryState:       "met",
		DashboardStoreMode:       "postgres",
		RankingStoreMode:         "postgres",
		CommentInsightStoreMode:  "postgres",
		ReportSummaryStoreMode:   "postgres",
		WorkspaceStoreMode:       "postgres",
		IngestStoreMode:          "postgres",
		PostgresConfigured:       true,
		PostgresReady:            true,
		RedisConfigured:          true,
		RedisReady:               true,
		ReadCacheConfigured:      true,
		ReadCacheActive:          true,
		ReadCacheScope:           []string{"dashboard", "hot_videos", "hot_topics", "comment_insights", "report_summary"},
		JobsEntrypointState:      "available",
		JobsRuntimeState:         "recent-ok",
		JobsLastMode:             "operator-loop",
		JobsLastStatus:           "ok",
		JobsLastFinishedAt:       now,
		JobsLastRequestedCount:   2,
		JobsLastRequestedSources: []string{"wx_comment_export", "wx_video_search"},
		TrustedRuntimeEvidence:   runtime.TrustedRuntimeEvidence{State: "verified", Source: "verify-insight-docker-stack", VerifiedAt: now, HubUserID: "hub:user:alice", TrustedBoundaryMode: "trusted-only", APITrustedContextState: "verified", APIBrowserQueryState: "blocked", WebTrustedContextState: "verified", WebBrowserQueryState: "isolated"},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status?hub_user_id=hub:user:alice&device_count=2", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.replaceCalled {
		t.Fatalf("expected runtime status launch readiness path to avoid auto materialization")
	}
}

func TestHandleRuntimeStatusReadsJobsRuntimeStateFile(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil)

	path := filepath.Join(t.TempDir(), "jobs-runtime-state.json")
	if err := runtime.SaveJobsRuntimeState(path, runtime.JobsRuntimeState{
		LastMode:             "operator-loop",
		LastStatus:           "failed",
		LastHubUserID:        "hub:user:alice",
		LastFinishedAt:       time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC),
		LastIteration:        1,
		LastIterations:       3,
		LastInterval:         "5m0s",
		LastError:            "wx_feed_list timeout",
		LastRequestedCount:   2,
		LastRequestedSources: []string{"wx_comment_export", "wx_feed_list"},
		UpdatedAt:            time.Date(2026, 7, 3, 9, 0, 1, 0, time.UTC),
	}); err != nil {
		t.Fatalf("expected jobs runtime state save success, got %v", err)
	}

	srv := newServer(nil, nil, nil, nil, runtime.Status{
		Phase:               "team-edition-phase-a",
		JobsEntrypointState: "available",
		JobsRuntimeState:    "ingest-and-read-model-ops-available",
	})
	srv.jobsStatePath = path
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["jobsRuntimeState"] != "recent-failed" {
		t.Fatalf("expected jobs runtime state to reflect file, got %+v", payload)
	}
	if payload["jobsLastMode"] != "operator-loop" || payload["jobsLastError"] != "wx_feed_list timeout" {
		t.Fatalf("expected jobs runtime file fields, got %+v", payload)
	}
	if payload["jobsLastRequestedCount"] != float64(2) {
		t.Fatalf("expected requested count from jobs runtime file, got %+v", payload)
	}
	requestedSources, ok := payload["jobsLastRequestedSources"].([]any)
	if !ok || len(requestedSources) != 2 || requestedSources[0] != "wx_comment_export" || requestedSources[1] != "wx_feed_list" {
		t.Fatalf("expected requested sources from jobs runtime file, got %+v", payload)
	}
}

func TestHandleRuntimeStatusIgnoresRetiredTrustedRuntimeProof(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil)

	root := t.TempDir()
	proofDir := filepath.Join(root, ".tmp-codex")
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		t.Fatalf("mkdir proof dir failed: %v", err)
	}
	payload := `{
  "status": "success",
  "hubUserId": "hub:user:proof",
  "apiPort": 61288,
  "webPort": 61289,
  "results": [
    {
      "name": "check-team-runtime-status",
      "exitCode": 0,
      "output": "{\"hubBridgeMode\":\"trusted-header\",\"demoContextEnabled\":false}"
    },
    {
      "name": "check-team-runtime-trusted-context",
      "exitCode": 0,
      "output": "{\"hubUserId\":\"hub:user:proof\",\"contextSource\":\"trusted_header\"}"
    },
    {
      "name": "check-team-runtime-browser-query-blocked",
      "exitCode": 0,
      "output": "{\"statusCode\":401,\"json\":null}"
    },
    {
      "name": "check-trusted-web-context",
      "exitCode": 0,
      "output": "{\"hubUserId\":\"hub:user:proof\",\"contextSource\":\"trusted_header\"}"
    },
    {
      "name": "check-trusted-web-browser-query-isolated",
      "exitCode": 0,
      "output": "{\"hubUserId\":\"hub:user:proof\",\"contextSource\":\"trusted_header\"}"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(proofDir, "verify-team-runtime-proof-2.json"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write proof file failed: %v", err)
	}

	srv := newServer(nil, nil, nil, nil, runtime.Status{
		Phase:               "team-edition-phase-a",
		JobsEntrypointState: "available",
		JobsRuntimeState:    "ingest-and-read-model-ops-available",
	})
	srv.trustedRuntimeEvidenceRoot = root
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	proof, ok := result["trustedRuntimeEvidence"].(map[string]any)
	if !ok {
		t.Fatalf("expected trusted runtime evidence payload, got %+v", result)
	}
	if proof["state"] != "missing" || result["liveRuntimeProofState"] != "missing" {
		t.Fatalf("retired proof must not populate runtime status: %+v", result)
	}
	if _, exists := result["latestTeamWorkbench"]; exists {
		t.Fatalf("retired workbench status must not be exposed: %+v", result)
	}
	requirements, ok := result["liveRuntimeRequirements"].([]any)
	if !ok || len(requirements) != 4 {
		t.Fatalf("expected live runtime requirements in proof response, got %+v", result["liveRuntimeRequirements"])
	}
}

func TestHandleRuntimeStatusReadsStoredTrustedRuntimeEvidence(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil)

	root := t.TempDir()
	outputPath := filepath.Join(root, "var", "trusted-runtime-evidence.json")
	_, err := runtime.SaveTrustedRuntimeEvidence(outputPath, runtime.TrustedRuntimeEvidence{
		VerifiedAt:             time.Date(2026, 7, 7, 11, 12, 0, 0, time.UTC),
		Source:                 "stored-trusted-runtime-proof",
		HubUserID:              "hub:user:stored-proof",
		APIURL:                 "http://127.0.0.1:18881",
		WebURL:                 "http://127.0.0.1:15176",
		TrustedBoundaryMode:    "trusted-only",
		APITrustedContextState: "verified",
		APIBrowserQueryState:   "blocked",
		WebTrustedContextState: "verified",
		WebBrowserQueryState:   "isolated",
	})
	if err != nil {
		t.Fatalf("save stored trusted runtime evidence failed: %v", err)
	}

	srv := newServer(nil, nil, nil, nil, runtime.Status{
		Phase:               "team-edition-phase-a",
		JobsEntrypointState: "available",
		JobsRuntimeState:    "ingest-and-read-model-ops-available",
	})
	srv.trustedRuntimeEvidenceRoot = root
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	proof, ok := result["trustedRuntimeEvidence"].(map[string]any)
	if !ok {
		t.Fatalf("expected trusted runtime evidence payload, got %+v", result)
	}
	if proof["source"] != "stored-trusted-runtime-proof" || proof["hubUserId"] != "hub:user:stored-proof" {
		t.Fatalf("expected stored trusted runtime evidence, got %+v", proof)
	}
	if result["liveRuntimeProofState"] != "historical" {
		t.Fatalf("expected historical live runtime proof state from stored evidence, got %+v", result)
	}
}

func TestHandleIngestSummaryReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/summary?hub_user_id=hub:user:alice&device_count=3", nil)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:          "hub:user:alice",
			State:              "ready",
			FreshnessState:     "fresh",
			TotalEvents:        3,
			SourceCount:        2,
			LatestEventAt:      time.Date(2026, 6, 30, 20, 24, 8, 0, time.FixedZone("CST", 8*3600)),
			LastSuccessAt:      time.Date(2026, 6, 30, 20, 24, 8, 0, time.FixedZone("CST", 8*3600)),
			LastFailureAt:      time.Date(2026, 6, 30, 19, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			LastFailureMessage: "timeout",
			LatestBatchID:      "mock-batch-001",
			LastEventType:      "topic_cluster",
			AggregatedVideos:   16,
			AggregatedComments: 174,
			AggregatedTopics:   8,
			RecentEvents: []ingest.Event{{
				ID:           "evt-1",
				BatchID:      "mock-batch-001",
				MachineID:    "machine-01",
				EventType:    "topic_cluster",
				Source:       "insight_topic_cluster",
				CollectedAt:  time.Date(2026, 6, 30, 20, 24, 8, 0, time.FixedZone("CST", 8*3600)),
				VideoCount:   0,
				CommentCount: 0,
				TopicCount:   5,
			}},
			SourceStatuses: []ingest.SourceStatus{{
				Source:         "wx_feed_list",
				State:          "ready",
				FreshnessState: "fresh",
				LastRunAt:      time.Date(2026, 6, 30, 20, 24, 8, 0, time.FixedZone("CST", 8*3600)),
				LastSuccessAt:  time.Date(2026, 6, 30, 20, 24, 8, 0, time.FixedZone("CST", 8*3600)),
				LastFailureAt:  time.Date(2026, 6, 30, 19, 30, 0, 0, time.FixedZone("CST", 8*3600)),
				LastError:      "timeout",
				LatestBatchID:  "mock-batch-001",
				LastEventType:  "topic_cluster",
				EventCount:     3,
			}},
			BindingStates: []ingest.SourceBindingState{{
				Source:        "wx_feed_list",
				BaseURL:       "http://127.0.0.1:2026",
				Username:      "gh_123",
				DeviceCount:   3,
				UpdatedAt:     time.Date(2026, 6, 30, 20, 10, 0, 0, time.FixedZone("CST", 8*3600)),
				LastRunStatus: "failed",
				LastRunAt:     time.Date(2026, 6, 30, 20, 24, 8, 0, time.FixedZone("CST", 8*3600)),
				LastSuccessAt: time.Date(2026, 6, 30, 19, 24, 8, 0, time.FixedZone("CST", 8*3600)),
				LastFailureAt: time.Date(2026, 6, 30, 20, 24, 8, 0, time.FixedZone("CST", 8*3600)),
				LastError:     "timeout",
				LatestBatchID: "mock-batch-001",
				CanRetry:      true,
			}},
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["latestBatchId"] != "mock-batch-001" {
		t.Fatalf("expected batch id, got %v", payload["latestBatchId"])
	}
	if payload["state"] != "ready" {
		t.Fatalf("expected ready state, got %v", payload["state"])
	}
	if payload["freshnessState"] != "fresh" {
		t.Fatalf("expected freshness state, got %v", payload["freshnessState"])
	}
	if payload["lastFailureMessage"] != "timeout" {
		t.Fatalf("expected failure message, got %v", payload["lastFailureMessage"])
	}
	if bindingStates, ok := payload["bindingStates"].([]any); !ok || len(bindingStates) != 1 {
		t.Fatalf("expected one binding state, got %v", payload["bindingStates"])
	}
}

func TestHandleIngestSummaryUsesReadOnlyProjection(t *testing.T) {
	var summaryCalls int
	var readOnlyCalls int
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/summary", nil)

	newServer(nil, nil, stubIngestService{
		summary:              ingest.Summary{LatestBatchID: "live-probe"},
		readOnlySummary:      ingest.Summary{LatestBatchID: "read-only"},
		hasReadOnlySummary:   true,
		summaryCalls:         &summaryCalls,
		readOnlySummaryCalls: &readOnlyCalls,
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if summaryCalls != 0 || readOnlyCalls != 1 {
		t.Fatalf("expected only the read-only summary path, got summary=%d readOnly=%d", summaryCalls, readOnlyCalls)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload["latestBatchId"] != "read-only" {
		t.Fatalf("expected read-only response, got %+v", payload)
	}
}

func TestHandleIngestBindingsReturnsBindingStates(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/bindings?hub_user_id=hub:user:alice&device_count=3", nil)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			BindingStates: []ingest.SourceBindingState{{
				Source:        "wx_comment_export",
				Path:          "E:\\netdisk\\wx-down\\comments.json",
				DeviceCount:   3,
				UpdatedAt:     time.Date(2026, 7, 1, 4, 0, 0, 0, time.FixedZone("CST", 8*3600)),
				LastRunStatus: "failed",
				LastRunAt:     time.Date(2026, 7, 1, 4, 10, 0, 0, time.FixedZone("CST", 8*3600)),
				LastFailureAt: time.Date(2026, 7, 1, 4, 10, 0, 0, time.FixedZone("CST", 8*3600)),
				LastError:     "path missing",
				CanRetry:      true,
			}},
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one binding item, got %v", payload["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first binding item object, got %T", items[0])
	}
	if first["source"] != "wx_comment_export" {
		t.Fatalf("expected wx_comment_export source, got %v", first["source"])
	}
	if first["canRetry"] != true {
		t.Fatalf("expected canRetry true, got %v", first["canRetry"])
	}
}

func TestHandleOperatorRunsReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/operator-runs?hub_user_id=hub:user:alice&device_count=3", nil)

	newServer(nil, nil, stubIngestService{
		runs: []ingest.OperatorRun{{
			ID:               "operator-cycle-001",
			Status:           "failed",
			StartedAt:        time.Date(2026, 7, 3, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			FinishedAt:       time.Date(2026, 7, 3, 20, 1, 0, 0, time.FixedZone("CST", 8*3600)),
			WorkspaceID:      "ws-default-hub-user-alice",
			LatestBatchID:    "batch-008",
			AttemptedSources: 2,
			SucceededSources: 1,
			FailedSources:    1,
			FreshnessState:   "lagging",
			ReportHeadline:   "世界模型热视频继续冲高",
			TopVideoTitle:    "团队运营版验证视频",
			TopTopicKeyword:  "世界模型",
			Message:          "wx_comment_export timeout",
			Outcomes: []ingest.OperatorRunOutcome{{
				Source:  "wx_comment_export",
				Status:  "failed",
				Summary: "wx_comment_export timeout",
			}},
		}},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one operator run item, got %v", payload["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first operator run object, got %T", items[0])
	}
	if first["status"] != "failed" {
		t.Fatalf("expected failed status, got %v", first["status"])
	}
	if first["topTopicKeyword"] != "世界模型" {
		t.Fatalf("expected top topic keyword, got %v", first["topTopicKeyword"])
	}
	outcomes, ok := first["outcomes"].([]any)
	if !ok || len(outcomes) != 1 {
		t.Fatalf("expected operator run outcomes, got %+v", first)
	}
}

func TestHandleOperatorRunsAppliesBoundedLimit(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/operator-runs?limit=1", nil)

	newServer(nil, nil, stubIngestService{
		runs: []ingest.OperatorRun{
			{ID: "operator-cycle-002", Status: "success"},
			{ID: "operator-cycle-001", Status: "failed"},
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one bounded operator run item, got %v", payload["items"])
	}
}

func TestHandleRunOperatorCycleReturnsStructuredSuccessPayload(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/run-cycle?hub_user_id=hub:user:alice&device_count=3", strings.NewReader(`{"sources":["wx_comment_export"]}`))
	req.Header.Set("Content-Type", "application/json")

	runner := stubOperatorRunner{
		result: operatorflow.Result{
			Status:           "ok",
			AttemptedSources: 1,
			Imports: []operatorflow.ImportedSummary{{
				Source: "wx_comment_export",
				Summary: ingest.Summary{
					HubUserID:      "hub:user:alice",
					State:          "ready",
					FreshnessState: "fresh",
					TotalEvents:    3,
					LatestBatchID:  "batch-operator-001",
					LastEventType:  "comment_snapshot",
				},
			}},
			ReadModel: insight.Dashboard{
				Workspace:       insight.WorkspaceSummary{Name: "默认运营工作区", DeviceCount: 3},
				WorkspaceBridge: insight.WorkspaceBridge{WorkspaceID: "ws-default-hub-user-alice", HubUserID: "hub:user:alice", ContextSource: "query"},
				ReportSummary:   insight.ReportSummary{Headline: "今日重点跟进评论追问"},
				IngestSummary:   insight.IngestSummary{FreshnessState: "fresh", LatestBatchID: "batch-operator-001"},
				ReadModel:       insight.ReadModelState{Mode: "materialized", LatestBatchID: "batch-operator-001"},
				HotTopics:       []insight.HotTopic{{ID: "topic-1", Keyword: "世界模型"}},
			},
		},
	}

	newServerWithOperator(nil, nil, nil, nil, runner, nil, runtime.Status{Phase: "team-edition-phase-a"}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("expected ok status, got %+v", payload)
	}
	brief, ok := payload["brief"].(map[string]any)
	if !ok {
		t.Fatalf("expected brief payload, got %+v", payload)
	}
	signals, ok := brief["signals"].(map[string]any)
	if !ok {
		t.Fatalf("expected signals payload, got %+v", brief)
	}
	topTopic, ok := signals["topTopic"].(map[string]any)
	if !ok || topTopic["keyword"] != "世界模型" {
		t.Fatalf("expected top topic keyword, got %+v", signals)
	}
}

func TestHandleRunOperatorLoopPassesSelectedBindingTargets(t *testing.T) {
	var recorded []operatorflow.Options
	runner := stubOperatorRunner{
		recordedOptions: &recorded,
		result: operatorflow.Result{
			Status:           "ok",
			FinalStatus:      "ok",
			AttemptedSources: 1,
			Imports: []operatorflow.ImportedSummary{{
				Source: "wx_comment_export",
				Summary: ingest.Summary{
					HubUserID:      "hub:user:alice",
					State:          "ready",
					FreshnessState: "fresh",
					LatestBatchID:  "batch-selected-comment",
				},
			}},
			ReadModel: insight.Dashboard{
				WorkspaceBridge: insight.WorkspaceBridge{WorkspaceID: "ws-selected", HubUserID: "hub:user:alice"},
				ReadModel:       insight.ReadModelState{Mode: "materialized", LatestBatchID: "batch-selected-comment"},
			},
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/run-loop?hub_user_id=hub:user:alice&device_count=1", strings.NewReader(`{
  "sources": ["wx_comment_export"],
  "targets": [{"source":"wx_comment_export","targetKey":"comment-1"}],
  "strict": true,
  "iterations": 1
}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newServerWithOperator(nil, nil, nil, nil, runner, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(recorded) != 1 || len(recorded[0].TargetRefs) != 1 {
		t.Fatalf("expected one selected target, got %+v", recorded)
	}
	if recorded[0].TargetRefs[0].Source != "wx_comment_export" || recorded[0].TargetRefs[0].TargetKey != "comment-1" {
		t.Fatalf("unexpected selected target: %+v", recorded[0].TargetRefs)
	}
}

func TestHandleRunOperatorCycleUpdatesJobsRuntimeStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs-runtime-state.json")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/run-cycle?hub_user_id=hub:user:alice&device_count=3", strings.NewReader(`{"sources":["wx_comment_export"]}`))
	req.Header.Set("Content-Type", "application/json")

	startedAt := time.Date(2026, 7, 4, 20, 58, 49, 0, time.FixedZone("CST", 8*3600))
	finishedAt := startedAt.Add(3 * time.Second)
	runner := stubOperatorRunner{
		result: operatorflow.Result{
			Status:           "ok",
			FinalStatus:      "ok",
			StartedAt:        startedAt,
			FinishedAt:       finishedAt,
			AttemptedSources: 1,
			Imports: []operatorflow.ImportedSummary{{
				Source: "wx_comment_export",
				Summary: ingest.Summary{
					HubUserID:      "hub:user:alice",
					State:          "ready",
					FreshnessState: "fresh",
					TotalEvents:    3,
					LatestBatchID:  "batch-operator-001",
					LastEventType:  "comment_snapshot",
				},
			}},
			ReadModel: insight.Dashboard{
				Workspace:       insight.WorkspaceSummary{Name: "默认运营工作区", DeviceCount: 3},
				WorkspaceBridge: insight.WorkspaceBridge{WorkspaceID: "ws-default-hub-user-alice", HubUserID: "hub:user:alice", ContextSource: "trusted_header"},
				ReportSummary:   insight.ReportSummary{Headline: "今日重点跟进评论追问"},
				IngestSummary:   insight.IngestSummary{FreshnessState: "fresh", LatestBatchID: "batch-operator-001"},
				ReadModel:       insight.ReadModelState{Mode: "materialized", LatestBatchID: "batch-operator-001"},
			},
		},
	}

	srv := newServerWithOperator(nil, nil, nil, nil, runner, nil, runtime.Status{
		Phase:                   "team-edition-phase-a",
		DashboardStoreMode:      "postgres",
		RankingStoreMode:        "postgres",
		CommentInsightStoreMode: "postgres",
		ReportSummaryStoreMode:  "postgres",
		WorkspaceStoreMode:      "postgres",
		IngestStoreMode:         "postgres",
	})
	srv.jobsStatePath = path
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	state, err := runtime.LoadJobsRuntimeState(path)
	if err != nil {
		t.Fatalf("load jobs runtime state failed: %v", err)
	}
	if state.LastMode != "operator-cycle" || state.LastStatus != "ok" {
		t.Fatalf("expected operator-cycle ok state, got %+v", state)
	}
	if state.LastHubUserID != "hub:user:alice" {
		t.Fatalf("expected hub user persisted, got %+v", state)
	}
	if state.LastRequestedCount != 1 {
		t.Fatalf("expected requested source count 1, got %+v", state)
	}
	if len(state.LastRequestedSources) != 1 || state.LastRequestedSources[0] != "wx_comment_export" {
		t.Fatalf("expected requested source list persisted, got %+v", state)
	}
	if !state.LastStartedAt.Equal(startedAt) || !state.LastFinishedAt.Equal(finishedAt) {
		t.Fatalf("expected run times persisted, got %+v", state)
	}

	statusRec := httptest.NewRecorder()
	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil)
	srv.routes().ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected runtime status 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(statusRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid runtime status json, got %v", err)
	}
	if payload["jobsLastMode"] != "operator-cycle" || payload["jobsLastStatus"] != "ok" {
		t.Fatalf("expected runtime status to reflect latest operator cycle, got %+v", payload)
	}
	if payload["jobsLastHubUserId"] != "hub:user:alice" {
		t.Fatalf("expected runtime status hub user updated, got %+v", payload)
	}
	requestedSources, ok := payload["jobsLastRequestedSources"].([]any)
	if !ok || len(requestedSources) != 1 || requestedSources[0] != "wx_comment_export" {
		t.Fatalf("expected runtime status requested sources updated, got %+v", payload)
	}
	if payload["ownerBoundaryState"] != "met" {
		t.Fatalf("expected runtime status owner boundary state, got %+v", payload)
	}
	if payload["ownerBoundaryEvidence"] == "" || payload["ownerBoundarySummary"] == "" {
		t.Fatalf("expected runtime status owner boundary evidence payload, got %+v", payload)
	}
}

func TestHandleRunOperatorCycleStrictFailureReturnsUnprocessableEntityWithPayload(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/run-cycle?hub_user_id=hub:user:alice&device_count=3", strings.NewReader(`{"sources":["wx_comment_export"],"strict":true}`))
	req.Header.Set("Content-Type", "application/json")

	runner := stubOperatorRunner{
		result: operatorflow.Result{
			Status:           "failed",
			AttemptedSources: 1,
			Message:          "wx_comment_export path missing",
			ReadModel: insight.Dashboard{
				WorkspaceBridge: insight.WorkspaceBridge{WorkspaceID: "ws-default-hub-user-alice", HubUserID: "hub:user:alice"},
			},
		},
		err: operatorflow.ExecutionError{Err: errors.New("wx_comment_export path missing")},
	}

	newServerWithOperator(nil, nil, nil, nil, runner, nil, runtime.Status{Phase: "team-edition-phase-a"}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["status"] != "failed" || payload["error"] == "" {
		t.Fatalf("expected failed payload with error, got %+v", payload)
	}
	syncResult, ok := payload["sync"].(map[string]any)
	if !ok || syncResult["status"] != "failed" {
		t.Fatalf("expected failed sync payload, got %+v", payload)
	}
}

func TestHandleRunOperatorCycleReturnsPartialPayloadWithoutStrict(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/run-cycle?hub_user_id=hub:user:alice&device_count=3", strings.NewReader(`{"sources":[]}`))
	req.Header.Set("Content-Type", "application/json")

	runner := stubOperatorRunner{
		result: operatorflow.Result{
			Status:           "partial",
			FinalStatus:      "partial",
			AttemptedSources: 1,
			Skipped: []operatorflow.Skipped{{
				Source: "wx_feed_list",
				Reason: "视频列表来源尚未保存，今天这轮会缺少热视频和热话题输入",
			}},
			ReadModel: insight.Dashboard{
				WorkspaceBridge: insight.WorkspaceBridge{WorkspaceID: "ws-default-hub-user-alice", HubUserID: "hub:user:alice", ContextSource: "query"},
				ReportSummary:   insight.ReportSummary{Headline: "今日先看评论追问"},
				IngestSummary:   insight.IngestSummary{FreshnessState: "fresh", LatestBatchID: "batch-operator-002"},
				ReadModel:       insight.ReadModelState{Mode: "materialized", LatestBatchID: "batch-operator-002"},
			},
		},
	}

	newServerWithOperator(nil, nil, nil, nil, runner, nil, runtime.Status{Phase: "team-edition-phase-a"}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["status"] != "partial" {
		t.Fatalf("expected partial status, got %+v", payload)
	}
	syncResult, ok := payload["sync"].(map[string]any)
	if !ok || syncResult["finalStatus"] != "partial" {
		t.Fatalf("expected partial sync payload, got %+v", payload)
	}
}

func TestHandleRunOperatorLoopWritesLoopJobsStateAndUsesLoopTrigger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs-runtime-state.json")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/run-loop?hub_user_id=hub:user:alice&device_count=3", strings.NewReader(`{"iterations":1}`))
	req.Header.Set("Content-Type", "application/json")

	startedAt := time.Date(2026, 7, 7, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	finishedAt := startedAt.Add(4 * time.Second)
	recordedOptions := []operatorflow.Options{}
	runner := stubOperatorRunner{
		recordedOptions: &recordedOptions,
		result: operatorflow.Result{
			Status:           "ok",
			FinalStatus:      "ok",
			StartedAt:        startedAt,
			FinishedAt:       finishedAt,
			AttemptedSources: 2,
			Imports: []operatorflow.ImportedSummary{
				{
					Source: "wx_comment_export",
					Summary: ingest.Summary{
						HubUserID:      "hub:user:alice",
						State:          "ready",
						FreshnessState: "fresh",
						TotalEvents:    3,
						LatestBatchID:  "batch-loop-001",
						LastEventType:  "comment_snapshot",
					},
				},
				{
					Source: "wx_video_search",
					Summary: ingest.Summary{
						HubUserID:        "hub:user:alice",
						State:            "ready",
						FreshnessState:   "fresh",
						TotalEvents:      5,
						LatestBatchID:    "batch-loop-001",
						LastEventType:    "feed_snapshot",
						AggregatedVideos: 12,
						AggregatedTopics: 4,
					},
				},
			},
			ReadModel: insight.Dashboard{
				Workspace:       insight.WorkspaceSummary{Name: "默认运营工作区", DeviceCount: 3},
				WorkspaceBridge: insight.WorkspaceBridge{WorkspaceID: "ws-default-hub-user-alice", HubUserID: "hub:user:alice", ContextSource: "trusted_header"},
				ReportSummary:   insight.ReportSummary{Headline: "这轮 loop 已补上双来源证据"},
				IngestSummary:   insight.IngestSummary{FreshnessState: "fresh", LatestBatchID: "batch-loop-001"},
				ReadModel:       insight.ReadModelState{Mode: "materialized", LatestBatchID: "batch-loop-001"},
				HotTopics:       []insight.HotTopic{{ID: "topic-1", Keyword: "世界模型"}},
			},
		},
	}

	srv := newServerWithOperator(nil, nil, nil, nil, runner, nil, runtime.Status{
		Phase:                   "team-edition-phase-a",
		DashboardStoreMode:      "postgres",
		RankingStoreMode:        "postgres",
		CommentInsightStoreMode: "postgres",
		ReportSummaryStoreMode:  "postgres",
		WorkspaceStoreMode:      "postgres",
		IngestStoreMode:         "postgres",
	})
	srv.jobsStatePath = path
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(recordedOptions) != 1 || recordedOptions[0].TriggerMode != ingest.OperatorTriggerLoop {
		t.Fatalf("expected operator loop trigger mode, got %+v", recordedOptions)
	}

	state, err := runtime.LoadJobsRuntimeState(path)
	if err != nil {
		t.Fatalf("load jobs runtime state failed: %v", err)
	}
	if state.LastMode != "operator-loop" || state.LastStatus != "ok" {
		t.Fatalf("expected operator-loop ok state, got %+v", state)
	}
	if state.LastIterations != 1 || state.LastIteration != 1 {
		t.Fatalf("expected single bounded loop iteration, got %+v", state)
	}
	if state.LastInterval != "" {
		t.Fatalf("expected empty interval for one-shot loop proof, got %+v", state)
	}
	if len(state.LastRequestedSources) != 2 || state.LastRequestedSources[0] != "wx_comment_export" || state.LastRequestedSources[1] != "wx_video_search" {
		t.Fatalf("expected two primary sources persisted for loop proof, got %+v", state)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid json, got %v", err)
	}
	loop, ok := payload["loop"].(map[string]any)
	if !ok {
		t.Fatalf("expected loop metadata payload, got %+v", payload)
	}
	if loop["iteration"] != float64(1) || loop["iterations"] != float64(1) {
		t.Fatalf("expected loop iteration metadata, got %+v", loop)
	}
}

func TestHandleRunOperatorLoopRejectsTooManyIterations(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator/run-loop?hub_user_id=hub:user:alice&device_count=3", strings.NewReader(`{"iterations":4}`))
	req.Header.Set("Content-Type", "application/json")

	calls := 0
	runner := stubOperatorRunner{
		callCount: &calls,
		result: operatorflow.Result{
			Status:      "ok",
			FinalStatus: "ok",
		},
	}

	newServerWithOperator(nil, nil, nil, nil, runner, nil, runtime.Status{Phase: "team-edition-phase-a"}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if calls != 0 {
		t.Fatalf("expected validation failure before running operator, got %d calls", calls)
	}
	if !strings.Contains(rec.Body.String(), "iterations must be 3 or less") {
		t.Fatalf("expected iterations validation message, got %s", rec.Body.String())
	}
}

func TestHandleAppendMockBatchReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/mock-batch?hub_user_id=hub:user:alice&device_count=3", nil)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:   "hub:user:alice",
			State:       "ready",
			TotalEvents: 3,
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestHandleImportEventsReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"events":[{"id":"evt-real-1","eventType":"comment_snapshot","source":"feed_comment_list","collectedAt":"2026-07-01T01:20:00+08:00","keyword":"世界模型怎么真正落地","representativeComment":"别只讲路线，先说落地场景。","commentCount":88,"topicCount":4}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/events/import?hub_user_id=hub:user:alice&device_count=3", body)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:          "hub:user:alice",
			State:              "ready",
			TotalEvents:        1,
			SourceCount:        1,
			LatestEventAt:      time.Date(2026, 7, 1, 1, 20, 0, 0, time.FixedZone("CST", 8*3600)),
			LastEventType:      "comment_snapshot",
			AggregatedComments: 88,
			AggregatedTopics:   4,
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["lastEventType"] != "comment_snapshot" {
		t.Fatalf("expected comment_snapshot, got %v", payload["lastEventType"])
	}
}

func TestHandleImportWXCommentExportReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"path":"E:\\\\netdisk\\\\wx-down\\\\comments.json"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/wx-channel/comment-export/import?hub_user_id=hub:user:alice&device_count=3", body)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:          "hub:user:alice",
			State:              "ready",
			TotalEvents:        3,
			SourceCount:        1,
			LatestBatchID:      "import-20260701T040000.000000000Z",
			LastEventType:      "video_snapshot",
			AggregatedVideos:   1,
			AggregatedComments: 24,
			AggregatedTopics:   3,
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["latestBatchId"] != "import-20260701T040000.000000000Z" {
		t.Fatalf("expected import batch id, got %v", payload["latestBatchId"])
	}
}

func TestHandleSyncWXCommentExportReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"baseUrl":"http://127.0.0.1:2026","objectId":"oid-sync-1","nonceId":"nid-sync-1","title":"同步评论样本","author":"Insight Test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/wx-channel/comment-export/sync?hub_user_id=hub:user:alice&device_count=3", body)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:          "hub:user:alice",
			State:              "ready",
			TotalEvents:        3,
			SourceCount:        1,
			LatestBatchID:      "import-20260701T043000.000000000Z",
			LastEventType:      "video_snapshot",
			AggregatedVideos:   1,
			AggregatedComments: 12,
			AggregatedTopics:   3,
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["latestBatchId"] != "import-20260701T043000.000000000Z" {
		t.Fatalf("expected synced import batch id, got %v", payload["latestBatchId"])
	}
}

func TestHandleSaveWXCommentExportBindingReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"path":"E:\\\\netdisk\\\\wx-down\\\\comment_data\\\\2026-07-04\\\\comment-export.json"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/wx-channel/comment-export/save?hub_user_id=hub:user:alice&device_count=3", body)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:      "hub:user:alice",
			State:          "idle",
			FreshnessState: "idle",
			BindingStates: []ingest.SourceBindingState{{
				Source:      "wx_comment_export",
				Path:        "E:\\netdisk\\wx-down\\comment_data\\2026-07-04\\comment-export.json",
				DeviceCount: 3,
				Runnable:    true,
			}},
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	bindingStates, ok := payload["bindingStates"].([]any)
	if !ok || len(bindingStates) != 1 {
		t.Fatalf("expected one binding state in payload, got %v", payload["bindingStates"])
	}
}

func TestHandleImportWXFeedListReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"baseUrl":"http://127.0.0.1:2026","username":"gh_123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/wx-channel/feed-list/import?hub_user_id=hub:user:alice&device_count=3", body)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:        "hub:user:alice",
			State:            "ready",
			TotalEvents:      2,
			SourceCount:      1,
			LatestBatchID:    "import-20260701T050000.000000000Z",
			LastEventType:    "video_snapshot",
			AggregatedVideos: 2,
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["aggregatedVideos"] != float64(2) {
		t.Fatalf("expected aggregatedVideos 2, got %v", payload["aggregatedVideos"])
	}
}

func TestHandleSearchWXFeedAccountsReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/wx-channel/feed-list/search?keyword=teamlight&base_url=http://127.0.0.1:2026", nil)

	newServer(nil, nil, stubIngestService{
		accounts: []ingest.WXFeedAccount{{
			Username:  "v2_060000231003b20faec8c5e28b10c6d6ce01ea36b077f6a0e58226dc63fe48550812f2bdbca9@finder",
			Nickname:  "TEAMLIGHT-",
			Signature: "",
		}},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one search item, got %v", payload["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object item, got %T", items[0])
	}
	if first["nickname"] != "TEAMLIGHT-" {
		t.Fatalf("expected nickname TEAMLIGHT-, got %v", first["nickname"])
	}
}

func TestHandleSearchWXVideosReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/wx-channel/video-search/search?keyword=AI&base_url=http://127.0.0.1:2026", nil)

	newServer(nil, nil, stubIngestService{
		videoSearch: ingest.WXFeedListResult{
			NextMarker: "next",
			Items: []ingest.WXFeedListItem{{
				ID:              "feed-1",
				ObjectNonceID:   "nonce-1",
				Title:           "AI 热门视频",
				ChannelName:     "科技号",
				ChannelUsername: "tech@finder",
				PublishedAt:     time.Date(2026, 8, 14, 10, 30, 0, 0, time.FixedZone("CST", 8*60*60)),
				CoverURL:        "https://example.com/cover.jpg",
				Tags:            []string{"AI", "科技"},
				PlayCount:       12000,
				LikeCount:       360,
				CommentCount:    88,
				FavCount:        42,
				ForwardCount:    18,
				Signal:          "评论活跃",
			}},
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one video search item, got %v", payload["items"])
	}
	if payload["keyword"] != "AI" || payload["nextMarker"] != "next" {
		t.Fatalf("expected keyword/nextMarker, got %v", payload)
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected video search item object, got %T", items[0])
	}
	if first["channelUsername"] != "tech@finder" || first["publishedAt"] != "2026-08-14T10:30:00+08:00" {
		t.Fatalf("expected identity and publishedAt, got %v", first)
	}
	if first["playCount"] != float64(12000) || first["likeCount"] != float64(360) || first["commentCount"] != float64(88) || first["favCount"] != float64(42) || first["forwardCount"] != float64(18) {
		t.Fatalf("expected video metrics, got %v", first)
	}
}

func TestHandleImportWXVideoSearchReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"baseUrl":"http://127.0.0.1:2026","keyword":"AI"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/wx-channel/video-search/import?hub_user_id=hub:user:alice&device_count=3", body)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:        "hub:user:alice",
			State:            "ready",
			TotalEvents:      3,
			SourceCount:      1,
			LatestBatchID:    "import-20260701T053000.000000000Z",
			LastEventType:    "video_snapshot",
			AggregatedVideos: 3,
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["aggregatedVideos"] != float64(3) {
		t.Fatalf("expected aggregatedVideos 3, got %v", payload["aggregatedVideos"])
	}
}

func TestHandleImportWXVideoShareReturnsVideoMetadataAndPublicationTime(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"baseUrl":"http://127.0.0.1:2026","shareUrl":"https://weixin.qq.com/sph/share-video-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/wx-channel/video-share/import?hub_user_id=hub:user:alice", body)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:        "hub:user:alice",
			State:            "ready",
			AggregatedVideos: 1,
		},
		videoProfile: ingest.WXFeedListItem{
			ID:              "shared-video-1",
			ObjectNonceID:   "shared-nonce-1",
			Title:           "从分享链接识别的视频",
			ChannelName:     "分享账号",
			ChannelUsername: "shared_account@finder",
			PublishedAt:     time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
			CoverURL:        "https://cdn.example.test/shared-cover.jpg",
			LikeCount:       41,
			CommentCount:    12,
			FavCount:        9,
			ForwardCount:    3,
			VideoURL:        "https://cdn.example.test/shared.mp4",
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		InputURL string `json:"inputUrl"`
		Video    struct {
			ID                string `json:"id"`
			Source            string `json:"source"`
			PublishedAt       string `json:"publishedAt"`
			LikeCount         int    `json:"likeCount"`
			CommentCount      int    `json:"commentCount"`
			FavCount          int    `json:"favCount"`
			ForwardCount      int    `json:"forwardCount"`
			VideoURLAvailable bool   `json:"videoUrlAvailable"`
		} `json:"video"`
		Summary struct {
			AggregatedVideos int `json:"aggregatedVideos"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload.InputURL != "https://weixin.qq.com/sph/share-video-1" || payload.Video.ID != "shared-video-1" {
		t.Fatalf("expected shared video identity, got %+v", payload)
	}
	if payload.Video.Source != ingest.RunSourceWXManualShare || payload.Video.PublishedAt == "" {
		t.Fatalf("expected manual source and publication time, got %+v", payload.Video)
	}
	if payload.Video.LikeCount != 41 || payload.Video.CommentCount != 12 || payload.Video.FavCount != 9 || payload.Video.ForwardCount != 3 {
		t.Fatalf("expected four interaction metrics, got %+v", payload.Video)
	}
	if !payload.Video.VideoURLAvailable || payload.Summary.AggregatedVideos != 1 {
		t.Fatalf("expected playable metadata and summary, got %+v", payload)
	}
}

func TestHandleSaveWXFeedListBindingReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"baseUrl":"http://127.0.0.1:2026","username":"gh_saved"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/wx-channel/feed-list/save?hub_user_id=hub:user:alice&device_count=3", body)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:      "hub:user:alice",
			State:          "idle",
			FreshnessState: "idle",
			BindingStates: []ingest.SourceBindingState{{
				Source:      "wx_feed_list",
				BaseURL:     "http://127.0.0.1:2026",
				Username:    "gh_saved",
				DeviceCount: 3,
				Runnable:    true,
			}},
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	bindingStates, ok := payload["bindingStates"].([]any)
	if !ok || len(bindingStates) != 1 {
		t.Fatalf("expected one binding state in payload, got %v", payload["bindingStates"])
	}
}

func TestHandleSaveWXVideoSearchBindingReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"baseUrl":"http://127.0.0.1:2026","keyword":"AI"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/wx-channel/video-search/save?hub_user_id=hub:user:alice&device_count=3", body)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:      "hub:user:alice",
			State:          "idle",
			FreshnessState: "idle",
			BindingStates: []ingest.SourceBindingState{{
				Source:      "wx_video_search",
				BaseURL:     "http://127.0.0.1:2026",
				Keyword:     "AI",
				DeviceCount: 3,
				Runnable:    true,
			}},
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	bindingStates, ok := payload["bindingStates"].([]any)
	if !ok || len(bindingStates) != 1 {
		t.Fatalf("expected one binding state in payload, got %v", payload["bindingStates"])
	}
}

func TestHandleRetryIngestReturnsTransportResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"source":"wx_feed_list"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/retry?hub_user_id=hub:user:alice&device_count=3", body)

	newServer(nil, nil, stubIngestService{
		summary: ingest.Summary{
			HubUserID:        "hub:user:alice",
			State:            "ready",
			TotalEvents:      2,
			SourceCount:      1,
			LatestBatchID:    "import-20260701T060000.000000000Z",
			LastEventType:    "video_snapshot",
			AggregatedVideos: 2,
			BindingStates: []ingest.SourceBindingState{{
				Source:        "wx_feed_list",
				BaseURL:       "http://127.0.0.1:2026",
				Username:      "gh_123",
				DeviceCount:   3,
				LastRunStatus: "success",
				CanRetry:      false,
			}},
		},
	}, nil, runtime.Status{}).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["latestBatchId"] != "import-20260701T060000.000000000Z" {
		t.Fatalf("expected latest batch id, got %v", payload["latestBatchId"])
	}
}
