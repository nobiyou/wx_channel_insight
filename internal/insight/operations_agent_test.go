package insight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type agentTestStore struct {
	curationTestStore
	states map[string]OperationsState
}
type agentTestResolver struct{}

func (agentTestResolver) Resolve(_ context.Context, q workspace.ContextQuery) (workspace.ResolvedContext, error) {
	return workspace.ResolvedContext{Workspace: workspace.DefaultWorkspace{ID: q.HubUserID, OwnerUserID: q.HubUserID}}, nil
}
func (s *agentTestStore) LoadOperations(_ context.Context, workspace string) (OperationsState, error) {
	return s.states[workspace], nil
}
func (s *agentTestStore) MutateOperations(_ context.Context, workspace string, mutate OperationsMutation) error {
	// Copy before mutation to model database rollback and JSON persistence.
	raw, _ := json.Marshal(s.states[workspace])
	var state OperationsState
	_ = json.Unmarshal(raw, &state)
	current := map[string]VideoCuration{}
	for _, v := range s.items {
		if v.WorkspaceID == workspace {
			current[v.VideoID] = v
		}
	}
	writes, err := mutate(&state, current)
	if err != nil {
		return err
	}
	s.states[workspace] = state
	return s.SaveBatch(context.Background(), writes)
}
func agentFixture() (*DataCenterService, *agentTestStore, *dataCenterProviderStub, *time.Time) {
	now := time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)
	provider := &dataCenterProviderStub{}
	for i, total := range []int{5, 100, 100, 100, 500} {
		provider.observations = append(provider.observations, ingest.VideoMetricObservation{ID: fmt.Sprint(i), ObjectID: fmt.Sprint(i), ChannelUsername: "peer", ObservedAt: now, PublishedAt: now.Add(-5 * 24 * time.Hour), LikeCount: total})
	}
	for i := 0; i < 5; i++ {
		older := provider.observations[i]
		older.ID += "-old"
		older.ObservedAt = now.Add(-time.Hour)
		older.LikeCount = 1
		provider.observations = append(provider.observations, older)
	}
	store := &agentTestStore{curationTestStore: curationTestStore{items: map[string]VideoCuration{}}, states: map[string]OperationsState{}}
	svc := NewDataCenterService(DataCenterServiceConfig{DataProvider: provider, CurationRepository: store, WorkspaceContextResolver: agentTestResolver{}, Now: func() time.Time { return now }})
	return svc, store, provider, &now
}

func TestOperationsAutomaticInspectionSkipsEmptyAndNeverCurates(t *testing.T) {
	svc, store, provider, _ := agentFixture()
	ctx := context.Background()
	q := DataCenterQuery{HubUserID: "one"}
	state, _, err := svc.InspectOperationsAfterCollection(ctx, q)
	if err != nil || len(state.Runs) != 1 || state.Runs[0].Trigger != "collection" || len(state.Proposals) == 0 {
		t.Fatalf("automatic inspection missing: %+v %v", state, err)
	}
	if len(store.items) != 0 {
		t.Fatal("automatic inspection changed manual curations")
	}
	provider.observations = nil
	state, _, err = svc.InspectOperationsAfterCollection(ctx, DataCenterQuery{HubUserID: "empty"})
	if err != nil || len(state.Runs) != 0 {
		t.Fatalf("empty collection created a run: %+v %v", state, err)
	}
}

func TestOperationsAIClaimSkipsEmptyAndSerializesIdenticalEvidenceAcrossRuns(t *testing.T) {
	svc, store, _, now := agentFixture()
	ctx := context.Background()
	q := DataCenterQuery{HubUserID: "one"}
	store.states["one"] = OperationsState{Runs: []OperationsRun{
		{ID: "empty", AIStatus: "not_requested"},
		{ID: "first", AIFingerprint: "same", EvidenceIDs: []string{"proposal"}, AIStatus: "not_requested"},
		{ID: "second", AIFingerprint: "same", EvidenceIDs: []string{"proposal"}, AIStatus: "not_requested"},
	}}
	claimed, err := svc.ClaimOperationsAI(ctx, q, "empty")
	if err != nil || claimed || store.states["one"].Runs[0].AIStatus != "insufficient_evidence" {
		t.Fatalf("empty evidence billed: %v %v", claimed, err)
	}
	claimed, err = svc.ClaimOperationsAI(ctx, q, "first")
	if err != nil || !claimed {
		t.Fatalf("first claim: %v %v", claimed, err)
	}
	claimed, err = svc.ClaimOperationsAI(ctx, q, "second")
	if err != nil || claimed {
		t.Fatalf("concurrent duplicate billed: %v %v", claimed, err)
	}
	if err = svc.SaveOperationsAI(ctx, q, "first", "completed", "done", "ai-1", "model", json.RawMessage(`{"summary":"saved"}`)); err != nil {
		t.Fatal(err)
	}
	claimed, err = svc.ClaimOperationsAI(ctx, q, "second")
	if err != nil || claimed || store.states["one"].Runs[2].AIAnalysisID != "ai-1" {
		t.Fatalf("completed result not reused: %+v %v", store.states["one"], err)
	}
	// A genuinely interrupted claim can be recovered after the lease window.
	state := store.states["one"]
	state.Runs = []OperationsRun{{ID: "interrupted", AIFingerprint: "new", EvidenceIDs: []string{"new"}, AIStatus: "running", AIStartedAt: now.Add(-4 * time.Minute)}}
	store.states["one"] = state
	claimed, err = svc.ClaimOperationsAI(ctx, q, "interrupted")
	if err != nil || !claimed {
		t.Fatalf("stale claim not recovered: %v %v", claimed, err)
	}
}
func TestOperationsProtectsQualityAndHumanDecisions(t *testing.T) {
	svc, store, provider, now := agentFixture()
	ctx := context.Background()
	q := DataCenterQuery{HubUserID: "one"}
	provider.observations[0].MissingMetrics = ingest.MetricComment
	provider.observations[5].MissingMetrics = ingest.MetricComment
	provider.observations[4].ObservedAt = now.Add(-48 * time.Hour)
	provider.observations[9].ObservedAt = now.Add(-49 * time.Hour)
	state, _, err := svc.InspectOperations(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if state.Runs[0].Quality.Missing != 1 || state.Runs[0].Quality.Stale != 1 {
		t.Fatalf("quality: %+v", state.Runs[0])
	}
	for _, p := range state.Proposals {
		if (p.VideoID == "video:0" || p.VideoID == "video:4") && p.Action != "refresh" {
			t.Fatalf("bad quality rated: %+v", p)
		}
	}
	store.items["one|video:2"] = VideoCuration{WorkspaceID: "one", VideoID: "video:2", Status: "selected", UpdatedBy: "human", UpdatedAt: *now}
	state, _, err = svc.InspectOperations(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range state.Proposals {
		if p.VideoID == "video:2" && p.Action != "refresh" && p.Status == "pending" {
			t.Fatal("human selection targeted")
		}
	}
}
func TestOperationsApplyIsIdempotentAndUndoProtectsLaterEdits(t *testing.T) {
	svc, _, _, _ := agentFixture()
	ctx := context.Background()
	q := DataCenterQuery{HubUserID: "one"}
	state, _, err := svc.InspectOperations(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, p := range state.Proposals {
		if p.Action == "selected" {
			id = p.ID
		}
	}
	if id == "" {
		t.Fatal("missing standout proposal")
	}
	result, err := svc.DecideOperations(ctx, q, "apply", []string{id}, "human")
	if err != nil || result[0].Status != "applied" {
		t.Fatalf("apply: %+v %v", result, err)
	}
	result, err = svc.DecideOperations(ctx, q, "apply", []string{id}, "human")
	if err != nil || result[0].Status != "applied" {
		t.Fatal("repeat apply failed")
	}
	note := "人工补充"
	_, err = svc.CurateVideos(ctx, q, VideoCurationPatch{IDs: []string{"video:4"}, Note: &note}, "human")
	if err != nil {
		t.Fatal(err)
	}
	result, err = svc.DecideOperations(ctx, q, "undo", []string{id}, "human")
	if err != nil || result[0].Status != "conflict" {
		t.Fatalf("undo overwrote later edit: %+v %v", result, err)
	}
	cur, _ := svc.GetVideoCuration(ctx, q, "video:4")
	if cur.Note != note || cur.Status != "selected" {
		t.Fatal("human data lost")
	}
	other, _ := svc.Operations(ctx, DataCenterQuery{HubUserID: "two"})
	if len(other.Proposals) != 0 {
		t.Fatal("workspace leak")
	}
}
func TestOperationsChangedEvidenceAndCurationInvalidateProposal(t *testing.T) {
	for _, change := range []string{"metrics", "manual", "age"} {
		t.Run(change, func(t *testing.T) {
			svc, _, provider, now := agentFixture()
			ctx := context.Background()
			q := DataCenterQuery{HubUserID: "one"}
			state, _, err := svc.InspectOperations(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			var proposal OperationsProposal
			for _, p := range state.Proposals {
				if p.Action == "selected" {
					proposal = p
				}
			}
			switch change {
			case "metrics":
				provider.observations[4].LikeCount = 501
			case "manual":
				note := "reviewed"
				_, _ = svc.CurateVideos(ctx, q, VideoCurationPatch{IDs: []string{proposal.VideoID}, Note: &note}, "human")
			case "age":
				*now = now.Add(25 * time.Hour)
			}
			result, err := svc.DecideOperations(ctx, q, "apply", []string{proposal.ID}, "human")
			if err != nil || result[0].Status != "stale" {
				t.Fatalf("changed evidence applied: %+v %v", result, err)
			}
		})
	}
}
func TestOperationsUndoRestoresAndDismissDoesNotReappear(t *testing.T) {
	svc, _, _, _ := agentFixture()
	ctx := context.Background()
	q := DataCenterQuery{HubUserID: "one"}
	state, _, err := svc.InspectOperations(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Proposals[0].ID
	_, err = svc.DecideOperations(ctx, q, "dismiss", []string{id}, "human")
	if err != nil {
		t.Fatal(err)
	}
	state, _, _ = svc.InspectOperations(ctx, q)
	count := 0
	for _, p := range state.Proposals {
		if p.ID == id {
			count++
			if p.Status != "dismissed" {
				t.Fatal("dismiss reverted")
			}
		}
	}
	if count != 1 {
		t.Fatal("duplicate proposal")
	}
	if results, err := svc.DecideOperations(ctx, q, "undo", []string{id}, "human"); err != nil || results[0].Status != "pending" {
		t.Fatalf("dismissal cannot be restored: %+v %v", results, err)
	}
	state, _ = svc.Operations(ctx, q)
	var pick string
	for _, p := range state.Proposals {
		if p.Status == "pending" && p.Action != "refresh" {
			pick = p.ID
			break
		}
	}
	if pick == "" {
		t.Fatal("fixture missing second candidate")
	}
	_, _ = svc.DecideOperations(ctx, q, "apply", []string{pick}, "human")
	results, err := svc.DecideOperations(ctx, q, "undo", []string{pick}, "human")
	if err != nil || results[0].Status != "undone" {
		t.Fatalf("undo: %+v %v", results, err)
	}
	if _, err = svc.DecideOperations(ctx, q, "apply", []string{"absent"}, "human"); !errors.Is(err, ErrDataCenterNotFound) {
		t.Fatalf("missing id: %v", err)
	}
}

func TestOperationsFreshOpportunitiesAreNotStarvedByQualityBacklog(t *testing.T) {
	svc, _, provider, now := agentFixture()
	for i := 0; i < 40; i++ {
		provider.observations = append(provider.observations, ingest.VideoMetricObservation{ID: fmt.Sprintf("stale-%d", i), ObjectID: fmt.Sprintf("stale-%d", i), ObservedAt: now.Add(-48 * time.Hour), PublishedAt: now.Add(-5 * 24 * time.Hour)})
	}
	state, _, err := svc.InspectOperations(context.Background(), DataCenterQuery{HubUserID: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Proposals) != 20 {
		t.Fatalf("queue size %d", len(state.Proposals))
	}
	fresh := 0
	for _, p := range state.Proposals {
		if p.Action != "refresh" {
			fresh++
		}
	}
	if fresh == 0 {
		t.Fatal("quality backlog hid actionable opportunities")
	}
}

func TestOperationsAIReuseFollowsCandidateBatch(t *testing.T) {
	svc, _, provider, now := agentFixture()
	provider.observations = nil
	for i := 0; i < 40; i++ {
		provider.observations = append(provider.observations, ingest.VideoMetricObservation{ID: fmt.Sprint(i), ObjectID: fmt.Sprint(i), ObservedAt: now.Add(-48 * time.Hour), PublishedAt: now.Add(-5 * 24 * time.Hour)})
	}
	ctx := context.Background()
	q := DataCenterQuery{HubUserID: "one"}
	first, _, err := svc.InspectOperations(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.SaveOperationsAI(ctx, q, first.Runs[0].ID, "completed", "summary", "ai-1", "model", json.RawMessage(`{"summary":"batch one"}`))
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(2 * time.Minute)
	second, _, err := svc.InspectOperations(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if second.Runs[0].Proposed != 20 || second.Runs[0].AIFingerprint == first.Runs[0].AIFingerprint || second.Runs[0].AIStatus != "not_requested" {
		t.Fatal("new batch reused unrelated AI evidence")
	}
}

func TestOperationsSeparatesHistoricalCasesFromCurrentOpportunities(t *testing.T) {
	for _, scenario := range []string{"static-old", "growing-old", "tiny-growth-old", "unknown-publication", "future-publication", "recent"} {
		t.Run(scenario, func(t *testing.T) {
			svc, _, provider, now := agentFixture()
			for i := range provider.observations {
				if scenario != "recent" {
					provider.observations[i].PublishedAt = now.AddDate(-4, 0, 0)
				}
				if scenario == "unknown-publication" {
					provider.observations[i].PublishedAt = time.Time{}
				}
				if scenario == "future-publication" {
					provider.observations[i].PublishedAt = now.Add(time.Hour)
				}
				if i >= 5 {
					provider.observations[i].LikeCount = provider.observations[i-5].LikeCount
				}
			}
			if scenario == "growing-old" {
				provider.observations[9].LikeCount = 300
			}
			if scenario == "tiny-growth-old" {
				provider.observations[9].LikeCount = 499
			}
			state, _, err := svc.InspectOperations(context.Background(), DataCenterQuery{HubUserID: "one"})
			if err != nil {
				t.Fatal(err)
			}
			wanted := map[string]string{"static-old": "historical", "growing-old": "watching", "tiny-growth-old": "historical", "recent": "selected"}[scenario]
			found := ""
			for _, p := range state.Proposals {
				if p.VideoID == "video:4" {
					found = p.Action
				}
				if scenario != "recent" && (p.Action == "selected" || p.Action == "excluded") {
					t.Fatalf("old or unknown-age content treated as current: %+v", p)
				}
			}
			if found != wanted {
				t.Fatalf("%s: got %q, want %q", scenario, found, wanted)
			}
		})
	}
}

func TestOperationsOldBackfillRequiresFocusAndCannotDisplaceRecentData(t *testing.T) {
	svc, store, provider, now := agentFixture()
	for _, id := range []string{"archive", "focus", "new"} {
		published := now.AddDate(-4, 0, 0)
		if id == "new" {
			published = now.Add(-72 * time.Hour)
		}
		provider.observations = append(provider.observations, ingest.VideoMetricObservation{ID: id, ObjectID: id, PublishedAt: published, ObservedAt: now.Add(-48 * time.Hour), MissingMetrics: ingest.MetricComment})
	}
	store.items["one|video:focus"] = VideoCuration{WorkspaceID: "one", VideoID: "video:focus", Status: "watching", UpdatedBy: "human"}
	state, _, err := svc.InspectOperations(context.Background(), DataCenterQuery{HubUserID: "one"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range state.Proposals {
		if p.Action == "refresh" {
			got[p.VideoID] = true
		}
	}
	if got["video:archive"] || !got["video:focus"] || !got["video:new"] {
		t.Fatalf("backfill scope: %+v", got)
	}
}

func TestOperationsLegacyAdviceIsRetiredWithoutChangingHumanCuration(t *testing.T) {
	svc, store, _, now := agentFixture()
	q := DataCenterQuery{HubUserID: "one"}
	store.states["one"] = OperationsState{Proposals: []OperationsProposal{{ID: "legacy", VideoID: "video:4", Action: "selected", Status: "pending"}, {ID: "executed", Action: "selected", Status: "applied"}}}
	store.items["one|video:4"] = VideoCuration{WorkspaceID: "one", VideoID: "video:4", Status: "selected", UpdatedBy: "human", UpdatedAt: *now, Note: "保留案例"}
	state, err := svc.Operations(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if state.Proposals[0].Status != "stale" || len(state.Proposals[0].Events) != 1 || !strings.Contains(state.Proposals[0].Message, "规则") {
		t.Fatalf("legacy advice still actionable: %+v", state.Proposals[0])
	}
	persisted := store.states["one"]
	if persisted.Proposals[0].Status != "stale" || persisted.Proposals[1].Status != "applied" || store.items["one|video:4"].Note != "保留案例" {
		t.Fatal("migration lost audit or human state")
	}
	state, _ = svc.Operations(context.Background(), q)
	if len(state.Proposals[0].Events) != 1 {
		t.Fatal("repeated read duplicates migration events")
	}
}

func TestOperationsHistoricalReferenceCannotApplyAsCuration(t *testing.T) {
	svc, store, provider, now := agentFixture()
	for i := range provider.observations {
		provider.observations[i].PublishedAt = now.AddDate(-4, 0, 0)
		if i >= 5 {
			provider.observations[i].LikeCount = provider.observations[i-5].LikeCount
		}
	}
	ctx, q := context.Background(), DataCenterQuery{HubUserID: "one"}
	state, _, err := svc.InspectOperations(ctx, q)
	if err != nil || len(state.Proposals) == 0 {
		t.Fatalf("historical reference: %+v %v", state, err)
	}
	result, err := svc.DecideOperations(ctx, q, "apply", []string{state.Proposals[0].ID}, "human")
	if err != nil || result[0].Status != "reference_only" || len(store.items) != 0 {
		t.Fatalf("historical applied: %+v %v", result, err)
	}
}

func TestOperationsOldGrowthNeedsRecentMeaningfulComparableEvidence(t *testing.T) {
	now := time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)
	total, delta, speed := 10000, 100, 100.0
	row := DataCenterItem{ObservedAt: now, GrowthStatus: "available", GrowthWindow: VideoGrowthWindow{BaselineObservedAt: now.Add(-time.Hour), LatestObservedAt: now}, Performance: VideoPerformance{Interactions: &total, GrowthInteractions: &delta, GrowthPerHour: &speed, Freshness: "fresh"}}
	if !operationsOldGrowth(row, now) {
		t.Fatal("valid recent growth rejected")
	}
	for _, scenario := range []string{"tiny-share", "stale-baseline", "single", "correction", "too-short", "stale-latest"} {
		t.Run(scenario, func(t *testing.T) {
			x := row
			switch scenario {
			case "tiny-share":
				huge := 1000000
				x.Performance.Interactions = &huge
			case "stale-baseline":
				x.GrowthWindow.BaselineObservedAt = now.Add(-73 * time.Hour)
			case "single":
				x.GrowthStatus = "insufficient_history"
			case "correction":
				x.GrowthStatus = "counter_correction"
			case "too-short":
				x.GrowthWindow.BaselineObservedAt = now.Add(-time.Minute)
			case "stale-latest":
				x.Performance.Freshness = "stale"
			}
			if operationsOldGrowth(x, now) {
				t.Fatalf("invalid growth accepted: %s", scenario)
			}
		})
	}
}

func TestOperationsPublicationWindowIsRecheckedBeforeApply(t *testing.T) {
	svc, _, provider, now := agentFixture()
	for i := range provider.observations {
		provider.observations[i].PublishedAt = now.Add(-30 * 24 * time.Hour)
	}
	ctx, q := context.Background(), DataCenterQuery{HubUserID: "one"}
	state, _, err := svc.InspectOperations(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, p := range state.Proposals {
		if p.Action == "selected" {
			id = p.ID
		}
	}
	if id == "" {
		t.Fatal("30-day boundary should be eligible")
	}
	*now = now.Add(time.Minute)
	result, err := svc.DecideOperations(ctx, q, "apply", []string{id}, "human")
	if err != nil || result[0].Status != "stale" {
		t.Fatalf("aged content applied: %+v %v", result, err)
	}
}
