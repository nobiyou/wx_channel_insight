package normalize

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type Query struct {
	HubUserID     string
	DeviceCount   int
	ContextSource string
}

type Service struct {
	repo            Repository
	contextResolver workspaceContextResolver
	ingestEvents    ingestEventProvider
	now             func() time.Time
}

const maxStableIDBytes = 240

type ServiceConfig struct {
	WorkspaceContextResolver workspaceContextResolver
	IngestEvents             ingestEventProvider
	Now                      func() time.Time
}

type workspaceContextResolver interface {
	Resolve(ctx context.Context, query workspace.ContextQuery) (workspace.ResolvedContext, error)
}

type ingestEventProvider interface {
	ListEvents(ctx context.Context, query ingest.SummaryQuery) ([]ingest.Event, error)
}

func NewService(repo Repository, config ServiceConfig) *Service {
	now := config.Now
	if now == nil {
		now = time.Now
	}

	return &Service{
		repo:            repo,
		contextResolver: config.WorkspaceContextResolver,
		ingestEvents:    config.IngestEvents,
		now:             now,
	}
}

func (s *Service) Normalize(ctx context.Context, query Query) (Snapshot, error) {
	if s.contextResolver == nil {
		return Snapshot{}, fmt.Errorf("workspace context resolver is not configured")
	}
	workspaceContext, err := s.contextResolver.Resolve(ctx, workspace.ContextQuery{
		HubUserID:     query.HubUserID,
		DeviceCount:   query.DeviceCount,
		ContextSource: query.ContextSource,
	})
	if err != nil {
		return Snapshot{}, err
	}
	return s.NormalizeWithWorkspace(ctx, workspaceContext)
}

func (s *Service) NormalizeWithWorkspace(ctx context.Context, workspaceContext workspace.ResolvedContext) (Snapshot, error) {
	if s.repo == nil {
		return Snapshot{}, fmt.Errorf("normalization repository is not configured")
	}
	if s.ingestEvents == nil {
		return Snapshot{}, fmt.Errorf("ingest events provider is not configured")
	}

	events, err := s.ingestEvents.ListEvents(ctx, ingest.SummaryQuery{
		HubUserID:   workspaceContext.Workspace.OwnerUserID,
		DeviceCount: workspaceContext.Workspace.DeviceCount,
	})
	if err != nil {
		return Snapshot{}, err
	}

	snapshot := BuildSnapshot(workspaceContext, events, s.now())
	if err := s.repo.ReplaceSnapshot(ctx, workspaceContext.Workspace.ID, snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s *Service) RefreshSnapshot(ctx context.Context, workspaceContext workspace.ResolvedContext) error {
	_, err := s.NormalizeWithWorkspace(ctx, workspaceContext)
	return err
}

func (s *Service) GetSummary(ctx context.Context, workspaceID string) (Summary, bool, error) {
	if s.repo == nil {
		return Summary{}, false, fmt.Errorf("normalization repository is not configured")
	}
	return s.repo.GetSummary(ctx, workspaceID)
}

func (s *Service) GetSnapshot(ctx context.Context, workspaceID string) (Snapshot, bool, error) {
	if s.repo == nil {
		return Snapshot{}, false, fmt.Errorf("normalization repository is not configured")
	}
	return s.repo.GetSnapshot(ctx, workspaceID)
}

func BuildSnapshot(workspaceContext workspace.ResolvedContext, events []ingest.Event, generatedAt time.Time) Snapshot {
	sortedEvents := append([]ingest.Event(nil), events...)
	sort.Slice(sortedEvents, func(i, j int) bool {
		if sortedEvents[i].CollectedAt.Equal(sortedEvents[j].CollectedAt) {
			return sortedEvents[i].ID > sortedEvents[j].ID
		}
		return sortedEvents[i].CollectedAt.After(sortedEvents[j].CollectedAt)
	})

	contentByKey := map[string]ContentSnapshot{}
	commentByKey := map[string]CommentSnapshot{}
	topicByKey := map[string]TopicObservation{}
	accountByKey := map[string]AccountObservation{}

	latestBatchID := ""
	latestEventAt := time.Time{}
	for _, event := range sortedEvents {
		if latestBatchID == "" && strings.TrimSpace(event.BatchID) != "" {
			latestBatchID = strings.TrimSpace(event.BatchID)
		}
		if latestEventAt.IsZero() && !event.CollectedAt.IsZero() {
			latestEventAt = event.CollectedAt
		}

		if item, ok := buildContentSnapshot(event); ok {
			contentByKey[item.ID] = item
		}
		if item, ok := buildCommentSnapshot(event); ok {
			commentByKey[item.ID] = item
		}
		if item, ok := buildTopicObservation(event); ok {
			topicByKey[item.ID] = item
		}
		if item, ok := buildAccountObservation(event); ok {
			accountByKey[item.ID] = item
		}
	}

	content := mapValues(contentByKey)
	comments := mapValues(commentByKey)
	topics := mapValues(topicByKey)
	accounts := mapValues(accountByKey)

	sort.Slice(content, func(i, j int) bool {
		if content[i].CollectedAt.Equal(content[j].CollectedAt) {
			return content[i].ID < content[j].ID
		}
		return content[i].CollectedAt.After(content[j].CollectedAt)
	})
	sort.Slice(comments, func(i, j int) bool {
		if comments[i].CollectedAt.Equal(comments[j].CollectedAt) {
			return comments[i].ID < comments[j].ID
		}
		return comments[i].CollectedAt.After(comments[j].CollectedAt)
	})
	sort.Slice(topics, func(i, j int) bool {
		if topics[i].CollectedAt.Equal(topics[j].CollectedAt) {
			return topics[i].ID < topics[j].ID
		}
		return topics[i].CollectedAt.After(topics[j].CollectedAt)
	})
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].CollectedAt.Equal(accounts[j].CollectedAt) {
			return accounts[i].ID < accounts[j].ID
		}
		return accounts[i].CollectedAt.After(accounts[j].CollectedAt)
	})

	state := "empty"
	if len(sortedEvents) > 0 {
		state = "ready"
	}

	return Snapshot{
		Summary: Summary{
			WorkspaceID:   workspaceContext.Workspace.ID,
			HubUserID:     workspaceContext.Workspace.OwnerUserID,
			State:         state,
			LatestBatchID: latestBatchID,
			GeneratedAt:   generatedAt,
			LatestEventAt: latestEventAt,
			ContentCount:  len(content),
			CommentCount:  len(comments),
			TopicCount:    len(topics),
			AccountCount:  len(accounts),
		},
		ContentSnapshots:    content,
		CommentSnapshots:    comments,
		TopicObservations:   topics,
		AccountObservations: accounts,
	}
}

func buildContentSnapshot(event ingest.Event) (ContentSnapshot, bool) {
	title := firstNonEmpty(event.Title, event.Keyword, event.ChannelName)
	if title == "" {
		return ContentSnapshot{}, false
	}
	channelName := firstNonEmpty(event.ChannelName, event.Source)
	item := ContentSnapshot{
		ID:              buildStableID(channelName, title),
		BatchID:         event.BatchID,
		EventID:         event.ID,
		Source:          event.Source,
		CollectedAt:     event.CollectedAt,
		PublishedAt:     event.PublishedAt,
		ObjectID:        event.ObjectID,
		ObjectNonceID:   event.ObjectNonceID,
		ChannelUsername: strings.TrimSpace(event.ChannelUsername),
		CoverURL:        strings.TrimSpace(event.CoverURL),
		Title:           title,
		ChannelName:     channelName,
		Keyword:         strings.TrimSpace(event.Keyword),
		Signal:          strings.TrimSpace(event.Signal),
		PromptAngle:     strings.TrimSpace(event.PromptAngle),
		TargetAudience:  strings.TrimSpace(event.TargetAudience),
		VideoCount:      event.VideoCount,
		PlayCount:       event.PlayCount,
		LikeCount:       event.LikeCount,
		CommentCount:    event.CommentCount,
		FavCount:        event.FavCount,
		ForwardCount:    event.ForwardCount,
		TopicCount:      event.TopicCount,
		Tags:            append([]string(nil), event.Tags...),
		Keywords:        append([]string(nil), event.Keywords...),
	}
	return item, true
}

func buildCommentSnapshot(event ingest.Event) (CommentSnapshot, bool) {
	representative := strings.TrimSpace(event.RepresentativeComment)
	if representative == "" && event.CommentCount <= 0 {
		return CommentSnapshot{}, false
	}
	title := firstNonEmpty(event.Title, event.Keyword, event.ChannelName)
	item := CommentSnapshot{
		ID:                    buildStableID(title, representative),
		BatchID:               event.BatchID,
		EventID:               event.ID,
		Source:                event.Source,
		CollectedAt:           event.CollectedAt,
		ObjectID:              event.ObjectID,
		ObjectNonceID:         event.ObjectNonceID,
		Title:                 title,
		ChannelName:           strings.TrimSpace(event.ChannelName),
		Keyword:               strings.TrimSpace(event.Keyword),
		Signal:                strings.TrimSpace(event.Signal),
		RepresentativeComment: representative,
		TargetAudience:        strings.TrimSpace(event.TargetAudience),
		VideoCount:            event.VideoCount,
		CommentCount:          event.CommentCount,
		TopicCount:            event.TopicCount,
		Keywords:              append([]string(nil), event.Keywords...),
	}
	return item, true
}

func buildTopicObservation(event ingest.Event) (TopicObservation, bool) {
	keyword := strings.TrimSpace(event.Keyword)
	cluster := strings.TrimSpace(event.Cluster)
	promptAngle := strings.TrimSpace(event.PromptAngle)
	if keyword == "" && cluster == "" && promptAngle == "" && event.TopicCount <= 0 {
		return TopicObservation{}, false
	}
	item := TopicObservation{
		ID:             buildStableID(keyword, cluster),
		BatchID:        event.BatchID,
		EventID:        event.ID,
		Source:         event.Source,
		CollectedAt:    event.CollectedAt,
		ObjectID:       event.ObjectID,
		ObjectNonceID:  event.ObjectNonceID,
		Keyword:        keyword,
		Cluster:        cluster,
		Signal:         strings.TrimSpace(event.Signal),
		PromptAngle:    promptAngle,
		TargetAudience: strings.TrimSpace(event.TargetAudience),
		VideoCount:     event.VideoCount,
		CommentCount:   event.CommentCount,
		TopicCount:     event.TopicCount,
		Keywords:       append([]string(nil), event.Keywords...),
	}
	return item, true
}

func buildAccountObservation(event ingest.Event) (AccountObservation, bool) {
	channelName := firstNonEmpty(event.ChannelName, event.Source)
	if channelName == "" {
		return AccountObservation{}, false
	}
	item := AccountObservation{
		ID:           buildStableID(firstNonEmpty(event.ChannelUsername, channelName)),
		BatchID:      event.BatchID,
		EventID:      event.ID,
		Source:       event.Source,
		CollectedAt:  event.CollectedAt,
		Username:     strings.TrimSpace(event.ChannelUsername),
		ChannelName:  channelName,
		Title:        firstNonEmpty(event.Title, event.Keyword),
		Signal:       strings.TrimSpace(event.Signal),
		VideoCount:   event.VideoCount,
		CommentCount: event.CommentCount,
		TopicCount:   event.TopicCount,
		Tags:         append([]string(nil), event.Tags...),
	}
	return item, true
}

func buildStableID(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(strings.ToLower(part))
		if value == "" {
			continue
		}
		replacer := strings.NewReplacer(
			" ", "-",
			"\t", "-",
			"\n", "-",
			"\r", "-",
			"/", "-",
			"\\", "-",
			":", "-",
			"|", "-",
		)
		value = replacer.Replace(value)
		value = strings.Trim(value, "-")
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	if len(normalized) == 0 {
		return "unknown"
	}
	stableID := strings.Join(normalized, "::")
	if len(stableID) <= maxStableIDBytes {
		return stableID
	}

	// Keep persistence keys bounded without truncating distinct long titles into
	// the same identifier.
	digest := sha256.Sum256([]byte(stableID))
	return fmt.Sprintf("stable-%x", digest)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mapValues[T any](items map[string]T) []T {
	results := make([]T, 0, len(items))
	for _, item := range items {
		results = append(results, item)
	}
	return results
}
