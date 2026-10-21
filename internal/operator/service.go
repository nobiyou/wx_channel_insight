package operator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

const operatorTerminalPersistenceTimeout = 10 * time.Second

type Options struct {
	HubUserID        string
	DeviceCount      int
	TaskType         string
	RequestedSources map[string]struct{}
	TargetRefs       []monitoring.BindingRef
	AccountTargets   []monitoring.TaskTarget
	Strict           bool
	TriggerMode      string
}

type Failure struct {
	Source    string `json:"source"`
	TargetKey string `json:"targetKey,omitempty"`
	Error     string `json:"error"`
}

type Skipped struct {
	Source    string `json:"source"`
	TargetKey string `json:"targetKey,omitempty"`
	Reason    string `json:"reason"`
}

type Result struct {
	Status           string
	FinalStatus      string
	AttemptedSources int
	Imports          []ImportedSummary
	Failures         []Failure
	Skipped          []Skipped
	ReadModel        insight.Dashboard
	StartedAt        time.Time
	FinishedAt       time.Time
	Message          string
	OperatorRunID    string
}

type ImportedSummary struct {
	Source  string
	Summary ingest.Summary
}

type ExecutionError struct {
	Err error
}

func (e ExecutionError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e ExecutionError) Unwrap() error {
	return e.Err
}

type Service struct {
	ingestService    *ingest.Service
	readModelService readModelRefresher
	now              func() time.Time
	cycleLocker      CycleLocker
	cycleRunning     atomic.Bool
	accountMonitor   *AccountMonitor
}

type readModelRefresher interface {
	RefreshDashboard(ctx context.Context, query insight.DashboardQuery, source string) (insight.Dashboard, error)
}

func NewService(ingestService *ingest.Service, readModelService readModelRefresher) (*Service, error) {
	if ingestService == nil {
		return nil, errors.New("ingest service is not configured")
	}
	if readModelService == nil {
		return nil, errors.New("read model refresh service is not configured")
	}
	return &Service{
		ingestService:    ingestService,
		readModelService: readModelService,
		now:              time.Now,
	}, nil
}

func (s *Service) SetCycleLocker(locker CycleLocker) {
	if s == nil {
		return
	}
	s.cycleLocker = locker
}

func (s *Service) SetAccountMonitor(monitor *AccountMonitor) {
	if s == nil {
		return
	}
	s.accountMonitor = monitor
}

func NormalizeRequestedSourcesCSV(value string) (map[string]struct{}, error) {
	requested := map[string]struct{}{}
	for _, part := range strings.Split(value, ",") {
		source := canonicalSource(part)
		if source == "" {
			return nil, fmt.Errorf("unsupported source in --sources: %s", strings.TrimSpace(part))
		}
		requested[source] = struct{}{}
	}
	if len(requested) == 0 {
		return nil, fmt.Errorf("--sources requires at least one supported source")
	}
	return requested, nil
}

func NormalizeRequestedSources(items []string) (map[string]struct{}, error) {
	if len(items) == 0 {
		return nil, nil
	}
	requested := map[string]struct{}{}
	for _, item := range items {
		source := canonicalSource(item)
		if source == "" {
			return nil, fmt.Errorf("unsupported source: %s", strings.TrimSpace(item))
		}
		requested[source] = struct{}{}
	}
	if len(requested) == 0 {
		return nil, fmt.Errorf("sources requires at least one supported source")
	}
	return requested, nil
}

func NormalizeTargetRefs(items []monitoring.BindingRef) ([]monitoring.BindingRef, error) {
	if len(items) == 0 {
		return nil, nil
	}
	refs := make([]monitoring.BindingRef, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		ref, err := monitoring.NormalizeBindingRef(item)
		if err != nil {
			return nil, err
		}
		key := ref.Source + "\x00" + ref.TargetKey
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate operator target %s[%s]", ref.Source, ref.TargetKey)
		}
		seen[key] = struct{}{}
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Source == refs[j].Source {
			return refs[i].TargetKey < refs[j].TargetKey
		}
		return refs[i].Source < refs[j].Source
	})
	return refs, nil
}

func NormalizeAccountTargets(items []monitoring.TaskTarget) ([]monitoring.TaskTarget, error) {
	if len(items) == 0 {
		return nil, nil
	}
	result := make([]monitoring.TaskTarget, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		normalized, err := monitoring.NormalizeTaskTarget(item)
		if err != nil {
			return nil, err
		}
		if normalized.TargetKind != monitoring.TargetKindAccount {
			return nil, fmt.Errorf("operator account target must use target kind %q", monitoring.TargetKindAccount)
		}
		if _, exists := seen[normalized.AccountID]; exists {
			return nil, fmt.Errorf("duplicate operator account target %s", normalized.AccountID)
		}
		seen[normalized.AccountID] = struct{}{}
		result = append(result, normalized)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].AccountID < result[j].AccountID })
	return result, nil
}

func (s *Service) SyncSavedBindings(ctx context.Context, options Options) (Result, error) {
	startedAt := s.now()
	failResult := func(message string) Result {
		return Result{
			Status:      "failed",
			FinalStatus: "failed",
			StartedAt:   startedAt,
			FinishedAt:  s.now(),
			Message:     message,
		}
	}
	targetRefs, err := NormalizeTargetRefs(options.TargetRefs)
	if err != nil {
		return failResult(err.Error()), err
	}
	accountTargets, err := NormalizeAccountTargets(options.AccountTargets)
	if err != nil {
		return failResult(err.Error()), err
	}

	query := ingest.SummaryQuery{
		HubUserID:   options.HubUserID,
		DeviceCount: options.DeviceCount,
	}
	if monitoring.IsSeparatedTaskType(options.TaskType) {
		return s.syncSeparatedAccountTargets(ctx, options, query, accountTargets, startedAt, failResult)
	}
	bindings, err := s.ingestService.ListBindings(ctx, query)
	if err != nil {
		return failResult(err.Error()), err
	}
	if len(bindings) == 0 && len(targetRefs) == 0 && len(accountTargets) == 0 {
		err := fmt.Errorf("no saved source bindings found")
		return failResult(err.Error()), err
	}
	bindingStates, err := s.ingestService.ListBindingStates(ctx, query)
	if err != nil {
		return failResult(err.Error()), err
	}

	availableBySource := make(map[string][]ingest.SourceBinding, len(bindings))
	availableByTarget := make(map[string]ingest.SourceBinding, len(bindings))
	for _, binding := range bindings {
		source := canonicalSource(binding.Source)
		if source == "" {
			continue
		}
		binding.Source = source
		binding = ingest.NormalizeSourceBinding(binding)
		availableBySource[source] = append(availableBySource[source], binding)
		availableByTarget[bindingTargetID(source, ingest.BindingTargetKey(binding), binding.Username, binding.Keyword)] = binding
	}
	expectedSources := expectedSourcesForOptions(options, availableBySource)
	if len(targetRefs) > 0 {
		expectedSources = targetSources(targetRefs)
	}
	stateByTarget := make(map[string]ingest.SourceBindingState, len(bindingStates))
	for _, item := range bindingStates {
		source := canonicalSource(item.Source)
		if source == "" {
			continue
		}
		item.Source = source
		stateByTarget[bindingTargetID(source, item.TargetKey, item.Username, item.Keyword)] = item
	}

	imports := make([]ImportedSummary, 0, len(bindings))
	failures := make([]Failure, 0)
	skipped := make([]Skipped, 0)
	attemptedSources := 0
	refreshDeviceCount := options.DeviceCount

	processBinding := func(source string, binding ingest.SourceBinding) {
		targetKey := ingest.BindingTargetKey(binding)
		attemptedSources++

		state, hasState := stateByTarget[bindingTargetID(source, targetKey, binding.Username, binding.Keyword)]
		if hasState && !state.Runnable {
			skipped = append(skipped, Skipped{Source: source, TargetKey: targetKey, Reason: firstNonEmpty(state.RetryReason, state.LastError, "saved binding is not runnable")})
			return
		}

		deviceCount := binding.DeviceCount
		if deviceCount <= 0 {
			deviceCount = options.DeviceCount
		}
		if refreshDeviceCount <= 0 && deviceCount > 0 {
			refreshDeviceCount = deviceCount
		}

		summary, skipReason, runErr := s.runSavedBindingImport(ctx, source, binding, ingest.SummaryQuery{
			HubUserID:   options.HubUserID,
			DeviceCount: deviceCount,
		})
		switch {
		case skipReason != "":
			skipped = append(skipped, Skipped{Source: source, TargetKey: targetKey, Reason: skipReason})
		case runErr != nil:
			failures = append(failures, Failure{Source: source, TargetKey: targetKey, Error: runErr.Error()})
		default:
			imports = append(imports, ImportedSummary{Source: source, Summary: summary})
		}
	}

	processAccount := func(target monitoring.TaskTarget) {
		targetKey := target.AccountID
		attemptedSources++
		if s.accountMonitor == nil {
			failures = append(failures, Failure{Source: monitoring.TargetKindAccount, TargetKey: targetKey, Error: "account monitor is not configured"})
			return
		}
		monitorResult, runErr := s.accountMonitor.RunTask(ctx, query, target, options.TaskType)
		if strings.TrimSpace(monitorResult.Summary.HubUserID) != "" {
			imports = append(imports, ImportedSummary{Source: monitoring.TargetKindAccount, Summary: monitorResult.Summary})
		}
		if runErr != nil {
			failures = append(failures, Failure{Source: monitoring.TargetKindAccount, TargetKey: targetKey, Error: runErr.Error()})
			return
		}
		if len(monitorResult.Errors) == 0 {
			if monitorResult.Status == monitoring.RunStatusSkipped {
				skipped = append(skipped, Skipped{Source: monitoring.TargetKindAccount, TargetKey: targetKey, Reason: "账号身份不完整，请先确认账号 ID"})
			}
			return
		}
		for _, item := range monitorResult.Errors {
			message := strings.TrimSpace(item.Message)
			if message == "" {
				message = "账号目标未提供错误详情"
			}
			if item.VideoObjectID != "" {
				message = fmt.Sprintf("视频[%s]: %s", item.VideoObjectID, message)
			}
			switch item.Status {
			case monitoring.RunStatusSkipped:
				skipped = append(skipped, Skipped{Source: monitoring.TargetKindAccount, TargetKey: targetKey, Reason: message})
			default:
				failures = append(failures, Failure{Source: monitoring.TargetKindAccount, TargetKey: targetKey, Error: message})
			}
		}
	}

	for _, target := range accountTargets {
		processAccount(target)
	}

	if len(targetRefs) > 0 || len(accountTargets) > 0 {
		for _, ref := range targetRefs {
			binding, ok := availableByTarget[bindingTargetID(ref.Source, ref.TargetKey, "", "")]
			if !ok {
				attemptedSources++
				skipped = append(skipped, Skipped{Source: ref.Source, TargetKey: ref.TargetKey, Reason: "saved target binding not found"})
				continue
			}
			processBinding(ref.Source, binding)
		}
	} else {
		for _, source := range expectedSources {
			targets, ok := availableBySource[source]
			if !ok || len(targets) == 0 {
				skipped = append(skipped, Skipped{Source: source, Reason: missingSourceReason(source)})
				continue
			}
			targets = selectAutomaticBindingTargets(source, targets, options)
			for _, binding := range targets {
				processBinding(source, binding)
			}
		}
	}

	if attemptedSources == 0 {
		if len(targetRefs) > 0 || len(accountTargets) > 0 {
			err := fmt.Errorf("no selected saved bindings matched operator targets")
			return failResult(err.Error()), err
		}
		if len(options.RequestedSources) > 0 {
			err := fmt.Errorf("no saved bindings matched requested sources")
			return failResult(err.Error()), err
		}
		err := fmt.Errorf("no supported saved bindings found")
		return failResult(err.Error()), err
	}
	if len(imports) == 0 {
		if len(accountTargets) > 0 && len(targetRefs) == 0 && len(failures) == 0 && len(skipped) > 0 {
			result := failResult("账号目标已跳过：" + buildSyncSavedBindingsFailure(failures, skipped).Error())
			result.Status = monitoring.RunStatusSkipped
			result.FinalStatus = monitoring.RunStatusSkipped
			result.AttemptedSources = attemptedSources
			result.Failures = failures
			result.Skipped = skipped
			return result, nil
		}
		err := buildSyncSavedBindingsFailure(failures, skipped)
		result := failResult(err.Error())
		result.AttemptedSources = attemptedSources
		result.Failures = failures
		result.Skipped = skipped
		return result, err
	}

	readModel, err := s.readModelService.RefreshDashboard(ctx, insight.DashboardQuery{
		HubUserID:     options.HubUserID,
		DeviceCount:   refreshDeviceCount,
		ContextSource: "jobs",
	}, "jobs")
	if err != nil {
		result := failResult(err.Error())
		result.AttemptedSources = attemptedSources
		result.Imports = imports
		result.Failures = failures
		result.Skipped = skipped
		return result, err
	}

	status := "ok"
	if len(failures) > 0 || len(skipped) > 0 {
		status = "partial"
	}
	for _, item := range imports {
		if item.Summary.Collection != nil && !item.Summary.Collection.Complete {
			status = "partial"
		}
	}
	finalStatus := status
	if options.Strict && status != "ok" {
		finalStatus = "failed"
	}
	result := Result{
		Status:           status,
		FinalStatus:      finalStatus,
		AttemptedSources: attemptedSources,
		Imports:          imports,
		Failures:         failures,
		Skipped:          skipped,
		ReadModel:        readModel,
		StartedAt:        startedAt,
		FinishedAt:       s.now(),
	}
	if options.Strict && status != "ok" {
		result.Message = buildStrictCycleFailure(status, failures, skipped)
		return result, ExecutionError{Err: errors.New(result.Message)}
	}
	return result, nil
}

func (s *Service) RunCycle(ctx context.Context, options Options) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("operator cycle context is nil")
	}
	release, err := s.acquireCycle(ctx, cycleLockKey(options))
	if err != nil {
		return Result{}, err
	}
	defer release()

	result, err := s.SyncSavedBindings(ctx, options)
	if result.FinishedAt.IsZero() {
		result.FinishedAt = s.now()
	}
	operatorRunID := result.OperatorRunID
	if operatorRunID == "" {
		operatorRunID = buildOperatorRunID(result.FinishedAt, s.now)
	}
	result.OperatorRunID = ""
	cycleErr := err
	if cycleErr != nil {
		result.FinalStatus = "failed"
		result.Message = cycleErr.Error()
	}

	queryHubUserID := options.HubUserID
	if queryHubUserID == "" {
		queryHubUserID = result.ReadModel.WorkspaceBridge.HubUserID
	}
	persistedResult := result
	persistedResult.OperatorRunID = operatorRunID
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), operatorTerminalPersistenceTimeout)
	err = s.appendOperatorRun(persistCtx, queryHubUserID, persistedResult, options)
	cancelPersist()
	if err != nil {
		persistErr := fmt.Errorf("persist operator run: %w", err)
		if cycleErr != nil {
			return result, errors.Join(ExecutionError{Err: cycleErr}, persistErr)
		}
		return result, persistErr
	}
	result.OperatorRunID = operatorRunID
	if cycleErr != nil {
		return result, ExecutionError{Err: cycleErr}
	}
	return result, nil
}

func (s *Service) acquireCycle(ctx context.Context, key string) (func(), error) {
	if !s.cycleRunning.CompareAndSwap(false, true) {
		return nil, ErrCycleAlreadyRunning
	}
	releaseLocal := func() {
		s.cycleRunning.Store(false)
	}
	if s.cycleLocker == nil {
		return releaseLocal, nil
	}

	releaseExternal, acquired, err := s.cycleLocker.TryAcquire(ctx, key)
	if err != nil {
		releaseLocal()
		return nil, fmt.Errorf("acquire operator cycle lock: %w", err)
	}
	if !acquired {
		releaseLocal()
		return nil, ErrCycleAlreadyRunning
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			if releaseExternal != nil {
				releaseExternal()
			}
			releaseLocal()
		})
	}, nil
}

func cycleLockKey(options Options) string {
	if key := strings.TrimSpace(options.HubUserID); key != "" {
		return key
	}
	return "insight:operator:local"
}

func (s *Service) appendOperatorRun(ctx context.Context, hubUserID string, result Result, options Options) error {
	topVideoTitle := ""
	if len(result.ReadModel.HotVideos) > 0 {
		topVideoTitle = result.ReadModel.HotVideos[0].Title
	}
	topTopicKeyword := ""
	if len(result.ReadModel.HotTopics) > 0 {
		topTopicKeyword = result.ReadModel.HotTopics[0].Keyword
	}
	message := ""
	if result.Message != "" {
		message = result.Message
	} else if len(result.Failures) > 0 {
		parts := make([]string, 0, len(result.Failures))
		for _, failure := range result.Failures {
			parts = append(parts, fmt.Sprintf("%s=%s", failure.Source, failure.Error))
		}
		message = strings.Join(parts, "; ")
	}
	triggerMode := strings.TrimSpace(options.TriggerMode)
	if triggerMode == "" {
		triggerMode = ingest.OperatorTriggerCycle
	}
	operatorRunID := result.OperatorRunID
	if operatorRunID == "" {
		operatorRunID = buildOperatorRunID(result.FinishedAt, s.now)
	}
	return s.ingestService.AppendOperatorRun(ctx, ingest.SummaryQuery{
		HubUserID: hubUserID,
	}, ingest.OperatorRun{
		ID:               operatorRunID,
		Status:           BuildCycleStatus(result),
		TriggerMode:      triggerMode,
		StartedAt:        result.StartedAt,
		FinishedAt:       result.FinishedAt,
		WorkspaceID:      result.ReadModel.WorkspaceBridge.WorkspaceID,
		LatestBatchID:    result.ReadModel.ReadModel.LatestBatchID,
		AttemptedSources: result.AttemptedSources,
		SucceededSources: len(result.Imports),
		FailedSources:    len(result.Failures),
		SkippedSources:   len(result.Skipped),
		FreshnessState:   result.ReadModel.IngestSummary.FreshnessState,
		ReportHeadline:   result.ReadModel.ReportSummary.Headline,
		TopVideoTitle:    topVideoTitle,
		TopTopicKeyword:  topTopicKeyword,
		Message:          message,
		Outcomes:         buildOperatorRunOutcomes(result),
	})
}

func (s *Service) runSavedBindingImport(ctx context.Context, source string, binding ingest.SourceBinding, query ingest.SummaryQuery) (ingest.Summary, string, error) {
	assessment := s.ingestService.AssessBinding(source, binding)
	if !assessment.Runnable {
		return ingest.Summary{}, assessment.Reason, nil
	}

	switch source {
	case ingest.RunSourceWXCommentExport:
		summary, err := s.ingestService.RefreshWXCommentExportBinding(ctx, query, binding)
		return summary, "", err
	case ingest.RunSourceWXFeedList:
		summary, err := s.ingestService.ImportWXFeedList(ctx, query, binding.BaseURL, binding.Username, "")
		return summary, "", err
	case ingest.RunSourceWXVideoSearch:
		summary, err := s.ingestService.ImportWXVideoSearch(ctx, query, binding.BaseURL, binding.Keyword, "")
		return summary, "", err
	case ingest.RunSourceWXManualShare:
		summary, err := s.ingestService.RefreshWXVideoShareBinding(ctx, query, binding)
		return summary, "", err
	default:
		return ingest.Summary{}, "", fmt.Errorf("saved binding source %s is not supported", source)
	}
}

func canonicalSource(source string) string {
	switch strings.TrimSpace(source) {
	case ingest.RunSourceWXCommentExport:
		return ingest.RunSourceWXCommentExport
	case ingest.RunSourceWXFeedList:
		return ingest.RunSourceWXFeedList
	case ingest.RunSourceWXVideoSearch:
		return ingest.RunSourceWXVideoSearch
	case ingest.RunSourceWXManualShare:
		return ingest.RunSourceWXManualShare
	default:
		return ""
	}
}

func buildSyncSavedBindingsFailure(failures []Failure, skipped []Skipped) error {
	parts := make([]string, 0, len(failures)+len(skipped))
	for _, item := range failures {
		parts = append(parts, formatTargetResult(item.Source, item.TargetKey, item.Error))
	}
	for _, item := range skipped {
		parts = append(parts, formatTargetResult(item.Source, item.TargetKey, item.Reason))
	}
	if len(parts) == 0 {
		return fmt.Errorf("sync-saved-bindings did not find runnable sources")
	}
	return fmt.Errorf("sync-saved-bindings did not complete any source: %s", strings.Join(parts, "; "))
}

func expectedSourcesForOptions(options Options, availableBySource map[string][]ingest.SourceBinding) []string {
	if len(options.RequestedSources) > 0 {
		expected := make([]string, 0, len(options.RequestedSources))
		for _, source := range []string{ingest.RunSourceWXCommentExport, ingest.RunSourceWXVideoSearch, ingest.RunSourceWXFeedList, ingest.RunSourceWXManualShare} {
			if _, ok := options.RequestedSources[source]; ok {
				expected = append(expected, source)
			}
		}
		for source := range options.RequestedSources {
			if source == ingest.RunSourceWXCommentExport || source == ingest.RunSourceWXVideoSearch || source == ingest.RunSourceWXFeedList || source == ingest.RunSourceWXManualShare {
				continue
			}
			expected = append(expected, source)
		}
		return expected
	}

	_, hasVideoSearch := availableBySource[ingest.RunSourceWXVideoSearch]
	_, hasFeedList := availableBySource[ingest.RunSourceWXFeedList]
	expected := []string{ingest.RunSourceWXCommentExport}
	if hasVideoSearch {
		expected = append(expected, ingest.RunSourceWXVideoSearch)
	} else if hasFeedList {
		expected = append(expected, ingest.RunSourceWXFeedList)
	} else {
		expected = append(expected, ingest.RunSourceWXVideoSearch)
	}
	if hasVideoSearch && hasFeedList {
		expected = append(expected, ingest.RunSourceWXFeedList)
	}
	if _, hasManualShare := availableBySource[ingest.RunSourceWXManualShare]; hasManualShare {
		expected = append(expected, ingest.RunSourceWXManualShare)
	}
	return expected
}

// selectAutomaticBindingTargets keeps the single wx_channel page client from
// being asked to serve multiple comment-export pages in one unattended cycle.
// Manual cycles retain their historical all-bindings behavior, while explicit
// target references always take precedence over this guard.
func selectAutomaticBindingTargets(source string, bindings []ingest.SourceBinding, options Options) []ingest.SourceBinding {
	if source != ingest.RunSourceWXCommentExport || len(bindings) <= 1 || len(options.TargetRefs) > 0 || !isAutomaticTriggerMode(options.TriggerMode) {
		return bindings
	}

	latestIndex := 0
	for index := 1; index < len(bindings); index++ {
		current := bindings[index]
		latest := bindings[latestIndex]
		if current.UpdatedAt.After(latest.UpdatedAt) {
			latestIndex = index
			continue
		}
		if current.UpdatedAt.Equal(latest.UpdatedAt) && ingest.BindingTargetKey(current) > ingest.BindingTargetKey(latest) {
			latestIndex = index
		}
	}
	return []ingest.SourceBinding{bindings[latestIndex]}
}

func isAutomaticTriggerMode(triggerMode string) bool {
	switch strings.TrimSpace(triggerMode) {
	case ingest.OperatorTriggerLoop, ingest.OperatorTriggerScheduled:
		return true
	default:
		return false
	}
}

func targetSources(refs []monitoring.BindingRef) []string {
	seen := make(map[string]struct{}, len(refs))
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, exists := seen[ref.Source]; exists {
			continue
		}
		seen[ref.Source] = struct{}{}
		result = append(result, ref.Source)
	}
	return result
}

func bindingTargetID(source, targetKey, username, keyword string) string {
	if targetKey == "" {
		targetKey = ingest.BindingTargetKey(ingest.SourceBinding{
			Source:   source,
			Username: username,
			Keyword:  keyword,
		})
	}
	return source + "\x00" + targetKey
}

func formatTargetResult(source, targetKey, detail string) string {
	if targetKey == "" {
		return fmt.Sprintf("%s: %s", source, detail)
	}
	return fmt.Sprintf("%s[%s]: %s", source, targetKey, detail)
}

func missingSourceReason(source string) string {
	switch source {
	case ingest.RunSourceWXCommentExport:
		return "评论导出来源尚未保存，今天这轮会缺少评论洞察输入"
	case ingest.RunSourceWXFeedList:
		return "视频列表来源尚未保存，今天这轮会缺少热视频和热话题输入"
	case ingest.RunSourceWXVideoSearch:
		return "视频搜索来源尚未保存，今天这轮会缺少关键词候选热视频输入"
	default:
		return "当前来源尚未保存"
	}
}

func buildStrictCycleFailure(status string, failures []Failure, skipped []Skipped) string {
	if status == "partial" {
		parts := make([]string, 0, len(failures)+len(skipped))
		for _, item := range failures {
			parts = append(parts, fmt.Sprintf("%s=%s", item.Source, item.Error))
		}
		for _, item := range skipped {
			parts = append(parts, fmt.Sprintf("%s=%s", item.Source, item.Reason))
		}
		if len(parts) == 0 {
			return "strict operator cycle requires every requested source to complete successfully"
		}
		return "strict operator cycle requires every requested source to complete successfully: " + strings.Join(parts, "; ")
	}
	return "strict operator cycle failed"
}

func buildOperatorRunOutcomes(result Result) []ingest.OperatorRunOutcome {
	outcomes := make([]ingest.OperatorRunOutcome, 0, len(result.Imports)+len(result.Failures)+len(result.Skipped))
	for _, item := range result.Imports {
		status := "success"
		if item.Summary.Collection != nil && !item.Summary.Collection.Complete {
			status = "partial"
		}
		outcomes = append(outcomes, ingest.OperatorRunOutcome{
			Source:  item.Source,
			Status:  status,
			Summary: buildImportOutcomeSummary(item.Source, item.Summary),
		})
	}
	for _, item := range result.Failures {
		outcomes = append(outcomes, ingest.OperatorRunOutcome{
			Source:  item.Source,
			Status:  "failed",
			Summary: item.Error,
		})
	}
	for _, item := range result.Skipped {
		outcomes = append(outcomes, ingest.OperatorRunOutcome{
			Source:  item.Source,
			Status:  "skipped",
			Summary: item.Reason,
		})
	}
	return outcomes
}

func buildImportOutcomeSummary(source string, summary ingest.Summary) string {
	if summary.Collection != nil && !summary.Collection.Complete {
		return summary.Collection.Notice
	}
	if scoped, ok := findSourceOutcomeSummary(source, summary.SourceStatuses); ok {
		return scoped
	}

	parts := make([]string, 0, 3)
	if summary.LatestBatchID != "" {
		parts = append(parts, "批次 "+summary.LatestBatchID)
	}
	if summary.LastEventType != "" {
		parts = append(parts, "事件 "+summary.LastEventType)
	}
	if summary.TotalEvents > 0 {
		parts = append(parts, fmt.Sprintf("%d 条事件", summary.TotalEvents))
	}
	return strings.Join(parts, " · ")
}

func findSourceOutcomeSummary(source string, statuses []ingest.SourceStatus) (string, bool) {
	for _, item := range statuses {
		if item.Source != source {
			continue
		}

		parts := make([]string, 0, 3)
		if item.LatestBatchID != "" {
			parts = append(parts, "批次 "+item.LatestBatchID)
		}
		if item.LastEventType != "" {
			parts = append(parts, "事件 "+item.LastEventType)
		}
		if item.EventCount > 0 {
			parts = append(parts, fmt.Sprintf("%d 条事件", item.EventCount))
		}
		if len(parts) == 0 {
			return "", false
		}
		return strings.Join(parts, " · "), true
	}

	return "", false
}

func buildOperatorRunID(finishedAt time.Time, now func() time.Time) string {
	if finishedAt.IsZero() {
		finishedAt = now()
	}
	return "operator-cycle-" + finishedAt.UTC().Format("20060102T150405.000000000Z")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
