package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

const metricFreshnessWindow = 10 * time.Minute

type metricRefreshState struct {
	running bool
	retryAt time.Time
}

type metricRefreshInput struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Force bool   `json:"force"`
}

// Reserve before calling the collector. A failure has a short cooldown, while
// successful freshness comes from persisted observations rather than memory.
func (s *server) reserveMetricRefresh(key string, now time.Time) (func(), bool) {
	s.metricRefreshMu.Lock()
	defer s.metricRefreshMu.Unlock()
	if s.metricRefreshes == nil {
		s.metricRefreshes = map[string]metricRefreshState{}
	}
	if state := s.metricRefreshes[key]; state.running || now.Before(state.retryAt) {
		return nil, false
	}
	for id, state := range s.metricRefreshes {
		if !state.running && !now.Before(state.retryAt) {
			delete(s.metricRefreshes, id)
		}
	}
	s.metricRefreshes[key] = metricRefreshState{running: true}
	return func() {
		s.metricRefreshMu.Lock()
		defer s.metricRefreshMu.Unlock()
		s.metricRefreshes[key] = metricRefreshState{retryAt: time.Now().Add(time.Minute)}
	}, true
}

func (s *server) handleMetricRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input metricRefreshInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil || strings.TrimSpace(input.ID) == "" || (input.Kind != "account" && input.Kind != "video") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind must be account or video and id is required"})
		return
	}
	s.refreshMetrics(w, r, input)
}

func (s *server) refreshMetrics(w http.ResponseWriter, r *http.Request, input metricRefreshInput) {
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "采集服务尚未配置"})
		return
	}
	hub, err := s.resolveMonitoringContext(r)
	if err != nil {
		writeMonitoringError(w, err)
		return
	}
	query := ingest.SummaryQuery{HubUserID: hub.HubUserID, DeviceCount: hub.DeviceCount}
	var observedAt time.Time
	var nonceID string
	if input.Kind == "account" {
		if s.accountService == nil || s.accountMonitor == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "账号采集服务尚未配置"})
			return
		}
		account, err := s.accountService.Get(r.Context(), accountQueryFromRequest(r), input.ID)
		if err != nil {
			writeAccountError(w, err)
			return
		}
		if account.IdentityStatus != insight.AccountIdentityVerified {
			writeAccountError(w, insight.ErrAccountIdentityIncomplete)
			return
		}
		items, err := s.ingestService.ListVideoMetricObservations(r.Context(), query)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		for _, item := range items {
			if item.ChannelUsername == account.ChannelUsername && item.Source == "wx_channel_feed_list" && item.ObservedAt.After(observedAt) {
				observedAt = item.ObservedAt
			}
		}
	} else {
		items, _, err := s.savedVideoObservations(r.Context(), query, input.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		found := false
		for _, item := range items {
			if item.ObjectID == input.ID {
				found = true
				if item.ObservedAt.After(observedAt) {
					observedAt = item.ObservedAt
					nonceID = item.ObjectNonceID
				}
			}
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "video was not found"})
			return
		}
	}
	if !input.Force && !observedAt.IsZero() && time.Since(observedAt) < metricFreshnessWindow {
		writeJSON(w, http.StatusOK, map[string]any{"status": "fresh", "observedAt": observedAt})
		return
	}
	release, reserved := s.reserveMetricRefresh(hub.HubUserID+":"+input.Kind+":"+input.ID, time.Now())
	if !reserved {
		writeJSON(w, http.StatusOK, map[string]string{"status": "cooldown", "message": "正在采集或处于短暂重试间隔，页面将自动更新"})
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if s.metricRefreshLocker != nil {
		unlock, acquired, err := s.metricRefreshLocker.TryAcquire(ctx, hub.HubUserID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "无法取得采集锁，请稍后重试"})
			return
		}
		if !acquired {
			writeJSON(w, http.StatusOK, map[string]string{"status": "busy", "message": "后台监控正在采集，稍后自动更新"})
			return
		}
		defer unlock()
	}
	response := map[string]string{"status": "updated", "message": "已采集最新指标"}
	if input.Kind == "account" {
		result, runErr := s.accountMonitor.RunTask(ctx, query, monitoring.TaskTarget{TargetKind: monitoring.TargetKindAccount, AccountID: input.ID}, monitoring.TaskTypeVideoList)
		err = runErr
		if err == nil && (result.Status == monitoring.RunStatusFailed || result.Status == monitoring.RunStatusSkipped) {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "账号实时采集未完成，已保留上次数据", "details": result.Errors})
			return
		}
		if err == nil && result.Status == monitoring.RunStatusPartial {
			response["collectionStatus"] = "partial"
			response["message"] = "已更新本轮指标，账号视频列表尚未全部采集"
			for _, detail := range result.Errors {
				if detail.Message != "" {
					response["message"] += "；" + detail.Message
				}
			}
		}
	} else {
		refresher, ok := s.ingestService.(interface {
			RefreshWXVideoMetrics(context.Context, ingest.SummaryQuery, string, string) (ingest.Summary, error)
		})
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "视频指标刷新尚未配置"})
			return
		}
		_, err = refresher.RefreshWXVideoMetrics(ctx, query, input.ID, nonceID)
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "实时采集未完成，已保留上次有效数据：" + err.Error()})
		return
	}
	// Facts are committed before materializing secondary summaries. A failed
	// summary refresh must not hide a successful metrics collection.
	if err := s.refreshDashboardAfterIngest(ctx, hub); err != nil {
		response["message"] += "；汇总视图稍后重试"
		response["summaryError"] = err.Error()
	}
	writeJSON(w, http.StatusOK, response)
}
