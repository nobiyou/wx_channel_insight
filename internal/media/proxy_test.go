package media

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestServeVideoDecryptsRangeAtLogicalOffset(t *testing.T) {
	plain := bytes.Repeat([]byte("video-data-"), 16000)
	key := uint64(987654321)
	ciphertext, err := io.ReadAll(NewDecryptReader(bytes.NewReader(plain), key, 0, EncryptedPrefixSize))
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=97-" {
			t.Fatalf("expected range forwarding, got %q", got)
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", "bytes 97-"+strconv.Itoa(len(ciphertext)-1)+"/"+strconv.Itoa(len(plain)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(ciphertext[97:])
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodGet, "/play", nil)
	req.Header.Set("Range", "bytes=97-")
	rec := httptest.NewRecorder()
	ServeVideo(rec, req, upstream.URL, strconv.FormatUint(key, 10))

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("expected content type to survive, got %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), plain[97:]) {
		t.Fatalf("proxy returned encrypted or misaligned video bytes")
	}
}
