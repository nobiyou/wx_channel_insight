package insightapi_test

import (
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
)

func TestNewDashboardResponseMapsTransportContract(t *testing.T) {
	payload := insight.Dashboard{
		GeneratedAt: time.Date(2026, 6, 30, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
		ReadModel: insight.ReadModelState{
			Mode:           "materialized",
			MaterializedAt: time.Date(2026, 6, 30, 10, 33, 0, 0, time.FixedZone("CST", 8*3600)),
			Source:         "jobs",
			LatestBatchID:  "mock-batch-001",
			ViewStatuses: []insight.ReadModelViewStatus{{
				Key:            "videos",
				Label:          "Hot Videos",
				FreshnessState: "fresh",
				Summary:        "当前热视频榜已经跟到最新物化批次，可直接支撑今天的样本判断。",
				SourceNote:     "wx_feed_list fresh",
				LatestBatchID:  "mock-batch-001",
				MaterializedAt: time.Date(2026, 6, 30, 10, 33, 0, 0, time.FixedZone("CST", 8*3600)),
			}},
		},
		Workspace: insight.WorkspaceSummary{
			Name:        "默认运营工作区",
			Owner:       "hub:user:demo",
			DeviceCount: 6,
		},
		WorkspaceBridge: insight.WorkspaceBridge{
			WorkspaceID:   "ws-default-hub-user-demo",
			HubUserID:     "hub:user:demo",
			ContextSource: "header",
			BootstrapMode: "lazy_create",
			Source:        "insight_default_workspace",
			State:         "ready",
			Resolution:    "created",
			LastSyncedAt:  time.Date(2026, 6, 30, 10, 35, 0, 0, time.FixedZone("CST", 8*3600)),
		},
		IngestSummary: insight.IngestSummary{
			State:              "ready",
			FreshnessState:     "fresh",
			TotalEvents:        3,
			SourceCount:        2,
			LatestEventAt:      time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
			LastSuccessAt:      time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
			LastFailureAt:      time.Date(2026, 6, 30, 9, 55, 0, 0, time.FixedZone("CST", 8*3600)),
			LastFailureMessage: "page client disconnected",
			LatestBatchID:      "mock-batch-001",
			LastEventType:      "topic_cluster",
			AggregatedVideos:   16,
			AggregatedComments: 174,
			AggregatedTopics:   8,
			RecentEvents: []insight.IngestEvent{{
				ID:           "evt-1",
				BatchID:      "mock-batch-001",
				MachineID:    "machine-01",
				EventType:    "topic_cluster",
				Source:       "insight_topic_cluster",
				CollectedAt:  time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
				VideoCount:   0,
				CommentCount: 0,
				TopicCount:   5,
			}},
			SourceStatuses: []insight.IngestSourceStatus{{
				Source:         "wx_feed_list",
				State:          "ready",
				FreshnessState: "fresh",
				LastRunAt:      time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
				LastSuccessAt:  time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
				LastFailureAt:  time.Date(2026, 6, 30, 9, 55, 0, 0, time.FixedZone("CST", 8*3600)),
				LastError:      "page client disconnected",
				LatestBatchID:  "mock-batch-001",
				LastEventType:  "topic_cluster",
				EventCount:     3,
			}},
			RecentRuns: []insight.IngestRun{{
				ID:            "wx_feed_list-20260630T023200.000000000Z",
				Source:        "wx_feed_list",
				Status:        "success",
				StartedAt:     time.Date(2026, 6, 30, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
				FinishedAt:    time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
				BatchID:       "mock-batch-001",
				EventCount:    3,
				LatestEventAt: time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
				LastEventType: "topic_cluster",
			}},
			BindingStates: []insight.IngestBindingState{{
				Source:            "wx_feed_list",
				BaseURL:           "http://127.0.0.1:2026",
				Username:          "gh_demo",
				DeviceCount:       6,
				UpdatedAt:         time.Date(2026, 6, 30, 10, 29, 0, 0, time.FixedZone("CST", 8*3600)),
				LastRunStatus:     "success",
				LastRunAt:         time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
				LastSuccessAt:     time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
				LatestBatchID:     "mock-batch-001",
				LastAutoRunStatus: "success",
				LastAutoRunAt:     time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
				LastAutoSuccessAt: time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
				LastAutoBatchID:   "mock-batch-001",
				Runnable:          true,
				Diagnostic: &insight.BindingDiagnostic{
					Code:                 "wx_feed_list_health_unreachable",
					StatusURL:            "http://127.0.0.1:2026/api/channels/status",
					WSHealthURL:          "http://127.0.0.1:2026/ws/health",
					WXChannelStatusError: "timeout after 1s",
				},
			}},
			OperatorReadiness: insight.OperatorReadiness{
				State:           "partial",
				ExpectedStatus:  "partial",
				RunnableSources: 1,
				MissingSources:  1,
				Headline:        "今天这轮 operator 只能部分完成",
				Summary:         "视频列表可以直接跑，但评论导出还没保存。",
				NextAction:      "先补评论导出来源。",
				Runnable: []insight.OperatorReadinessSource{{
					Source:            "wx_feed_list",
					Label:             "视频列表",
					State:             "runnable",
					Summary:           "视频列表参数已保存。",
					Action:            "直接跑今天这轮。",
					BaseURL:           "http://127.0.0.1:2026",
					Username:          "gh_demo",
					LastAutoRunStatus: "success",
					LastAutoRunAt:     time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
					LastAutoSuccessAt: time.Date(2026, 6, 30, 10, 32, 0, 0, time.FixedZone("CST", 8*3600)),
					LastAutoBatchID:   "mock-batch-001",
					Runnable:          true,
					Diagnostic: &insight.BindingDiagnostic{
						Code:                 "wx_feed_list_health_unreachable",
						StatusURL:            "http://127.0.0.1:2026/api/channels/status",
						WSHealthURL:          "http://127.0.0.1:2026/ws/health",
						WXChannelStatusError: "timeout after 1s",
					},
				}},
				Missing: []insight.OperatorReadinessSource{{
					Source:  "wx_comment_export",
					Label:   "评论导出",
					State:   "missing",
					Summary: "还没有保存评论导出来源。",
					Action:  "先导入评论导出。",
				}},
			},
		},
		Overview: insight.OverviewMetrics{
			MonitoredAccounts: 18,
			TrackedVideos:     326,
			EmergingTopics:    12,
			NewCommentBursts:  9,
			DailyCoverageRate: 93.4,
			ReportFreshness:   "8 分钟前",
		},
		HotVideos: []insight.HotVideo{{
			ID:              "vid-1",
			Title:           "demo",
			ChannelName:     "channel",
			Source:          "wx_channel_feed_list",
			ObjectID:        "oid-demo-1",
			ObjectNonceID:   "nid-demo-1",
			CoverURL:        "https://cdn.example.test/demo-cover.jpg",
			Tags:            []string{"a"},
			PublishedAt:     time.Date(2026, 6, 30, 8, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			PublishedAgo:    "2 小时前",
			Views:           10,
			Likes:           11,
			Comments:        12,
			GrowthRate:      13.4,
			EngagementScore: 14,
			Signal:          "high",
		}},
		HotVideoWindowRankings: insight.HotVideoWindowRankings{
			insight.HotVideoWindow24h: {
				insight.HotVideoRankingLikes: {{ID: "vid-24h-likes", Title: "24h 点赞榜"}},
			},
		},
		HotTopics: []insight.HotTopic{{
			ID:         "topic-1",
			Keyword:    "world-model",
			Cluster:    "AI",
			Source:     "wx_channel_comment_export",
			ObservedAt: time.Date(2026, 6, 30, 10, 28, 0, 0, time.FixedZone("CST", 8*3600)),
			SourceAccounts: []string{
				"channel",
			},
			Mentions:      1,
			GrowthRate:    2.3,
			RelatedVideos: 4,
			PromptAngle:   "angle",
			Keywords:      []string{"k1"},
		}},
		WatchlistAlerts: []insight.WatchlistAlert{{
			ID:           "alert-1",
			AccountName:  "acct",
			AlertType:    "burst",
			Source:       "wx_channel_feed_list",
			Severity:     "high",
			Summary:      "sum",
			OccurredAt:   time.Date(2026, 6, 30, 10, 31, 0, 0, time.FixedZone("CST", 8*3600)),
			HappenedAt:   "today",
			Recommended:  "act",
			VideoCount:   1,
			CommentDelta: 2,
		}},
		CommentInsights: []insight.CommentInsight{{
			ID:               "ci-1",
			Theme:            "theme",
			Source:           "wx_channel_comment_export",
			ObservedAt:       time.Date(2026, 6, 30, 10, 27, 0, 0, time.FixedZone("CST", 8*3600)),
			SourceAccounts:   []string{"channel"},
			QuestionRate:     20,
			Sentiment:        "neutral",
			Representative:   "rep",
			ActionSuggestion: "do it",
			RelatedKeywords:  []string{"x"},
		}},
		ReportSummary: insight.ReportSummary{
			Period:            "2026-06-30",
			Headline:          "headline",
			KeyFinding:        "finding",
			RecommendedAction: "action",
			Watchout:          "watchout",
			GeneratedAt:       time.Date(2026, 6, 30, 10, 34, 0, 0, time.FixedZone("CST", 8*3600)),
			AutoGeneratedAt:   "today 10:30",
		},
	}

	result := insightapi.NewDashboardResponse(payload)

	if result.GeneratedAt != "2026-06-30T10:30:00+08:00" {
		t.Fatalf("expected RFC3339 generatedAt, got %q", result.GeneratedAt)
	}
	if result.Workspace.Owner != "hub:user:demo" {
		t.Fatalf("expected workspace owner to map, got %q", result.Workspace.Owner)
	}
	if result.WorkspaceBridge.WorkspaceID != "ws-default-hub-user-demo" {
		t.Fatalf("expected workspace bridge workspace id, got %q", result.WorkspaceBridge.WorkspaceID)
	}
	if result.WorkspaceBridge.ContextSource != "header" {
		t.Fatalf("expected workspace bridge context source, got %q", result.WorkspaceBridge.ContextSource)
	}
	if result.WorkspaceBridge.LastSyncedAt != "2026-06-30T10:35:00+08:00" {
		t.Fatalf("expected RFC3339 lastSyncedAt, got %q", result.WorkspaceBridge.LastSyncedAt)
	}
	if len(result.HotVideos) != 1 || result.HotVideos[0].ObjectID != "oid-demo-1" || result.HotVideos[0].ObjectNonceID != "nid-demo-1" {
		t.Fatalf("expected hot video object identifiers to map, got %+v", result.HotVideos)
	}
	if result.HotVideos[0].CoverURL != "https://cdn.example.test/demo-cover.jpg" {
		t.Fatalf("expected hot video cover URL to map, got %+v", result.HotVideos[0])
	}
	if result.HotVideoWindowRankings[insight.HotVideoWindow24h][insight.HotVideoRankingLikes][0].ID != "vid-24h-likes" {
		t.Fatalf("expected publication-window ranking to map, got %+v", result.HotVideoWindowRankings)
	}
	if result.IngestSummary.LatestBatchID != "mock-batch-001" {
		t.Fatalf("expected ingest batch id, got %q", result.IngestSummary.LatestBatchID)
	}
	if result.ReadModel.LatestBatchID != "mock-batch-001" {
		t.Fatalf("expected read model batch id, got %q", result.ReadModel.LatestBatchID)
	}
	if len(result.ReadModel.ViewStatuses) != 1 || result.ReadModel.ViewStatuses[0].Label != "Hot Videos" {
		t.Fatalf("expected read model view status mapping, got %+v", result.ReadModel.ViewStatuses)
	}
	if result.IngestSummary.FreshnessState != "fresh" {
		t.Fatalf("expected ingest freshness state, got %q", result.IngestSummary.FreshnessState)
	}
	if result.IngestSummary.LastFailureMessage != "page client disconnected" {
		t.Fatalf("expected ingest last failure message, got %q", result.IngestSummary.LastFailureMessage)
	}
	if len(result.IngestSummary.RecentEvents) != 1 || result.IngestSummary.RecentEvents[0].EventType != "topic_cluster" {
		t.Fatalf("expected ingest event mapping, got %+v", result.IngestSummary.RecentEvents)
	}
	if len(result.IngestSummary.SourceStatuses) != 1 || result.IngestSummary.SourceStatuses[0].Source != "wx_feed_list" {
		t.Fatalf("expected ingest source status mapping, got %+v", result.IngestSummary.SourceStatuses)
	}
	if len(result.IngestSummary.RecentRuns) != 1 || result.IngestSummary.RecentRuns[0].Status != "success" {
		t.Fatalf("expected ingest recent run mapping, got %+v", result.IngestSummary.RecentRuns)
	}
	if len(result.IngestSummary.BindingStates) != 1 || !result.IngestSummary.BindingStates[0].Runnable {
		t.Fatalf("expected ingest binding state runnable mapping, got %+v", result.IngestSummary.BindingStates)
	}
	if result.IngestSummary.BindingStates[0].LastAutoRunStatus != "success" ||
		result.IngestSummary.BindingStates[0].LastAutoBatchID != "mock-batch-001" {
		t.Fatalf("expected ingest binding auto-run mapping, got %+v", result.IngestSummary.BindingStates[0])
	}
	if result.IngestSummary.BindingStates[0].Diagnostic == nil ||
		result.IngestSummary.BindingStates[0].Diagnostic.WXChannelStatusError != "timeout after 1s" {
		t.Fatalf("expected binding diagnostic mapping, got %+v", result.IngestSummary.BindingStates[0].Diagnostic)
	}
	if result.IngestSummary.OperatorReadiness.State != "partial" || result.IngestSummary.OperatorReadiness.MissingSources != 1 {
		t.Fatalf("expected operator readiness mapping, got %+v", result.IngestSummary.OperatorReadiness)
	}
	if len(result.IngestSummary.OperatorReadiness.Runnable) != 1 ||
		result.IngestSummary.OperatorReadiness.Runnable[0].LastAutoRunStatus != "success" ||
		result.IngestSummary.OperatorReadiness.Runnable[0].LastAutoBatchID != "mock-batch-001" {
		t.Fatalf("expected operator readiness auto-run mapping, got %+v", result.IngestSummary.OperatorReadiness.Runnable)
	}
	if result.IngestSummary.OperatorReadiness.Runnable[0].Diagnostic == nil ||
		result.IngestSummary.OperatorReadiness.Runnable[0].Diagnostic.StatusURL != "http://127.0.0.1:2026/api/channels/status" {
		t.Fatalf("expected operator readiness diagnostic mapping, got %+v", result.IngestSummary.OperatorReadiness.Runnable[0].Diagnostic)
	}
	if len(result.HotVideos) != 1 ||
		result.HotVideos[0].Signal != "high" ||
		result.HotVideos[0].Source != "wx_channel_feed_list" ||
		result.HotVideos[0].PublishedAt != "2026-06-30T08:30:00+08:00" {
		t.Fatalf("expected hot video mapping, got %+v", result.HotVideos)
	}
	if len(result.HotTopics) != 1 ||
		result.HotTopics[0].Keyword != "world-model" ||
		result.HotTopics[0].Source != "wx_channel_comment_export" ||
		result.HotTopics[0].ObservedAt != "2026-06-30T10:28:00+08:00" ||
		len(result.HotTopics[0].SourceAccounts) != 1 ||
		result.HotTopics[0].SourceAccounts[0] != "channel" {
		t.Fatalf("expected hot topic mapping, got %+v", result.HotTopics)
	}
	if len(result.WatchlistAlerts) != 1 ||
		result.WatchlistAlerts[0].Severity != "high" ||
		result.WatchlistAlerts[0].Source != "wx_channel_feed_list" ||
		result.WatchlistAlerts[0].OccurredAt != "2026-06-30T10:31:00+08:00" {
		t.Fatalf("expected alert mapping, got %+v", result.WatchlistAlerts)
	}
	if len(result.CommentInsights) != 1 ||
		result.CommentInsights[0].Theme != "theme" ||
		result.CommentInsights[0].Source != "wx_channel_comment_export" ||
		result.CommentInsights[0].ObservedAt != "2026-06-30T10:27:00+08:00" ||
		len(result.CommentInsights[0].SourceAccounts) != 1 ||
		result.CommentInsights[0].SourceAccounts[0] != "channel" {
		t.Fatalf("expected comment insight mapping, got %+v", result.CommentInsights)
	}
	if result.ReportSummary.GeneratedAt != "2026-06-30T10:34:00+08:00" ||
		result.ReportSummary.AutoGeneratedAt != "today 10:30" {
		t.Fatalf("expected report summary generatedAt mapping, got %+v", result.ReportSummary)
	}
}

func TestNewDashboardPageResponseLimitsRecommendationPayload(t *testing.T) {
	sourceAccounts := make([]string, 21)
	payload := insight.Dashboard{
		AccountRecommendations: make([]insight.AccountRecommendation, 51),
		VideoRecommendations:   make([]insight.VideoRecommendation, 51),
		HotVideos:              make([]insight.HotVideo, 11),
		HotVideoRankings: insight.HotVideoRankings{
			insight.HotVideoRankingHot: make([]insight.HotVideo, 11),
		},
		HotTopics: make([]insight.HotTopic, 6),
		IngestSummary: insight.IngestSummary{
			BindingStates: make([]insight.IngestBindingState, 21),
			OperatorReadiness: insight.OperatorReadiness{
				RunnableSources: 21,
				BlockedSources:  21,
				MissingSources:  21,
				Runnable:        make([]insight.OperatorReadinessSource, 21),
				Blocked:         make([]insight.OperatorReadinessSource, 21),
				Missing:         make([]insight.OperatorReadinessSource, 21),
			},
		},
	}
	payload.HotTopics[0].SourceAccounts = sourceAccounts

	page := insightapi.NewDashboardPageResponse(payload)
	if len(page.AccountRecommendations) != 50 {
		t.Fatalf("expected dashboard account recommendations to be capped at 50, got %d", len(page.AccountRecommendations))
	}
	if len(page.VideoRecommendations) != 50 {
		t.Fatalf("expected dashboard video recommendations to be capped at 50, got %d", len(page.VideoRecommendations))
	}
	if len(page.HotVideos) != 10 || len(page.HotVideoRankings[insight.HotVideoRankingHot]) != 10 || len(page.HotTopics) != 5 {
		t.Fatalf("expected dashboard ranking payload to stay bounded, got videos=%d rankings=%d topics=%d", len(page.HotVideos), len(page.HotVideoRankings[insight.HotVideoRankingHot]), len(page.HotTopics))
	}
	if len(page.IngestSummary.BindingStates) != 20 ||
		len(page.IngestSummary.OperatorReadiness.Runnable) != 20 ||
		len(page.IngestSummary.OperatorReadiness.Blocked) != 20 ||
		len(page.IngestSummary.OperatorReadiness.Missing) != 20 {
		t.Fatalf("expected dashboard ingest detail to stay bounded, got bindings=%d runnable=%d blocked=%d missing=%d", len(page.IngestSummary.BindingStates), len(page.IngestSummary.OperatorReadiness.Runnable), len(page.IngestSummary.OperatorReadiness.Blocked), len(page.IngestSummary.OperatorReadiness.Missing))
	}
	if len(page.HotTopics[0].SourceAccounts) != 20 || page.HotTopics[0].SourceAccountCount != 21 {
		t.Fatalf("expected dashboard topic accounts to be capped without losing the total, got accounts=%d total=%d", len(page.HotTopics[0].SourceAccounts), page.HotTopics[0].SourceAccountCount)
	}
	if page.IngestSummary.OperatorReadiness.RunnableSources != 21 || page.IngestSummary.OperatorReadiness.BlockedSources != 21 || page.IngestSummary.OperatorReadiness.MissingSources != 21 {
		t.Fatalf("expected dashboard readiness totals to remain complete, got runnable=%d blocked=%d missing=%d", page.IngestSummary.OperatorReadiness.RunnableSources, page.IngestSummary.OperatorReadiness.BlockedSources, page.IngestSummary.OperatorReadiness.MissingSources)
	}

	full := insightapi.NewDashboardResponse(payload)
	if len(full.AccountRecommendations) != 51 || len(full.VideoRecommendations) != 51 || len(full.HotVideos) != 11 || len(full.HotVideoRankings[insight.HotVideoRankingHot]) != 11 || len(full.HotTopics) != 6 {
		t.Fatalf("expected full dashboard mapper to preserve all recommendations and rankings, got accounts=%d videos=%d hotVideos=%d hotRankings=%d topics=%d", len(full.AccountRecommendations), len(full.VideoRecommendations), len(full.HotVideos), len(full.HotVideoRankings[insight.HotVideoRankingHot]), len(full.HotTopics))
	}
	if len(full.IngestSummary.BindingStates) != 21 ||
		len(full.IngestSummary.OperatorReadiness.Runnable) != 21 ||
		len(full.IngestSummary.OperatorReadiness.Blocked) != 21 ||
		len(full.IngestSummary.OperatorReadiness.Missing) != 21 ||
		len(full.HotTopics[0].SourceAccounts) != 21 ||
		full.HotTopics[0].SourceAccountCount != 21 {
		t.Fatalf("expected full dashboard mapper to preserve all nested detail, got bindings=%d runnable=%d blocked=%d missing=%d topicAccounts=%d topicTotal=%d", len(full.IngestSummary.BindingStates), len(full.IngestSummary.OperatorReadiness.Runnable), len(full.IngestSummary.OperatorReadiness.Blocked), len(full.IngestSummary.OperatorReadiness.Missing), len(full.HotTopics[0].SourceAccounts), full.HotTopics[0].SourceAccountCount)
	}
}
