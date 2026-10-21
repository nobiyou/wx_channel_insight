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

// Account profiles change much less often than monitoring tasks run. Keep a
// successful observed profile for one day before making another upstream
// request; manual refreshes still use the account detail API directly.
const accountProfileFreshnessWindow = 24 * time.Hour

// RunTask is the separated account-task entry point. The legacy type keeps
// the pre-existing combined account monitor for old persisted tasks only.
func (m *AccountMonitor) RunTask(ctx context.Context, query ingest.SummaryQuery, target monitoring.TaskTarget, taskType string) (AccountMonitorResult, error) {
	switch strings.ToLower(strings.TrimSpace(taskType)) {
	case monitoring.TaskTypeAccountProfile:
		return m.RunAccountProfile(ctx, query, target)
	case monitoring.TaskTypeVideoList:
		return m.RunAccountVideoList(ctx, query, target)
	case monitoring.TaskTypeCommentCollect:
		return m.RunAccountComments(ctx, query, target)
	default:
		return m.Run(ctx, query, target)
	}
}

func (m *AccountMonitor) verifiedAccount(ctx context.Context, query ingest.SummaryQuery, target monitoring.TaskTarget) (insight.Account, error) {
	normalized, err := monitoring.NormalizeTaskTarget(target)
	if err != nil {
		return insight.Account{}, err
	}
	if normalized.TargetKind != monitoring.TargetKindAccount {
		return insight.Account{}, errors.New("monitoring target is not an account")
	}
	account, err := m.accounts.Get(ctx, insight.AccountQuery{
		HubUserID: query.HubUserID, DeviceCount: query.DeviceCount, ContextSource: "jobs",
	}, normalized.AccountID)
	if err != nil {
		return insight.Account{}, err
	}
	if account.IdentityStatus != insight.AccountIdentityVerified || strings.TrimSpace(account.ChannelUsername) == "" {
		return insight.Account{}, insight.ErrAccountIdentityIncomplete
	}
	return account, nil
}

func accountTaskSummary(query ingest.SummaryQuery, state string) ingest.Summary {
	return ingest.Summary{HubUserID: query.HubUserID, State: state, FreshnessState: "fresh"}
}

func isAccountProfileFresh(account insight.Account, now time.Time) bool {
	if account.ProfileObservedAt.IsZero() || !hasObservedAccountProfileEvidence(account) {
		return false
	}
	return account.ProfileObservedAt.Add(accountProfileFreshnessWindow).After(now.UTC())
}

func hasObservedAccountProfileEvidence(account insight.Account) bool {
	return strings.TrimSpace(account.Signature) != "" ||
		strings.TrimSpace(account.AvatarURL) != "" ||
		strings.TrimSpace(account.Region) != "" ||
		strings.TrimSpace(account.Industry) != "" ||
		len(account.Tags) > 0
}

// RunAccountProfile refreshes only the account profile endpoint and persists
// the observed fields. It never imports the account feed or comments.
func (m *AccountMonitor) RunAccountProfile(ctx context.Context, query ingest.SummaryQuery, target monitoring.TaskTarget) (AccountMonitorResult, error) {
	account, err := m.verifiedAccount(ctx, query, target)
	if err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusSkipped, Skipped: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusSkipped, Message: err.Error()}}}, nil
	}
	if isAccountProfileFresh(account, m.now()) {
		return AccountMonitorResult{
			Status:    monitoring.RunStatusSuccess,
			Summary:   accountTaskSummary(query, "profile_fresh"),
			Attempted: 1,
			Succeeded: 1,
		}, nil
	}
	profiles, err := m.ingestService.SearchWXFeedAccounts(ctx, query, "", account.ChannelUsername, "")
	if err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusFailed, Message: err.Error()}}}, err
	}
	if len(profiles) == 0 {
		err := errors.New("账号资料接口未返回匹配账号")
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusFailed, Message: err.Error()}}}, err
	}
	profile := profiles[0]
	if strings.TrimSpace(profile.Username) == "" {
		profile.Username = account.ChannelUsername
	}
	if _, err := m.accounts.UpsertObservedProfile(ctx, insight.AccountQuery{
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
	}); err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusFailed, Message: err.Error()}}}, err
	}
	return AccountMonitorResult{Status: monitoring.RunStatusSuccess, Summary: accountTaskSummary(query, "profile_synced"), Attempted: 1, Succeeded: 1}, nil
}

// RunAccountVideoList imports only the account feed list. Comment collection
// is intentionally owned by the separate comment task.
func (m *AccountMonitor) RunAccountVideoList(ctx context.Context, query ingest.SummaryQuery, target monitoring.TaskTarget) (AccountMonitorResult, error) {
	account, err := m.verifiedAccount(ctx, query, target)
	if err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusSkipped, Skipped: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusSkipped, Message: err.Error()}}}, nil
	}
	feed, summary, err := m.ingestService.ImportWXFeedListForAccount(ctx, query, account.ChannelUsername, "")
	if err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusFailed, Message: err.Error()}}}, err
	}
	if feed.PageCount > 0 && !feed.Complete {
		return AccountMonitorResult{Status: monitoring.RunStatusPartial, Summary: summary, Attempted: 1, Succeeded: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusPartial, Message: feed.Notice}}}, nil
	}
	return AccountMonitorResult{Status: monitoring.RunStatusSuccess, Summary: summary, Attempted: 1, Succeeded: 1}, nil
}

// RunAccountComments reads already persisted video observations. It does not
// call the feed-list endpoint, so a comment task can be scheduled independently.
func (m *AccountMonitor) RunAccountComments(ctx context.Context, query ingest.SummaryQuery, target monitoring.TaskTarget) (AccountMonitorResult, error) {
	account, err := m.verifiedAccount(ctx, query, target)
	if err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusSkipped, Skipped: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusSkipped, Message: err.Error()}}}, nil
	}
	observations, err := m.ingestService.ListLatestVideoMetricObservations(ctx, query)
	if err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusFailed, Message: err.Error()}}}, err
	}
	items := make([]ingest.WXFeedListItem, 0)
	for _, observation := range observations {
		if !strings.EqualFold(strings.TrimSpace(observation.ChannelUsername), strings.TrimSpace(account.ChannelUsername)) {
			continue
		}
		items = append(items, feedItemFromObservation(observation))
	}
	result, runErr := m.collectCommentItems(ctx, query, account.ID, items, target.CommentMode)
	result.Summary = accountTaskSummary(query, "comments_synced")
	return result, runErr
}

func feedItemFromObservation(observation ingest.VideoMetricObservation) ingest.WXFeedListItem {
	return ingest.WXFeedListItem{
		ID: observation.ObjectID, ObjectNonceID: observation.ObjectNonceID,
		Title: observation.Title, ChannelName: observation.ChannelName,
		ChannelUsername: observation.ChannelUsername, PublishedAt: observation.PublishedAt,
		CoverURL: observation.CoverURL, PlayCount: observation.PlayCount,
		LikeCount: observation.LikeCount, CommentCount: observation.CommentCount,
		FavCount: observation.FavCount, ForwardCount: observation.ForwardCount,
		Tags: append([]string(nil), observation.Tags...),
	}
}

func (m *AccountMonitor) collectCommentItems(ctx context.Context, query ingest.SummaryQuery, accountID string, items []ingest.WXFeedListItem, commentMode string) (AccountMonitorResult, error) {
	result := AccountMonitorResult{Status: monitoring.RunStatusSuccess, Attempted: len(items)}
	if err := m.acquireCommentGate(ctx); err != nil {
		return result, err
	}
	defer m.releaseCommentGate()

	videoStates := make([]accountVideoSync, 0, len(items))
	for _, item := range items {
		state, found, syncErr := m.ingestService.GetVideoCommentSync(ctx, query, accountID, item.ID)
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
		result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusSkipped, VideoObjectID: blocked.item.ID, Message: fmt.Sprintf("检测到平台频控，账号评论采集已熔断，将在 %s 后恢复", until.Format("2006-01-02 15:04:05"))})
		result.Status = monitoring.RunStatusPartial
		return result, nil
	}

	candidates := make([]accountVideoSync, 0, len(videoStates))
	for _, video := range videoStates {
		decision := evaluateCommentSync(now, commentMode, video, m.policy)
		if !decision.collect {
			if decision.reportSkip {
				result.Skipped++
				result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusSkipped, VideoObjectID: video.item.ID, Message: decision.reason})
			}
			continue
		}
		if strings.TrimSpace(video.item.ObjectNonceID) == "" {
			result.Skipped++
			result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusSkipped, VideoObjectID: video.item.ID, Message: "视频缺少 objectNonceId，已跳过评论采集"})
			_ = m.saveSyncState(ctx, query, accountID, video.item, ingest.VideoCommentSyncStatusSkipped, "视频缺少 objectNonceId，已跳过评论采集", video.state)
			continue
		}
		candidates = append(candidates, video)
	}
	candidates, deferred := limitCommentCandidates(candidates, m.policy.MaxVideosPerRun)
	if deferred > 0 {
		result.Skipped++
		result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusSkipped, Message: fmt.Sprintf("本轮已达到评论采集安全上限 %d 个视频，其余 %d 个将在后续运行继续采集", len(candidates), deferred)})
	}
	for _, video := range candidates {
		if err := m.waitForVideoSlot(ctx); err != nil {
			return result, err
		}
		item := video.item
		_, collectErr := m.ingestService.ImportWXCommentExportByAPIForAccount(ctx, query, accountID, "", item.ID, item.ObjectNonceID, item.Title, item.ChannelName, "")
		m.lastVideoAt = m.now().UTC()
		if collectErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusFailed, VideoObjectID: item.ID, Message: collectErr.Error()})
			_ = m.saveSyncState(ctx, query, accountID, item, ingest.VideoCommentSyncStatusFailed, collectErr.Error(), video.state)
			if isCommentRiskError(collectErr.Error()) {
				break
			}
			continue
		}
		result.Succeeded++
		if syncErr := m.saveSyncState(ctx, query, accountID, item, ingest.VideoCommentSyncStatusSuccess, "", video.state); syncErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, AccountMonitorError{Status: monitoring.RunStatusFailed, VideoObjectID: item.ID, Message: syncErr.Error()})
		}
	}
	if result.Failed > 0 || result.Skipped > 0 {
		result.Status = monitoring.RunStatusPartial
	}
	return result, nil
}

// RefreshVideoComments performs the single-video action without refreshing
// the account feed list. It still uses the same serialized comment gate.
func (m *AccountMonitor) RefreshVideoComments(ctx context.Context, query ingest.SummaryQuery, accountID string, item ingest.WXFeedListItem) (AccountMonitorResult, error) {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.ObjectNonceID) == "" {
		return AccountMonitorResult{Status: monitoring.RunStatusSkipped, Skipped: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusSkipped, VideoObjectID: item.ID, Message: "视频缺少评论接口所需的 objectNonceId"}}}, nil
	}
	if _, err := m.accounts.Get(ctx, insight.AccountQuery{HubUserID: query.HubUserID, DeviceCount: query.DeviceCount, ContextSource: "api"}, accountID); err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusFailed, VideoObjectID: item.ID, Message: err.Error()}}}, err
	}
	if err := m.acquireCommentGate(ctx); err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1}, err
	}
	defer m.releaseCommentGate()
	state, found, syncErr := m.ingestService.GetVideoCommentSync(ctx, query, accountID, item.ID)
	if syncErr != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusFailed, VideoObjectID: item.ID, Message: syncErr.Error()}}}, syncErr
	}
	video := accountVideoSync{item: item, state: state, found: found}
	decision := evaluateSingleCommentSync(m.now().UTC(), video, m.policy)
	if !decision.collect {
		return AccountMonitorResult{
			Status: monitoring.RunStatusSkipped, Summary: accountTaskSummary(query, "comment_skipped"), Attempted: 1, Skipped: 1,
			Errors: []AccountMonitorError{{Status: monitoring.RunStatusSkipped, VideoObjectID: item.ID, Message: decision.reason}},
		}, nil
	}
	if err := m.waitForVideoSlot(ctx); err != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1}, err
	}
	_, err := m.ingestService.ImportWXCommentExportByAPIForAccount(ctx, query, accountID, "", item.ID, item.ObjectNonceID, item.Title, item.ChannelName, "")
	m.lastVideoAt = m.now().UTC()
	if err != nil {
		_ = m.saveSyncState(ctx, query, accountID, item, ingest.VideoCommentSyncStatusFailed, err.Error(), state)
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusFailed, VideoObjectID: item.ID, Message: err.Error()}}}, err
	}
	if syncErr := m.saveSyncState(ctx, query, accountID, item, ingest.VideoCommentSyncStatusSuccess, "", state); syncErr != nil {
		return AccountMonitorResult{Status: monitoring.RunStatusFailed, Failed: 1, Errors: []AccountMonitorError{{Status: monitoring.RunStatusFailed, VideoObjectID: item.ID, Message: syncErr.Error()}}}, syncErr
	}
	return AccountMonitorResult{Status: monitoring.RunStatusSuccess, Summary: accountTaskSummary(query, "comment_synced"), Attempted: 1, Succeeded: 1}, nil
}

func evaluateSingleCommentSync(now time.Time, video accountVideoSync, policy accountCommentPolicy) commentSyncDecision {
	if blocked, until := activeCommentRiskCooldown([]accountVideoSync{video}, now, policy); blocked != nil {
		return commentSyncDecision{
			reportSkip: true,
			reason:     fmt.Sprintf("检测到平台频控，该视频评论采集已熔断，将在 %s 后恢复", until.Format(time.RFC3339)),
		}
	}
	decision := evaluateCommentSync(now, monitoring.CommentModeIncremental, video, policy)
	if !decision.collect && strings.TrimSpace(decision.reason) == "" {
		decision.reportSkip = true
		decision.reason = "评论指标未发生变化，已跳过重复采集"
	}
	return decision
}
