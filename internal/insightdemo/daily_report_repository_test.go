package insightdemo

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

func TestDailyReportRepositoryUpsertsByDailyIdentity(t *testing.T) {
	repo := NewDailyReportRepository()
	first, err := insight.NewDailyReport("ws-1", "2026-08-25", insight.TimeWindowAll, time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC), insight.Dashboard{})
	if err != nil {
		t.Fatalf("new first report: %v", err)
	}
	if err := repo.Upsert(context.Background(), first); err != nil {
		t.Fatalf("upsert first report: %v", err)
	}
	updated := first
	updated.GeneratedAt = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	updated.Snapshot.ReportSummary.Headline = "updated"
	if err := repo.Upsert(context.Background(), updated); err != nil {
		t.Fatalf("upsert updated report: %v", err)
	}
	items, err := repo.List(context.Background(), "ws-1", insight.TimeWindowAll, 30)
	if err != nil {
		t.Fatalf("list reports: %v", err)
	}
	if len(items) != 1 || items[0].Snapshot.ReportSummary.Headline != "updated" {
		t.Fatalf("expected one updated report, got %+v", items)
	}
}
