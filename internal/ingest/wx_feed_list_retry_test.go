package ingest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWXFeedListClientFetchRetriesTimeoutOnce(t *testing.T) {
	var attempts int32
	client := WXFeedListClient{
		BaseURL: "http://127.0.0.1:2026",
		Client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				current := atomic.AddInt32(&attempts, 1)
				if current == 1 {
					return nil, context.DeadlineExceeded
				}
				body := `{"code":0,"message":"success","data":{"data":{"BaseResponse":{"Ret":0},"objectList":[{"id":"video-1","objectDesc":{"description":"retry title","media":[{"commentCount":12}]},"contact":{"nickname":"Retry Channel"}}],"lastBuffer":""}}}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	result, err := client.Fetch(context.Background(), "retry_user@finder")
	if err != nil {
		t.Fatalf("expected retry fetch success, got %v", err)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("expected timeout retry to perform 2 attempts, got %d", attempts)
	}
	if result.Username != "retry_user@finder" || len(result.Items) != 1 || result.Items[0].ID != "video-1" {
		t.Fatalf("expected parsed retry result, got %+v", result)
	}
}

func TestWXFeedListClientFetchDoesNotRetryNonTimeoutError(t *testing.T) {
	var attempts int32
	client := WXFeedListClient{
		BaseURL: "http://127.0.0.1:2026",
		Client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				atomic.AddInt32(&attempts, 1)
				return nil, errors.New("boom")
			}),
		},
	}

	_, err := client.Fetch(context.Background(), "retry_user@finder")
	if err == nil {
		t.Fatal("expected non-timeout fetch failure")
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Fatalf("expected non-timeout failure to avoid retry, got %d attempts", attempts)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected original non-timeout error, got %v", err)
	}
}
