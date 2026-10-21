package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestHandleRawCommentExportStreamsJSONAttachment(t *testing.T) {
	srv := newServer(nil, nil, stubIngestService{
		exportedJSON: `{"exportedAt":"2026-08-23T01:02:03Z","hubUserId":"hub:test","total":1,"items":[{"commentId":"c1","content":"评论"}]}`,
	}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/comments/export?q=%E8%AF%84%E8%AE%BA&objectId=video-1", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected json content type, got %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "insight-comments-") {
		t.Fatalf("expected attachment filename, got %q", rec.Header().Get("Content-Disposition"))
	}
	var payload struct {
		Total int `json:"total"`
		Items []struct {
			CommentID string `json:"commentId"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode export: %v body=%s", err, rec.Body.String())
	}
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Items[0].CommentID != "c1" {
		t.Fatalf("unexpected export payload: %+v", payload)
	}
}

func TestHandleRawCommentExportRejectsInvalidDate(t *testing.T) {
	srv := newServer(nil, nil, stubIngestService{}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/comments/export?from=not-a-date", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "comment export from") {
		t.Fatalf("expected invalid date response, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleRawCommentExportRequiresGet(t *testing.T) {
	srv := newServer(nil, nil, stubIngestService{}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/read-models/comments/export", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("expected GET-only export route, got status=%d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}
