package insightpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type RankingRepository struct {
	client *postgres.Client
	db     *sql.DB
}

type hotVideoSnapshotPayload struct {
	Items    []insight.HotVideo            `json:"items"`
	Rankings insight.HotVideoRankings      `json:"rankings,omitempty"`
	Meta     insight.ReadModelArtifactMeta `json:"meta"`
}

type hotTopicSnapshotPayload struct {
	Items []insight.HotTopic            `json:"items"`
	Meta  insight.ReadModelArtifactMeta `json:"meta"`
}

var _ insight.RankingRepository = (*RankingRepository)(nil)

func NewRankingRepository(client *postgres.Client) (*RankingRepository, error) {
	if client == nil {
		return nil, errors.New("postgres client is required")
	}
	db := client.DB()
	if err := postgres.EnsureRankingSchema(db); err != nil {
		return nil, err
	}
	return &RankingRepository{client: client, db: db}, nil
}

func (r *RankingRepository) GetHotVideos(ctx context.Context, workspaceID string) ([]insight.HotVideo, bool, error) {
	return r.loadHotVideos(ctx, workspaceID)
}

func (r *RankingRepository) ReplaceHotVideos(ctx context.Context, workspaceID string, items []insight.HotVideo) error {
	return r.saveHotVideos(ctx, workspaceID, items)
}

func (r *RankingRepository) ResetHotVideos(ctx context.Context, workspaceID string) error {
	return r.saveHotVideos(ctx, workspaceID, []insight.HotVideo{})
}

func (r *RankingRepository) GetHotVideoRankings(ctx context.Context, workspaceID string) (insight.HotVideoRankings, bool, error) {
	return r.loadHotVideoRankings(ctx, workspaceID)
}

func (r *RankingRepository) ReplaceHotVideoRankings(ctx context.Context, workspaceID string, rankings insight.HotVideoRankings) error {
	return r.saveHotVideoRankings(ctx, workspaceID, rankings)
}

func (r *RankingRepository) ResetHotVideoRankings(ctx context.Context, workspaceID string) error {
	return r.saveHotVideoRankings(ctx, workspaceID, insight.HotVideoRankings{})
}

func (r *RankingRepository) GetHotVideoMeta(ctx context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	meta, found, err := r.loadHotVideoMeta(ctx, workspaceID)
	if err != nil {
		return insight.ReadModelArtifactMeta{}, false, err
	}
	return meta, found, nil
}

func (r *RankingRepository) ReplaceHotVideoMeta(ctx context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	return r.saveHotVideoMeta(ctx, workspaceID, meta)
}

func (r *RankingRepository) GetHotTopics(ctx context.Context, workspaceID string) ([]insight.HotTopic, bool, error) {
	return r.loadHotTopics(ctx, workspaceID)
}

// GetDashboardPageRankings keeps the dashboard transport bounded while the
// dedicated ranking endpoints continue to expose their requested depth.
func (r *RankingRepository) GetDashboardPageRankings(ctx context.Context, workspaceID string) ([]insight.HotVideo, insight.HotVideoRankings, bool, []insight.HotTopic, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	videoRow := r.db.QueryRowContext(ctx, `
SELECT jsonb_build_object(
  'items', jsonb_path_query_array(payload, '$.items[0 to $last]', jsonb_build_object('last', $2::int)),
  'rankings', (
      SELECT COALESCE(
        jsonb_object_agg(
          ranking_key,
          jsonb_path_query_array(ranking_value, '$[0 to $last]', jsonb_build_object('last', $2::int))
        ),
        '{}'::jsonb
      )
      FROM jsonb_each(
        CASE WHEN jsonb_typeof(payload->'rankings') = 'object' THEN payload->'rankings' ELSE '{}'::jsonb END
      ) AS rankings(ranking_key, ranking_value)
  )
), updated_at
FROM insight_hot_video_rankings
WHERE snapshot_id = $1
`, snapshotKey(workspaceID), insight.DashboardPageHotVideoLimit-1)
	var videoPayload []byte
	var videoUpdatedAt time.Time
	videoFound := true
	if err := videoRow.Scan(&videoPayload, &videoUpdatedAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, nil, false, nil, false, err
		}
		videoFound = false
	}
	var videos []insight.HotVideo
	var rankings insight.HotVideoRankings
	if videoFound {
		var err error
		videos, _, err = parseHotVideoSnapshotPayload(videoPayload, videoUpdatedAt)
		if err != nil {
			return nil, nil, true, nil, false, err
		}
		rankings, err = parseHotVideoRankings(videoPayload, videoUpdatedAt)
		if err != nil {
			return nil, nil, true, nil, false, err
		}
	}

	topicRow := r.db.QueryRowContext(ctx, `
SELECT jsonb_build_object(
  'items', (
    SELECT COALESCE(
      jsonb_agg(
        (item - 'sourceAccounts') || jsonb_build_object(
          'sourceAccounts',
          jsonb_path_query_array(item, '$.sourceAccounts[0 to $last]', jsonb_build_object('last', $3::int))
        )
        ORDER BY ordinality
      ),
      '[]'::jsonb
    )
    FROM jsonb_array_elements(
      jsonb_path_query_array(payload, '$.items[0 to $last]', jsonb_build_object('last', $2::int))
    ) WITH ORDINALITY AS topics(item, ordinality)
  )
), updated_at
FROM insight_hot_topic_rankings
WHERE snapshot_id = $1
`, snapshotKey(workspaceID), insight.DashboardPageHotTopicLimit-1, insight.DashboardPageNestedDetailLimit-1)
	var topicPayload []byte
	var topicUpdatedAt time.Time
	topicFound := true
	if err := topicRow.Scan(&topicPayload, &topicUpdatedAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, nil, videoFound, nil, false, err
		}
		topicFound = false
	}
	var topics []insight.HotTopic
	if topicFound {
		var err error
		topics, _, err = parseHotTopicSnapshotPayload(topicPayload, topicUpdatedAt)
		if err != nil {
			return nil, nil, videoFound, nil, true, err
		}
	}
	return insight.CloneHotVideos(videos), insight.CloneHotVideoRankings(rankings), videoFound, insight.CloneHotTopics(topics), topicFound, nil
}

func (r *RankingRepository) ReplaceHotTopics(ctx context.Context, workspaceID string, items []insight.HotTopic) error {
	return r.saveHotTopics(ctx, workspaceID, items)
}

func (r *RankingRepository) ResetHotTopics(ctx context.Context, workspaceID string) error {
	return r.saveHotTopics(ctx, workspaceID, []insight.HotTopic{})
}

func (r *RankingRepository) GetHotTopicMeta(ctx context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	meta, found, err := r.loadHotTopicMeta(ctx, workspaceID)
	if err != nil {
		return insight.ReadModelArtifactMeta{}, false, err
	}
	return meta, found, nil
}

func (r *RankingRepository) ReplaceHotTopicMeta(ctx context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	return r.saveHotTopicMeta(ctx, workspaceID, meta)
}

func (r *RankingRepository) loadHotVideos(ctx context.Context, workspaceID string) ([]insight.HotVideo, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload, updated_at FROM insight_hot_video_rankings WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	var updatedAt time.Time
	if err := row.Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	items, _, err := parseHotVideoSnapshotPayload(payload, updatedAt)
	if err != nil {
		return nil, true, err
	}
	return insight.CloneHotVideos(items), true, nil
}

func (r *RankingRepository) saveHotVideos(ctx context.Context, workspaceID string, items []insight.HotVideo) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if items == nil {
		items = []insight.HotVideo{}
	}
	rankings, _, err := r.loadHotVideoRankings(ctx, workspaceID)
	if err != nil {
		return err
	}
	if rankings == nil {
		rankings = insight.HotVideoRankings{}
	}
	rankings[insight.HotVideoRankingHot] = insight.CloneHotVideos(items)
	meta, _, err := r.loadHotVideoMeta(ctx, workspaceID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(hotVideoSnapshotPayload{
		Items:    items,
		Rankings: rankings,
		Meta:     meta,
	})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_hot_video_rankings (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = now()
`, snapshotKey(workspaceID), string(payload))
	return err
}

func (r *RankingRepository) loadHotVideoRankings(ctx context.Context, workspaceID string) (insight.HotVideoRankings, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload, updated_at FROM insight_hot_video_rankings WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	var updatedAt time.Time
	if err := row.Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	rankings, err := parseHotVideoRankings(payload, updatedAt)
	if err != nil {
		return nil, true, err
	}
	return insight.CloneHotVideoRankings(rankings), true, nil
}

func (r *RankingRepository) saveHotVideoRankings(ctx context.Context, workspaceID string, rankings insight.HotVideoRankings) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if rankings == nil {
		rankings = insight.HotVideoRankings{}
	}
	meta, _, err := r.loadHotVideoMeta(ctx, workspaceID)
	if err != nil {
		return err
	}
	items := insight.CloneHotVideos(rankings[insight.HotVideoRankingHot])
	payload, err := json.Marshal(hotVideoSnapshotPayload{
		Items:    items,
		Rankings: insight.CloneHotVideoRankings(rankings),
		Meta:     meta,
	})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_hot_video_rankings (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = now()
`, snapshotKey(workspaceID), string(payload))
	return err
}

func (r *RankingRepository) loadHotTopics(ctx context.Context, workspaceID string) ([]insight.HotTopic, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload, updated_at FROM insight_hot_topic_rankings WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	var updatedAt time.Time
	if err := row.Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	items, _, err := parseHotTopicSnapshotPayload(payload, updatedAt)
	if err != nil {
		return nil, true, err
	}
	return insight.CloneHotTopics(items), true, nil
}

func (r *RankingRepository) saveHotTopics(ctx context.Context, workspaceID string, items []insight.HotTopic) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if items == nil {
		items = []insight.HotTopic{}
	}
	meta, _, err := r.loadHotTopicMeta(ctx, workspaceID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(hotTopicSnapshotPayload{
		Items: items,
		Meta:  meta,
	})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_hot_topic_rankings (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = now()
`, snapshotKey(workspaceID), string(payload))
	return err
}

func (r *RankingRepository) loadHotVideoMeta(ctx context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload->'meta', updated_at FROM insight_hot_video_rankings WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	var updatedAt time.Time
	if err := row.Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return insight.ReadModelArtifactMeta{}, false, nil
		}
		return insight.ReadModelArtifactMeta{}, false, err
	}
	meta, err := parseReadModelArtifactMeta(payload, updatedAt)
	if err != nil {
		return insight.ReadModelArtifactMeta{}, true, err
	}
	return meta, true, nil
}

func (r *RankingRepository) saveHotVideoMeta(ctx context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	items, found, err := r.loadHotVideos(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !found {
		items = []insight.HotVideo{}
	}
	rankings, _, err := r.loadHotVideoRankings(ctx, workspaceID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(hotVideoSnapshotPayload{
		Items:    items,
		Rankings: rankings,
		Meta:     meta,
	})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_hot_video_rankings (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = now()
`, snapshotKey(workspaceID), string(payload))
	return err
}

func (r *RankingRepository) loadHotTopicMeta(ctx context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload->'meta', updated_at FROM insight_hot_topic_rankings WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	var updatedAt time.Time
	if err := row.Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return insight.ReadModelArtifactMeta{}, false, nil
		}
		return insight.ReadModelArtifactMeta{}, false, err
	}
	meta, err := parseReadModelArtifactMeta(payload, updatedAt)
	if err != nil {
		return insight.ReadModelArtifactMeta{}, true, err
	}
	return meta, true, nil
}

func (r *RankingRepository) saveHotTopicMeta(ctx context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	items, found, err := r.loadHotTopics(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !found {
		items = []insight.HotTopic{}
	}
	payload, err := json.Marshal(hotTopicSnapshotPayload{
		Items: items,
		Meta:  meta,
	})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_hot_topic_rankings (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = now()
`, snapshotKey(workspaceID), string(payload))
	return err
}

func parseHotVideoSnapshotPayload(payload []byte, updatedAt time.Time) ([]insight.HotVideo, insight.ReadModelArtifactMeta, error) {
	var wrapped hotVideoSnapshotPayload
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return nil, insight.ReadModelArtifactMeta{}, err
	}
	if wrapped.Items == nil {
		wrapped.Items = []insight.HotVideo{}
	}
	if wrapped.Meta.MaterializedAt.IsZero() {
		wrapped.Meta.MaterializedAt = updatedAt
	}
	return wrapped.Items, wrapped.Meta, nil
}

func parseHotVideoRankings(payload []byte, updatedAt time.Time) (insight.HotVideoRankings, error) {
	var wrapped hotVideoSnapshotPayload
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return nil, err
	}
	if wrapped.Rankings == nil {
		wrapped.Rankings = insight.HotVideoRankings{}
	}
	return wrapped.Rankings, nil
}

func parseHotTopicSnapshotPayload(payload []byte, updatedAt time.Time) ([]insight.HotTopic, insight.ReadModelArtifactMeta, error) {
	var wrapped hotTopicSnapshotPayload
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return nil, insight.ReadModelArtifactMeta{}, err
	}
	if wrapped.Items == nil {
		wrapped.Items = []insight.HotTopic{}
	}
	if wrapped.Meta.MaterializedAt.IsZero() {
		wrapped.Meta.MaterializedAt = updatedAt
	}
	return wrapped.Items, wrapped.Meta, nil
}

func parseReadModelArtifactMeta(payload []byte, updatedAt time.Time) (insight.ReadModelArtifactMeta, error) {
	var meta insight.ReadModelArtifactMeta
	if len(payload) > 0 && string(payload) != "null" {
		if err := json.Unmarshal(payload, &meta); err != nil {
			return insight.ReadModelArtifactMeta{}, err
		}
	}
	if meta.MaterializedAt.IsZero() {
		meta.MaterializedAt = updatedAt
	}
	return meta, nil
}
