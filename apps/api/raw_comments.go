package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func (s *server) handleRawCommentExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ingest service is not configured"})
		return
	}
	from, err := parseAccountTime(r.URL.Query().Get("from"), false)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "comment export from must be RFC3339 or YYYY-MM-DD"})
		return
	}
	to, err := parseAccountTime(r.URL.Query().Get("to"), true)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "comment export to must be RFC3339 or YYYY-MM-DD"})
		return
	}
	values := r.URL.Query()
	objectID := strings.TrimSpace(values.Get("objectId"))
	if objectID == "" {
		objectID = strings.TrimSpace(values.Get("object_id"))
	}
	accountID := strings.TrimSpace(values.Get("account"))
	if accountID == "" {
		accountID = strings.TrimSpace(values.Get("account_id"))
	}
	hub := hubContextFromRequest(r)
	filename := fmt.Sprintf("insight-comments-%s.json", time.Now().UTC().Format("20060102T150405Z"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	stream := &commentExportResponseWriter{ResponseWriter: w}
	if err := s.ingestService.ExportRawComments(r.Context(), ingest.SummaryQuery{
		HubUserID: hub.HubUserID, DeviceCount: hub.DeviceCount,
	}, ingest.RawCommentQuery{
		ObjectID: objectID, AccountID: accountID, Source: values.Get("source"), Keyword: values.Get("q"), From: from, To: to,
	}, stream); err != nil && !stream.wrote {
		w.Header().Del("Content-Disposition")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

type commentExportResponseWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *commentExportResponseWriter) WriteHeader(status int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *commentExportResponseWriter) Write(payload []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(payload)
}
