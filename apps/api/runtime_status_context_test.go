package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestHandleRuntimeStatusCancelsUpstreamProbeWithRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	upstreamCanceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	defer upstream.Close()
	srv := newServer(nil, nil, nil, nil, runtime.Status{WXChannelBaseURL: upstream.URL})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	started := time.Now()
	srv.routes().ServeHTTP(rec, req)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("runtime probe ignored request cancellation: %s", elapsed)
	}
	select {
	case <-upstreamCanceled:
	case <-time.After(time.Second):
		t.Fatal("upstream status probe was not canceled")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected partial runtime status, got %d: %s", rec.Code, rec.Body.String())
	}
}
