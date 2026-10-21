package insightdemo

import (
	"context"
	"sync"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type DashboardRepository struct {
	mu        sync.RWMutex
	seed      insight.Dashboard
	snapshots map[string]insight.Dashboard
}

func NewDashboardRepository() *DashboardRepository {
	seed := buildSeedDashboard()
	return &DashboardRepository{
		seed: seed,
		snapshots: map[string]insight.Dashboard{
			"": seed,
		},
	}
}

func (r *DashboardRepository) GetDashboard(_ context.Context, workspaceID string) (insight.Dashboard, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot, ok := r.snapshots[workspaceID]
	if ok {
		return snapshot, nil
	}
	return r.seed, nil
}

func buildSeedDashboard() insight.Dashboard {
	now := time.Now()
	hotVideoAt := []time.Time{
		now.Add(-2 * time.Hour),
		now.Add(-5 * time.Hour),
		now.Add(-7 * time.Hour),
	}
	hotTopicAt := []time.Time{
		now.Add(-75 * time.Minute),
		now.Add(-105 * time.Minute),
		now.Add(-11 * time.Hour),
	}
	alertAt := []time.Time{
		now.Add(-132 * time.Minute),
		now.Add(-215 * time.Minute),
		now.Add(-14 * time.Hour),
	}
	commentAt := []time.Time{
		now.Add(-70 * time.Minute),
		now.Add(-160 * time.Minute),
		now.Add(-9 * time.Hour),
	}
	return insight.Dashboard{
		GeneratedAt: now,
		Workspace: insight.WorkspaceSummary{
			Name:        "默认运营工作区",
			DeviceCount: 6,
		},
		Overview: insight.OverviewMetrics{
			MonitoredAccounts: 18,
			TrackedVideos:     326,
			EmergingTopics:    12,
			NewCommentBursts:  9,
			DailyCoverageRate: 93.4,
			ReportFreshness:   "刚刚",
		},
		HotVideos: []insight.HotVideo{
			{
				ID:              "vid-ai-001",
				Title:           "图灵奖得主重新定义 AI 路线，世界模型为何突然爆发",
				ChannelName:     "硬核前沿观察",
				Source:          "wx_feed_list",
				Tags:            []string{"AI", "世界模型", "路线判断"},
				PublishedAt:     hotVideoAt[0],
				PublishedAgo:    "2 小时前",
				Views:           486000,
				Likes:           18400,
				Comments:        207,
				GrowthRate:      72.3,
				EngagementScore: 91,
				Signal:          "高争议 + 高转评",
			},
			{
				ID:              "vid-ecom-004",
				Title:           "直播间客单价下滑后，三个改标题动作把转化拉回来了",
				ChannelName:     "操盘手日报",
				Source:          "wx_feed_list",
				Tags:            []string{"电商运营", "转化率", "直播复盘"},
				PublishedAt:     hotVideoAt[1],
				PublishedAgo:    "5 小时前",
				Views:           219000,
				Likes:           9100,
				Comments:        316,
				GrowthRate:      58.1,
				EngagementScore: 83,
				Signal:          "高问题密度",
			},
			{
				ID:              "vid-local-008",
				Title:           "本地门店如何用短视频做团购，30 天投放数据拆给你看",
				ChannelName:     "增长实验室",
				Source:          "wx_feed_list",
				Tags:            []string{"本地生活", "团购", "投放"},
				PublishedAt:     hotVideoAt[2],
				PublishedAgo:    "7 小时前",
				Views:           173000,
				Likes:           7600,
				Comments:        188,
				GrowthRate:      41.7,
				EngagementScore: 78,
				Signal:          "选题可迁移",
			},
		},
		HotTopics: []insight.HotTopic{
			{
				ID:             "topic-001",
				Keyword:        "世界模型",
				Cluster:        "AI 认知路线",
				Source:         "wx_comment_export",
				ObservedAt:     hotTopicAt[0],
				SourceAccounts: []string{"硬核前沿观察"},
				Mentions:       142,
				GrowthRate:     68.2,
				RelatedVideos:  19,
				PromptAngle:    "从“大模型不够”切到“下一代智能范式”",
				Keywords:       []string{"AGI", "推理", "具身", "认知"},
			},
			{
				ID:             "topic-002",
				Keyword:        "直播间客单修复",
				Cluster:        "直播复盘模板",
				Source:         "wx_comment_export",
				ObservedAt:     hotTopicAt[1],
				SourceAccounts: []string{"操盘手日报"},
				Mentions:       97,
				GrowthRate:     44.5,
				RelatedVideos:  12,
				PromptAngle:    "用改标题、改节奏、改赠品三段法做复盘",
				Keywords:       []string{"转化", "停留", "商品卡"},
			},
			{
				ID:             "topic-003",
				Keyword:        "本地生活团购投放",
				Cluster:        "门店增长",
				Source:         "wx_feed_list",
				ObservedAt:     hotTopicAt[2],
				SourceAccounts: []string{"增长实验室"},
				Mentions:       73,
				GrowthRate:     35.9,
				RelatedVideos:  9,
				PromptAngle:    "给门店老板看的预算友好型案例",
				Keywords:       []string{"门店", "团购", "ROI"},
			},
		},
		WatchlistAlerts: []insight.WatchlistAlert{
			{
				ID:           "alert-001",
				AccountName:  "硬核前沿观察",
				AlertType:    "新热视频",
				Source:       "wx_feed_list",
				Severity:     "high",
				Summary:      "2 小时内连续两条 AI 路线争议内容进入高增速区间。",
				OccurredAt:   alertAt[0],
				HappenedAt:   "今天 09:48",
				Recommended:  "24 小时内做同题不同角度拆解，避免正面复述。",
				VideoCount:   2,
				CommentDelta: 161,
			},
			{
				ID:           "alert-002",
				AccountName:  "操盘手日报",
				AlertType:    "评论爆发",
				Source:       "wx_comment_export",
				Severity:     "medium",
				Summary:      "围绕直播标题模板的追问显著上升，出现大量“能否直接套用”评论。",
				OccurredAt:   alertAt[1],
				HappenedAt:   "今天 08:25",
				Recommended:  "优先产出模板拆解图，再补一条避坑版内容。",
				VideoCount:   1,
				CommentDelta: 204,
			},
			{
				ID:           "alert-003",
				AccountName:  "增长实验室",
				AlertType:    "主题切换",
				Source:       "wx_feed_list",
				Severity:     "low",
				Summary:      "账号近三条内容从 SaaS 增长转向本地生活案例。",
				OccurredAt:   alertAt[2],
				HappenedAt:   "昨天 21:10",
				Recommended:  "观察是否形成连续栏目，再决定是否加入重点监控。",
				VideoCount:   3,
				CommentDelta: 48,
			},
		},
		CommentInsights: []insight.CommentInsight{
			{
				ID:               "ci-001",
				Theme:            "用户想知道世界模型和大模型到底差在哪",
				Source:           "wx_comment_export",
				ObservedAt:       commentAt[0],
				SourceAccounts:   []string{"硬核前沿观察"},
				QuestionRate:     58,
				Sentiment:        "争议高",
				Representative:   "别光说路线错了，能不能讲讲世界模型实际能做什么？",
				ActionSuggestion: "补一条 90 秒“概念翻译版”，先回答差异，再给行业影响。",
				RelatedKeywords:  []string{"世界模型", "大模型", "AGI"},
			},
			{
				ID:               "ci-002",
				Theme:            "直播间标题模板是否可以直接照抄",
				Source:           "wx_comment_export",
				ObservedAt:       commentAt[1],
				SourceAccounts:   []string{"操盘手日报"},
				QuestionRate:     41,
				Sentiment:        "求方法",
				Representative:   "这种标题到底适合高客单还是低客单？有没有分类模板？",
				ActionSuggestion: "沉淀为分场景标题库，并在评论区置顶下载方式。",
				RelatedKeywords:  []string{"标题模板", "高客单", "低客单"},
			},
			{
				ID:               "ci-003",
				Theme:            "门店案例更想看预算和转化细节",
				Source:           "wx_feed_list",
				ObservedAt:       commentAt[2],
				SourceAccounts:   []string{"增长实验室"},
				QuestionRate:     36,
				Sentiment:        "理性关注",
				Representative:   "预算花了多少、投了几天、转化成本多少才是重点。",
				ActionSuggestion: "下一条内容用账本式结构讲数据，不再先讲方法论。",
				RelatedKeywords:  []string{"预算", "转化成本", "团购"},
			},
		},
		ReportSummary: insight.ReportSummary{
			Period:            now.Format("2006-01-02") + " 日报",
			Headline:          "AI 路线争议型内容和直播复盘模板型内容同时走高。",
			KeyFinding:        "高传播内容不只靠观点，更依赖“可带走的方法片段”和“强问题区”。",
			RecommendedAction: "上午追热点拆解，下午补模板沉淀，晚间用问答视频回收评论需求。",
			Watchout:          "避免直接跟随争议结论，优先做翻译与框架整理型内容。",
			GeneratedAt:       now,
			AutoGeneratedAt:   "刚刚",
		},
	}
}

func (r *DashboardRepository) ReplaceDashboard(_ context.Context, workspaceID string, snapshot insight.Dashboard) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.snapshots[workspaceID] = snapshot
	return nil
}

func (r *DashboardRepository) ResetDashboard(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.snapshots[workspaceID] = r.seed
	return nil
}
