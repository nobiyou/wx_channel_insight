package insight_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspacedemo"
)

type automaticDashboardRepository struct {
	insight.DashboardRepository
	fail bool
}

func (r *automaticDashboardRepository) ReplaceDashboard(ctx context.Context, id string, d insight.Dashboard) error {
	if r.fail {
		return errors.New("publish failed")
	}
	return r.DashboardRepository.ReplaceDashboard(ctx, id, d)
}

func TestReadModelNotifiesAutomationOnlyAfterSuccessfulPublication(t *testing.T) {
	ctx := context.Background()
	repo := &automaticDashboardRepository{DashboardRepository: insightdemo.NewDashboardRepository()}
	svc := insight.NewReadModelRefreshService(insight.ReadModelRefreshServiceConfig{
		Repository: repo, Materializer: insight.NewMaterializer(insight.Dashboard{}),
	})
	w := workspace.ResolvedContext{Workspace: workspace.DefaultWorkspace{ID: "auto-ws", Name: "Auto test", OwnerUserID: "alice", DeviceCount: 2}}
	calls := 0
	summary := ingest.Summary{TotalEvents: 1, LatestEventAt: time.Now()}
	svc.SetAfterRefresh(func(got workspace.ResolvedContext) {
		calls++
		if got.Workspace.ID != w.Workspace.ID || got.Workspace.OwnerUserID != "alice" {
			t.Fatal("wrong notification owner")
		}
		saved, err := repo.GetDashboard(ctx, got.Workspace.ID)
		if err != nil || saved.ReadModel.Mode != "materialized" {
			t.Fatal("notification preceded persistence")
		}
	})
	if _, err := svc.MaterializeDashboard(ctx, w, summary, "jobs"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("successful publication was not notified")
	}
	repo.fail = true
	if _, err := svc.MaterializeDashboard(ctx, w, summary, "api"); err == nil {
		t.Fatal("expected failed publication")
	}
	if calls != 1 {
		t.Fatal("failed publication triggered inspection")
	}
}

func TestReadModelRefreshServiceMaterializesDashboardFromSeed(t *testing.T) {
	dashboardRepo := insightdemo.NewDashboardRepository()
	seed, err := dashboardRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("load seed failed: %v", err)
	}

	workspaceService := workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository())
	contextResolver := workspace.NewContextResolver(workspaceService, workspace.ContextResolverConfig{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
	})
	ingestRepo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 3, 10, 14, 0, 0, time.FixedZone("CST", 8*3600))
	ingestService := ingest.NewService(ingestRepo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return now
		},
	})
	_, err = ingestService.ImportEvents(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, []ingest.Event{
		{
			ID:                    "comment-1",
			HubUserID:             "hub:user:alice",
			BatchID:               "batch-1",
			MachineID:             "machine-1",
			EventType:             "comment_snapshot",
			Source:                "wx_channel_comment_export",
			CollectedAt:           now,
			ObjectID:              "oid-comment-1",
			ObjectNonceID:         "nid-comment-1",
			VideoCount:            1,
			CommentCount:          1,
			TopicCount:            1,
			Title:                 "团队运营版要怎么落地",
			ChannelName:           "Insight Test",
			Keyword:               "团队运营版",
			Cluster:               "执行规划",
			Signal:                "高追问",
			PromptAngle:           "先给运营台一个能每天看的主视图",
			RepresentativeComment: "先说说怎么落地",
			TargetAudience:        "运营负责人",
			Keywords:              []string{"团队运营版", "日报"},
		},
	})
	if err != nil {
		t.Fatalf("import events failed: %v", err)
	}

	service := insight.NewReadModelRefreshService(insight.ReadModelRefreshServiceConfig{
		Repository:               dashboardRepo,
		WorkspaceContextResolver: contextResolver,
		IngestService:            ingestService,
		Materializer:             insight.NewMaterializer(seed),
	})

	payload, err := service.RefreshDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	}, "jobs")
	if err != nil {
		t.Fatalf("refresh dashboard failed: %v", err)
	}

	if payload.ReadModel.Mode != "materialized" {
		t.Fatalf("expected materialized mode, got %+v", payload.ReadModel)
	}
	if payload.Workspace.Owner != "hub:user:alice" {
		t.Fatalf("expected workspace owner from context, got %+v", payload.Workspace)
	}
	if payload.IngestSummary.LatestBatchID == "" {
		t.Fatalf("expected latest batch id, got %+v", payload.IngestSummary)
	}
	if len(payload.CommentInsights) == 0 || payload.CommentInsights[0].Theme != "团队运营版" {
		t.Fatalf("expected ingest-derived comment insight, got %+v", payload.CommentInsights)
	}
	if len(payload.HotVideos) == 0 || payload.HotVideos[0].Title != "团队运营版要怎么落地" {
		t.Fatalf("expected comment-only materialization to synthesize a live hot video instead of prototype filler, got %+v", payload.HotVideos)
	}
	if payload.HotVideos[0].ObjectID != "oid-comment-1" || payload.HotVideos[0].ObjectNonceID != "nid-comment-1" {
		t.Fatalf("expected hot video to preserve comment export identifiers, got %+v", payload.HotVideos[0])
	}
	if len(payload.HotTopics) == 0 || payload.HotTopics[0].Keyword != "团队运营版" {
		t.Fatalf("expected comment-only materialization to synthesize a live hot topic instead of prototype filler, got %+v", payload.HotTopics)
	}
	for _, item := range payload.HotVideos {
		if item.ID == "vid-ai-001" {
			t.Fatalf("expected comment-only materialization to exclude prototype filler videos, got %+v", payload.HotVideos)
		}
	}

	persisted, err := dashboardRepo.GetDashboard(context.Background(), "ws-default-hub-user-alice")
	if err != nil {
		t.Fatalf("load persisted dashboard failed: %v", err)
	}
	if persisted.ReadModel.Mode != "materialized" {
		t.Fatalf("expected persisted materialized mode, got %+v", persisted.ReadModel)
	}
	if len(persisted.CommentInsights) == 0 || persisted.CommentInsights[0].Theme != "团队运营版" {
		t.Fatalf("expected persisted ingest comment insight, got %+v", persisted.CommentInsights)
	}
	if len(payload.ReadModel.ViewStatuses) != 4 {
		t.Fatalf("expected materialized payload to include view statuses, got %+v", payload.ReadModel.ViewStatuses)
	}
}

func TestMaterializerBuildWithNormalizedPrefersNormalizedSignals(t *testing.T) {
	dashboardRepo := insightdemo.NewDashboardRepository()
	seed, err := dashboardRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("load seed failed: %v", err)
	}

	workspaceContext := workspace.ResolvedContext{
		Workspace: workspace.DefaultWorkspace{
			ID:            "ws-default-hub-user-alice",
			OwnerUserID:   "hub:user:alice",
			Name:          "Alice Workspace",
			DeviceCount:   2,
			BootstrapMode: "created",
			Source:        "workspace",
			State:         "ready",
		},
		ContextSource: "jobs",
		Resolution:    "found",
	}
	summary := ingest.Summary{
		HubUserID:          "hub:user:alice",
		State:              "ready",
		FreshnessState:     "fresh",
		TotalEvents:        9,
		SourceCount:        2,
		LatestEventAt:      time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
		LastSuccessAt:      time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
		LatestBatchID:      "batch-normalized-1",
		LastEventType:      "comment_snapshot",
		AggregatedVideos:   60,
		AggregatedComments: 240,
		AggregatedTopics:   12,
		RecentEvents: []ingest.Event{{
			ID:                    "raw-comment-1",
			BatchID:               "raw-batch-1",
			EventType:             "comment_snapshot",
			Source:                "wx_channel_comment_export",
			CollectedAt:           time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			CommentCount:          9,
			Keyword:               "raw-theme",
			RepresentativeComment: "raw representative",
			TargetAudience:        "raw audience",
		}},
	}
	snapshot := normalize.Snapshot{
		Summary: normalize.Summary{
			WorkspaceID:   workspaceContext.Workspace.ID,
			HubUserID:     workspaceContext.Workspace.OwnerUserID,
			State:         "ready",
			LatestBatchID: "batch-normalized-1",
			GeneratedAt:   time.Date(2026, 7, 5, 9, 31, 0, 0, time.FixedZone("CST", 8*3600)),
			LatestEventAt: time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			ContentCount:  1,
			CommentCount:  1,
			TopicCount:    1,
			AccountCount:  1,
		},
		ContentSnapshots: []normalize.ContentSnapshot{{
			ID:             "normalized-content-1",
			BatchID:        "batch-normalized-1",
			EventID:        "video-1",
			Source:         "wx_channel_feed_list",
			CollectedAt:    time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			Title:          "normalized hot video",
			ChannelName:    "Normalized Channel",
			Keyword:        "normalized topic",
			Signal:         "高评论密度",
			PromptAngle:    "从 normalized 追问里切题",
			TargetAudience: "运营负责人",
			VideoCount:     1,
			CommentCount:   132,
			TopicCount:     2,
			Tags:           []string{"normalized topic", "热点"},
			Keywords:       []string{"normalized topic", "回答型内容"},
		}},
		CommentSnapshots: []normalize.CommentSnapshot{{
			ID:                    "normalized-comment-1",
			BatchID:               "batch-normalized-1",
			EventID:               "comment-1",
			Source:                "wx_channel_comment_export",
			CollectedAt:           time.Date(2026, 7, 5, 9, 29, 0, 0, time.FixedZone("CST", 8*3600)),
			Title:                 "normalized hot video",
			ChannelName:           "Normalized Channel",
			Keyword:               "normalized theme",
			Signal:                "追问密集",
			RepresentativeComment: "normalized representative",
			TargetAudience:        "运营负责人",
			VideoCount:            1,
			CommentCount:          88,
			TopicCount:            1,
			Keywords:              []string{"normalized theme", "评论追问"},
		}},
		TopicObservations: []normalize.TopicObservation{{
			ID:             "normalized-topic-1",
			BatchID:        "batch-normalized-1",
			EventID:        "topic-1",
			Source:         "wx_channel_feed_list",
			CollectedAt:    time.Date(2026, 7, 5, 9, 28, 0, 0, time.FixedZone("CST", 8*3600)),
			Keyword:        "normalized topic",
			Cluster:        "normalized cluster",
			Signal:         "持续升温",
			PromptAngle:    "围绕 normalized topic 拆选题",
			TargetAudience: "运营负责人",
			VideoCount:     1,
			CommentCount:   44,
			TopicCount:     1,
			Keywords:       []string{"normalized topic", "追问"},
		}},
	}

	payload := insight.NewMaterializer(seed).BuildWithNormalized(summary, workspaceContext, snapshot, true, "jobs")
	if payload.HotVideos[0].Title != "normalized hot video" {
		t.Fatalf("expected normalized content to drive hot videos, got %+v", payload.HotVideos[0])
	}
	if payload.HotTopics[0].Keyword != "normalized topic" {
		t.Fatalf("expected normalized topic to drive hot topics, got %+v", payload.HotTopics[0])
	}
	if payload.CommentInsights[0].Theme != "normalized theme" {
		t.Fatalf("expected normalized comment to drive comment insights, got %+v", payload.CommentInsights[0])
	}
	if payload.ReportSummary.Headline == "" || payload.ReportSummary.Headline == "raw-theme" {
		t.Fatalf("expected normalized snapshot to drive report summary, got %+v", payload.ReportSummary)
	}
	if payload.IngestSummary.LatestBatchID != "batch-normalized-1" {
		t.Fatalf("expected ingest summary metadata preserved, got %+v", payload.IngestSummary)
	}
}

func TestMaterializerBuildWithAggregatePrefersAggregateSignals(t *testing.T) {
	dashboardRepo := insightdemo.NewDashboardRepository()
	seed, err := dashboardRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("load seed failed: %v", err)
	}

	workspaceContext := workspace.ResolvedContext{
		Workspace: workspace.DefaultWorkspace{
			ID:            "ws-default-hub-user-alice",
			OwnerUserID:   "hub:user:alice",
			Name:          "Alice Workspace",
			DeviceCount:   2,
			BootstrapMode: "created",
			Source:        "workspace",
			State:         "ready",
		},
		ContextSource: "jobs",
		Resolution:    "found",
	}
	summary := ingest.Summary{
		HubUserID:          "hub:user:alice",
		State:              "ready",
		FreshnessState:     "fresh",
		TotalEvents:        3,
		SourceCount:        2,
		LatestEventAt:      time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
		LastSuccessAt:      time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
		LatestBatchID:      "batch-aggregate-1",
		AggregatedVideos:   1,
		AggregatedComments: 2,
		AggregatedTopics:   1,
	}
	aggregateData := aggregate.Aggregate{
		Summary: aggregate.Summary{
			WorkspaceID:        workspaceContext.Workspace.ID,
			HubUserID:          workspaceContext.Workspace.OwnerUserID,
			State:              "ready",
			LatestBatchID:      "batch-aggregate-1",
			GeneratedAt:        time.Date(2026, 7, 5, 9, 31, 0, 0, time.FixedZone("CST", 8*3600)),
			LatestObservedAt:   time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			VideoMetricCount:   1,
			TopicMetricCount:   1,
			CommentFactCount:   1,
			SourceAccountCount: 2,
		},
		VideoMetrics: []aggregate.VideoMetric{{
			ID:              "agg-video-1",
			EventID:         "video-1",
			BatchID:         "batch-aggregate-1",
			Source:          "wx_channel_feed_list",
			ObservedAt:      time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			ObjectID:        "oid-aggregate-1",
			ObjectNonceID:   "nid-aggregate-1",
			Title:           "aggregate hot video",
			ChannelName:     "Aggregate Channel",
			Signal:          "高评论密度",
			Tags:            []string{"aggregate topic"},
			Views:           24000,
			Likes:           1800,
			Comments:        132,
			GrowthRate:      66.5,
			EngagementScore: 91,
		}},
		TopicMetrics: []aggregate.TopicMetric{{
			ID:             "agg-topic-1",
			EventID:        "topic-1",
			BatchID:        "batch-aggregate-1",
			Source:         "wx_channel_feed_list",
			ObservedAt:     time.Date(2026, 7, 5, 9, 29, 0, 0, time.FixedZone("CST", 8*3600)),
			Keyword:        "aggregate topic",
			Cluster:        "aggregate cluster",
			Signal:         "持续升温",
			PromptAngle:    "围绕 aggregate topic 拆选题",
			TargetAudience: "运营负责人",
			Keywords:       []string{"aggregate topic", "回答型内容"},
			Mentions:       88,
			GrowthRate:     57.2,
			RelatedVideos:  3,
		}},
		CommentInsightFacts: []aggregate.CommentInsightFact{{
			ID:               "agg-comment-1",
			EventID:          "comment-1",
			BatchID:          "batch-aggregate-1",
			Source:           "wx_channel_comment_export",
			ObservedAt:       time.Date(2026, 7, 5, 9, 28, 0, 0, time.FixedZone("CST", 8*3600)),
			Theme:            "aggregate theme",
			Sentiment:        "追问密集",
			Representative:   "aggregate representative",
			ActionSuggestion: "aggregate answer",
			TargetAudience:   "运营负责人",
			RelatedKeywords:  []string{"aggregate theme", "评论追问"},
			QuestionRate:     73,
			CommentCount:     88,
		}},
	}

	payload := insight.NewMaterializer(seed).BuildWithAggregate(summary, workspaceContext, aggregateData, true, normalize.Snapshot{}, false, "jobs")
	if payload.HotVideos[0].Title != "aggregate hot video" {
		t.Fatalf("expected aggregate hot video first, got %+v", payload.HotVideos[0])
	}
	if payload.HotVideos[0].ObjectID != "oid-aggregate-1" || payload.HotVideos[0].ObjectNonceID != "nid-aggregate-1" {
		t.Fatalf("expected aggregate hot video to preserve comment export identifiers, got %+v", payload.HotVideos[0])
	}
	if len(payload.HotVideos) != 1 {
		t.Fatalf("expected aggregate materialization to avoid prototype filler videos, got %+v", payload.HotVideos)
	}
	if payload.HotTopics[0].Keyword != "aggregate topic" {
		t.Fatalf("expected aggregate topic first, got %+v", payload.HotTopics[0])
	}
	if len(payload.HotTopics) != 1 {
		t.Fatalf("expected aggregate materialization to avoid prototype filler topics, got %+v", payload.HotTopics)
	}
	if payload.ReportSummary.Headline == "" {
		t.Fatal("aggregate materialization must include an analytical summary")
	}
	if payload.CommentInsights[0].Theme != "aggregate theme" {
		t.Fatalf("expected aggregate comment insight first, got %+v", payload.CommentInsights[0])
	}
	if len(payload.CommentInsights) != 1 {
		t.Fatalf("expected aggregate materialization to avoid prototype filler comment insights, got %+v", payload.CommentInsights)
	}
	if payload.Overview.MonitoredAccounts != 2 || payload.Overview.TrackedVideos != 1 || payload.Overview.EmergingTopics != 1 {
		t.Fatalf("expected overview metrics to come from aggregate/live counts, got %+v", payload.Overview)
	}
	if payload.Overview.DailyCoverageRate != 100 {
		t.Fatalf("expected aggregate overview coverage to come from live source/device coverage, got %+v", payload.Overview)
	}
	for _, item := range payload.HotVideos {
		if item.ID == "vid-ai-001" {
			t.Fatalf("expected aggregate materialization to exclude prototype hot video filler, got %+v", payload.HotVideos)
		}
	}
}

func TestMaterializerBuildWithAggregateSeparatesMaterializedAtFromObservedAt(t *testing.T) {
	dashboardRepo := insightdemo.NewDashboardRepository()
	seed, err := dashboardRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("load seed failed: %v", err)
	}

	workspaceContext := workspace.ResolvedContext{
		Workspace: workspace.DefaultWorkspace{
			ID:            "ws-default-hub-user-alice",
			OwnerUserID:   "hub:user:alice",
			Name:          "默认运营工作区",
			DeviceCount:   2,
			BootstrapMode: "lazy_create",
			Source:        "insight_default_workspace",
			State:         "ready",
		},
		Resolution: "reused",
	}
	observedAt := time.Date(2026, 7, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*3600))
	materializedAt := observedAt.Add(3 * time.Hour)
	summary := ingest.Summary{
		State:          "ready",
		FreshnessState: "stale",
		LatestBatchID:  "batch-aggregate-2",
		LatestEventAt:  observedAt,
	}
	aggregateData := aggregate.Aggregate{
		Summary: aggregate.Summary{
			WorkspaceID:      workspaceContext.Workspace.ID,
			HubUserID:        workspaceContext.Workspace.OwnerUserID,
			State:            "ready",
			LatestBatchID:    "batch-aggregate-2",
			GeneratedAt:      materializedAt.Add(-time.Minute),
			LatestObservedAt: observedAt,
		},
		VideoMetrics: []aggregate.VideoMetric{{
			ID:              "agg-video-2",
			EventID:         "video-2",
			BatchID:         "batch-aggregate-2",
			ObservedAt:      observedAt,
			PublishedAt:     observedAt,
			Title:           "aggregate aged video",
			ChannelName:     "Insight Aggregate",
			Comments:        6,
			EngagementScore: 88,
		}},
		CommentInsightFacts: []aggregate.CommentInsightFact{{
			ID:             "agg-comment-2",
			EventID:        "comment-2",
			BatchID:        "batch-aggregate-2",
			ObservedAt:     observedAt,
			Theme:          "aggregate aged theme",
			Representative: "代表性追问",
			CommentCount:   6,
		}},
	}

	payload := insight.NewMaterializerWithClock(seed, func() time.Time { return materializedAt }).BuildWithAggregate(summary, workspaceContext, aggregateData, true, normalize.Snapshot{}, false, "jobs")
	if !payload.GeneratedAt.Equal(materializedAt) {
		t.Fatalf("expected generatedAt to follow materialization time, got %s", payload.GeneratedAt.Format(time.RFC3339))
	}
	if !payload.ReadModel.MaterializedAt.Equal(materializedAt) {
		t.Fatalf("expected read model materializedAt to follow materialization time, got %s", payload.ReadModel.MaterializedAt.Format(time.RFC3339))
	}
	if payload.Overview.ReportFreshness != "3 小时前" {
		t.Fatalf("expected report freshness to reflect observed data age, got %q", payload.Overview.ReportFreshness)
	}
	if payload.HotVideos[0].PublishedAgo != "3 小时前" {
		t.Fatalf("expected aggregate hot video age to reflect observed data age, got %q", payload.HotVideos[0].PublishedAgo)
	}
	if payload.WatchlistAlerts[0].HappenedAt != "3 小时前" {
		t.Fatalf("expected aggregate watchlist age to reflect observed data age, got %q", payload.WatchlistAlerts[0].HappenedAt)
	}
}
