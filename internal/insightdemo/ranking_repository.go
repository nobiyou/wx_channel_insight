package insightdemo

import (
	"context"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type RankingRepository struct {
	mu            sync.RWMutex
	seedVideos    []insight.HotVideo
	seedTopics    []insight.HotTopic
	videos        map[string][]insight.HotVideo
	videoRankings map[string]insight.HotVideoRankings
	topics        map[string][]insight.HotTopic
	videoMeta     map[string]insight.ReadModelArtifactMeta
	topicMeta     map[string]insight.ReadModelArtifactMeta
}

func NewRankingRepository(seedVideos []insight.HotVideo, seedTopics []insight.HotTopic) *RankingRepository {
	return &RankingRepository{
		seedVideos: cloneHotVideoSeed(seedVideos),
		seedTopics: cloneHotTopicSeed(seedTopics),
		videos: map[string][]insight.HotVideo{
			"": cloneHotVideoSeed(seedVideos),
		},
		videoRankings: map[string]insight.HotVideoRankings{},
		topics: map[string][]insight.HotTopic{
			"": cloneHotTopicSeed(seedTopics),
		},
		videoMeta: map[string]insight.ReadModelArtifactMeta{},
		topicMeta: map[string]insight.ReadModelArtifactMeta{},
	}
}

func (r *RankingRepository) GetHotVideos(_ context.Context, workspaceID string) ([]insight.HotVideo, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, ok := r.videos[workspaceID]
	if ok {
		return cloneHotVideoSeed(items), true, nil
	}
	return cloneHotVideoSeed(r.seedVideos), true, nil
}

func (r *RankingRepository) ReplaceHotVideos(_ context.Context, workspaceID string, items []insight.HotVideo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.videos[workspaceID] = cloneHotVideoSeed(items)
	rankings := insight.CloneHotVideoRankings(r.videoRankings[workspaceID])
	if rankings == nil {
		rankings = insight.HotVideoRankings{}
	}
	rankings[insight.HotVideoRankingHot] = cloneHotVideoSeed(items)
	r.videoRankings[workspaceID] = rankings
	return nil
}

func (r *RankingRepository) ResetHotVideos(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.videos[workspaceID] = cloneHotVideoSeed(r.seedVideos)
	rankings := insight.CloneHotVideoRankings(r.videoRankings[workspaceID])
	if rankings == nil {
		rankings = insight.HotVideoRankings{}
	}
	rankings[insight.HotVideoRankingHot] = cloneHotVideoSeed(r.seedVideos)
	r.videoRankings[workspaceID] = rankings
	return nil
}

func (r *RankingRepository) GetHotVideoRankings(_ context.Context, workspaceID string) (insight.HotVideoRankings, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if rankings, ok := r.videoRankings[workspaceID]; ok {
		return insight.CloneHotVideoRankings(rankings), true, nil
	}
	items, ok := r.videos[workspaceID]
	if !ok {
		items = r.seedVideos
	}
	return insight.HotVideoRankings{
		insight.HotVideoRankingHot: cloneHotVideoSeed(items),
	}, true, nil
}

func (r *RankingRepository) ReplaceHotVideoRankings(_ context.Context, workspaceID string, rankings insight.HotVideoRankings) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.videoRankings[workspaceID] = insight.CloneHotVideoRankings(rankings)
	items := rankings[insight.HotVideoRankingHot]
	r.videos[workspaceID] = cloneHotVideoSeed(items)
	return nil
}

func (r *RankingRepository) ResetHotVideoRankings(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	seed := cloneHotVideoSeed(r.seedVideos)
	r.videoRankings[workspaceID] = insight.HotVideoRankings{
		insight.HotVideoRankingHot: seed,
	}
	r.videos[workspaceID] = cloneHotVideoSeed(seed)
	return nil
}

func (r *RankingRepository) GetHotVideoMeta(_ context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, ok := r.videoMeta[workspaceID]
	return meta, ok, nil
}

func (r *RankingRepository) ReplaceHotVideoMeta(_ context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.videoMeta[workspaceID] = meta
	return nil
}

func (r *RankingRepository) GetHotTopics(_ context.Context, workspaceID string) ([]insight.HotTopic, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, ok := r.topics[workspaceID]
	if ok {
		return cloneHotTopicSeed(items), true, nil
	}
	return cloneHotTopicSeed(r.seedTopics), true, nil
}

func (r *RankingRepository) ReplaceHotTopics(_ context.Context, workspaceID string, items []insight.HotTopic) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.topics[workspaceID] = cloneHotTopicSeed(items)
	return nil
}

func (r *RankingRepository) ResetHotTopics(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.topics[workspaceID] = cloneHotTopicSeed(r.seedTopics)
	return nil
}

func (r *RankingRepository) GetHotTopicMeta(_ context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, ok := r.topicMeta[workspaceID]
	return meta, ok, nil
}

func (r *RankingRepository) ReplaceHotTopicMeta(_ context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.topicMeta[workspaceID] = meta
	return nil
}

func cloneHotVideoSeed(items []insight.HotVideo) []insight.HotVideo {
	results := append([]insight.HotVideo(nil), items...)
	for i := range results {
		results[i].Tags = append([]string(nil), results[i].Tags...)
	}
	return results
}

func cloneHotTopicSeed(items []insight.HotTopic) []insight.HotTopic {
	results := append([]insight.HotTopic(nil), items...)
	for i := range results {
		results[i].Keywords = append([]string(nil), results[i].Keywords...)
	}
	return results
}
