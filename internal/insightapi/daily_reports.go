package insightapi

import (
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type DailyReportListItemResponse struct {
	ID              string `json:"id"`
	ReportDate      string `json:"reportDate"`
	DataWindow      string `json:"dataWindow"`
	DataWindowLabel string `json:"dataWindowLabel"`
	Title           string `json:"title"`
	Period          string `json:"period"`
	GeneratedAt     string `json:"generatedAt"`
	EvidenceCount   int    `json:"evidenceCount"`
}

type DailyReportsResponse struct {
	WorkspaceID string                        `json:"workspaceId"`
	DataWindow  string                        `json:"dataWindow,omitempty"`
	Items       []DailyReportListItemResponse `json:"items"`
	Count       int                           `json:"count"`
}

type DailyReportResponse struct {
	DailyReportListItemResponse
	Snapshot DashboardResponse `json:"snapshot"`
}

func NewDailyReportsResponse(workspaceID, dataWindow string, reports []insight.DailyReport) DailyReportsResponse {
	items := make([]DailyReportListItemResponse, 0, len(reports))
	for _, report := range reports {
		items = append(items, newDailyReportListItemResponse(report))
	}
	return DailyReportsResponse{
		WorkspaceID: workspaceID,
		DataWindow:  dataWindow,
		Items:       items,
		Count:       len(items),
	}
}

func NewDailyReportResponse(report insight.DailyReport) DailyReportResponse {
	return DailyReportResponse{
		DailyReportListItemResponse: newDailyReportListItemResponse(report),
		Snapshot:                    NewDashboardResponse(report.Snapshot),
	}
}

func newDailyReportListItemResponse(report insight.DailyReport) DailyReportListItemResponse {
	return DailyReportListItemResponse{
		ID:              report.ID,
		ReportDate:      report.ReportDate,
		DataWindow:      report.DataWindow,
		DataWindowLabel: dailyReportWindowLabel(report.DataWindow),
		Title:           report.Snapshot.ReportSummary.Headline,
		Period:          report.Snapshot.ReportSummary.Period,
		GeneratedAt:     report.GeneratedAt.Format(time.RFC3339),
		EvidenceCount:   len(report.Snapshot.HotVideos) + len(report.Snapshot.HotTopics) + len(report.Snapshot.CommentInsights),
	}
}

func dailyReportWindowLabel(window string) string {
	switch window {
	case insight.TimeWindow24h:
		return "近 24 小时"
	case insight.TimeWindow72h:
		return "近 72 小时"
	default:
		return "全部数据"
	}
}
