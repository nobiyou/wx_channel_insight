package operator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

type accountStore interface {
	Get(context.Context, insight.AccountQuery, string) (insight.Account, error)
	UpsertObservedProfile(context.Context, insight.AccountQuery, insight.AccountObservedProfile) (insight.Account, error)
}

type AccountMonitor struct {
	ingestService *ingest.Service
	accounts      accountStore
	now           func() time.Time
	policy        accountCommentPolicy
	wait          func(context.Context, time.Duration) error
	commentGate   chan struct{}
	lastVideoAt   time.Time
}

type accountCommentPolicy struct {
	MaxVideosPerRun int
	VideoInterval   time.Duration
	SuccessCooldown time.Duration
	FailureCooldown time.Duration
	RiskCooldown    time.Duration
}

var defaultAccountCommentPolicy = accountCommentPolicy{
	MaxVideosPerRun: 3,
	VideoInterval:   10 * time.Second,
	SuccessCooldown: monitoring.MinCommentTaskInterval,
	FailureCooldown: 2 * time.Hour,
	RiskCooldown:    24 * time.Hour,
}

type accountVideoSync struct {
	item  ingest.WXFeedListItem
	state ingest.VideoCommentSyncState
	found bool
}

type commentSyncDecision struct {
	collect    bool
	reportSkip bool
	reason     string
}

type AccountMonitorError struct {
	Status        string
	VideoObjectID string
	Message       string
}

type AccountMonitorResult struct {
	Status    string
	Summary   ingest.Summary
	Attempted int
	Succeeded int
	Failed    int
	Skipped   int
	Errors    []AccountMonitorError
}

func NewAccountMonitor(ingestService *ingest.Service, accounts accountStore) (*AccountMonitor, error) {
	if ingestService == nil {
		return nil, errors.New("ingest service is not configured")
	}
	if accounts == nil {
		return nil, errors.New("account service is not configured")
	}
	return &AccountMonitor{
		ingestService: ingestService,
		accounts:      accounts,
		now:           time.Now,
		policy:        defaultAccountCommentPolicy,
		wait:          waitForAccountMonitor,
		commentGate:   make(chan struct{}, 1),
	}, nil
}

func (m *AccountMonitor) Run(ctx context.Context, query ingest.SummaryQuery, target monitoring.TaskTarget) (AccountMonitorResult, error) {
	if m == nil || m.ingestService == nil || m.accounts == nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed}, errors.New("account monitor is not configured")
	}
	normalized, err := monitoring.NormalizeTaskTarget(target)
	if err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed}, err
	}
	if normalized.TargetKind != monitoring.TargetKindAccount {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed}, errors.New("monitoring target is not an account")
	}
	account, err := m.accounts.Get(ctx, insight.AccountQuery{
		HubUserID: query.HubUserID, DeviceCount: query.DeviceCount, ContextSource: "jobs",
	}, normalized.AccountID)
	if err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed}, err
	}
	if account.IdentityStatus != insight.AccountIdentityVerified || strings.TrimSpace(account.ChannelUsername) == "" {
		return AccountMonitorResult{
			Status:  monitoring.RunStatusSkipped,
			Skipped: 1,
			Errors:  []AccountMonitorError{{Status: monitoring.RunStatusSkipped, Message: "账号身份不完整，请先确认账号 ID"}},
		}, nil
	}

	result := AccountMonitorResult{Status: monitoring.RunStatusSuccess}
	feed, summary, err := m.ingestService.ImportWXFeedListForAccount(ctx, query, account.ChannelUsername, "")
	if err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Errors: []AccountMonitorError{{Status: monitoring.RunStatusFailed, Message: err.Error()}}}, err
	}
	result.Summary = summary
	result.Attempted = len(feed.Items)
	if feed.PageCount > 0 && !feed.Complete {
		result.Status = monitoring.RunStatusPartial
		result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusPartial, Message: feed.Notice})
	}

	// Profile refresh is supplementary to the feed run. A failed profile lookup
	// must not discard the verified feed/video evidence already collected.
	if profiles, profileErr := m.ingestService.SearchWXFeedAccounts(ctx, query, "", account.ChannelUsername, ""); profileErr == nil && len(profiles) > 0 {
		profile := profiles[0]
		_, _ = m.accounts.UpsertObservedProfile(ctx, insight.AccountQuery{
			HubUserID: query.HubUserID, DeviceCount: query.DeviceCount, ContextSource: "jobs",
		}, insight.AccountObservedProfile{
			Platform:        insight.AccountPlatformWXChannels,
			ChannelUsername: profile.Username,
			ChannelName:     profile.Nickname,
			Signature:       profile.Signature,
			AvatarURL:       profile.HeadURL,
			Region:          profile.Region,
			Industry:        profile.Industry,
			Tags:            append([]string(nil), profile.Tags...),
			ObservedAt:      m.now().UTC(),
		})
	}

	if err := m.acquireCommentGate(ctx); err != nil {
		return result, err
	}
	defer m.releaseCommentGate()

	videoStates := make([]accountVideoSync, 0, len(feed.Items))
	for _, item := range feed.Items {
		state, found, syncErr := m.ingestService.GetVideoCommentSync(ctx, query, account.ID, item.ID)
		if syncErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusFailed, VideoObjectID: item.ID, Message: syncErr.Error()})
			continue
		}
		videoStates = append(videoStates, accountVideoSync{item: item, state: state, found: found})
	}

	now := m.now().UTC()
	if blocked, until := activeCommentRiskCooldown(videoStates, now, m.policy); blocked != nil {
		result.Skipped++
		result.Errors = append(result.Errors, AccountMonitorError{
			Status:        monitoring.RunStatusSkipped,
			VideoObjectID: blocked.item.ID,
			Message:       fmt.Sprintf("检测到平台频控，账号评论采集已熔断，将在 %s 后恢复", until.Format(time.RFC3339)),
		})
		result.Status = monitoring.RunStatusPartial
		return result, nil
	}
	if blocked, until := activeAccountCommentCooldown(videoStates, now, m.policy); blocked != nil {
		result.Skipped++
		result.Errors = append(result.Errors, AccountMonitorError{
			Status:        monitoring.RunStatusSkipped,
			VideoObjectID: blocked.item.ID,
			Message:       fmt.Sprintf("账号评论采集批次冷却中，将在 %s 后继续", until.Format(time.RFC3339)),
		})
		result.Status = monitoring.RunStatusPartial
		return result, nil
	}

	candidates := make([]accountVideoSync, 0, len(videoStates))
	for _, video := range videoStates {
		decision := evaluateCommentSync(now, normalized.CommentMode, video, m.policy)
		if !decision.collect {
			if decision.reportSkip {
				result.Skipped++
				result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusSkipped, VideoObjectID: video.item.ID, Message: decision.reason})
			}
			continue
		}
		if strings.TrimSpace(video.item.ObjectNonceID) == "" {
			result.Skipped++
			result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusSkipped, VideoObjectID: video.item.ID, Message: "视频缺少 objectNonceId，已保留视频数据并跳过评论采集"})
			_ = m.saveSyncState(ctx, query, account.ID, video.item, ingest.VideoCommentSyncStatusSkipped, "视频缺少 objectNonceId，已跳过评论采集", video.state)
			continue
		}
		candidates = append(candidates, video)
	}

	candidates, deferred := limitCommentCandidates(candidates, m.policy.MaxVideosPerRun)
	if deferred > 0 {
		result.Skipped++
		result.Errors = append(result.Errors, AccountMonitorError{
			Status:  monitoring.RunStatusSkipped,
			Message: fmt.Sprintf("本轮已达到评论采集安全上限 %d 个视频，其余 %d 个将在后续运行继续采集", len(candidates), deferred),
		})
	}

	for _, video := range candidates {
		if waitErr := m.waitForVideoSlot(ctx); waitErr != nil {
			return result, waitErr
		}
		item := video.item
		_, collectErr := m.ingestService.ImportWXCommentExportByAPIForAccount(ctx, query, account.ID, "", item.ID, item.ObjectNonceID, item.Title, item.ChannelName, "")
		m.lastVideoAt = m.now().UTC()
		if collectErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusFailed, VideoObjectID: item.ID, Message: collectErr.Error()})
			_ = m.saveSyncState(ctx, query, account.ID, item, ingest.VideoCommentSyncStatusFailed, collectErr.Error(), video.state)
			if isCommentRiskError(collectErr.Error()) {
				break
			}
			continue
		}
		result.Succeeded++
		if syncErr := m.saveSyncState(ctx, query, account.ID, item, ingest.VideoCommentSyncStatusSuccess, "", video.state); syncErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusFailed, VideoObjectID: item.ID, Message: syncErr.Error()})
		}
	}
	if result.Failed > 0 || result.Skipped > 0 {
		result.Status = monitoring.RunStatusPartial
	}
	return result, nil
}

func evaluateCommentSync(now time.Time, commentMode string, video accountVideoSync, policy accountCommentPolicy) commentSyncDecision {
	if !video.found {
		return commentSyncDecision{collect: true}
	}
	state := video.state
	item := video.item
	changed := state.LastCommentCount != item.CommentCount || strings.TrimSpace(state.VideoObjectNonceID) != strings.TrimSpace(item.ObjectNonceID)
	retryIncomplete := state.Status == ingest.VideoCommentSyncStatusFailed || state.Status == ingest.VideoCommentSyncStatusPartial || state.Status == ingest.VideoCommentSyncStatusSkipped
	needsBackfill := commentMode == monitoring.CommentModeFullBackfill && state.LastCollectedAt.IsZero()
	if !changed && !retryIncomplete && !needsBackfill {
		return commentSyncDecision{}
	}

	if state.Status == ingest.VideoCommentSyncStatusFailed {
		lastAttempt := state.LastObservedAt
		if !lastAttempt.IsZero() && now.Before(lastAttempt.Add(policy.FailureCooldown)) {
			return commentSyncDecision{reportSkip: true, reason: fmt.Sprintf("上次评论采集失败，已进入 %s 冷却", policy.FailureCooldown)}
		}
	}
	if !state.LastCollectedAt.IsZero() && now.Before(state.LastCollectedAt.Add(policy.SuccessCooldown)) {
		return commentSyncDecision{reportSkip: true, reason: fmt.Sprintf("评论有变化，但距离上次采集不足 %s，已延后", policy.SuccessCooldown)}
	}
	return commentSyncDecision{collect: true}
}

func limitCommentCandidates(candidates []accountVideoSync, maxVideos int) ([]accountVideoSync, int) {
	if maxVideos <= 0 {
		maxVideos = defaultAccountCommentPolicy.MaxVideosPerRun
	}
	if len(candidates) <= maxVideos {
		return candidates, 0
	}
	return candidates[:maxVideos], len(candidates) - maxVideos
}

func activeCommentRiskCooldown(videos []accountVideoSync, now time.Time, policy accountCommentPolicy) (*accountVideoSync, time.Time) {
	for index := range videos {
		state := videos[index].state
		if !videos[index].found || !isCommentRiskError(state.LastError) || state.LastObservedAt.IsZero() {
			continue
		}
		until := state.LastObservedAt.Add(policy.RiskCooldown)
		if now.Before(until) {
			return &videos[index], until
		}
	}
	return nil, time.Time{}
}

func activeAccountCommentCooldown(videos []accountVideoSync, now time.Time, policy accountCommentPolicy) (*accountVideoSync, time.Time) {
	var blocked *accountVideoSync
	var latestUntil time.Time
	for index := range videos {
		collectedAt := videos[index].state.LastCollectedAt
		if !videos[index].found || collectedAt.IsZero() {
			continue
		}
		until := collectedAt.Add(policy.SuccessCooldown)
		if now.Before(until) && until.After(latestUntil) {
			blocked = &videos[index]
			latestUntil = until
		}
	}
	return blocked, latestUntil
}

func isCommentRiskError(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	for _, signal := range []string{
		"http 403", "http 429", "status 403", "status 429", "forbidden", "too many requests",
		"rate limit", "risk control", "操作频繁", "请求频繁", "访问频繁", "风控", "安全验证", "需要验证",
	} {
		if strings.Contains(normalized, signal) {
			return true
		}
	}
	return false
}

func waitForAccountMonitor(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (m *AccountMonitor) acquireCommentGate(ctx context.Context) error {
	if m.commentGate == nil {
		return nil
	}
	select {
	case m.commentGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *AccountMonitor) releaseCommentGate() {
	if m.commentGate == nil {
		return
	}
	<-m.commentGate
}

func (m *AccountMonitor) waitForVideoSlot(ctx context.Context) error {
	if m.policy.VideoInterval <= 0 || m.lastVideoAt.IsZero() {
		return nil
	}
	remaining := m.policy.VideoInterval - m.now().UTC().Sub(m.lastVideoAt)
	if remaining <= 0 {
		return nil
	}
	wait := m.wait
	if wait == nil {
		wait = waitForAccountMonitor
	}
	return wait(ctx, remaining)
}

func (m *AccountMonitor) saveSyncState(ctx context.Context, query ingest.SummaryQuery, accountID string, item ingest.WXFeedListItem, status, lastError string, previous ingest.VideoCommentSyncState) error {
	collectedAt := previous.LastCollectedAt
	if status == ingest.VideoCommentSyncStatusSuccess {
		collectedAt = m.now().UTC()
	}
	return m.ingestService.UpsertVideoCommentSync(ctx, query, ingest.VideoCommentSyncState{
		HubUserID:          query.HubUserID,
		AccountID:          accountID,
		VideoObjectID:      item.ID,
		VideoObjectNonceID: item.ObjectNonceID,
		LastCommentCount:   item.CommentCount,
		LastObservedAt:     m.now().UTC(),
		LastCollectedAt:    collectedAt,
		Status:             status,
		LastError:          strings.TrimSpace(lastError),
	})
}

func formatAccountMonitorErrors(errorsList []AccountMonitorError) string {
	parts := make([]string, 0, len(errorsList))
	for _, item := range errorsList {
		message := strings.TrimSpace(item.Message)
		if message == "" {
			message = "账号目标未提供错误详情"
		}
		if strings.TrimSpace(item.VideoObjectID) != "" {
			parts = append(parts, fmt.Sprintf("视频[%s]: %s", item.VideoObjectID, message))
		} else {
			parts = append(parts, message)
		}
	}
	return strings.Join(parts, "; ")
}
