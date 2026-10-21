package insight

import (
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
)

func TestMergeHotTopicsNormalizesPersistedTopicValues(t *testing.T) {
	items := MergeHotTopics([]HotTopic{
		{ID: "hashes", Keyword: "#Codex#AI#人工智能#科技#热门#热点"},
		{ID: "sentence", Keyword: "凌晨2点被它嗨起来当工具人"},
		{ID: "number", Keyword: "2"},
		{ID: "plain", Keyword: "codex"},
	}, nil, 10)

	if len(items) != 1 {
		t.Fatalf("expected invalid topics and canonical duplicates to be removed, got %+v", items)
	}
	if items[0].Keyword != "Codex" {
		t.Fatalf("expected hashtag display to be normalized, got %+v", items[0])
	}
	if len(items[0].Keywords) != 6 || items[0].Keywords[0] != "Codex" || items[0].Keywords[1] != "AI" {
		t.Fatalf("expected independent hashtag candidates to be retained as metadata, got %+v", items[0].Keywords)
	}
}

func TestMapAggregateTopicMetricsNormalizesRawTopicValues(t *testing.T) {
	items := mapAggregateTopicMetrics([]aggregate.TopicMetric{
		{ID: "aggregate-hashes", Keyword: "#Codex#AI#人工智能#科技#热门#热点"},
		{ID: "aggregate-sentence", Keyword: "凌晨2点被它嗨起来当工具人"},
	})

	if len(items) != 1 || items[0].Keyword != "Codex" {
		t.Fatalf("expected aggregate mapping to use normalized topic owner, got %+v", items)
	}
	if len(items[0].Keywords) != 6 {
		t.Fatalf("expected aggregate hashtag metadata to be normalized, got %+v", items[0].Keywords)
	}
}
