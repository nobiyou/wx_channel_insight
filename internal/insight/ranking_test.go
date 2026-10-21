package insight_test

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
	"github.com/nobiyou/wx_channel_insight/internal/rediscache"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspacedemo"
)

func TestReadModelServiceLoadsHotVideosAndTopicsFromDedicatedRankingRepository(t *testing.T) {
	dashboardRepo := insightdemo.NewDashboardRepository()
	rankingRepo := insightdemo.NewRankingRepository(
		[]insight.HotVideo{{ID: "seed-video", Title: "seed", ChannelName: "seed"}},
		[]insight.HotTopic{{ID: "seed-topic", Keyword: "seed", Cluster: "seed"}},
	)
	contextResolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	service := insight.NewDashboardService(dashboardRepo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: contextResolver,
		RankingRepository:        rankingRepo,
	})
	if err := rankingRepo.ReplaceHotVideos(context.Background(), "ws-default-hub-user-alice", []insight.HotVideo{{
		ID:          "video-1",
		Title:       "独立热视频榜",
		ChannelName: "Insight Test",
		Tags:        []string{"独立读模型"},
	}}); err != nil {
		t.Fatalf("replace hot videos failed: %v", err)
	}
	if err := rankingRepo.ReplaceHotTopics(context.Background(), "ws-default-hub-user-alice", []insight.HotTopic{{
		ID:      "topic-1",
		Keyword: "#Codex#AI#人工智能#科技#热门#热点",
		Cluster: "Insight Test",
	}}); err != nil {
		t.Fatalf("replace hot topics failed: %v", err)
	}

	readService := insight.NewReadModelService(
		service,
		insight.WithRankingRepository(rankingRepo),
		insight.WithWorkspaceContextResolver(contextResolver),
	)

	videos, err := readService.GetHotVideos(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	})
	if err != nil {
		t.Fatalf("get hot videos failed: %v", err)
	}
	if len(videos) == 0 || videos[0].Title != "独立热视频榜" {
		t.Fatalf("expected dedicated hot videos, got %+v", videos)
	}
	if err := rankingRepo.ReplaceHotVideoRankings(context.Background(), "ws-default-hub-user-alice", insight.HotVideoRankings{
		insight.HotVideoRankingHot:   {{ID: "video-1", Title: "独立热视频榜"}},
		insight.HotVideoRankingLikes: {{ID: "video-likes", Title: "独立点赞榜"}},
	}); err != nil {
		t.Fatalf("replace hot video rankings failed: %v", err)
	}
	likedVideos, err := readService.GetHotVideosByRanking(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	}, insight.HotVideoRankingLikes)
	if err != nil {
		t.Fatalf("get likes ranking failed: %v", err)
	}
	if len(likedVideos) != 1 || likedVideos[0].Title != "独立点赞榜" {
		t.Fatalf("expected dedicated likes ranking, got %+v", likedVideos)
	}

	topics, err := readService.GetHotTopics(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	})
	if err != nil {
		t.Fatalf("get hot topics failed: %v", err)
	}
	if len(topics) == 0 || topics[0].Keyword != "Codex" {
		t.Fatalf("expected dedicated hot topics, got %+v", topics)
	}
}

func TestReadModelServiceFallsBackWhenDedicatedRankingSnapshotLacksBoard(t *testing.T) {
	dashboardRepo := staticDashboardRepo{payload: insight.Dashboard{
		HotVideoRankings: insight.HotVideoRankings{
			insight.HotVideoRankingLikes: {{ID: "dashboard-likes", Title: "Dashboard 点赞榜"}},
		},
	}}
	rankingRepo := insightdemo.NewRankingRepository(nil, nil)
	if err := rankingRepo.ReplaceHotVideoRankings(context.Background(), "ws-default-hub-user-alice", insight.HotVideoRankings{
		insight.HotVideoRankingHot: {{ID: "legacy-hot", Title: "旧综合榜"}},
	}); err != nil {
		t.Fatalf("replace legacy rankings failed: %v", err)
	}
	contextResolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	dashboardService := insight.NewDashboardService(dashboardRepo, insight.DashboardServiceConfig{
		WorkspaceContextResolver: contextResolver,
		RankingRepository:        rankingRepo,
	})
	readService := insight.NewReadModelService(
		dashboardService,
		insight.WithRankingRepository(rankingRepo),
		insight.WithWorkspaceContextResolver(contextResolver),
	)

	items, err := readService.GetHotVideosByRanking(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	}, insight.HotVideoRankingLikes)
	if err != nil {
		t.Fatalf("get likes ranking failed: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected dashboard fallback for missing likes board, got %+v", items)
	}
}

func TestReadModelServicePrefersRedisHotRankingsWhenAvailable(t *testing.T) {
	stub := &stubKV{}
	client, err := rediscache.NewClientWithKV("127.0.0.1:6379", "", 0, stub)
	if err != nil {
		t.Fatalf("build redis stub client failed: %v", err)
	}
	cache := insight.NewReadCache(client, time.Minute)
	rankingRepo := insightdemo.NewRankingRepository(
		[]insight.HotVideo{{ID: "database-video-1", Title: "数据库热视频榜"}},
		[]insight.HotTopic{{ID: "database-topic-1", Keyword: "数据库话题"}},
	)
	contextResolver := workspace.NewContextResolver(
		workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository()),
		workspace.ContextResolverConfig{
			DefaultHubUserID:   "hub:user:demo",
			DefaultDeviceCount: 2,
		},
	)
	if err := cache.SaveHotVideos(context.Background(), "ws-default-hub-user-alice", []insight.HotVideo{{
		ID:          "cached-video-1",
		Title:       "缓存热视频榜",
		ChannelName: "Redis Cache",
	}}); err != nil {
		t.Fatalf("seed redis hot videos failed: %v", err)
	}
	if err := cache.SaveHotTopics(context.Background(), "ws-default-hub-user-alice", []insight.HotTopic{{
		ID:      "cached-topic-1",
		Keyword: "缓存话题榜",
		Cluster: "Redis Cache",
	}}); err != nil {
		t.Fatalf("seed redis hot topics failed: %v", err)
	}

	service := insight.NewDashboardService(insightdemo.NewDashboardRepository(), insight.DashboardServiceConfig{
		WorkspaceContextResolver: contextResolver,
		ReadCache:                cache,
		RankingRepository:        rankingRepo,
	})
	readService := insight.NewReadModelService(
		service,
		insight.WithWorkspaceContextResolver(contextResolver),
		insight.WithRankingRepository(rankingRepo),
		insight.WithReadCache(cache),
	)

	videos, err := readService.GetHotVideos(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	})
	if err != nil {
		t.Fatalf("get hot videos failed: %v", err)
	}
	if len(videos) == 0 || videos[0].Title != "缓存热视频榜" {
		t.Fatalf("expected redis cached hot videos, got %+v", videos)
	}

	topics, err := readService.GetHotTopics(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	})
	if err != nil {
		t.Fatalf("get hot topics failed: %v", err)
	}
	if len(topics) == 0 || topics[0].Keyword != "缓存话题榜" {
		t.Fatalf("expected redis cached hot topics, got %+v", topics)
	}
}

func TestRefreshDashboardAlsoPersistsHotRankings(t *testing.T) {
	dashboardRepo := insightdemo.NewDashboardRepository()
	seed, err := dashboardRepo.GetDashboard(context.Background(), "")
	if err != nil {
		t.Fatalf("load seed failed: %v", err)
	}
	rankingRepo := insightdemo.NewRankingRepository(seed.HotVideos, seed.HotTopics)
	workspaceService := workspace.NewDefaultWorkspaceService(workspacedemo.NewDefaultWorkspaceRepository())
	contextResolver := workspace.NewContextResolver(workspaceService, workspace.ContextResolverConfig{
		DefaultHubUserID:   "hub:user:demo",
		DefaultDeviceCount: 2,
	})
	ingestRepo := ingestdemo.NewRepository()
	now := time.Date(2026, 7, 3, 10, 14, 0, 0, time.FixedZone("CST", 8*3600))
	ingestService := ingest.NewService(ingestRepo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now:              func() time.Time { return now },
	})
	_, err = ingestService.ImportEvents(context.Background(), ingest.SummaryQuery{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}, []ingest.Event{{
		ID:           "video-1",
		HubUserID:    "hub:user:alice",
		BatchID:      "batch-1",
		MachineID:    "machine-1",
		EventType:    "video_snapshot",
		Source:       "wx_feed_list",
		CollectedAt:  now,
		VideoCount:   1,
		CommentCount: 12,
		TopicCount:   1,
		Title:        "refresh hot ranking 标题",
		ChannelName:  "Insight Test",
		Keyword:      "团队运营版",
		Signal:       "高增速",
		Tags:         []string{"团队运营版"},
	}})
	if err != nil {
		t.Fatalf("import events failed: %v", err)
	}

	refreshService := insight.NewReadModelRefreshService(insight.ReadModelRefreshServiceConfig{
		Repository:               dashboardRepo,
		RankingRepository:        rankingRepo,
		WorkspaceContextResolver: contextResolver,
		IngestService:            ingestService,
		Materializer:             insight.NewMaterializer(seed),
	})

	payload, err := refreshService.RefreshDashboard(context.Background(), insight.DashboardQuery{
		HubUserID:     "hub:user:alice",
		DeviceCount:   2,
		ContextSource: "jobs",
	}, "jobs")
	if err != nil {
		t.Fatalf("refresh dashboard failed: %v", err)
	}

	videos, found, err := rankingRepo.GetHotVideos(context.Background(), "ws-default-hub-user-alice")
	if err != nil {
		t.Fatalf("get persisted hot videos failed: %v", err)
	}
	if !found {
		t.Fatalf("expected hot videos to be persisted")
	}
	if len(videos) == 0 || videos[0].Title != payload.HotVideos[0].Title {
		t.Fatalf("expected persisted hot videos to match payload, got videos=%+v payload=%+v", videos, payload.HotVideos)
	}

	topics, found, err := rankingRepo.GetHotTopics(context.Background(), "ws-default-hub-user-alice")
	if err != nil {
		t.Fatalf("get persisted hot topics failed: %v", err)
	}
	if !found {
		t.Fatalf("expected hot topics to be persisted")
	}
	if len(topics) == 0 || topics[0].Keyword != payload.HotTopics[0].Keyword {
		t.Fatalf("expected persisted hot topics to match payload, got topics=%+v payload=%+v", topics, payload.HotTopics)
	}
}
