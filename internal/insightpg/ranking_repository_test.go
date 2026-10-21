package insightpg

import (
	"testing"
	"time"
)

func TestParseHotVideoSnapshotPayloadAcceptsNullItems(t *testing.T) {
	updatedAt := time.Date(2026, 8, 7, 2, 0, 0, 0, time.UTC)
	items, meta, err := parseHotVideoSnapshotPayload(
		[]byte(`{"meta":{"latestBatchId":"batch-video"},"items":null}`),
		updatedAt,
	)
	if err != nil {
		t.Fatalf("parse hot video snapshot failed: %v", err)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("expected a non-nil empty item list, got %#v", items)
	}
	if meta.LatestBatchID != "batch-video" || !meta.MaterializedAt.Equal(updatedAt) {
		t.Fatalf("unexpected hot video metadata: %+v", meta)
	}
}

func TestParseHotTopicSnapshotPayloadAcceptsNullItems(t *testing.T) {
	updatedAt := time.Date(2026, 8, 7, 2, 0, 0, 0, time.UTC)
	items, meta, err := parseHotTopicSnapshotPayload(
		[]byte(`{"meta":{"latestBatchId":"batch-topic"},"items":null}`),
		updatedAt,
	)
	if err != nil {
		t.Fatalf("parse hot topic snapshot failed: %v", err)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("expected a non-nil empty item list, got %#v", items)
	}
	if meta.LatestBatchID != "batch-topic" || !meta.MaterializedAt.Equal(updatedAt) {
		t.Fatalf("unexpected hot topic metadata: %+v", meta)
	}
}

func TestParseReadModelArtifactMetaAcceptsNullPayload(t *testing.T) {
	updatedAt := time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC)
	meta, err := parseReadModelArtifactMeta(nil, updatedAt)
	if err != nil {
		t.Fatalf("parse optional metadata failed: %v", err)
	}
	if !meta.MaterializedAt.Equal(updatedAt) {
		t.Fatalf("expected updated_at for optional metadata, got %+v", meta)
	}
}

func TestSnapshotParsersRejectOldPayloads(t *testing.T) {
	updatedAt := time.Now()
	if _, _, err := parseHotVideoSnapshotPayload([]byte(`[{"id":"old"}]`), updatedAt); err == nil {
		t.Error("old hot-video array was accepted")
	}
	if _, err := parseHotVideoRankings([]byte(`[{"id":"old"}]`), updatedAt); err == nil {
		t.Error("old ranking array was accepted")
	}
	if _, _, err := parseHotTopicSnapshotPayload([]byte(`[{"id":"old"}]`), updatedAt); err == nil {
		t.Error("old hot-topic array was accepted")
	}
	if _, _, err := parseCommentInsightSnapshotPayload([]byte(`[{"id":"old"}]`), updatedAt); err == nil {
		t.Error("old comment array was accepted")
	}
	if _, _, err := parseReportSummarySnapshotPayload([]byte(`{"headline":"old summary"}`), updatedAt); err == nil {
		t.Error("old report object was accepted")
	}
}
