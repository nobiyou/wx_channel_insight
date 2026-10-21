package ingestpg_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestpg"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func TestRepositoryReadsBoundedSummaryFacts(t *testing.T) {
	db, dsn := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, dsn)
	if err != nil {
		t.Fatalf("new postgres client failed: %v", err)
	}
	repo, err := ingestpg.NewRepository(client)
	if err != nil {
		t.Fatalf("new ingest repository failed: %v", err)
	}

	ctx := context.Background()
	hubUserID := "test:ingest-summary:" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() {
		if err := repo.Reset(ctx, hubUserID); err != nil {
			t.Errorf("clean summary fixtures: %v", err)
		}
	})
	base := time.Date(2026, 10, 16, 1, 0, 0, 0, time.UTC)
	events := make([]ingest.Event, 12)
	for i := range events {
		events[i] = ingest.Event{
			ID: fmt.Sprintf("summary-event-%02d", i), HubUserID: hubUserID,
			BatchID: "summary-batch", Source: ingest.RunSourceWXVideoSearch,
			EventType: "video_snapshot", CollectedAt: base.Add(time.Duration(i) * time.Minute),
			ObjectID: fmt.Sprintf("summary-video-%02d", i),
		}
	}
	if err := repo.AppendEvents(ctx, hubUserID, events); err != nil {
		t.Fatalf("seed summary events: %v", err)
	}
	for i := 0; i < 8; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		if err := repo.AppendRun(ctx, hubUserID, ingest.Run{
			ID: fmt.Sprintf("summary-run-%d", i), Source: ingest.RunSourceWXVideoSearch,
			Status: "success", StartedAt: at, FinishedAt: at.Add(time.Second), BatchID: "summary-batch",
		}); err != nil {
			t.Fatalf("seed summary run: %v", err)
		}
	}
	if err := repo.AppendOperatorRun(ctx, hubUserID, ingest.OperatorRun{
		ID: "summary-operator", Status: "success", TriggerMode: ingest.OperatorTriggerLoop,
		StartedAt: base, FinishedAt: base.Add(time.Minute), LatestBatchID: "summary-batch",
		Outcomes: []ingest.OperatorRunOutcome{{Source: ingest.RunSourceWXVideoSearch, Status: "success"}},
	}); err != nil {
		t.Fatalf("seed automatic source outcome: %v", err)
	}
	facts, err := repo.GetEventSummaryFacts(ctx, hubUserID, 9)
	if err != nil {
		t.Fatalf("read event summary facts failed: %v", err)
	}
	if facts.TotalEvents != 12 || len(facts.RecentEvents) != 9 {
		t.Fatalf("expected populated bounded event facts, got total=%d recent=%d", facts.TotalEvents, len(facts.RecentEvents))
	}
	if facts.LatestEvent.ID != facts.RecentEvents[0].ID {
		t.Fatalf("latest event must match the first recent event: latest=%q recent=%q", facts.LatestEvent.ID, facts.RecentEvents[0].ID)
	}
	if len(facts.Sources) == 0 {
		t.Fatalf("expected per-source event facts, got %+v", facts)
	}

	runs, err := repo.ListSummaryRuns(ctx, hubUserID, 6)
	if err != nil {
		t.Fatalf("read summary runs failed: %v", err)
	}
	if len(runs) < 6 || len(runs) > 100 {
		t.Fatalf("expected bounded run facts, got %d", len(runs))
	}

	autoFacts, err := repo.GetSourceAutoRunFacts(ctx, hubUserID)
	if err != nil {
		t.Fatalf("read source auto-run facts failed: %v", err)
	}
	if len(autoFacts) == 0 {
		t.Fatal("expected source auto-run facts from persisted operator outcomes")
	}
	seen := make(map[string]struct{}, len(autoFacts))
	for _, item := range autoFacts {
		if item.Source == "" || item.LastRunAt.IsZero() {
			t.Fatalf("expected complete source auto-run fact, got %+v", item)
		}
		if _, exists := seen[item.Source]; exists {
			t.Fatalf("expected one auto-run fact per source, got duplicate %q", item.Source)
		}
		seen[item.Source] = struct{}{}
	}
}
