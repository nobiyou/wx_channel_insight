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
)

const dataCenterRoutePrefix = "/api/v1/read-models/data-center/"

func (s *server) handleDataCenter(w http.ResponseWriter, r *http.Request) {
	if s.dataCenterService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "data center service is not configured"})
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, dataCenterRoutePrefix), "/"), "/")
	if len(parts) == 1 && strings.TrimSpace(parts[0]) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "data center entity is required"})
		return
	}
	entity, ok := insight.NormalizeDataCenterEntity(parts[0])
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported data center entity"})
		return
	}
	query, err := parseDataCenterQuery(r, entity)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	switch {
	case len(parts) == 3 && parts[2] == "curation" && entity == insight.DataCenterEntityVideos && r.Method == http.MethodGet:
		id, err := url.PathUnescape(parts[1])
		if err != nil || !strings.HasPrefix(id, "video:") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video id"})
			return
		}
		item, err := s.dataCenterService.GetVideoCuration(r.Context(), query, id)
		if err != nil {
			writeDataCenterError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	case len(parts) == 2 && parts[1] == "curation" && entity == insight.DataCenterEntityVideos && r.Method == http.MethodPost:
		var patch insight.VideoCurationPatch
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&patch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video curation request: " + err.Error()})
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video curation request: expected one JSON object"})
			return
		}
		actor := hubContextFromRequest(r).HubUserID
		if actor == "" {
			actor = "insight:operator:local"
		}
		items, err := s.dataCenterService.CurateVideos(r.Context(), query, patch, actor)
		if err != nil {
			writeDataCenterError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "updated": len(items)})
	case len(parts) == 1 && r.Method == http.MethodGet:
		payload, err := s.dataCenterService.Query(r.Context(), query)
		if err != nil {
			writeDataCenterError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, insightapi.NewDataCenterPageResponse(payload))
	case len(parts) == 2 && r.Method == http.MethodDelete:
		entityID, err := url.PathUnescape(parts[1])
		if err != nil || strings.TrimSpace(entityID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid data center entity id"})
			return
		}
		hub := hubContextFromRequest(r)
		actor := strings.TrimSpace(hub.HubUserID)
		if actor == "" {
			actor = "insight:operator:local"
		}
		payload, err := s.dataCenterService.SoftDelete(r.Context(), query, entityID, actor)
		if err != nil {
			writeDataCenterError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, insightapi.NewDataCenterMutationResponse(payload))
	case len(parts) == 3 && r.Method == http.MethodGet && strings.EqualFold(parts[2], "history"):
		entityID, err := url.PathUnescape(parts[1])
		if err != nil || strings.TrimSpace(entityID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid data center entity id"})
			return
		}
		payload, err := s.dataCenterService.History(r.Context(), query, entityID)
		if err != nil {
			writeDataCenterError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, insightapi.NewDataCenterHistoryResponse(payload))
	case len(parts) == 3 && r.Method == http.MethodPost && strings.EqualFold(parts[2], "restore"):
		entityID, err := url.PathUnescape(parts[1])
		if err != nil || strings.TrimSpace(entityID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid data center entity id"})
			return
		}
		payload, err := s.dataCenterService.Restore(r.Context(), query, entityID)
		if err != nil {
			writeDataCenterError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, insightapi.NewDataCenterMutationResponse(payload))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "data center method or route is not allowed"})
	}
}

func parseDataCenterQuery(r *http.Request, entity insight.DataCenterEntity) (insight.DataCenterQuery, error) {
	values := r.URL.Query()
	page, err := parseDataCenterInt(values.Get("page"), 1)
	if err != nil {
		return insight.DataCenterQuery{}, errors.New("data center page must be a positive integer")
	}
	pageSize, err := parseDataCenterInt(values.Get("pageSize"), 20)
	if err != nil {
		return insight.DataCenterQuery{}, errors.New("data center pageSize must be 20 or 50")
	}
	from, err := parseDataCenterTime(values.Get("from"), false)
	if err != nil {
		return insight.DataCenterQuery{}, errors.New("data center from must be an RFC3339 timestamp or YYYY-MM-DD")
	}
	to, err := parseDataCenterTime(values.Get("to"), true)
	if err != nil {
		return insight.DataCenterQuery{}, errors.New("data center to must be an RFC3339 timestamp or YYYY-MM-DD")
	}
	publishedFrom, err := parseDataCenterTime(values.Get("publishedFrom"), false)
	if err != nil {
		return insight.DataCenterQuery{}, errors.New("发布时间起始日期无效")
	}
	publishedTo, err := parseDataCenterTime(values.Get("publishedTo"), true)
	if err != nil {
		return insight.DataCenterQuery{}, errors.New("发布时间截止日期无效")
	}
	if !publishedFrom.IsZero() && !publishedTo.IsZero() && publishedFrom.After(publishedTo) {
		return insight.DataCenterQuery{}, errors.New("发布时间起始日期不能晚于截止日期")
	}
	compareBy := values.Get("compareBy")
	if compareBy != "" && compareBy != "account" && compareBy != "keyword" {
		return insight.DataCenterQuery{}, errors.New("对比维度仅支持账号或关键词")
	}
	includeArchived := false
	if raw := strings.TrimSpace(values.Get("includeArchived")); raw != "" {
		includeArchived, err = strconv.ParseBool(raw)
		if err != nil {
			return insight.DataCenterQuery{}, errors.New("data center includeArchived must be boolean")
		}
	}
	hub := hubContextFromRequest(r)
	var minInteractions, minLikes *int
	for name, target := range map[string]**int{"minInteractions": &minInteractions, "minLikes": &minLikes} {
		if raw := values.Get(name); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				return insight.DataCenterQuery{}, errors.New(name + " must be a non-negative integer")
			}
			*target = &value
		}
	}
	freshOnly, momentumOnly, hasKeyword, refresh, recentOnly := false, false, false, false, false
	for name, target := range map[string]*bool{"freshOnly": &freshOnly, "momentumOnly": &momentumOnly, "hasKeyword": &hasKeyword, "refresh": &refresh, "recentOnly": &recentOnly} {
		if raw := values.Get(name); raw != "" {
			value, err := strconv.ParseBool(raw)
			if err != nil {
				return insight.DataCenterQuery{}, errors.New(name + " must be boolean")
			}
			*target = value
		}
	}
	return insight.DataCenterQuery{
		Refresh:    refresh,
		RecentOnly: recentOnly,
		HasKeyword: hasKeyword,
		Curation:   values.Get("curation"), Performance: values.Get("performance"), MinInteractions: minInteractions, MinLikes: minLikes, FreshOnly: freshOnly, MomentumOnly: momentumOnly, Tag: values.Get("tag"),
		PublishedFrom: publishedFrom, PublishedTo: publishedTo, CompareBy: compareBy,
		HubUserID: hub.HubUserID, DeviceCount: hub.DeviceCount, ContextSource: hub.ContextSource,
		Entity: entity, Keyword: values.Get("q"), MonitorKeyword: values.Get("monitorKeyword"), From: from, To: to, Source: values.Get("source"), Account: values.Get("account"),
		Page: page, PageSize: pageSize, Sort: values.Get("sort"), Direction: values.Get("direction"), IncludeArchived: includeArchived,
	}, nil
}

func parseDataCenterInt(raw string, fallback int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return 0, err
	}
	return value, nil
}

func parseDataCenterTime(raw string, endOfDay bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if len(raw) == len("2006-01-02") {
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return time.Time{}, err
		}
		if endOfDay {
			return value.Add(24*time.Hour - time.Nanosecond), nil
		}
		return value, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func writeDataCenterError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, insight.ErrDataCenterNotFound):
		status = http.StatusNotFound
	case errors.Is(err, insight.ErrDataCenterServiceUnavailable), errors.Is(err, insight.ErrDataCenterExclusionStore), errors.Is(err, insight.ErrVideoCurationStore):
		status = http.StatusServiceUnavailable
	case strings.Contains(err.Error(), "unsupported"), strings.Contains(err.Error(), "not supported"), strings.Contains(err.Error(), "must"), strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), "invalid"):
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
