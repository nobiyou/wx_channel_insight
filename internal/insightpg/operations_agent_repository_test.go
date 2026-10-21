package insightpg

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

func TestOperationsPostgresConcurrentAutomaticAndManualAIClaim(t *testing.T) {
	db, _ := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewVideoCurationRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ws := fmt.Sprintf("agent-ai-claim-%d", time.Now().UnixNano())
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM insight_operations_agent WHERE workspace_id=$1", ws) })
	err = repo.MutateOperations(ctx, ws, func(state *insight.OperationsState, _ map[string]insight.VideoCuration) ([]insight.VideoCuration, error) {
		state.Runs = []insight.OperationsRun{
			{ID: "manual", AIFingerprint: "same-evidence", EvidenceIDs: []string{"proposal"}, AIStatus: "not_requested"},
			{ID: "automatic", Trigger: "collection", AIFingerprint: "same-evidence", EvidenceIDs: []string{"proposal"}, AIStatus: "not_requested"},
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := insight.NewDataCenterService(insight.DataCenterServiceConfig{CurationRepository: repo, WorkspaceContextResolver: operationsPGResolver{}})
	var wg sync.WaitGroup
	var claims atomic.Int32
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := []string{"manual", "automatic"}[i%2]
			claimed, err := svc.ClaimOperationsAI(ctx, insight.DataCenterQuery{HubUserID: ws}, id)
			if err != nil {
				errors <- err
			}
			if claimed {
				claims.Add(1)
			}
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if claims.Load() != 1 {
		t.Fatalf("identical evidence claimed %d times", claims.Load())
	}
}

func TestOperationsPostgresAtomicAuditAndWorkspaceLock(t *testing.T) {
	db, _ := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewVideoCurationRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	workspace := fmt.Sprintf("agent-test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM insight_operations_agent WHERE workspace_id=$1", workspace)
		_, _ = db.ExecContext(ctx, "DELETE FROM insight_video_curations WHERE workspace_id=$1", workspace)
	})
	mutate := func(state *insight.OperationsState, _ map[string]insight.VideoCuration) ([]insight.VideoCuration, error) {
		state.Runs = append(state.Runs, insight.OperationsRun{ID: "atomic"})
		return []insight.VideoCuration{{WorkspaceID: workspace, VideoID: "video:a", Status: "invalid", UpdatedAt: time.Now(), UpdatedBy: "test"}}, nil
	}
	if err = repo.MutateOperations(ctx, workspace, mutate); err == nil {
		t.Fatal("invalid write committed")
	}
	state, err := repo.LoadOperations(ctx, workspace)
	if err != nil || len(state.Runs) > 0 {
		t.Fatalf("audit partially committed: %+v %v", state, err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- repo.MutateOperations(ctx, workspace, func(state *insight.OperationsState, _ map[string]insight.VideoCuration) ([]insight.VideoCuration, error) {
			close(entered)
			<-release
			state.Runs = []insight.OperationsRun{{ID: "committed"}}
			return []insight.VideoCuration{{WorkspaceID: workspace, VideoID: "video:a", Status: "selected", UpdatedAt: time.Now(), UpdatedBy: "agent"}}, nil
		})
	}()
	<-entered
	waitCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	err = repo.MutateCurations(waitCtx, workspace, func(_ map[string]insight.VideoCuration) ([]insight.VideoCuration, error) {
		return nil, errors.New("lock failed to serialize")
	})
	deadlineErr := waitCtx.Err()
	cancel()
	close(release)
	if err == nil || !errors.Is(deadlineErr, context.DeadlineExceeded) {
		t.Fatal("parallel manual write bypassed lock")
	}
	if err = <-first; err != nil {
		t.Fatal(err)
	}
	err = repo.MutateCurations(ctx, workspace, func(items map[string]insight.VideoCuration) ([]insight.VideoCuration, error) {
		c := items["video:a"]
		if c.Status != "selected" {
			t.Fatal("did not observe committed selection")
		}
		c.Note = "later manual note"
		return []insight.VideoCuration{c}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := repo.LoadOperations(ctx, workspace+"-other")
	if err != nil || len(other.Runs) > 0 {
		t.Fatal("workspace leak")
	}
	state, err = repo.LoadOperations(ctx, workspace)
	if err != nil || len(state.Runs) != 1 || state.Runs[0].ID != "committed" {
		t.Fatal("committed audit missing")
	}
}

type operationsPGProvider struct {
	rows []ingest.VideoMetricObservation
}

func (p operationsPGProvider) ListEvents(context.Context, ingest.SummaryQuery) ([]ingest.Event, error) {
	return nil, nil
}
func (p operationsPGProvider) ListVideoMetricObservations(context.Context, ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	return p.rows, nil
}

type operationsPGResolver struct{}

func (operationsPGResolver) Resolve(_ context.Context, q workspace.ContextQuery) (workspace.ResolvedContext, error) {
	return workspace.ResolvedContext{Workspace: workspace.DefaultWorkspace{ID: q.HubUserID, OwnerUserID: q.HubUserID}}, nil
}

func TestOperationsPostgresServiceApplyUndoAndManualConflict(t *testing.T) {
	db, _ := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewVideoCurationRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ws := fmt.Sprintf("agent-cycle-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM insight_operations_agent WHERE workspace_id=$1", ws)
		_, _ = db.ExecContext(ctx, "DELETE FROM insight_video_curations WHERE workspace_id=$1", ws)
	})
	now := time.Now().UTC().Truncate(time.Microsecond)
	provider := operationsPGProvider{}
	for i, total := range []int{5, 100, 100, 100, 500} {
		provider.rows = append(provider.rows, ingest.VideoMetricObservation{ID: fmt.Sprint(i), ObjectID: fmt.Sprint(i), ChannelUsername: "peers", LikeCount: total, ObservedAt: now, PublishedAt: now.Add(-5 * 24 * time.Hour)})
	}
	svc := insight.NewDataCenterService(insight.DataCenterServiceConfig{DataProvider: &provider, CurationRepository: repo, WorkspaceContextResolver: operationsPGResolver{}, Now: func() time.Time { return now }})
	q := insight.DataCenterQuery{HubUserID: ws}
	state, _, err := svc.InspectOperations(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	var p insight.OperationsProposal
	for _, candidate := range state.Proposals {
		if candidate.Action == "selected" {
			p = candidate
		}
	}
	if p.ID == "" {
		t.Fatal("no selection candidate")
	}
	applied, err := svc.DecideOperations(ctx, q, "apply", []string{p.ID}, "tester")
	if err != nil || applied[0].Status != "applied" {
		t.Fatalf("apply %+v %v", applied, err)
	}
	cur, err := svc.GetVideoCuration(ctx, q, p.VideoID)
	if err != nil || cur.Status != "selected" {
		t.Fatal("selection did not persist")
	}
	// DB timestamp precision must survive persistence and equality on undo.
	undone, err := svc.DecideOperations(ctx, q, "undo", []string{p.ID}, "tester")
	if err != nil || undone[0].Status != "undone" {
		t.Fatalf("undo %+v %v", undone, err)
	}
	state, err = svc.Operations(ctx, q)
	if err != nil || len(state.Proposals[0].Events) != 2 {
		t.Fatalf("audit missing %+v %v", state, err)
	}
	// A new scan protects the restored record and can propose new evidence.
	extra := provider.rows[4]
	extra.ID = "extra"
	extra.ObjectID = "extra"
	extra.LikeCount = 900
	provider.rows = append(provider.rows, extra)
	state, _, err = svc.InspectOperations(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	var extraID string
	for _, candidate := range state.Proposals {
		if candidate.VideoID == "video:extra" {
			extraID = candidate.ID
		}
	}
	if extraID == "" {
		t.Fatal("new evidence was not inspected")
	}
	if _, err = svc.DecideOperations(ctx, q, "apply", []string{extraID}, "tester"); err != nil {
		t.Fatal(err)
	}
	note := "operator note"
	_, err = svc.CurateVideos(ctx, q, insight.VideoCurationPatch{IDs: []string{"video:extra"}, Note: &note}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := svc.DecideOperations(ctx, q, "undo", []string{extraID}, "tester")
	if err != nil || conflict[0].Status != "conflict" {
		t.Fatalf("manual edit overwritten: %+v %v", conflict, err)
	}
	cur, _ = svc.GetVideoCuration(ctx, q, "video:extra")
	if cur.Note != note || cur.Status != "selected" {
		t.Fatal("manual patch failed")
	}
	other, _ := svc.Operations(ctx, insight.DataCenterQuery{HubUserID: ws + "-other"})
	if len(other.Proposals) != 0 {
		t.Fatal("workspace leak")
	}
	// Opening a persisted legacy queue retires advice atomically, while leaving
	// executed decisions and later human notes available for audit and undo.
	err = repo.MutateOperations(ctx, ws, func(state *insight.OperationsState, _ map[string]insight.VideoCuration) ([]insight.VideoCuration, error) {
		state.Proposals = append(state.Proposals, insight.OperationsProposal{ID: "legacy-advice", RuleVersion: "operations-v1", VideoID: "video:extra", Action: "selected", Status: "pending"})
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err = svc.Operations(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := repo.LoadOperations(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	legacy := persisted.Proposals[len(persisted.Proposals)-1]
	if legacy.Status != "stale" || len(legacy.Events) != 1 || state.RuleVersion == "operations-v1" {
		t.Fatalf("legacy advice survived upgrade: %+v", legacy)
	}
	cur, _ = svc.GetVideoCuration(ctx, q, "video:extra")
	if cur.Note != note || cur.Status != "selected" {
		t.Fatal("rule upgrade changed human curation")
	}
}
