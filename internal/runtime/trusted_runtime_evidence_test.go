package runtime

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadTrustedRuntimeEvidenceReadsStoredEvidence(t *testing.T) {
	root := t.TempDir()
	storedAt := time.Date(2026, 7, 7, 10, 30, 0, 0, time.UTC)
	outputPath := filepath.Join(root, "var", "trusted-runtime-evidence.json")

	_, err := SaveTrustedRuntimeEvidence(outputPath, TrustedRuntimeEvidence{
		VerifiedAt:             storedAt,
		Source:                 "stored-trusted-runtime-proof",
		HubUserID:              "hub:user:stored",
		APIURL:                 "http://127.0.0.1:18881",
		WebURL:                 "http://127.0.0.1:15176",
		TrustedBoundaryMode:    "trusted-only",
		APITrustedContextState: "verified",
		APIBrowserQueryState:   "blocked",
		WebTrustedContextState: "verified",
		WebBrowserQueryState:   "isolated",
		Notes:                  []string{"持久化 trusted runtime 证据。"},
	})
	if err != nil {
		t.Fatalf("save trusted runtime evidence failed: %v", err)
	}

	evidence := LoadTrustedRuntimeEvidence(root)
	if evidence.Source != "stored-trusted-runtime-proof" {
		t.Fatalf("expected stored evidence source, got %+v", evidence)
	}
	if !evidence.VerifiedAt.Equal(storedAt) {
		t.Fatalf("expected stored verifiedAt, got %+v", evidence)
	}
	if evidence.State != "verified" {
		t.Fatalf("expected stored evidence to be verified, got %+v", evidence)
	}
}
