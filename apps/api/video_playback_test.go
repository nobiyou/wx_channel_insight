package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/media"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

type countingVideoProfileIngestService struct {
	stubIngestService
	profileCalls int
}

func (s *countingVideoProfileIngestService) FetchWXVideoProfile(ctx context.Context, objectID, nonceID string) (ingest.WXFeedListItem, error) {
	s.profileCalls++
	return s.stubIngestService.FetchWXVideoProfile(ctx, objectID, nonceID)
}

func TestHandleVideoPlaybackRefreshesProfileAndDecryptsStream(t *testing.T) {
	plain := bytes.Repeat([]byte("playback-fixture-"), 10000)
	key := uint64(246813579)
	ciphertext, err := io.ReadAll(media.NewDecryptReader(bytes.NewReader(plain), key, 0, media.EncryptedPrefixSize))
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa(len(ciphertext)))
		_, _ = w.Write(ciphertext)
	}))
	defer upstream.Close()

	srv := newServer(nil, nil, stubIngestService{
		observations: []ingest.VideoMetricObservation{{ObjectID: "object-1", ObjectNonceID: "nonce-1"}},
		videoProfile: ingest.WXFeedListItem{
			ID: "object-1", ObjectNonceID: "nonce-1", VideoURL: upstream.URL,
			DecryptKey: strconv.FormatUint(key, 10),
		},
	}, nil, runtime.Status{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/play/object-1?nonce_id=nonce-1", nil)
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), plain) {
		t.Fatalf("expected decrypted stream body")
	}
	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("expected video content type, got %q", got)
	}
}

func TestHandleVideoPlaybackReusesProfileAcrossRangeRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=0-3" {
			t.Fatalf("expected range forwarding, got %q", got)
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 0-3/4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("test"))
	}))
	defer upstream.Close()

	ingestStub := &countingVideoProfileIngestService{
		stubIngestService: stubIngestService{
			videoProfile: ingest.WXFeedListItem{
				ID: "object-1", ObjectNonceID: "nonce-1", VideoURL: upstream.URL,
			},
		},
	}
	srv := newServer(nil, nil, ingestStub, nil, runtime.Status{})

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/play/object-1?nonce_id=nonce-1", nil)
		req.Header.Set("Range", "bytes=0-3")
		srv.routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusPartialContent || rec.Body.String() != "test" {
			t.Fatalf("unexpected range response %d %q", rec.Code, rec.Body.String())
		}
	}
	if ingestStub.profileCalls != 1 {
		t.Fatalf("expected one profile fetch for two range requests, got %d", ingestStub.profileCalls)
	}
}

func TestHandleVideoDetailWarmsPlaybackProfileCache(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("test"))
	}))
	defer upstream.Close()

	ingestStub := &countingVideoProfileIngestService{
		stubIngestService: stubIngestService{
			observations: []ingest.VideoMetricObservation{{
				ID: "observation-1", ObjectID: "object-1", ObjectNonceID: "nonce-1", ObservedAt: time.Now().UTC(), LikeCount: 1,
			}},
			videoProfile: ingest.WXFeedListItem{
				ID: "object-1", ObjectNonceID: "nonce-1", VideoURL: upstream.URL,
			},
		},
	}
	srv := newServer(nil, nil, ingestStub, nil, runtime.Status{})

	detailRec := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/object-1", nil)
	srv.routes().ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected detail response 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}

	playbackRec := httptest.NewRecorder()
	playbackReq := httptest.NewRequest(http.MethodGet, "/api/v1/read-models/videos/play/object-1?nonce_id=nonce-1", nil)
	srv.routes().ServeHTTP(playbackRec, playbackReq)
	if playbackRec.Code != http.StatusOK || playbackRec.Body.String() != "test" {
		t.Fatalf("unexpected warmed playback response %d %q", playbackRec.Code, playbackRec.Body.String())
	}
	if ingestStub.profileCalls != 1 {
		t.Fatalf("expected detail request to warm playback profile, got %d profile fetches", ingestStub.profileCalls)
	}
}
