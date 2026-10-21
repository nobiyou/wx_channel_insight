package insight

import (
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func TestBuildHotTopicFromIngestScopesIDToEvent(t *testing.T) {
	summary := ingest.Summary{
		AggregatedComments: 2000,
		AggregatedTopics:   200,
		AggregatedVideos:   2000,
	}
	first := ingest.Event{
		ID:           "topic-1",
		BatchID:      "batch-1",
		Keyword:      "第一个主题",
		VideoCount:   2,
		CommentCount: 3,
		CollectedAt:  time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC),
	}
	second := first
	second.ID = "topic-2"
	second.Keyword = "第二个主题"
	second.VideoCount = 7
	second.CommentCount = 97

	firstTopic := buildHotTopicFromIngest(summary, first)
	secondTopic := buildHotTopicFromIngest(summary, second)
	if firstTopic == nil || secondTopic == nil {
		t.Fatalf("expected both topic events to produce hot topics: first=%+v second=%+v", firstTopic, secondTopic)
	}
	if firstTopic.ID != "ingest-topic-batch-1-topic-1" {
		t.Fatalf("expected event-scoped topic id, got %q", firstTopic.ID)
	}
	if firstTopic.ID == secondTopic.ID {
		t.Fatalf("expected same-batch topics to have distinct ids, got %q", firstTopic.ID)
	}
	if firstTopic.Mentions != 3 || firstTopic.RelatedVideos != 2 || firstTopic.GrowthRate != 0 {
		t.Fatalf("expected first topic to use source metrics without synthetic growth, got %+v", firstTopic)
	}
	if secondTopic.Mentions != 97 || secondTopic.RelatedVideos != 7 || secondTopic.GrowthRate != 0 {
		t.Fatalf("expected second topic to use its own source metrics, got %+v", secondTopic)
	}
}

func TestBuildHotTopicFromVideoAggregatesSourceMetricsByKeyword(t *testing.T) {
	summary := ingest.Summary{}
	stats := buildHotTopicMetricStats([]ingest.Event{
		{ID: "video-1", EventType: "video_snapshot", Keyword: "codex", VideoCount: 1, CommentCount: 34},
		{ID: "video-2", EventType: "video_snapshot", Keyword: "codex", VideoCount: 1, CommentCount: 182},
		{ID: "video-3", EventType: "video_snapshot", Keyword: "折叠屏", VideoCount: 1, CommentCount: 9},
	})

	topic := buildHotTopicFromVideoWithStats(summary, ingest.Event{
		ID:           "video-1",
		EventType:    "video_snapshot",
		Keyword:      "codex",
		VideoCount:   1,
		CommentCount: 34,
		Title:        "Codex 视频",
	}, stats)
	if topic == nil {
		t.Fatal("expected video event to produce a topic")
	}
	if topic.Mentions != 216 || topic.RelatedVideos != 2 || topic.GrowthRate != 0 {
		t.Fatalf("expected keyword aggregate to use source observations, got %+v", topic)
	}
}

func TestNormalizeTopicCandidatesSplitsAndCleansImportedLabels(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		values []string
		want   []string
	}{
		{name: "continuous hashtags", input: "#Codex#AI#人工智能", want: []string{"Codex", "AI", "人工智能"}},
		{name: "full width and duplicate hashtag", input: "＃ＡＩ", values: []string{"#ai"}, want: []string{"AI"}},
		{name: "plain keyword", input: "DeepSeek", values: []string{"AI 工具"}, want: []string{"DeepSeek", "AI 工具"}},
		{name: "numeric and fragments", input: "2", values: []string{"A", "步搞定", "小白也能轻松上手", "6分钟掌握DeepSeekV4-Flash接入Codex"}, want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeTopicCandidates(test.input, test.values...)
			if len(got) != len(test.want) {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
			for index := range test.want {
				if got[index] != test.want[index] {
					t.Fatalf("expected candidate %d to be %q, got %q", index, test.want[index], got[index])
				}
			}
		})
	}

	if got := canonicalTopicKey("#ＡＩ"); got != "ai" {
		t.Fatalf("expected canonical AI key, got %q", got)
	}
}

func TestBuildHotTopicMetricStatsMergesCanonicalHashtags(t *testing.T) {
	stats := buildHotTopicMetricStats([]ingest.Event{
		{ID: "video-1", EventType: "video_snapshot", Keyword: "AI", VideoCount: 1, CommentCount: 10},
		{ID: "video-2", EventType: "video_snapshot", Keyword: "#AI", VideoCount: 1, CommentCount: 20},
		{ID: "video-3", EventType: "video_snapshot", Keyword: "2", VideoCount: 1, CommentCount: 90},
	})

	metric, ok := stats["ai"]
	if !ok || metric.Mentions != 30 || metric.RelatedVideos != 2 {
		t.Fatalf("expected AI and #AI to share source metrics, got %+v", stats)
	}
	if _, exists := stats["2"]; exists {
		t.Fatalf("pure numeric candidate must be excluded, got %+v", stats)
	}

	topic := buildHotTopicFromVideoWithStats(ingest.Summary{}, ingest.Event{
		ID: "video-2", EventType: "video_snapshot", Keyword: "#AI", VideoCount: 1, CommentCount: 20,
	}, stats)
	if topic == nil || topic.Keyword != "AI" || topic.Mentions != 30 || topic.RelatedVideos != 2 {
		t.Fatalf("expected normalized topic view, got %+v", topic)
	}
}
