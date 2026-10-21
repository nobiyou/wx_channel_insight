package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

const dailyReportsRoute = "/api/v1/reports"

type dailyReportSnapshotRequest struct {
	DataWindow string `json:"dataWindow"`
}

func (s *server) handleDailyReports(w http.ResponseWriter, r *http.Request) {
	if s.dailyReportRepo == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "daily report repository is not configured"})
		return
	}

	suffix := strings.TrimPrefix(r.URL.Path, dailyReportsRoute)
	suffix = strings.Trim(suffix, "/")
	switch {
	case suffix == "" && r.Method == http.MethodGet:
		s.handleDailyReportList(w, r)
	case (suffix == "" || suffix == "snapshot") && r.Method == http.MethodPost:
		s.handleDailyReportSnapshot(w, r)
	case suffix != "" && suffix != "snapshot" && r.Method == http.MethodGet:
		s.handleDailyReportDetail(w, r, suffix)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "daily report method or route is not allowed"})
	}
}

func (s *server) handleDailyReportList(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.dailyReportWorkspaceID(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	window := strings.TrimSpace(r.URL.Query().Get("window"))
	if window == "" {
		window = insight.TimeWindow72h
	}
	normalized, ok := insight.NormalizeTimeWindow(window)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": insight.ErrInvalidTimeWindow.Error()})
		return
	}
	window = normalized
	limit, err := dailyReportLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	items, err := s.dailyReportRepo.List(r.Context(), workspaceID, window, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewDailyReportsResponse(workspaceID, window, items))
}

func (s *server) handleDailyReportDetail(w http.ResponseWriter, r *http.Request, reportID string) {
	workspaceID, err := s.dailyReportWorkspaceID(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	decodedID, err := url.PathUnescape(reportID)
	if err != nil || strings.TrimSpace(decodedID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid daily report id"})
		return
	}
	report, found, err := s.dailyReportRepo.Get(r.Context(), workspaceID, decodedID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "daily report was not found"})
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewDailyReportResponse(report))
}

func (s *server) handleDailyReportSnapshot(w http.ResponseWriter, r *http.Request) {
	if s.dashboardService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dashboard service is not configured"})
		return
	}
	request := dailyReportSnapshotRequest{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	window := strings.TrimSpace(r.URL.Query().Get("window"))
	if window == "" {
		window = strings.TrimSpace(request.DataWindow)
	}
	if window == "" {
		window = insight.TimeWindow72h
	}
	normalizedWindow, ok := insight.NormalizeTimeWindow(window)
	if !ok || normalizedWindow == insight.TimeWindowAll {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "daily report data window must be 24h or 72h"})
		return
	}
	query, err := dashboardQueryFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	query.TimeWindow = normalizedWindow
	// Creating a report is an explicit refresh boundary. Use the full dashboard
	// path so a new ingest batch is materialized before the snapshot is saved.
	snapshot, err := s.dashboardService.GetDashboard(r.Context(), query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	workspaceID, err := s.dailyReportWorkspaceID(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(snapshot.WorkspaceBridge.WorkspaceID) != "" {
		workspaceID = snapshot.WorkspaceBridge.WorkspaceID
	}
	report, err := insight.NewDailyReport(workspaceID, insight.DailyReportDate(time.Now()), normalizedWindow, time.Now(), snapshot)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.dailyReportRepo.Upsert(r.Context(), report); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, insightapi.NewDailyReportResponse(report))
}

func (s *server) dailyReportWorkspaceID(r *http.Request) (string, error) {
	if s.contextResolver == nil {
		return "", nil
	}
	hub := hubContextFromRequest(r)
	resolved, err := s.contextResolver.Resolve(r.Context(), workspace.ContextQuery{
		HubUserID:     hub.HubUserID,
		DeviceCount:   hub.DeviceCount,
		ContextSource: hub.ContextSource,
	})
	if err != nil {
		return "", err
	}
	return resolved.Workspace.ID, nil
}

func dailyReportLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 30, nil
	}
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit < 1 || limit > 100 {
		return 0, errors.New("daily report limit must be between 1 and 100")
	}
	return limit, nil
}
