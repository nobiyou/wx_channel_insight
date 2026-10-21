package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadTrustedRuntimeEvidenceIgnoresRetiredProofFiles(t *testing.T) {
	verifyProof := map[string]any{
		"status": "success", "hubUserId": "hub:user:retired",
		"results": []map[string]any{{
			"name": "check-team-runtime-status", "exitCode": 0,
			"output": `{"hubBridgeMode":"trusted-header","demoContextEnabled":false}`,
		}},
	}
	for _, tc := range []struct {
		name  string
		path  string
		proof any
	}{
		{"verify", ".tmp-codex/verify-team-runtime-proof-latest.json", verifyProof},
		{"child-verify", ".tmp-codex/workbench/verify-team-runtime-child-latest.json", verifyProof},
		{"workbench-api", ".tmp-codex/workbench/team-runtime-api-latest.json", map[string]any{
			"status": "ready", "trustedBoundary": map[string]any{"mode": "trusted-only"},
		}},
		{"workbench-state", ".tmp-codex/workbench/latest-team-workbench.json", map[string]any{
			"status": "ready", "proxy": map[string]any{"hubUserId": "hub:user:retired"},
			"api": map[string]any{"trustedBoundary": map[string]any{"mode": "trusted-only"}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(tc.proof)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			if evidence := LoadTrustedRuntimeEvidence(root); evidence.State != "missing" || evidence.Source != "" {
				t.Fatalf("retired proof must not supply runtime evidence: %+v", evidence)
			}
			storedAt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			if _, err := SaveTrustedRuntimeEvidence(filepath.Join(root, "var", "trusted-runtime-evidence.json"), TrustedRuntimeEvidence{
				Source: "current-explicit-proof", VerifiedAt: storedAt,
			}); err != nil {
				t.Fatal(err)
			}
			if evidence := LoadTrustedRuntimeEvidence(root); evidence.Source != "current-explicit-proof" || !evidence.VerifiedAt.Equal(storedAt) {
				t.Fatalf("retired files must not override the standard evidence file: %+v", evidence)
			}
		})
	}
}
