package wxchannel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDownloadClientStartAndWaitPollsCompletedRecord(t *testing.T) {
	var startBody map[string]interface{}
	var authValues []string
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authValues = append(authValues, r.Header.Get("X-Local-Auth"))
		if r.URL.Path == "/__wx_channels_api/download_video" {
			if err := json.NewDecoder(r.Body).Decode(&startBody); err != nil {
				t.Fatalf("decode start body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"started":true}`))
			return
		}
		if r.URL.Path != "/api/downloads/video-1" {
			http.NotFound(w, r)
			return
		}
		polls++
		if polls == 1 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"video-1","videoId":"video-1","title":"参考视频","status":"completed","filePath":"C:/wx_channel/downloads/作者/参考视频.mp4","relativePath":"作者/参考视频.mp4","fileSize":1024}}`))
	}))
	defer server.Close()

	client := NewDownloadClient(server.URL, "local-secret", server.Client())
	client.PollInterval = time.Millisecond
	client.PollTimeout = time.Second
	record, err := client.StartAndWait(context.Background(), DownloadRequest{
		VideoURL: "https://cdn.example/video.mp4?token=temporary",
		VideoID:  "video-1",
		Title:    "参考视频",
		Author:   "作者",
		Key:      "temporary-decrypt-key",
	})
	if err != nil {
		t.Fatalf("StartAndWait() error = %v", err)
	}
	if record.RelativePath != "作者/参考视频.mp4" || polls < 2 {
		t.Fatalf("unexpected record=%+v polls=%d", record, polls)
	}
	if startBody["videoUrl"] != "https://cdn.example/video.mp4?token=temporary" || startBody["key"] != "temporary-decrypt-key" {
		t.Fatalf("unexpected start body=%+v", startBody)
	}
	for _, value := range authValues {
		if value != "local-secret" {
			t.Fatalf("unexpected auth header %q", value)
		}
	}
}

func TestDownloadClientRequiresDownloadedPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "download_video") {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"status":"completed"}}`))
	}))
	defer server.Close()

	client := NewDownloadClient(server.URL, "", server.Client())
	client.PollInterval = time.Millisecond
	client.PollTimeout = time.Second
	_, err := client.StartAndWait(context.Background(), DownloadRequest{VideoURL: "https://cdn.example/video.mp4", VideoID: "video-1"})
	if err == nil || !strings.Contains(err.Error(), "relativePath or filePath") {
		t.Fatalf("error = %v, want downloaded path error", err)
	}
}

func TestDownloadClientAcceptsAbsoluteFilePathWithoutRelativePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "download_video") {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"status":"completed","filePath":"E:/wx_channel/release/v5.7.9/downloads/作者/视频.mp4"}}`))
	}))
	defer server.Close()

	client := NewDownloadClient(server.URL, "", server.Client())
	client.PollInterval = time.Millisecond
	client.PollTimeout = time.Second
	record, err := client.StartAndWait(context.Background(), DownloadRequest{VideoURL: "https://cdn.example/video.mp4", VideoID: "video-1"})
	if err != nil {
		t.Fatalf("StartAndWait() error = %v", err)
	}
	if record.RelativePath != "" || record.FilePath == "" {
		t.Fatalf("unexpected record=%+v", record)
	}
}
