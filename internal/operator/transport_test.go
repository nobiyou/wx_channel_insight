package operator_test

import (
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	operatorflow "github.com/nobiyou/wx_channel_insight/internal/operator"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestBuildBriefIncludesRuntimeProfileAndStoreModes(t *testing.T) {
	brief := operatorflow.BuildBrief(insight.Dashboard{
		Workspace: insight.WorkspaceSummary{
			Name:        "默认运营工作区",
			DeviceCount: 2,
		},
		WorkspaceBridge: insight.WorkspaceBridge{
			WorkspaceID:   "ws-default-hub-user-alice",
			HubUserID:     "hub:user:alice",
			ContextSource: "trusted_header",
		},
		ReadModel: insight.ReadModelState{
			Mode:          "materialized",
			LatestBatchID: "batch-brief-001",
		},
		IngestSummary: insight.IngestSummary{
			State:          "ready",
			FreshnessState: "fresh",
			LatestBatchID:  "batch-brief-001",
		},
	}, runtime.Status{
		Phase:                   "team-edition-phase-a",
		RuntimeProfile:          runtime.RuntimeProfileTeamPreview,
		DashboardStoreMode:      "postgres",
		RankingStoreMode:        "postgres",
		CommentInsightStoreMode: "postgres",
		ReportSummaryStoreMode:  "postgres",
		WorkspaceStoreMode:      "postgres",
		IngestStoreMode:         "postgres",
		PostgresConfigured:      true,
		RedisConfigured:         true,
		ReadCacheConfigured:     true,
		ReadCacheActive:         true,
	})

	runtimeBrief, ok := brief["runtime"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtime brief payload, got %+v", brief)
	}
	if runtimeBrief["phase"] != "team-edition-phase-a" {
		t.Fatalf("expected phase in runtime brief, got %+v", runtimeBrief)
	}
	if runtimeBrief["runtimeProfile"] != runtime.RuntimeProfileTeamPreview {
		t.Fatalf("expected runtime profile in runtime brief, got %+v", runtimeBrief)
	}
	for _, field := range []string{
		"dashboardStoreMode",
		"rankingStoreMode",
		"commentInsightStoreMode",
		"reportSummaryStoreMode",
		"workspaceStoreMode",
		"ingestStoreMode",
	} {
		if runtimeBrief[field] != "postgres" {
			t.Fatalf("expected %s=postgres in runtime brief, got %+v", field, runtimeBrief)
		}
	}
}
