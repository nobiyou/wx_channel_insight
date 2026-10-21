package insightai

import "github.com/nobiyou/wx_channel_insight/internal/prompt"

func stringArraySchema() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
}

func moduleObjectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             required,
	}
}

var moduleResultSchemas = map[string]map[string]any{
	prompt.KeyContentAnalysis: moduleObjectSchema(map[string]any{
		"summary":         map[string]any{"type": "string"},
		"strengths":       stringArraySchema(),
		"weaknesses":      stringArraySchema(),
		"audience":        map[string]any{"type": "string"},
		"recommendations": stringArraySchema(),
		"risks":           stringArraySchema(),
		"evidenceRefs":    stringArraySchema(),
	}, []string{"summary", "strengths", "weaknesses", "audience", "recommendations", "risks", "evidenceRefs"}),
	prompt.KeyTopicTrends: moduleObjectSchema(map[string]any{
		"summary":       map[string]any{"type": "string"},
		"trendStage":    map[string]any{"type": "string"},
		"drivers":       stringArraySchema(),
		"opportunities": stringArraySchema(),
		"caveats":       stringArraySchema(),
		"evidenceRefs":  stringArraySchema(),
	}, []string{"summary", "trendStage", "drivers", "opportunities", "caveats", "evidenceRefs"}),
	prompt.KeyAccountPerformance: moduleObjectSchema(map[string]any{
		"summary":          map[string]any{"type": "string"},
		"positioning":      map[string]any{"type": "string"},
		"strengths":        stringArraySchema(),
		"opportunities":    stringArraySchema(),
		"monitoringAdvice": stringArraySchema(),
		"limitations":      stringArraySchema(),
		"evidenceRefs":     stringArraySchema(),
	}, []string{"summary", "positioning", "strengths", "opportunities", "monitoringAdvice", "limitations", "evidenceRefs"}),
	prompt.KeyCommentInsights: moduleObjectSchema(map[string]any{
		"summary":        map[string]any{"type": "string"},
		"themes":         stringArraySchema(),
		"needs":          stringArraySchema(),
		"sentiment":      map[string]any{"type": "string"},
		"opportunities":  stringArraySchema(),
		"sampleBoundary": map[string]any{"type": "string"},
		"evidenceRefs":   stringArraySchema(),
	}, []string{"summary", "themes", "needs", "sentiment", "opportunities", "sampleBoundary", "evidenceRefs"}),
}

var moduleSchemaNames = map[string]string{
	prompt.KeyContentAnalysis:    "content_analysis",
	prompt.KeyTopicTrends:        "topic_trends",
	prompt.KeyAccountPerformance: "account_performance",
	prompt.KeyCommentInsights:    "comment_insights",
}

var moduleRequiredVariables = map[string][]string{
	prompt.KeyContentAnalysis:    {"data_json", "video_json"},
	prompt.KeyTopicTrends:        {"data_json"},
	prompt.KeyAccountPerformance: {"data_json", "account_json"},
	prompt.KeyCommentInsights:    {"data_json", "comments_json"},
}

var modulePromptVersions = map[string]string{
	prompt.KeyContentAnalysis:    "openai-content-analysis-2026-09-01",
	prompt.KeyTopicTrends:        "openai-topic-trends-2026-09-01",
	prompt.KeyAccountPerformance: "openai-account-performance-2026-09-01",
	prompt.KeyCommentInsights:    "openai-comment-insights-2026-09-01",
}
