package media

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var videoHTTPClient = &http.Client{
	Timeout: 0,
	Transport: &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	},
	CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		_, err := validateTargetURL(req.URL.String())
		return err
	},
}

// ServeVideo proxies the request-time wx_channel media URL and decrypts the
// encrypted prefix when decodeKey is present. The target URL is never exposed
// to the browser, so temporary URLs and keys stay on the API side.
func ServeVideo(w http.ResponseWriter, r *http.Request, targetURL, decryptKey string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	validatedURL, err := validateTargetURL(targetURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	key, needsDecryption, err := parseDecryptKey(decryptKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	upstreamRequest, err := http.NewRequestWithContext(r.Context(), r.Method, validatedURL.String(), nil)
	if err != nil {
		http.Error(w, "invalid video URL", http.StatusBadGateway)
		return
	}
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		upstreamRequest.Header.Set("Range", rangeHeader)
	}

	upstreamResponse, err := videoHTTPClient.Do(upstreamRequest)
	if err != nil {
		http.Error(w, "failed to fetch video", http.StatusBadGateway)
		return
	}
	defer upstreamResponse.Body.Close()

	for name, values := range upstreamResponse.Header {
		w.Header()[name] = append([]string(nil), values...)
	}
	if w.Header().Get("Accept-Ranges") == "" {
		w.Header().Set("Accept-Ranges", "bytes")
	}

	w.WriteHeader(upstreamResponse.StatusCode)
	if r.Method == http.MethodHead {
		return
	}

	var reader io.Reader = upstreamResponse.Body
	if needsDecryption && upstreamResponse.StatusCode >= http.StatusOK && upstreamResponse.StatusCode < http.StatusMultipleChoices {
		// wx_channel encrypts the logical first 128 KiB of the file. A Range
		// response starts at its Content-Range offset, so seed the keystream at
		// that logical position before decrypting the response body.
		reader = NewDecryptReader(upstreamResponse.Body, key, contentRangeStart(upstreamResponse.Header.Get("Content-Range")), EncryptedPrefixSize)
	}
	_, _ = io.Copy(w, reader)
}

func contentRangeStart(value string) uint64 {
	parts := strings.Fields(strings.TrimSpace(value))
	if len(parts) != 2 {
		return 0
	}
	dash := strings.IndexByte(parts[1], '-')
	if dash <= 0 {
		return 0
	}
	start, err := strconv.ParseUint(parts[1][:dash], 10, 64)
	if err != nil {
		return 0
	}
	return start
}

func parseDecryptKey(raw string) (uint64, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	key, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid decryption key")
	}
	return key, true, nil
}

func validateTargetURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("invalid video URL")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("video URL userinfo is not allowed")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported video URL scheme")
	}
	return parsed, nil
}
