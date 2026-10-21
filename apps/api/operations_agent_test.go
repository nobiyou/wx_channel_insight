package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
	"github.com/nobiyou/wx_channel_insight/internal/prompt"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type apiAgentStore struct {
	states    map[string]insight.OperationsState
	curations map[string]insight.VideoCuration
}

func (s *apiAgentStore) List(_ context.Context, w string) ([]insight.VideoCuration, error) {
	items := []insight.VideoCuration{}
	for _, c := range s.curations {
		if c.WorkspaceID == w {
			items = append(items, c)
		}
	}
	return items, nil
}
func (s *apiAgentStore) SaveBatch(_ context.Context, items []insight.VideoCuration) error {
	for _, c := range items {
		s.curations[c.WorkspaceID+"|"+c.VideoID] = c
	}
	return nil
}
func (s *apiAgentStore) LoadOperations(_ context.Context, w string) (insight.OperationsState, error) {
	return s.states[w], nil
}
func (s *apiAgentStore) MutateOperations(ctx context.Context, w string, fn insight.OperationsMutation) error {
	raw, _ := json.Marshal(s.states[w])
	var state insight.OperationsState
	_ = json.Unmarshal(raw, &state)
	current := map[string]insight.VideoCuration{}
	for _, c := range s.curations {
		if c.WorkspaceID == w {
			current[c.VideoID] = c
		}
	}
	writes, err := fn(&state, current)
	if err != nil {
		return err
	}
	s.states[w] = state
	return s.SaveBatch(ctx, writes)
}

func TestAutomaticOperationsUsesWorkspaceAIAndReusesEvidenceWithoutCurating(t *testing.T) {
	runner := &aiAnalysisRunnerStub{err: errors.New("model unavailable")}
	srv := newAIAnalysisTestServer(t, runner, insightdemo.NewAIAnalysisRepository())
	store := &apiAgentStore{states: map[string]insight.OperationsState{}, curations: map[string]insight.VideoCuration{}}
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{
		CurationRepository: store, WorkspaceContextResolver: srv.contextResolver,
		DataProvider: apiDataCenterProvider{observations: []ingest.VideoMetricObservation{{ID: "auto-e", ObjectID: "real", ObservedAt: time.Now().Add(-48 * time.Hour), PublishedAt: time.Now().Add(-5 * 24 * time.Hour)}}},
	})
	q := insight.DataCenterQuery{HubUserID: "alice", DeviceCount: 2, ContextSource: "collection"}
	state, _, err := srv.inspectOperations(context.Background(), q, true, true)
	if err != nil || len(state.Runs) != 1 || state.Runs[0].Trigger != "collection" || state.Runs[0].AIStatus != "failed" || runner.called != 1 {
		t.Fatalf("automatic fallback not persisted: %+v %v", state, err)
	}
	state, _, err = srv.inspectOperations(context.Background(), q, true, true)
	if err != nil || runner.called != 1 || len(state.Proposals) != 1 || len(store.curations) != 0 {
		t.Fatal("repeated AI or automatic curation")
	}
	other, err := srv.dataCenterService.Operations(context.Background(), insight.DataCenterQuery{HubUserID: "bob"})
	if err != nil || len(other.Runs) != 0 {
		t.Fatal("cross workspace automatic inspection")
	}
}

func TestOperationsManualEmptyEvidenceDoesNotCallAI(t *testing.T) {
	runner := &aiAnalysisRunnerStub{}
	srv := newAIAnalysisTestServer(t, runner, insightdemo.NewAIAnalysisRepository())
	store := &apiAgentStore{states: map[string]insight.OperationsState{}, curations: map[string]insight.VideoCuration{}}
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{CurationRepository: store, WorkspaceContextResolver: srv.contextResolver, DataProvider: apiDataCenterProvider{}})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, operationsAgentRoute+"/inspect?hub_user_id=alice", strings.NewReader(`{"analyze":true}`)))
	if rec.Code != 200 || runner.called != 0 || !strings.Contains(rec.Body.String(), "insufficient_evidence") {
		t.Fatalf("empty AI request: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPublishedCollectionTriggersOperationsAndNewEvidenceAnalysis(t *testing.T) {
	runner := &aiAnalysisRunnerStub{execution: insightai.ModuleExecution{RunID: "auto-analysis", Status: "completed", InputFingerprint: "sha256:auto", RunAt: time.Now(), Result: json.RawMessage(`{"summary":"based on evidence"}`)}}
	repo := insightdemo.NewAIAnalysisRepository()
	srv := newAIAnalysisTestServer(t, runner, repo)
	store := &apiAgentStore{states: map[string]insight.OperationsState{}, curations: map[string]insight.VideoCuration{}}
	observations := []ingest.VideoMetricObservation{{ID: "event", ObjectID: "real", ObservedAt: time.Now().Add(-48 * time.Hour), PublishedAt: time.Now().Add(-5 * 24 * time.Hour)}}
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{CurationRepository: store, WorkspaceContextResolver: srv.contextResolver, DataProvider: apiDataCenterProvider{observations: observations}})
	completed := make(chan error, 4)
	worker := newOperationsAutomation(context.Background(), func(ctx context.Context, q insight.DataCenterQuery) error {
		_, _, err := srv.inspectOperations(ctx, q, true, true)
		completed <- err
		return err
	}, nil)
	defer worker.Close()
	readModel := insight.NewReadModelRefreshService(insight.ReadModelRefreshServiceConfig{Repository: insightdemo.NewDashboardRepository(), Materializer: insight.NewMaterializer(insight.Dashboard{})})
	readModel.SetAfterRefresh(worker.Notify)
	w, err := srv.contextResolver.Resolve(context.Background(), workspace.ContextQuery{HubUserID: "alice", DeviceCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 3; iteration++ {
		if iteration == 2 {
			observations[0].ObservedAt = observations[0].ObservedAt.Add(time.Minute)
		}
		if _, err := readModel.MaterializeDashboard(context.Background(), w, ingest.Summary{TotalEvents: 1, LatestEventAt: time.Now()}, "jobs"); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-completed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("publication did not trigger operations")
		}
	}
	if runner.called != 2 {
		t.Fatalf("expected two distinct evidence calls, got %d", runner.called)
	}
	state, err := srv.dataCenterService.Operations(context.Background(), insight.DataCenterQuery{HubUserID: "alice", DeviceCount: 2})
	if err != nil || len(state.Runs) != 2 || state.Runs[0].AIStatus != "completed" || state.Runs[0].Trigger != "collection" || len(store.curations) != 0 {
		t.Fatalf("bad automatic result: %+v %v", state, err)
	}
	if _, found, err := repo.Get(context.Background(), w.Workspace.ID, "auto-analysis"); err != nil || !found {
		t.Fatal("analysis not persisted under source workspace")
	}
}
func TestOperationsRoutePersistsRuleFallbackAndRejectsForgedDecisions(t *testing.T) {
	runner := &aiAnalysisRunnerStub{err: errors.New("model unavailable")}
	srv := newAIAnalysisTestServer(t, runner, insightdemo.NewAIAnalysisRepository())
	store := &apiAgentStore{states: map[string]insight.OperationsState{}, curations: map[string]insight.VideoCuration{}}
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{CurationRepository: store, WorkspaceContextResolver: srv.contextResolver, DataProvider: apiDataCenterProvider{observations: []ingest.VideoMetricObservation{{ID: "e", ObjectID: "real", ObservedAt: time.Now().Add(-48 * time.Hour), PublishedAt: time.Now().Add(-5 * 24 * time.Hour), Title: "真实视频"}}}})
	call := func(path, user, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/operations-agent/"+path+"?hub_user_id="+user, strings.NewReader(body)))
		return rec
	}
	rec := call("inspect", "alice", `{"analyze":true}`)
	if rec.Code != 200 {
		t.Fatalf("inspect %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		State insight.OperationsState `json:"state"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &response)
	if len(response.State.Runs) != 1 || response.State.Runs[0].AIStatus != "failed" || len(response.State.Proposals) != 1 {
		t.Fatalf("false AI success: %s", rec.Body.String())
	}
	if response.State.Proposals[0].Action != "refresh" {
		t.Fatal("stale video curated")
	}
	rec = call("inspect", "alice", `{"analyze":true}`)
	if rec.Code != 200 || runner.called != 1 {
		t.Fatal("unchanged input repeatedly billed")
	}
	id := response.State.Proposals[0].ID
	rec = call("apply", "alice", `{"ids":["`+id+`"],"action":"selected"}`)
	if rec.Code != 400 {
		t.Fatal("client chose arbitrary action")
	}
	rec = call("dismiss", "bob", `{"ids":["`+id+`"]}`)
	if rec.Code != 404 {
		t.Fatalf("cross workspace decision: %d %s", rec.Code, rec.Body.String())
	}
	rec = call("refresh", "alice", `{"ids":["`+id+`"]}`)
	if rec.Code != 200 {
		t.Fatalf("refresh result: %d %s", rec.Code, rec.Body.String())
	}
	state, _ := srv.dataCenterService.Operations(context.Background(), insight.DataCenterQuery{HubUserID: "alice"})
	p := state.Proposals[0]
	if p.Status != "pending" || len(p.Events) != 1 || p.Events[0].Status != "failed" {
		t.Fatalf("collector failure hidden: %+v", p)
	}
}

func TestOperationsAIInputContainsEvidenceButNoHumanNotes(t *testing.T) {
	run := insight.OperationsRun{ID: "run", Fingerprint: "evidence-hash", Scope: "全部视频"}
	input := operationsAIInput(run, []insight.OperationsProposal{{RunID: "run", VideoID: "video:a", Title: "公开标题", PublishedAt: time.Date(2022, 2, 4, 12, 0, 0, 0, time.UTC), Action: "historical", Before: insight.VideoCuration{Note: "private-human-note", Tags: []string{"private-tag"}}}})
	raw, _ := json.Marshal(input.Variables)
	if strings.Contains(string(raw), "private-") || !strings.Contains(string(raw), "公开标题") {
		t.Fatalf("invalid evidence boundary: %s", raw)
	}
	if !strings.Contains(string(raw), `"publishedAt":"2022-02-04`) || !strings.Contains(string(raw), `"ruleSuggestion":"historical"`) || !strings.Contains(string(raw), "不能推荐为当前热门") {
		t.Fatalf("AI lacks temporal context: %s", raw)
	}
	profile, _ := prompt.DefaultProfile(prompt.KeyContentAnalysis)
	values := map[string]string{}
	for key, value := range input.Variables {
		values[key] = string(value)
	}
	rendered, err := prompt.Render(profile.UserPromptTemplate, values)
	if err != nil || !strings.Contains(rendered, "公开标题") || !strings.Contains(rendered, "2022-02-04") || strings.Contains(rendered, "private-") {
		t.Fatalf("rendered prompt lost evidence: %s %v", rendered, err)
	}
}

func TestOperationsAIEvidenceUsesActualObservationWindowWithoutZeroQueryBounds(t *testing.T) {
	now := time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)
	speed := 100.0
	p := insight.OperationsProposal{RunID: "run", VideoID: "video:grow", PublishedAt: now.Add(-48 * time.Hour), ObservedAt: now, GrowthWindow: insight.VideoGrowthWindow{BaselineObservedAt: now.Add(-time.Hour), LatestObservedAt: now}, Performance: insight.VideoPerformance{GrowthPerHour: &speed}}
	input := operationsAIInput(insight.OperationsRun{ID: "run", CreatedAt: now}, []insight.OperationsProposal{p})
	data := string(input.Variables["data_json"])
	if strings.Contains(data, "0001-") || !strings.Contains(data, `"baselineObservedAt":"2026-10-19T05:00:00Z"`) {
		t.Fatalf("invalid AI growth window: %s", data)
	}
	p.GrowthWindow = insight.VideoGrowthWindow{}
	input = operationsAIInput(insight.OperationsRun{ID: "run", CreatedAt: now}, []insight.OperationsProposal{p})
	if strings.Contains(string(input.Variables["data_json"]), "growthWindow") {
		t.Fatal("absent window invented as year-one dates")
	}
}
