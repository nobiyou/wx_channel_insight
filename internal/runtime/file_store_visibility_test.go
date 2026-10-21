package runtime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestBuildDashboardRuntimeFileStoresSeeCrossProcessWrites(t *testing.T) {
	root := t.TempDir()
	cfg := Config{
		DefaultHubUserID:    "hub:user:demo",
		DefaultDeviceCount:  2,
		DashboardStore:      StoreConfig{Mode: StoreModeFile, Path: filepath.Join(root, "dashboard.json")},
		RankingStore:        StoreConfig{Mode: StoreModeFile, Path: filepath.Join(root, "rankings.json")},
		CommentInsightStore: StoreConfig{Mode: StoreModeFile, Path: filepath.Join(root, "comment-insights.json")},
		ReportSummaryStore:  StoreConfig{Mode: StoreModeFile, Path: filepath.Join(root, "report-summary.json")},
		WorkspaceStore:      StoreConfig{Mode: StoreModeFile, Path: filepath.Join(root, "workspace.json")},
		IngestStore:         StoreConfig{Mode: StoreModeFile, Path: filepath.Join(root, "ingest.json")},
	}

	apiRuntime, err := BuildDashboardRuntime(cfg)
	if err != nil {
		t.Fatalf("build api runtime failed: %v", err)
	}
	defer apiRuntime.Close()

	jobsRuntime, err := BuildDashboardRuntime(cfg)
	if err != nil {
		t.Fatalf("build jobs runtime failed: %v", err)
	}
	defer jobsRuntime.Close()

	ctx := context.Background()
	hubUserID := "hub:user:alice"
	deviceCount := 2
	collectedAt := time.Date(2026, 7, 3, 11, 0, 0, 0, time.FixedZone("CST", 8*3600))
	batchID := "batch-live-proof"

	if _, err := jobsRuntime.IngestService.ImportEvents(ctx, ingest.SummaryQuery{
		HubUserID:   hubUserID,
		DeviceCount: deviceCount,
	}, []ingest.Event{
		{
			ID:              "event-video-1",
			BatchID:         batchID,
			HubUserID:       hubUserID,
			MachineID:       "wx-channel-import-02",
			EventType:       "video_snapshot",
			Source:          ingest.RunSourceWXFeedList,
			CollectedAt:     collectedAt,
			VideoCount:      1,
			LikeCount:       12,
			CommentCount:    1,
			FavCount:        4,
			ForwardCount:    2,
			TopicCount:      2,
			Title:           "runtime live proof",
			ChannelUsername: "runtime-proof",
			ChannelName:     "Insight Test",
			Keyword:         "runtime",
			Tags:            []string{"runtime", "proof"},
			Signal:          "评论区有追问",
		},
		{
			ID:                    "event-comment-1",
			BatchID:               batchID,
			HubUserID:             hubUserID,
			MachineID:             "wx-channel-import-02",
			EventType:             "comment_snapshot",
			Source:                ingest.RunSourceWXCommentExport,
			CollectedAt:           collectedAt.Add(-time.Minute),
			CommentCount:          1,
			TopicCount:            2,
			Keyword:               "runtime live proof",
			RepresentativeComment: "先看 jobs 和 api 能不能对齐",
			TargetAudience:        "视频号内容运营",
			Keywords:              []string{"runtime", "proof"},
		},
		{
			ID:          "event-topic-1",
			BatchID:     batchID,
			HubUserID:   hubUserID,
			MachineID:   "wx-channel-import-02",
			EventType:   "topic_cluster",
			Source:      ingest.RunSourceWXCommentExport,
			CollectedAt: collectedAt.Add(-2 * time.Minute),
			TopicCount:  2,
			Keyword:     "runtime live proof",
			Cluster:     "团队运营版验证",
			PromptAngle: "先验证长跑 API 是否能看到 jobs 最新状态。",
			Keywords:    []string{"runtime", "proof"},
		},
	}); err != nil {
		t.Fatalf("import events failed: %v", err)
	}

	readModel, err := jobsRuntime.ReadModelService.RefreshDashboard(ctx, insight.DashboardQuery{
		HubUserID:     hubUserID,
		DeviceCount:   deviceCount,
		ContextSource: "jobs",
	}, "jobs")
	if err != nil {
		t.Fatalf("refresh dashboard failed: %v", err)
	}
	if err := jobsRuntime.IngestService.AppendOperatorRun(ctx, ingest.SummaryQuery{
		HubUserID:   hubUserID,
		DeviceCount: deviceCount,
	}, ingest.OperatorRun{
		ID:               "operator-live-proof-1",
		Status:           "ok",
		StartedAt:        collectedAt,
		FinishedAt:       collectedAt,
		WorkspaceID:      readModel.WorkspaceBridge.WorkspaceID,
		LatestBatchID:    batchID,
		AttemptedSources: 1,
		SucceededSources: 1,
		FreshnessState:   "fresh",
		ReportHeadline:   readModel.ReportSummary.Headline,
		TopVideoTitle:    readModel.HotVideos[0].Title,
		TopTopicKeyword:  readModel.HotTopics[0].Keyword,
	}); err != nil {
		t.Fatalf("append operator run failed: %v", err)
	}

	runs, err := apiRuntime.IngestService.ListOperatorRuns(ctx, ingest.SummaryQuery{
		HubUserID:   hubUserID,
		DeviceCount: deviceCount,
	})
	if err != nil {
		t.Fatalf("api runtime list operator runs failed: %v", err)
	}
	if len(runs) == 0 || runs[0].LatestBatchID != batchID {
		t.Fatalf("expected api runtime to see latest operator run, got %+v", runs)
	}

	payload, err := apiRuntime.DashboardService.GetDashboard(ctx, insight.DashboardQuery{
		HubUserID:     hubUserID,
		DeviceCount:   deviceCount,
		ContextSource: "test",
	})
	if err != nil {
		t.Fatalf("api runtime get dashboard failed: %v", err)
	}
	if payload.ReadModel.LatestBatchID != batchID {
		t.Fatalf("expected latest batch id %q, got %q", batchID, payload.ReadModel.LatestBatchID)
	}
	if payload.ReadModel.Mode != "materialized" {
		t.Fatalf("expected cross-runtime dashboard to be materialized, got %+v", payload.ReadModel)
	}
	if payload.ReadModel.Source != "auto_refresh" && payload.ReadModel.Source != "jobs" {
		t.Fatalf("expected materialized source to be auto_refresh or jobs, got %+v", payload.ReadModel)
	}
	if payload.IngestSummary.TotalEvents != 3 {
		t.Fatalf("expected total events 3, got %d", payload.IngestSummary.TotalEvents)
	}
	if len(payload.HotVideos) == 0 || payload.HotVideos[0].Title != "runtime live proof" {
		t.Fatalf("expected live dashboard hot video, got %+v", payload.HotVideos)
	}
}
