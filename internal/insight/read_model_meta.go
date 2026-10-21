package insight

import "context"

func collectReadModelArtifactMeta(
	ctx context.Context,
	workspaceID string,
	rankingRepo RankingRepository,
	commentInsightRepo CommentInsightRepository,
	reportSummaryRepo ReportSummaryRepository,
) (map[string]ReadModelArtifactMeta, error) {
	result := map[string]ReadModelArtifactMeta{}
	if rankingRepo != nil {
		if meta, found, err := rankingRepo.GetHotVideoMeta(ctx, workspaceID); err != nil {
			return nil, err
		} else if found {
			result["videos"] = meta
		}
		if meta, found, err := rankingRepo.GetHotTopicMeta(ctx, workspaceID); err != nil {
			return nil, err
		} else if found {
			result["topics"] = meta
		}
	}
	if commentInsightRepo != nil {
		if meta, found, err := commentInsightRepo.GetMeta(ctx, workspaceID); err != nil {
			return nil, err
		} else if found {
			result["comments"] = meta
		}
	}
	if reportSummaryRepo != nil {
		if meta, found, err := reportSummaryRepo.GetMeta(ctx, workspaceID); err != nil {
			return nil, err
		} else if found {
			result["reports"] = meta
		}
	}
	return result, nil
}

func persistReadModelArtifactMeta(
	ctx context.Context,
	workspaceID string,
	readModel ReadModelState,
	rankingRepo RankingRepository,
	commentInsightRepo CommentInsightRepository,
	reportSummaryRepo ReportSummaryRepository,
) error {
	meta := ReadModelArtifactMeta{
		LatestBatchID:  readModel.LatestBatchID,
		MaterializedAt: readModel.MaterializedAt,
	}
	if rankingRepo != nil {
		if err := rankingRepo.ReplaceHotVideoMeta(ctx, workspaceID, meta); err != nil {
			return err
		}
		if err := rankingRepo.ReplaceHotTopicMeta(ctx, workspaceID, meta); err != nil {
			return err
		}
	}
	if commentInsightRepo != nil {
		if err := commentInsightRepo.ReplaceCommentInsightMeta(ctx, workspaceID, meta); err != nil {
			return err
		}
	}
	if reportSummaryRepo != nil {
		if err := reportSummaryRepo.ReplaceReportSummaryMeta(ctx, workspaceID, meta); err != nil {
			return err
		}
	}
	return nil
}

func resetReadModelArtifactMeta(
	ctx context.Context,
	workspaceID string,
	rankingRepo RankingRepository,
	commentInsightRepo CommentInsightRepository,
	reportSummaryRepo ReportSummaryRepository,
) error {
	meta := ReadModelArtifactMeta{}
	if rankingRepo != nil {
		if err := rankingRepo.ReplaceHotVideoMeta(ctx, workspaceID, meta); err != nil {
			return err
		}
		if err := rankingRepo.ReplaceHotTopicMeta(ctx, workspaceID, meta); err != nil {
			return err
		}
	}
	if commentInsightRepo != nil {
		if err := commentInsightRepo.ReplaceCommentInsightMeta(ctx, workspaceID, meta); err != nil {
			return err
		}
	}
	if reportSummaryRepo != nil {
		if err := reportSummaryRepo.ReplaceReportSummaryMeta(ctx, workspaceID, meta); err != nil {
			return err
		}
	}
	return nil
}
