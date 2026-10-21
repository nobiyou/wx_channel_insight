package ingestapi_test

import (
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestapi"
)

func TestNewSummaryResponseMapsTransportContract(t *testing.T) {
	summary := ingest.Summary{
		HubUserID:          "hub:user:alice",
		State:              "ready",
		FreshnessState:     "fresh",
		TotalEvents:        5,
		SourceCount:        2,
		LatestEventAt:      time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		LastSuccessAt:      time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		LastFailureAt:      time.Date(2026, 6, 30, 18, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		LastFailureMessage: "wx_channel api timeout",
		LatestBatchID:      "mock-batch-002",
		LastEventType:      "comment_snapshot",
		AggregatedVideos:   40,
		AggregatedComments: 120,
		AggregatedTopics:   9,
		RecentEvents: []ingest.Event{{
			ID:                    "event-1",
			BatchID:               "mock-batch-002",
			MachineID:             "machine-01",
			EventType:             "comment_snapshot",
			Source:                "feed_comment_list",
			CollectedAt:           time.Date(2026, 6, 30, 19, 58, 0, 0, time.FixedZone("CST", 8*3600)),
			VideoCount:            4,
			CommentCount:          88,
			TopicCount:            1,
			Title:                 "评论区代表性问题",
			Keyword:               "世界模型怎么真正落地",
			Signal:                "高追问",
			RepresentativeComment: "别只讲路线，能不能讲讲世界模型到底能替代现在哪类工作流？",
			TargetAudience:        "泛科技内容受众",
			Keywords:              []string{"世界模型", "落地", "工作流"},
		}},
		SourceStatuses: []ingest.SourceStatus{{
			Source:             "wx_feed_list",
			State:              "ready",
			FreshnessState:     "lagging",
			SyncFreshnessState: "fresh",
			LastRunAt:          time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			LastSuccessAt:      time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			LastFailureAt:      time.Date(2026, 6, 30, 18, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			LastError:          "wx_channel api timeout",
			LatestBatchID:      "mock-batch-002",
			LastEventType:      "comment_snapshot",
			LatestEventAt:      time.Date(2026, 6, 30, 19, 40, 0, 0, time.FixedZone("CST", 8*3600)),
			EventCount:         3,
		}},
		RecentRuns: []ingest.Run{{
			ID:            "wx_feed_list-20260630T120000.000000000Z",
			Source:        "wx_feed_list",
			Status:        "success",
			StartedAt:     time.Date(2026, 6, 30, 19, 58, 0, 0, time.FixedZone("CST", 8*3600)),
			FinishedAt:    time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			BatchID:       "mock-batch-002",
			EventCount:    3,
			LatestEventAt: time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			LastEventType: "comment_snapshot",
			Message:       "",
		}},
		BindingStates: []ingest.SourceBindingState{{
			Source:            ingest.RunSourceWXFeedList,
			Origin:            ingest.BindingOriginMonitoring,
			BaseURL:           "http://127.0.0.1:2026",
			Username:          "gh_demo",
			DeviceCount:       2,
			UpdatedAt:         time.Date(2026, 6, 30, 19, 50, 0, 0, time.FixedZone("CST", 8*3600)),
			LastRunStatus:     "success",
			LastRunAt:         time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			LastSuccessAt:     time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			LatestBatchID:     "mock-batch-002",
			LastAutoRunStatus: "success",
			LastAutoRunAt:     time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			LastAutoSuccessAt: time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			LastAutoBatchID:   "mock-batch-002",
			Runnable:          true,
			Diagnostic: &ingest.SourceBindingDiagnostic{
				Code:                          "wx_feed_list_no_ready_page_client",
				ResolvedBaseURL:               "http://127.0.0.1:2026",
				StatusURL:                     "http://127.0.0.1:2026/api/channels/status",
				WSHealthURL:                   "http://127.0.0.1:2026/ws/health",
				WXChannelStatusError:          "timeout after 1s",
				WXChannelProxyMode:            "process_proxy",
				WXChannelProxyAddress:         "127.0.0.1:2025",
				WXChannelInjectionChecked:     boolPtr(true),
				WXChannelInjectionStarted:     boolPtr(false),
				WXChannelInjectionError:       "administrator permission may be required",
				WXChannelCertificateInstalled: boolPtr(false),
				WXChannelCertificateError:     "certificate store unavailable",
			},
		}},
		OperatorReadiness: ingest.OperatorReadiness{
			State:           "partial",
			ExpectedStatus:  "partial",
			RunnableSources: 1,
			BlockedSources:  0,
			MissingSources:  1,
			Headline:        "今天这轮 operator 只能部分完成",
			Summary:         "评论来源未配置，视频列表来源可直接复用。",
			NextAction:      "先补评论导出来源。",
			Runnable: []ingest.OperatorReadinessSource{{
				Source:            ingest.RunSourceWXFeedList,
				Label:             "视频列表",
				State:             "runnable",
				Summary:           "视频列表参数已保存。",
				Action:            "直接运行今天这轮。",
				BaseURL:           "http://127.0.0.1:2026",
				Username:          "gh_demo",
				LastAutoRunStatus: "success",
				LastAutoRunAt:     time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
				LastAutoSuccessAt: time.Date(2026, 6, 30, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
				LastAutoBatchID:   "mock-batch-002",
				Runnable:          true,
				Diagnostic: &ingest.SourceBindingDiagnostic{
					Code:                 "wx_feed_list_no_ready_page_client",
					ResolvedBaseURL:      "http://127.0.0.1:2026",
					StatusURL:            "http://127.0.0.1:2026/api/channels/status",
					WXChannelStatusError: "timeout after 1s",
				},
			}},
			Missing: []ingest.OperatorReadinessSource{{
				Source:   ingest.RunSourceWXCommentExport,
				Label:    "评论导出",
				State:    "missing",
				Summary:  "还没有保存评论导出来源。",
				Action:   "先导入评论导出。",
				Runnable: false,
			}},
		},
	}

	result := ingestapi.NewSummaryResponse(summary)

	if result.HubUserID != "hub:user:alice" {
		t.Fatalf("expected hub user id, got %q", result.HubUserID)
	}
	if result.FreshnessState != "fresh" {
		t.Fatalf("expected freshness state, got %q", result.FreshnessState)
	}
	if result.LatestEventAt != "2026-06-30T20:00:00+08:00" {
		t.Fatalf("expected RFC3339 latestEventAt, got %q", result.LatestEventAt)
	}
	if result.LastFailureMessage != "wx_channel api timeout" {
		t.Fatalf("expected last failure message, got %q", result.LastFailureMessage)
	}
	if len(result.RecentEvents) != 1 || result.RecentEvents[0].EventType != "comment_snapshot" {
		t.Fatalf("expected mapped recent events, got %+v", result.RecentEvents)
	}
	if result.RecentEvents[0].Keyword != "世界模型怎么真正落地" {
		t.Fatalf("expected keyword metadata, got %+v", result.RecentEvents[0])
	}
	if result.RecentEvents[0].RepresentativeComment == "" || len(result.RecentEvents[0].Keywords) != 3 {
		t.Fatalf("expected comment metadata passthrough, got %+v", result.RecentEvents[0])
	}
	if len(result.SourceStatuses) != 1 || result.SourceStatuses[0].Source != "wx_feed_list" {
		t.Fatalf("expected source statuses, got %+v", result.SourceStatuses)
	}
	if result.SourceStatuses[0].FreshnessState != "lagging" || result.SourceStatuses[0].SyncFreshnessState != "fresh" {
		t.Fatalf("expected separate source freshness fields, got %+v", result.SourceStatuses[0])
	}
	if result.SourceStatuses[0].LatestEventAt != "2026-06-30T19:40:00+08:00" {
		t.Fatalf("expected latest event timestamp passthrough, got %+v", result.SourceStatuses[0])
	}
	if len(result.RecentRuns) != 1 || result.RecentRuns[0].Status != "success" {
		t.Fatalf("expected recent runs, got %+v", result.RecentRuns)
	}
	if len(result.BindingStates) != 1 || !result.BindingStates[0].Runnable {
		t.Fatalf("expected binding states with runnable flag, got %+v", result.BindingStates)
	}
	if result.BindingStates[0].Origin != ingest.BindingOriginMonitoring {
		t.Fatalf("expected binding origin mapping, got %+v", result.BindingStates[0])
	}
	if result.BindingStates[0].LastAutoRunStatus != "success" || result.BindingStates[0].LastAutoBatchID != "mock-batch-002" {
		t.Fatalf("expected binding auto run facts, got %+v", result.BindingStates[0])
	}
	if result.BindingStates[0].Diagnostic == nil || result.BindingStates[0].Diagnostic.WSHealthURL != "http://127.0.0.1:2026/ws/health" {
		t.Fatalf("expected binding diagnostic mapping, got %+v", result.BindingStates[0])
	}
	if result.BindingStates[0].Diagnostic.WXChannelStatusError != "timeout after 1s" ||
		result.BindingStates[0].Diagnostic.WXChannelProxyMode != "process_proxy" ||
		result.BindingStates[0].Diagnostic.WXChannelInjectionStarted == nil ||
		*result.BindingStates[0].Diagnostic.WXChannelInjectionStarted {
		t.Fatalf("expected wx_channel runtime diagnostic mapping, got %+v", result.BindingStates[0].Diagnostic)
	}
	if result.OperatorReadiness.State != "partial" || result.OperatorReadiness.MissingSources != 1 {
		t.Fatalf("expected operator readiness mapping, got %+v", result.OperatorReadiness)
	}
	if len(result.OperatorReadiness.Runnable) != 1 || result.OperatorReadiness.Runnable[0].Label != "视频列表" {
		t.Fatalf("expected operator readiness runnable source mapping, got %+v", result.OperatorReadiness)
	}
	if result.OperatorReadiness.Runnable[0].LastAutoRunStatus != "success" || result.OperatorReadiness.Runnable[0].LastAutoBatchID != "mock-batch-002" {
		t.Fatalf("expected operator readiness auto run facts, got %+v", result.OperatorReadiness.Runnable[0])
	}
	if result.OperatorReadiness.Runnable[0].Diagnostic == nil || result.OperatorReadiness.Runnable[0].Diagnostic.Code != "wx_feed_list_no_ready_page_client" {
		t.Fatalf("expected operator readiness diagnostic mapping, got %+v", result.OperatorReadiness.Runnable[0])
	}
	if result.OperatorReadiness.Runnable[0].Diagnostic.WXChannelStatusError != "timeout after 1s" {
		t.Fatalf("expected operator readiness status error mapping, got %+v", result.OperatorReadiness.Runnable[0].Diagnostic)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestNewOperatorRunsResponseMapsTransportContract(t *testing.T) {
	runs := []ingest.OperatorRun{{
		ID:               "operator-cycle-20260703T120000Z",
		Status:           "failed",
		TriggerMode:      ingest.OperatorTriggerCycle,
		StartedAt:        time.Date(2026, 7, 3, 20, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		FinishedAt:       time.Date(2026, 7, 3, 20, 1, 30, 0, time.FixedZone("CST", 8*3600)),
		WorkspaceID:      "ws-default-hub-user-alice",
		LatestBatchID:    "batch-007",
		AttemptedSources: 2,
		SucceededSources: 1,
		FailedSources:    1,
		SkippedSources:   0,
		FreshnessState:   "lagging",
		ReportHeadline:   "热点延续但评论追问转向方法论",
		TopVideoTitle:    "团队日报测试视频",
		TopTopicKeyword:  "世界模型",
		Message:          "wx_comment_export timeout",
		Outcomes: []ingest.OperatorRunOutcome{{
			Source:  ingest.RunSourceWXCommentExport,
			Status:  "failed",
			Summary: "wx_comment_export timeout",
		}, {
			Source:  ingest.RunSourceWXFeedList,
			Status:  "success",
			Summary: "批次 batch-007 · 3 条事件",
		}},
	}}

	result := ingestapi.NewOperatorRunsResponse(runs)
	if len(result.Items) != 1 {
		t.Fatalf("expected one operator run, got %+v", result.Items)
	}
	if result.Items[0].ID != "operator-cycle-20260703T120000Z" {
		t.Fatalf("expected mapped id, got %+v", result.Items[0])
	}
	if result.Items[0].FinishedAt != "2026-07-03T20:01:30+08:00" {
		t.Fatalf("expected RFC3339 finishedAt, got %+v", result.Items[0])
	}
	if result.Items[0].FailedSources != 1 || result.Items[0].SucceededSources != 1 {
		t.Fatalf("expected source counters, got %+v", result.Items[0])
	}
	if result.Items[0].TriggerMode != ingest.OperatorTriggerCycle {
		t.Fatalf("expected mapped trigger mode, got %+v", result.Items[0])
	}
	if result.Items[0].TopTopicKeyword != "世界模型" || result.Items[0].Message == "" {
		t.Fatalf("expected mapped operator metadata, got %+v", result.Items[0])
	}
	if len(result.Items[0].Outcomes) != 2 || result.Items[0].Outcomes[0].Status != "failed" {
		t.Fatalf("expected mapped operator run outcomes, got %+v", result.Items[0])
	}
}
