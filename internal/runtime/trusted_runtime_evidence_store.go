package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const trustedRuntimeEvidenceStoreFile = "trusted-runtime-evidence.json"

func defaultTrustedRuntimeEvidenceStorePath(root string) string {
	resolvedRoot := resolveTrustedRuntimeEvidenceRoot(root)
	if resolvedRoot == "" {
		return ""
	}
	return filepath.Join(resolvedRoot, "var", trustedRuntimeEvidenceStoreFile)
}

func resolveTrustedRuntimeEvidenceOutputPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return ResolveStorePath(filepath.Join("var", trustedRuntimeEvidenceStoreFile))
	}
	return ResolveStorePath(path)
}

func loadStoredTrustedRuntimeEvidence(root string) (TrustedRuntimeEvidence, bool) {
	path := defaultTrustedRuntimeEvidenceStorePath(root)
	if path == "" {
		return TrustedRuntimeEvidence{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return TrustedRuntimeEvidence{}, false
	}

	var evidence TrustedRuntimeEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		return TrustedRuntimeEvidence{}, false
	}
	evidence = finalizeTrustedRuntimeEvidence(evidence)
	if !hasTrustedRuntimeEvidence(evidence) && !hasTrustedRuntimeMetadataEvidence(evidence) {
		return TrustedRuntimeEvidence{}, false
	}
	return evidence, true
}

func LoadTrustedRuntimeEvidenceFromVerifyProofPath(proofPath string) (TrustedRuntimeEvidence, error) {
	if strings.TrimSpace(proofPath) == "" {
		return TrustedRuntimeEvidence{}, fmt.Errorf("trusted runtime proof path is required")
	}

	resolvedProofPath, err := filepath.Abs(proofPath)
	if err != nil {
		return TrustedRuntimeEvidence{}, err
	}
	info, err := os.Stat(resolvedProofPath)
	if err != nil {
		return TrustedRuntimeEvidence{}, err
	}
	if info.IsDir() {
		return TrustedRuntimeEvidence{}, fmt.Errorf("trusted runtime proof path must be a file: %s", resolvedProofPath)
	}

	evidence, ok := loadVerifyTrustedRuntimeEvidence(resolvedProofPath, info.ModTime())
	if !ok {
		return TrustedRuntimeEvidence{}, fmt.Errorf("trusted runtime proof file is not a successful Docker runtime verification result: %s", resolvedProofPath)
	}
	return finalizeTrustedRuntimeEvidence(evidence), nil
}

func SaveTrustedRuntimeEvidence(path string, evidence TrustedRuntimeEvidence) (string, error) {
	resolvedPath, err := resolveTrustedRuntimeEvidenceOutputPath(path)
	if err != nil {
		return "", err
	}

	evidence = finalizeTrustedRuntimeEvidence(evidence)
	if !hasTrustedRuntimeEvidence(evidence) && !hasTrustedRuntimeMetadataEvidence(evidence) {
		return "", fmt.Errorf("trusted runtime evidence is empty")
	}

	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return "", err
	}
	tempPath := resolvedPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, resolvedPath); err != nil {
		return "", err
	}
	return resolvedPath, nil
}

func SaveTrustedRuntimeEvidenceFromVerifyProof(proofPath, outputPath string) (TrustedRuntimeEvidence, string, error) {
	evidence, err := LoadTrustedRuntimeEvidenceFromVerifyProofPath(proofPath)
	if err != nil {
		return TrustedRuntimeEvidence{}, "", err
	}
	resolvedOutputPath, err := SaveTrustedRuntimeEvidence(outputPath, evidence)
	if err != nil {
		return TrustedRuntimeEvidence{}, "", err
	}
	return evidence, resolvedOutputPath, nil
}
