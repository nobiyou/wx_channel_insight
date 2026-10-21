package edge

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type recordingCommentPageFetcher struct {
	pages []json.RawMessage
	calls int
}

func (f *recordingCommentPageFetcher) FetchCommentListPage(context.Context, string, string, string, string) (json.RawMessage, error) {
	page := f.pages[f.calls]
	f.calls++
	return page, nil
}

func TestFetchCommentPagesPacesEveryPageAfterTheFirst(t *testing.T) {
	fetcher := &recordingCommentPageFetcher{pages: []json.RawMessage{
		json.RawMessage(`{"data":{"commentInfo":[{"commentId":"c1"}],"lastBuffer":"next","countInfo":{"commentCount":2}}}`),
		json.RawMessage(`{"data":{"commentInfo":[{"commentId":"c2"}],"countInfo":{"commentCount":2}}}`),
	}}
	waits := make([]time.Duration, 0, 1)
	pacer := newCommentRequestPacer(1200*time.Millisecond, func(_ context.Context, duration time.Duration) error {
		waits = append(waits, duration)
		return nil
	})

	items, reported, err := fetchCommentPagesVia(context.Background(), fetcher, "video-1", "nonce-1", "", pacer)
	if err != nil {
		t.Fatalf("fetch comment pages: %v", err)
	}
	if len(items) != 2 || reported != 2 || fetcher.calls != 2 {
		t.Fatalf("unexpected pagination result: items=%d reported=%d calls=%d", len(items), reported, fetcher.calls)
	}
	if len(waits) != 1 || waits[0] != 1200*time.Millisecond {
		t.Fatalf("expected one paced interval, got %#v", waits)
	}
}
