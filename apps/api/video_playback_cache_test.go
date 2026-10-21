package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func TestVideoPlaybackProfileCacheReusesAndExpiresProfiles(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	cache := newVideoPlaybackProfileCache()
	cache.now = func() time.Time { return now }

	calls := 0
	fetch := func(context.Context) (ingest.WXFeedListItem, error) {
		calls++
		return ingest.WXFeedListItem{VideoURL: "https://cdn.example.test/video.mp4"}, nil
	}

	if _, err := cache.getOrFetch(context.Background(), "object-1", "nonce-1", fetch); err != nil {
		t.Fatalf("first profile fetch: %v", err)
	}
	if _, err := cache.getOrFetch(context.Background(), "object-1", "nonce-1", fetch); err != nil {
		t.Fatalf("cached profile fetch: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one fetch before expiry, got %d", calls)
	}

	now = now.Add(videoPlaybackProfileCacheTTL + time.Nanosecond)
	if _, err := cache.getOrFetch(context.Background(), "object-1", "nonce-1", fetch); err != nil {
		t.Fatalf("expired profile refresh: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected refresh after expiry, got %d fetches", calls)
	}
}

func TestVideoPlaybackProfileCacheDeduplicatesConcurrentFetches(t *testing.T) {
	cache := newVideoPlaybackProfileCache()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	fetch := func(context.Context) (ingest.WXFeedListItem, error) {
		calls.Add(1)
		close(started)
		<-release
		return ingest.WXFeedListItem{VideoURL: "https://cdn.example.test/video.mp4"}, nil
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := cache.getOrFetch(context.Background(), "object-1", "nonce-1", fetch)
		firstDone <- err
	}()
	<-started

	secondDone := make(chan error, 1)
	go func() {
		_, err := cache.getOrFetch(context.Background(), "object-1", "nonce-1", fetch)
		secondDone <- err
	}()
	close(release)

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first concurrent fetch: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first concurrent fetch did not finish")
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second concurrent fetch: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second concurrent fetch did not finish")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one upstream fetch for concurrent callers, got %d", got)
	}
}
