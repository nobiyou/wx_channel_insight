package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type cachedCommentBindingAssessment struct {
	size       int64
	modifiedAt time.Time
	validated  bool
	assessment SourceBindingAssessment
}

type Service struct {
	repo             Repository
	defaultHubUserID string
	now              func() time.Time
	wxFeedListClient WXFeedListClient
	wxCommentClient  WXCommentExportClient
	commentDataRoot  string
	enableLiveChecks bool
	rawCommentRepo   RawCommentRepository
	commentSyncRepo  VideoCommentSyncRepository
	eventObserver    EventObserver
	commentCacheMu   sync.Mutex
	commentCache     map[string]cachedCommentBindingAssessment
}

const (
	recentSummaryEventsReadLimit = 9
	recentSummaryRunsReadLimit   = 6
	recentOperatorRunsReadLimit  = 100
)

var ErrAccountMetricProviderUnavailable = errors.New("bounded account metric provider is not configured")

type recentOperatorRunsRepository interface {
	ListOperatorRunsLimited(context.Context, string, int) ([]OperatorRun, error)
}

// EventObserver receives normalized events after they are persisted. It is a
// supplementary projection hook; observer errors never invalidate the
// already-persisted ingest evidence.
type EventObserver func(context.Context, SummaryQuery, []Event) error

type ServiceConfig struct {
	DefaultHubUserID           string
	Now                        func() time.Time
	WXFeedListClient           WXFeedListClient
	WXCommentClient            WXCommentExportClient
	CommentDataRoot            string
	EnableLiveChecks           bool
	RawCommentRepository       RawCommentRepository
	VideoCommentSyncRepository VideoCommentSyncRepository
	EventObserver              EventObserver
}

type SummaryQuery struct {
	HubUserID   string
	DeviceCount int
	ObjectIDs   []string
}

type mockBatchProfile struct {
	videoTitle            string
	videoChannelName      string
	videoSignal           string
	videoTags             []string
	commentTitle          string
	commentKeyword        string
	commentSignal         string
	commentRepresentative string
	targetAudience        string
	commentKeywords       []string
	topicKeyword          string
	topicCluster          string
	topicSignal           string
	topicPromptAngle      string
	topicKeywords         []string
}

var mockBatchProfiles = []mockBatchProfile{
	{
		videoTitle:            "世界模型路线争议为什么还在升温，评论区开始追问落地场景",
		videoChannelName:      "Insight Mock Radar",
		videoSignal:           "高争议 + 高问题密度",
		videoTags:             []string{"AI", "世界模型", "路线争议"},
		commentTitle:          "评论区代表性问题",
		commentKeyword:        "世界模型怎么真正落地",
		commentSignal:         "高追问",
		commentRepresentative: "别只讲路线，能不能讲讲世界模型到底能替代现在哪类工作流？",
		targetAudience:        "泛科技内容受众",
		commentKeywords:       []string{"世界模型", "落地", "工作流"},
		topicKeyword:          "世界模型",
		topicCluster:          "AI 认知路线",
		topicSignal:           "持续升温",
		topicPromptAngle:      "从评论区的实际追问切入，而不是重复结论口水战",
		topicKeywords:         []string{"AGI", "推理", "路线之争"},
	},
	{
		videoTitle:            "直播标题模板为什么又火了一轮，评论区都在问客单价怎么分层",
		videoChannelName:      "操盘手日报",
		videoSignal:           "强模板 + 强追问",
		videoTags:             []string{"直播电商", "标题模板", "转化"},
		commentTitle:          "评论区代表性问题",
		commentKeyword:        "标题模板到底怎么按客单价分层",
		commentSignal:         "求模板",
		commentRepresentative: "这种标题到底适合高客单还是低客单？有没有分类模板？",
		targetAudience:        "直播电商操盘手",
		commentKeywords:       []string{"标题模板", "高客单", "低客单"},
		topicKeyword:          "直播标题模板",
		topicCluster:          "直播复盘模板",
		topicSignal:           "评论爆发",
		topicPromptAngle:      "从能不能直接套用切到按客单价分层的模板拆解",
		topicKeywords:         []string{"标题模板", "直播复盘", "转化"},
	},
	{
		videoTitle:            "门店团购预算表为什么又被翻出来，老板更想看回本周期",
		videoChannelName:      "增长实验室",
		videoSignal:           "经营细节 + 数据感",
		videoTags:             []string{"本地生活", "团购", "预算"},
		commentTitle:          "评论区代表性问题",
		commentKeyword:        "门店团购预算和回本周期怎么算",
		commentSignal:         "理性关注",
		commentRepresentative: "预算花了多少、投了几天、转化成本多少才是重点。",
		targetAudience:        "本地生活门店老板",
		commentKeywords:       []string{"预算", "回本周期", "团购"},
		topicKeyword:          "门店团购预算",
		topicCluster:          "门店增长",
		topicSignal:           "经营关注",
		topicPromptAngle:      "直接晒预算与回本周期，比空讲方法更容易转化",
		topicKeywords:         []string{"本地生活", "预算", "ROI"},
	},
}

func NewService(repo Repository, config ServiceConfig) *Service {
	now := config.Now
	if now == nil {
		now = time.Now
	}

	return &Service{
		repo:             repo,
		defaultHubUserID: config.DefaultHubUserID,
		now:              now,
		wxFeedListClient: config.WXFeedListClient,
		wxCommentClient:  config.WXCommentClient,
		commentDataRoot:  strings.TrimSpace(config.CommentDataRoot),
		enableLiveChecks: config.EnableLiveChecks,
		rawCommentRepo:   config.RawCommentRepository,
		commentSyncRepo:  config.VideoCommentSyncRepository,
		eventObserver:    config.EventObserver,
		commentCache:     make(map[string]cachedCommentBindingAssessment),
	}
}

func (s *Service) SetEventObserver(observer EventObserver) {
	if s == nil {
		return
	}
	s.eventObserver = observer
}

func (s *Service) observeEvents(ctx context.Context, query SummaryQuery, events []Event) {
	if s == nil || s.eventObserver == nil || len(events) == 0 {
		return
	}
	_ = s.eventObserver(ctx, query, append([]Event(nil), events...))
}

// AssessBinding applies runtime-local path and endpoint resolution before the
// live health probe. Persisted bindings can outlive the process that created
// them, so host-local addresses and paths must not become deployment state.
func (s *Service) AssessBinding(source string, binding SourceBinding) SourceBindingAssessment {
	return assessSourceBinding(source, s.resolveRuntimeSourceBinding(binding), s.commentDataRoot)
}

func (s *Service) resolveRuntimeSourceBinding(binding SourceBinding) SourceBinding {
	source := canonicalSource(binding.Source)
	if source == RunSourceWXFeedList || source == RunSourceWXVideoSearch {
		binding.BaseURL = s.resolveWXFeedBaseURL(binding.BaseURL)
		if strings.TrimSpace(binding.BaseURL) == "" {
			binding.BaseURL = strings.TrimSpace(s.wxFeedListClient.BaseURL)
		}
	}
	return binding
}

func (s *Service) resolveWXFeedBaseURL(baseURL string) string {
	requested := strings.TrimSpace(baseURL)
	configured := strings.TrimSpace(s.wxFeedListClient.BaseURL)
	if requested == "" || configured == "" {
		return requested
	}
	if (isLoopbackWXFeedListBaseURL(requested) && isHostDockerInternalWXFeedListBaseURL(configured)) ||
		(isHostDockerInternalWXFeedListBaseURL(requested) && isLoopbackWXFeedListBaseURL(configured)) {
		return configured
	}
	return requested
}

func (s *Service) persistedWXFeedBaseURL(baseURL string) string {
	resolved := strings.TrimSpace(s.resolveWXFeedBaseURL(baseURL))
	if resolved == "" {
		return ""
	}
	configured := strings.TrimSpace(s.wxFeedListClient.BaseURL)
	if isHostDockerInternalWXFeedListBaseURL(resolved) ||
		(configured != "" && sameWXFeedListBaseURL(resolved, configured) &&
			sameWXFeedListBaseURL(resolved, defaultLocalWXFeedListBaseURL)) {
		return ""
	}
	return normalizeWXFeedListBaseURL(resolved)
}

func (s *Service) resolveWXCommentExportPath(path string) (wxCommentExportPathResolution, error) {
	return resolveWXCommentExportPathWithRoot(path, s.commentDataRoot)
}

func (s *Service) GetSummary(ctx context.Context, query SummaryQuery) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	eventFacts, err := s.loadEventSummaryFacts(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	runs, err := s.listSummaryRuns(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	return s.buildSummaryFromEventFactsWithBindingStates(ctx, query, hubUserID, eventFacts, runs, s.now(), true)
}

// GetSummaryReadOnly serves page reads from persisted ingest data without
// probing the external wx_channel runtime. Live readiness belongs to source
// management and collection execution; dashboard reads must stay local.
func (s *Service) GetSummaryReadOnly(ctx context.Context, query SummaryQuery) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	eventFacts, err := s.loadEventSummaryFacts(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	runs, err := s.listSummaryRuns(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	return s.buildSummaryFromEventFactsWithBindingStates(ctx, query, hubUserID, eventFacts, runs, s.now(), false)
}

func (s *Service) loadEventSummaryFacts(ctx context.Context, hubUserID string) (EventSummaryFacts, error) {
	if provider, ok := s.repo.(EventSummaryFactsProvider); ok {
		facts, err := provider.GetEventSummaryFacts(ctx, hubUserID, recentSummaryEventsReadLimit)
		if err != nil {
			return EventSummaryFacts{}, err
		}
		return normalizeEventSummaryFacts(facts, recentSummaryEventsReadLimit), nil
	}
	events, err := s.repo.ListEvents(ctx, hubUserID)
	if err != nil {
		return EventSummaryFacts{}, err
	}
	return summarizeEventFacts(events, recentSummaryEventsReadLimit), nil
}

func (s *Service) listSummaryRuns(ctx context.Context, hubUserID string) ([]Run, error) {
	if provider, ok := s.repo.(SummaryRunsProvider); ok {
		return provider.ListSummaryRuns(ctx, hubUserID, recentSummaryRunsReadLimit)
	}
	return s.repo.ListRuns(ctx, hubUserID)
}

func (s *Service) ListEvents(ctx context.Context, query SummaryQuery) ([]Event, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	events, err := s.repo.ListEvents(ctx, hubUserID)
	if err != nil {
		return nil, err
	}
	results := normalizeEvents(events)
	sort.Slice(results, func(i, j int) bool {
		if results[i].CollectedAt.Equal(results[j].CollectedAt) {
			return results[i].ID > results[j].ID
		}
		return results[i].CollectedAt.After(results[j].CollectedAt)
	})
	return results, nil
}

func (s *Service) ListVideoMetricObservations(ctx context.Context, query SummaryQuery) ([]VideoMetricObservation, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	observations, err := s.repo.ListVideoMetricObservations(ctx, hubUserID)
	if err != nil {
		return nil, err
	}
	return append([]VideoMetricObservation(nil), observations...), nil
}

func (s *Service) ListVideoHistory(ctx context.Context, query SummaryQuery, objectID string) ([]Event, []VideoMetricObservation, error) {
	provider, ok := s.repo.(VideoHistoryProvider)
	if !ok {
		return nil, nil, errors.New("bounded video history provider is not configured")
	}
	return provider.ListVideoHistory(ctx, s.resolveHubUserID(query.HubUserID), strings.TrimSpace(objectID))
}

// ListLatestVideoMetricObservations returns one current observation per video.
// Repositories with a database-backed latest-row query can implement the
// optional method below; other stores use the same deterministic reduction in
// memory while preserving the full-history method for analysis and history.
func (s *Service) ListLatestVideoMetricObservations(ctx context.Context, query SummaryQuery) ([]VideoMetricObservation, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	if provider, ok := s.repo.(interface {
		ListLatestVideoMetricObservations(context.Context, string) ([]VideoMetricObservation, error)
	}); ok {
		observations, err := provider.ListLatestVideoMetricObservations(ctx, hubUserID)
		if err != nil {
			return nil, err
		}
		return append([]VideoMetricObservation(nil), observations...), nil
	}

	observations, err := s.repo.ListVideoMetricObservations(ctx, hubUserID)
	if err != nil {
		return nil, err
	}
	latest := map[string]VideoMetricObservation{}
	for _, observation := range observations {
		key := latestVideoMetricObservationKey(observation)
		current, found := latest[key]
		if !found || observation.ObservedAt.After(current.ObservedAt) ||
			(observation.ObservedAt.Equal(current.ObservedAt) && observation.ID < current.ID) {
			latest[key] = observation
		}
	}
	result := make([]VideoMetricObservation, 0, len(latest))
	for _, observation := range latest {
		result = append(result, observation)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ObservedAt.Equal(result[j].ObservedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].ObservedAt.After(result[j].ObservedAt)
	})
	return result, nil
}

func latestVideoMetricObservationKey(observation VideoMetricObservation) string {
	for _, value := range []string{observation.ObjectID, observation.ObjectNonceID, observation.ID} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// FetchWXVideoProfile loads the current profile for one video. The returned
// play URL is short-lived and must remain a request-time value; it is never
// written to ingest events or metric history.
func (s *Service) FetchWXVideoProfile(ctx context.Context, objectID, nonceID string) (WXFeedListItem, error) {
	if s == nil {
		return WXFeedListItem{}, errors.New("ingest service is not configured")
	}
	return s.wxFeedListClient.FetchVideoProfile(ctx, objectID, nonceID)
}

// RefreshWXVideoMetrics saves an actual profile response as a new observation.
// It never turns absent source counters into zero or creates a source binding.
func (s *Service) RefreshWXVideoMetrics(ctx context.Context, query SummaryQuery, objectID, nonceID string) (Summary, error) {
	startedAt := s.now()
	item, err := s.FetchWXVideoProfile(ctx, objectID, nonceID)
	if err != nil {
		return Summary{}, err
	}
	if strings.TrimSpace(item.ID) != strings.TrimSpace(objectID) {
		return Summary{}, errors.New("video profile identity does not match requested video")
	}
	hubUserID := s.resolveHubUserID(query.HubUserID)
	event := BuildEventFromWXManualShare(hubUserID, "wx-channel-video-profile", item, startedAt)
	event.ID = "wx-video-profile-" + item.ID
	event.Source = RunSourceWXVideoProfile
	return s.importEventsWithSource(ctx, query, RunSourceWXVideoProfile, startedAt, []Event{event}, nil)
}

// ImportWXVideoShare resolves one user-supplied share URL, persists its
// metadata and first metric observation, and saves the URL as a reusable
// manual-share source binding for the default monitoring task.
func (s *Service) ImportWXVideoShare(ctx context.Context, query SummaryQuery, baseURL, shareURL, clientID string) (WXFeedListItem, Summary, error) {
	if s == nil {
		return WXFeedListItem{}, Summary{}, errors.New("ingest service is not configured")
	}
	if strings.TrimSpace(clientID) != "" {
		s.SetWXFeedPreferClientID(clientID)
		defer s.SetWXFeedPreferClientID("")
	}

	client := s.wxFeedListClient
	baseURL = s.resolveWXFeedBaseURL(baseURL)
	if strings.TrimSpace(baseURL) != "" {
		client = clientForExplicitWXFeedBaseURL(client, baseURL)
	}
	normalizedShareURL, err := normalizeWXSharedFeedURL(shareURL)
	if err != nil {
		return WXFeedListItem{}, Summary{}, err
	}
	item, err := client.ResolveVideoShare(ctx, normalizedShareURL)
	if err != nil {
		return WXFeedListItem{}, Summary{}, err
	}
	if strings.TrimSpace(item.ID) == "" {
		return WXFeedListItem{}, Summary{}, errors.New("分享链接未返回视频标识")
	}

	startedAt := s.now()
	hubUserID := s.resolveHubUserID(query.HubUserID)
	machineID := "wx-channel-manual-share"
	if query.DeviceCount > 0 {
		machineID = fmt.Sprintf("wx-channel-manual-share-%02d", query.DeviceCount)
	}
	event := BuildEventFromWXManualShare(hubUserID, machineID, item, startedAt)
	binding := SourceBinding{
		Source:        RunSourceWXManualShare,
		Origin:        BindingOriginManual,
		BaseURL:       s.persistedWXFeedBaseURL(bindingBaseURLForClient(client, baseURL)),
		ShareURL:      normalizedShareURL,
		ObjectID:      item.ID,
		ObjectNonceID: item.ObjectNonceID,
		DeviceCount:   query.DeviceCount,
		UpdatedAt:     startedAt,
	}
	summary, err := s.importEventsWithSource(ctx, SummaryQuery{
		HubUserID:   hubUserID,
		DeviceCount: query.DeviceCount,
	}, RunSourceWXManualShare, startedAt, []Event{event}, &binding)
	if err != nil {
		return WXFeedListItem{}, Summary{}, err
	}
	return item, summary, nil
}

// RefreshWXVideoShareBinding resolves a saved share URL again and appends a
// new metric observation. The short-lived media URL is intentionally kept out
// of the saved binding and is only available from the request-time resolver.
func (s *Service) RefreshWXVideoShareBinding(ctx context.Context, query SummaryQuery, binding SourceBinding) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()
	normalizedShareURL, err := normalizeWXSharedFeedURL(binding.ShareURL)
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXManualShare, startedAt, err))
		return Summary{}, err
	}

	client := s.wxFeedListClient
	baseURL := s.resolveWXFeedBaseURL(binding.BaseURL)
	if strings.TrimSpace(baseURL) != "" {
		client = clientForExplicitWXFeedBaseURL(client, baseURL)
	}
	item, err := client.ResolveVideoShare(ctx, normalizedShareURL)
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXManualShare, startedAt, err))
		return Summary{}, err
	}
	if strings.TrimSpace(item.ID) == "" {
		err := errors.New("分享链接未返回视频标识")
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXManualShare, startedAt, err))
		return Summary{}, err
	}

	nextBinding := binding
	nextBinding.Source = RunSourceWXManualShare
	nextBinding.BaseURL = s.persistedWXFeedBaseURL(bindingBaseURLForClient(client, baseURL))
	nextBinding.ShareURL = normalizedShareURL
	nextBinding.ObjectID = item.ID
	nextBinding.ObjectNonceID = item.ObjectNonceID
	if nextBinding.DeviceCount <= 0 {
		nextBinding.DeviceCount = query.DeviceCount
	}
	nextBinding.UpdatedAt = startedAt
	machineID := "wx-channel-manual-share"
	if query.DeviceCount > 0 {
		machineID = fmt.Sprintf("wx-channel-manual-share-%02d", query.DeviceCount)
	}
	event := BuildEventFromWXManualShare(hubUserID, machineID, item, startedAt)
	summary, err := s.importEventsWithSource(ctx, SummaryQuery{
		HubUserID:   hubUserID,
		DeviceCount: query.DeviceCount,
	}, RunSourceWXManualShare, startedAt, []Event{event}, &nextBinding)
	if err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func (s *Service) ListBindings(ctx context.Context, query SummaryQuery) ([]SourceBinding, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	bindings, err := s.repo.ListBindings(ctx, hubUserID)
	if err != nil {
		return nil, err
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].UpdatedAt.Equal(bindings[j].UpdatedAt) {
			return bindings[i].Source < bindings[j].Source
		}
		return bindings[i].UpdatedAt.After(bindings[j].UpdatedAt)
	})
	return bindings, nil
}

// DeleteBinding removes only the saved source binding. Ingest events, metric
// observations, comments and read models are intentionally left untouched.
func (s *Service) DeleteBinding(ctx context.Context, query SummaryQuery, source, targetKey string) error {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	source = canonicalSource(source)
	targetKey = strings.TrimSpace(targetKey)
	if source == "" {
		return errors.New("source binding source is required")
	}
	if targetKey == "" {
		return errors.New("source binding target key is required")
	}
	return s.repo.DeleteBinding(ctx, hubUserID, source, targetKey)
}

func (s *Service) ListBindingStates(ctx context.Context, query SummaryQuery) ([]SourceBindingState, error) {
	return s.listBindingStates(ctx, query, true)
}

// ListBindingStatesReadOnly validates saved paths and binding fields locally,
// but deliberately skips wx_channel endpoint health probes.
func (s *Service) ListBindingStatesReadOnly(ctx context.Context, query SummaryQuery) ([]SourceBindingState, error) {
	return s.listBindingStates(ctx, query, false)
}

func (s *Service) listBindingStates(ctx context.Context, query SummaryQuery, includeLiveChecks bool) ([]SourceBindingState, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	runs, err := s.listSummaryRuns(ctx, hubUserID)
	if err != nil {
		return nil, err
	}
	return s.listBindingStatesWithRuns(ctx, hubUserID, runs, includeLiveChecks)
}

func (s *Service) listBindingStatesWithRuns(ctx context.Context, hubUserID string, runs []Run, includeLiveChecks bool) ([]SourceBindingState, error) {
	eventFacts, err := s.loadEventSummaryFacts(ctx, hubUserID)
	if err != nil {
		return nil, err
	}
	return s.listBindingStatesWithRunsAndFacts(ctx, hubUserID, runs, eventFacts, includeLiveChecks)
}

func (s *Service) listBindingStatesWithRunsAndFacts(ctx context.Context, hubUserID string, runs []Run, eventFacts EventSummaryFacts, includeLiveChecks bool) ([]SourceBindingState, error) {
	bindings, err := s.repo.ListBindings(ctx, hubUserID)
	if err != nil {
		return nil, err
	}

	normalizedRuns := normalizeRuns(runs)
	healthCache := bindingHealthCache{}
	states := make([]SourceBindingState, 0, len(bindings))
	for _, binding := range bindings {
		source := canonicalSource(binding.Source)
		var assessment SourceBindingAssessment
		if includeLiveChecks {
			assessment = s.assessBindingWithCache(ctx, source, binding, healthCache)
		} else {
			assessment = s.assessBindingForRead(source, binding)
		}
		resolvedPath := binding.Path
		if assessment.PersistPath != "" {
			resolvedPath = assessment.PersistPath
		}
		resolvedBaseURL := binding.BaseURL
		if assessment.ResolvedBaseURL != "" {
			resolvedBaseURL = assessment.ResolvedBaseURL
		}
		state := SourceBindingState{
			Source:        source,
			Origin:        NormalizeBindingOrigin(binding.Origin),
			TargetKey:     BindingTargetKey(binding),
			Path:          resolvedPath,
			BaseURL:       resolvedBaseURL,
			ShareURL:      binding.ShareURL,
			ObjectID:      binding.ObjectID,
			ObjectNonceID: binding.ObjectNonceID,
			Username:      binding.Username,
			Keyword:       binding.Keyword,
			DeviceCount:   binding.DeviceCount,
			UpdatedAt:     binding.UpdatedAt,
			Runnable:      assessment.Runnable,
			Target:        buildSourceBindingTarget(binding, assessment, eventFacts),
			Diagnostic:    cloneSourceBindingDiagnostic(assessment.Diagnostic),
		}
		latestRun, ok := findLatestRunBySource(normalizedRuns, source)
		if !ok {
			if !assessment.Runnable {
				state.RetryReason = firstNonEmpty(assessment.Detail, assessment.Reason)
			} else {
				state.RetryReason = "还没有运行记录"
			}
			states = append(states, state)
			continue
		}
		state.LastRunStatus = latestRun.Status
		state.LastRunAt = latestRun.FinishedAt
		state.LastError = latestRun.Message
		state.LatestBatchID = latestRun.BatchID
		for _, run := range normalizedRuns {
			if run.Source != source {
				continue
			}
			switch run.Status {
			case "success":
				if state.LastSuccessAt.IsZero() {
					state.LastSuccessAt = run.FinishedAt
					if state.LatestBatchID == "" {
						state.LatestBatchID = run.BatchID
					}
				}
			case "failed":
				if state.LastFailureAt.IsZero() {
					state.LastFailureAt = run.FinishedAt
					if state.LastError == "" {
						state.LastError = run.Message
					}
				}
			}
		}
		if latestRun.Status == "success" {
			state.LastError = ""
		}
		if !assessment.Runnable {
			displayReason := firstNonEmpty(assessment.Detail, assessment.Reason)
			if runMessageLooksLikeBindingFailure(state.LastError, source) {
				state.LastError = displayReason
			}
			if state.LastError == "" {
				state.LastError = displayReason
			}
			state.RetryReason = displayReason
		} else if latestRun.Status == "failed" {
			if source == RunSourceWXCommentExport && isWXCommentExportContextMismatchMessage(state.LastError) {
				state.Runnable = false
				state.CanRetry = false
				state.Diagnostic = buildWXCommentExportContextMismatchDiagnostic(s.wxCommentClient.BaseURL, state.Diagnostic)
				state.RetryReason = buildWXCommentExportContextMismatchDetail(state.Diagnostic)
				state.LastError = state.RetryReason
			} else {
				state.CanRetry = true
			}
		}
		states = append(states, state)
	}

	sort.Slice(states, func(i, j int) bool {
		if states[i].UpdatedAt.Equal(states[j].UpdatedAt) {
			return states[i].Source < states[j].Source
		}
		return states[i].UpdatedAt.After(states[j].UpdatedAt)
	})
	return states, nil
}

func (s *Service) ListOperatorRuns(ctx context.Context, query SummaryQuery) ([]OperatorRun, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	runs, err := s.listOperatorRunsForRead(ctx, hubUserID)
	if err != nil {
		return nil, err
	}
	return normalizeOperatorRunsForRead(runs), nil
}

func (s *Service) listOperatorRunsForRead(ctx context.Context, hubUserID string) ([]OperatorRun, error) {
	if provider, ok := s.repo.(recentOperatorRunsRepository); ok {
		return provider.ListOperatorRunsLimited(ctx, hubUserID, recentOperatorRunsReadLimit)
	}
	return s.repo.ListOperatorRuns(ctx, hubUserID)
}

func (s *Service) loadSourceAutoRunFacts(ctx context.Context, hubUserID string) (map[string]autoRunFact, error) {
	if provider, ok := s.repo.(SourceAutoRunFactsProvider); ok {
		items, err := provider.GetSourceAutoRunFacts(ctx, hubUserID)
		if err != nil {
			return nil, err
		}
		return buildAutoRunFactsFromSourceFacts(items), nil
	}
	runs, err := s.listOperatorRunsForRead(ctx, hubUserID)
	if err != nil {
		return nil, err
	}
	return buildAutoRunFactsBySource(runs), nil
}

func normalizeOperatorRunsForRead(runs []OperatorRun) []OperatorRun {
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].FinishedAt.Equal(runs[j].FinishedAt) {
			return runs[i].ID > runs[j].ID
		}
		return runs[i].FinishedAt.After(runs[j].FinishedAt)
	})
	if len(runs) > recentOperatorRunsReadLimit {
		return runs[:recentOperatorRunsReadLimit]
	}
	return runs
}

func (s *Service) AppendOperatorRun(ctx context.Context, query SummaryQuery, run OperatorRun) error {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	if run.ID == "" {
		return errors.New("operator run id is required")
	}
	if run.Status == "" {
		return errors.New("operator run status is required")
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = s.now()
	}
	if run.FinishedAt.IsZero() {
		run.FinishedAt = run.StartedAt
	}
	return s.repo.AppendOperatorRun(ctx, hubUserID, run)
}

func (s *Service) AppendMockBatch(ctx context.Context, query SummaryQuery) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	existing, err := s.repo.ListEvents(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}

	startedAt := s.now()
	batchIndex := nextBatchIndex(existing)
	events := buildMockBatch(hubUserID, query.DeviceCount, batchIndex, startedAt)
	if err := s.repo.AppendEvents(ctx, hubUserID, events); err != nil {
		return Summary{}, err
	}
	s.observeEvents(ctx, SummaryQuery{HubUserID: hubUserID, DeviceCount: query.DeviceCount}, events)
	if err := s.repo.AppendRun(ctx, hubUserID, buildSuccessfulRun("mock_batch", startedAt, s.now(), events)); err != nil {
		return Summary{}, err
	}

	combined := append(append([]Event(nil), existing...), events...)
	runs, err := s.repo.ListRuns(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	return s.buildSummaryWithBindings(ctx, query, hubUserID, combined, runs, startedAt)
}

func (s *Service) ImportEvents(ctx context.Context, query SummaryQuery, events []Event) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()
	if len(events) == 0 {
		err := errors.New("events are required")
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun("manual_import", startedAt, err))
		return Summary{}, err
	}

	importBatchID := buildImportBatchID(startedAt)
	normalized := make([]Event, 0, len(events))
	for index, event := range events {
		next := event
		if next.HubUserID == "" {
			next.HubUserID = hubUserID
		}
		if next.HubUserID != hubUserID {
			err := fmt.Errorf("event %d hubUserID mismatch", index)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun("manual_import", startedAt, err))
			return Summary{}, err
		}
		if next.ID == "" {
			err := fmt.Errorf("event %d id is required", index)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun("manual_import", startedAt, err))
			return Summary{}, err
		}
		if next.EventType == "" {
			err := fmt.Errorf("event %d eventType is required", index)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun("manual_import", startedAt, err))
			return Summary{}, err
		}
		if next.Source == "" {
			err := fmt.Errorf("event %d source is required", index)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun("manual_import", startedAt, err))
			return Summary{}, err
		}
		if next.CollectedAt.IsZero() {
			next.CollectedAt = s.now()
		}
		if next.BatchID == "" {
			next.BatchID = importBatchID
		}
		normalized = append(normalized, next)
	}

	existing, err := s.repo.ListEvents(ctx, hubUserID)
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun("manual_import", startedAt, err))
		return Summary{}, err
	}
	combined := mergeImportedEvents(existing, normalized)
	if err := s.repo.ReplaceEvents(ctx, hubUserID, combined); err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun("manual_import", startedAt, err))
		return Summary{}, err
	}
	if err := s.repo.AppendRun(ctx, hubUserID, buildSuccessfulRun("manual_import", startedAt, s.now(), normalized)); err != nil {
		return Summary{}, err
	}
	s.observeEvents(ctx, SummaryQuery{HubUserID: hubUserID, DeviceCount: query.DeviceCount}, normalized)
	runs, err := s.repo.ListRuns(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	return s.buildSummaryWithBindings(ctx, query, hubUserID, combined, runs, startedAt)
}

func (s *Service) ImportWXCommentExport(ctx context.Context, query SummaryQuery, path string) (Summary, error) {
	return s.importWXCommentExportForAccount(ctx, query, path, "", "")
}

func (s *Service) importWXCommentExport(ctx context.Context, query SummaryQuery, path, targetKey string) (Summary, error) {
	return s.importWXCommentExportForAccount(ctx, query, path, targetKey, "")
}

func (s *Service) ImportWXCommentExportForAccount(ctx context.Context, query SummaryQuery, accountID, path string) (Summary, error) {
	return s.importWXCommentExportForAccount(ctx, query, path, "", accountID)
}

func (s *Service) importWXCommentExportForAccount(ctx context.Context, query SummaryQuery, path, targetKey, accountID string) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()
	path = strings.TrimSpace(path)
	if path == "" {
		err := errors.New("path is required")
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}

	resolution, err := s.resolveWXCommentExportPath(path)
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}

	payload, err := LoadWXCommentExportFile(resolution.ResolvedPath)
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}

	machineID := "wx-channel-import"
	if query.DeviceCount > 0 {
		machineID = fmt.Sprintf("wx-channel-import-%02d", query.DeviceCount)
	}

	events, err := BuildEventsFromWXCommentExport(hubUserID, machineID, payload, startedAt)
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}
	if s.rawCommentRepo != nil {
		rawComments := FlattenWXCommentExport(hubUserID, accountID, payload, startedAt)
		if err := s.rawCommentRepo.Upsert(ctx, hubUserID, rawComments); err != nil {
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
			return Summary{}, fmt.Errorf("persist raw comments: %w", err)
		}
	}

	binding := SourceBinding{
		Source:        RunSourceWXCommentExport,
		TargetKey:     strings.TrimSpace(targetKey),
		Path:          normalizeBindingPath(firstNonEmpty(resolution.PersistPath, resolution.ResolvedPath, path)),
		ObjectID:      strings.TrimSpace(payload.ObjectID),
		ObjectNonceID: strings.TrimSpace(payload.ObjectNonceID),
		DeviceCount:   query.DeviceCount,
		UpdatedAt:     startedAt,
	}

	return s.importEventsWithSource(ctx, SummaryQuery{
		HubUserID:   hubUserID,
		DeviceCount: query.DeviceCount,
	}, RunSourceWXCommentExport, startedAt, events, &binding)
}

func (s *Service) ImportWXCommentExportByAPI(ctx context.Context, query SummaryQuery, baseURL, objectID, nonceID, title, author, clientID string) (Summary, error) {
	return s.importWXCommentExportByAPIForAccount(ctx, query, "", baseURL, objectID, nonceID, title, author, clientID)
}

func (s *Service) ImportWXCommentExportByAPIForAccount(ctx context.Context, query SummaryQuery, accountID, baseURL, objectID, nonceID, title, author, clientID string) (Summary, error) {
	return s.importWXCommentExportByAPIForAccount(ctx, query, accountID, baseURL, objectID, nonceID, title, author, clientID)
}

func (s *Service) importWXCommentExportByAPIForAccount(ctx context.Context, query SummaryQuery, accountID, baseURL, objectID, nonceID, title, author, clientID string) (Summary, error) {
	if strings.TrimSpace(clientID) != "" {
		s.SetWXFeedPreferClientID(clientID)
		defer s.SetWXFeedPreferClientID("")
	}
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()

	objectID = strings.TrimSpace(objectID)
	nonceID = strings.TrimSpace(nonceID)
	if objectID == "" {
		err := errors.New("object_id is required")
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}
	if nonceID == "" {
		err := errors.New("nonce_id is required")
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}

	client := s.wxCommentClient
	if strings.TrimSpace(baseURL) != "" {
		client.BaseURL = strings.TrimSpace(baseURL)
	}

	if strings.TrimSpace(client.BaseURL) != "" && s.enableLiveChecks {
		assessment := assessWXCommentExportBindingHealth(ctx, client.BaseURL, client.Client, SourceBindingAssessment{Runnable: true})
		if !assessment.Runnable {
			err := buildWXCommentExportBindingError(client.BaseURL, assessment.Reason)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
			return Summary{}, err
		}
	}

	result, err := client.Export(ctx, WXCommentExportRequest{
		ObjectID: objectID,
		NonceID:  nonceID,
		Title:    strings.TrimSpace(title),
		Author:   strings.TrimSpace(author),
	})
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}
	nextPath := strings.TrimSpace(result.SavedPath)
	if nextPath == "" {
		err := fmt.Errorf("wx_channel comment export did not return saved_path")
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}
	return s.ImportWXCommentExportForAccount(ctx, query, accountID, nextPath)
}

func (s *Service) ListRawComments(ctx context.Context, query SummaryQuery, rawQuery RawCommentQuery) (RawCommentPage, error) {
	if s == nil || s.rawCommentRepo == nil {
		return RawCommentPage{}, errors.New("raw comment repository is not configured")
	}
	return s.rawCommentRepo.List(ctx, s.resolveHubUserID(query.HubUserID), rawQuery)
}

func (s *Service) ListAccountMetricRows(ctx context.Context, query SummaryQuery, channelUsernames []string) ([]AccountMetricRow, error) {
	provider, ok := s.repo.(AccountMetricProvider)
	if !ok {
		return nil, ErrAccountMetricProviderUnavailable
	}
	return provider.ListAccountMetricRows(ctx, s.resolveHubUserID(query.HubUserID), channelUsernames)
}

func (s *Service) ExportRawComments(ctx context.Context, query SummaryQuery, rawQuery RawCommentQuery, dst io.Writer) error {
	if s == nil || s.rawCommentRepo == nil {
		return errors.New("raw comment repository is not configured")
	}
	if dst == nil {
		return errors.New("comment export writer is required")
	}
	hubUserID := s.resolveHubUserID(query.HubUserID)
	if exporter, ok := s.rawCommentRepo.(RawCommentExporter); ok {
		return exporter.ExportJSON(ctx, hubUserID, rawQuery, dst)
	}

	// File and demo stores remain compatible, but only an explicit export may
	// walk their records and serialize them.
	exportQuery := NormalizeRawCommentQuery(rawQuery)
	exportQuery.Page = 1
	exportQuery.PageSize = 100
	firstPage, err := s.rawCommentRepo.List(ctx, hubUserID, exportQuery)
	if err != nil {
		return err
	}
	writer, err := NewRawCommentJSONWriter(dst, RawCommentExportMetadata{
		HubUserID: hubUserID, Total: firstPage.Total,
	}, exportQuery)
	if err != nil {
		return err
	}
	for page := 1; page <= firstPage.TotalPages; page++ {
		current := firstPage
		if page > 1 {
			exportQuery.Page = page
			current, err = s.rawCommentRepo.List(ctx, hubUserID, exportQuery)
			if err != nil {
				return err
			}
		}
		for _, item := range current.Items {
			if err := writer.Write(item); err != nil {
				return err
			}
		}
	}
	return writer.Close()
}

func (s *Service) GetVideoCommentSync(ctx context.Context, query SummaryQuery, accountID, videoObjectID string) (VideoCommentSyncState, bool, error) {
	if s == nil || s.commentSyncRepo == nil {
		return VideoCommentSyncState{}, false, errors.New("video comment sync repository is not configured")
	}
	return s.commentSyncRepo.GetState(ctx, s.resolveHubUserID(query.HubUserID), strings.TrimSpace(accountID), strings.TrimSpace(videoObjectID))
}

func (s *Service) UpsertVideoCommentSync(ctx context.Context, query SummaryQuery, state VideoCommentSyncState) error {
	if s == nil || s.commentSyncRepo == nil {
		return errors.New("video comment sync repository is not configured")
	}
	state.HubUserID = s.resolveHubUserID(query.HubUserID)
	return s.commentSyncRepo.UpsertState(ctx, state)
}

func (s *Service) SaveWXCommentExportBinding(ctx context.Context, query SummaryQuery, path string) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()
	path = strings.TrimSpace(path)
	if path == "" {
		return Summary{}, errors.New("path is required")
	}

	resolution, _, err := s.loadWXCommentExportBindingPayload(SourceBinding{
		Source: RunSourceWXCommentExport,
		Path:   path,
	})
	if err != nil {
		return Summary{}, err
	}

	binding := SourceBinding{
		Source:      RunSourceWXCommentExport,
		Origin:      BindingOriginManual,
		Path:        normalizeBindingPath(firstNonEmpty(resolution.PersistPath, resolution.ResolvedPath, path)),
		DeviceCount: query.DeviceCount,
		UpdatedAt:   startedAt,
	}
	if err := s.repo.UpsertBinding(ctx, hubUserID, binding); err != nil {
		return Summary{}, err
	}

	events, err := s.repo.ListEvents(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	runs, err := s.repo.ListRuns(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	return s.buildSummaryWithBindings(ctx, query, hubUserID, events, runs, startedAt)
}

func (s *Service) RefreshWXCommentExportBinding(ctx context.Context, query SummaryQuery, binding SourceBinding) (Summary, error) {
	if strings.TrimSpace(s.wxCommentClient.BaseURL) == "" {
		return s.importWXCommentExport(ctx, query, binding.Path, binding.TargetKey)
	}
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()
	assessment := s.assessBinding(ctx, RunSourceWXCommentExport, binding)
	if !assessment.Runnable {
		err := buildWXCommentExportBindingError(s.wxCommentClient.BaseURL, assessment.Reason)
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}
	_, payload, err := s.loadWXCommentExportBindingPayload(binding)
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}
	result, err := s.wxCommentClient.Export(ctx, WXCommentExportRequest{
		ObjectID: payload.ObjectID,
		NonceID:  payload.ObjectNonceID,
		Title:    payload.Title,
		Author:   payload.Author,
	})
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}
	nextPath := strings.TrimSpace(result.SavedPath)
	if nextPath == "" {
		err := fmt.Errorf("wx_channel comment export did not return saved_path")
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXCommentExport, startedAt, err))
		return Summary{}, err
	}
	return s.importWXCommentExport(ctx, query, nextPath, binding.TargetKey)
}

// SetWXFeedListTransport installs a remote/local hybrid transport (edge gateway).
func (s *Service) SetWXFeedListTransport(transport FeedListTransport) {
	if s == nil {
		return
	}
	s.wxFeedListClient.Transport = transport
}

// SetWXFeedPreferClientID sticky-selects an edge node when transport supports it.
func (s *Service) SetWXFeedPreferClientID(clientID string) {
	if s == nil {
		return
	}
	type preferSetter interface{ SetPreferClientID(string) }
	if ps, ok := s.wxFeedListClient.Transport.(preferSetter); ok {
		ps.SetPreferClientID(clientID)
	}
	if ps, ok := any(s.wxCommentClient.Transport).(preferSetter); ok {
		ps.SetPreferClientID(clientID)
	}
}

// SetWXCommentTransport installs comment list transport (edge/local hybrid).
func (s *Service) SetWXCommentTransport(transport CommentListTransport) {
	if s == nil {
		return
	}
	s.wxCommentClient.Transport = transport
}

func (s *Service) SearchWXFeedAccounts(ctx context.Context, _ SummaryQuery, baseURL, keyword, clientID string) ([]WXFeedAccount, error) {
	if strings.TrimSpace(clientID) != "" {
		s.SetWXFeedPreferClientID(clientID)
		defer s.SetWXFeedPreferClientID("")
	}
	client := s.wxFeedListClient
	baseURL = s.resolveWXFeedBaseURL(baseURL)
	if strings.TrimSpace(baseURL) != "" {
		client = clientForExplicitWXFeedBaseURL(client, baseURL)
	}
	keyword = strings.TrimSpace(keyword)
	if looksLikeWXFeedFinderUsername(keyword) {
		account, err := client.ResolveAccount(ctx, keyword)
		if err != nil {
			return nil, err
		}
		return []WXFeedAccount{account}, nil
	}
	return client.SearchAccounts(ctx, keyword)
}

func (s *Service) SearchWXVideos(ctx context.Context, _ SummaryQuery, baseURL, keyword, clientID string) (WXFeedListResult, error) {
	if strings.TrimSpace(clientID) != "" {
		s.SetWXFeedPreferClientID(clientID)
		defer s.SetWXFeedPreferClientID("")
	}
	client := s.wxFeedListClient
	baseURL = s.resolveWXFeedBaseURL(baseURL)
	if strings.TrimSpace(baseURL) != "" {
		client = clientForExplicitWXFeedBaseURL(client, baseURL)
	}
	return client.SearchVideos(ctx, keyword)
}

func (s *Service) ImportWXFeedList(ctx context.Context, query SummaryQuery, baseURL, username, clientID string) (Summary, error) {
	if strings.TrimSpace(clientID) != "" {
		s.SetWXFeedPreferClientID(clientID)
		defer s.SetWXFeedPreferClientID("")
	}
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()
	client := s.wxFeedListClient
	baseURL = s.resolveWXFeedBaseURL(baseURL)
	if strings.TrimSpace(baseURL) != "" {
		client = clientForExplicitWXFeedBaseURL(client, baseURL)
	}

	binding := SourceBinding{
		Source:      RunSourceWXFeedList,
		BaseURL:     s.persistedWXFeedBaseURL(bindingBaseURLForClient(client, baseURL)),
		Username:    strings.TrimSpace(username),
		DeviceCount: query.DeviceCount,
		UpdatedAt:   startedAt,
	}
	assessment := s.assessBinding(ctx, RunSourceWXFeedList, binding)
	if assessment.ResolvedBaseURL != "" && assessment.ResolvedBaseURL != binding.BaseURL {
		binding.BaseURL = s.persistedWXFeedBaseURL(assessment.ResolvedBaseURL)
		client.BaseURL = assessment.ResolvedBaseURL
	}
	if binding.Username != "" {
		if err := s.repo.UpsertBinding(ctx, hubUserID, binding); err != nil {
			return Summary{}, err
		}
	}
	if !assessment.Runnable {
		// Edge/server mode: empty BaseURL + FeedListTransport means live HTTP probe is not applicable.
		if !(client.Transport != nil && strings.TrimSpace(binding.BaseURL) == "") {
			err := buildWXFeedListBindingError(binding.BaseURL, assessment.Reason)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXFeedList, startedAt, err))
			return Summary{}, err
		}
	}

	progress, err := s.collectionStart(ctx, query, RunSourceWXFeedList, strings.TrimSpace(username))
	if err != nil {
		return Summary{}, err
	}
	client.StartMarker = progress.NextMarker
	result, err := client.Fetch(ctx, username)
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXFeedList, startedAt, err))
		return Summary{}, err
	}

	machineID := "wx-channel-feed-list"
	if query.DeviceCount > 0 {
		machineID = fmt.Sprintf("wx-channel-feed-list-%02d", query.DeviceCount)
	}

	events := BuildEventsFromWXFeedList(hubUserID, machineID, result, startedAt)
	if len(events) == 0 && progress.NextMarker == "" {
		err := buildWXFeedListEmptyResultError(username)
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXFeedList, startedAt, err))
		return Summary{}, err
	}

	return s.importEventsWithSource(ctx, SummaryQuery{
		HubUserID:   hubUserID,
		DeviceCount: query.DeviceCount,
	}, RunSourceWXFeedList, startedAt, events, &binding, progressFromResult(progress, result, s.now()))
}

// ImportWXFeedListForAccount persists an account feed without creating a
// generic saved binding. Account monitoring owns the target identity; the
// resulting events and metric observations still use the normal ingest path.
func (s *Service) ImportWXFeedListForAccount(ctx context.Context, query SummaryQuery, username, clientID string) (WXFeedListResult, Summary, error) {
	if strings.TrimSpace(clientID) != "" {
		s.SetWXFeedPreferClientID(clientID)
		defer s.SetWXFeedPreferClientID("")
	}
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()
	username = strings.TrimSpace(username)
	if username == "" {
		return WXFeedListResult{}, Summary{}, errors.New("username is required")
	}
	client := s.wxFeedListClient
	progress, err := s.collectionStart(ctx, query, RunSourceWXFeedList, strings.TrimSpace(username))
	if err != nil {
		return WXFeedListResult{}, Summary{}, err
	}
	client.StartMarker = progress.NextMarker
	result, err := client.Fetch(ctx, username)
	if err != nil {
		return WXFeedListResult{}, Summary{}, err
	}
	if len(result.Items) == 0 && progress.NextMarker == "" {
		return WXFeedListResult{}, Summary{}, buildWXFeedListEmptyResultError(username)
	}
	machineID := "wx-channel-account-feed"
	if query.DeviceCount > 0 {
		machineID = fmt.Sprintf("wx-channel-account-feed-%02d", query.DeviceCount)
	}
	events := BuildEventsFromWXFeedList(hubUserID, machineID, result, startedAt)
	summary, err := s.importEventsWithSource(ctx, SummaryQuery{
		HubUserID:   hubUserID,
		DeviceCount: query.DeviceCount,
	}, RunSourceWXFeedList, startedAt, events, nil, progressFromResult(progress, result, s.now()))
	if err != nil {
		return WXFeedListResult{}, Summary{}, err
	}
	return result, summary, nil
}

func (s *Service) ImportWXVideoSearch(ctx context.Context, query SummaryQuery, baseURL, keyword, clientID string) (Summary, error) {
	if strings.TrimSpace(clientID) != "" {
		s.SetWXFeedPreferClientID(clientID)
		defer s.SetWXFeedPreferClientID("")
	}
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()
	client := s.wxFeedListClient
	baseURL = s.resolveWXFeedBaseURL(baseURL)
	if strings.TrimSpace(baseURL) != "" {
		client = clientForExplicitWXFeedBaseURL(client, baseURL)
	}

	binding := SourceBinding{
		Source:      RunSourceWXVideoSearch,
		BaseURL:     s.persistedWXFeedBaseURL(bindingBaseURLForClient(client, baseURL)),
		Keyword:     strings.TrimSpace(keyword),
		DeviceCount: query.DeviceCount,
		UpdatedAt:   startedAt,
	}
	assessment := s.assessBinding(ctx, RunSourceWXVideoSearch, binding)
	if assessment.ResolvedBaseURL != "" && assessment.ResolvedBaseURL != binding.BaseURL {
		binding.BaseURL = s.persistedWXFeedBaseURL(assessment.ResolvedBaseURL)
		client.BaseURL = assessment.ResolvedBaseURL
	}
	if binding.Keyword != "" {
		if err := s.repo.UpsertBinding(ctx, hubUserID, binding); err != nil {
			return Summary{}, err
		}
	}
	if !assessment.Runnable {
		if !(client.Transport != nil && strings.TrimSpace(binding.BaseURL) == "") {
			err := buildWXVideoSearchBindingError(binding.BaseURL, assessment.Reason)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXVideoSearch, startedAt, err))
			return Summary{}, err
		}
	}

	progress, err := s.collectionStart(ctx, query, RunSourceWXVideoSearch, strings.TrimSpace(keyword))
	if err != nil {
		return Summary{}, err
	}
	client.StartMarker = progress.NextMarker
	result, err := client.SearchVideos(ctx, keyword)
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXVideoSearch, startedAt, err))
		return Summary{}, err
	}

	machineID := "wx-channel-video-search"
	if query.DeviceCount > 0 {
		machineID = fmt.Sprintf("wx-channel-video-search-%02d", query.DeviceCount)
	}

	events := BuildEventsFromWXVideoSearch(hubUserID, machineID, result, keyword, startedAt)
	if len(events) == 0 && progress.NextMarker == "" {
		err := buildWXVideoSearchEmptyResultError(keyword)
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(RunSourceWXVideoSearch, startedAt, err))
		return Summary{}, err
	}

	return s.importEventsWithSource(ctx, SummaryQuery{
		HubUserID:   hubUserID,
		DeviceCount: query.DeviceCount,
	}, RunSourceWXVideoSearch, startedAt, events, &binding, progressFromResult(progress, result, s.now()))
}

func clientForExplicitWXFeedBaseURL(client WXFeedListClient, baseURL string) WXFeedListClient {
	client.BaseURL = strings.TrimSpace(baseURL)
	// An explicit URL is an operator choice. Do not let the default Edge
	// transport mask that endpoint with a successful but unrelated response.
	client.Transport = nil
	return client
}

func bindingBaseURLForClient(client WXFeedListClient, baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" {
		return normalizeWXFeedListBaseURL(baseURL)
	}
	if client.Transport != nil {
		// Edge/server topology: empty base means "use transport/edge", not loopback.
		return ""
	}
	if strings.TrimSpace(client.BaseURL) != "" {
		return normalizeWXFeedListBaseURL(client.BaseURL)
	}
	return normalizeWXFeedListBaseURL("")
}

func (s *Service) SaveWXFeedListBinding(ctx context.Context, query SummaryQuery, baseURL, username string) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()
	client := s.wxFeedListClient
	baseURL = s.resolveWXFeedBaseURL(baseURL)
	if strings.TrimSpace(baseURL) != "" {
		client.BaseURL = strings.TrimSpace(baseURL)
	}

	binding := SourceBinding{
		Source:      RunSourceWXFeedList,
		Origin:      BindingOriginManual,
		BaseURL:     s.persistedWXFeedBaseURL(bindingBaseURLForClient(client, baseURL)),
		Username:    strings.TrimSpace(username),
		DeviceCount: query.DeviceCount,
		UpdatedAt:   startedAt,
	}
	assessment := s.assessBinding(ctx, RunSourceWXFeedList, binding)
	if assessment.ResolvedBaseURL != "" {
		binding.BaseURL = s.persistedWXFeedBaseURL(assessment.ResolvedBaseURL)
	}
	if binding.Username == "" {
		return Summary{}, errors.New("username is required")
	}
	if err := s.repo.UpsertBinding(ctx, hubUserID, binding); err != nil {
		return Summary{}, err
	}

	events, err := s.repo.ListEvents(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	runs, err := s.repo.ListRuns(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	return s.buildSummaryWithBindings(ctx, query, hubUserID, events, runs, startedAt)
}

func (s *Service) SaveWXVideoSearchBinding(ctx context.Context, query SummaryQuery, baseURL, keyword string) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := s.now()
	client := s.wxFeedListClient
	baseURL = s.resolveWXFeedBaseURL(baseURL)
	if strings.TrimSpace(baseURL) != "" {
		client.BaseURL = strings.TrimSpace(baseURL)
	}

	binding := SourceBinding{
		Source:      RunSourceWXVideoSearch,
		Origin:      BindingOriginManual,
		BaseURL:     s.persistedWXFeedBaseURL(bindingBaseURLForClient(client, baseURL)),
		Keyword:     strings.TrimSpace(keyword),
		DeviceCount: query.DeviceCount,
		UpdatedAt:   startedAt,
	}
	assessment := s.assessBinding(ctx, RunSourceWXVideoSearch, binding)
	if assessment.ResolvedBaseURL != "" {
		binding.BaseURL = s.persistedWXFeedBaseURL(assessment.ResolvedBaseURL)
	}
	if binding.Keyword == "" {
		return Summary{}, errors.New("keyword is required")
	}
	if err := s.repo.UpsertBinding(ctx, hubUserID, binding); err != nil {
		return Summary{}, err
	}

	events, err := s.repo.ListEvents(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	runs, err := s.repo.ListRuns(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	return s.buildSummaryWithBindings(ctx, query, hubUserID, events, runs, startedAt)
}

func (s *Service) RetryLastFailedImport(ctx context.Context, query SummaryQuery, source string) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	source = strings.TrimSpace(source)
	if source == "" {
		return Summary{}, errors.New("source is required")
	}

	runs, err := s.repo.ListRuns(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	latestRun, ok := findLatestRunBySource(runs, source)
	if !ok {
		return Summary{}, fmt.Errorf("no ingest runs found for source %s", source)
	}
	if latestRun.Status != "failed" {
		return Summary{}, fmt.Errorf("latest ingest run for source %s is not failed", source)
	}

	bindings, err := s.repo.ListBindings(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	binding, ok := findBindingBySource(bindings, source)
	if !ok {
		return Summary{}, fmt.Errorf("no saved source binding for source %s", source)
	}
	assessment := s.assessBinding(ctx, source, binding)
	if assessment.ResolvedBaseURL != "" {
		binding.BaseURL = s.persistedWXFeedBaseURL(assessment.ResolvedBaseURL)
	}
	if !assessment.Runnable {
		return Summary{}, fmt.Errorf("saved binding for source %s is not runnable: %s", source, assessment.Reason)
	}

	retryQuery := SummaryQuery{
		HubUserID: hubUserID,
	}
	if binding.DeviceCount > 0 {
		retryQuery.DeviceCount = binding.DeviceCount
	} else {
		retryQuery.DeviceCount = query.DeviceCount
	}

	switch source {
	case RunSourceWXCommentExport:
		return s.RefreshWXCommentExportBinding(ctx, retryQuery, binding)
	case RunSourceWXFeedList:
		return s.ImportWXFeedList(ctx, retryQuery, binding.BaseURL, binding.Username, "")
	case RunSourceWXVideoSearch:
		return s.ImportWXVideoSearch(ctx, retryQuery, binding.BaseURL, binding.Keyword, "")
	default:
		return Summary{}, fmt.Errorf("retry is not supported for source %s", source)
	}
}

func (s *Service) loadWXCommentExportBindingPayload(binding SourceBinding) (wxCommentExportPathResolution, WXCommentExportFile, error) {
	resolution, err := s.resolveWXCommentExportPath(binding.Path)
	if err != nil {
		return wxCommentExportPathResolution{}, WXCommentExportFile{}, err
	}
	payload, err := LoadWXCommentExportFile(resolution.ResolvedPath)
	if err != nil {
		return wxCommentExportPathResolution{}, WXCommentExportFile{}, err
	}
	if strings.TrimSpace(payload.ObjectID) == "" {
		return wxCommentExportPathResolution{}, WXCommentExportFile{}, fmt.Errorf("objectId is required")
	}
	if strings.TrimSpace(payload.ObjectNonceID) == "" {
		return wxCommentExportPathResolution{}, WXCommentExportFile{}, fmt.Errorf("objectNonceId is required")
	}
	return resolution, payload, nil
}

func (s *Service) Reset(ctx context.Context, query SummaryQuery) (Summary, error) {
	hubUserID := s.resolveHubUserID(query.HubUserID)
	if err := s.repo.Reset(ctx, hubUserID); err != nil {
		return Summary{}, err
	}
	return Summary{
		HubUserID:      hubUserID,
		State:          "idle",
		FreshnessState: "idle",
	}, nil
}

func (s *Service) importEventsWithSource(ctx context.Context, query SummaryQuery, runSource string, startedAt time.Time, events []Event, binding *SourceBinding, progress ...CollectionProgress) (Summary, error) {
	// Save completed pages even when cancellation interrupted a later request.
	// This bounded context is only used after collection has already stopped.
	if len(progress) > 0 && ctx.Err() != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
	}
	hubUserID := s.resolveHubUserID(query.HubUserID)
	if len(events) == 0 && !(len(progress) > 0 && progress[0].Complete && progress[0].PageCount > 0) {
		err := errors.New("events are required")
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(runSource, startedAt, err))
		return Summary{}, err
	}

	importBatchID := buildImportBatchID(startedAt)
	normalized := make([]Event, 0, len(events))
	for index, event := range events {
		next := event
		if next.HubUserID == "" {
			next.HubUserID = hubUserID
		}
		if next.HubUserID != hubUserID {
			err := fmt.Errorf("event %d hubUserID mismatch", index)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(runSource, startedAt, err))
			return Summary{}, err
		}
		if next.ID == "" {
			err := fmt.Errorf("event %d id is required", index)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(runSource, startedAt, err))
			return Summary{}, err
		}
		if next.EventType == "" {
			err := fmt.Errorf("event %d eventType is required", index)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(runSource, startedAt, err))
			return Summary{}, err
		}
		if next.Source == "" {
			err := fmt.Errorf("event %d source is required", index)
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(runSource, startedAt, err))
			return Summary{}, err
		}
		if next.CollectedAt.IsZero() {
			next.CollectedAt = s.now()
		}
		if next.BatchID == "" {
			next.BatchID = importBatchID
		}
		normalized = append(normalized, next)
	}

	existing, err := s.repo.ListEvents(ctx, hubUserID)
	if err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(runSource, startedAt, err))
		return Summary{}, err
	}
	combined := mergeImportedEvents(existing, normalized)
	if err := s.repo.ReplaceEvents(ctx, hubUserID, combined); err != nil {
		_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(runSource, startedAt, err))
		return Summary{}, err
	}
	observations := buildVideoMetricObservations(normalized)
	if len(observations) > 0 {
		if err := s.repo.AppendVideoMetricObservations(ctx, hubUserID, observations); err != nil {
			_ = s.repo.AppendRun(ctx, hubUserID, buildFailedRun(runSource, startedAt, err))
			return Summary{}, err
		}
	}
	run := buildSuccessfulRun(runSource, startedAt, s.now(), normalized)
	if len(progress) > 0 {
		run.Collection = &progress[0]
		if !progress[0].Complete {
			run.Status = "partial"
			run.Message = progress[0].Notice
		}
	}
	if err := s.repo.AppendRun(ctx, hubUserID, run); err != nil {
		return Summary{}, err
	}
	if binding != nil {
		nextBinding := *binding
		if nextBinding.Source == "" {
			nextBinding.Source = runSource
		}
		if nextBinding.UpdatedAt.IsZero() {
			nextBinding.UpdatedAt = startedAt
		}
		if err := s.repo.UpsertBinding(ctx, hubUserID, nextBinding); err != nil {
			return Summary{}, err
		}
	}
	s.observeEvents(ctx, SummaryQuery{HubUserID: hubUserID, DeviceCount: query.DeviceCount}, normalized)
	runs, err := s.repo.ListRuns(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	summary, err := s.buildSummaryWithBindings(ctx, query, hubUserID, combined, runs, startedAt)
	summary.Collection = run.Collection
	return summary, err
}

func buildVideoMetricObservations(events []Event) []VideoMetricObservation {
	observations := make([]VideoMetricObservation, 0, len(events))
	for _, event := range events {
		if event.EventType != "video_snapshot" || !isWXVideoMetricSource(event.Source) {
			continue
		}
		observations = append(observations, VideoObservationFromEvent(event))
	}
	return observations
}

// VideoObservationFromEvent projects a saved snapshot without creating or
// persisting an additional historical observation.
func VideoObservationFromEvent(event Event) VideoMetricObservation {
	id := strings.TrimSpace(event.ID) + "@" + event.CollectedAt.UTC().Format(time.RFC3339Nano)
	if keyword := strings.TrimSpace(event.Keyword); keyword != "" {
		id += "@keyword=" + keyword
	}
	return VideoMetricObservation{
		MissingMetrics: event.MissingMetrics, ID: id, HubUserID: event.HubUserID,
		EventID: event.ID, BatchID: event.BatchID, Source: event.Source, ObservedAt: event.CollectedAt,
		PublishedAt: event.PublishedAt, ObjectID: event.ObjectID, ObjectNonceID: event.ObjectNonceID,
		ChannelUsername: event.ChannelUsername, CoverURL: event.CoverURL, Title: event.Title, ChannelName: event.ChannelName,
		Keyword: event.Keyword, Tags: append([]string(nil), event.Tags...), PlayCount: event.PlayCount,
		LikeCount: event.LikeCount, CommentCount: event.CommentCount, FavCount: event.FavCount, ForwardCount: event.ForwardCount,
	}
}

func isWXVideoMetricSource(source string) bool {
	return source == "wx_channel_feed_list" || source == RunSourceWXVideoSearch || source == RunSourceWXManualShare || source == RunSourceWXVideoProfile
}

func (s *Service) buildSummaryWithBindings(ctx context.Context, query SummaryQuery, hubUserID string, events []Event, runs []Run, now time.Time) (Summary, error) {
	return s.buildSummaryWithBindingStates(ctx, query, hubUserID, events, runs, now, true)
}

func (s *Service) buildSummaryReadOnlyWithBindings(ctx context.Context, query SummaryQuery, hubUserID string, events []Event, runs []Run, now time.Time) (Summary, error) {
	return s.buildSummaryWithBindingStates(ctx, query, hubUserID, events, runs, now, false)
}

func (s *Service) buildSummaryWithBindingStates(ctx context.Context, query SummaryQuery, hubUserID string, events []Event, runs []Run, now time.Time, includeLiveChecks bool) (Summary, error) {
	return s.buildSummaryFromEventFactsWithBindingStates(
		ctx,
		query,
		hubUserID,
		summarizeEventFacts(events, recentSummaryEventsReadLimit),
		runs,
		now,
		includeLiveChecks,
	)
}

func (s *Service) buildSummaryFromEventFactsWithBindingStates(ctx context.Context, query SummaryQuery, hubUserID string, eventFacts EventSummaryFacts, runs []Run, now time.Time, includeLiveChecks bool) (Summary, error) {
	bindingStates, err := s.listBindingStatesWithRunsAndFacts(ctx, hubUserID, runs, eventFacts, includeLiveChecks)
	if err != nil {
		return Summary{}, err
	}
	autoRunFacts, err := s.loadSourceAutoRunFacts(ctx, hubUserID)
	if err != nil {
		return Summary{}, err
	}
	return buildSummaryFromEventFacts(hubUserID, eventFacts, runs, autoRunFacts, bindingStates, now), nil
}

func (s *Service) resolveHubUserID(value string) string {
	if value != "" {
		return value
	}
	if s.defaultHubUserID != "" {
		return s.defaultHubUserID
	}
	return "hub:user:demo"
}

func nextBatchIndex(events []Event) int {
	seen := map[string]struct{}{}
	for _, event := range events {
		if event.BatchID == "" {
			continue
		}
		seen[event.BatchID] = struct{}{}
	}
	return len(seen) + 1
}

func buildImportBatchID(now time.Time) string {
	return fmt.Sprintf("import-%s", now.UTC().Format("20060102T150405.000000000Z"))
}

func normalizeBindingPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolutePath
}

func mergeImportedEvents(existing, incoming []Event) []Event {
	uniqueExisting := collapseEventsByID(existing)
	uniqueIncoming := collapseEventsByID(incoming)
	if len(uniqueIncoming) == 0 {
		return uniqueExisting
	}

	incomingIDs := make(map[string]struct{}, len(uniqueIncoming))
	for _, event := range uniqueIncoming {
		incomingIDs[event.ID] = struct{}{}
	}

	combined := make([]Event, 0, len(uniqueExisting)+len(uniqueIncoming))
	for _, event := range uniqueExisting {
		if _, overridden := incomingIDs[event.ID]; overridden {
			continue
		}
		combined = append(combined, event)
	}
	combined = append(combined, uniqueIncoming...)
	return combined
}

func collapseEventsByID(events []Event) []Event {
	if len(events) <= 1 {
		return append([]Event(nil), events...)
	}

	lastIndexByID := make(map[string]int, len(events))
	for index, event := range events {
		lastIndexByID[event.ID] = index
	}

	collapsed := make([]Event, 0, len(lastIndexByID))
	for index, event := range events {
		if lastIndexByID[event.ID] != index {
			continue
		}
		collapsed = append(collapsed, event)
	}
	return collapsed
}

func pickMockBatchProfile(batchIndex int) mockBatchProfile {
	if len(mockBatchProfiles) == 0 {
		return mockBatchProfile{}
	}
	return mockBatchProfiles[(batchIndex-1)%len(mockBatchProfiles)]
}

func buildMockBatch(hubUserID string, deviceCount, batchIndex int, now time.Time) []Event {
	sourceCount := deviceCount
	if sourceCount <= 0 {
		sourceCount = 2
	}
	if sourceCount > 3 {
		sourceCount = 3
	}

	batchID := fmt.Sprintf("mock-batch-%03d", batchIndex)
	machines := make([]string, 0, sourceCount)
	for i := 0; i < sourceCount; i++ {
		machines = append(machines, fmt.Sprintf("machine-%02d", i+1))
	}

	offset := batchIndex - 1
	profile := pickMockBatchProfile(batchIndex)
	return []Event{
		{
			ID:           fmt.Sprintf("%s-video-%02d", batchID, 1),
			BatchID:      batchID,
			HubUserID:    hubUserID,
			MachineID:    machines[0],
			EventType:    "video_snapshot",
			Source:       "channel_feed_pull",
			CollectedAt:  now.Add(-3 * time.Minute),
			PublishedAt:  now.Add(-3 * time.Minute),
			VideoCount:   12 + offset*3,
			CommentCount: 48 + offset*9,
			TopicCount:   2 + offset,
			Title:        fmt.Sprintf("%s（第 %d 批）", profile.videoTitle, batchIndex),
			ChannelName:  profile.videoChannelName,
			Signal:       profile.videoSignal,
			Tags:         append([]string(nil), profile.videoTags...),
		},
		{
			ID:                    fmt.Sprintf("%s-comment-%02d", batchID, 2),
			BatchID:               batchID,
			HubUserID:             hubUserID,
			MachineID:             machines[len(machines)-1],
			EventType:             "comment_snapshot",
			Source:                "feed_comment_list",
			CollectedAt:           now.Add(-2 * time.Minute),
			VideoCount:            4 + offset,
			CommentCount:          126 + offset*18,
			TopicCount:            1 + offset,
			Title:                 profile.commentTitle,
			Keyword:               profile.commentKeyword,
			Signal:                profile.commentSignal,
			RepresentativeComment: profile.commentRepresentative,
			TargetAudience:        profile.targetAudience,
			Keywords:              append([]string(nil), profile.commentKeywords...),
		},
		{
			ID:             fmt.Sprintf("%s-topic-%02d", batchID, 3),
			BatchID:        batchID,
			HubUserID:      hubUserID,
			MachineID:      machines[0],
			EventType:      "topic_cluster",
			Source:         "insight_topic_cluster",
			CollectedAt:    now.Add(-1 * time.Minute),
			VideoCount:     0,
			CommentCount:   0,
			TopicCount:     5 + offset,
			Keyword:        profile.topicKeyword,
			Cluster:        profile.topicCluster,
			Signal:         profile.topicSignal,
			PromptAngle:    profile.topicPromptAngle,
			TargetAudience: profile.targetAudience,
			Keywords:       append([]string(nil), profile.topicKeywords...),
		},
	}
}

func summarizeEventFacts(events []Event, recentLimit int) EventSummaryFacts {
	normalized := normalizeEvents(events)
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].CollectedAt.Equal(normalized[j].CollectedAt) {
			if normalized[i].BatchID == normalized[j].BatchID {
				return normalized[i].ID > normalized[j].ID
			}
			return normalized[i].BatchID > normalized[j].BatchID
		}
		return normalized[i].CollectedAt.After(normalized[j].CollectedAt)
	})

	facts := EventSummaryFacts{TotalEvents: len(normalized)}
	machines := map[string]struct{}{}
	sources := map[string]*EventSourceFacts{}
	for _, event := range normalized {
		if event.MachineID != "" {
			machines[event.MachineID] = struct{}{}
		}
		facts.AggregatedVideos += event.VideoCount
		facts.AggregatedComments += event.CommentCount
		facts.AggregatedTopics += event.TopicCount
		source := canonicalSource(event.Source)
		sourceFacts, ok := sources[source]
		if !ok {
			sourceFacts = &EventSourceFacts{Source: source}
			sources[source] = sourceFacts
		}
		sourceFacts.EventCount++
		if sourceFacts.LatestEvent.ID == "" {
			sourceFacts.LatestEvent = event
		}
	}
	facts.SourceCount = len(machines)
	if len(normalized) > 0 {
		facts.LatestEvent = normalized[0]
		limit := recentLimit
		if limit <= 0 || limit > len(normalized) {
			limit = len(normalized)
		}
		facts.RecentEvents = append([]Event(nil), normalized[:limit]...)
	}
	facts.Sources = make([]EventSourceFacts, 0, len(sources))
	for _, sourceFacts := range sources {
		facts.Sources = append(facts.Sources, *sourceFacts)
	}
	sort.Slice(facts.Sources, func(i, j int) bool { return facts.Sources[i].Source < facts.Sources[j].Source })
	return facts
}

func normalizeEventSummaryFacts(facts EventSummaryFacts, recentLimit int) EventSummaryFacts {
	recent := normalizeEvents(facts.RecentEvents)
	sort.Slice(recent, func(i, j int) bool {
		if recent[i].CollectedAt.Equal(recent[j].CollectedAt) {
			if recent[i].BatchID == recent[j].BatchID {
				return recent[i].ID > recent[j].ID
			}
			return recent[i].BatchID > recent[j].BatchID
		}
		return recent[i].CollectedAt.After(recent[j].CollectedAt)
	})
	if recentLimit > 0 && len(recent) > recentLimit {
		recent = recent[:recentLimit]
	}
	facts.RecentEvents = append([]Event(nil), recent...)
	latest := normalizeEvents([]Event{facts.LatestEvent})
	if len(latest) > 0 {
		facts.LatestEvent = latest[0]
	}
	if len(recent) > 0 && (facts.LatestEvent.ID == "" || recent[0].CollectedAt.After(facts.LatestEvent.CollectedAt)) {
		facts.LatestEvent = recent[0]
	}

	sources := make(map[string]EventSourceFacts, len(facts.Sources))
	for _, item := range facts.Sources {
		item.Source = canonicalSource(item.Source)
		latestEvents := normalizeEvents([]Event{item.LatestEvent})
		if len(latestEvents) > 0 {
			item.LatestEvent = latestEvents[0]
		}
		current := sources[item.Source]
		current.Source = item.Source
		current.EventCount += item.EventCount
		if current.LatestEvent.ID == "" || item.LatestEvent.CollectedAt.After(current.LatestEvent.CollectedAt) ||
			(item.LatestEvent.CollectedAt.Equal(current.LatestEvent.CollectedAt) && item.LatestEvent.ID > current.LatestEvent.ID) {
			current.LatestEvent = item.LatestEvent
		}
		sources[item.Source] = current
	}
	facts.Sources = make([]EventSourceFacts, 0, len(sources))
	for _, item := range sources {
		facts.Sources = append(facts.Sources, item)
	}
	sort.Slice(facts.Sources, func(i, j int) bool { return facts.Sources[i].Source < facts.Sources[j].Source })
	return facts
}

func buildSummary(hubUserID string, events []Event, runs []Run, operatorRuns []OperatorRun, bindingStates []SourceBindingState, now time.Time) Summary {
	return buildSummaryFromEventFacts(
		hubUserID,
		summarizeEventFacts(events, recentSummaryEventsReadLimit),
		runs,
		buildAutoRunFactsBySource(operatorRuns),
		bindingStates,
		now,
	)
}

func buildSummaryFromEventFacts(hubUserID string, eventFacts EventSummaryFacts, runs []Run, autoRunFacts map[string]autoRunFact, bindingStates []SourceBindingState, now time.Time) Summary {
	eventFacts = normalizeEventSummaryFacts(eventFacts, recentSummaryEventsReadLimit)
	enrichedBindingStates := enrichBindingStatesWithAutoRunFactMap(bindingStates, autoRunFacts)
	summary := Summary{
		HubUserID:      hubUserID,
		State:          "idle",
		FreshnessState: "idle",
		BindingStates:  append([]SourceBindingState(nil), enrichedBindingStates...),
	}
	summary.OperatorReadiness = buildOperatorReadiness(summary.BindingStates)
	sortedRuns := normalizeRuns(runs)
	sort.Slice(sortedRuns, func(i, j int) bool {
		if sortedRuns[i].FinishedAt.Equal(sortedRuns[j].FinishedAt) {
			return sortedRuns[i].ID > sortedRuns[j].ID
		}
		return sortedRuns[i].FinishedAt.After(sortedRuns[j].FinishedAt)
	})

	summary.RecentRuns = limitRuns(sortedRuns, 6)
	sourceStatusMap := map[string]*SourceStatus{}
	for _, run := range sortedRuns {
		status, ok := sourceStatusMap[run.Source]
		if !ok {
			status = &SourceStatus{
				Source: run.Source,
				State:  "idle",
			}
			sourceStatusMap[run.Source] = status
		}
		if status.LastRunAt.IsZero() {
			status.LastRunAt = run.FinishedAt
		}
		if run.Status == "success" {
			if status.LastSuccessAt.IsZero() {
				status.LastSuccessAt = run.FinishedAt
				status.LatestBatchID = run.BatchID
				status.LastEventType = run.LastEventType
				status.EventCount = run.EventCount
			}
		} else if run.Status == "failed" && status.LastFailureAt.IsZero() {
			status.LastFailureAt = run.FinishedAt
			status.LastError = run.Message
		}
	}

	if eventFacts.TotalEvents == 0 {
		summary.SourceStatuses = buildSourceStatuses(sourceStatusMap, now)
		applyRunFacts(&summary, sortedRuns)
		return summary
	}

	summary.State = "ready"
	summary.TotalEvents = eventFacts.TotalEvents
	summary.SourceCount = eventFacts.SourceCount
	summary.AggregatedVideos = eventFacts.AggregatedVideos
	summary.AggregatedComments = eventFacts.AggregatedComments
	summary.AggregatedTopics = eventFacts.AggregatedTopics

	latest := eventFacts.LatestEvent
	summary.LatestEventAt = latest.CollectedAt
	summary.LatestBatchID = latest.BatchID
	summary.LastEventType = latest.EventType
	summary.RecentEvents = append([]Event(nil), eventFacts.RecentEvents...)

	for _, sourceFacts := range eventFacts.Sources {
		source := sourceFacts.Source
		status, ok := sourceStatusMap[source]
		if !ok {
			status = &SourceStatus{
				Source: source,
				State:  "ready",
			}
			sourceStatusMap[source] = status
		}
		status.EventCount = maxInt(status.EventCount, sourceFacts.EventCount)
		status.Target = buildSourceBindingTarget(
			SourceBinding{Source: source},
			SourceBindingAssessment{},
			eventFacts,
		)
		if latestEventForSource := &sourceFacts.LatestEvent; latestEventForSource.ID != "" {
			if status.LastSuccessAt.IsZero() {
				status.LastSuccessAt = latestEventForSource.CollectedAt
			}
			if status.LastRunAt.IsZero() {
				status.LastRunAt = latestEventForSource.CollectedAt
			}
			if status.LatestBatchID == "" {
				status.LatestBatchID = latestEventForSource.BatchID
			}
			if status.LastEventType == "" {
				status.LastEventType = latestEventForSource.EventType
			}
			if status.LatestEventAt.IsZero() || latestEventForSource.CollectedAt.After(status.LatestEventAt) {
				status.LatestEventAt = latestEventForSource.CollectedAt
			}
		}
	}

	summary.SourceStatuses = buildSourceStatuses(sourceStatusMap, now)
	applyRunFacts(&summary, sortedRuns)
	if summary.FreshnessState == "idle" {
		summary.FreshnessState = deriveFreshnessState(summary.LatestEventAt, now)
	}
	return summary
}

type autoRunFact struct {
	LastRunStatus string
	LastRunAt     time.Time
	LastSuccessAt time.Time
	LastBatchID   string
}

func enrichBindingStatesWithAutoRunFacts(bindingStates []SourceBindingState, operatorRuns []OperatorRun) []SourceBindingState {
	return enrichBindingStatesWithAutoRunFactMap(bindingStates, buildAutoRunFactsBySource(operatorRuns))
}

func enrichBindingStatesWithAutoRunFactMap(bindingStates []SourceBindingState, factsBySource map[string]autoRunFact) []SourceBindingState {
	if len(bindingStates) == 0 {
		return nil
	}
	enriched := append([]SourceBindingState(nil), bindingStates...)
	for index := range enriched {
		facts, ok := factsBySource[canonicalSource(enriched[index].Source)]
		if !ok {
			continue
		}
		enriched[index].LastAutoRunStatus = facts.LastRunStatus
		enriched[index].LastAutoRunAt = facts.LastRunAt
		enriched[index].LastAutoSuccessAt = facts.LastSuccessAt
		enriched[index].LastAutoBatchID = facts.LastBatchID
	}
	return enriched
}

func buildAutoRunFactsFromSourceFacts(items []SourceAutoRunFact) map[string]autoRunFact {
	factsBySource := make(map[string]autoRunFact, len(items))
	for _, item := range items {
		source := canonicalSource(item.Source)
		if source == "" {
			continue
		}
		facts := factsBySource[source]
		if facts.LastRunAt.IsZero() || item.LastRunAt.After(facts.LastRunAt) {
			facts.LastRunStatus = item.LastRunStatus
			facts.LastRunAt = item.LastRunAt
		}
		if !item.LastSuccessAt.IsZero() && (facts.LastSuccessAt.IsZero() || item.LastSuccessAt.After(facts.LastSuccessAt)) {
			facts.LastSuccessAt = item.LastSuccessAt
			facts.LastBatchID = item.LastBatchID
		}
		factsBySource[source] = facts
	}
	return factsBySource
}

func buildAutoRunFactsBySource(operatorRuns []OperatorRun) map[string]autoRunFact {
	if len(operatorRuns) == 0 {
		return map[string]autoRunFact{}
	}
	sortedRuns := append([]OperatorRun(nil), operatorRuns...)
	sort.Slice(sortedRuns, func(i, j int) bool {
		if sortedRuns[i].FinishedAt.Equal(sortedRuns[j].FinishedAt) {
			return sortedRuns[i].ID > sortedRuns[j].ID
		}
		return sortedRuns[i].FinishedAt.After(sortedRuns[j].FinishedAt)
	})

	factsBySource := make(map[string]autoRunFact, len(sortedRuns))
	for _, run := range sortedRuns {
		if !operatorRunCountsAsAuto(run) {
			continue
		}
		for _, outcome := range run.Outcomes {
			source := canonicalSource(outcome.Source)
			if source == "" {
				continue
			}
			facts := factsBySource[source]
			if facts.LastRunAt.IsZero() {
				facts.LastRunStatus = outcome.Status
				facts.LastRunAt = run.FinishedAt
			}
			if outcome.Status == "success" && facts.LastSuccessAt.IsZero() {
				facts.LastSuccessAt = run.FinishedAt
				facts.LastBatchID = run.LatestBatchID
			}
			factsBySource[source] = facts
		}
	}
	return factsBySource
}

func operatorRunCountsAsAuto(run OperatorRun) bool {
	triggerMode := strings.TrimSpace(run.TriggerMode)
	return triggerMode == OperatorTriggerLoop || triggerMode == OperatorTriggerScheduled
}

func buildSuccessfulRun(source string, startedAt, finishedAt time.Time, events []Event) Run {
	run := Run{
		ID:        fmt.Sprintf("%s-%s", source, startedAt.UTC().Format("20060102T150405.000000000Z")),
		Source:    source,
		Status:    "success",
		StartedAt: startedAt,
	}
	if finishedAt.IsZero() {
		finishedAt = startedAt
	}
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
	}
	if len(events) > 0 {
		run.EventCount = len(events)
		latestEvent := latestEventForRun(events)
		run.BatchID = latestEvent.BatchID
		run.LastEventType = latestEvent.EventType
		run.LatestEventAt = latestEvent.CollectedAt
	}
	run.FinishedAt = finishedAt
	return run
}

func buildFailedRun(source string, startedAt time.Time, err error) Run {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return Run{
		ID:         fmt.Sprintf("%s-%s", source, startedAt.UTC().Format("20060102T150405.000000000Z")),
		Source:     source,
		Status:     "failed",
		StartedAt:  startedAt,
		FinishedAt: startedAt,
		Message:    message,
	}
}

func findLatestRunBySource(runs []Run, source string) (Run, bool) {
	filtered := make([]Run, 0, len(runs))
	for _, run := range runs {
		if canonicalSource(run.Source) == canonicalSource(source) {
			filtered = append(filtered, run)
		}
	}
	if len(filtered) == 0 {
		return Run{}, false
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].FinishedAt.Equal(filtered[j].FinishedAt) {
			return filtered[i].ID > filtered[j].ID
		}
		return filtered[i].FinishedAt.After(filtered[j].FinishedAt)
	})
	return filtered[0], true
}

func findBindingBySource(bindings []SourceBinding, source string) (SourceBinding, bool) {
	for _, binding := range bindings {
		if canonicalSource(binding.Source) == canonicalSource(source) {
			return binding, true
		}
	}
	return SourceBinding{}, false
}

func canonicalSource(source string) string {
	switch strings.TrimSpace(source) {
	case "wx_channel_comment_export":
		fallthrough
	case "finderGetCommentList":
		fallthrough
	case "finderGetCommentList.partial":
		return RunSourceWXCommentExport
	case "wx_channel_feed_list":
		return RunSourceWXFeedList
	case "wx_channel_video_search":
		return RunSourceWXVideoSearch
	default:
		return strings.TrimSpace(source)
	}
}

func normalizeEvents(events []Event) []Event {
	results := make([]Event, 0, len(events))
	for _, event := range events {
		next := event
		next.Source = canonicalSource(next.Source)
		results = append(results, next)
	}
	return backfillEventObjectIdentifiers(results)
}

func normalizeRuns(runs []Run) []Run {
	results := make([]Run, 0, len(runs))
	for _, run := range runs {
		next := run
		next.Source = canonicalSource(next.Source)
		results = append(results, next)
	}
	return results
}

func latestCollectedAt(events []Event) time.Time {
	var latest time.Time
	for _, event := range events {
		if event.CollectedAt.After(latest) {
			latest = event.CollectedAt
		}
	}
	return latest
}

func latestEventForRun(events []Event) Event {
	if len(events) == 0 {
		return Event{}
	}

	latest := events[0]
	for _, event := range events[1:] {
		if event.CollectedAt.After(latest.CollectedAt) {
			latest = event
			continue
		}
		if event.CollectedAt.Equal(latest.CollectedAt) && event.ID > latest.ID {
			latest = event
		}
	}
	return latest
}

func limitRuns(runs []Run, limit int) []Run {
	if len(runs) <= limit {
		return append([]Run(nil), runs...)
	}
	return append([]Run(nil), runs[:limit]...)
}

func applyRunFacts(summary *Summary, runs []Run) {
	if summary == nil {
		return
	}
	lastFailureSource := ""
	for _, run := range runs {
		switch run.Status {
		case "success":
			if summary.LastSuccessAt.IsZero() {
				summary.LastSuccessAt = run.FinishedAt
			}
		case "failed":
			if summary.LastFailureAt.IsZero() {
				summary.LastFailureAt = run.FinishedAt
				summary.LastFailureMessage = run.Message
				lastFailureSource = run.Source
			}
		}
	}
	normalizeBindingDerivedFailures(summary, lastFailureSource)
	if summary.LastFailureAt.After(summary.LastSuccessAt) {
		summary.State = "degraded"
		summary.FreshnessState = "error"
	}
}

func normalizeBindingDerivedFailures(summary *Summary, lastFailureSource string) {
	if summary == nil || len(summary.BindingStates) == 0 {
		return
	}

	reasonsBySource := make(map[string]string, len(summary.BindingStates))
	for _, binding := range summary.BindingStates {
		if strings.TrimSpace(binding.RetryReason) != "" && bindingRetryReasonShouldOverrideFailure(binding) {
			reasonsBySource[canonicalSource(binding.Source)] = binding.RetryReason
		}
	}
	if len(reasonsBySource) == 0 {
		return
	}

	if summary.LastFailureMessage != "" {
		summary.LastFailureMessage = normalizeSummaryFailureMessage(
			summary.LastFailureMessage,
			lastFailureSource,
			reasonsBySource,
		)
	}

	for index := range summary.SourceStatuses {
		reason, ok := reasonsBySource[canonicalSource(summary.SourceStatuses[index].Source)]
		if !ok || reason == "" {
			continue
		}
		if runMessageLooksLikeBindingFailure(summary.SourceStatuses[index].LastError, summary.SourceStatuses[index].Source) {
			summary.SourceStatuses[index].LastError = reason
		}
	}
}

func normalizeSummaryFailureMessage(message, source string, reasonsBySource map[string]string) string {
	source = canonicalSource(source)
	if source == "" {
		return message
	}
	reason := strings.TrimSpace(reasonsBySource[source])
	if reason == "" {
		return message
	}
	if !runMessageLooksLikeBindingFailure(message, source) {
		return message
	}
	return reason
}

func bindingRetryReasonShouldOverrideFailure(binding SourceBindingState) bool {
	if strings.TrimSpace(binding.RetryReason) == "" {
		return false
	}
	if !binding.Runnable {
		return true
	}
	return binding.LastRunStatus == "failed"
}

func runMessageLooksLikeBindingFailure(message, source string) bool {
	message = strings.TrimSpace(message)
	if message == "" {
		return false
	}
	source = canonicalSource(source)

	switch source {
	case RunSourceWXCommentExport:
		lowerMessage := strings.ToLower(message)
		return strings.Contains(message, "cannot find the file specified") ||
			strings.Contains(message, "cannot find the path specified") ||
			strings.Contains(message, "系统找不到") ||
			strings.Contains(message, "The system cannot find") ||
			strings.Contains(message, "open ") ||
			strings.Contains(message, "deadline exceeded") ||
			strings.Contains(message, "client.timeout") ||
			strings.Contains(message, "没有就绪的视频号页面客户端") ||
			strings.Contains(message, "没有就绪的视频号客户端") ||
			strings.Contains(message, "无法连接到 wx_channel api") ||
			strings.Contains(message, "当前页面上下文与保存的评论导出视频不匹配") ||
			strings.Contains(lowerMessage, "jsapi_jsonparse_failed")
	case RunSourceWXFeedList, RunSourceWXVideoSearch:
		lowerMessage := strings.ToLower(message)
		return strings.Contains(lowerMessage, "username is required") ||
			strings.Contains(lowerMessage, "keyword is required") ||
			strings.Contains(lowerMessage, "@finder username") ||
			strings.Contains(lowerMessage, "no usable videos") ||
			strings.Contains(lowerMessage, "connectex") ||
			strings.Contains(lowerMessage, "connection refused") ||
			strings.Contains(lowerMessage, "actively refused") ||
			strings.Contains(lowerMessage, "deadline exceeded") ||
			strings.Contains(lowerMessage, "client.timeout") ||
			strings.Contains(lowerMessage, "ws/health") ||
			strings.Contains(lowerMessage, "无法连接到 wx_channel api") ||
			strings.Contains(lowerMessage, "没有就绪的视频号页面客户端") ||
			strings.Contains(lowerMessage, "没有就绪的视频号客户端")
	default:
		return false
	}
}

func buildWXVideoSearchBindingError(baseURL, reason string) error {
	baseURL = normalizeWXFeedListBaseURL(baseURL)
	switch strings.TrimSpace(reason) {
	case wxVideoSearchNoReadyClientReason:
		return normalizeWXFeedListMessage(baseURL, "no ready client")
	case wxVideoSearchHealthUnreachableReason:
		return fmt.Errorf("无法连接到 wx_channel API。请先启动 wx_channel 或 wx_channel_radar，并确认 %s 可访问", baseURL)
	case "":
		return fmt.Errorf("视频搜索来源当前不可用，请检查 %s 的连接状态", baseURL)
	default:
		return fmt.Errorf("%s", reason)
	}
}

func buildWXFeedListBindingError(baseURL, reason string) error {
	baseURL = normalizeWXFeedListBaseURL(baseURL)
	switch strings.TrimSpace(reason) {
	case "feed list has no ready page client":
		return normalizeWXFeedListMessage(baseURL, "no ready client")
	case "feed list ws/health is unreachable":
		return fmt.Errorf("无法连接到 wx_channel API。请先启动 wx_channel 或 wx_channel_radar，并确认 %s 可访问", baseURL)
	case "":
		return fmt.Errorf("视频列表来源当前不可用，请检查 %s 的连接状态", baseURL)
	default:
		return fmt.Errorf("%s", reason)
	}
}

func buildWXCommentExportBindingError(baseURL, reason string) error {
	baseURL = normalizeWXFeedListBaseURL(baseURL)
	switch strings.TrimSpace(reason) {
	case wxCommentExportNoReadyClientReason:
		return fmt.Errorf("wx_channel 已启动，但当前没有就绪的视频号客户端。请先在视频号独立客户端切到目标视频，等待 API 初始化完成后再重试评论导出")
	case wxCommentExportHealthUnreachableReason:
		return fmt.Errorf("无法连接到 wx_channel API。请先启动 wx_channel 或 wx_channel_radar，并确认 %s 可访问", baseURL)
	case "":
		return fmt.Errorf("评论导出来源当前不可用，请检查 %s 的连接状态", baseURL)
	default:
		return fmt.Errorf("%s", reason)
	}
}

func buildWXCommentExportContextMismatchDiagnostic(baseURL string, diagnostic *SourceBindingDiagnostic) *SourceBindingDiagnostic {
	copy := cloneSourceBindingDiagnostic(diagnostic)
	if copy == nil {
		copy = &SourceBindingDiagnostic{}
	}
	copy.Code = wxCommentExportContextMismatchCode
	if copy.ResolvedBaseURL == "" {
		copy.ResolvedBaseURL = normalizeWXFeedListBaseURL(baseURL)
	}
	if copy.StatusURL == "" && copy.ResolvedBaseURL != "" {
		copy.StatusURL = strings.TrimRight(copy.ResolvedBaseURL, "/") + "/api/channels/status"
	}
	return copy
}

func buildWXCommentExportContextMismatchDetail(diagnostic *SourceBindingDiagnostic) string {
	detail := "当前客户端上下文与保存的评论导出视频不匹配。请先在视频号独立客户端中切到目标视频，等待当前视频信息完全加载后再重试；如果这是旧绑定，请重新导出评论或重新绑定当前视频"
	if diagnostic != nil && strings.TrimSpace(diagnostic.CurrentPagePath) != "" {
		detail += " 当前客户端上下文 " + strings.TrimSpace(diagnostic.CurrentPagePath) + "。"
	}
	return detail
}

func isWXCommentExportContextMismatchMessage(message string) bool {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, "jsapi_jsonparse_failed") ||
		strings.Contains(trimmed, "当前页面上下文与保存的评论导出视频不匹配") ||
		strings.Contains(trimmed, "当前客户端上下文与保存的评论导出视频不匹配")
}

func buildSourceStatuses(items map[string]*SourceStatus, now time.Time) []SourceStatus {
	statuses := make([]SourceStatus, 0, len(items))
	for _, item := range items {
		status := *item
		switch {
		case !status.LastFailureAt.IsZero() && status.LastFailureAt.After(status.LastSuccessAt):
			status.State = "failing"
			status.FreshnessState = deriveSourceDataFreshness(status, now)
			if status.FreshnessState == "idle" {
				status.FreshnessState = "error"
			}
			status.SyncFreshnessState = "error"
		case !status.LastSuccessAt.IsZero():
			status.State = "ready"
			status.FreshnessState = deriveSourceDataFreshness(status, now)
			status.SyncFreshnessState = deriveFreshnessState(status.LastSuccessAt, now)
			if status.LastRunAt.Equal(status.LastSuccessAt) {
				status.LastError = ""
			}
		case !status.LastRunAt.IsZero():
			status.State = "degraded"
			status.FreshnessState = deriveSourceDataFreshness(status, now)
			if status.FreshnessState == "idle" {
				status.FreshnessState = "stale"
			}
			status.SyncFreshnessState = "stale"
		default:
			status.State = "idle"
			status.FreshnessState = "idle"
			status.SyncFreshnessState = "idle"
		}
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].LastRunAt.Equal(statuses[j].LastRunAt) {
			return statuses[i].Source < statuses[j].Source
		}
		return statuses[i].LastRunAt.After(statuses[j].LastRunAt)
	})
	return statuses
}

func deriveSourceDataFreshness(status SourceStatus, now time.Time) string {
	switch {
	case !status.LatestEventAt.IsZero():
		return deriveFreshnessState(status.LatestEventAt, now)
	case !status.LastSuccessAt.IsZero():
		return deriveFreshnessState(status.LastSuccessAt, now)
	case !status.LastRunAt.IsZero():
		return deriveFreshnessState(status.LastRunAt, now)
	default:
		return "idle"
	}
}

func deriveFreshnessState(latest, now time.Time) string {
	if latest.IsZero() {
		return "idle"
	}
	if now.IsZero() || latest.After(now) {
		now = latest
	}
	diff := now.Sub(latest)
	switch {
	case diff <= 30*time.Minute:
		return "fresh"
	case diff <= 6*time.Hour:
		return "lagging"
	default:
		return "stale"
	}
}

func findLatestEventBySource(events []Event, source string) *Event {
	for _, event := range events {
		if event.Source == source {
			copy := event
			return &copy
		}
	}
	return nil
}
