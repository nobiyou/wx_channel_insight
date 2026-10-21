package insightfile_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
)

func TestDailyReportRepositoryRoundTripsAndOrdersHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daily-reports.json")
	repo, err := insightfile.NewDailyReportRepository(path)
	if err != nil {
		t.Fatalf("new daily report repository: %v", err)
	}
	for _, date := range []string{"2026-08-23", "2026-08-25", "2026-08-24"} {
		report, err := insight.NewDailyReport("ws-1", date, insight.TimeWindowAll, time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC), insight.Dashboard{})
		if err != nil {
			t.Fatalf("new report %s: %v", date, err)
		}
		if err := repo.Upsert(context.Background(), report); err != nil {
			t.Fatalf("upsert report %s: %v", date, err)
		}
	}
	reloaded, err := insightfile.NewDailyReportRepository(path)
	if err != nil {
		t.Fatalf("reload daily report repository: %v", err)
	}
	items, err := reloaded.List(context.Background(), "ws-1", insight.TimeWindowAll, 2)
	if err != nil {
		t.Fatalf("list daily reports: %v", err)
	}
	if len(items) != 2 || items[0].ReportDate != "2026-08-25" || items[1].ReportDate != "2026-08-24" {
		t.Fatalf("expected descending dates, got %+v", items)
	}
}
