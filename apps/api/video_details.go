package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

func (s *server) handleVideoDetail(w http.ResponseWriter, r *http.Request) {
	pathRemainder := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/read-models/videos/"), "/")
	pathParts := strings.Split(pathRemainder, "/")
	if len(pathParts) == 2 && pathParts[1] == "playback" {
		s.handleVideoPlaybackStatus(w, r, pathParts[0])
		return
	}
	if len(pathParts) == 3 && strings.EqualFold(pathParts[1], "comments") && strings.EqualFold(pathParts[2], "refresh") {
		if decoded, err := url.PathUnescape(pathParts[0]); err == nil {
			s.handleVideoCommentRefresh(w, r, strings.TrimSpace(decoded))
			return
		}
	}
	if len(pathParts) == 2 && strings.EqualFold(pathParts[1], "comments") {
		if decoded, err := url.PathUnescape(pathParts[0]); err == nil {
			s.handleVideoComments(w, r, strings.TrimSpace(decoded))
			return
		}
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ingest service is not configured"})
		return
	}

	objectID := pathRemainder
	if objectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "video object id is required"})
		return
	}
	if decoded, err := url.PathUnescape(objectID); err == nil {
		objectID = strings.TrimSpace(decoded)
	}

	reqContext := hubContextFromRequest(r)
	observations, fromEvents, err := s.savedVideoObservations(r.Context(), ingest.SummaryQuery{
		HubUserID:   reqContext.HubUserID,
		DeviceCount: reqContext.DeviceCount,
	}, objectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	detail, found := insight.BuildVideoDetail(objectID, observations, nil)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "video was not found"})
		return
	}
	detail.Playback = insight.VideoPlayback{Status: "deferred", Message: "视频预览独立加载，历史分析可直接查看。"}
	if fromEvents {
		detail.DataBasis = "来自已保存的视频快照事件；曲线仅包含实际保存的时刻。"
	}

	writeJSON(w, http.StatusOK, insightapi.NewVideoDetailResponse(detail))
}

func (s *server) handleVideoPlaybackStatus(w http.ResponseWriter, r *http.Request, objectID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ingest service is not configured"})
		return
	}
	if decoded, err := url.PathUnescape(objectID); err == nil {
		objectID = strings.TrimSpace(decoded)
	}
	requestContext := hubContextFromRequest(r)
	observations, _, err := s.savedVideoObservations(r.Context(), ingest.SummaryQuery{HubUserID: requestContext.HubUserID, DeviceCount: requestContext.DeviceCount}, objectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if _, found := insight.BuildVideoDetail(objectID, observations, nil); !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "video was not found"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	profile, err := s.fetchVideoPlaybackProfile(ctx, objectID, videoObjectNonceID(objectID, observations))
	playback := insightapi.VideoPlaybackResponse{Status: "unavailable", FetchedAt: time.Now().UTC().Format(time.RFC3339), Message: "当前无法获取视频预览，请检查采集器连接后重试。历史数据分析仍可用。"}
	if err == nil && strings.TrimSpace(profile.VideoURL) != "" {
		playback.Status = "ready"
		playback.URL = buildVideoPlaybackURL(objectID, profile.ObjectNonceID)
		playback.Message = ""
	}
	writeJSON(w, http.StatusOK, playback)
}

func (s *server) savedVideoObservations(ctx context.Context, query ingest.SummaryQuery, objectID string) ([]ingest.VideoMetricObservation, bool, error) {
	observations, err := s.ingestService.ListVideoMetricObservations(ctx, query)
	if err != nil {
		return nil, false, err
	}
	if _, found := insight.BuildVideoDetail(objectID, observations, nil); found {
		return observations, false, nil
	}
	events, err := s.ingestService.ListEvents(ctx, query)
	if err != nil {
		return nil, false, err
	}
	for _, event := range events {
		if event.EventType == "video_snapshot" && strings.TrimSpace(event.ObjectID) == objectID {
			observations = append(observations, ingest.VideoObservationFromEvent(event))
		}
	}
	return observations, true, nil
}

func (s *server) handleVideoComments(w http.ResponseWriter, r *http.Request, objectID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ingest service is not configured"})
		return
	}
	objectID = strings.TrimSpace(objectID)
	if objectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "video object id is required"})
		return
	}
	page, err := parseAccountPageParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "video comments " + err.Error()})
		return
	}
	query := hubContextFromRequest(r)
	comments, err := s.ingestService.ListRawComments(r.Context(), ingest.SummaryQuery{
		HubUserID: query.HubUserID, DeviceCount: query.DeviceCount,
	}, ingest.RawCommentQuery{
		ObjectID: objectID, Keyword: r.URL.Query().Get("q"), Page: page.page, PageSize: page.pageSize,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

func (s *server) handleVideoCommentRefresh(w http.ResponseWriter, r *http.Request, objectID string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.ingestService == nil || s.accountService == nil || s.accountMonitor == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "comment refresh services are not configured"})
		return
	}
	if strings.TrimSpace(objectID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "video object id is required"})
		return
	}
	reqContext := hubContextFromRequest(r)
	observations, err := s.ingestService.ListLatestVideoMetricObservations(r.Context(), ingest.SummaryQuery{
		HubUserID: reqContext.HubUserID, DeviceCount: reqContext.DeviceCount,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var item ingest.WXFeedListItem
	for _, observation := range observations {
		if strings.TrimSpace(observation.ObjectID) != objectID {
			continue
		}
		item = ingest.WXFeedListItem{
			ID: observation.ObjectID, ObjectNonceID: observation.ObjectNonceID,
			Title: observation.Title, ChannelName: observation.ChannelName,
			ChannelUsername: observation.ChannelUsername, CommentCount: observation.CommentCount,
		}
		break
	}
	if strings.TrimSpace(item.ID) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "video observation was not found"})
		return
	}
	if strings.TrimSpace(item.ChannelUsername) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "video observation has no account identity"})
		return
	}
	account, err := s.accountService.FindByChannelUsername(r.Context(), insight.AccountQuery{
		HubUserID: reqContext.HubUserID, DeviceCount: reqContext.DeviceCount, ContextSource: reqContext.ContextSource,
	}, item.ChannelUsername)
	if err != nil {
		writeAccountError(w, errors.New("视频所属账号尚未进入账户主数据"))
		return
	}
	result, runErr := s.accountMonitor.RefreshVideoComments(r.Context(), ingest.SummaryQuery{
		HubUserID: reqContext.HubUserID, DeviceCount: reqContext.DeviceCount,
	}, account.ID, item)
	if runErr != nil && result.Status == monitoring.RunStatusFailed {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": runErr.Error(), "status": result.Status, "errors": result.Errors})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"videoObjectId": objectID, "accountId": account.ID, "status": result.Status,
		"attempted": result.Attempted, "succeeded": result.Succeeded, "failed": result.Failed,
		"skipped": result.Skipped, "errors": result.Errors,
	})
}

func videoObjectNonceID(objectID string, observations []ingest.VideoMetricObservation) string {
	for _, observation := range observations {
		if strings.TrimSpace(observation.ObjectID) == objectID && strings.TrimSpace(observation.ObjectNonceID) != "" {
			return strings.TrimSpace(observation.ObjectNonceID)
		}
	}
	return ""
}

func buildVideoPlaybackURL(objectID, nonceID string) string {
	playbackURL := "/api/v1/read-models/videos/play/" + url.PathEscape(strings.TrimSpace(objectID))
	if nonceID = strings.TrimSpace(nonceID); nonceID != "" {
		playbackURL += "?nonce_id=" + url.QueryEscape(nonceID)
	}
	return playbackURL
}
