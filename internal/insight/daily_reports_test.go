package insight

import (
	"testing"
	"time"
)

func TestDailyReportIDAndDateAreDeterministic(t *testing.T) {
	at := time.Date(2026, 8, 25, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	report, err := NewDailyReport("ws-1", "2026-08-25", TimeWindow24h, at, Dashboard{})
	if err != nil {
		t.Fatalf("new daily report failed: %v", err)
	}
	if report.ID != "ws-1::2026-08-25::24h" || report.ReportDate != "2026-08-25" {
		t.Fatalf("unexpected report identity: %+v", report)
	}
	if DailyReportDate(at) != "2026-08-25" {
		t.Fatalf("unexpected report date: %s", DailyReportDate(at))
	}
}

func TestValidateDailyReportRejectsMismatchedIdentity(t *testing.T) {
	report, err := NewDailyReport("ws-1", "2026-08-25", TimeWindowAll, time.Now(), Dashboard{})
	if err != nil {
		t.Fatalf("new daily report failed: %v", err)
	}
	report.ID = "wrong"
	if err := ValidateDailyReport(report); err == nil {
		t.Fatal("expected mismatched report id to fail validation")
	}
}
