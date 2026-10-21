package insight

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type persistedRecommendationResolver struct {
	resolved workspace.ResolvedContext
}

func (s persistedRecommendationResolver) Resolve(context.Context, workspace.ContextQuery) (workspace.ResolvedContext, error) {
	return s.resolved, nil
}

type countingRecommendationProvider struct {
	calls int
}

type recommendationMemoryRepository struct {
	snapshot RecommendationSnapshot
	found    bool
}

type recommendationCommentRepository struct {
	items []CommentInsight
}

func (r recommendationCommentRepository) GetCommentInsights(context.Context, string) ([]CommentInsight, bool, error) {
	return CloneCommentInsights(r.items), true, nil
}

func (recommendationCommentRepository) GetMeta(context.Context, string) (ReadModelArtifactMeta, bool, error) {
	return ReadModelArtifactMeta{}, false, nil
}

func (recommendationCommentRepository) ReplaceCommentInsights(context.Context, string, []CommentInsight) error {
	return nil
}

func (recommendationCommentRepository) ReplaceCommentInsightMeta(context.Context, string, ReadModelArtifactMeta) error {
	return nil
}

func (recommendationCommentRepository) ResetCommentInsights(context.Context, string) error {
	return nil
}

func (r *recommendationMemoryRepository) GetRecommendations(context.Context, string) (RecommendationSnapshot, bool, error) {
	return CloneRecommendationSnapshot(r.snapshot), r.found, nil
}

func (r *recommendationMemoryRepository) ReplaceRecommendations(_ context.Context, _ string, snapshot RecommendationSnapshot) error {
	r.snapshot = CloneRecommendationSnapshot(snapshot)
	r.found = true
	return nil
}

func (r *recommendationMemoryRepository) ResetRecommendations(context.Context, string) error {
	r.snapshot = RecommendationSnapshot{}
	r.found = false
	return nil
}

func (p *countingRecommendationProvider) ListEvents(context.Context, ingest.SummaryQuery) ([]ingest.Event, error) {
	p.calls++
	return nil, nil
}

func (p *countingRecommendationProvider) ListVideoMetricObservations(context.Context, ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	p.calls++
	return nil, nil
}

func (p *countingRecommendationProvider) ListBindings(context.Context, ingest.SummaryQuery) ([]ingest.SourceBinding, error) {
	p.calls++
	return nil, nil
}

func TestReadModelRecommendationsPreferPersistedSnapshot(t *testing.T) {
	ctx := context.Background()
	workspaceContext := workspace.ResolvedContext{
		Workspace: workspace.DefaultWorkspace{ID: "ws-persisted-recommendations", OwnerUserID: "hub:user:test"},
	}
	resolver := persistedRecommendationResolver{resolved: workspaceContext}
	repository := &recommendationMemoryRepository{}
	generatedAt := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	if err := repository.ReplaceRecommendations(ctx, workspaceContext.Workspace.ID, RecommendationSnapshot{
		WorkspaceID: workspaceContext.Workspace.ID,
		GeneratedAt: generatedAt,
		AccountRecommendations: []AccountRecommendation{{
			ID: "persisted-account", Username: "persisted-user", LastObservedAt: generatedAt.Add(-2 * time.Hour),
		}},
	}); err != nil {
		t.Fatalf("persist recommendation snapshot: %v", err)
	}
	provider := &countingRecommendationProvider{}
	recommendationService := NewRecommendationService(RecommendationServiceConfig{
		DataProvider: provider, Repository: repository, WorkspaceContextResolver: resolver,
	})
	readModelService := NewReadModelService(nil,
		WithRecommendationService(recommendationService),
		WithWorkspaceContextResolver(resolver),
	)

	items, _, err := readModelService.GetAccountRecommendations(ctx, DashboardQuery{
		HubUserID: "hub:user:test", TimeWindow: TimeWindow24h,
	})
	if err != nil {
		t.Fatalf("read persisted recommendations: %v", err)
	}
	if len(items) != 1 || items[0].ID != "persisted-account" {
		t.Fatalf("expected persisted recommendation, got %+v", items)
	}
	if provider.calls != 0 {
		t.Fatalf("read model should not refresh recommendation data, provider calls=%d", provider.calls)
	}
}

func TestRecommendationScoreRetainsItsRelativeBaselineEligibility(t *testing.T) {
	base := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	observations := []ingest.VideoMetricObservation{
		{ID: "zero", ObjectID: "new-video", Source: ingest.RunSourceWXVideoSearch, ObservedAt: base},
		{ID: "positive", ObjectID: "new-video", Source: ingest.RunSourceWXVideoSearch, ObservedAt: base.Add(time.Hour), LikeCount: 20},
	}
	snapshot := BuildRecommendationSnapshot("workspace", "user", nil, observations, nil, base.Add(time.Hour))
	if len(snapshot.VideoRecommendations) != 1 {
		t.Fatalf("missing recommendation: %+v", snapshot)
	}
	row := snapshot.VideoRecommendations[0]
	if row.HasGrowthBaseline || row.GrowthScore != 0 || row.PositiveDeltas.Total != 0 {
		t.Fatalf("factual absolute increments changed recommendation eligibility: %+v", row)
	}
}

func TestBuildRecommendationSnapshotUsesFourMetricsAndPositiveGrowth(t *testing.T) {
	firstAt := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Hour)
	observations := []ingest.VideoMetricObservation{
		{
			ID: "candidate-old", EventID: "event-old", Source: ingest.RunSourceWXVideoSearch,
			ObservedAt: firstAt, ObjectID: "video-candidate", ChannelUsername: "candidate@finder",
			ChannelName: "候选账号", Title: "真实候选视频", Keyword: "真实主题", Tags: []string{"真实主题"},
			LikeCount: 10, CommentCount: 2, FavCount: 3, ForwardCount: 1,
		},
		{
			ID: "candidate-new", EventID: "event-new", Source: ingest.RunSourceWXVideoSearch,
			ObservedAt: secondAt, ObjectID: "video-candidate", ChannelUsername: "candidate@finder",
			ChannelName: "候选账号", Title: "真实候选视频", Keyword: "真实主题", Tags: []string{"真实主题"},
			LikeCount: 25, CommentCount: 5, FavCount: 6, ForwardCount: 4,
		},
		{
			ID: "monitored-new", EventID: "event-monitored", Source: ingest.RunSourceWXFeedList,
			ObservedAt: secondAt, ObjectID: "video-monitored", ChannelUsername: "already@finder",
			ChannelName: "已监控账号", Title: "已监控视频", LikeCount: 999,
		},
	}
	snapshot := BuildRecommendationSnapshot(
		"workspace-test",
		"hub:user:test",
		nil,
		observations,
		[]ingest.SourceBinding{{Source: ingest.RunSourceWXFeedList, Username: "already@finder"}},
		secondAt.Add(time.Minute),
	)

	if len(snapshot.AccountRecommendations) != 1 {
		t.Fatalf("expected monitored account exclusion, got %+v", snapshot.AccountRecommendations)
	}
	account := snapshot.AccountRecommendations[0]
	if account.Username != "candidate@finder" || account.VideoCount != 1 {
		t.Fatalf("expected candidate account, got %+v", account)
	}
	if account.Metrics.Likes != 25 || account.Metrics.Comments != 5 || account.Metrics.Favorites != 6 || account.Metrics.Forwards != 4 {
		t.Fatalf("expected all current interaction metrics, got %+v", account.Metrics)
	}
	if account.PositiveDeltas.Total != 24 || !account.HasGrowthBaseline || account.InsufficientHistory {
		t.Fatalf("expected positive historical evidence, got %+v", account)
	}

	var candidate VideoRecommendation
	for _, item := range snapshot.VideoRecommendations {
		if item.ObjectID == "video-candidate" {
			candidate = item
		}
	}
	if candidate.RecommendationScore <= 0 || candidate.GrowthScore <= 0 {
		t.Fatalf("expected growth-weighted video score, got %+v", candidate)
	}
	if candidate.PositiveDeltas.Total != 24 || candidate.GrowthRate != 150 {
		t.Fatalf("expected video growth evidence to include total delta and rate, got %+v", candidate)
	}
	if candidate.Metrics.Views != 0 {
		t.Fatalf("views are display-only in this fixture, got %+v", candidate.Metrics)
	}
	if len(snapshot.TopicRecommendations) != 1 || snapshot.TopicRecommendations[0].Keyword != "真实主题" {
		t.Fatalf("expected topic to derive from real video keyword, got %+v", snapshot.TopicRecommendations)
	}
	topic := snapshot.TopicRecommendations[0]
	if topic.EvidenceStatus != TopicEvidenceStatusAvailable || len(topic.ReferenceVideos) != 1 {
		t.Fatalf("expected topic to carry one usable reference video, got %+v", topic)
	}
	if topic.ReferenceVideos[0].Title != "真实候选视频" || topic.ReferenceVideos[0].Metrics.Likes != 25 {
		t.Fatalf("expected reference video title and current metrics, got %+v", topic.ReferenceVideos)
	}
}

func TestTopicEvidenceMatchesBoundedCommentsByObjectIDThenKeyword(t *testing.T) {
	observedAt := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	topic := TopicRecommendation{
		Keyword: "真实主题",
		ReferenceVideos: []VideoRecommendation{{
			ID: "video-1", ObjectID: "object-1", Keyword: "真实主题", Title: "真实标题",
		}},
	}
	comments := []CommentInsight{
		{ID: "keyword-old", ObservedAt: observedAt.Add(-time.Hour), RelatedKeywords: []string{"真实主题"}},
		{ID: "exact-late", ObjectID: "object-1", ObservedAt: observedAt.Add(time.Hour)},
		{ID: "keyword-new", ObservedAt: observedAt.Add(2 * time.Hour), RelatedKeywords: []string{"真实主题"}},
	}
	for index := 0; index < 6; index++ {
		comments = append(comments, CommentInsight{ID: "keyword-extra-" + string(rune('a'+index)), RelatedKeywords: []string{"真实主题"}})
	}
	matched := matchTopicCommentEvidence(topic, comments)
	if len(matched) != 5 {
		t.Fatalf("expected five bounded comment insights, got %d: %+v", len(matched), matched)
	}
	if matched[0].ID != "exact-late" || matched[1].ID != "keyword-new" {
		t.Fatalf("expected exact object match before keyword matches, got %+v", matched)
	}
}

func TestRecommendationServiceAttachesCommentEvidenceAndMarksLegacyTopicUnavailable(t *testing.T) {
	service := NewRecommendationService(RecommendationServiceConfig{
		CommentInsightRepository: recommendationCommentRepository{items: []CommentInsight{{
			ID: "comment-1", ObjectID: "object-1", Representative: "真实追问",
		}}},
	})
	snapshot := RecommendationSnapshot{TopicRecommendations: []TopicRecommendation{
		{ID: "with-video", Keyword: "真实主题", ReferenceVideos: []VideoRecommendation{{ID: "video-1", ObjectID: "object-1", Title: "真实标题"}}},
		{ID: "legacy", Keyword: "只有关键词"},
	}}
	if err := service.attachTopicCommentEvidence(context.Background(), "ws-1", &snapshot); err != nil {
		t.Fatalf("attach comment evidence failed: %v", err)
	}
	if snapshot.TopicRecommendations[0].EvidenceStatus != TopicEvidenceStatusAvailable || len(snapshot.TopicRecommendations[0].CommentEvidence) != 1 {
		t.Fatalf("expected bound comment evidence, got %+v", snapshot.TopicRecommendations[0])
	}
	if snapshot.TopicRecommendations[1].EvidenceStatus != TopicEvidenceStatusUnavailable || len(snapshot.TopicRecommendations[1].ReferenceVideos) != 0 {
		t.Fatalf("expected legacy topic to be explicitly unavailable, got %+v", snapshot.TopicRecommendations[1])
	}
}

func TestCloneRecommendationSnapshotDeepCopiesTopicEvidence(t *testing.T) {
	source := RecommendationSnapshot{
		TopicRecommendations: []TopicRecommendation{{
			ReferenceVideos: []VideoRecommendation{{Tags: []string{"tag"}}},
			CommentEvidence: []CommentInsight{{RelatedKeywords: []string{"keyword"}, SourceAccounts: []string{"account"}}},
		}},
	}
	cloned := CloneRecommendationSnapshot(source)
	cloned.TopicRecommendations[0].ReferenceVideos[0].Tags[0] = "changed"
	cloned.TopicRecommendations[0].CommentEvidence[0].RelatedKeywords[0] = "changed"
	cloned.TopicRecommendations[0].CommentEvidence[0].SourceAccounts[0] = "changed"
	if source.TopicRecommendations[0].ReferenceVideos[0].Tags[0] != "tag" || source.TopicRecommendations[0].CommentEvidence[0].RelatedKeywords[0] != "keyword" || source.TopicRecommendations[0].CommentEvidence[0].SourceAccounts[0] != "account" {
		t.Fatalf("topic evidence slices must be deeply copied, source=%+v", source)
	}
}

func TestBuildRecommendationSnapshotMarksFirstObservationWithoutGrowth(t *testing.T) {
	snapshot := BuildRecommendationSnapshot("workspace-test", "hub:user:test", nil, []ingest.VideoMetricObservation{{
		ID: "one", EventID: "event-one", Source: ingest.RunSourceWXVideoSearch,
		ObservedAt: time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC), ObjectID: "one-video",
		ChannelUsername: "new@finder", Title: "第一次观察", LikeCount: 8,
	}}, nil, time.Date(2026, 8, 6, 8, 1, 0, 0, time.UTC))

	if len(snapshot.VideoRecommendations) != 1 {
		t.Fatalf("expected one video recommendation, got %+v", snapshot.VideoRecommendations)
	}
	item := snapshot.VideoRecommendations[0]
	if item.GrowthScore != 0 || item.GrowthRate != 0 || !item.InsufficientHistory || item.HasGrowthBaseline {
		t.Fatalf("first observation must not fabricate growth, got %+v", item)
	}
}

func TestBuildRecommendationSnapshotIgnoresRemovedManualTopicSources(t *testing.T) {
	observedAt := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	snapshot := BuildRecommendationSnapshot(
		"workspace-test",
		"hub:user:test",
		[]ingest.Event{
			{ID: "manual-topic:AI", EventType: "topic_manual", Source: "manual", CollectedAt: observedAt.Add(-time.Hour), Keyword: "AI"},
			{ID: "cluster-ai", EventType: "topic_cluster", Source: ingest.RunSourceWXVideoSearch, CollectedAt: observedAt, Keyword: "AI", TopicCount: 8, VideoCount: 3, CommentCount: 4},
		},
		nil,
		nil,
		observedAt.Add(time.Minute),
	)
	if len(snapshot.TopicRecommendations) != 1 {
		t.Fatalf("expected one merged topic candidate, got %+v", snapshot.TopicRecommendations)
	}
	topic := snapshot.TopicRecommendations[0]
	if topic.ObservationCount != 1 || topic.RelatedVideos != 3 || topic.Mentions != 8 {
		t.Fatalf("expected collected evidence to drive topic candidate, got %+v", topic)
	}
	if len(topic.Sources) != 1 || topic.Sources[0] != ingest.RunSourceWXVideoSearch {
		t.Fatalf("expected only current collected sources, got %v", topic.Sources)
	}

	var dashboard Dashboard
	ApplyRecommendationSnapshot(&dashboard, snapshot)
	if len(dashboard.HotTopics) != 1 || dashboard.HotTopics[0].Keyword != "AI" {
		t.Fatalf("collected topic should enter hot ranking, got %+v", dashboard.HotTopics)
	}
}

func TestBuildRecommendationSnapshotNormalizesVideoTopics(t *testing.T) {
	observedAt := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	snapshot := BuildRecommendationSnapshot(
		"workspace-test",
		"hub:user:test",
		nil,
		[]ingest.VideoMetricObservation{
			{ID: "video-1", EventID: "event-1", Source: ingest.RunSourceWXVideoSearch, ObservedAt: observedAt, ObjectID: "video-1", Title: "AI 视频 #AI#Codex", Keyword: "AI", Tags: []string{"小白也能轻松上手", "2"}, LikeCount: 10},
			{ID: "video-2", EventID: "event-2", Source: ingest.RunSourceWXVideoSearch, ObservedAt: observedAt.Add(time.Minute), ObjectID: "video-2", Title: "AI 视频 2", Keyword: "ai", Tags: []string{"AI"}, LikeCount: 20},
		},
		nil,
		observedAt.Add(time.Hour),
	)

	byKey := map[string]TopicRecommendation{}
	for _, topic := range snapshot.TopicRecommendations {
		byKey[canonicalTopicKey(topic.Keyword)] = topic
		if topic.Keyword == "2" || topic.Keyword == "小白也能轻松上手" || strings.Contains(topic.Keyword, "#") {
			t.Fatalf("invalid topic leaked into recommendations: %+v", topic)
		}
	}
	if len(byKey) != 2 {
		t.Fatalf("expected only AI and Codex topics, got %+v", snapshot.TopicRecommendations)
	}
	if got := byKey["ai"].RelatedVideos; got != 2 {
		t.Fatalf("expected AI and #AI to merge related videos, got %d", got)
	}
}

func TestRemovedManualTopicDoesNotEnterAnalysis(t *testing.T) {
	snapshot := BuildRecommendationSnapshot(
		"workspace-test",
		"hub:user:test",
		[]ingest.Event{{ID: "manual-topic:待采集", EventType: "topic_manual", Source: "manual", CollectedAt: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC), Keyword: "待采集"}},
		nil,
		nil,
		time.Date(2026, 8, 16, 9, 1, 0, 0, time.UTC),
	)
	if len(snapshot.TopicRecommendations) != 0 {
		t.Fatalf("removed manual candidate must not enter analysis, got %+v", snapshot.TopicRecommendations)
	}
	var dashboard Dashboard
	ApplyRecommendationSnapshot(&dashboard, snapshot)
	if len(dashboard.HotTopics) != 0 {
		t.Fatalf("manual-only topic must stay out of hot ranking, got %+v", dashboard.HotTopics)
	}
}

func TestBuildRecommendationSnapshotPreservesKnownPublishedAtWhenLatestObservationOmitsIt(t *testing.T) {
	publishedAt := time.Date(2026, 8, 6, 23, 0, 0, 0, time.UTC)
	observedAt := time.Date(2026, 8, 7, 22, 28, 0, 0, time.UTC)
	snapshot := BuildRecommendationSnapshot(
		"workspace-test",
		"hub:user:test",
		[]ingest.Event{{
			ID:          "event-latest",
			EventType:   "video_snapshot",
			Source:      ingest.RunSourceWXVideoSearch,
			CollectedAt: observedAt,
			PublishedAt: publishedAt,
			ObjectID:    "video-date-recovery",
			Title:       "发布日期保留",
		}},
		[]ingest.VideoMetricObservation{{
			ID:         "observation-latest",
			EventID:    "event-latest",
			Source:     ingest.RunSourceWXVideoSearch,
			ObservedAt: observedAt,
			ObjectID:   "video-date-recovery",
			Title:      "发布日期保留",
			LikeCount:  10,
		}},
		nil,
		observedAt.Add(time.Minute),
	)

	if len(snapshot.VideoRecommendations) != 1 {
		t.Fatalf("expected one video recommendation, got %+v", snapshot.VideoRecommendations)
	}
	if got := snapshot.VideoRecommendations[0].PublishedAt; !got.Equal(publishedAt) {
		t.Fatalf("expected event publication time to fill missing observation field, got %v", got)
	}
}

func TestBuildRecommendationSnapshotFallsBackToEarlierPublishedAt(t *testing.T) {
	publishedAt := time.Date(2026, 8, 5, 9, 30, 0, 0, time.UTC)
	snapshot := BuildRecommendationSnapshot(
		"workspace-test",
		"hub:user:test",
		nil,
		[]ingest.VideoMetricObservation{
			{
				ID: "observation-old", EventID: "event-old", Source: ingest.RunSourceWXVideoSearch,
				ObservedAt: publishedAt.Add(time.Hour), PublishedAt: publishedAt, ObjectID: "video-history-date",
				Title: "历史发布日期",
			},
			{
				ID: "observation-new", EventID: "event-new", Source: ingest.RunSourceWXVideoSearch,
				ObservedAt: publishedAt.Add(24 * time.Hour), ObjectID: "video-history-date",
				Title: "历史发布日期", LikeCount: 8,
			},
		},
		nil,
		publishedAt.Add(25*time.Hour),
	)

	if len(snapshot.VideoRecommendations) != 1 {
		t.Fatalf("expected one video recommendation, got %+v", snapshot.VideoRecommendations)
	}
	if got := snapshot.VideoRecommendations[0].PublishedAt; !got.Equal(publishedAt) {
		t.Fatalf("expected earlier known publication time to survive, got %v", got)
	}
}

func TestApplyRecommendationSnapshotLimitsHotRankingsButKeepsAllCandidates(t *testing.T) {
	snapshot := RecommendationSnapshot{
		VideoRecommendations: make([]VideoRecommendation, hotVideoRankingLimit+1),
		TopicRecommendations: make([]TopicRecommendation, hotTopicRankingLimit+1),
	}
	for i := range snapshot.VideoRecommendations {
		snapshot.VideoRecommendations[i] = VideoRecommendation{ID: "video-" + string(rune('a'+i))}
	}
	for i := range snapshot.TopicRecommendations {
		snapshot.TopicRecommendations[i] = TopicRecommendation{ID: "topic-" + string(rune('a'+i)), ObservationCount: 1}
	}

	var dashboard Dashboard
	ApplyRecommendationSnapshot(&dashboard, snapshot)

	if len(dashboard.VideoRecommendations) != hotVideoRankingLimit+1 || len(snapshot.TopicRecommendations) != hotTopicRankingLimit+1 {
		t.Fatalf("all recommendation candidates must remain available, got videos=%d topics=%d", len(dashboard.VideoRecommendations), len(snapshot.TopicRecommendations))
	}
	if len(dashboard.HotVideos) != hotVideoRankingLimit || len(dashboard.HotTopics) != hotTopicRankingLimit {
		t.Fatalf("hot rankings must be limited to videos=%d topics=%d, got videos=%d topics=%d", hotVideoRankingLimit, hotTopicRankingLimit, len(dashboard.HotVideos), len(dashboard.HotTopics))
	}
	if dashboard.HotVideos[0].ID != "video-a" || dashboard.HotTopics[0].ID != "topic-a" {
		t.Fatalf("hot rankings must preserve recommendation order, got videos=%+v topics=%+v", dashboard.HotVideos, dashboard.HotTopics)
	}
}

func TestBuildHotVideoRankingsSortsEachBoardFromCompleteCandidateSet(t *testing.T) {
	observedAt := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	items := []HotVideo{
		{ID: "composite", Likes: 8, Comments: 80, Favorites: 2, Forwards: 1, ObservedAt: observedAt, RecommendationScore: 0.9},
		{ID: "favorites", Likes: 4, Comments: 3, Favorites: 99, Forwards: 2, ObservedAt: observedAt.Add(2 * time.Minute)},
		{ID: "forwards", Likes: 3, Comments: 2, Favorites: 1, Forwards: 88, ObservedAt: observedAt.Add(3 * time.Minute)},
	}
	for i := 0; i < 8; i++ {
		items = append(items, HotVideo{ID: "low-" + string(rune('a'+i)), Likes: i + 1, Comments: i + 1, ObservedAt: observedAt.Add(time.Duration(4+i) * time.Minute)})
	}
	items = append(items, HotVideo{ID: "likes", Likes: 120, Comments: 2, Favorites: 4, Forwards: 3, ObservedAt: observedAt.Add(20 * time.Minute)})

	rankings := BuildHotVideoRankings(items)
	expectedCount := len(items)
	if expectedCount > hotVideoRankingLimit {
		expectedCount = hotVideoRankingLimit
	}
	for _, kind := range HotVideoRankingKinds() {
		if len(rankings[kind]) != expectedCount {
			t.Fatalf("expected %d candidates under %s, got %d", expectedCount, kind, len(rankings[kind]))
		}
	}
	if rankings[HotVideoRankingHot][0].ID != "composite" {
		t.Fatalf("expected composite order to remain the hot default, got %+v", rankings[HotVideoRankingHot])
	}
	if rankings[HotVideoRankingLikes][0].ID != "likes" {
		t.Fatalf("expected likes board to use likes count, got %+v", rankings[HotVideoRankingLikes])
	}
	if rankings[HotVideoRankingComments][0].ID != "composite" {
		t.Fatalf("expected comments board to use comments count, got %+v", rankings[HotVideoRankingComments])
	}
	if rankings[HotVideoRankingFavorites][0].ID != "favorites" {
		t.Fatalf("expected favorites board to use favorites count, got %+v", rankings[HotVideoRankingFavorites])
	}
	if rankings[HotVideoRankingForwards][0].ID != "forwards" {
		t.Fatalf("expected forwards board to use forwards count, got %+v", rankings[HotVideoRankingForwards])
	}
}

func TestBuildHotVideoWindowRankingsUsesPublishedAtBeforeRanking(t *testing.T) {
	referenceAt := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	items := []HotVideo{
		{
			ID:          "old-high-interaction",
			PublishedAt: referenceAt.Add(-48 * time.Hour),
			ObservedAt:  referenceAt.Add(-time.Hour),
			Likes:       1000,
		},
		{
			ID:          "recent-video",
			PublishedAt: referenceAt.Add(-2 * time.Hour),
			ObservedAt:  referenceAt.Add(-time.Minute),
			Likes:       5,
		},
		{
			ID:         "unknown-publication",
			ObservedAt: referenceAt.Add(-time.Minute),
			Likes:      2000,
		},
	}

	rankings := BuildHotVideoWindowRankings(items, referenceAt)
	if got := rankings[HotVideoWindow24h][HotVideoRankingLikes]; len(got) != 1 || got[0].ID != "recent-video" {
		t.Fatalf("expected 24h likes board to use recent publication candidates, got %+v", got)
	}
	if got := rankings[HotVideoWindow24h][HotVideoRankingHot]; len(got) != 1 || got[0].ID != "recent-video" {
		t.Fatalf("expected 24h hot board to exclude old and unknown publication videos, got %+v", got)
	}
	if got := rankings[HotVideoWindow72h][HotVideoRankingLikes]; len(got) != 2 || got[0].ID != "old-high-interaction" {
		t.Fatalf("expected 72h likes board to rank the complete publication-window candidate set, got %+v", got)
	}
}
