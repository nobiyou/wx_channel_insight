package insightfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type persistedRankings struct {
	HotVideos     map[string][]insight.HotVideo            `json:"hotVideos"`
	HotVideoRanks map[string]insight.HotVideoRankings      `json:"hotVideoRankings"`
	HotTopics     map[string][]insight.HotTopic            `json:"hotTopics"`
	HotVideoMeta  map[string]insight.ReadModelArtifactMeta `json:"hotVideoMeta"`
	HotTopicMeta  map[string]insight.ReadModelArtifactMeta `json:"hotTopicMeta"`
}

type RankingRepository struct {
	mu               sync.RWMutex
	path             string
	hotVideos        map[string][]insight.HotVideo
	hotVideoRankings map[string]insight.HotVideoRankings
	hotTopics        map[string][]insight.HotTopic
	videoMeta        map[string]insight.ReadModelArtifactMeta
	topicMeta        map[string]insight.ReadModelArtifactMeta
}

func NewRankingRepository(path string) (*RankingRepository, error) {
	if path == "" {
		return nil, errors.New("ranking repository path is required")
	}

	repo := &RankingRepository{
		path:             path,
		hotVideos:        map[string][]insight.HotVideo{},
		hotVideoRankings: map[string]insight.HotVideoRankings{},
		hotTopics:        map[string][]insight.HotTopic{},
		videoMeta:        map[string]insight.ReadModelArtifactMeta{},
		topicMeta:        map[string]insight.ReadModelArtifactMeta{},
	}
	if err := repo.load(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *RankingRepository) GetHotVideos(_ context.Context, workspaceID string) ([]insight.HotVideo, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return nil, false, err
	}
	items, ok := r.hotVideos[workspaceID]
	if ok {
		return insight.CloneHotVideos(items), true, nil
	}
	return nil, false, nil
}

func (r *RankingRepository) ReplaceHotVideos(_ context.Context, workspaceID string, items []insight.HotVideo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.hotVideos[workspaceID] = insight.CloneHotVideos(items)
	rankings := insight.CloneHotVideoRankings(r.hotVideoRankings[workspaceID])
	if rankings == nil {
		rankings = insight.HotVideoRankings{}
	}
	rankings[insight.HotVideoRankingHot] = insight.CloneHotVideos(items)
	r.hotVideoRankings[workspaceID] = rankings
	return r.persist()
}

func (r *RankingRepository) ResetHotVideos(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.hotVideos[workspaceID] = []insight.HotVideo{}
	r.hotVideoRankings[workspaceID] = insight.HotVideoRankings{
		insight.HotVideoRankingHot: []insight.HotVideo{},
	}
	return r.persist()
}

func (r *RankingRepository) GetHotVideoRankings(_ context.Context, workspaceID string) (insight.HotVideoRankings, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return nil, false, err
	}
	if rankings, ok := r.hotVideoRankings[workspaceID]; ok {
		return insight.CloneHotVideoRankings(rankings), true, nil
	}
	items, ok := r.hotVideos[workspaceID]
	if !ok {
		return nil, false, nil
	}
	return insight.HotVideoRankings{
		insight.HotVideoRankingHot: insight.CloneHotVideos(items),
	}, true, nil
}

func (r *RankingRepository) ReplaceHotVideoRankings(_ context.Context, workspaceID string, rankings insight.HotVideoRankings) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.hotVideoRankings[workspaceID] = insight.CloneHotVideoRankings(rankings)
	r.hotVideos[workspaceID] = insight.CloneHotVideos(rankings[insight.HotVideoRankingHot])
	return r.persist()
}

func (r *RankingRepository) ResetHotVideoRankings(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.hotVideoRankings[workspaceID] = insight.HotVideoRankings{}
	r.hotVideos[workspaceID] = []insight.HotVideo{}
	return r.persist()
}

func (r *RankingRepository) GetHotVideoMeta(_ context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return insight.ReadModelArtifactMeta{}, false, err
	}
	meta, ok := r.videoMeta[workspaceID]
	return meta, ok, nil
}

func (r *RankingRepository) ReplaceHotVideoMeta(_ context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.videoMeta[workspaceID] = meta
	return r.persist()
}

func (r *RankingRepository) GetHotTopics(_ context.Context, workspaceID string) ([]insight.HotTopic, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return nil, false, err
	}
	items, ok := r.hotTopics[workspaceID]
	if ok {
		return insight.CloneHotTopics(items), true, nil
	}
	return nil, false, nil
}

func (r *RankingRepository) ReplaceHotTopics(_ context.Context, workspaceID string, items []insight.HotTopic) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.hotTopics[workspaceID] = insight.CloneHotTopics(items)
	return r.persist()
}

func (r *RankingRepository) ResetHotTopics(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.hotTopics[workspaceID] = []insight.HotTopic{}
	return r.persist()
}

func (r *RankingRepository) GetHotTopicMeta(_ context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return insight.ReadModelArtifactMeta{}, false, err
	}
	meta, ok := r.topicMeta[workspaceID]
	return meta, ok, nil
}

func (r *RankingRepository) ReplaceHotTopicMeta(_ context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.topicMeta[workspaceID] = meta
	return r.persist()
}

func (r *RankingRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *RankingRepository) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r.persist()
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return r.persist()
	}

	var payload persistedRankings
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if len(payload.HotVideos) > 0 {
		r.hotVideos = payload.HotVideos
	} else {
		r.hotVideos = map[string][]insight.HotVideo{}
	}
	if payload.HotVideoRanks != nil {
		r.hotVideoRankings = payload.HotVideoRanks
	} else if r.hotVideoRankings == nil {
		r.hotVideoRankings = map[string]insight.HotVideoRankings{}
	}
	if len(payload.HotTopics) > 0 {
		r.hotTopics = payload.HotTopics
	} else {
		r.hotTopics = map[string][]insight.HotTopic{}
	}
	if payload.HotVideoMeta != nil {
		r.videoMeta = payload.HotVideoMeta
	} else if r.videoMeta == nil {
		r.videoMeta = map[string]insight.ReadModelArtifactMeta{}
	}
	if payload.HotTopicMeta != nil {
		r.topicMeta = payload.HotTopicMeta
	} else if r.topicMeta == nil {
		r.topicMeta = map[string]insight.ReadModelArtifactMeta{}
	}
	return nil
}

func (r *RankingRepository) persist() error {
	payload := persistedRankings{
		HotVideos:     r.hotVideos,
		HotVideoRanks: r.hotVideoRankings,
		HotTopics:     r.hotTopics,
		HotVideoMeta:  r.videoMeta,
		HotTopicMeta:  r.topicMeta,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}
