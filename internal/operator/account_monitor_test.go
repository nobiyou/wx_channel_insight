package operator

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

type accountMonitorStoreStub struct {
	account insight.Account
	upserts int
}

func (s *accountMonitorStoreStub) Get(context.Context, insight.AccountQuery, string) (insight.Account, error) {
	return insight.CloneAccount(s.account), nil
}

func (s *accountMonitorStoreStub) UpsertObservedProfile(context.Context, insight.AccountQuery, insight.AccountObservedProfile) (insight.Account, error) {
	s.upserts++
	return s.account, nil
}

func TestRunAccountProfileSkipsFreshProfileWithoutUpstreamRequest(t *testing.T) {
	now := time.Date(2026, 8, 24, 14, 0, 0, 0, time.UTC)
	store := &accountMonitorStoreStub{account: insight.Account{
		ID:                "account-fresh",
		IdentityStatus:    insight.AccountIdentityVerified,
		ChannelUsername:   "gh_fresh@finder",
		AvatarURL:         "https://example.test/fresh.png",
		ProfileObservedAt: now.Add(-23 * time.Hour),
	}}
	monitor, err := NewAccountMonitor(ingest.NewService(ingestdemo.NewRepository(), ingest.ServiceConfig{}), store)
	if err != nil {
		t.Fatalf("new account monitor: %v", err)
	}
	monitor.now = func() time.Time { return now }

	result, err := monitor.RunAccountProfile(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:test"}, monitoring.TaskTarget{
		TargetKind: monitoring.TargetKindAccount,
		AccountID:  "account-fresh",
	})
	if err != nil {
		t.Fatalf("fresh profile should be a successful no-op: %v", err)
	}
	if result.Status != monitoring.RunStatusSuccess || result.Attempted != 1 || result.Succeeded != 1 || result.Failed != 0 || result.Skipped != 0 {
		t.Fatalf("expected successful fresh-profile no-op, got %+v", result)
	}
	if result.Summary.State != "profile_fresh" || result.Summary.FreshnessState != "fresh" {
		t.Fatalf("expected explicit fresh summary, got %+v", result.Summary)
	}
	if store.upserts != 0 {
		t.Fatalf("fresh profile must not persist an upstream update, got %d upserts", store.upserts)
	}
}

func TestIsAccountProfileFreshExpiresAtWindowBoundary(t *testing.T) {
	now := time.Date(2026, 8, 24, 14, 0, 0, 0, time.UTC)
	account := insight.Account{AvatarURL: "https://example.test/avatar.png", ProfileObservedAt: now.Add(-accountProfileFreshnessWindow)}
	if isAccountProfileFresh(account, now) {
		t.Fatal("profile at the freshness boundary must be refreshed")
	}
	account.ProfileObservedAt = now.Add(-accountProfileFreshnessWindow + time.Nanosecond)
	if !isAccountProfileFresh(account, now) {
		t.Fatal("profile inside the freshness window should be treated as fresh")
	}
	account.AvatarURL = ""
	if isAccountProfileFresh(account, now) {
		t.Fatal("a feed-derived identity timestamp without profile fields must not suppress refresh")
	}
}

func TestEvaluateCommentSyncBackfillProgressesWithoutRepeatingSuccessfulVideos(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	video := accountVideoSync{
		found: true,
		item:  ingest.WXFeedListItem{ID: "video-1", ObjectNonceID: "nonce-1", CommentCount: 8},
		state: ingest.VideoCommentSyncState{
			VideoObjectID: "video-1", VideoObjectNonceID: "nonce-1", LastCommentCount: 8,
			LastCollectedAt: now.Add(-time.Hour), Status: ingest.VideoCommentSyncStatusSuccess,
		},
	}
	if decision := evaluateCommentSync(now, monitoring.CommentModeFullBackfill, video, defaultAccountCommentPolicy); decision.collect {
		t.Fatalf("successful full-backfill video should not be collected again: %#v", decision)
	}
	video.found = false
	if decision := evaluateCommentSync(now, monitoring.CommentModeFullBackfill, video, defaultAccountCommentPolicy); !decision.collect {
		t.Fatalf("uncollected full-backfill video should be selected: %#v", decision)
	}
}

func TestEvaluateCommentSyncAppliesFailureAndSuccessCooldowns(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	failed := accountVideoSync{
		found: true,
		item:  ingest.WXFeedListItem{ID: "video-failed", ObjectNonceID: "nonce", CommentCount: 9},
		state: ingest.VideoCommentSyncState{
			VideoObjectID: "video-failed", VideoObjectNonceID: "nonce", LastCommentCount: 9,
			LastObservedAt: now.Add(-time.Hour), Status: ingest.VideoCommentSyncStatusFailed, LastError: "temporary failure",
		},
	}
	decision := evaluateCommentSync(now, monitoring.CommentModeIncremental, failed, defaultAccountCommentPolicy)
	if decision.collect || !decision.reportSkip {
		t.Fatalf("recent ordinary failure should cool down: %#v", decision)
	}
	failed.state.LastObservedAt = now.Add(-3 * time.Hour)
	if decision = evaluateCommentSync(now, monitoring.CommentModeIncremental, failed, defaultAccountCommentPolicy); !decision.collect {
		t.Fatalf("expired ordinary failure should retry: %#v", decision)
	}

	changed := accountVideoSync{
		found: true,
		item:  ingest.WXFeedListItem{ID: "video-changed", ObjectNonceID: "nonce", CommentCount: 11},
		state: ingest.VideoCommentSyncState{
			VideoObjectID: "video-changed", VideoObjectNonceID: "nonce", LastCommentCount: 10,
			LastCollectedAt: now.Add(-10 * time.Minute), Status: ingest.VideoCommentSyncStatusSuccess,
		},
	}
	if decision = evaluateCommentSync(now, monitoring.CommentModeIncremental, changed, defaultAccountCommentPolicy); decision.collect || !decision.reportSkip {
		t.Fatalf("recent successful collection should cool down: %#v", decision)
	}
}

func TestAccountCommentRiskCooldownAndBatchLimit(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	videos := []accountVideoSync{{
		found: true,
		item:  ingest.WXFeedListItem{ID: "video-risk"},
		state: ingest.VideoCommentSyncState{
			LastObservedAt: now.Add(-time.Hour), Status: ingest.VideoCommentSyncStatusFailed,
			LastError: "edge comment call HTTP 429: 操作频繁",
		},
	}}
	blocked, until := activeCommentRiskCooldown(videos, now, defaultAccountCommentPolicy)
	if blocked == nil || blocked.item.ID != "video-risk" || !until.Equal(now.Add(23*time.Hour)) {
		t.Fatalf("expected 24-hour account risk cooldown, got blocked=%#v until=%s", blocked, until)
	}

	candidates := make([]accountVideoSync, 5)
	selected, deferred := limitCommentCandidates(candidates, 3)
	if len(selected) != 3 || deferred != 2 {
		t.Fatalf("expected 3 selected and 2 deferred, got %d and %d", len(selected), deferred)
	}
	if !isCommentRiskError("HTTP 403 forbidden") || isCommentRiskError("temporary network timeout") {
		t.Fatal("unexpected risk error classification")
	}
}

func TestActiveAccountCommentCooldownBlocksImmediateFollowUpBatch(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	videos := []accountVideoSync{{
		found: true,
		item:  ingest.WXFeedListItem{ID: "video-recent"},
		state: ingest.VideoCommentSyncState{
			LastCollectedAt: now.Add(-10 * time.Minute), Status: ingest.VideoCommentSyncStatusSuccess,
		},
	}}
	blocked, until := activeAccountCommentCooldown(videos, now, defaultAccountCommentPolicy)
	if blocked == nil || blocked.item.ID != "video-recent" || !until.Equal(now.Add(20*time.Minute)) {
		t.Fatalf("expected account batch cooldown, got blocked=%#v until=%s", blocked, until)
	}
	videos[0].state.LastCollectedAt = now.Add(-31 * time.Minute)
	if blocked, _ = activeAccountCommentCooldown(videos, now, defaultAccountCommentPolicy); blocked != nil {
		t.Fatalf("expired account batch cooldown should not block: %#v", blocked)
	}
}

func TestEvaluateSingleCommentSyncUsesRiskAndSuccessCooldowns(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	video := accountVideoSync{
		found: true,
		item:  ingest.WXFeedListItem{ID: "video-single", ObjectNonceID: "nonce-single", CommentCount: 12},
		state: ingest.VideoCommentSyncState{
			LastCommentCount: 10,
			LastCollectedAt:  now.Add(-10 * time.Minute),
			Status:           ingest.VideoCommentSyncStatusSuccess,
		},
	}
	decision := evaluateSingleCommentSync(now, video, defaultAccountCommentPolicy)
	if decision.collect || !decision.reportSkip || decision.reason == "" {
		t.Fatalf("recent single-video collection should be skipped with a reason: %#v", decision)
	}

	video.state.LastCollectedAt = time.Time{}
	video.state.LastObservedAt = now.Add(-time.Hour)
	video.state.Status = ingest.VideoCommentSyncStatusFailed
	video.state.LastError = "HTTP 429: 操作频繁"
	decision = evaluateSingleCommentSync(now, video, defaultAccountCommentPolicy)
	if decision.collect || !decision.reportSkip || decision.reason == "" {
		t.Fatalf("single-video risk cooldown should be skipped with a reason: %#v", decision)
	}
}
