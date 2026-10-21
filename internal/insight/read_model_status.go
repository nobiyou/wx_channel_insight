package insight

import (
	"strings"
	"time"
)

func BuildReadModelViewStatuses(payload Dashboard) []ReadModelViewStatus {
	return BuildReadModelViewStatusesWithMeta(payload, nil)
}

func BuildReadModelViewStatusesWithMeta(payload Dashboard, metaByKey map[string]ReadModelArtifactMeta) []ReadModelViewStatus {
	readModel := payload.ReadModel
	summary := payload.IngestSummary
	sourceStatuses := summary.SourceStatuses

	overallState := "lagging"
	if readModel.Mode == "materialized" &&
		readModel.LatestBatchID != "" &&
		readModel.LatestBatchID == summary.LatestBatchID {
		if strings.TrimSpace(summary.FreshnessState) != "" {
			overallState = summary.FreshnessState
		} else {
			overallState = "fresh"
		}
	}

	return []ReadModelViewStatus{
		buildReadModelViewStatus(
			"videos",
			"Hot Videos",
			overallState,
			resolveReadModelArtifactMeta(metaByKey, "videos", readModel),
			"当前热视频榜已经跟到最新物化批次，可直接支撑今天的样本判断。",
			"热视频榜还没有完全对齐到最新批次，更适合先看趋势，不适合直接定榜。",
			sourceStatuses,
			"wx_feed_list",
			"wx_video_search",
			"channel_feed_pull",
		),
		buildReadModelViewStatus(
			"topics",
			"Hot Topics",
			overallState,
			resolveReadModelArtifactMeta(metaByKey, "topics", readModel),
			"热门话题已经追平最新批次，当前适合直接拿来判断今天该跟什么。",
			"热门话题仍可能混有旧样本，先结合来源 freshness 再做选题判断。",
			sourceStatuses,
			"wx_feed_list",
			"wx_video_search",
			"insight_topic_cluster",
		),
		buildReadModelViewStatus(
			"comments",
			"Comment Insights",
			preferSourceFreshness(sourceStatuses, "wx_comment_export", overallState),
			resolveReadModelArtifactMeta(metaByKey, "comments", readModel),
			"评论洞察已被最新评论批次刷新，适合直接拿来写回答型内容。",
			"评论洞察没有完全跟上最新评论来源，优先把它当作方向线索而不是结论。",
			sourceStatuses,
			"wx_comment_export",
			"feed_comment_list",
		),
		buildReadModelViewStatus(
			"reports",
			"Reports",
			overallState,
			resolveReadModelArtifactMeta(metaByKey, "reports", readModel),
			"日报摘要和建议动作已跟到最新物化批次，当前可以直接承接团队日报。",
			"日报摘要还没完全对齐最新物化结果，发布前需要先复核最新批次是否进来。",
			sourceStatuses,
			"wx_comment_export",
			"wx_feed_list",
			"wx_video_search",
			"insight_topic_cluster",
		),
	}
}

func buildReadModelViewStatus(
	key string,
	label string,
	freshnessState string,
	meta ReadModelArtifactMeta,
	freshSummary string,
	laggingSummary string,
	sourceStatuses []IngestSourceStatus,
	preferredSources ...string,
) ReadModelViewStatus {
	summary := laggingSummary
	if freshnessState == "fresh" {
		summary = freshSummary
	}

	return ReadModelViewStatus{
		Key:            key,
		Label:          label,
		FreshnessState: freshnessState,
		Summary:        summary,
		SourceNote:     buildSourceNote(sourceStatuses, preferredSources...),
		LatestBatchID:  meta.LatestBatchID,
		MaterializedAt: meta.MaterializedAt,
	}
}

func resolveReadModelArtifactMeta(metaByKey map[string]ReadModelArtifactMeta, key string, readModel ReadModelState) ReadModelArtifactMeta {
	if meta, ok := metaByKey[key]; ok {
		if strings.TrimSpace(meta.LatestBatchID) != "" || !meta.MaterializedAt.IsZero() {
			return meta
		}
	}
	return ReadModelArtifactMeta{
		LatestBatchID:  readModel.LatestBatchID,
		MaterializedAt: readModel.MaterializedAt,
	}
}

func preferSourceFreshness(sourceStatuses []IngestSourceStatus, source string, fallback string) string {
	for _, item := range sourceStatuses {
		if item.Source != source {
			continue
		}
		if strings.TrimSpace(item.FreshnessState) == "" {
			return fallback
		}
		return item.FreshnessState
	}
	return fallback
}

func buildSourceNote(sourceStatuses []IngestSourceStatus, preferredSources ...string) string {
	if len(preferredSources) == 0 {
		return "当前还没有来源级 freshness 细分，只能先沿用整体 ingest 状态。"
	}

	parts := make([]string, 0, len(preferredSources))
	for _, preferred := range preferredSources {
		for _, item := range sourceStatuses {
			if item.Source != preferred {
				continue
			}
			state := strings.TrimSpace(item.FreshnessState)
			if state == "" {
				state = "unknown"
			}
			parts = append(parts, item.Source+" "+state)
			break
		}
	}
	if len(parts) == 0 {
		return "当前还没有来源级 freshness 细分，只能先沿用整体 ingest 状态。"
	}
	return strings.Join(parts, " · ")
}

func zeroReadModelArtifactMeta() ReadModelArtifactMeta {
	return ReadModelArtifactMeta{
		LatestBatchID:  "",
		MaterializedAt: time.Time{},
	}
}
