package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	"github.com/nobiyou/wx_channel_insight/internal/prompt"
)

func openTestClient(t *testing.T, path string) *Client {
	t.Helper()
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestIngestAtomicBatchesDeduplicationAndIsolation(t *testing.T) {
	c := openTestClient(t, filepath.Join(t.TempDir(), "insight.db"))
	r, err := NewIngestRepository(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	event := ingest.Event{ID: "event-1", HubUserID: "owner-a", ObjectID: "video-1", Title: "first"}
	if err := r.AppendEvents(ctx, "owner-a", []ingest.Event{event}); err != nil {
		t.Fatal(err)
	}
	// An unencodable timestamp in the second event must roll back the first insert, too.
	bad := event
	bad.CollectedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := r.AppendEvents(ctx, "owner-a", []ingest.Event{event, bad}); err == nil {
		t.Fatal("expected atomic batch failure")
	}
	items, err := r.ListEvents(ctx, "owner-a")
	if err != nil || len(items) != 1 {
		t.Fatalf("partial batch was persisted: %d %v", len(items), err)
	}
	other, err := r.ListEvents(ctx, "owner-b")
	if err != nil || len(other) != 0 {
		t.Fatal("owner isolation failed")
	}
	metric := ingest.VideoMetricObservation{ID: "metric-1", ObjectID: "video-1"}
	if err := r.AppendVideoMetricObservations(ctx, "owner-a", []ingest.VideoMetricObservation{metric, metric}); err != nil {
		t.Fatal(err)
	}
	metrics, err := r.ListVideoMetricObservations(ctx, "owner-a")
	if err != nil || len(metrics) != 1 {
		t.Fatalf("metrics must deduplicate: %v", err)
	}
	binding := ingest.SourceBinding{Source: ingest.RunSourceWXFeedList, Username: "account-a"}
	if err := r.UpsertBinding(ctx, "owner-a", binding); err != nil {
		t.Fatal(err)
	}
	if err := r.UpsertBinding(ctx, "owner-a", binding); err != nil {
		t.Fatal(err)
	}
	bindings, err := r.ListBindings(ctx, "owner-a")
	if err != nil || len(bindings) != 1 {
		t.Fatalf("binding upsert: %v", err)
	}
	if err := r.Reset(ctx, "owner-b"); err != nil {
		t.Fatal(err)
	}
	items, err = r.ListEvents(ctx, "owner-a")
	if err != nil || len(items) != 1 {
		t.Fatal("reset crossed owner boundary")
	}
}

func TestMonitoringConcurrentClaimAcrossHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "insight.db")
	a, _ := NewMonitoringRepository(openTestClient(t, path))
	b, _ := NewMonitoringRepository(openTestClient(t, path))
	ctx := context.Background()
	now := time.Now().UTC()
	task := monitoring.Task{ID: "task-1", HubUserID: "owner", Name: "monitor", Status: monitoring.StatusEnabled, ScopeMode: monitoring.ScopeSelected, Interval: 5 * time.Minute, NextRunAt: now.Add(-time.Minute), Targets: []monitoring.TaskTarget{{BindingRef: monitoring.BindingRef{Source: "wx_feed_list", TargetKey: "account-1"}}}}
	if err := a.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var claimed atomic.Int32
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			repo := a
			if i%2 == 0 {
				repo = b
			}
			_, err := repo.ClaimDueTask(ctx, "owner", fmt.Sprintf("run-%d", i), now, now.Add(time.Minute))
			if err == nil {
				claimed.Add(1)
			} else if !errors.Is(err, monitoring.ErrNoDueTask) {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if claimed.Load() != 1 {
		t.Fatalf("one task was claimed %d times", claimed.Load())
	}
	runs, err := b.ListTaskRuns(ctx, "owner", "task-1", 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("claim history: %d %v", len(runs), err)
	}
	if err := a.CompleteTaskRun(ctx, "owner", runs[0].ID, monitoring.TaskRunUpdate{Status: monitoring.RunStatusSuccess, FinishedAt: now.Add(time.Second), NextRunAt: now.Add(5 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	updated, err := b.GetTask(ctx, "owner", task.ID)
	if err != nil || updated.LastStatus != monitoring.RunStatusSuccess {
		t.Fatalf("completion not visible: %v", err)
	}
}

func TestAISettingsEncryptionAndClearingPersistedCiphertext(t *testing.T) {
	c := openTestClient(t, filepath.Join(t.TempDir(), "insight.db"))
	codec, err := insightai.NewSecretCodec(strings.Repeat("12", 32))
	if err != nil {
		t.Fatal(err)
	}
	r, _ := NewAIServiceRepository(c, codec)
	ctx := context.Background()
	v := insightai.StoredServiceSettings{Provider: insightai.ProviderOpenAI, BaseURL: insightai.DefaultBaseURL, Model: "test-model", TimeoutSeconds: 45, APIKey: "sqlite-private-test-secret"}
	if err := r.Save(ctx, "owner-a", v); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := c.db.QueryRow(`SELECT payload FROM insight_documents WHERE bucket='ai_service'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), v.APIKey) || !strings.Contains(string(raw), "apiKeyCiphertext") {
		t.Fatal("API key must be encrypted at rest")
	}
	loaded, found, err := r.Load(ctx, "owner-a")
	if err != nil || !found || loaded.APIKey != v.APIKey {
		t.Fatalf("decrypt settings: %v", err)
	}
	if _, found, err := r.Load(ctx, "owner-b"); err != nil || found {
		t.Fatal("owner isolation failed")
	}
	withoutCodec, _ := NewAIServiceRepository(c, nil)
	if err := withoutCodec.Save(ctx, "owner-b", v); err == nil {
		t.Fatal("plaintext persistence must be rejected")
	}
	if err := r.ClearKey(ctx, "owner-a"); err != nil {
		t.Fatal(err)
	}
	loaded, found, err = r.Load(ctx, "owner-a")
	if err != nil || !found || loaded.APIKey != "" || loaded.APIKeyCiphertext != "" || loaded.APIKeyNonce != "" {
		t.Fatal("key or ciphertext survived clearing")
	}
}

func TestOperationsRollbackAndWorkspaceIsolation(t *testing.T) {
	c := openTestClient(t, filepath.Join(t.TempDir(), "insight.db"))
	r, _ := NewVideoCurationRepository(c)
	ctx := context.Background()
	write := func(state *insight.OperationsState, current map[string]insight.VideoCuration) ([]insight.VideoCuration, error) {
		state.RuleVersion = "v1"
		return []insight.VideoCuration{{WorkspaceID: "workspace-a", VideoID: "video", Status: "selected"}}, nil
	}
	if err := r.MutateOperations(ctx, "workspace-a", write); err != nil {
		t.Fatal(err)
	}
	err := r.MutateOperations(ctx, "workspace-a", func(state *insight.OperationsState, _ map[string]insight.VideoCuration) ([]insight.VideoCuration, error) {
		state.RuleVersion = "must-rollback"
		return []insight.VideoCuration{{WorkspaceID: "workspace-b", VideoID: "video"}}, nil
	})
	if err == nil {
		t.Fatal("cross-workspace write must fail")
	}
	state, err := r.LoadOperations(ctx, "workspace-a")
	if err != nil || state.RuleVersion != "v1" {
		t.Fatal("operations state did not roll back")
	}
	items, err := r.List(ctx, "workspace-a")
	if err != nil || len(items) != 1 || items[0].WorkspaceID != "workspace-a" || items[0].Status != "selected" {
		t.Fatal("curation did not round trip")
	}
	other, err := r.List(ctx, "workspace-b")
	if err != nil || len(other) != 0 {
		t.Fatal("curation escaped workspace")
	}
}

func TestHistoryAndPromptRepositoriesRoundTrip(t *testing.T) {
	c := openTestClient(t, filepath.Join(t.TempDir(), "insight.db"))
	ctx := context.Background()
	reports, err := NewDailyReportRepository(c)
	if err != nil {
		t.Fatal(err)
	}
	report, err := insight.NewDailyReport("workspace-a", "2026-10-20", "24h", time.Now().UTC(), insight.Dashboard{})
	if err != nil {
		t.Fatal(err)
	}
	if err := reports.Upsert(ctx, report); err != nil {
		t.Fatal(err)
	}
	if _, found, err := reports.Get(ctx, "workspace-b", report.ID); err != nil || found {
		t.Fatal("report crossed workspace")
	}
	items, err := reports.List(ctx, "workspace-a", "24h", 1)
	if err != nil || len(items) != 1 || items[0].ID != report.ID {
		t.Fatalf("daily report round trip: %v", err)
	}
	runs, _ := NewAIAnalysisRepository(c)
	run := insight.AIAnalysisRun{ID: "run-1", WorkspaceID: "workspace-a", Module: "content-analysis", ScopeKey: "dashboard", Window: "24h", InputFingerprint: "sha256:test", Status: insight.AIAnalysisStatusCompleted, Result: json.RawMessage(`{"summary":"stored"}`), ModelProvider: "openai", ModelName: "test-model", ModelAttempted: true, RunAt: time.Now().UTC()}
	if err := runs.Save(ctx, run); err != nil {
		t.Fatal(err)
	}
	history, err := runs.List(ctx, "workspace-a", "content-analysis", "dashboard", "24h", 1)
	if err != nil || len(history) != 1 || history[0].ID != run.ID {
		t.Fatalf("AI history round trip: %v", err)
	}
	if _, found, err := runs.Get(ctx, "workspace-b", run.ID); err != nil || found {
		t.Fatal("analysis crossed workspace")
	}
	settings, _ := NewPromptRepository(c)
	value := prompt.DefaultSettings()
	if err := settings.Save(ctx, "owner", value); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.Load(ctx, "owner")
	if err != nil || len(loaded.Profiles) != len(value.Profiles) {
		t.Fatalf("prompt round trip: %v", err)
	}
}
