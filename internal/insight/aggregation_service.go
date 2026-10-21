package insight

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/normalize"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type AggregationService struct {
	repo       aggregate.Repository
	normalizer normalizationSnapshotProvider
	now        func() time.Time
}

type AggregationServiceConfig struct {
	Normalizer normalizationSnapshotProvider
	Now        func() time.Time
}

func NewAggregationService(repo aggregate.Repository, config AggregationServiceConfig) *AggregationService {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &AggregationService{
		repo:       repo,
		normalizer: config.Normalizer,
		now:        now,
	}
}

func (s *AggregationService) RefreshAggregate(ctx context.Context, workspaceContext workspace.ResolvedContext) (aggregate.Aggregate, error) {
	if s.repo == nil {
		return aggregate.Aggregate{}, fmt.Errorf("aggregation repository is not configured")
	}
	if s.normalizer == nil {
		return aggregate.Aggregate{}, fmt.Errorf("normalization provider is not configured")
	}
	if err := s.normalizer.RefreshSnapshot(ctx, workspaceContext); err != nil {
		return aggregate.Aggregate{}, err
	}
	snapshot, found, err := s.normalizer.GetSnapshot(ctx, workspaceContext.Workspace.ID)
	if err != nil {
		return aggregate.Aggregate{}, err
	}
	if !found {
		return aggregate.Aggregate{}, fmt.Errorf("normalized snapshot is missing for workspace %s", workspaceContext.Workspace.ID)
	}
	result := s.buildAggregate(snapshot, workspaceContext)
	if err := s.repo.ReplaceAggregate(ctx, workspaceContext.Workspace.ID, result); err != nil {
		return aggregate.Aggregate{}, err
	}
	return result, nil
}

func (s *AggregationService) GetSummary(ctx context.Context, workspaceID string) (aggregate.Summary, bool, error) {
	if s.repo == nil {
		return aggregate.Summary{}, false, fmt.Errorf("aggregation repository is not configured")
	}
	return s.repo.GetSummary(ctx, workspaceID)
}

func (s *AggregationService) GetAggregate(ctx context.Context, workspaceID string) (aggregate.Aggregate, bool, error) {
	if s.repo == nil {
		return aggregate.Aggregate{}, false, fmt.Errorf("aggregation repository is not configured")
	}
	return s.repo.GetAggregate(ctx, workspaceID)
}

func (s *AggregationService) buildAggregate(snapshot normalize.Snapshot, workspaceContext workspace.ResolvedContext) aggregate.Aggregate {
	summary := buildSnapshotIngestSummary(snapshot)
	topicStats := buildHotTopicMetricStats(summary.RecentEvents)
	videoMetrics := make([]aggregate.VideoMetric, 0, len(snapshot.ContentSnapshots))
	topicMetrics := make([]aggregate.TopicMetric, 0, len(snapshot.TopicObservations))
	commentFacts := make([]aggregate.CommentInsightFact, 0, len(snapshot.CommentSnapshots))
	videoEvents := make([]ingest.Event, 0, len(summary.RecentEvents))
	seenVideos := map[string]struct{}{}
	seenTopics := map[string]struct{}{}
	seenComments := map[string]struct{}{}

	for _, event := range summary.RecentEvents {
		switch event.EventType {
		case "video_snapshot":
			videoEvents = append(videoEvents, event)
			if view := buildHotVideoFromIngest(summary, event); view != nil {
				key := view.Title
				if key == "" {
					key = view.ID
				}
				if _, ok := seenVideos[key]; !ok {
					seenVideos[key] = struct{}{}
					videoMetrics = append(videoMetrics, mapHotVideoToMetric(*view, event))
				}
			}
		case "topic_cluster":
			if view := buildHotTopicFromIngestWithStats(summary, event, topicStats); view != nil {
				key := view.Keyword
				if key == "" {
					key = view.ID
				}
				if _, ok := seenTopics[key]; !ok {
					seenTopics[key] = struct{}{}
					topicMetrics = append(topicMetrics, mapHotTopicToMetric(*view, event))
				}
			}
		case "comment_snapshot":
			if view := buildCommentInsightFromIngest(summary, event); view != nil {
				key := view.Theme
				if key == "" {
					key = view.ID
				}
				if _, ok := seenComments[key]; !ok {
					seenComments[key] = struct{}{}
					commentFacts = append(commentFacts, mapCommentInsightToFact(*view, event))
				}
			}
		}
	}

	for _, event := range videoEvents {
		if view := buildHotTopicFromVideoWithStats(summary, event, topicStats); view != nil {
			key := view.Keyword
			if key == "" {
				key = view.ID
			}
			if _, ok := seenTopics[key]; !ok {
				seenTopics[key] = struct{}{}
				topicMetrics = append(topicMetrics, mapHotTopicToMetric(*view, event))
			}
		}
		if view := buildCommentInsightFromVideo(summary, event); view != nil {
			key := view.Theme
			if key == "" {
				key = view.ID
			}
			if _, ok := seenComments[key]; !ok {
				seenComments[key] = struct{}{}
				commentFacts = append(commentFacts, mapCommentInsightToFact(*view, event))
			}
		}
	}

	sort.Slice(videoMetrics, func(i, j int) bool {
		if videoMetrics[i].ObservedAt.Equal(videoMetrics[j].ObservedAt) {
			return videoMetrics[i].ID < videoMetrics[j].ID
		}
		return videoMetrics[i].ObservedAt.After(videoMetrics[j].ObservedAt)
	})
	sort.Slice(topicMetrics, func(i, j int) bool {
		if topicMetrics[i].ObservedAt.Equal(topicMetrics[j].ObservedAt) {
			return topicMetrics[i].ID < topicMetrics[j].ID
		}
		return topicMetrics[i].ObservedAt.After(topicMetrics[j].ObservedAt)
	})
	sort.Slice(commentFacts, func(i, j int) bool {
		if commentFacts[i].ObservedAt.Equal(commentFacts[j].ObservedAt) {
			return commentFacts[i].ID < commentFacts[j].ID
		}
		return commentFacts[i].ObservedAt.After(commentFacts[j].ObservedAt)
	})

	return aggregate.Aggregate{
		Summary: aggregate.Summary{
			WorkspaceID:        workspaceContext.Workspace.ID,
			HubUserID:          workspaceContext.Workspace.OwnerUserID,
			State:              snapshot.Summary.State,
			LatestBatchID:      snapshot.Summary.LatestBatchID,
			GeneratedAt:        s.now(),
			LatestObservedAt:   snapshot.Summary.LatestEventAt,
			VideoMetricCount:   len(videoMetrics),
			TopicMetricCount:   len(topicMetrics),
			CommentFactCount:   len(commentFacts),
			SourceAccountCount: snapshot.Summary.AccountCount,
		},
		VideoMetrics:        videoMetrics,
		TopicMetrics:        topicMetrics,
		CommentInsightFacts: commentFacts,
	}
}

func buildSnapshotIngestSummary(snapshot normalize.Snapshot) ingest.Summary {
	events := make([]ingest.Event, 0, len(snapshot.ContentSnapshots)+len(snapshot.CommentSnapshots)+len(snapshot.TopicObservations))
	for _, item := range snapshot.ContentSnapshots {
		events = append(events, ingest.Event{
			ID:              item.EventID,
			BatchID:         item.BatchID,
			EventType:       "video_snapshot",
			Source:          item.Source,
			CollectedAt:     item.CollectedAt,
			PublishedAt:     item.PublishedAt,
			ObjectID:        item.ObjectID,
			ObjectNonceID:   item.ObjectNonceID,
			ChannelUsername: item.ChannelUsername,
			VideoCount:      item.VideoCount,
			PlayCount:       item.PlayCount,
			LikeCount:       item.LikeCount,
			CommentCount:    item.CommentCount,
			FavCount:        item.FavCount,
			ForwardCount:    item.ForwardCount,
			TopicCount:      item.TopicCount,
			Title:           item.Title,
			ChannelName:     item.ChannelName,
			Keyword:         item.Keyword,
			Signal:          item.Signal,
			PromptAngle:     item.PromptAngle,
			TargetAudience:  item.TargetAudience,
			Tags:            append([]string(nil), item.Tags...),
			Keywords:        append([]string(nil), item.Keywords...),
		})
	}
	for _, item := range snapshot.CommentSnapshots {
		events = append(events, ingest.Event{
			ID:                    item.EventID,
			BatchID:               item.BatchID,
			EventType:             "comment_snapshot",
			Source:                item.Source,
			CollectedAt:           item.CollectedAt,
			ObjectID:              item.ObjectID,
			ObjectNonceID:         item.ObjectNonceID,
			VideoCount:            item.VideoCount,
			CommentCount:          item.CommentCount,
			TopicCount:            item.TopicCount,
			Title:                 item.Title,
			ChannelName:           item.ChannelName,
			Keyword:               item.Keyword,
			Signal:                item.Signal,
			RepresentativeComment: item.RepresentativeComment,
			TargetAudience:        item.TargetAudience,
			Keywords:              append([]string(nil), item.Keywords...),
		})
	}
	for _, item := range snapshot.TopicObservations {
		events = append(events, ingest.Event{
			ID:             item.EventID,
			BatchID:        item.BatchID,
			EventType:      "topic_cluster",
			Source:         item.Source,
			CollectedAt:    item.CollectedAt,
			ObjectID:       item.ObjectID,
			ObjectNonceID:  item.ObjectNonceID,
			VideoCount:     item.VideoCount,
			CommentCount:   item.CommentCount,
			TopicCount:     item.TopicCount,
			Keyword:        item.Keyword,
			Cluster:        item.Cluster,
			Signal:         item.Signal,
			PromptAngle:    item.PromptAngle,
			TargetAudience: item.TargetAudience,
			Keywords:       append([]string(nil), item.Keywords...),
		})
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].CollectedAt.Equal(events[j].CollectedAt) {
			if events[i].BatchID == events[j].BatchID {
				return events[i].ID > events[j].ID
			}
			return events[i].BatchID > events[j].BatchID
		}
		return events[i].CollectedAt.After(events[j].CollectedAt)
	})
	return ingest.Summary{
		HubUserID:          snapshot.Summary.HubUserID,
		State:              snapshot.Summary.State,
		FreshnessState:     "fresh",
		TotalEvents:        len(events),
		SourceCount:        snapshot.Summary.AccountCount,
		LatestEventAt:      snapshot.Summary.LatestEventAt,
		LastSuccessAt:      snapshot.Summary.GeneratedAt,
		LatestBatchID:      snapshot.Summary.LatestBatchID,
		AggregatedVideos:   snapshot.Summary.ContentCount,
		AggregatedComments: snapshot.Summary.CommentCount,
		AggregatedTopics:   snapshot.Summary.TopicCount,
		RecentEvents:       events,
	}
}

func mapHotVideoToMetric(view HotVideo, event ingest.Event) aggregate.VideoMetric {
	return aggregate.VideoMetric{
		ID:              view.ID,
		EventID:         event.ID,
		BatchID:         event.BatchID,
		Source:          event.Source,
		ObservedAt:      event.CollectedAt,
		PublishedAt:     view.PublishedAt,
		ObjectID:        event.ObjectID,
		ObjectNonceID:   event.ObjectNonceID,
		CoverURL:        view.CoverURL,
		Title:           view.Title,
		ChannelName:     view.ChannelName,
		Keyword:         event.Keyword,
		Signal:          view.Signal,
		PromptAngle:     event.PromptAngle,
		TargetAudience:  event.TargetAudience,
		Tags:            append([]string(nil), view.Tags...),
		Keywords:        append([]string(nil), event.Keywords...),
		Views:           view.Views,
		Likes:           view.Likes,
		Comments:        view.Comments,
		Favorites:       view.Favorites,
		Forwards:        view.Forwards,
		GrowthRate:      view.GrowthRate,
		EngagementScore: view.EngagementScore,
	}
}

func mapHotTopicToMetric(view HotTopic, event ingest.Event) aggregate.TopicMetric {
	return aggregate.TopicMetric{
		ID:             view.ID,
		EventID:        event.ID,
		BatchID:        event.BatchID,
		Source:         event.Source,
		ObservedAt:     event.CollectedAt,
		Keyword:        view.Keyword,
		Cluster:        view.Cluster,
		Signal:         event.Signal,
		PromptAngle:    view.PromptAngle,
		TargetAudience: event.TargetAudience,
		Keywords:       append([]string(nil), view.Keywords...),
		Mentions:       view.Mentions,
		GrowthRate:     view.GrowthRate,
		RelatedVideos:  view.RelatedVideos,
	}
}

func mapCommentInsightToFact(view CommentInsight, event ingest.Event) aggregate.CommentInsightFact {
	return aggregate.CommentInsightFact{
		ID:               view.ID,
		EventID:          event.ID,
		BatchID:          event.BatchID,
		Source:           event.Source,
		ObservedAt:       event.CollectedAt,
		ObjectID:         event.ObjectID,
		ObjectNonceID:    event.ObjectNonceID,
		Theme:            view.Theme,
		Sentiment:        view.Sentiment,
		Representative:   view.Representative,
		ActionSuggestion: view.ActionSuggestion,
		TargetAudience:   event.TargetAudience,
		RelatedKeywords:  append([]string(nil), view.RelatedKeywords...),
		QuestionRate:     view.QuestionRate,
		CommentCount:     event.CommentCount,
	}
}
