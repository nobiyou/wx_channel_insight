package insight

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrDailyReportNotFound = errors.New("daily report was not found")
	ErrInvalidReportDate   = errors.New("report date must be YYYY-MM-DD")
	ErrInvalidDailyReport  = errors.New("invalid daily report")
)

const DailyReportHistoryLimit = 100

// DailyReport is a bounded, immutable-in-shape snapshot of the dashboard that
// was visible when a report was generated. Raw source records remain owned by
// ingest and are deliberately not copied into the archive.
type DailyReport struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	ReportDate  string    `json:"reportDate"`
	DataWindow  string    `json:"dataWindow"`
	GeneratedAt time.Time `json:"generatedAt"`
	Snapshot    Dashboard `json:"snapshot"`
}

func NormalizeReportDate(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return "", false
	}
	return value, true
}

func DailyReportDate(at time.Time) string {
	if at.IsZero() {
		at = time.Now()
	}
	return at.In(time.Local).Format("2006-01-02")
}

func DailyReportID(workspaceID, reportDate, dataWindow string) string {
	return fmt.Sprintf("%s::%s::%s", strings.TrimSpace(workspaceID), reportDate, dataWindow)
}

func NewDailyReport(workspaceID, reportDate, dataWindow string, generatedAt time.Time, snapshot Dashboard) (DailyReport, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return DailyReport{}, fmt.Errorf("%w: workspaceId is required", ErrInvalidDailyReport)
	}
	normalizedDate, ok := NormalizeReportDate(reportDate)
	if !ok {
		return DailyReport{}, ErrInvalidReportDate
	}
	normalizedWindow, ok := NormalizeTimeWindow(dataWindow)
	if !ok {
		return DailyReport{}, ErrInvalidTimeWindow
	}
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	return DailyReport{
		ID:          DailyReportID(workspaceID, normalizedDate, normalizedWindow),
		WorkspaceID: workspaceID,
		ReportDate:  normalizedDate,
		DataWindow:  normalizedWindow,
		GeneratedAt: generatedAt,
		Snapshot:    cloneDashboard(snapshot),
	}, nil
}

func ValidateDailyReport(report DailyReport) error {
	if strings.TrimSpace(report.WorkspaceID) == "" {
		return fmt.Errorf("%w: workspaceId is required", ErrInvalidDailyReport)
	}
	date, ok := NormalizeReportDate(report.ReportDate)
	if !ok {
		return ErrInvalidReportDate
	}
	window, ok := NormalizeTimeWindow(report.DataWindow)
	if !ok {
		return ErrInvalidTimeWindow
	}
	if report.GeneratedAt.IsZero() {
		return fmt.Errorf("%w: generatedAt is required", ErrInvalidDailyReport)
	}
	if report.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidDailyReport)
	}
	if report.ID != DailyReportID(report.WorkspaceID, date, window) {
		return fmt.Errorf("%w: id does not match workspace, date, and window", ErrInvalidDailyReport)
	}
	return nil
}

func CloneDailyReport(source DailyReport) DailyReport {
	source.Snapshot = cloneDashboard(source.Snapshot)
	return source
}
