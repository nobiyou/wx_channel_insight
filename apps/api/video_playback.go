package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/media"
)

func (s *server) handleVideoPlayback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ingest service is not configured"})
		return
	}

	objectID := strings.TrimPrefix(r.URL.Path, "/api/v1/read-models/videos/play/")
	objectID = strings.Trim(objectID, "/")
	if decoded, err := url.PathUnescape(objectID); err == nil {
		objectID = strings.TrimSpace(decoded)
	}
	if objectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "video object id is required"})
		return
	}

	nonceID := strings.TrimSpace(r.URL.Query().Get("nonce_id"))
	if nonceID == "" {
		requestContext := hubContextFromRequest(r)
		observations, err := s.ingestService.ListVideoMetricObservations(r.Context(), ingest.SummaryQuery{
			HubUserID:   requestContext.HubUserID,
			DeviceCount: requestContext.DeviceCount,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		nonceID = videoObjectNonceID(objectID, observations)
	}

	profile, err := s.fetchVideoPlaybackProfile(r.Context(), objectID, nonceID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to refresh video playback source: " + err.Error()})
		return
	}
	if strings.TrimSpace(profile.VideoURL) == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "wx_channel returned no playable video URL"})
		return
	}

	media.ServeVideo(w, r, profile.VideoURL, profile.DecryptKey)
}

func (s *server) fetchVideoPlaybackProfile(ctx context.Context, objectID, nonceID string) (ingest.WXFeedListItem, error) {
	fetch := func(fetchCtx context.Context) (ingest.WXFeedListItem, error) {
		return s.ingestService.FetchWXVideoProfile(fetchCtx, objectID, nonceID)
	}
	if s.videoPlaybackProfiles == nil {
		return fetch(ctx)
	}
	return s.videoPlaybackProfiles.getOrFetch(ctx, objectID, nonceID, fetch)
}
