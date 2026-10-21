package aggregate

func cloneAggregate(source Aggregate) Aggregate {
	cloned := source
	cloned.VideoMetrics = append([]VideoMetric(nil), source.VideoMetrics...)
	cloned.TopicMetrics = append([]TopicMetric(nil), source.TopicMetrics...)
	cloned.CommentInsightFacts = append([]CommentInsightFact(nil), source.CommentInsightFacts...)

	for i := range cloned.VideoMetrics {
		cloned.VideoMetrics[i].Tags = append([]string(nil), cloned.VideoMetrics[i].Tags...)
		cloned.VideoMetrics[i].Keywords = append([]string(nil), cloned.VideoMetrics[i].Keywords...)
	}
	for i := range cloned.TopicMetrics {
		cloned.TopicMetrics[i].Keywords = append([]string(nil), cloned.TopicMetrics[i].Keywords...)
	}
	for i := range cloned.CommentInsightFacts {
		cloned.CommentInsightFacts[i].RelatedKeywords = append([]string(nil), cloned.CommentInsightFacts[i].RelatedKeywords...)
	}

	return cloned
}
