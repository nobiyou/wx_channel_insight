package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

const (
	videoPlaybackProfileCacheTTL      = 5 * time.Minute
	videoPlaybackProfileCacheCapacity = 256
)

type videoPlaybackProfileCache struct {
	mu       sync.Mutex
	ttl      time.Duration
	capacity int
	now      func() time.Time
	entries  map[string]videoPlaybackProfileCacheEntry
	pending  map[string]chan struct{}
}

type videoPlaybackProfileCacheEntry struct {
	profile   ingest.WXFeedListItem
	expiresAt time.Time
}

func newVideoPlaybackProfileCache() *videoPlaybackProfileCache {
	return &videoPlaybackProfileCache{
		ttl:      videoPlaybackProfileCacheTTL,
		capacity: videoPlaybackProfileCacheCapacity,
		now:      time.Now,
		entries:  make(map[string]videoPlaybackProfileCacheEntry),
		pending:  make(map[string]chan struct{}),
	}
}

func (c *videoPlaybackProfileCache) put(objectID, nonceID string, profile ingest.WXFeedListItem) {
	if c == nil || strings.TrimSpace(profile.VideoURL) == "" {
		return
	}
	key := videoPlaybackProfileCacheKey(objectID, nonceID)
	if key == "" || c.capacity <= 0 || c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.entries[key] = videoPlaybackProfileCacheEntry{
		profile:   profile,
		expiresAt: now.Add(c.ttl),
	}
	c.trimLocked(now)
}

func (c *videoPlaybackProfileCache) getOrFetch(
	ctx context.Context,
	objectID, nonceID string,
	fetch func(context.Context) (ingest.WXFeedListItem, error),
) (ingest.WXFeedListItem, error) {
	if c == nil || c.capacity <= 0 || c.ttl <= 0 {
		return fetch(ctx)
	}
	key := videoPlaybackProfileCacheKey(objectID, nonceID)
	if key == "" {
		return fetch(ctx)
	}

	for {
		now := c.now()
		c.mu.Lock()
		if entry, ok := c.entries[key]; ok {
			if now.Before(entry.expiresAt) {
				c.mu.Unlock()
				return entry.profile, nil
			}
			delete(c.entries, key)
		}
		if wait, ok := c.pending[key]; ok {
			c.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return ingest.WXFeedListItem{}, ctx.Err()
			}
		}

		ready := make(chan struct{})
		c.pending[key] = ready
		c.mu.Unlock()

		profile, err := fetch(ctx)

		c.mu.Lock()
		delete(c.pending, key)
		if err == nil && strings.TrimSpace(profile.VideoURL) != "" {
			now = c.now()
			c.entries[key] = videoPlaybackProfileCacheEntry{
				profile:   profile,
				expiresAt: now.Add(c.ttl),
			}
			c.trimLocked(now)
		}
		close(ready)
		c.mu.Unlock()
		return profile, err
	}
}

func (c *videoPlaybackProfileCache) trimLocked(now time.Time) {
	for key, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	for len(c.entries) > c.capacity {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range c.entries {
			if oldestKey == "" || entry.expiresAt.Before(oldest) {
				oldestKey = key
				oldest = entry.expiresAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(c.entries, oldestKey)
	}
}

func videoPlaybackProfileCacheKey(objectID, nonceID string) string {
	objectID = strings.TrimSpace(objectID)
	if objectID == "" {
		return ""
	}
	return objectID + "\x00" + strings.TrimSpace(nonceID)
}
