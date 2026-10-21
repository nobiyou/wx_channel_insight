package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/nobiyou/wx_channel_insight/internal/aggregate"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/normalize"
)

func requireClient(c *Client) error {
	if c == nil || c.db == nil {
		return errors.New("sqlite client is required for sqlite store mode")
	}
	return nil
}

type DashboardRepository struct {
	c    *Client
	seed insight.Dashboard
}

func NewDashboardRepository(c *Client, seed insight.Dashboard) (*DashboardRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &DashboardRepository{c, seed}, nil
}
func (r *DashboardRepository) GetDashboard(ctx context.Context, scope string) (insight.Dashboard, error) {
	v, found, err := get[insight.Dashboard](ctx, r.c.db, "dashboard", scope, "snapshot")
	if !found && err == nil {
		v = r.seed
	}
	return v, err
}
func (r *DashboardRepository) ReplaceDashboard(ctx context.Context, scope string, v insight.Dashboard) error {
	return put(ctx, r.c.db, "dashboard", scope, "snapshot", v)
}
func (r *DashboardRepository) ResetDashboard(ctx context.Context, scope string) error {
	return r.ReplaceDashboard(ctx, scope, r.seed)
}

type artifactMeta struct {
	c      *Client
	bucket string
}

func (r artifactMeta) GetMeta(ctx context.Context, scope string) (insight.ReadModelArtifactMeta, bool, error) {
	return get[insight.ReadModelArtifactMeta](ctx, r.c.db, r.bucket, scope, "meta")
}
func (r artifactMeta) replaceMeta(ctx context.Context, scope string, v insight.ReadModelArtifactMeta) error {
	return put(ctx, r.c.db, r.bucket, scope, "meta", v)
}

type ReportSummaryRepository struct {
	artifactMeta
	seed insight.ReportSummary
}

func NewReportSummaryRepository(c *Client, seed insight.ReportSummary) (*ReportSummaryRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &ReportSummaryRepository{artifactMeta{c, "report_summary"}, seed}, nil
}
func (r *ReportSummaryRepository) GetReportSummary(ctx context.Context, scope string) (insight.ReportSummary, bool, error) {
	v, found, err := get[insight.ReportSummary](ctx, r.c.db, r.bucket, scope, "snapshot")
	if !found && err == nil {
		v = r.seed
	}
	return v, found, err
}
func (r *ReportSummaryRepository) ReplaceReportSummary(ctx context.Context, scope string, v insight.ReportSummary) error {
	return put(ctx, r.c.db, r.bucket, scope, "snapshot", v)
}
func (r *ReportSummaryRepository) ReplaceReportSummaryMeta(ctx context.Context, scope string, v insight.ReadModelArtifactMeta) error {
	return r.replaceMeta(ctx, scope, v)
}
func (r *ReportSummaryRepository) ResetReportSummary(ctx context.Context, scope string) error {
	return r.ReplaceReportSummary(ctx, scope, r.seed)
}

type CommentInsightRepository struct {
	artifactMeta
	seed []insight.CommentInsight
}

func NewCommentInsightRepository(c *Client, seed []insight.CommentInsight) (*CommentInsightRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &CommentInsightRepository{artifactMeta{c, "comment_insight"}, seed}, nil
}
func (r *CommentInsightRepository) GetCommentInsights(ctx context.Context, scope string) ([]insight.CommentInsight, bool, error) {
	v, found, err := get[[]insight.CommentInsight](ctx, r.c.db, r.bucket, scope, "snapshot")
	if !found && err == nil {
		v = insight.CloneCommentInsights(r.seed)
	}
	return v, found, err
}
func (r *CommentInsightRepository) ReplaceCommentInsights(ctx context.Context, scope string, v []insight.CommentInsight) error {
	return put(ctx, r.c.db, r.bucket, scope, "snapshot", v)
}
func (r *CommentInsightRepository) ReplaceCommentInsightMeta(ctx context.Context, scope string, v insight.ReadModelArtifactMeta) error {
	return r.replaceMeta(ctx, scope, v)
}
func (r *CommentInsightRepository) ResetCommentInsights(ctx context.Context, scope string) error {
	return r.ReplaceCommentInsights(ctx, scope, r.seed)
}

type RankingRepository struct{ c *Client }

func NewRankingRepository(c *Client) (*RankingRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &RankingRepository{c}, nil
}
func (r *RankingRepository) GetHotVideos(ctx context.Context, scope string) ([]insight.HotVideo, bool, error) {
	return get[[]insight.HotVideo](ctx, r.c.db, "ranking", scope, "videos")
}
func (r *RankingRepository) ReplaceHotVideos(ctx context.Context, scope string, items []insight.HotVideo) error {
	return r.c.update(ctx, func(tx *sql.Tx) error {
		rankings, _, err := get[insight.HotVideoRankings](ctx, tx, "ranking", scope, "video_rankings")
		if err != nil {
			return err
		}
		if rankings == nil {
			rankings = insight.HotVideoRankings{}
		}
		rankings[insight.HotVideoRankingHot] = items
		if err := put(ctx, tx, "ranking", scope, "video_rankings", rankings); err != nil {
			return err
		}
		return put(ctx, tx, "ranking", scope, "videos", items)
	})
}
func (r *RankingRepository) ResetHotVideos(ctx context.Context, scope string) error {
	return r.ReplaceHotVideoRankings(ctx, scope, insight.HotVideoRankings{insight.HotVideoRankingHot: []insight.HotVideo{}})
}
func (r *RankingRepository) GetHotVideoRankings(ctx context.Context, scope string) (insight.HotVideoRankings, bool, error) {
	return get[insight.HotVideoRankings](ctx, r.c.db, "ranking", scope, "video_rankings")
}
func (r *RankingRepository) ReplaceHotVideoRankings(ctx context.Context, scope string, v insight.HotVideoRankings) error {
	return r.c.update(ctx, func(tx *sql.Tx) error {
		if err := put(ctx, tx, "ranking", scope, "video_rankings", v); err != nil {
			return err
		}
		return put(ctx, tx, "ranking", scope, "videos", v[insight.HotVideoRankingHot])
	})
}
func (r *RankingRepository) ResetHotVideoRankings(ctx context.Context, scope string) error {
	return r.ReplaceHotVideoRankings(ctx, scope, insight.HotVideoRankings{})
}
func (r *RankingRepository) GetHotTopics(ctx context.Context, scope string) ([]insight.HotTopic, bool, error) {
	return get[[]insight.HotTopic](ctx, r.c.db, "ranking", scope, "topics")
}
func (r *RankingRepository) ReplaceHotTopics(ctx context.Context, scope string, v []insight.HotTopic) error {
	return put(ctx, r.c.db, "ranking", scope, "topics", v)
}
func (r *RankingRepository) ResetHotTopics(ctx context.Context, scope string) error {
	return r.ReplaceHotTopics(ctx, scope, []insight.HotTopic{})
}
func (r *RankingRepository) GetHotVideoMeta(ctx context.Context, scope string) (insight.ReadModelArtifactMeta, bool, error) {
	return get[insight.ReadModelArtifactMeta](ctx, r.c.db, "ranking", scope, "video_meta")
}
func (r *RankingRepository) ReplaceHotVideoMeta(ctx context.Context, scope string, v insight.ReadModelArtifactMeta) error {
	return put(ctx, r.c.db, "ranking", scope, "video_meta", v)
}
func (r *RankingRepository) GetHotTopicMeta(ctx context.Context, scope string) (insight.ReadModelArtifactMeta, bool, error) {
	return get[insight.ReadModelArtifactMeta](ctx, r.c.db, "ranking", scope, "topic_meta")
}
func (r *RankingRepository) ReplaceHotTopicMeta(ctx context.Context, scope string, v insight.ReadModelArtifactMeta) error {
	return put(ctx, r.c.db, "ranking", scope, "topic_meta", v)
}

type RecommendationRepository struct{ c *Client }

func NewRecommendationRepository(c *Client) (*RecommendationRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &RecommendationRepository{c}, nil
}
func (r *RecommendationRepository) GetRecommendations(ctx context.Context, scope string) (insight.RecommendationSnapshot, bool, error) {
	return get[insight.RecommendationSnapshot](ctx, r.c.db, "recommendation", scope, "snapshot")
}
func (r *RecommendationRepository) ReplaceRecommendations(ctx context.Context, scope string, v insight.RecommendationSnapshot) error {
	return put(ctx, r.c.db, "recommendation", scope, "snapshot", v)
}
func (r *RecommendationRepository) ResetRecommendations(ctx context.Context, scope string) error {
	_, err := remove(ctx, r.c.db, "recommendation", scope, "snapshot")
	return err
}

type NormalizationRepository struct{ c *Client }

func NewNormalizationRepository(c *Client) (*NormalizationRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &NormalizationRepository{c}, nil
}
func (r *NormalizationRepository) ReplaceSnapshot(ctx context.Context, scope string, v normalize.Snapshot) error {
	return put(ctx, r.c.db, "normalize", scope, "snapshot", v)
}
func (r *NormalizationRepository) GetSnapshot(ctx context.Context, scope string) (normalize.Snapshot, bool, error) {
	return get[normalize.Snapshot](ctx, r.c.db, "normalize", scope, "snapshot")
}
func (r *NormalizationRepository) GetSummary(ctx context.Context, scope string) (normalize.Summary, bool, error) {
	v, found, err := r.GetSnapshot(ctx, scope)
	return v.Summary, found, err
}
func (r *NormalizationRepository) Reset(ctx context.Context, scope string) error {
	return clear(ctx, r.c.db, "normalize", scope)
}

type AggregationRepository struct{ c *Client }

func NewAggregationRepository(c *Client) (*AggregationRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &AggregationRepository{c}, nil
}
func (r *AggregationRepository) ReplaceAggregate(ctx context.Context, scope string, v aggregate.Aggregate) error {
	return put(ctx, r.c.db, "aggregate", scope, "snapshot", v)
}
func (r *AggregationRepository) GetAggregate(ctx context.Context, scope string) (aggregate.Aggregate, bool, error) {
	return get[aggregate.Aggregate](ctx, r.c.db, "aggregate", scope, "snapshot")
}
func (r *AggregationRepository) GetSummary(ctx context.Context, scope string) (aggregate.Summary, bool, error) {
	v, found, err := r.GetAggregate(ctx, scope)
	return v.Summary, found, err
}
func (r *AggregationRepository) Reset(ctx context.Context, scope string) error {
	return clear(ctx, r.c.db, "aggregate", scope)
}

var (
	_ insight.DashboardRepository      = (*DashboardRepository)(nil)
	_ insight.ReportSummaryRepository  = (*ReportSummaryRepository)(nil)
	_ insight.CommentInsightRepository = (*CommentInsightRepository)(nil)
	_ insight.RankingRepository        = (*RankingRepository)(nil)
	_ insight.RecommendationRepository = (*RecommendationRepository)(nil)
	_ normalize.Repository             = (*NormalizationRepository)(nil)
	_ aggregate.Repository             = (*AggregationRepository)(nil)
)
