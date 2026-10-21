package insight

import (
	"context"
)

type RecommendationRepository interface {
	GetRecommendations(ctx context.Context, workspaceID string) (RecommendationSnapshot, bool, error)
	ReplaceRecommendations(ctx context.Context, workspaceID string, snapshot RecommendationSnapshot) error
	ResetRecommendations(ctx context.Context, workspaceID string) error
}

func cloneRecommendationSnapshot(source RecommendationSnapshot) RecommendationSnapshot {
	cloned := source
	cloned.AccountRecommendations = append([]AccountRecommendation(nil), source.AccountRecommendations...)
	cloned.VideoRecommendations = append([]VideoRecommendation(nil), source.VideoRecommendations...)
	cloned.TopicRecommendations = append([]TopicRecommendation(nil), source.TopicRecommendations...)
	for i := range cloned.AccountRecommendations {
		cloned.AccountRecommendations[i].Sources = append([]string(nil), source.AccountRecommendations[i].Sources...)
	}
	for i := range cloned.VideoRecommendations {
		cloned.VideoRecommendations[i].Tags = append([]string(nil), source.VideoRecommendations[i].Tags...)
	}
	for i := range cloned.TopicRecommendations {
		cloned.TopicRecommendations[i].Sources = append([]string(nil), source.TopicRecommendations[i].Sources...)
		cloned.TopicRecommendations[i].SourceAccounts = append([]string(nil), source.TopicRecommendations[i].SourceAccounts...)
		cloned.TopicRecommendations[i].ReferenceVideos = cloneVideoRecommendations(source.TopicRecommendations[i].ReferenceVideos)
		cloned.TopicRecommendations[i].CommentEvidence = cloneCommentInsights(source.TopicRecommendations[i].CommentEvidence)
	}
	return cloned
}

func cloneVideoRecommendations(items []VideoRecommendation) []VideoRecommendation {
	results := append([]VideoRecommendation(nil), items...)
	for i := range results {
		results[i].Tags = append([]string(nil), items[i].Tags...)
	}
	return results
}

func CloneRecommendationSnapshot(source RecommendationSnapshot) RecommendationSnapshot {
	return cloneRecommendationSnapshot(source)
}
