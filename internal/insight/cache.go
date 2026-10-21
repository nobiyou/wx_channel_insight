package insight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/rediscache"
	"github.com/redis/go-redis/v9"
)

const defaultReadCacheTTL = 15 * time.Minute

const (
	readCacheScopeDashboard       = "dashboard"
	readCacheScopeHotVideos       = "hot_videos"
	readCacheScopeHotTopics       = "hot_topics"
	readCacheScopeCommentInsights = "comment_insights"
	readCacheScopeReportSummary   = "report_summary"
)

var readCacheTrackedScopes = []string{
	readCacheScopeDashboard,
	readCacheScopeHotVideos,
	readCacheScopeHotTopics,
	readCacheScopeCommentInsights,
	readCacheScopeReportSummary,
}

type ReadCache struct {
	client *rediscache.Client
	ttl    time.Duration
	stats  readCacheStats
	scopes map[string]*readCacheScopeStats
}

type ReadCacheSnapshot struct {
	Enabled         bool      `json:"enabled"`
	HitCount        int64     `json:"hitCount"`
	MissCount       int64     `json:"missCount"`
	FallbackCount   int64     `json:"fallbackCount"`
	WriteCount      int64     `json:"writeCount"`
	InvalidateCount int64     `json:"invalidateCount"`
	LastHitAt       time.Time `json:"lastHitAt"`
	LastMissAt      time.Time `json:"lastMissAt"`
	LastFallbackAt  time.Time `json:"lastFallbackAt"`
	LastWriteAt     time.Time `json:"lastWriteAt"`
	LastInvalidateAt time.Time `json:"lastInvalidateAt"`
	ScopeStats      map[string]ReadCacheScopeSnapshot `json:"scopeStats,omitempty"`
}

type ReadCacheScopeSnapshot struct {
	HitCount       int64     `json:"hitCount"`
	MissCount      int64     `json:"missCount"`
	FallbackCount  int64     `json:"fallbackCount"`
	WriteCount     int64     `json:"writeCount"`
	LastHitAt      time.Time `json:"lastHitAt"`
	LastMissAt     time.Time `json:"lastMissAt"`
	LastFallbackAt time.Time `json:"lastFallbackAt"`
	LastWriteAt    time.Time `json:"lastWriteAt"`
}

type readCacheStats struct {
	hitCount         atomic.Int64
	missCount        atomic.Int64
	fallbackCount    atomic.Int64
	writeCount       atomic.Int64
	invalidateCount  atomic.Int64
	lastHitAtUnix    atomic.Int64
	lastMissAtUnix   atomic.Int64
	lastFallbackAtUnix atomic.Int64
	lastWriteAtUnix  atomic.Int64
	lastInvalidateAtUnix atomic.Int64
}

type readCacheScopeStats struct {
	hitCount          atomic.Int64
	missCount         atomic.Int64
	fallbackCount     atomic.Int64
	writeCount        atomic.Int64
	lastHitAtUnix     atomic.Int64
	lastMissAtUnix    atomic.Int64
	lastFallbackAtUnix atomic.Int64
	lastWriteAtUnix   atomic.Int64
}

func NewReadCache(client *rediscache.Client, ttl time.Duration) *ReadCache {
	if ttl <= 0 {
		ttl = defaultReadCacheTTL
	}
	scopeStats := make(map[string]*readCacheScopeStats, len(readCacheTrackedScopes))
	for _, scope := range readCacheTrackedScopes {
		scopeStats[scope] = &readCacheScopeStats{}
	}
	return &ReadCache{
		client: client,
		ttl:    ttl,
		scopes: scopeStats,
	}
}

func (c *ReadCache) Enabled() bool {
	return c != nil && c.client != nil
}

func (c *ReadCache) Snapshot() ReadCacheSnapshot {
	if c == nil {
		return ReadCacheSnapshot{}
	}
	return ReadCacheSnapshot{
		Enabled:          c.Enabled(),
		HitCount:         c.stats.hitCount.Load(),
		MissCount:        c.stats.missCount.Load(),
		FallbackCount:    c.stats.fallbackCount.Load(),
		WriteCount:       c.stats.writeCount.Load(),
		InvalidateCount:  c.stats.invalidateCount.Load(),
		LastHitAt:        unixToTime(c.stats.lastHitAtUnix.Load()),
		LastMissAt:       unixToTime(c.stats.lastMissAtUnix.Load()),
		LastFallbackAt:   unixToTime(c.stats.lastFallbackAtUnix.Load()),
		LastWriteAt:      unixToTime(c.stats.lastWriteAtUnix.Load()),
		LastInvalidateAt: unixToTime(c.stats.lastInvalidateAtUnix.Load()),
		ScopeStats:       c.snapshotScopeStats(),
	}
}

func (c *ReadCache) RecordFallback(now time.Time) {
	c.recordFallback("", now)
}

func (c *ReadCache) RecordScopeFallback(scope string, now time.Time) {
	c.recordFallback(scope, now)
}

func (c *ReadCache) recordFallback(scope string, now time.Time) {
	if c == nil || !c.Enabled() {
		return
	}
	c.stats.fallbackCount.Add(1)
	c.stats.lastFallbackAtUnix.Store(now.Unix())
	if scoped := c.scopeStatsFor(scope); scoped != nil {
		scoped.fallbackCount.Add(1)
		scoped.lastFallbackAtUnix.Store(now.Unix())
	}
}

func (c *ReadCache) LoadDashboard(ctx context.Context, workspaceID string) (Dashboard, bool, error) {
	var payload Dashboard
	found, err := c.loadJSON(ctx, dashboardCacheKey(workspaceID), &payload, readCacheScopeDashboard)
	return payload, found, err
}

func (c *ReadCache) SaveDashboard(ctx context.Context, workspaceID string, payload Dashboard) error {
	return c.saveJSON(ctx, dashboardCacheKey(workspaceID), payload, readCacheScopeDashboard)
}

func (c *ReadCache) LoadHotVideos(ctx context.Context, workspaceID string) ([]HotVideo, bool, error) {
	var items []HotVideo
	found, err := c.loadJSON(ctx, hotVideosCacheKey(workspaceID), &items, readCacheScopeHotVideos)
	if !found || err != nil {
		return nil, found, err
	}
	return cloneHotVideos(items), true, nil
}

func (c *ReadCache) SaveHotVideos(ctx context.Context, workspaceID string, items []HotVideo) error {
	return c.saveJSON(ctx, hotVideosCacheKey(workspaceID), cloneHotVideos(items), readCacheScopeHotVideos)
}

func (c *ReadCache) LoadHotTopics(ctx context.Context, workspaceID string) ([]HotTopic, bool, error) {
	var items []HotTopic
	found, err := c.loadJSON(ctx, hotTopicsCacheKey(workspaceID), &items, readCacheScopeHotTopics)
	if !found || err != nil {
		return nil, found, err
	}
	return cloneHotTopics(items), true, nil
}

func (c *ReadCache) SaveHotTopics(ctx context.Context, workspaceID string, items []HotTopic) error {
	return c.saveJSON(ctx, hotTopicsCacheKey(workspaceID), cloneHotTopics(items), readCacheScopeHotTopics)
}

func (c *ReadCache) LoadCommentInsights(ctx context.Context, workspaceID string) ([]CommentInsight, bool, error) {
	var items []CommentInsight
	found, err := c.loadJSON(ctx, commentInsightsCacheKey(workspaceID), &items, readCacheScopeCommentInsights)
	if !found || err != nil {
		return nil, found, err
	}
	return cloneCommentInsights(items), true, nil
}

func (c *ReadCache) SaveCommentInsights(ctx context.Context, workspaceID string, items []CommentInsight) error {
	return c.saveJSON(ctx, commentInsightsCacheKey(workspaceID), cloneCommentInsights(items), readCacheScopeCommentInsights)
}

func (c *ReadCache) LoadReportSummary(ctx context.Context, workspaceID string) (ReportSummary, bool, error) {
	var payload ReportSummary
	found, err := c.loadJSON(ctx, reportSummaryCacheKey(workspaceID), &payload, readCacheScopeReportSummary)
	return payload, found, err
}

func (c *ReadCache) SaveReportSummary(ctx context.Context, workspaceID string, payload ReportSummary) error {
	return c.saveJSON(ctx, reportSummaryCacheKey(workspaceID), payload, readCacheScopeReportSummary)
}

func (c *ReadCache) InvalidateWorkspace(ctx context.Context, workspaceID string) error {
	if !c.Enabled() {
		return nil
	}
	if err := c.client.Delete(ctx,
		dashboardCacheKey(workspaceID),
		hotVideosCacheKey(workspaceID),
		hotTopicsCacheKey(workspaceID),
		commentInsightsCacheKey(workspaceID),
		reportSummaryCacheKey(workspaceID),
	); err != nil {
		return err
	}
	c.recordInvalidate(time.Now())
	return nil
}

func (c *ReadCache) loadJSON(ctx context.Context, key string, target any, scope string) (bool, error) {
	if !c.Enabled() {
		return false, nil
	}
	payload, err := c.client.Get(ctx, key)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			c.recordMiss(time.Now(), scope)
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal([]byte(payload), target); err != nil {
		return false, err
	}
	c.recordHit(time.Now(), scope)
	return true, nil
}

func (c *ReadCache) saveJSON(ctx context.Context, key string, payload any, scope string) error {
	if !c.Enabled() {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := c.client.Set(ctx, key, string(data), c.ttl); err != nil {
		return err
	}
	c.recordWrite(time.Now(), scope)
	return nil
}

func (c *ReadCache) recordHit(now time.Time, scope string) {
	c.stats.hitCount.Add(1)
	c.stats.lastHitAtUnix.Store(now.Unix())
	if scoped := c.scopeStatsFor(scope); scoped != nil {
		scoped.hitCount.Add(1)
		scoped.lastHitAtUnix.Store(now.Unix())
	}
}

func (c *ReadCache) recordMiss(now time.Time, scope string) {
	c.stats.missCount.Add(1)
	c.stats.lastMissAtUnix.Store(now.Unix())
	if scoped := c.scopeStatsFor(scope); scoped != nil {
		scoped.missCount.Add(1)
		scoped.lastMissAtUnix.Store(now.Unix())
	}
}

func (c *ReadCache) recordWrite(now time.Time, scope string) {
	c.stats.writeCount.Add(1)
	c.stats.lastWriteAtUnix.Store(now.Unix())
	if scoped := c.scopeStatsFor(scope); scoped != nil {
		scoped.writeCount.Add(1)
		scoped.lastWriteAtUnix.Store(now.Unix())
	}
}

func (c *ReadCache) recordInvalidate(now time.Time) {
	c.stats.invalidateCount.Add(1)
	c.stats.lastInvalidateAtUnix.Store(now.Unix())
}

func unixToTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(value, 0).UTC()
}

func (c *ReadCache) scopeStatsFor(scope string) *readCacheScopeStats {
	if c == nil || scope == "" {
		return nil
	}
	return c.scopes[scope]
}

func (c *ReadCache) snapshotScopeStats() map[string]ReadCacheScopeSnapshot {
	if c == nil || len(c.scopes) == 0 {
		return nil
	}
	result := make(map[string]ReadCacheScopeSnapshot, len(readCacheTrackedScopes))
	for _, scope := range readCacheTrackedScopes {
		stats := c.scopeStatsFor(scope)
		if stats == nil {
			continue
		}
		result[scope] = ReadCacheScopeSnapshot{
			HitCount:       stats.hitCount.Load(),
			MissCount:      stats.missCount.Load(),
			FallbackCount:  stats.fallbackCount.Load(),
			WriteCount:     stats.writeCount.Load(),
			LastHitAt:      unixToTime(stats.lastHitAtUnix.Load()),
			LastMissAt:     unixToTime(stats.lastMissAtUnix.Load()),
			LastFallbackAt: unixToTime(stats.lastFallbackAtUnix.Load()),
			LastWriteAt:    unixToTime(stats.lastWriteAtUnix.Load()),
		}
	}
	return result
}

func dashboardCacheKey(workspaceID string) string {
	return fmt.Sprintf("insight:dashboard:%s", normalizeCacheWorkspaceID(workspaceID))
}

func hotVideosCacheKey(workspaceID string) string {
	return fmt.Sprintf("insight:hot-videos:%s", normalizeCacheWorkspaceID(workspaceID))
}

func hotTopicsCacheKey(workspaceID string) string {
	return fmt.Sprintf("insight:hot-topics:%s", normalizeCacheWorkspaceID(workspaceID))
}

func commentInsightsCacheKey(workspaceID string) string {
	return fmt.Sprintf("insight:comment-insights:%s", normalizeCacheWorkspaceID(workspaceID))
}

func reportSummaryCacheKey(workspaceID string) string {
	return fmt.Sprintf("insight:report-summary:%s", normalizeCacheWorkspaceID(workspaceID))
}

func normalizeCacheWorkspaceID(workspaceID string) string {
	if workspaceID == "" {
		return "default"
	}
	return workspaceID
}
