package insight

import (
	"context"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type DashboardService struct {
	repo                  DashboardRepository
	reportSummaryRepo     ReportSummaryRepository
	dailyReportRepo       DailyReportRepository
	rankingRepo           RankingRepository
	commentInsightRepo    CommentInsightRepository
	exclusionRepo         DataCenterExclusionRepository
	recommendationService *RecommendationService
	readCache             *ReadCache
	contextResolver       WorkspaceContextResolver
	ingestService         ingestSummaryProvider
	aggregator            aggregationSnapshotProvider
	normalizer            normalizationSnapshotProvider
	materializer          *Materializer
	now                   func() time.Time
}

type dashboardOverlayOptions struct {
	autoMaterialize              bool
	includeReadModelViewStatuses bool
	refreshRelativeLabels        bool
	readOnlySummary              bool
	skipDashboardCache           bool
	boundedCollectionOverlays    bool
	launchReadinessOnly          bool
}

var fullDashboardOverlayOptions = dashboardOverlayOptions{
	autoMaterialize:              true,
	includeReadModelViewStatuses: true,
	refreshRelativeLabels:        true,
}

var launchReadinessDashboardOptions = dashboardOverlayOptions{
	autoMaterialize:              false,
	includeReadModelViewStatuses: false,
	refreshRelativeLabels:        false,
	readOnlySummary:              true,
	launchReadinessOnly:          true,
}

var readOnlyDashboardOptions = dashboardOverlayOptions{
	autoMaterialize:              false,
	includeReadModelViewStatuses: true,
	refreshRelativeLabels:        true,
	readOnlySummary:              true,
	skipDashboardCache:           true,
	boundedCollectionOverlays:    true,
}

type DashboardQuery struct {
	HubUserID     string
	DeviceCount   int
	ContextSource string
	TimeWindow    string
	ReferenceAt   time.Time
}

type DashboardServiceConfig struct {
	WorkspaceContextResolver      WorkspaceContextResolver
	IngestService                 ingestSummaryProvider
	Aggregator                    aggregationSnapshotProvider
	Normalizer                    normalizationSnapshotProvider
	ReportSummaryRepository       ReportSummaryRepository
	DailyReportRepository         DailyReportRepository
	RankingRepository             RankingRepository
	CommentInsightRepository      CommentInsightRepository
	DataCenterExclusionRepository DataCenterExclusionRepository
	RecommendationService         *RecommendationService
	ReadCache                     *ReadCache
	Materializer                  *Materializer
	Now                           func() time.Time
}

type WorkspaceContextResolver interface {
	Resolve(ctx context.Context, query workspace.ContextQuery) (workspace.ResolvedContext, error)
}

type ingestSummaryProvider interface {
	GetSummary(ctx context.Context, query ingest.SummaryQuery) (ingest.Summary, error)
}

type readOnlyIngestSummaryProvider interface {
	GetSummaryReadOnly(ctx context.Context, query ingest.SummaryQuery) (ingest.Summary, error)
}

type dashboardReadOnlyRepository interface {
	GetDashboardReadOnly(ctx context.Context, workspaceID string) (Dashboard, error)
}

type dashboardLaunchReadinessRepository interface {
	GetDashboardLaunchReadiness(ctx context.Context, workspaceID string) (Dashboard, error)
}

type dashboardPageRankingRepository interface {
	GetDashboardPageRankings(ctx context.Context, workspaceID string) ([]HotVideo, HotVideoRankings, bool, []HotTopic, bool, error)
}

type dashboardRankingProjection struct {
	videos      []HotVideo
	rankings    HotVideoRankings
	videosFound bool
	topics      []HotTopic
	topicsFound bool
}

type normalizationSnapshotProvider interface {
	RefreshSnapshot(ctx context.Context, workspaceContext workspace.ResolvedContext) error
	GetSnapshot(ctx context.Context, workspaceID string) (normalize.Snapshot, bool, error)
}

type aggregationSnapshotProvider interface {
	RefreshAggregate(ctx context.Context, workspaceContext workspace.ResolvedContext) (aggregate.Aggregate, error)
	GetAggregate(ctx context.Context, workspaceID string) (aggregate.Aggregate, bool, error)
}

func NewDashboardService(repo DashboardRepository, config DashboardServiceConfig) *DashboardService {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &DashboardService{
		repo:                  repo,
		reportSummaryRepo:     config.ReportSummaryRepository,
		dailyReportRepo:       config.DailyReportRepository,
		rankingRepo:           config.RankingRepository,
		commentInsightRepo:    config.CommentInsightRepository,
		exclusionRepo:         config.DataCenterExclusionRepository,
		recommendationService: config.RecommendationService,
		readCache:             config.ReadCache,
		contextResolver:       config.WorkspaceContextResolver,
		ingestService:         config.IngestService,
		aggregator:            config.Aggregator,
		normalizer:            config.Normalizer,
		materializer:          config.Materializer,
		now:                   now,
	}
}

func (s *DashboardService) GetDashboard(ctx context.Context, query DashboardQuery) (Dashboard, error) {
	return s.getDashboard(ctx, query, fullDashboardOverlayOptions)
}

// GetReadOnlyDashboard serves page reads without turning a stale snapshot into
// a synchronous materialization job. Refresh jobs own materialization; page
// requests should return the latest persisted/read-cache view quickly.
func (s *DashboardService) GetReadOnlyDashboard(ctx context.Context, query DashboardQuery) (Dashboard, error) {
	return s.getDashboard(ctx, query, readOnlyDashboardOptions)
}

func (s *DashboardService) GetLaunchReadinessDashboard(ctx context.Context, query DashboardQuery) (Dashboard, error) {
	return s.getDashboard(ctx, query, launchReadinessDashboardOptions)
}

func (s *DashboardService) getDashboard(ctx context.Context, query DashboardQuery, options dashboardOverlayOptions) (Dashboard, error) {
	normalizedWindow, ok := NormalizeTimeWindow(query.TimeWindow)
	if !ok {
		return Dashboard{}, ErrInvalidTimeWindow
	}
	query.TimeWindow = normalizedWindow
	if s.contextResolver == nil {
		payload, err := s.repo.GetDashboard(ctx, "")
		if err != nil {
			return Dashboard{}, err
		}
		if err := FilterDashboardByTimeWindow(&payload, query.TimeWindow, query.ReferenceAt); err != nil {
			return Dashboard{}, err
		}
		return cloneDashboard(payload), nil
	}

	workspaceContext, err := s.contextResolver.Resolve(ctx, workspace.ContextQuery{
		HubUserID:     query.HubUserID,
		DeviceCount:   query.DeviceCount,
		ContextSource: query.ContextSource,
	})
	if err != nil {
		return Dashboard{}, err
	}

	workspaceID := workspaceContext.Workspace.ID
	if s.readCache != nil && !options.skipDashboardCache {
		if payload, found, err := s.readCache.LoadDashboard(ctx, workspaceID); err == nil && found {
			overlaid, err := s.overlayDashboard(ctx, payload, workspaceContext, options)
			if err != nil {
				return Dashboard{}, err
			}
			if err := FilterDashboardByTimeWindow(&overlaid, query.TimeWindow, query.ReferenceAt); err != nil {
				return Dashboard{}, err
			}
			return overlaid, nil
		}
	}

	var payload Dashboard
	if options.launchReadinessOnly {
		if launchRepo, ok := s.repo.(dashboardLaunchReadinessRepository); ok {
			payload, err = launchRepo.GetDashboardLaunchReadiness(ctx, workspaceID)
		} else {
			payload, err = s.repo.GetDashboard(ctx, workspaceID)
		}
	} else if options.skipDashboardCache {
		if readOnlyRepo, ok := s.repo.(dashboardReadOnlyRepository); ok {
			payload, err = readOnlyRepo.GetDashboardReadOnly(ctx, workspaceID)
		} else {
			payload, err = s.repo.GetDashboard(ctx, workspaceID)
		}
	} else {
		payload, err = s.repo.GetDashboard(ctx, workspaceID)
	}
	if err != nil {
		return Dashboard{}, err
	}
	if s.readCache != nil && !options.skipDashboardCache {
		s.readCache.RecordScopeFallback(readCacheScopeDashboard, time.Now())
		_ = s.readCache.SaveDashboard(ctx, workspaceID, payload)
	}

	overlaid, err := s.overlayDashboard(ctx, payload, workspaceContext, options)
	if err != nil {
		return Dashboard{}, err
	}
	if err := FilterDashboardByTimeWindow(&overlaid, query.TimeWindow, query.ReferenceAt); err != nil {
		return Dashboard{}, err
	}
	return overlaid, nil
}

func (s *DashboardService) overlayDashboard(ctx context.Context, payload Dashboard, workspaceContext workspace.ResolvedContext, options dashboardOverlayOptions) (Dashboard, error) {
	workspaceID := workspaceContext.Workspace.ID
	applyResolvedWorkspaceContext(&payload, workspaceContext)
	if s.ingestService != nil {
		summaryQuery := ingest.SummaryQuery{
			HubUserID:   workspaceContext.Workspace.OwnerUserID,
			DeviceCount: workspaceContext.Workspace.DeviceCount,
		}
		var summary ingest.Summary
		var err error
		if options.readOnlySummary {
			if readOnlyProvider, ok := s.ingestService.(readOnlyIngestSummaryProvider); ok {
				summary, err = readOnlyProvider.GetSummaryReadOnly(ctx, summaryQuery)
			} else {
				summary, err = s.ingestService.GetSummary(ctx, summaryQuery)
			}
		} else {
			summary, err = s.ingestService.GetSummary(ctx, summaryQuery)
		}
		if err != nil {
			return Dashboard{}, err
		}
		if options.autoMaterialize && s.shouldAutoMaterialize(payload, summary) {
			materialized, err := s.materializeCurrentWorkspace(ctx, workspaceContext, summary)
			if err != nil {
				return Dashboard{}, err
			}
			payload = materialized
			applyResolvedWorkspaceContext(&payload, workspaceContext)
		}
		payload.IngestSummary = mapIngestSummary(summary)
		if payload.ReadModel.Mode != "materialized" || payload.ReadModel.LatestBatchID != summary.LatestBatchID {
			applyIngestOverlay(&payload, summary)
		}
	}
	if options.launchReadinessOnly {
		return cloneDashboard(payload), nil
	}
	if s.rankingRepo != nil {
		projection, err := s.loadDashboardRankingProjection(ctx, workspaceID, options.boundedCollectionOverlays)
		if err != nil {
			return Dashboard{}, err
		}
		if projection.videosFound {
			payload.HotVideos = cloneHotVideos(projection.videos)
		} else {
			payload.HotVideos = []HotVideo{}
		}
		if projection.videosFound {
			payload.HotVideoRankings = mergeHotVideoRankings(projection.rankings, payload.HotVideoRankings)
		}
		if projection.topicsFound {
			payload.HotTopics = cloneHotTopics(projection.topics)
		} else {
			payload.HotTopics = []HotTopic{}
		}
	}
	if len(payload.HotVideoWindowRankings) == 0 && len(payload.VideoRecommendations) > 0 {
		payload.HotVideoWindowRankings = BuildHotVideoWindowRankings(
			hotVideosFromRecommendations(payload.VideoRecommendations),
			payload.GeneratedAt,
		)
	}
	if options.refreshRelativeLabels {
		refreshRelativeTimeLabels(&payload, s.now())
	}
	if options.includeReadModelViewStatuses {
		metaByKey, err := collectReadModelArtifactMeta(ctx, workspaceID, s.rankingRepo, s.commentInsightRepo, s.reportSummaryRepo)
		if err != nil {
			return Dashboard{}, err
		}
		payload.ReadModel.ViewStatuses = BuildReadModelViewStatusesWithMeta(payload, metaByKey)
	}
	if err := FilterDashboardByDataCenterExclusions(ctx, &payload, workspaceID, s.exclusionRepo); err != nil {
		return Dashboard{}, err
	}
	return cloneDashboard(payload), nil
}

func (s *DashboardService) loadDashboardRankingProjection(ctx context.Context, workspaceID string, bounded bool) (dashboardRankingProjection, error) {
	if bounded {
		if pageRepo, ok := s.rankingRepo.(dashboardPageRankingRepository); ok {
			videos, rankings, videosFound, topics, topicsFound, err := pageRepo.GetDashboardPageRankings(ctx, workspaceID)
			return dashboardRankingProjection{
				videos:      videos,
				rankings:    rankings,
				videosFound: videosFound,
				topics:      topics,
				topicsFound: topicsFound,
			}, err
		}
	}

	videos, videosFound, err := s.rankingRepo.GetHotVideos(ctx, workspaceID)
	if err != nil {
		return dashboardRankingProjection{}, err
	}
	rankings, rankingsFound, err := s.rankingRepo.GetHotVideoRankings(ctx, workspaceID)
	if err != nil {
		return dashboardRankingProjection{}, err
	}
	topics, topicsFound, err := s.rankingRepo.GetHotTopics(ctx, workspaceID)
	if err != nil {
		return dashboardRankingProjection{}, err
	}
	return dashboardRankingProjection{
		videos:      videos,
		rankings:    rankings,
		videosFound: videosFound || rankingsFound,
		topics:      topics,
		topicsFound: topicsFound,
	}, nil
}

func applyResolvedWorkspaceContext(payload *Dashboard, workspaceContext workspace.ResolvedContext) {
	if payload == nil {
		return
	}
	payload.Workspace = WorkspaceSummary{
		Name:        workspaceContext.Workspace.Name,
		Owner:       workspaceContext.Workspace.OwnerUserID,
		DeviceCount: workspaceContext.Workspace.DeviceCount,
	}
	payload.WorkspaceBridge = WorkspaceBridge{
		WorkspaceID:   workspaceContext.Workspace.ID,
		HubUserID:     workspaceContext.Workspace.OwnerUserID,
		ContextSource: workspaceContext.ContextSource,
		BootstrapMode: workspaceContext.Workspace.BootstrapMode,
		Source:        workspaceContext.Workspace.Source,
		State:         workspaceContext.Workspace.State,
		Resolution:    workspaceContext.Resolution,
		LastSyncedAt:  workspaceContext.Workspace.LastSyncedAt,
	}
}

func refreshRelativeTimeLabels(payload *Dashboard, referenceAt time.Time) {
	if payload == nil {
		return
	}
	if referenceAt.IsZero() {
		referenceAt = time.Now()
	}
	if !payload.IngestSummary.LatestEventAt.IsZero() {
		payload.Overview.ReportFreshness = humanizeFreshness(payload.IngestSummary.LatestEventAt, referenceAt)
	}
	for i := range payload.HotVideos {
		payload.HotVideos[i].PublishedAgo = publishedAgo(payload.HotVideos[i].PublishedAt, referenceAt)
	}
	for kind, items := range payload.HotVideoRankings {
		for i := range items {
			items[i].PublishedAgo = publishedAgo(items[i].PublishedAt, referenceAt)
		}
		payload.HotVideoRankings[kind] = items
	}
	for i := range payload.WatchlistAlerts {
		if !payload.WatchlistAlerts[i].OccurredAt.IsZero() {
			payload.WatchlistAlerts[i].HappenedAt = humanizeFreshness(payload.WatchlistAlerts[i].OccurredAt, referenceAt)
		}
	}
	reportGeneratedAt := payload.ReportSummary.GeneratedAt
	if reportGeneratedAt.IsZero() {
		reportGeneratedAt = payload.GeneratedAt
	}
	if !reportGeneratedAt.IsZero() {
		payload.ReportSummary.AutoGeneratedAt = humanizeFreshness(reportGeneratedAt, referenceAt)
	}
}

func (s *DashboardService) shouldAutoMaterialize(payload Dashboard, summary ingest.Summary) bool {
	if s.materializer == nil || s.repo == nil || summary.TotalEvents == 0 || summary.LatestBatchID == "" {
		return false
	}
	if payload.ReadModel.Mode != "materialized" || payload.ReadModel.LatestBatchID != summary.LatestBatchID {
		return true
	}
	return hotVideoCommentSyncIdentifiersNeedRepair(payload, summary)
}

func hotVideoCommentSyncIdentifiersNeedRepair(payload Dashboard, summary ingest.Summary) bool {
	if !summaryHasFeedListHotVideoSignals(summary) {
		return false
	}
	for _, item := range payload.HotVideos {
		if !isWXFeedListVideoSource(item.Source) {
			continue
		}
		if strings.TrimSpace(item.ObjectID) == "" || strings.TrimSpace(item.ObjectNonceID) == "" {
			return true
		}
	}
	return false
}

func summaryHasFeedListHotVideoSignals(summary ingest.Summary) bool {
	for _, event := range summary.RecentEvents {
		if !isWXFeedListVideoSource(event.Source) {
			continue
		}
		if strings.TrimSpace(event.EventType) != "video_snapshot" {
			continue
		}
		return true
	}
	return false
}

func isWXFeedListVideoSource(source string) bool {
	switch strings.TrimSpace(source) {
	case "wx_feed_list", "wx_channel_feed_list", "wx_video_search", "wx_channel_video_search":
		return true
	default:
		return false
	}
}

func (s *DashboardService) materializeCurrentWorkspace(ctx context.Context, workspaceContext workspace.ResolvedContext, summary ingest.Summary) (Dashboard, error) {
	var aggregateData aggregate.Aggregate
	hasAggregate := false
	var normalizedSnapshot normalize.Snapshot
	hasNormalizedSnapshot := false
	if s.aggregator != nil {
		data, err := s.aggregator.RefreshAggregate(ctx, workspaceContext)
		if err != nil {
			return Dashboard{}, err
		}
		aggregateData = data
		hasAggregate = true
	}
	if s.normalizer != nil {
		if err := s.normalizer.RefreshSnapshot(ctx, workspaceContext); err != nil {
			return Dashboard{}, err
		}
		snapshot, found, err := s.normalizer.GetSnapshot(ctx, workspaceContext.Workspace.ID)
		if err != nil {
			return Dashboard{}, err
		}
		if found {
			normalizedSnapshot = snapshot
			hasNormalizedSnapshot = true
		}
	}
	payload := s.materializer.BuildWithAggregate(summary, workspaceContext, aggregateData, hasAggregate, normalizedSnapshot, hasNormalizedSnapshot, "auto_refresh")
	if s.recommendationService != nil {
		recommendations, err := s.recommendationService.Refresh(ctx, workspaceContext)
		if err != nil {
			return Dashboard{}, err
		}
		ApplyRecommendationSnapshot(&payload, recommendations)
	}
	if err := ValidateDashboardSnapshot(payload); err != nil {
		return Dashboard{}, err
	}
	persisted, err := persistDashboardSnapshot(
		ctx,
		workspaceContext.Workspace.ID,
		payload,
		s.repo,
		s.rankingRepo,
		s.commentInsightRepo,
		s.reportSummaryRepo,
		s.dailyReportRepo,
		s.readCache,
	)
	if err != nil {
		return Dashboard{}, err
	}
	return persisted, nil
}

func (s *DashboardService) ReplaceDashboard(ctx context.Context, snapshot Dashboard) error {
	workspaceID := snapshot.WorkspaceBridge.WorkspaceID
	if workspaceID == "" {
		workspaceID = deriveWorkspaceIDFromSnapshot(snapshot)
	}
	if err := ValidateDashboardSnapshot(snapshot); err != nil {
		return err
	}
	_, err := persistDashboardSnapshot(
		ctx,
		workspaceID,
		snapshot,
		s.repo,
		s.rankingRepo,
		s.commentInsightRepo,
		s.reportSummaryRepo,
		s.dailyReportRepo,
		s.readCache,
	)
	return err
}

func (s *DashboardService) ResetDashboard(ctx context.Context) error {
	return s.ResetDashboardForQuery(ctx, DashboardQuery{})
}

func (s *DashboardService) ResetDashboardForQuery(ctx context.Context, query DashboardQuery) error {
	workspaceID := ""
	if s.contextResolver != nil {
		resolved, err := s.contextResolver.Resolve(ctx, workspace.ContextQuery{
			HubUserID:     query.HubUserID,
			DeviceCount:   query.DeviceCount,
			ContextSource: query.ContextSource,
		})
		if err != nil {
			return err
		}
		workspaceID = resolved.Workspace.ID
	}
	if err := s.repo.ResetDashboard(ctx, workspaceID); err != nil {
		return err
	}
	if s.rankingRepo != nil {
		if err := s.rankingRepo.ResetHotVideos(ctx, workspaceID); err != nil {
			return err
		}
		if err := s.rankingRepo.ResetHotVideoRankings(ctx, workspaceID); err != nil {
			return err
		}
		if err := s.rankingRepo.ResetHotTopics(ctx, workspaceID); err != nil {
			return err
		}
	}
	if s.commentInsightRepo != nil {
		if err := s.commentInsightRepo.ResetCommentInsights(ctx, workspaceID); err != nil {
			return err
		}
	}
	if s.reportSummaryRepo != nil {
		if err := s.reportSummaryRepo.ResetReportSummary(ctx, workspaceID); err != nil {
			return err
		}
	}
	if s.recommendationService != nil {
		if err := s.recommendationService.Reset(ctx, workspaceID); err != nil {
			return err
		}
	}
	if err := resetReadModelArtifactMeta(ctx, workspaceID, s.rankingRepo, s.commentInsightRepo, s.reportSummaryRepo); err != nil {
		return err
	}
	if s.readCache != nil {
		_ = s.readCache.InvalidateWorkspace(ctx, workspaceID)
	}
	return nil
}

func mapIngestSummary(summary ingest.Summary) IngestSummary {
	events := make([]IngestEvent, 0, len(summary.RecentEvents))
	for _, event := range summary.RecentEvents {
		events = append(events, IngestEvent{
			ID:                    event.ID,
			BatchID:               event.BatchID,
			MachineID:             event.MachineID,
			EventType:             event.EventType,
			Source:                event.Source,
			CollectedAt:           event.CollectedAt,
			VideoCount:            event.VideoCount,
			CommentCount:          event.CommentCount,
			TopicCount:            event.TopicCount,
			Title:                 event.Title,
			ChannelName:           event.ChannelName,
			Keyword:               event.Keyword,
			Cluster:               event.Cluster,
			Signal:                event.Signal,
			PromptAngle:           event.PromptAngle,
			RepresentativeComment: event.RepresentativeComment,
			TargetAudience:        event.TargetAudience,
			Tags:                  append([]string(nil), event.Tags...),
			Keywords:              append([]string(nil), event.Keywords...),
		})
	}

	return IngestSummary{
		State:              summary.State,
		FreshnessState:     summary.FreshnessState,
		TotalEvents:        summary.TotalEvents,
		SourceCount:        summary.SourceCount,
		LatestEventAt:      summary.LatestEventAt,
		LastSuccessAt:      summary.LastSuccessAt,
		LastFailureAt:      summary.LastFailureAt,
		LastFailureMessage: summary.LastFailureMessage,
		LatestBatchID:      summary.LatestBatchID,
		LastEventType:      summary.LastEventType,
		AggregatedVideos:   summary.AggregatedVideos,
		AggregatedComments: summary.AggregatedComments,
		AggregatedTopics:   summary.AggregatedTopics,
		RecentEvents:       events,
		SourceStatuses:     mapIngestSourceStatuses(summary.SourceStatuses),
		RecentRuns:         mapIngestRuns(summary.RecentRuns),
		BindingStates:      mapIngestBindingStates(summary.BindingStates),
		OperatorReadiness:  mapOperatorReadiness(summary.OperatorReadiness),
	}
}

func mapIngestSourceStatuses(items []ingest.SourceStatus) []IngestSourceStatus {
	results := make([]IngestSourceStatus, 0, len(items))
	for _, item := range items {
		results = append(results, IngestSourceStatus{
			Source:         item.Source,
			State:          item.State,
			FreshnessState: item.FreshnessState,
			LastRunAt:      item.LastRunAt,
			LastSuccessAt:  item.LastSuccessAt,
			LastFailureAt:  item.LastFailureAt,
			LastError:      item.LastError,
			LatestBatchID:  item.LatestBatchID,
			LastEventType:  item.LastEventType,
			EventCount:     item.EventCount,
			Target:         mapIngestBindingTarget(item.Target),
		})
	}
	return results
}

func mapIngestRuns(items []ingest.Run) []IngestRun {
	results := make([]IngestRun, 0, len(items))
	for _, item := range items {
		results = append(results, IngestRun{
			ID:            item.ID,
			Source:        item.Source,
			Status:        item.Status,
			StartedAt:     item.StartedAt,
			FinishedAt:    item.FinishedAt,
			BatchID:       item.BatchID,
			EventCount:    item.EventCount,
			LatestEventAt: item.LatestEventAt,
			LastEventType: item.LastEventType,
			Message:       item.Message,
		})
	}
	return results
}

func mapIngestBindingStates(items []ingest.SourceBindingState) []IngestBindingState {
	results := make([]IngestBindingState, 0, len(items))
	for _, item := range items {
		results = append(results, IngestBindingState{
			Source:            item.Source,
			Origin:            item.Origin,
			TargetKey:         item.TargetKey,
			Path:              item.Path,
			BaseURL:           item.BaseURL,
			Username:          item.Username,
			DeviceCount:       item.DeviceCount,
			UpdatedAt:         item.UpdatedAt,
			LastRunStatus:     item.LastRunStatus,
			LastRunAt:         item.LastRunAt,
			LastSuccessAt:     item.LastSuccessAt,
			LastFailureAt:     item.LastFailureAt,
			LastError:         item.LastError,
			LatestBatchID:     item.LatestBatchID,
			LastAutoRunStatus: item.LastAutoRunStatus,
			LastAutoRunAt:     item.LastAutoRunAt,
			LastAutoSuccessAt: item.LastAutoSuccessAt,
			LastAutoBatchID:   item.LastAutoBatchID,
			Runnable:          item.Runnable,
			CanRetry:          item.CanRetry,
			RetryReason:       item.RetryReason,
			Target:            mapIngestBindingTarget(item.Target),
			Diagnostic:        mapBindingDiagnostic(item.Diagnostic),
		})
	}
	return results
}

func mapIngestBindingTarget(item *ingest.SourceBindingTarget) *IngestBindingTarget {
	if item == nil {
		return nil
	}
	return &IngestBindingTarget{
		Title:           item.Title,
		ChannelName:     item.ChannelName,
		ChannelUsername: item.ChannelUsername,
		Keyword:         item.Keyword,
		Comment:         item.Comment,
		CommentCount:    item.CommentCount,
		ObjectID:        item.ObjectID,
		ObjectNonceID:   item.ObjectNonceID,
		PublishedAt:     item.PublishedAt,
	}
}

func mapOperatorReadiness(source ingest.OperatorReadiness) OperatorReadiness {
	return OperatorReadiness{
		State:           source.State,
		ExpectedStatus:  source.ExpectedStatus,
		RunnableSources: source.RunnableSources,
		BlockedSources:  source.BlockedSources,
		MissingSources:  source.MissingSources,
		Headline:        source.Headline,
		Summary:         source.Summary,
		NextAction:      source.NextAction,
		Runnable:        mapOperatorReadinessSources(source.Runnable),
		Blocked:         mapOperatorReadinessSources(source.Blocked),
		Missing:         mapOperatorReadinessSources(source.Missing),
	}
}

func mapOperatorReadinessSources(items []ingest.OperatorReadinessSource) []OperatorReadinessSource {
	results := make([]OperatorReadinessSource, 0, len(items))
	for _, item := range items {
		results = append(results, OperatorReadinessSource{
			Source:            item.Source,
			TargetKey:         item.TargetKey,
			Label:             item.Label,
			State:             item.State,
			Summary:           item.Summary,
			Action:            item.Action,
			Path:              item.Path,
			BaseURL:           item.BaseURL,
			Username:          item.Username,
			DeviceCount:       item.DeviceCount,
			UpdatedAt:         item.UpdatedAt,
			LastRunStatus:     item.LastRunStatus,
			LastSuccessAt:     item.LastSuccessAt,
			LastFailureAt:     item.LastFailureAt,
			LatestBatchID:     item.LatestBatchID,
			LastAutoRunStatus: item.LastAutoRunStatus,
			LastAutoRunAt:     item.LastAutoRunAt,
			LastAutoSuccessAt: item.LastAutoSuccessAt,
			LastAutoBatchID:   item.LastAutoBatchID,
			Runnable:          item.Runnable,
			CanRetry:          item.CanRetry,
			RetryReason:       item.RetryReason,
			Diagnostic:        mapBindingDiagnostic(item.Diagnostic),
		})
	}
	return results
}

func mapBindingDiagnostic(item *ingest.SourceBindingDiagnostic) *BindingDiagnostic {
	if item == nil {
		return nil
	}
	result := &BindingDiagnostic{
		Code:                       item.Code,
		ResolvedBaseURL:            item.ResolvedBaseURL,
		StatusURL:                  item.StatusURL,
		WSHealthURL:                item.WSHealthURL,
		WXChannelStatusError:       item.WXChannelStatusError,
		HealthStatus:               item.HealthStatus,
		CurrentBrowserProxyAddress: item.CurrentBrowserProxyAddress,
		ExpectedProxyAddress:       item.ExpectedProxyAddress,
		WXChannelProxyMode:         item.WXChannelProxyMode,
		WXChannelProxyAddress:      item.WXChannelProxyAddress,
		WXChannelInjectionError:    item.WXChannelInjectionError,
		WXChannelCertificateError:  item.WXChannelCertificateError,
		CurrentPagePath:            item.CurrentPagePath,
		CurrentPageHref:            item.CurrentPageHref,
	}
	if item.ConnectedClientCount != nil {
		value := *item.ConnectedClientCount
		result.ConnectedClientCount = &value
	}
	if item.ReadyClientCount != nil {
		value := *item.ReadyClientCount
		result.ReadyClientCount = &value
	}
	if item.FeedReadyClientCount != nil {
		value := *item.FeedReadyClientCount
		result.FeedReadyClientCount = &value
	}
	if item.WXChannelInjectionChecked != nil {
		value := *item.WXChannelInjectionChecked
		result.WXChannelInjectionChecked = &value
	}
	if item.WXChannelInjectionStarted != nil {
		value := *item.WXChannelInjectionStarted
		result.WXChannelInjectionStarted = &value
	}
	if item.WXChannelCertificateInstalled != nil {
		value := *item.WXChannelCertificateInstalled
		result.WXChannelCertificateInstalled = &value
	}
	return result
}

func deriveWorkspaceIDFromSnapshot(snapshot Dashboard) string {
	if snapshot.Workspace.Owner == "" {
		return ""
	}
	return "ws-default-" + workspaceIDFragment(snapshot.Workspace.Owner)
}

func workspaceIDFragment(owner string) string {
	var result []rune
	for _, char := range owner {
		switch {
		case char >= 'a' && char <= 'z':
			result = append(result, char)
		case char >= 'A' && char <= 'Z':
			result = append(result, char+('a'-'A'))
		case char >= '0' && char <= '9':
			result = append(result, char)
		default:
			result = append(result, '-')
		}
	}
	trimmed := strings.Trim(string(result), "-")
	if trimmed == "" {
		return "workspace"
	}
	return trimmed
}
