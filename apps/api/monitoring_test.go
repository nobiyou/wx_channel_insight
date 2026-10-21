package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	"github.com/nobiyou/wx_channel_insight/internal/monitoringapi"
)

type monitoringAPIRunner struct {
	options []monitoring.OperatorRunOptions
}

func (r *monitoringAPIRunner) RunMonitoringTask(_ context.Context, options monitoring.OperatorRunOptions) (monitoring.OperatorRunResult, error) {
	r.options = append(r.options, options)
	return monitoring.OperatorRunResult{
		Status: "success", FinalStatus: "success", Attempted: 1, Succeeded: 1,
		OperatorRunID: "operator-api-1", FinishedAt: time.Date(2026, 8, 9, 17, 0, 1, 0, time.UTC),
	}, nil
}

type blockingMonitoringRunner struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingMonitoringRunner) RunMonitoringTask(ctx context.Context, _ monitoring.OperatorRunOptions) (monitoring.OperatorRunResult, error) {
	r.once.Do(func() { close(r.started) })
	select {
	case <-r.release:
		return monitoring.OperatorRunResult{
			Status: "success", FinalStatus: "success", Attempted: 1, Succeeded: 1,
			OperatorRunID: "operator-blocking-1", FinishedAt: time.Date(2026, 8, 9, 17, 0, 1, 0, time.UTC),
		}, nil
	case <-ctx.Done():
		return monitoring.OperatorRunResult{}, ctx.Err()
	}
}

func TestMonitoringTaskHTTPContract(t *testing.T) {
	repo := monitoring.NewMemoryRepository()
	service, err := monitoring.NewService(repo, monitoring.ServiceConfig{
		DefaultHubUserID: "operator-api",
		Now:              func() time.Time { return time.Date(2026, 8, 9, 17, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("new monitoring service: %v", err)
	}
	runner := &monitoringAPIRunner{}
	service.SetOperatorRunner(runner)
	srv := &server{monitoringService: service}

	create := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/tasks?hub_user_id=operator-api", bytes.NewBufferString(`{
  "name":"API selected task",
  "scopeMode":"selected",
  "interval":"5m",
  "targets":[{"source":"wx_feed_list","targetKey":"account-1"}]
}`))
	createResponse := httptest.NewRecorder()
	srv.handleMonitoringTasks(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create task status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	var created monitoringapi.TaskResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created task: %v", err)
	}
	if created.ID == "" || created.Status != monitoring.StatusEnabled || len(created.Targets) != 1 {
		t.Fatalf("unexpected created task: %+v", created)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/monitoring/tasks?hub_user_id=operator-api", nil)
	listResponse := httptest.NewRecorder()
	srv.handleMonitoringTasks(listResponse, list)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list task status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var listed monitoringapi.TasksResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode task list: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != created.ID {
		t.Fatalf("unexpected task list: %+v", listed)
	}

	runRequest := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/tasks/"+created.ID+"/run?hub_user_id=operator-api", nil)
	runResponse := httptest.NewRecorder()
	srv.handleMonitoringTasks(runResponse, runRequest)
	if runResponse.Code != http.StatusOK {
		t.Fatalf("run task status=%d body=%s", runResponse.Code, runResponse.Body.String())
	}
	var runPayload struct {
		Run monitoringapi.TaskRunResponse `json:"run"`
	}
	if err := json.Unmarshal(runResponse.Body.Bytes(), &runPayload); err != nil {
		t.Fatalf("decode task run: %v", err)
	}
	if runPayload.Run.Status != monitoring.RunStatusSuccess || runPayload.Run.OperatorRunID != "operator-api-1" {
		t.Fatalf("unexpected task run: %+v", runPayload.Run)
	}
	if len(runner.options) != 1 || len(runner.options[0].TargetRefs) != 1 {
		t.Fatalf("expected selected target passed to runner: %+v", runner.options)
	}

	historyRequest := httptest.NewRequest(http.MethodGet, "/api/v1/monitoring/tasks/"+created.ID+"/runs?hub_user_id=operator-api", nil)
	historyResponse := httptest.NewRecorder()
	srv.handleMonitoringTasks(historyResponse, historyRequest)
	if historyResponse.Code != http.StatusOK || !bytes.Contains(historyResponse.Body.Bytes(), []byte("operator-api-1")) {
		t.Fatalf("unexpected history response status=%d body=%s", historyResponse.Code, historyResponse.Body.String())
	}
}

func TestMonitoringTaskHTTPIsolatesUsersAndSupportsPauseArchive(t *testing.T) {
	service, err := monitoring.NewService(monitoring.NewMemoryRepository(), monitoring.ServiceConfig{
		DefaultHubUserID: "operator-api",
		Now:              func() time.Time { return time.Date(2026, 8, 9, 17, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("new monitoring service: %v", err)
	}
	handler := (&server{monitoringService: service}).routes()

	create := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/tasks?hub_user_id=operator-a", bytes.NewBufferString(`{
  "name":"private task",
  "scopeMode":"all_saved_bindings",
  "interval":"5m"
}`))
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create task status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	var created monitoringapi.TaskResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created task: %v", err)
	}

	otherUser := httptest.NewRequest(http.MethodGet, "/api/v1/monitoring/tasks/"+created.ID+"?hub_user_id=operator-b", nil)
	otherUserResponse := httptest.NewRecorder()
	handler.ServeHTTP(otherUserResponse, otherUser)
	if otherUserResponse.Code != http.StatusNotFound {
		t.Fatalf("expected cross-user task lookup to be hidden, status=%d body=%s", otherUserResponse.Code, otherUserResponse.Body.String())
	}

	patch := httptest.NewRequest(http.MethodPatch, "/api/v1/monitoring/tasks/"+created.ID+"?hub_user_id=operator-a", bytes.NewBufferString(`{"status":"paused"}`))
	patchResponse := httptest.NewRecorder()
	handler.ServeHTTP(patchResponse, patch)
	if patchResponse.Code != http.StatusOK || !bytes.Contains(patchResponse.Body.Bytes(), []byte(`"status":"paused"`)) {
		t.Fatalf("pause task status=%d body=%s", patchResponse.Code, patchResponse.Body.String())
	}

	delete := httptest.NewRequest(http.MethodDelete, "/api/v1/monitoring/tasks/"+created.ID+"?hub_user_id=operator-a", nil)
	deleteResponse := httptest.NewRecorder()
	handler.ServeHTTP(deleteResponse, delete)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("archive task status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}

	archived := httptest.NewRequest(http.MethodGet, "/api/v1/monitoring/tasks?hub_user_id=operator-a&includeArchived=true", nil)
	archivedResponse := httptest.NewRecorder()
	handler.ServeHTTP(archivedResponse, archived)
	if archivedResponse.Code != http.StatusOK || !bytes.Contains(archivedResponse.Body.Bytes(), []byte(`"status":"archived"`)) {
		t.Fatalf("archived task lookup status=%d body=%s", archivedResponse.Code, archivedResponse.Body.String())
	}
}

func TestMonitoringSchedulerTickDoesNotRunSameTaskConcurrently(t *testing.T) {
	now := time.Date(2026, 8, 9, 17, 0, 0, 0, time.UTC)
	service, err := monitoring.NewService(monitoring.NewMemoryRepository(), monitoring.ServiceConfig{
		DefaultHubUserID: "operator-api",
		Now:              func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new monitoring service: %v", err)
	}
	created, err := service.CreateTask(context.Background(), "operator-api", monitoring.Task{
		Name:      "due task",
		ScopeMode: monitoring.ScopeAllSavedBindings,
		Interval:  5 * time.Minute,
		NextRunAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create due task: %v", err)
	}
	runner := &blockingMonitoringRunner{started: make(chan struct{}), release: make(chan struct{})}
	service.SetOperatorRunner(runner)
	handler := (&server{monitoringService: service}).routes()

	firstResponse := httptest.NewRecorder()
	firstDone := make(chan struct{})
	go func() {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/scheduler/tick?hub_user_id=operator-api", nil)
		handler.ServeHTTP(firstResponse, request)
		close(firstDone)
	}()
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("first scheduler tick did not start the monitoring runner")
	}

	secondRequest := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/scheduler/tick?hub_user_id=operator-api", nil)
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusOK || !strings.Contains(secondResponse.Body.String(), `"status":"idle"`) {
		t.Fatalf("expected concurrent scheduler tick to be idle, status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}

	close(runner.release)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first scheduler tick did not finish after runner release")
	}
	if firstResponse.Code != http.StatusOK || !strings.Contains(firstResponse.Body.String(), `"status":"completed"`) {
		t.Fatalf("unexpected first scheduler tick status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	if task, err := service.GetTask(context.Background(), "operator-api", created.ID); err != nil || task.LastStatus != monitoring.RunStatusSuccess {
		t.Fatalf("expected one completed task run, task=%#v err=%v", task, err)
	}
}

func TestMonitoringTaskMutationMethodsAreAllowedByCORS(t *testing.T) {
	handler := (&server{corsOrigins: []string{"http://127.0.0.1:5176"}}).routes()
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		request := httptest.NewRequest(http.MethodOptions, "/api/v1/monitoring/tasks/task-1", nil)
		request.Header.Set("Origin", "http://127.0.0.1:5176")
		request.Header.Set("Access-Control-Request-Method", method)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("preflight %s status=%d body=%s", method, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), method) {
			t.Fatalf("preflight %s missing allowed method: %q", method, response.Header().Get("Access-Control-Allow-Methods"))
		}
	}
}
