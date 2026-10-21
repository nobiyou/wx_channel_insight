package ingest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResumedCollectionRefreshesNewestPageBeforeHistory(t *testing.T) {
	for _, search := range []bool{false, true} {
		t.Run(fmt.Sprint(search), func(t *testing.T) {
			var calls []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				marker := r.URL.Query().Get("next_marker")
				calls = append(calls, marker)
				if marker == "" {
					fmt.Fprint(w, `{"data":{"objectList":[{"id":"new","likeCount":9},{"id":"overlap","likeCount":8}],"lastBuffer":"new-cursor"}}`)
				} else if marker == "saved-cursor" {
					fmt.Fprint(w, `{"data":{"objectList":[{"id":"old"},{"id":"overlap","likeCount":1}],"lastBuffer":"next-old"}}`)
				} else {
					t.Error("historical cursor overwritten", marker)
				}
			}))
			defer upstream.Close()
			client := WXFeedListClient{BaseURL: upstream.URL, Client: upstream.Client(), MaxPages: 1, StartMarker: "saved-cursor"}
			result, err := client.collectPages(context.Background(), "target", search)
			if err != nil || len(calls) != 2 || calls[0] != "" || calls[1] != "saved-cursor" || len(result.Items) != 3 || result.NextMarker != "next-old" || result.Complete {
				t.Fatalf("newest data or historical progress lost: calls=%v result=%+v err=%v", calls, result, err)
			}
			if result.Items[1].LikeCount != 8 {
				t.Fatalf("fresh overlap replaced: %+v", result.Items)
			}
			progress := progressFromResult(CollectionProgress{PageCount: 10, VideoCount: 100}, result, time.Now())
			if progress.PageCount != 11 || progress.VideoCount != 101 {
				t.Fatalf("latest-page refresh inflated historical coverage: %+v", progress)
			}
		})
	}
}

func TestResumedCollectionRetainsFreshHeadWhenHistoryFails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("next_marker") != "" {
			http.Error(w, "cursor unavailable", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, `{"data":{"objectList":[{"id":"new","likeCount":9}],"lastBuffer":"new-cursor"}}`)
	}))
	defer upstream.Close()
	client := WXFeedListClient{BaseURL: upstream.URL, Client: upstream.Client(), StartMarker: "historical-cursor"}
	result, err := client.Fetch(context.Background(), "target")
	if err != nil || len(result.Items) != 1 || result.Items[0].ID != "new" || result.Complete || result.NextMarker != "historical-cursor" || result.Notice == "" {
		t.Fatalf("fresh data or retry cursor lost: %+v %v", result, err)
	}
}

func TestFeedPaginationFollowsCursorAndDeduplicates(t *testing.T) {
	for _, search := range []bool{false, true} {
		t.Run(fmt.Sprint(search), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Query().Get("next_marker") {
				case "":
					fmt.Fprint(w, `{"code":0,"data":{"objectList":[{"id":"1"}],"lastBuffer":"page2"}}`)
				case "page2":
					fmt.Fprint(w, `{"code":0,"data":{"objectList":[{"id":"1"},{"id":"2"}],"lastBuffer":""}}`)
				default:
					t.Error("unexpected cursor")
				}
			}))
			defer server.Close()
			client := WXFeedListClient{BaseURL: server.URL, Client: server.Client()}
			var result WXFeedListResult
			var err error
			if search {
				result, err = client.SearchVideos(context.Background(), "topic")
			} else {
				result, err = client.Fetch(context.Background(), "account")
			}
			if err != nil || len(result.Items) != 2 || !result.Complete || result.PageCount != 2 || calls != 2 {
				t.Fatalf("pagination result: %+v err=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestFeedPaginationKeepsProgressOnFailureAndStopsRepeatedCursor(t *testing.T) {
	for _, fail := range []bool{true, false} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if fail && r.URL.Query().Get("next_marker") != "" {
					http.Error(w, "offline", 503)
					return
				}
				fmt.Fprint(w, `{"code":0,"data":{"objectList":[{"id":"1"}],"lastBuffer":"same"}}`)
			}))
			defer server.Close()
			result, err := (WXFeedListClient{BaseURL: server.URL, Client: server.Client()}).Fetch(context.Background(), "account")
			if err != nil || len(result.Items) != 1 || result.Complete || result.NextMarker != "same" || result.Notice == "" {
				t.Fatalf("must retain partial progress: %+v %v", result, err)
			}
		})
	}
}
