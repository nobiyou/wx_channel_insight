package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWXCommentExportClientExportTranslatesJSAPIContextMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/comment/export" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":500,"message":"API error (code=-70003): JSAPI_JSONPARSE_FAILED"}`))
	}))
	defer server.Close()

	client := WXCommentExportClient{BaseURL: server.URL, Client: server.Client()}
	_, err := client.Export(context.Background(), WXCommentExportRequest{
		ObjectID: "oid-1",
		NonceID:  "nid-1",
		Title:    "ctx mismatch",
		Author:   "Insight Test",
	})
	if err == nil {
		t.Fatal("expected context mismatch error")
	}
	if !strings.Contains(err.Error(), "当前客户端上下文与保存的评论导出视频不匹配") {
		t.Fatalf("expected translated context mismatch guidance, got %v", err)
	}
}

func TestWXCommentExportClientExportPollsAcceptedJob(t *testing.T) {
	var statusCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/channels/feed/comment/export":
			if r.Method != http.MethodPost {
				t.Fatalf("export method = %s, want POST", r.Method)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"job_id":"job-async-1","status":"queued"}}`))
		case "/api/channels/feed/comment/export/status":
			statusCalls.Add(1)
			if r.URL.Query().Get("job_id") != "job-async-1" {
				t.Fatalf("job_id = %q", r.URL.Query().Get("job_id"))
			}
			result, _ := json.Marshal(map[string]any{
				"object_id":     "oid-async-1",
				"saved_path":    "C:\\tmp\\comment-async.json",
				"relative_path": `comment_data\\comment-async.json`,
				"title":         "async comment",
				"author":        "Insight Test",
				"source":        "finderGetCommentList",
			})
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"job_id":"job-async-1","status":"succeeded","result":` + string(result) + `}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := WXCommentExportClient{
		BaseURL:      server.URL,
		Client:       server.Client(),
		PollInterval: time.Millisecond,
		PollTimeout:  time.Second,
	}
	result, err := client.Export(context.Background(), WXCommentExportRequest{
		ObjectID: "oid-async-1",
		NonceID:  "nid-async-1",
		Title:    "async comment",
		Author:   "Insight Test",
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if result.ObjectID != "oid-async-1" || result.SavedPath != `C:\tmp\comment-async.json` {
		t.Fatalf("unexpected async export result: %+v", result)
	}
	if statusCalls.Load() == 0 {
		t.Fatal("expected accepted export job to be polled")
	}
}

func TestWXCommentExportClientExportReportsAsyncFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/channels/feed/comment/export" {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"job_id":"job-async-failed","status":"queued"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"job_id":"job-async-failed","status":"failed","error":"JSAPI_JSONPARSE_FAILED"}}`))
	}))
	defer server.Close()

	client := WXCommentExportClient{BaseURL: server.URL, Client: server.Client(), PollInterval: time.Millisecond, PollTimeout: time.Second}
	_, err := client.Export(context.Background(), WXCommentExportRequest{ObjectID: "oid-failed", NonceID: "nid-failed"})
	if err == nil || !strings.Contains(err.Error(), "当前客户端上下文与保存的评论导出视频不匹配") {
		t.Fatalf("expected translated async failure, got %v", err)
	}
}

type stubCommentListTransport struct {
	topLevel []map[string]any
	reported int
}

func (s stubCommentListTransport) CollectAllComments(context.Context, string, string) ([]map[string]any, int, error) {
	return s.topLevel, s.reported, nil
}

func TestWXCommentExportClientTransportWritesImportableExportFile(t *testing.T) {
	t.Chdir(t.TempDir())

	client := WXCommentExportClient{
		Transport: stubCommentListTransport{
			reported: 2,
			topLevel: []map[string]any{
				{
					"commentId":          "c1",
					"content":            "第一条评论",
					"nickname":           "用户A",
					"username":           "user_a",
					"createtime":         "1719914400",
					"likeCount":          3,
					"expandCommentCount": 0,
					"levelTwoComment":    []map[string]any{},
				},
			},
		},
	}
	result, err := client.Export(context.Background(), WXCommentExportRequest{
		ObjectID: "oid-edge-1",
		NonceID:  "nid-edge-1",
		Title:    "edge comment export",
		Author:   "Insight Test",
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	payload, err := LoadWXCommentExportFile(result.SavedPath)
	if err != nil {
		t.Fatalf("transport export should be importable, got %v", err)
	}
	if payload.ObjectID != "oid-edge-1" || payload.ObjectNonceID != "nid-edge-1" {
		t.Fatalf("unexpected identifiers in transport export: %+v", payload)
	}
	if payload.CountInfo.CommentCount != 1 || payload.OriginalCommentCount != 2 {
		t.Fatalf("unexpected comment counts in transport export: %+v", payload)
	}
	if strings.TrimSpace(payload.SavedAt) == "" {
		t.Fatalf("expected savedAt in transport export")
	}
}
