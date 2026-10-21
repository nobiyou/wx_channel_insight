package wxchannel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type recordingAPICaller struct {
	key  string
	body any
	data json.RawMessage
	err  error
}

func (c *recordingAPICaller) CallAPI(_ context.Context, _ string, key string, body any, _ time.Duration) (json.RawMessage, error) {
	c.key = key
	c.body = body
	return c.data, c.err
}

func TestEdgeDownloadClientStartsThroughEdgeAndPollsLocalRecord(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"video-1","videoId":"video-1","status":"completed","filePath":"C:/wx_channel/downloads/作者/视频.mp4","relativePath":"作者/视频.mp4","fileSize":2048}}`))
	}))
	defer server.Close()

	caller := &recordingAPICaller{data: json.RawMessage(`{"success":true,"started":true}`)}
	statusClient := NewDownloadClient(server.URL, "local-secret", server.Client())
	statusClient.PollInterval = time.Millisecond
	statusClient.PollTimeout = time.Second
	client := NewEdgeDownloadClient(caller, statusClient, "edge-1")

	record, err := client.StartAndWait(context.Background(), DownloadRequest{
		VideoURL: "https://cdn.example/video.mp4?sig=temporary",
		VideoID:  "video-1",
		Title:    "视频",
		Author:   "作者",
		Key:      "decrypt-key",
	})
	if err != nil {
		t.Fatalf("StartAndWait() error = %v", err)
	}
	if caller.key != "key:channels:download_video" {
		t.Fatalf("caller key = %q, want key:channels:download_video", caller.key)
	}
	requestBody, ok := caller.body.(map[string]any)
	if !ok || requestBody["videoUrl"] != "https://cdn.example/video.mp4?sig=temporary" || requestBody["videoId"] != "video-1" || requestBody["key"] != "decrypt-key" {
		t.Fatalf("unexpected edge download body: %#v", caller.body)
	}
	if record.RelativePath != "作者/视频.mp4" || polls < 2 {
		t.Fatalf("unexpected record=%+v polls=%d", record, polls)
	}
}

func TestEdgeDownloadClientRejectsFrontendDownloadError(t *testing.T) {
	caller := &recordingAPICaller{data: json.RawMessage(`{"errCode":500,"errMsg":"download failed","response":{"success":false}}`)}
	client := NewEdgeDownloadClient(caller, NewDownloadClient("http://127.0.0.1:1", "", nil), "edge-1")

	_, err := client.StartAndWait(context.Background(), DownloadRequest{
		VideoURL: "https://cdn.example/video.mp4",
		VideoID:  "video-1",
	})
	if err == nil || err.Error() != "edge download rejected: download failed" {
		t.Fatalf("error = %v, want frontend rejection", err)
	}
}
