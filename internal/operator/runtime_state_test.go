package operator

import (
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func TestRuntimeRequestedSourcesForResultUsesActualDefaultVideoSource(t *testing.T) {
	state := BuildJobsRuntimeState(Result{
		Status:      "ok",
		FinalStatus: "ok",
		Imports: []ImportedSummary{
			{Source: ingest.RunSourceWXCommentExport},
			{Source: ingest.RunSourceWXFeedList},
		},
	}, Options{}, nil, JobsStateOptions{Mode: "operator-loop"})

	if len(state.LastRequestedSources) != 2 ||
		state.LastRequestedSources[0] != ingest.RunSourceWXCommentExport ||
		state.LastRequestedSources[1] != ingest.RunSourceWXFeedList {
		t.Fatalf("expected actual feed-list default sources, got %+v", state.LastRequestedSources)
	}
}

func TestRuntimeRequestedSourcesFromSyncPayloadUsesActualDefaultVideoSource(t *testing.T) {
	sources := RuntimeRequestedSourcesFromSyncPayload(nil, map[string]any{
		"imports": []any{
			map[string]any{"source": ingest.RunSourceWXCommentExport},
			map[string]any{"source": ingest.RunSourceWXFeedList},
		},
	})

	if len(sources) != 2 ||
		sources[0] != ingest.RunSourceWXCommentExport ||
		sources[1] != ingest.RunSourceWXFeedList {
		t.Fatalf("expected actual feed-list payload sources, got %+v", sources)
	}
}

func TestRuntimeRequestedSourcesKeepsExplicitRequestOrder(t *testing.T) {
	requested, err := NormalizeRequestedSources([]string{ingest.RunSourceWXFeedList, ingest.RunSourceWXCommentExport})
	if err != nil {
		t.Fatalf("NormalizeRequestedSources() error = %v", err)
	}
	sources := RuntimeRequestedSourcesForResult(requested, Result{
		Imports: []ImportedSummary{
			{Source: ingest.RunSourceWXCommentExport},
			{Source: ingest.RunSourceWXFeedList},
		},
	})

	if len(sources) != 2 ||
		sources[0] != ingest.RunSourceWXCommentExport ||
		sources[1] != ingest.RunSourceWXFeedList {
		t.Fatalf("expected explicit request canonical order, got %+v", sources)
	}
}
