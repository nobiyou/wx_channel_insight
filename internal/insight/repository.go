package insight

import (
	"context"
	"time"
)

type DashboardRepository interface {
	GetDashboard(ctx context.Context, workspaceID string) (Dashboard, error)
	ReplaceDashboard(ctx context.Context, workspaceID string, snapshot Dashboard) error
	ResetDashboard(ctx context.Context, workspaceID string) error
}

type ReportSummaryRepository interface {
	GetReportSummary(ctx context.Context, workspaceID string) (ReportSummary, bool, error)
	GetMeta(ctx context.Context, workspaceID string) (ReadModelArtifactMeta, bool, error)
	ReplaceReportSummary(ctx context.Context, workspaceID string, summary ReportSummary) error
	ReplaceReportSummaryMeta(ctx context.Context, workspaceID string, meta ReadModelArtifactMeta) error
	ResetReportSummary(ctx context.Context, workspaceID string) error
}

type DailyReportRepository interface {
	List(ctx context.Context, workspaceID, dataWindow string, limit int) ([]DailyReport, error)
	Get(ctx context.Context, workspaceID, reportID string) (DailyReport, bool, error)
	Upsert(ctx context.Context, report DailyReport) error
}

type RankingRepository interface {
	GetHotVideos(ctx context.Context, workspaceID string) ([]HotVideo, bool, error)
	ReplaceHotVideos(ctx context.Context, workspaceID string, items []HotVideo) error
	ResetHotVideos(ctx context.Context, workspaceID string) error
	GetHotVideoRankings(ctx context.Context, workspaceID string) (HotVideoRankings, bool, error)
	ReplaceHotVideoRankings(ctx context.Context, workspaceID string, rankings HotVideoRankings) error
	ResetHotVideoRankings(ctx context.Context, workspaceID string) error
	GetHotTopics(ctx context.Context, workspaceID string) ([]HotTopic, bool, error)
	ReplaceHotTopics(ctx context.Context, workspaceID string, items []HotTopic) error
	ResetHotTopics(ctx context.Context, workspaceID string) error
	GetHotVideoMeta(ctx context.Context, workspaceID string) (ReadModelArtifactMeta, bool, error)
	ReplaceHotVideoMeta(ctx context.Context, workspaceID string, meta ReadModelArtifactMeta) error
	GetHotTopicMeta(ctx context.Context, workspaceID string) (ReadModelArtifactMeta, bool, error)
	ReplaceHotTopicMeta(ctx context.Context, workspaceID string, meta ReadModelArtifactMeta) error
}

type CommentInsightRepository interface {
	GetCommentInsights(ctx context.Context, workspaceID string) ([]CommentInsight, bool, error)
	GetMeta(ctx context.Context, workspaceID string) (ReadModelArtifactMeta, bool, error)
	ReplaceCommentInsights(ctx context.Context, workspaceID string, items []CommentInsight) error
	ReplaceCommentInsightMeta(ctx context.Context, workspaceID string, meta ReadModelArtifactMeta) error
	ResetCommentInsights(ctx context.Context, workspaceID string) error
}

// DataCenterExclusionRepository stores recoverable workspace-scoped tombstones.
// Source events and metric observations remain append-only; the exclusion is
// applied by Insight read-model queries and can be removed by Restore.
type DataCenterExclusionRepository interface {
	List(ctx context.Context, workspaceID string) ([]DataCenterExclusion, error)
	Upsert(ctx context.Context, exclusion DataCenterExclusion) (DataCenterExclusion, error)
	Restore(ctx context.Context, workspaceID string, entity DataCenterEntity, entityID string) (bool, error)
}

type DataCenterExclusion struct {
	WorkspaceID string           `json:"workspaceId"`
	Entity      DataCenterEntity `json:"entity"`
	EntityID    string           `json:"entityId"`
	DeletedAt   time.Time        `json:"deletedAt"`
	DeletedBy   string           `json:"deletedBy"`
}
