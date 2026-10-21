package insight

import (
	"context"
	"errors"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

const (
	// Keep the materialized ranking set large enough for analysis views to
	// request a deeper slice without expanding the dashboard payload.
	hotVideoRankingLimit = 100
	hotTopicRankingLimit = 100
)

type RecommendationDataProvider interface {
	ListEvents(ctx context.Context, query ingest.SummaryQuery) ([]ingest.Event, error)
	ListVideoMetricObservations(ctx context.Context, query ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error)
	ListBindings(ctx context.Context, query ingest.SummaryQuery) ([]ingest.SourceBinding, error)
}

type RecommendationServiceConfig struct {
	DataProvider             RecommendationDataProvider
	Repository               RecommendationRepository
	CommentInsightRepository CommentInsightRepository
	WorkspaceContextResolver WorkspaceContextResolver
	Now                      func() time.Time
}

type RecommendationService struct {
	dataProvider RecommendationDataProvider
	repository   RecommendationRepository
	commentRepo  CommentInsightRepository
	resolver     WorkspaceContextResolver
	now          func() time.Time
}

func NewRecommendationService(config RecommendationServiceConfig) *RecommendationService {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &RecommendationService{
		dataProvider: config.DataProvider,
		repository:   config.Repository,
		commentRepo:  config.CommentInsightRepository,
		resolver:     config.WorkspaceContextResolver,
		now:          now,
	}
}

func (s *RecommendationService) Refresh(ctx context.Context, workspaceContext workspace.ResolvedContext) (RecommendationSnapshot, error) {
	if s == nil || s.dataProvider == nil {
		return RecommendationSnapshot{}, errors.New("recommendation data provider is not configured")
	}
	query := ingest.SummaryQuery{
		HubUserID:   workspaceContext.Workspace.OwnerUserID,
		DeviceCount: workspaceContext.Workspace.DeviceCount,
	}
	events, err := s.dataProvider.ListEvents(ctx, query)
	if err != nil {
		return RecommendationSnapshot{}, err
	}
	observations, err := s.dataProvider.ListVideoMetricObservations(ctx, query)
	if err != nil {
		return RecommendationSnapshot{}, err
	}
	bindings, err := s.dataProvider.ListBindings(ctx, query)
	if err != nil {
		return RecommendationSnapshot{}, err
	}
	snapshot := BuildRecommendationSnapshot(
		workspaceContext.Workspace.ID,
		workspaceContext.Workspace.OwnerUserID,
		events,
		observations,
		bindings,
		s.now(),
	)
	if err := s.attachTopicCommentEvidence(ctx, workspaceContext.Workspace.ID, &snapshot); err != nil {
		return RecommendationSnapshot{}, err
	}

	if s.repository != nil {
		if err := s.repository.ReplaceRecommendations(ctx, workspaceContext.Workspace.ID, snapshot); err != nil {
			return RecommendationSnapshot{}, err
		}
	}
	return cloneRecommendationSnapshot(snapshot), nil
}

func (s *RecommendationService) attachTopicCommentEvidence(ctx context.Context, workspaceID string, snapshot *RecommendationSnapshot) error {
	if snapshot == nil {
		return nil
	}
	var comments []CommentInsight
	if s.commentRepo != nil {
		items, _, err := s.commentRepo.GetCommentInsights(ctx, workspaceID)
		if err != nil {
			return err
		}
		comments = items
	}
	for index := range snapshot.TopicRecommendations {
		item := &snapshot.TopicRecommendations[index]
		item.ReferenceVideos = topicReferenceVideos(item.ReferenceVideos)
		item.EvidenceStatus = topicEvidenceStatus(item.ReferenceVideos)
		item.CommentEvidence = matchTopicCommentEvidence(*item, comments)
	}
	return nil
}

func (s *RecommendationService) Get(ctx context.Context, query DashboardQuery) (RecommendationSnapshot, error) {
	resolved, err := s.resolveWorkspace(ctx, query)
	if err != nil {
		return RecommendationSnapshot{}, err
	}
	return s.Refresh(ctx, resolved)
}

func (s *RecommendationService) resolveWorkspace(ctx context.Context, query DashboardQuery) (workspace.ResolvedContext, error) {
	if s == nil {
		return workspace.ResolvedContext{}, errors.New("recommendation service is not configured")
	}
	if s.resolver == nil {
		return workspace.ResolvedContext{}, errors.New("recommendation workspace resolver is not configured")
	}
	return s.resolver.Resolve(ctx, workspace.ContextQuery{
		HubUserID:     query.HubUserID,
		DeviceCount:   query.DeviceCount,
		ContextSource: query.ContextSource,
	})
}

func (s *RecommendationService) GetPersisted(ctx context.Context, workspaceID string) (RecommendationSnapshot, bool, error) {
	if s == nil || s.repository == nil {
		return RecommendationSnapshot{}, false, errors.New("recommendation repository is not configured")
	}
	snapshot, found, err := s.repository.GetRecommendations(ctx, workspaceID)
	return cloneRecommendationSnapshot(snapshot), found, err
}

func (s *RecommendationService) Reset(ctx context.Context, workspaceID string) error {
	if s == nil || s.repository == nil {
		return errors.New("recommendation repository is not configured")
	}
	return s.repository.ResetRecommendations(ctx, workspaceID)
}

func ApplyRecommendationSnapshot(payload *Dashboard, snapshot RecommendationSnapshot) {
	if payload == nil {
		return
	}
	payload.AccountRecommendations = append([]AccountRecommendation(nil), snapshot.AccountRecommendations...)
	payload.VideoRecommendations = append([]VideoRecommendation(nil), snapshot.VideoRecommendations...)
	hotVideos := hotVideosFromRecommendations(snapshot.VideoRecommendations)
	payload.HotVideoRankings = BuildHotVideoRankings(hotVideos)
	referenceAt := payload.GeneratedAt
	if referenceAt.IsZero() {
		referenceAt = snapshot.GeneratedAt
	}
	payload.HotVideoWindowRankings = BuildHotVideoWindowRankings(hotVideos, referenceAt)
	payload.HotVideos = cloneHotVideos(payload.HotVideoRankings[HotVideoRankingHot])
	payload.HotTopics = hotTopicsFromRecommendations(snapshot.TopicRecommendations)
}

func hotVideosFromRecommendations(items []VideoRecommendation) []HotVideo {
	results := make([]HotVideo, 0, len(items))
	for _, item := range items {
		results = append(results, HotVideo{
			ID:                  item.ID,
			Title:               item.Title,
			ChannelUsername:     item.ChannelUsername,
			ChannelName:         item.ChannelName,
			Source:              item.Source,
			ObjectID:            item.ObjectID,
			ObjectNonceID:       item.ObjectNonceID,
			CoverURL:            item.CoverURL,
			Tags:                append([]string(nil), item.Tags...),
			PublishedAt:         item.PublishedAt,
			ObservedAt:          item.LastObservedAt,
			Views:               item.Metrics.Views,
			Likes:               item.Metrics.Likes,
			Comments:            item.Metrics.Comments,
			Favorites:           item.Metrics.Favorites,
			Forwards:            item.Metrics.Forwards,
			GrowthRate:          item.GrowthRate,
			InteractionScore:    item.InteractionScore,
			GrowthScore:         item.GrowthScore,
			RecommendationScore: item.RecommendationScore,
			EngagementScore:     int(item.RecommendationScore * 100),
			PositiveDeltas:      item.PositiveDeltas,
			ObservationCount:    item.ObservationCount,
			HasGrowthBaseline:   item.HasGrowthBaseline,
			InsufficientHistory: item.InsufficientHistory,
			DataBasis:           item.DataBasis,
			Reason:              item.Reason,
		})
	}
	return results
}

func hotTopicsFromRecommendations(items []TopicRecommendation) []HotTopic {
	results := make([]HotTopic, 0, minInt(len(items), hotTopicRankingLimit))
	for _, item := range items {
		if item.ObservationCount == 0 {
			continue
		}
		results = append(results, HotTopic{
			ID:                  item.ID,
			Keyword:             item.Keyword,
			Cluster:             item.Cluster,
			Source:              item.Source,
			Sources:             append([]string(nil), item.Sources...),
			ObservedAt:          item.LastObservedAt,
			SourceAccounts:      append([]string(nil), item.SourceAccounts...),
			Mentions:            item.Mentions,
			Comments:            item.Metrics.Comments,
			GrowthRate:          item.GrowthRate,
			RelatedVideos:       item.RelatedVideos,
			InteractionScore:    item.InteractionScore,
			GrowthScore:         item.GrowthScore,
			RecommendationScore: item.RecommendationScore,
			PositiveDeltas:      item.PositiveDeltas,
			ObservationCount:    item.ObservationCount,
			HasGrowthBaseline:   item.HasGrowthBaseline,
			InsufficientHistory: item.InsufficientHistory,
			DataBasis:           item.DataBasis,
			Reason:              item.Reason,
		})
		if len(results) >= hotTopicRankingLimit {
			break
		}
	}
	return results
}
