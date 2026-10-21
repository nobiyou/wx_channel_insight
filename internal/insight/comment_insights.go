package insight

func MergeCommentInsights(primary, fallback []CommentInsight, limit int) []CommentInsight {
	results := make([]CommentInsight, 0, maxInt(len(primary), len(fallback)))
	seen := map[string]struct{}{}
	for _, item := range primary {
		appendUniqueCommentInsight(&results, seen, item)
		if limit > 0 && len(results) >= limit {
			return results
		}
	}
	for _, item := range fallback {
		appendUniqueCommentInsight(&results, seen, item)
		if limit > 0 && len(results) >= limit {
			return results
		}
	}
	return results
}

func cloneCommentInsights(items []CommentInsight) []CommentInsight {
	results := append([]CommentInsight(nil), items...)
	for i := range results {
		results[i].RelatedKeywords = append([]string(nil), results[i].RelatedKeywords...)
		results[i].SourceAccounts = append([]string(nil), results[i].SourceAccounts...)
	}
	return results
}

func CloneCommentInsights(items []CommentInsight) []CommentInsight {
	return cloneCommentInsights(items)
}
