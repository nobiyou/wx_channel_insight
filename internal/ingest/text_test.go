package ingest

import (
	"testing"
	"time"
)

func TestSanitizeWXTextStripsHighlightTagsAndDecodesEntities(t *testing.T) {
	input := `<em class="highlight">AI</em> &amp; &quot;世界&quot; &#x1F680;`

	got := sanitizeWXText(input)
	want := `AI & "世界" 🚀`
	if got != want {
		t.Fatalf("sanitizeWXText(%q) = %q, want %q", input, got, want)
	}
}

func TestSanitizeWXTextKeepsTextAroundMultipleHighlightedSegments(t *testing.T) {
	input := `<em class="highlight">世界</em>模型<em class="highlight">路线</em>争议`

	got := sanitizeWXText(input)
	want := "世界模型路线争议"
	if got != want {
		t.Fatalf("sanitizeWXText(%q) = %q, want %q", input, got, want)
	}
}

func TestSanitizeWXTextPreservesBoundariesForBlockTags(t *testing.T) {
	input := `<div>第一段</div><p>第二段<br/>第三段</p>`

	got := sanitizeWXText(input)
	want := "第一段 第二段 第三段"
	if got != want {
		t.Fatalf("sanitizeWXText(%q) = %q, want %q", input, got, want)
	}
}

func TestParseWXFeedListItemSanitizesTitleAtIngestionBoundary(t *testing.T) {
	item := map[string]any{
		"id": "video-1",
		"objectDesc": map[string]any{
			"description": `<em class="highlight">AI</em>全是死路 &amp; 争议`,
		},
	}

	parsed := parseWXFeedListItem("demo@finder", item)
	if parsed.Title != "AI全是死路 & 争议" {
		t.Fatalf("parsed title = %q, want plain text", parsed.Title)
	}
}

func TestParseWXFeedListItemCapturesAllVideoMetrics(t *testing.T) {
	item := map[string]any{
		"id":           "video-metrics-1",
		"createtime":   "1719828000",
		"readCount":    "12000",
		"likeCount":    340,
		"commentCount": 56,
		"favCount":     78,
		"forwardCount": 19,
		"objectDesc": map[string]any{
			"description": "真实指标",
			"media": []any{map[string]any{
				"commentCount": 999,
			}},
		},
	}

	parsed := parseWXFeedListItem("demo@finder", item)
	if parsed.PublishedAt.IsZero() || !parsed.PublishedAt.Equal(time.Unix(1719828000, 0).UTC()) {
		t.Fatalf("expected source publication time, got %v", parsed.PublishedAt)
	}
	if parsed.PlayCount != 12000 || parsed.LikeCount != 340 || parsed.CommentCount != 56 || parsed.FavCount != 78 || parsed.ForwardCount != 19 {
		t.Fatalf("expected all top-level video metrics, got %+v", parsed)
	}

	events := BuildEventsFromWXFeedList("hub:user:test", "machine-1", WXFeedListResult{
		Username: "demo@finder",
		Items:    []WXFeedListItem{parsed},
	}, time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC))
	if len(events) != 1 {
		t.Fatalf("expected one video event, got %+v", events)
	}
	event := events[0]
	if event.PublishedAt.IsZero() || !event.PublishedAt.Equal(parsed.PublishedAt) {
		t.Fatalf("expected event to preserve publication time, got %v", event.PublishedAt)
	}
	if event.PlayCount != 12000 || event.LikeCount != 340 || event.CommentCount != 56 || event.FavCount != 78 || event.ForwardCount != 19 {
		t.Fatalf("expected event to preserve all video metrics, got %+v", event)
	}
}

func TestParseWXFeedListItemCapturesRealCoverURL(t *testing.T) {
	item := map[string]any{
		"id":       "video-cover-1",
		"coverUrl": "https://cdn.example.test/top-level-cover.jpg",
		"objectDesc": map[string]any{
			"description": "真实封面",
			"media": []any{map[string]any{
				"thumbUrl":     "https://cdn.example.test/media-thumb.jpg",
				"fullThumbUrl": "https://cdn.example.test/media-full-thumb.jpg",
			}},
		},
	}

	parsed := parseWXFeedListItem("demo@finder", item)
	if parsed.CoverURL != "https://cdn.example.test/top-level-cover.jpg" {
		t.Fatalf("expected top-level cover URL to win, got %q", parsed.CoverURL)
	}

	parsed = parseWXFeedListItem("demo@finder", map[string]any{
		"id": "video-cover-2",
		"objectDesc": map[string]any{
			"description": "媒体封面",
			"media": []any{map[string]any{
				"thumbUrl": "https://cdn.example.test/media-thumb.jpg",
			}},
		},
	})
	if parsed.CoverURL != "https://cdn.example.test/media-thumb.jpg" {
		t.Fatalf("expected nested media cover URL, got %q", parsed.CoverURL)
	}
}
