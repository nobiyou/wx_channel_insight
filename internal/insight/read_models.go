package insight

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type ReadModelService struct {
	dashboardService      *DashboardService
	reportSummaryRepo     ReportSummaryRepository
	rankingRepo           RankingRepository
	commentInsightRepo    CommentInsightRepository
	exclusionRepo         DataCenterExclusionRepository
	recommendationService *RecommendationService
	contextResolver       WorkspaceContextResolver
	readCache             *ReadCache
}

type ReadModelServiceOption func(*ReadModelService)

func WithReportSummaryRepository(repo ReportSummaryRepository) ReadModelServiceOption {
	return func(service *ReadModelService) {
		service.reportSummaryRepo = repo
	}
}

func WithWorkspaceContextResolver(resolver WorkspaceContextResolver) ReadModelServiceOption {
	return func(service *ReadModelService) {
		service.contextResolver = resolver
	}
}

func WithReadCache(cache *ReadCache) ReadModelServiceOption {
	return func(service *ReadModelService) {
		service.readCache = cache
	}
}

func WithCommentInsightRepository(repo CommentInsightRepository) ReadModelServiceOption {
	return func(service *ReadModelService) {
		service.commentInsightRepo = repo
	}
}

func WithRankingRepository(repo RankingRepository) ReadModelServiceOption {
	return func(service *ReadModelService) {
		service.rankingRepo = repo
	}
}

func WithRecommendationService(service *RecommendationService) ReadModelServiceOption {
	return func(readModelService *ReadModelService) {
		readModelService.recommendationService = service
	}
}

func WithDataCenterExclusionRepository(repo DataCenterExclusionRepository) ReadModelServiceOption {
	return func(service *ReadModelService) {
		service.exclusionRepo = repo
	}
}

func NewReadModelService(dashboardService *DashboardService, options ...ReadModelServiceOption) *ReadModelService {
	service := &ReadModelService{
		dashboardService: dashboardService,
	}
	if dashboardService != nil {
		service.contextResolver = dashboardService.contextResolver
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *ReadModelService) GetHotVideos(ctx context.Context, query DashboardQuery) ([]HotVideo, error) {
	window, ok := NormalizeTimeWindow(query.TimeWindow)
	if !ok {
		return nil, ErrInvalidTimeWindow
	}
	query.TimeWindow = window
	if window != TimeWindowAll && s.dashboardService != nil {
		payload, err := s.dashboardService.GetReadOnlyDashboard(ctx, query)
		if err != nil {
			return nil, err
		}
		return filterHotVideosByTimeWindow(payload.HotVideos, window, dashboardReferenceAt(payload, query)), nil
	}
	workspaceID, err := s.resolveWorkspaceID(ctx, query)
	if err != nil {
		return nil, err
	}
	if s.readCache != nil {
		if items, found, err := s.readCache.LoadHotVideos(ctx, workspaceID); err == nil && found {
			filtered, filterErr := s.filterHotVideos(ctx, workspaceID, items, query)
			return filtered, filterErr
		}
	}
	if s.rankingRepo != nil {
		items, found, err := s.rankingRepo.GetHotVideos(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		if found {
			if s.readCache != nil {
				s.readCache.RecordScopeFallback(readCacheScopeHotVideos, time.Now())
				_ = s.readCache.SaveHotVideos(ctx, workspaceID, items)
			}
			filtered, err := s.filterHotVideos(ctx, workspaceID, items, query)
			return filtered, err
		}
		return []HotVideo{}, nil
	}
	payload, err := s.dashboardService.GetReadOnlyDashboard(ctx, query)
	if err != nil {
		return nil, err
	}
	if s.readCache != nil {
		s.readCache.RecordScopeFallback(readCacheScopeHotVideos, time.Now())
		_ = s.readCache.SaveHotVideos(ctx, workspaceID, payload.HotVideos)
	}
	filtered, err := s.filterHotVideos(ctx, workspaceID, payload.HotVideos, query)
	return filtered, err
}

func (s *ReadModelService) GetHotVideosByRanking(ctx context.Context, query DashboardQuery, ranking string) ([]HotVideo, error) {
	kind, ok := NormalizeHotVideoRanking(ranking)
	if !ok {
		return nil, fmt.Errorf("unsupported hot video ranking %q", ranking)
	}
	window, ok := NormalizeTimeWindow(query.TimeWindow)
	if !ok {
		return nil, ErrInvalidTimeWindow
	}
	query.TimeWindow = window
	if kind == HotVideoRankingHot {
		return s.GetHotVideos(ctx, query)
	}
	if window != TimeWindowAll && s.dashboardService != nil {
		payload, err := s.dashboardService.GetReadOnlyDashboard(ctx, query)
		if err != nil {
			return nil, err
		}
		return filterHotVideosByTimeWindow(payload.HotVideoRankings[kind], window, dashboardReferenceAt(payload, query)), nil
	}
	workspaceID, err := s.resolveWorkspaceID(ctx, query)
	if err != nil {
		return nil, err
	}
	if s.rankingRepo != nil {
		rankings, found, err := s.rankingRepo.GetHotVideoRankings(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		if found {
			if items, exists := rankings[kind]; exists {
				filtered, err := s.filterHotVideos(ctx, workspaceID, items, query)
				return filtered, err
			}
		}
	}
	if s.dashboardService == nil {
		return []HotVideo{}, nil
	}
	payload, err := s.dashboardService.GetReadOnlyDashboard(ctx, query)
	if err != nil {
		return nil, err
	}
	filtered, err := s.filterHotVideos(ctx, workspaceID, payload.HotVideoRankings[kind], query)
	return filtered, err
}

func (s *ReadModelService) GetHotTopics(ctx context.Context, query DashboardQuery) ([]HotTopic, error) {
	window, ok := NormalizeTimeWindow(query.TimeWindow)
	if !ok {
		return nil, ErrInvalidTimeWindow
	}
	query.TimeWindow = window
	workspaceID, err := s.resolveWorkspaceID(ctx, query)
	if err != nil {
		return nil, err
	}
	if s.readCache != nil {
		if items, found, err := s.readCache.LoadHotTopics(ctx, workspaceID); err == nil && found {
			filtered, filterErr := s.filterHotTopics(ctx, workspaceID, items, query)
			return filtered, filterErr
		}
	}
	if s.rankingRepo != nil {
		items, found, err := s.rankingRepo.GetHotTopics(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		if found {
			if s.readCache != nil {
				s.readCache.RecordScopeFallback(readCacheScopeHotTopics, time.Now())
				_ = s.readCache.SaveHotTopics(ctx, workspaceID, items)
			}
			filtered, err := s.filterHotTopics(ctx, workspaceID, items, query)
			return filtered, err
		}
		return []HotTopic{}, nil
	}
	payload, err := s.dashboardService.GetReadOnlyDashboard(ctx, query)
	if err != nil {
		return nil, err
	}
	if s.readCache != nil {
		s.readCache.RecordScopeFallback(readCacheScopeHotTopics, time.Now())
		_ = s.readCache.SaveHotTopics(ctx, workspaceID, payload.HotTopics)
	}
	filtered, err := s.filterHotTopics(ctx, workspaceID, payload.HotTopics, query)
	return filtered, err
}

func (s *ReadModelService) GetCommentInsights(ctx context.Context, query DashboardQuery) ([]CommentInsight, error) {
	window, ok := NormalizeTimeWindow(query.TimeWindow)
	if !ok {
		return nil, ErrInvalidTimeWindow
	}
	query.TimeWindow = window
	workspaceID, err := s.resolveWorkspaceID(ctx, query)
	if err != nil {
		return nil, err
	}
	if s.readCache != nil {
		if items, found, err := s.readCache.LoadCommentInsights(ctx, workspaceID); err == nil && found {
			filtered, filterErr := s.filterCommentInsights(ctx, workspaceID, items, query)
			return filtered, filterErr
		}
	}
	if s.commentInsightRepo != nil {
		items, found, err := s.commentInsightRepo.GetCommentInsights(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		if found {
			if s.readCache != nil {
				s.readCache.RecordScopeFallback(readCacheScopeCommentInsights, time.Now())
				_ = s.readCache.SaveCommentInsights(ctx, workspaceID, items)
			}
			filtered, err := s.filterCommentInsights(ctx, workspaceID, items, query)
			return filtered, err
		}
	}
	payload, err := s.dashboardService.GetReadOnlyDashboard(ctx, query)
	if err != nil {
		return nil, err
	}
	if s.commentInsightRepo != nil {
		_ = s.commentInsightRepo.ReplaceCommentInsights(ctx, workspaceID, payload.CommentInsights)
	}
	if s.readCache != nil {
		s.readCache.RecordScopeFallback(readCacheScopeCommentInsights, time.Now())
		_ = s.readCache.SaveCommentInsights(ctx, workspaceID, payload.CommentInsights)
	}
	filtered, err := s.filterCommentInsights(ctx, workspaceID, payload.CommentInsights, query)
	return filtered, err
}

func (s *ReadModelService) GetReportSummary(ctx context.Context, query DashboardQuery) (ReportSummary, error) {
	window, ok := NormalizeTimeWindow(query.TimeWindow)
	if !ok {
		return ReportSummary{}, ErrInvalidTimeWindow
	}
	query.TimeWindow = window
	if window != TimeWindowAll && s.dashboardService != nil {
		payload, err := s.dashboardService.GetReadOnlyDashboard(ctx, query)
		if err != nil {
			return ReportSummary{}, err
		}
		return payload.ReportSummary, nil
	}

	workspaceID, err := s.resolveWorkspaceID(ctx, query)
	if err != nil {
		return ReportSummary{}, err
	}
	if s.readCache != nil {
		if payload, found, err := s.readCache.LoadReportSummary(ctx, workspaceID); err == nil && found {
			return payload, nil
		}
	}
	if s.reportSummaryRepo != nil {
		summary, found, err := s.reportSummaryRepo.GetReportSummary(ctx, workspaceID)
		if err != nil {
			return ReportSummary{}, err
		}
		if found {
			if s.readCache != nil {
				s.readCache.RecordScopeFallback(readCacheScopeReportSummary, time.Now())
				_ = s.readCache.SaveReportSummary(ctx, workspaceID, summary)
			}
			return summary, nil
		}
	}
	payload, err := s.dashboardService.GetReadOnlyDashboard(ctx, query)
	if err != nil {
		return ReportSummary{}, err
	}
	if s.reportSummaryRepo != nil {
		_ = s.reportSummaryRepo.ReplaceReportSummary(ctx, workspaceID, payload.ReportSummary)
	}
	if s.readCache != nil {
		s.readCache.RecordScopeFallback(readCacheScopeReportSummary, time.Now())
		_ = s.readCache.SaveReportSummary(ctx, workspaceID, payload.ReportSummary)
	}
	return payload.ReportSummary, nil
}

func (s *ReadModelService) GetRecommendations(ctx context.Context, query DashboardQuery) (RecommendationSnapshot, error) {
	if s == nil || s.recommendationService == nil {
		return RecommendationSnapshot{}, errors.New("recommendation service is not configured")
	}
	window, ok := NormalizeTimeWindow(query.TimeWindow)
	if !ok {
		return RecommendationSnapshot{}, ErrInvalidTimeWindow
	}
	query.TimeWindow = window
	workspaceID, err := s.resolveWorkspaceID(ctx, query)
	if err != nil {
		return RecommendationSnapshot{}, err
	}
	var snapshot RecommendationSnapshot
	found := false
	if s.recommendationService.repository != nil {
		snapshot, found, err = s.recommendationService.GetPersisted(ctx, workspaceID)
		if err != nil {
			return RecommendationSnapshot{}, err
		}
	}
	if !found {
		snapshot, err = s.recommendationService.Get(ctx, query)
		if err != nil {
			return RecommendationSnapshot{}, err
		}
	}
	payload := Dashboard{
		AccountRecommendations: snapshot.AccountRecommendations,
		VideoRecommendations:   snapshot.VideoRecommendations,
	}
	if err := FilterDashboardByDataCenterExclusions(ctx, &payload, workspaceID, s.exclusionRepo); err != nil {
		return RecommendationSnapshot{}, err
	}
	snapshot.AccountRecommendations = payload.AccountRecommendations
	snapshot.VideoRecommendations = payload.VideoRecommendations
	if window != TimeWindowAll {
		referenceAt := snapshot.GeneratedAt
		if !query.ReferenceAt.IsZero() {
			referenceAt = query.ReferenceAt
		}
		snapshot.AccountRecommendations = filterAccountRecommendationsByTimeWindow(snapshot.AccountRecommendations, window, referenceAt)
		snapshot.VideoRecommendations = filterVideoRecommendationsByTimeWindow(snapshot.VideoRecommendations, window, referenceAt)
		snapshot.TopicRecommendations = filterTopicRecommendationsByTimeWindow(snapshot.TopicRecommendations, window, referenceAt)
	}
	return snapshot, nil
}

func (s *ReadModelService) GetAccountRecommendations(ctx context.Context, query DashboardQuery) ([]AccountRecommendation, RecommendationSnapshot, error) {
	snapshot, err := s.GetRecommendations(ctx, query)
	if err != nil {
		return nil, RecommendationSnapshot{}, err
	}
	return append([]AccountRecommendation(nil), snapshot.AccountRecommendations...), snapshot, nil
}

func (s *ReadModelService) GetVideoRecommendations(ctx context.Context, query DashboardQuery) ([]VideoRecommendation, RecommendationSnapshot, error) {
	snapshot, err := s.GetRecommendations(ctx, query)
	if err != nil {
		return nil, RecommendationSnapshot{}, err
	}
	return append([]VideoRecommendation(nil), snapshot.VideoRecommendations...), snapshot, nil
}

func (s *ReadModelService) GetViewStatus(ctx context.Context, query DashboardQuery, key string) (ReadModelViewStatus, bool, error) {
	if s.dashboardService == nil {
		return ReadModelViewStatus{}, false, nil
	}

	payload, err := s.dashboardService.GetReadOnlyDashboard(ctx, query)
	if err != nil {
		return ReadModelViewStatus{}, false, err
	}

	for _, item := range payload.ReadModel.ViewStatuses {
		if item.Key == key {
			return item, true, nil
		}
	}
	return ReadModelViewStatus{}, false, nil
}

func (s *ReadModelService) resolveWorkspaceID(ctx context.Context, query DashboardQuery) (string, error) {
	if s.contextResolver == nil {
		return "", nil
	}
	resolved, err := s.contextResolver.Resolve(ctx, workspace.ContextQuery{
		HubUserID:     query.HubUserID,
		DeviceCount:   query.DeviceCount,
		ContextSource: query.ContextSource,
	})
	if err != nil {
		return "", err
	}
	return resolved.Workspace.ID, nil
}

func (s *ReadModelService) filterHotVideos(ctx context.Context, workspaceID string, items []HotVideo, query DashboardQuery) ([]HotVideo, error) {
	window, ok := NormalizeTimeWindow(query.TimeWindow)
	if !ok {
		return nil, ErrInvalidTimeWindow
	}
	if window != TimeWindowAll {
		items = filterHotVideosByTimeWindow(items, window, s.timeWindowReference(ctx, workspaceID, query, "videos"))
	}
	payload := Dashboard{HotVideos: cloneHotVideos(items)}
	if err := FilterDashboardByDataCenterExclusions(ctx, &payload, workspaceID, s.exclusionRepo); err != nil {
		return nil, err
	}
	return payload.HotVideos, nil
}

func (s *ReadModelService) filterHotTopics(ctx context.Context, workspaceID string, items []HotTopic, query DashboardQuery) ([]HotTopic, error) {
	window, ok := NormalizeTimeWindow(query.TimeWindow)
	if !ok {
		return nil, ErrInvalidTimeWindow
	}
	if window != TimeWindowAll {
		items = filterHotTopicsByTimeWindow(items, window, s.timeWindowReference(ctx, workspaceID, query, "topics"))
	}
	payload := Dashboard{HotTopics: cloneHotTopics(items)}
	if err := FilterDashboardByDataCenterExclusions(ctx, &payload, workspaceID, s.exclusionRepo); err != nil {
		return nil, err
	}
	return payload.HotTopics, nil
}

func (s *ReadModelService) filterCommentInsights(ctx context.Context, workspaceID string, items []CommentInsight, query DashboardQuery) ([]CommentInsight, error) {
	window, ok := NormalizeTimeWindow(query.TimeWindow)
	if !ok {
		return nil, ErrInvalidTimeWindow
	}
	if window != TimeWindowAll {
		items = filterCommentInsightsByTimeWindow(items, window, s.timeWindowReference(ctx, workspaceID, query, "comments"))
	}
	payload := Dashboard{CommentInsights: cloneCommentInsights(items)}
	if err := FilterDashboardByDataCenterExclusions(ctx, &payload, workspaceID, s.exclusionRepo); err != nil {
		return nil, err
	}
	return payload.CommentInsights, nil
}

func dashboardReferenceAt(payload Dashboard, query DashboardQuery) time.Time {
	if !query.ReferenceAt.IsZero() {
		return query.ReferenceAt
	}
	if referenceAt := dashboardTimeWindowReference(payload); !referenceAt.IsZero() {
		return referenceAt
	}
	return time.Now()
}

func (s *ReadModelService) timeWindowReference(ctx context.Context, workspaceID string, query DashboardQuery, kind string) time.Time {
	if !query.ReferenceAt.IsZero() {
		return query.ReferenceAt
	}
	if kind == "videos" && s.rankingRepo != nil {
		if meta, found, err := s.rankingRepo.GetHotVideoMeta(ctx, workspaceID); err == nil && found && !meta.MaterializedAt.IsZero() {
			return meta.MaterializedAt
		}
	}
	if kind == "topics" && s.rankingRepo != nil {
		if meta, found, err := s.rankingRepo.GetHotTopicMeta(ctx, workspaceID); err == nil && found && !meta.MaterializedAt.IsZero() {
			return meta.MaterializedAt
		}
	}
	if kind == "comments" && s.commentInsightRepo != nil {
		if meta, found, err := s.commentInsightRepo.GetMeta(ctx, workspaceID); err == nil && found && !meta.MaterializedAt.IsZero() {
			return meta.MaterializedAt
		}
	}
	return time.Now()
}
