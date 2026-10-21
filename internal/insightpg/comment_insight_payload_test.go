package insightpg

import (
	"testing"
	"time"
)

func TestEmptyCommentInsightEnvelopeRemainsReadable(t *testing.T) {
	for _, payload := range []string{`{"items":null,"meta":{}}`, `{"items":[],"meta":{}}`} {
		items, meta, err := parseCommentInsightSnapshotPayload([]byte(payload), time.Unix(100, 0))
		if err != nil || len(items) != 0 || meta.MaterializedAt.IsZero() {
			t.Fatalf("empty snapshot %s: items=%v meta=%v err=%v", payload, items, meta, err)
		}
	}
	if _, _, err := parseCommentInsightSnapshotPayload([]byte(`{"items":{}}`), time.Now()); err == nil {
		t.Fatal("malformed items must not become an empty snapshot")
	}
}
