package insight_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	"github.com/nobiyou/wx_channel_insight/internal/rediscache"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspacedemo"
)

type staticDashboardRepo struct {
	payload insight.Dashboard
}

func (r staticDashboardRepo) GetDashboard(context.Context, string) (insight.Dashboard, error) {
	return r.payload, nil
}

func (r staticDashboardRepo) ReplaceDashboard(context.Context, string, insight.Dashboard) error {
	return nil
}

func (r staticDashboardRepo) ResetDashboard(context.Context, string) error {
	return nil
}

type recordingDashboardRepo struct {
	payload       insight.Dashboard
	replaced      insight.Dashboard
	replacedKey   string
	replaceCalled bool
}

type staticIngestSummaryProvider struct {
	summary ingest.Summary
}

type readOnlyIngestSummaryProbe struct {
	staticIngestSummaryProvider
	liveCalls     int
	readOnlyCalls int
}

func (p *readOnlyIngestSummaryProbe) GetSummary(context.Context, ingest.SummaryQuery) (ingest.Summary, error) {
	p.liveCalls++
	return p.summary, nil
}

func (p *readOnlyIngestSummaryProbe) GetSummaryReadOnly(context.Context, ingest.SummaryQuery) (ingest.Summary, error) {
	p.readOnlyCalls++
	return p.summary, nil
}

type launchReadinessDashboardRepo struct {
	fullPayload     insight.Dashboard
	projection      insight.Dashboard
	fullCalls       int
	projectionCalls int
}

type boundedPageRankingRepo struct {
	*insightdemo.RankingRepository
}

func (r boundedPageRankingRepo) GetDashboardPageRankings(ctx context.Context, workspaceID string) ([]insight.HotVideo, insight.HotVideoRankings, bool, []insight.HotTopic, bool, error) {
	videos, videosFound, err := r.GetHotVideos(ctx, workspaceID)
	if err != nil {
		return nil, nil, false, nil, false, err
	}
	rankings, rankingsFound, err := r.GetHotVideoRankings(ctx, workspaceID)
	if err != nil {
		return nil, nil, false, nil, false, err
	}
	topics, topicsFound, err := r.GetHotTopics(ctx, workspaceID)
	if err != nil {
		return nil, nil, false, nil, false, err
	}
	if len(videos) > insight.DashboardPageHotVideoLimit {
		videos = videos[:insight.DashboardPageHotVideoLimit]
	}
	boundedRankings := make(insight.HotVideoRankings, len(rankings))
	for key, items := range rankings {
		if len(items) > insight.DashboardPageHotVideoLimit {
			items = items[:insight.DashboardPageHotVideoLimit]
		}
		boundedRankings[key] = insight.CloneHotVideos(items)
	}
	if len(topics) > insight.DashboardPageHotTopicLimit {
		topics = topics[:insight.DashboardPageHotTopicLimit]
	}
	return insight.CloneHotVideos(videos), boundedRankings, videosFound || rankingsFound, insight.CloneHotTopics(topics), topicsFound, nil
}

func (p staticIngestSummaryProvider) GetSummary(context.Context, ingest.SummaryQuery) (ingest.Summary, error) {
	return p.summary, nil
}

func (r *launchReadinessDashboardRepo) GetDashboard(context.Context, string) (insight.Dashboard, error) {
	r.fullCalls++
	return r.fullPayload, nil
}

func (r *launchReadinessDashboardRepo) GetDashboardLaunchReadiness(context.Context, string) (insight.Dashboard, error) {
	r.projectionCalls++
	return r.projection, nil
}

func (r *launchReadinessDashboardRepo) ReplaceDashboard(context.Context, string, insight.Dashboard) error {
	return nil
}

func (r *launchReadinessDashboardRepo) ResetDashboard(context.Context, string) error {
	return nil
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

func TestDashboardServiceGetDashboardReturnsIsolatedDiagnostics(t *testing.T) {
	connectedCount := 0
	repo := staticDashboardRepo{
		payload: insight.Dashboard{
			IngestSummary: insight.IngestSummary{
				BindingStates: []insight.IngestBindingState{{
					Source: "wx_feed_list",
					Diagnostic: &insight.BindingDiagnostic{
						WXChannelStatusError: "timeout after 1s",
						ConnectedClientCount: &connectedCount,
					},
				}},
				OperatorReadiness: insight.OperatorReadiness{
					Blocked: []insight.OperatorReadinessSource{{
						Source: "wx_feed_list",
						Diagnostic: &insight.BindingDiagnostic{
							WXChannelStatusError: "timeout after 1s",
						},
					}},
				},
			},
		},
	}
	service := insight.NewDashboardService(repo, insight.DashboardServiceConfig{})

	first, err := service.GetDashboard(context.Background(), insight.DashboardQuery{})
	if err != nil {
		t.Fatalf("get first dashboard failed: %v", err)
	}
	first.IngestSummary.BindingStates[0].Diagnostic.WXChannelStatusError = "mutated"
	*first.IngestSummary.BindingStates[0].Diagnostic.ConnectedClientCount = 99
	first.IngestSummary.OperatorReadiness.Blocked[0].Diagnostic.WXChannelStatusError = "mutated"

	second, err := service.GetDashboard(context.Background(), insight.DashboardQuery{})
	if err != nil {
		t.Fatalf("get second dashboard failed: %v", err)
	}
	bindingDiagnostic := second.IngestSummary.BindingStates[0].Diagnostic
	if bindingDiagnostic.WXChannelStatusError != "timeout after 1s" {
		t.Fatalf("expected binding diagnostic to stay isolated, got %+v", bindingDiagnostic)
	}
	if bindingDiagnostic.ConnectedClientCount == nil || *bindingDiagnostic.ConnectedClientCount != 0 {
		t.Fatalf("expected copied connected client count to stay isolated, got %+v", bindingDiagnostic)
	}
	readinessDiagnostic := second.IngestSummary.OperatorReadiness.Blocked[0].Diagnostic
	if readinessDiagnostic.WXChannelStatusError != "timeout after 1s" {
		t.Fatalf("expected readiness diagnostic to stay isolated, got %+v", readinessDiagnostic)
	}
}

func TestDashboardServiceGetLaunchReadinessDashboardUsesMinimalProjection(t *testing.T) {
	repo := &launchReadinessDashboardRepo{
		fullPayload: insight.Dashboard{
			HotVideos: []insight.HotVideo{{ID: "must-not-load"}},
		},
		projection: insight.Dashboard{
			ReadModel:       insight.ReadModelState{Mode: "materialized", LatestBatchID: "batch-7"},
			WorkspaceBridge: insight.WorkspaceBridge{WorkspaceID: "ws-default-hub-user-demo"},
		},
	}
	resolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	provider := &readOnlyIngestSummaryProbe{
		staticIngestSummaryProvider: staticIngestSummaryProvider{summary: ingest.Summary{
			HubUserID:     "hub:user:demo",
			State:         "ready",
			TotalEvents:   5977,
			LatestBatchID: "batch-7",
		}},
	}
	service := insight.NewDashboardService(repo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: resolver,
		IngestService:            provider,
	})

	payload, err := service.GetLaunchReadinessDashboard(context.Background(), insight.DashboardQuery{})
	if err != nil {
		t.Fatalf("get launch-readiness dashboard failed: %v", err)
	}
	if repo.projectionCalls != 1 || repo.fullCalls != 0 {
		t.Fatalf("expected only the launch projection read, got projection=%d full=%d", repo.projectionCalls, repo.fullCalls)
	}
	if provider.liveCalls != 0 || provider.readOnlyCalls != 1 {
		t.Fatalf("runtime reads must use persisted source facts: live=%d readOnly=%d", provider.liveCalls, provider.readOnlyCalls)
	}
	if len(payload.HotVideos) != 0 {
		t.Fatalf("launch readiness must not load dashboard collections, got %+v", payload.HotVideos)
	}
	if payload.ReadModel.Mode != "materialized" || payload.IngestSummary.TotalEvents != 5977 {
		t.Fatalf("expected launch readiness facts to remain available, got %+v", payload)
	}
}

func TestDashboardServicePreservesBoardsMissingFromLegacyRankingSnapshot(t *testing.T) {
	rankingRepo := insightdemo.NewRankingRepository(nil, nil)
	if err := rankingRepo.ReplaceHotVideoRankings(context.Background(), "ws-default-hub-user-demo", insight.HotVideoRankings{
		insight.HotVideoRankingHot: {{ID: "legacy-hot", Title: "旧综合榜"}},
	}); err != nil {
		t.Fatalf("replace legacy rankings failed: %v", err)
	}

	service := insight.NewDashboardService(staticDashboardRepo{payload: insight.Dashboard{
		HotVideoRankings: insight.HotVideoRankings{
			insight.HotVideoRankingLikes:    {{ID: "likes-1", Title: "点赞榜"}},
			insight.HotVideoRankingComments: {{ID: "comments-1", Title: "评论榜"}},
		},
	}}, insight.DashboardServiceConfig{
		WorkspaceContextResolver: workspace.NewContextResolver(
			workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
			workspace.ContextResolverConfig{
				DefaultHubUserID:   "hub:user:demo",
				DefaultDeviceCount: 2,
			},
		),
		RankingRepository: rankingRepo,
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{})
	if err != nil {
		t.Fatalf("get dashboard failed: %v", err)
	}
	if len(payload.HotVideoRankings[insight.HotVideoRankingHot]) != 1 || payload.HotVideoRankings[insight.HotVideoRankingHot][0].ID != "legacy-hot" {
		t.Fatalf("expected legacy hot ranking to remain authoritative, got %+v", payload.HotVideoRankings)
	}
	if payload.HotVideoRankings[insight.HotVideoRankingLikes][0].ID != "likes-1" || payload.HotVideoRankings[insight.HotVideoRankingComments][0].ID != "comments-1" {
		t.Fatalf("expected missing legacy boards to be preserved, got %+v", payload.HotVideoRankings)
	}
}

func TestDashboardServiceReturnsCoreSections(t *testing.T) {
	service := insight.NewDashboardService(insightdemo.NewDashboardRepository(), insight.DashboardServiceConfig{
		WorkspaceContextResolver: workspace.NewContextResolver(
			workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
			workspace.ContextResolverConfig{
				DefaultHubUserID:   "hub:user:demo",
				DefaultDeviceCount: 6,
			},
		),
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if payload.Workspace.Name == "" {
		t.Fatal("workspace name should not be empty")
	}
	if len(payload.HotVideos) < 3 {
		t.Fatalf("expected at least 3 hot videos, got %d", len(payload.HotVideos))
	}
	if len(payload.HotTopics) < 3 {
		t.Fatalf("expected at least 3 hot topics, got %d", len(payload.HotTopics))
	}
	if len(payload.WatchlistAlerts) == 0 {
		t.Fatal("expected watchlist alerts")
	}
	if len(payload.CommentInsights) == 0 {
		t.Fatal("expected comment insights")
	}
	if payload.WorkspaceBridge.WorkspaceID == "" {
		t.Fatal("expected workspace bridge workspace id")
	}
	if payload.WorkspaceBridge.Resolution != "created" {
		t.Fatalf("expected created resolution, got %q", payload.WorkspaceBridge.Resolution)
	}
	if payload.WorkspaceBridge.ContextSource != "default" {
		t.Fatalf("expected default context source, got %q", payload.WorkspaceBridge.ContextSource)
	}
}

func TestDashboardServicePrefersRedisDashboardCacheWhenAvailable(t *testing.T) {
	client, err := rediscache.NewClientWithKV("127.0.0.1:6379", "", 0, &stubKV{})
	if err != nil {
		t.Fatalf("build redis stub client failed: %v", err)
	}
	cache := insight.NewReadCache(client, time.Minute)
	resolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	workspaceContext, err := resolver.Resolve(context.Background(), workspace.ContextQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	})
	if err != nil {
		t.Fatalf("resolve workspace failed: %v", err)
	}
	if err := cache.SaveDashboard(context.Background(), workspaceContext.Workspace.ID, insight.Dashboard{
		ReadModel: insight.ReadModelState{
			Mode:          "materialized",
			LatestBatchID: "cache-batch-001",
		},
	}); err != nil {
		t.Fatalf("seed dashboard cache failed: %v", err)
	}

	service := insight.NewDashboardService(staticDashboardRepo{
		payload: insight.Dashboard{
			ReadModel: insight.ReadModelState{
				Mode:          "materialized",
				LatestBatchID: "repo-batch-001",
			},
		},
	}, insight.DashboardServiceConfig{
		WorkspaceContextResolver: resolver,
		ReadCache:                cache,
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	})
	if err != nil {
		t.Fatalf("get dashboard failed: %v", err)
	}
	if payload.ReadModel.LatestBatchID != "cache-batch-001" {
		t.Fatalf("expected dashboard cache hit, got %+v", payload.ReadModel)
	}
}

func TestDashboardServiceReadOnlyDashboardSkipsLargeDashboardCache(t *testing.T) {
	store := &stubKV{}
	client, err := rediscache.NewClientWithKV("127.0.0.1:6379", "", 0, store)
	if err != nil {
		t.Fatalf("build redis stub client failed: %v", err)
	}
	cache := insight.NewReadCache(client, time.Minute)
	resolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	workspaceContext, err := resolver.Resolve(context.Background(), workspace.ContextQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	})
	if err != nil {
		t.Fatalf("resolve workspace failed: %v", err)
	}
	if err := cache.SaveDashboard(context.Background(), workspaceContext.Workspace.ID, insight.Dashboard{
		ReadModel: insight.ReadModelState{LatestBatchID: "cache-batch-001"},
	}); err != nil {
		t.Fatalf("seed dashboard cache failed: %v", err)
	}

	service := insight.NewDashboardService(staticDashboardRepo{
		payload: insight.Dashboard{
			ReadModel: insight.ReadModelState{LatestBatchID: "repo-batch-001"},
		},
	}, insight.DashboardServiceConfig{
		WorkspaceContextResolver: resolver,
		ReadCache:                cache,
	})

	payload, err := service.GetReadOnlyDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	})
	if err != nil {
		t.Fatalf("get read-only dashboard failed: %v", err)
	}
	if payload.ReadModel.LatestBatchID != "repo-batch-001" {
		t.Fatalf("read-only dashboard must use persisted snapshot, got %+v", payload.ReadModel)
	}
	if !strings.Contains(store.values["insight:dashboard:"+workspaceContext.Workspace.ID], "cache-batch-001") {
		t.Fatalf("read-only dashboard must not overwrite the dashboard cache")
	}
}

func TestDashboardServiceReadOnlyDashboardKeepsBoundedSnapshotCollections(t *testing.T) {
	resolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	workspaceContext, err := resolver.Resolve(context.Background(), workspace.ContextQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	})
	if err != nil {
		t.Fatalf("resolve workspace failed: %v", err)
	}
	rankingRepo := boundedPageRankingRepo{RankingRepository: insightdemo.NewRankingRepository(nil, nil)}
	completeVideos := make([]insight.HotVideo, 0, insight.DashboardPageHotVideoLimit+1)
	for index := 0; index <= insight.DashboardPageHotVideoLimit; index++ {
		completeVideos = append(completeVideos, insight.HotVideo{ID: fmt.Sprintf("complete-ranking-row-%02d", index)})
	}
	if err := rankingRepo.ReplaceHotVideos(context.Background(), workspaceContext.Workspace.ID, completeVideos); err != nil {
		t.Fatalf("seed complete ranking failed: %v", err)
	}
	service := insight.NewDashboardService(staticDashboardRepo{payload: insight.Dashboard{
		HotVideos: []insight.HotVideo{{ID: "bounded-snapshot-row"}},
	}}, insight.DashboardServiceConfig{
		WorkspaceContextResolver: resolver,
		RankingRepository:        rankingRepo,
	})

	page, err := service.GetReadOnlyDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	})
	if err != nil {
		t.Fatalf("get read-only dashboard failed: %v", err)
	}
	if len(page.HotVideos) != insight.DashboardPageHotVideoLimit || page.HotVideos[0].ID != "complete-ranking-row-00" {
		t.Fatalf("dashboard page must use the bounded current ranking projection, got %+v", page.HotVideos)
	}

	full, err := service.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	})
	if err != nil {
		t.Fatalf("get full dashboard failed: %v", err)
	}
	if len(full.HotVideos) != insight.DashboardPageHotVideoLimit+1 || full.HotVideos[0].ID != "complete-ranking-row-00" {
		t.Fatalf("full dashboard must retain the complete ranking overlay, got %+v", full.HotVideos)
	}
}

func TestDashboardServicePreservesExplicitContextSource(t *testing.T) {
	service := insight.NewDashboardService(insightdemo.NewDashboardRepository(), insight.DashboardServiceConfig{
		WorkspaceContextResolver: workspace.NewContextResolver(
			workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
			workspace.ContextResolverConfig{
				DefaultHubUserID:   "hub:user:demo",
				DefaultDeviceCount: 6,
			},
		),
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   9,
		ContextSource: "header",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if payload.Workspace.Owner != "hub:user:alice" {
		t.Fatalf("expected explicit hub user owner, got %q", payload.Workspace.Owner)
	}
	if payload.Workspace.DeviceCount != 9 {
		t.Fatalf("expected explicit device count 9, got %d", payload.Workspace.DeviceCount)
	}
	if payload.WorkspaceBridge.ContextSource != "header" {
		t.Fatalf("expected header context source, got %q", payload.WorkspaceBridge.ContextSource)
	}
}

func TestDashboardServiceOverlaysOverviewFromIngestSummary(t *testing.T) {
	ingestRepo := ingestdemo.NewRepository()
	base := time.Date(2026, 6, 30, 11, 0, 0, 0, time.FixedZone("CST", 8*3600))
	callCount := 0
	ingestService := ingest.NewService(ingestRepo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			value := base.Add(time.Duration(callCount) * 10 * time.Minute)
			callCount++
			return value
		},
	})
	for i := 0; i < 3; i++ {
		if _, err := ingestService.AppendMockBatch(context.Background(), ingest.SummaryQuery{
			HubUserID:   "hub:user:alice",
			DeviceCount: 3,
		}); err != nil {
			t.Fatalf("expected append mock batch success, got %v", err)
		}
	}
	summary, err := ingestService.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 3,
	})
	if err != nil {
		t.Fatalf("expected ingest summary success, got %v", err)
	}
	baseRepo := insightdemo.NewDashboardRepository()
	baseSeed, err := baseRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("expected demo seed success, got %v", err)
	}
	materializedAt := summary.LatestEventAt.Add(2 * time.Hour)
	baseSeed.GeneratedAt = materializedAt

	service := insight.NewDashboardService(staticDashboardRepo{payload: baseSeed}, insight.DashboardServiceConfig{
		WorkspaceContextResolver: workspace.NewContextResolver(
			workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
			workspace.ContextResolverConfig{
				DefaultHubUserID:   "hub:user:demo",
				DefaultDeviceCount: 6,
			},
		),
		IngestService: ingestService,
		Now: func() time.Time {
			return materializedAt
		},
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   3,
		ContextSource: "query",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if payload.Overview.TrackedVideos != 60 {
		t.Fatalf("expected tracked videos from ingest overlay, got %d", payload.Overview.TrackedVideos)
	}
	if payload.Overview.MonitoredAccounts != 3 {
		t.Fatalf("expected monitored accounts to follow unique ingest channels, got %d", payload.Overview.MonitoredAccounts)
	}
	if payload.Overview.NewCommentBursts != 10 {
		t.Fatalf("expected comment bursts from ingest overlay, got %d", payload.Overview.NewCommentBursts)
	}
	if payload.Overview.EmergingTopics != 33 {
		t.Fatalf("expected emerging topics from ingest overlay, got %d", payload.Overview.EmergingTopics)
	}
	if payload.Overview.DailyCoverageRate != 66.7 {
		t.Fatalf("expected coverage rate to follow ingest source/device coverage, got %v", payload.Overview.DailyCoverageRate)
	}
	if payload.Workspace.DeviceCount != 3 {
		t.Fatalf("expected workspace device count to stay on workspace context, got %d", payload.Workspace.DeviceCount)
	}
	expectedGeneratedAt := materializedAt
	if !payload.GeneratedAt.Equal(expectedGeneratedAt) {
		t.Fatalf("expected generatedAt to follow materialization time, got %s", payload.GeneratedAt.Format(time.RFC3339))
	}
	if payload.Overview.ReportFreshness != "2 小时前" {
		t.Fatalf("expected report freshness to reflect data age at materialization, got %q", payload.Overview.ReportFreshness)
	}
	if payload.HotVideos[0].PublishedAgo != "2 小时前" {
		t.Fatalf("expected ingest hot video age to reflect data age at materialization, got %q", payload.HotVideos[0].PublishedAgo)
	}
	if len(payload.WatchlistAlerts) < 3 || payload.WatchlistAlerts[0].ID != "alert-video-mock-batch-003-video-01" {
		t.Fatalf("expected ingest alert to be prepended, got %+v", payload.WatchlistAlerts)
	}
	if payload.WatchlistAlerts[0].HappenedAt != "2 小时前" {
		t.Fatalf("expected ingest alert age to reflect data age at materialization, got %q", payload.WatchlistAlerts[0].HappenedAt)
	}
	if payload.WatchlistAlerts[1].ID != "alert-comment-mock-batch-003" || payload.WatchlistAlerts[2].ID != "alert-topic-mock-batch-003" {
		t.Fatalf("expected multiple ingest alerts first, got %+v", payload.WatchlistAlerts[:3])
	}
	if payload.ReportSummary.Headline == "" || payload.ReportSummary.KeyFinding == "" {
		t.Fatalf("expected report summary to be overlaid from ingest, got %+v", payload.ReportSummary)
	}
	if payload.ReportSummary.AutoGeneratedAt != "刚刚" {
		t.Fatalf("expected auto generated at to follow materialization time, got %q", payload.ReportSummary.AutoGeneratedAt)
	}
	if payload.ReportSummary.Period != "2026-06-30 日报" {
		t.Fatalf("expected report period to follow ingest time, got %q", payload.ReportSummary.Period)
	}
	if payload.ReportSummary.Headline != "门店团购预算、直播标题模板 等 3 组主题正在走高，最新 ingest 已覆盖 2 个采集来源。" {
		t.Fatalf("expected report headline from top ingest topics, got %q", payload.ReportSummary.Headline)
	}
	if payload.ReportSummary.KeyFinding == "" || payload.ReportSummary.Watchout == "" {
		t.Fatalf("expected report text from ingest signals, got %+v", payload.ReportSummary)
	}
	if len(payload.HotVideos) < 3 || payload.HotVideos[0].ID != "ingest-video-mock-batch-003-video-01" {
		t.Fatalf("expected ingest hot video to be prepended, got %+v", payload.HotVideos)
	}
	if payload.HotVideos[0].Title == "" || payload.HotVideos[0].ChannelName != "增长实验室" {
		t.Fatalf("expected ingest hot video metadata, got %+v", payload.HotVideos[0])
	}
	if len(payload.HotVideos) < 3 || payload.HotVideos[1].ChannelName != "操盘手日报" || payload.HotVideos[2].ChannelName != "Insight Mock Radar" {
		t.Fatalf("expected three ingest-derived hot videos first, got %+v", payload.HotVideos[:3])
	}
	if len(payload.HotTopics) < 3 || payload.HotTopics[0].ID != "ingest-topic-mock-batch-003-topic-03" {
		t.Fatalf("expected ingest hot topic to be prepended, got %+v", payload.HotTopics)
	}
	if payload.HotTopics[0].Keyword != "门店团购预算" || payload.HotTopics[0].Cluster == "" {
		t.Fatalf("expected ingest hot topic metadata, got %+v", payload.HotTopics[0])
	}
	if len(payload.HotTopics) < 3 || payload.HotTopics[1].Keyword != "直播标题模板" || payload.HotTopics[2].Keyword != "世界模型" {
		t.Fatalf("expected three ingest-derived hot topics first, got %+v", payload.HotTopics[:3])
	}
	if len(payload.CommentInsights) < 3 || payload.CommentInsights[0].ID != "ci-ingest-mock-batch-003" {
		t.Fatalf("expected ingest comment insight to be prepended, got %+v", payload.CommentInsights)
	}
	if payload.CommentInsights[0].Theme != "门店团购预算和回本周期怎么算" || payload.CommentInsights[0].Representative == "" {
		t.Fatalf("expected ingest comment insight metadata, got %+v", payload.CommentInsights[0])
	}
	if len(payload.CommentInsights) < 3 || payload.CommentInsights[1].Theme != "标题模板到底怎么按客单价分层" || payload.CommentInsights[2].Theme != "世界模型怎么真正落地" {
		t.Fatalf("expected three ingest-derived comment insights first, got %+v", payload.CommentInsights[:3])
	}
	if payload.ReportSummary.RecommendedAction == "" || payload.ReportSummary.Watchout == "" {
		t.Fatalf("expected ingest report text to stay populated, got %+v", payload.ReportSummary)
	}
}

func TestDashboardServiceDerivesTopicAndCommentSignalsFromVideoOnlyIngest(t *testing.T) {
	ingestRepo := ingestdemo.NewRepository()
	ingestService := ingest.NewService(ingestRepo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 2, 19, 45, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	_, err := ingestService.ImportEvents(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, []ingest.Event{
		{
			ID:           "wx-feed-video-1",
			HubUserID:    "hub:user:alice",
			MachineID:    "wx-channel-feed-list-02",
			EventType:    "video_snapshot",
			Source:       "wx_channel_feed_list",
			CollectedAt:  time.Date(2026, 7, 2, 19, 44, 34, 0, time.FixedZone("CST", 8*3600)),
			VideoCount:   1,
			CommentCount: 132,
			TopicCount:   2,
			Title:        "当摄影学会了广西运镜",
			ChannelName:  "婉琳hajime呀",
			Signal:       "高评论密度",
			Tags:         []string{"广西运镜", "摄影运镜"},
		},
		{
			ID:           "wx-feed-video-2",
			HubUserID:    "hub:user:alice",
			MachineID:    "wx-channel-feed-list-02",
			EventType:    "video_snapshot",
			Source:       "wx_channel_feed_list",
			CollectedAt:  time.Date(2026, 7, 2, 19, 43, 34, 0, time.FixedZone("CST", 8*3600)),
			VideoCount:   1,
			CommentCount: 77,
			TopicCount:   2,
			Title:        "我不信你两眼空空",
			ChannelName:  "婉琳hajime呀",
			Signal:       "评论活跃",
			Tags:         []string{"棕色尘埃2", "白毛"},
		},
		{
			ID:           "wx-feed-video-3",
			HubUserID:    "hub:user:alice",
			MachineID:    "wx-channel-feed-list-02",
			EventType:    "video_snapshot",
			Source:       "wx_channel_feed_list",
			CollectedAt:  time.Date(2026, 7, 2, 19, 42, 34, 0, time.FixedZone("CST", 8*3600)),
			VideoCount:   1,
			CommentCount: 48,
			TopicCount:   2,
			Title:        "万物生于大地亦终将归于大地",
			ChannelName:  "婉琳hajime呀",
			Signal:       "评论活跃",
			Tags:         []string{"闪烁之光", "卡牌"},
		},
	})
	if err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	service := insight.NewDashboardService(insightdemo.NewDashboardRepository(), insight.DashboardServiceConfig{
		WorkspaceContextResolver: workspace.NewContextResolver(
			workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
			workspace.ContextResolverConfig{
				DefaultHubUserID:   "hub:user:demo",
				DefaultDeviceCount: 6,
			},
		),
		IngestService: ingestService,
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "query",
	})
	if err != nil {
		t.Fatalf("expected dashboard success, got %v", err)
	}

	if payload.HotVideos[0].Comments != 132 {
		t.Fatalf("expected hot video comment count to stay per-video, got %+v", payload.HotVideos[0])
	}
	if payload.Overview.MonitoredAccounts != 1 {
		t.Fatalf("expected monitored accounts to collapse to real ingest channel count, got %d", payload.Overview.MonitoredAccounts)
	}
	if payload.Overview.DailyCoverageRate != 50 {
		t.Fatalf("expected coverage rate to reflect 1/2 ingest sources, got %v", payload.Overview.DailyCoverageRate)
	}
	if payload.HotVideos[0].ID == payload.HotVideos[1].ID {
		t.Fatalf("expected hot videos to keep unique ids, got %+v", payload.HotVideos[:2])
	}
	if payload.WatchlistAlerts[0].ID == payload.WatchlistAlerts[1].ID {
		t.Fatalf("expected watchlist alerts to keep unique ids, got %+v", payload.WatchlistAlerts[:2])
	}
	if !strings.Contains(payload.HotVideos[0].ID, payload.IngestSummary.LatestBatchID) {
		t.Fatalf("expected real ingest hot video id to keep latest batch marker, got %+v", payload.HotVideos[0])
	}
	if !strings.Contains(payload.WatchlistAlerts[0].ID, payload.IngestSummary.LatestBatchID) {
		t.Fatalf("expected real ingest watchlist alert id to keep latest batch marker, got %+v", payload.WatchlistAlerts[0])
	}
	if !strings.Contains(payload.CommentInsights[0].ID, payload.IngestSummary.LatestBatchID) {
		t.Fatalf("expected real ingest comment insight id to keep latest batch marker, got %+v", payload.CommentInsights[0])
	}
	if payload.HotTopics[0].Keyword != "广西运镜" {
		t.Fatalf("expected first hot topic to derive from video tags, got %+v", payload.HotTopics[0])
	}
	if payload.CommentInsights[0].Theme != "广西运镜" {
		t.Fatalf("expected first comment insight to derive from video tags, got %+v", payload.CommentInsights[0])
	}
	if !strings.Contains(payload.CommentInsights[0].Representative, "132") {
		t.Fatalf("expected comment insight representative to mention real comment count, got %+v", payload.CommentInsights[0])
	}
	if !strings.Contains(payload.ReportSummary.Headline, "广西运镜") {
		t.Fatalf("expected report headline to derive from video topics, got %+v", payload.ReportSummary)
	}
	if strings.Contains(payload.CommentInsights[0].Theme, "世界模型") {
		t.Fatalf("expected video-only ingest to displace old mock comment theme, got %+v", payload.CommentInsights[0])
	}
}

func TestDashboardServiceSkipsDuplicateOverlayForMaterializedSameBatch(t *testing.T) {
	ingestRepo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 3, 10, 14, 0, 0, time.FixedZone("CST", 8*3600))
	ingestService := ingest.NewService(ingestRepo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	_, err := ingestService.ImportEvents(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, []ingest.Event{
		{
			ID:                    "comment-dup-1",
			HubUserID:             "hub:user:alice",
			BatchID:               "batch-dup-1",
			MachineID:             "machine-1",
			EventType:             "comment_snapshot",
			Source:                "wx_channel_comment_export",
			CollectedAt:           now,
			VideoCount:            1,
			CommentCount:          1,
			TopicCount:            1,
			Title:                 "团队运营版要怎么落地",
			ChannelName:           "Insight Test",
			Keyword:               "团队运营版",
			RepresentativeComment: "先说说怎么落地",
			Keywords:              []string{"团队运营版", "日报"},
		},
	})
	if err != nil {
		t.Fatalf("expected import success, got %v", err)
	}

	workspaceService := workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository())
	contextResolver := workspace.NewContextResolver(workspaceService, workspace.ContextResolverConfig{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
	})
	baseRepo := insightdemo.NewDashboardRepository()
	seed, err := baseRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("expected seed load success, got %v", err)
	}
	workspaceContext, err := contextResolver.Resolve(context.Background(), workspace.ContextQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	})
	if err != nil {
		t.Fatalf("expected workspace context success, got %v", err)
	}
	summary, err := ingestService.GetSummary(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	})
	if err != nil {
		t.Fatalf("expected ingest summary success, got %v", err)
	}

	materialized := insight.NewMaterializer(seed).Build(summary, workspaceContext, "jobs")
	service := insight.NewDashboardService(staticDashboardRepo{payload: materialized}, insight.DashboardServiceConfig{
		WorkspaceContextResolver: contextResolver,
		IngestService:            ingestService,
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	})
	if err != nil {
		t.Fatalf("expected dashboard success, got %v", err)
	}
	if payload.ReadModel.Mode != "materialized" {
		t.Fatalf("expected materialized read model, got %+v", payload.ReadModel)
	}
	if payload.HotVideos[0].Title != materialized.HotVideos[0].Title {
		t.Fatalf("expected no duplicate overlay on hot videos, got %+v want %+v", payload.HotVideos[0], materialized.HotVideos[0])
	}
	if payload.CommentInsights[0].Theme != materialized.CommentInsights[0].Theme {
		t.Fatalf("expected no duplicate overlay on comment insights, got %+v want %+v", payload.CommentInsights[0], materialized.CommentInsights[0])
	}
	if len(payload.ReadModel.ViewStatuses) != 4 {
		t.Fatalf("expected view statuses to be populated, got %+v", payload.ReadModel.ViewStatuses)
	}
	if payload.ReadModel.ViewStatuses[0].Label == "" || payload.ReadModel.ViewStatuses[0].Summary == "" {
		t.Fatalf("expected first view status to include label and summary, got %+v", payload.ReadModel.ViewStatuses[0])
	}
}

func TestDashboardServiceRefreshesStoredRelativeTimeLabelsOnRead(t *testing.T) {
	observedAt := time.Date(2026, 7, 5, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))
	materializedAt := observedAt.Add(3 * time.Hour)
	servedAt := materializedAt.Add(5 * time.Hour)

	service := insight.NewDashboardService(staticDashboardRepo{payload: insight.Dashboard{
		GeneratedAt: materializedAt,
		ReadModel: insight.ReadModelState{
			Mode:           "materialized",
			MaterializedAt: materializedAt,
			Source:         "jobs",
			LatestBatchID:  "batch-1",
		},
		IngestSummary: insight.IngestSummary{
			State:          "ready",
			FreshnessState: "stale",
			LatestBatchID:  "batch-1",
			LatestEventAt:  observedAt,
		},
		Overview: insight.OverviewMetrics{
			ReportFreshness: "刚刚",
		},
		HotVideos: []insight.HotVideo{{
			ID:           "video-1",
			Title:        "aged video",
			ChannelName:  "Insight Test",
			PublishedAt:  observedAt,
			PublishedAgo: "刚刚",
		}},
		WatchlistAlerts: []insight.WatchlistAlert{{
			ID:         "alert-1",
			Summary:    "aged alert",
			OccurredAt: observedAt,
			HappenedAt: "刚刚",
		}},
		ReportSummary: insight.ReportSummary{
			Period:            "2026-07-05 日报",
			Headline:          "aged report",
			KeyFinding:        "aged finding",
			RecommendedAction: "aged action",
			Watchout:          "aged watchout",
			GeneratedAt:       materializedAt,
			AutoGeneratedAt:   "刚刚",
		},
	}}, insight.DashboardServiceConfig{
		WorkspaceContextResolver: workspace.NewContextResolver(
			workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
			workspace.ContextResolverConfig{
				DefaultHubUserID:   "hub:user:demo",
				DefaultDeviceCount: 2,
			},
		),
		Now: func() time.Time {
			return servedAt
		},
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	})
	if err != nil {
		t.Fatalf("expected dashboard success, got %v", err)
	}
	if payload.Overview.ReportFreshness != "8 小时前" {
		t.Fatalf("expected report freshness to refresh on read, got %q", payload.Overview.ReportFreshness)
	}
	if payload.HotVideos[0].PublishedAgo != "8 小时前" {
		t.Fatalf("expected hot video age to refresh on read, got %q", payload.HotVideos[0].PublishedAgo)
	}
	if payload.WatchlistAlerts[0].HappenedAt != "8 小时前" {
		t.Fatalf("expected watchlist age to refresh on read, got %q", payload.WatchlistAlerts[0].HappenedAt)
	}
	if payload.ReportSummary.AutoGeneratedAt != "5 小时前" {
		t.Fatalf("expected report autoGeneratedAt to refresh on read, got %q", payload.ReportSummary.AutoGeneratedAt)
	}
}

func TestDashboardServiceUsesDedicatedReadModelMetaWhenAvailable(t *testing.T) {
	rankingRepo := insightdemo.NewRankingRepository(
		[]insight.HotVideo{{ID: "seed-video", Title: "seed", ChannelName: "seed"}},
		[]insight.HotTopic{{ID: "seed-topic", Keyword: "seed", Cluster: "seed"}},
	)
	commentRepo := insightdemo.NewCommentInsightRepository([]insight.CommentInsight{{
		ID:             "seed-comment",
		Theme:          "seed",
		Representative: "seed",
	}})
	reportRepo := insightdemo.NewReportSummaryRepository(insight.ReportSummary{
		Period:            "2026-07-03 日报",
		Headline:          "seed",
		KeyFinding:        "seed",
		RecommendedAction: "seed",
		Watchout:          "seed",
		AutoGeneratedAt:   "刚刚",
	})
	metaTime := time.Date(2026, 7, 3, 11, 0, 0, 0, time.FixedZone("CST", 8*3600))
	meta := insight.ReadModelArtifactMeta{
		LatestBatchID:  "batch-meta-001",
		MaterializedAt: metaTime,
	}
	if err := rankingRepo.ReplaceHotVideoMeta(context.Background(), "ws-default-hub-user-alice", meta); err != nil {
		t.Fatalf("seed hot video meta failed: %v", err)
	}
	if err := rankingRepo.ReplaceHotTopicMeta(context.Background(), "ws-default-hub-user-alice", meta); err != nil {
		t.Fatalf("seed hot topic meta failed: %v", err)
	}
	if err := commentRepo.ReplaceCommentInsightMeta(context.Background(), "ws-default-hub-user-alice", meta); err != nil {
		t.Fatalf("seed comment meta failed: %v", err)
	}
	if err := reportRepo.ReplaceReportSummaryMeta(context.Background(), "ws-default-hub-user-alice", meta); err != nil {
		t.Fatalf("seed report meta failed: %v", err)
	}

	service := insight.NewDashboardService(insightdemo.NewDashboardRepository(), insight.DashboardServiceConfig{
		WorkspaceContextResolver: workspace.NewContextResolver(
			workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
			workspace.ContextResolverConfig{
				DefaultHubUserID:   "hub:user:demo",
				DefaultDeviceCount: 2,
			},
		),
		RankingRepository:        rankingRepo,
		CommentInsightRepository: commentRepo,
		ReportSummaryRepository:  reportRepo,
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "query",
	})
	if err != nil {
		t.Fatalf("expected dashboard success, got %v", err)
	}

	if len(payload.ReadModel.ViewStatuses) != 4 {
		t.Fatalf("expected view statuses, got %+v", payload.ReadModel.ViewStatuses)
	}
	if !payload.ReadModel.ViewStatuses[0].MaterializedAt.Equal(metaTime) {
		t.Fatalf("expected dedicated meta materializedAt, got %+v", payload.ReadModel.ViewStatuses[0])
	}
	if payload.ReadModel.ViewStatuses[0].LatestBatchID != "batch-meta-001" {
		t.Fatalf("expected dedicated meta batch id, got %+v", payload.ReadModel.ViewStatuses[0])
	}
}

func TestDashboardServiceAutoMaterializesDashboardWhenIngestIsNewerThanSeed(t *testing.T) {
	ingestRepo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 3, 13, 0, 0, 0, time.FixedZone("CST", 8*3600))
	ingestService := ingest.NewService(ingestRepo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	_, err := ingestService.ImportEvents(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, []ingest.Event{
		{
			ID:                    "auto-refresh-comment-1",
			BatchID:               "batch-auto-001",
			HubUserID:             "hub:user:alice",
			MachineID:             "machine-auto",
			EventType:             "comment_snapshot",
			Source:                ingest.RunSourceWXCommentExport,
			CollectedAt:           now,
			CommentCount:          1,
			TopicCount:            1,
			Keyword:               "自动物化",
			RepresentativeComment: "首页应该直接看到 materialized",
			Keywords:              []string{"自动物化", "dashboard"},
		},
	})
	if err != nil {
		t.Fatalf("import events failed: %v", err)
	}

	baseRepo := insightdemo.NewDashboardRepository()
	seed, err := baseRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("load seed failed: %v", err)
	}
	repo := &recordingDashboardRepo{payload: seed}
	resolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)

	service := insight.NewDashboardService(repo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: resolver,
		IngestService:            ingestService,
		Materializer:             insight.NewMaterializer(seed),
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "trusted_header",
	})
	if err != nil {
		t.Fatalf("get dashboard failed: %v", err)
	}

	if !repo.replaceCalled {
		t.Fatalf("expected dashboard auto materialization to persist snapshot")
	}
	if repo.replacedKey != "ws-default-hub-user-alice" {
		t.Fatalf("expected workspace-scoped replace, got %q", repo.replacedKey)
	}
	if payload.ReadModel.Mode != "materialized" {
		t.Fatalf("expected materialized read model, got %+v", payload.ReadModel)
	}
	if payload.ReadModel.Source != "auto_refresh" {
		t.Fatalf("expected auto_refresh source, got %+v", payload.ReadModel)
	}
	if payload.WorkspaceBridge.ContextSource != "trusted_header" {
		t.Fatalf("expected request context source to survive auto materialization, got %+v", payload.WorkspaceBridge)
	}
	if payload.ReadModel.LatestBatchID != "batch-auto-001" {
		t.Fatalf("expected latest batch id to follow ingest, got %+v", payload.ReadModel)
	}
	if len(payload.CommentInsights) == 0 || payload.CommentInsights[0].Theme != "自动物化" {
		t.Fatalf("expected ingest-derived materialized payload, got %+v", payload.CommentInsights)
	}
	if repo.replaced.ReadModel.Mode != "materialized" || repo.replaced.ReadModel.LatestBatchID != "batch-auto-001" {
		t.Fatalf("expected persisted snapshot to be materialized, got %+v", repo.replaced.ReadModel)
	}
}

func TestDashboardServiceReadOnlyDashboardDoesNotMaterialize(t *testing.T) {
	seed := insight.Dashboard{
		ReadModel: insight.ReadModelState{
			Mode:          "materialized",
			LatestBatchID: "old-batch",
			Source:        "jobs",
		},
	}
	repo := &recordingDashboardRepo{payload: seed}
	resolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	service := insight.NewDashboardService(repo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: resolver,
		IngestService: staticIngestSummaryProvider{summary: ingest.Summary{
			HubUserID:     "hub:user:alice",
			TotalEvents:   1,
			LatestBatchID: "new-batch",
		}},
		Materializer: insight.NewMaterializer(seed),
	})

	payload, err := service.GetReadOnlyDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "trusted_header",
	})
	if err != nil {
		t.Fatalf("get read-only dashboard failed: %v", err)
	}
	if repo.replaceCalled {
		t.Fatalf("read-only dashboard must not persist materialization")
	}
	if payload.ReadModel.LatestBatchID != "old-batch" || payload.ReadModel.Source != "jobs" {
		t.Fatalf("read-only dashboard should retain persisted read model, got %+v", payload.ReadModel)
	}
}

func TestDashboardServiceReMaterializesWhenHotVideoIdentifiersAreMissingInCurrentBatch(t *testing.T) {
	ingestRepo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 7, 22, 30, 0, 0, time.FixedZone("CST", 8*3600))
	ingestService := ingest.NewService(ingestRepo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	_, err := ingestService.ImportEvents(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, []ingest.Event{{
		ID:            "wx-feed-video-live-1",
		BatchID:       "batch-feed-identifiers-001",
		HubUserID:     "hub:user:alice",
		MachineID:     "machine-feed",
		EventType:     "video_snapshot",
		Source:        "wx_channel_feed_list",
		CollectedAt:   now,
		ObjectID:      "oid-live-1",
		ObjectNonceID: "nid-live-1",
		VideoCount:    1,
		CommentCount:  18,
		TopicCount:    2,
		Title:         "live feed identifier repair",
		ChannelName:   "Insight Team",
		Signal:        "评论活跃",
		Tags:          []string{"live", "repair"},
	}})
	if err != nil {
		t.Fatalf("import events failed: %v", err)
	}

	baseRepo := insightdemo.NewDashboardRepository()
	seed, err := baseRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("load seed failed: %v", err)
	}
	seed.ReadModel = insight.ReadModelState{
		Mode:           "materialized",
		Source:         "jobs",
		LatestBatchID:  "batch-feed-identifiers-001",
		MaterializedAt: now.Add(-10 * time.Minute),
	}
	seed.HotVideos = []insight.HotVideo{{
		ID:              "ingest-video-batch-feed-identifiers-001-wx-feed-video-live-1",
		Title:           "live feed identifier repair",
		ChannelName:     "Insight Team",
		Source:          "wx_channel_feed_list",
		PublishedAt:     now,
		PublishedAgo:    "10 分钟前",
		Views:           1000,
		Likes:           200,
		Comments:        18,
		GrowthRate:      20,
		EngagementScore: 35,
		Signal:          "评论活跃",
	}}
	repo := &recordingDashboardRepo{payload: seed}
	resolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	normalizer := normalize.NewService(normalize.NewMemoryRepository(), normalize.ServiceConfig{
		WorkspaceContextResolver: resolver,
		IngestEvents:             ingestService,
		Now: func() time.Time {
			return now
		},
	})
	aggregator := insight.NewAggregationService(aggregate.NewMemoryRepository(), insight.AggregationServiceConfig{
		Normalizer: normalizer,
		Now: func() time.Time {
			return now
		},
	})

	service := insight.NewDashboardService(repo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: resolver,
		IngestService:            ingestService,
		Normalizer:               normalizer,
		Aggregator:               aggregator,
		Materializer:             insight.NewMaterializer(seed),
	})

	payload, err := service.GetDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "trusted_header",
	})
	if err != nil {
		t.Fatalf("get dashboard failed: %v", err)
	}

	if !repo.replaceCalled {
		t.Fatalf("expected same-batch dashboard to re-materialize when identifiers are missing")
	}
	if len(payload.HotVideos) == 0 {
		t.Fatalf("expected hot videos in payload")
	}
	if payload.WorkspaceBridge.ContextSource != "trusted_header" {
		t.Fatalf("expected request context source to survive same-batch re-materialization, got %+v", payload.WorkspaceBridge)
	}
	if payload.HotVideos[0].ObjectID != "oid-live-1" || payload.HotVideos[0].ObjectNonceID != "nid-live-1" {
		t.Fatalf("expected repaired hot video identifiers, got %+v", payload.HotVideos[0])
	}
	if repo.replaced.ReadModel.LatestBatchID != "batch-feed-identifiers-001" {
		t.Fatalf("expected re-materialized batch id to stay on current batch, got %+v", repo.replaced.ReadModel)
	}
}

func TestDashboardServiceGetLaunchReadinessDashboardSkipsAutoMaterialization(t *testing.T) {
	ingestRepo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 8, 9, 10, 0, 0, time.FixedZone("CST", 8*3600))
	ingestService := ingest.NewService(ingestRepo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})

	_, err := ingestService.ImportEvents(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, []ingest.Event{{
		ID:                    "wx-comment-launch-status-1",
		BatchID:               "batch-launch-status-001",
		HubUserID:             "hub:user:alice",
		MachineID:             "machine-launch-status",
		EventType:             "comment_snapshot",
		Source:                ingest.RunSourceWXCommentExport,
		CollectedAt:           now,
		CommentCount:          1,
		TopicCount:            1,
		Keyword:               "runtime status",
		RepresentativeComment: "launch readiness should stay read-only",
		Keywords:              []string{"runtime", "status"},
	}})
	if err != nil {
		t.Fatalf("import events failed: %v", err)
	}

	baseRepo := insightdemo.NewDashboardRepository()
	seed, err := baseRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("load seed failed: %v", err)
	}
	seed.ReadModel = insight.ReadModelState{
		Mode:           "materialized",
		Source:         "jobs",
		LatestBatchID:  "batch-launch-status-stale",
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

	service := insight.NewDashboardService(repo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: resolver,
		IngestService:            ingestService,
		Materializer:             insight.NewMaterializer(seed),
	})

	payload, err := service.GetLaunchReadinessDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "trusted_header",
	})
	if err != nil {
		t.Fatalf("get launch readiness dashboard failed: %v", err)
	}

	if repo.replaceCalled {
		t.Fatalf("expected launch readiness dashboard to stay read-only")
	}
	if payload.ReadModel.LatestBatchID != "batch-launch-status-stale" {
		t.Fatalf("expected read model batch to stay on persisted snapshot, got %+v", payload.ReadModel)
	}
	if payload.IngestSummary.LatestBatchID != "batch-launch-status-001" {
		t.Fatalf("expected ingest summary to reflect current batch, got %+v", payload.IngestSummary)
	}
	if payload.WorkspaceBridge.ContextSource != "trusted_header" {
		t.Fatalf("expected workspace bridge context source to be preserved, got %+v", payload.WorkspaceBridge)
	}
}
