package insight

import (
	"context"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type Materializer struct {
	base Dashboard
	now  func() time.Time
}

func NewMaterializer(base Dashboard) *Materializer {
	return NewMaterializerWithClock(base, nil)
}

func NewMaterializerWithClock(base Dashboard, now func() time.Time) *Materializer {
	if now == nil {
		now = time.Now
	}
	return &Materializer{
		base: base,
		now:  now,
	}
}

func (m *Materializer) Build(summary ingest.Summary, workspaceContext workspace.ResolvedContext, source string) Dashboard {
	return m.BuildWithAggregate(summary, workspaceContext, aggregate.Aggregate{}, false, normalize.Snapshot{}, false, source)
}

func (m *Materializer) BuildWithNormalized(summary ingest.Summary, workspaceContext workspace.ResolvedContext, snapshot normalize.Snapshot, hasSnapshot bool, source string) Dashboard {
	return m.BuildWithAggregate(summary, workspaceContext, aggregate.Aggregate{}, false, snapshot, hasSnapshot, source)
}

func (m *Materializer) BuildWithAggregate(summary ingest.Summary, workspaceContext workspace.ResolvedContext, aggregateData aggregate.Aggregate, hasAggregate bool, snapshot normalize.Snapshot, hasSnapshot bool, source string) Dashboard {
	// Materialized read models must be driven by live ingest/normalized/aggregate data.
	// Prototype seed content is allowed as a base template for repo bootstrap, but once
	// we materialize a live workspace snapshot we should not carry seed metrics or filler
	// cards forward into the formal read model.
	payload := Dashboard{}
	payload.GeneratedAt = m.materializedAt()

	payload.Workspace = WorkspaceSummary{
		Name:        workspaceContext.Workspace.Name,
		Owner:       workspaceContext.Workspace.OwnerUserID,
		DeviceCount: workspaceContext.Workspace.DeviceCount,
	}
	payload.WorkspaceBridge = WorkspaceBridge{
		WorkspaceID:   workspaceContext.Workspace.ID,
		HubUserID:     workspaceContext.Workspace.OwnerUserID,
		ContextSource: source,
		BootstrapMode: workspaceContext.Workspace.BootstrapMode,
		Source:        workspaceContext.Workspace.Source,
		State:         workspaceContext.Workspace.State,
		Resolution:    workspaceContext.Resolution,
		LastSyncedAt:  workspaceContext.Workspace.LastSyncedAt,
	}
	payload.IngestSummary = mapIngestSummary(summary)
	if hasAggregate {
		applyAggregateOverlay(&payload, summary, aggregateData)
	} else if hasSnapshot {
		applyNormalizedOverlay(&payload, summary, snapshot)
	} else {
		applyIngestOverlay(&payload, summary)
	}
	payload.ReadModel = ReadModelState{
		Mode:           "materialized",
		MaterializedAt: payload.GeneratedAt,
		Source:         source,
		LatestBatchID:  summary.LatestBatchID,
	}
	payload.ReadModel.ViewStatuses = BuildReadModelViewStatuses(payload)
	return payload
}

func (m *Materializer) materializedAt() time.Time {
	if m != nil && m.now != nil {
		if now := m.now(); !now.IsZero() {
			return now
		}
	}
	return time.Now()
}

type ReadModelRefreshService struct {
	repo                  DashboardRepository
	reportSummaryRepo     ReportSummaryRepository
	dailyReportRepo       DailyReportRepository
	rankingRepo           RankingRepository
	commentInsightRepo    CommentInsightRepository
	recommendationService *RecommendationService
	readCache             *ReadCache
	contextResolver       WorkspaceContextResolver
	ingestService         ingestSummaryProvider
	aggregator            aggregationSnapshotProvider
	normalizer            normalizationSnapshotProvider
	materializer          *Materializer
	afterRefresh          func(workspace.ResolvedContext)
}

// SetAfterRefresh is configured once at startup. The callback must enqueue work
// and return immediately: publication must never wait for an external model.
func (s *ReadModelRefreshService) SetAfterRefresh(notify func(workspace.ResolvedContext)) {
	s.afterRefresh = notify
}

type ReadModelRefreshServiceConfig struct {
	Repository               DashboardRepository
	ReportSummaryRepository  ReportSummaryRepository
	DailyReportRepository    DailyReportRepository
	RankingRepository        RankingRepository
	CommentInsightRepository CommentInsightRepository
	RecommendationService    *RecommendationService
	ReadCache                *ReadCache
	WorkspaceContextResolver WorkspaceContextResolver
	IngestService            ingestSummaryProvider
	Aggregator               aggregationSnapshotProvider
	Normalizer               normalizationSnapshotProvider
	Materializer             *Materializer
}

func NewReadModelRefreshService(config ReadModelRefreshServiceConfig) *ReadModelRefreshService {
	return &ReadModelRefreshService{
		repo:                  config.Repository,
		reportSummaryRepo:     config.ReportSummaryRepository,
		dailyReportRepo:       config.DailyReportRepository,
		rankingRepo:           config.RankingRepository,
		commentInsightRepo:    config.CommentInsightRepository,
		recommendationService: config.RecommendationService,
		readCache:             config.ReadCache,
		contextResolver:       config.WorkspaceContextResolver,
		ingestService:         config.IngestService,
		aggregator:            config.Aggregator,
		normalizer:            config.Normalizer,
		materializer:          config.Materializer,
	}
}

func (s *ReadModelRefreshService) MaterializeDashboard(ctx context.Context, workspaceContext workspace.ResolvedContext, summary ingest.Summary, source string) (Dashboard, error) {
	if source == "" {
		source = "jobs"
	}

	var aggregateData aggregate.Aggregate
	hasAggregate := false
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
	}
	normalizedSnapshot, hasNormalizedSnapshot, err := s.loadNormalizedSnapshot(ctx, workspaceContext.Workspace.ID)
	if err != nil {
		return Dashboard{}, err
	}
	payload := s.materializer.BuildWithAggregate(summary, workspaceContext, aggregateData, hasAggregate, normalizedSnapshot, hasNormalizedSnapshot, source)
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
	if s.afterRefresh != nil {
		s.afterRefresh(workspaceContext)
	}
	return persisted, nil
}

func (s *ReadModelRefreshService) RefreshDashboard(ctx context.Context, query DashboardQuery, source string) (Dashboard, error) {
	if source == "" {
		source = "jobs"
	}

	deviceCount := query.DeviceCount
	workspaceContext, err := s.contextResolver.Resolve(ctx, workspace.ContextQuery{
		HubUserID:     query.HubUserID,
		DeviceCount:   deviceCount,
		ContextSource: source,
	})
	if err != nil {
		return Dashboard{}, err
	}

	summary, err := s.ingestService.GetSummary(ctx, ingest.SummaryQuery{
		HubUserID:   workspaceContext.Workspace.OwnerUserID,
		DeviceCount: workspaceContext.Workspace.DeviceCount,
	})
	if err != nil {
		return Dashboard{}, err
	}

	return s.MaterializeDashboard(ctx, workspaceContext, summary, source)
}

func (s *ReadModelRefreshService) loadNormalizedSnapshot(ctx context.Context, workspaceID string) (normalize.Snapshot, bool, error) {
	if s.normalizer == nil {
		return normalize.Snapshot{}, false, nil
	}
	return s.normalizer.GetSnapshot(ctx, workspaceID)
}

func cloneDashboard(source Dashboard) Dashboard {
	cloned := source
	cloned.HotVideos = append([]HotVideo(nil), source.HotVideos...)
	cloned.HotVideoRankings = cloneHotVideoRankings(source.HotVideoRankings)
	cloned.HotVideoWindowRankings = cloneHotVideoWindowRankings(source.HotVideoWindowRankings)
	cloned.HotTopics = cloneHotTopics(source.HotTopics)
	cloned.AccountRecommendations = append([]AccountRecommendation(nil), source.AccountRecommendations...)
	cloned.VideoRecommendations = append([]VideoRecommendation(nil), source.VideoRecommendations...)
	cloned.WatchlistAlerts = append([]WatchlistAlert(nil), source.WatchlistAlerts...)
	cloned.CommentInsights = append([]CommentInsight(nil), source.CommentInsights...)
	cloned.IngestSummary.RecentEvents = append([]IngestEvent(nil), source.IngestSummary.RecentEvents...)
	cloned.IngestSummary.SourceStatuses = append([]IngestSourceStatus(nil), source.IngestSummary.SourceStatuses...)
	cloned.IngestSummary.RecentRuns = append([]IngestRun(nil), source.IngestSummary.RecentRuns...)
	cloned.IngestSummary.BindingStates = append([]IngestBindingState(nil), source.IngestSummary.BindingStates...)
	cloned.IngestSummary.OperatorReadiness.Runnable = append([]OperatorReadinessSource(nil), source.IngestSummary.OperatorReadiness.Runnable...)
	cloned.IngestSummary.OperatorReadiness.Blocked = append([]OperatorReadinessSource(nil), source.IngestSummary.OperatorReadiness.Blocked...)
	cloned.IngestSummary.OperatorReadiness.Missing = append([]OperatorReadinessSource(nil), source.IngestSummary.OperatorReadiness.Missing...)

	for i := range cloned.HotVideos {
		cloned.HotVideos[i].Tags = append([]string(nil), cloned.HotVideos[i].Tags...)
	}
	for i := range cloned.HotTopics {
		cloned.HotTopics[i].Keywords = append([]string(nil), cloned.HotTopics[i].Keywords...)
		cloned.HotTopics[i].SourceAccounts = append([]string(nil), cloned.HotTopics[i].SourceAccounts...)
	}
	for i := range cloned.AccountRecommendations {
		cloned.AccountRecommendations[i].Sources = append([]string(nil), cloned.AccountRecommendations[i].Sources...)
	}
	for i := range cloned.VideoRecommendations {
		cloned.VideoRecommendations[i].Tags = append([]string(nil), cloned.VideoRecommendations[i].Tags...)
	}
	for i := range cloned.CommentInsights {
		cloned.CommentInsights[i].RelatedKeywords = append([]string(nil), cloned.CommentInsights[i].RelatedKeywords...)
		cloned.CommentInsights[i].SourceAccounts = append([]string(nil), cloned.CommentInsights[i].SourceAccounts...)
	}
	for i := range cloned.IngestSummary.RecentEvents {
		cloned.IngestSummary.RecentEvents[i].Tags = append([]string(nil), cloned.IngestSummary.RecentEvents[i].Tags...)
		cloned.IngestSummary.RecentEvents[i].Keywords = append([]string(nil), cloned.IngestSummary.RecentEvents[i].Keywords...)
	}
	for i := range cloned.IngestSummary.BindingStates {
		cloned.IngestSummary.BindingStates[i].Diagnostic = cloneBindingDiagnostic(cloned.IngestSummary.BindingStates[i].Diagnostic)
	}
	for i := range cloned.IngestSummary.OperatorReadiness.Runnable {
		cloned.IngestSummary.OperatorReadiness.Runnable[i].Diagnostic = cloneBindingDiagnostic(cloned.IngestSummary.OperatorReadiness.Runnable[i].Diagnostic)
	}
	for i := range cloned.IngestSummary.OperatorReadiness.Blocked {
		cloned.IngestSummary.OperatorReadiness.Blocked[i].Diagnostic = cloneBindingDiagnostic(cloned.IngestSummary.OperatorReadiness.Blocked[i].Diagnostic)
	}
	for i := range cloned.IngestSummary.OperatorReadiness.Missing {
		cloned.IngestSummary.OperatorReadiness.Missing[i].Diagnostic = cloneBindingDiagnostic(cloned.IngestSummary.OperatorReadiness.Missing[i].Diagnostic)
	}

	return cloned
}

func cloneBindingDiagnostic(source *BindingDiagnostic) *BindingDiagnostic {
	if source == nil {
		return nil
	}
	cloned := *source
	if source.ConnectedClientCount != nil {
		value := *source.ConnectedClientCount
		cloned.ConnectedClientCount = &value
	}
	if source.ReadyClientCount != nil {
		value := *source.ReadyClientCount
		cloned.ReadyClientCount = &value
	}
	if source.FeedReadyClientCount != nil {
		value := *source.FeedReadyClientCount
		cloned.FeedReadyClientCount = &value
	}
	if source.WXChannelInjectionChecked != nil {
		value := *source.WXChannelInjectionChecked
		cloned.WXChannelInjectionChecked = &value
	}
	if source.WXChannelInjectionStarted != nil {
		value := *source.WXChannelInjectionStarted
		cloned.WXChannelInjectionStarted = &value
	}
	if source.WXChannelCertificateInstalled != nil {
		value := *source.WXChannelCertificateInstalled
		cloned.WXChannelCertificateInstalled = &value
	}
	return &cloned
}
