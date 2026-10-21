package main

import (
	"context"
	"net/http"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

type schedulerPoll struct {
	LastSeen    time.Time
	ActiveSince time.Time
	Active      int
}
type monitoringHealth struct {
	SchedulerStatus string                      `json:"schedulerStatus"`
	LastHeartbeatAt time.Time                   `json:"lastHeartbeatAt"`
	CheckedAt       time.Time                   `json:"checkedAt"`
	EnabledTasks    int                         `json:"enabledTasks"`
	OverdueTasks    int                         `json:"overdueTasks"`
	StaleTasks      int                         `json:"staleTasks"`
	FailedTasks     int                         `json:"failedTasks"`
	Coverage        []ingest.CollectionProgress `json:"coverage"`
}

func buildMonitoringHealth(tasks []monitoring.Task, poll schedulerPoll, now time.Time) monitoringHealth {
	health := monitoringHealth{SchedulerStatus: "unknown", LastHeartbeatAt: poll.LastSeen, CheckedAt: now, Coverage: []ingest.CollectionProgress{}}
	if !poll.LastSeen.IsZero() {
		health.SchedulerStatus = "offline"
		if now.Sub(poll.LastSeen) <= 90*time.Second {
			health.SchedulerStatus = "online"
		}
	}
	if poll.Active > 0 && now.Sub(poll.ActiveSince) <= 15*time.Minute {
		health.SchedulerStatus = "running"
	}
	for _, task := range tasks {
		if task.Status != monitoring.StatusEnabled {
			continue
		}
		health.EnabledTasks++
		if task.LastStatus != monitoring.RunStatusRunning && !task.NextRunAt.IsZero() && now.Sub(task.NextRunAt) > 90*time.Second {
			health.OverdueTasks++
		}
		window := 2 * task.Interval
		if window < 2*time.Minute {
			window = 2 * time.Minute
		}
		if task.LastSuccessAt.IsZero() || now.Sub(task.LastSuccessAt) > window {
			health.StaleTasks++
		}
		if task.LastStatus == monitoring.RunStatusFailed || task.LastStatus == monitoring.RunStatusPartial || task.LastStatus == monitoring.RunStatusAbandoned {
			health.FailedTasks++
		}
	}
	return health
}

func (s *server) beginSchedulerPoll(owner string) func() {
	s.schedulerMu.Lock()
	if s.schedulerPolls == nil {
		s.schedulerPolls = map[string]schedulerPoll{}
	}
	poll := s.schedulerPolls[owner]
	poll.LastSeen = time.Now().UTC()
	if poll.Active == 0 {
		poll.ActiveSince = poll.LastSeen
	}
	poll.Active++
	s.schedulerPolls[owner] = poll
	s.schedulerMu.Unlock()
	return func() {
		s.schedulerMu.Lock()
		defer s.schedulerMu.Unlock()
		poll := s.schedulerPolls[owner]
		poll.Active--
		poll.LastSeen = time.Now().UTC()
		s.schedulerPolls[owner] = poll
	}
}

type collectionProgressReader interface {
	ListCollectionProgress(context.Context, ingest.SummaryQuery) ([]ingest.CollectionProgress, error)
}

func (s *server) handleMonitoringHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.monitoringService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "monitoring service is not configured"})
		return
	}
	hub, err := s.resolveMonitoringContext(r)
	if err != nil {
		writeMonitoringError(w, err)
		return
	}
	tasks, err := s.monitoringService.ListTasks(r.Context(), hub.HubUserID, false)
	if err != nil {
		writeMonitoringError(w, err)
		return
	}
	s.schedulerMu.Lock()
	poll := s.schedulerPolls[hub.HubUserID]
	s.schedulerMu.Unlock()
	health := buildMonitoringHealth(tasks, poll, time.Now().UTC())
	if reader, ok := s.ingestService.(collectionProgressReader); ok {
		health.Coverage, err = reader.ListCollectionProgress(r.Context(), ingest.SummaryQuery{HubUserID: hub.HubUserID, DeviceCount: hub.DeviceCount})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "采集覆盖范围读取失败：" + err.Error()})
			return
		}
		for i := range health.Coverage {
			health.Coverage[i].NextMarker = ""
		}
	}
	writeJSON(w, http.StatusOK, health)
}
