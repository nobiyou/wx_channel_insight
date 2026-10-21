//go:build !windows

package ingest

func detectLocalWXFeedListProxyMismatch(baseURL string) *wxFeedListProxyMismatch {
	return nil
}
