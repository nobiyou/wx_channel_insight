package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

func TestMonitoringHealthUsesDefaultWorkspaceHeartbeat(t *testing.T) {
	service, err := monitoring.NewService(monitoring.NewMemoryRepository(), monitoring.ServiceConfig{DefaultHubUserID: "hub:user:accounts"})
	if err != nil {
		t.Fatal(err)
	}
	srv := &server{monitoringService: service, contextResolver: fixedAccountContextResolver{workspaceID: "workspace"}}
	srv.beginSchedulerPoll("hub:user:accounts")()
	for _, test := range []struct{ owner, want string }{{"", "online"}, {"another-owner", "unknown"}} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitoring/health", nil)
		if test.owner != "" {
			req.Header.Set(headerHubUserID, test.owner)
		}
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, req)
		var health monitoringHealth
		if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || health.SchedulerStatus != test.want {
			t.Fatalf("owner %q: %d %s", test.owner, rec.Code, rec.Body.String())
		}
	}
}

func TestMonitoringHealthSeparatesSchedulerFromAPIAndPausedTasks(t *testing.T) {
	now := time.Now()
	tasks := []monitoring.Task{{Status: monitoring.StatusEnabled, Interval: time.Hour, NextRunAt: now.Add(-time.Hour), LastSuccessAt: now.Add(-3 * time.Hour), LastStatus: monitoring.RunStatusFailed}, {Status: monitoring.StatusPaused, NextRunAt: now.Add(-24 * time.Hour)}}
	health := buildMonitoringHealth(tasks, schedulerPoll{}, now)
	if health.SchedulerStatus != "unknown" || health.EnabledTasks != 1 || health.OverdueTasks != 1 || health.StaleTasks != 1 || health.FailedTasks != 1 {
		t.Fatalf("unseen scheduler: %+v", health)
	}
	if state := buildMonitoringHealth(tasks, schedulerPoll{LastSeen: now.Add(-2 * time.Minute)}, now).SchedulerStatus; state != "offline" {
		t.Fatal(state)
	}
	if state := buildMonitoringHealth(tasks, schedulerPoll{LastSeen: now.Add(-time.Second)}, now).SchedulerStatus; state != "online" {
		t.Fatal(state)
	}
	if state := buildMonitoringHealth(tasks, schedulerPoll{LastSeen: now.Add(-2 * time.Minute), ActiveSince: now.Add(-2 * time.Minute), Active: 1}, now).SchedulerStatus; state != "running" {
		t.Fatal(state)
	}
	if state := buildMonitoringHealth(tasks, schedulerPoll{LastSeen: now.Add(-16 * time.Minute), ActiveSince: now.Add(-16 * time.Minute), Active: 1}, now).SchedulerStatus; state != "offline" {
		t.Fatal(state)
	}
}

func TestSchedulerHeartbeatsAreScopedAndConcurrent(t *testing.T) {
	server := &server{}
	first := server.beginSchedulerPoll("a")
	second := server.beginSchedulerPoll("a")
	second()
	if server.schedulerPolls["a"].Active != 1 || !server.schedulerPolls["b"].LastSeen.IsZero() {
		t.Fatal("incorrect scheduler ownership or running count")
	}
	first()
	if server.schedulerPolls["a"].Active != 0 {
		t.Fatal("poll never completed")
	}
}
