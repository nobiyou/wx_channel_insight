package insightai

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNormalizeServiceSettingsDefaultsAndValidation(t *testing.T) {
	settings, err := NormalizeServiceSettings(ServiceSettings{})
	if err != nil {
		t.Fatalf("normalize defaults: %v", err)
	}
	if settings.Provider != ProviderOpenAI || settings.BaseURL != DefaultBaseURL || settings.Model != DefaultModel || settings.Timeout != 45*time.Second || settings.AnalysisEngine != AnalysisEngineOpenAI || settings.FallbackPolicy != FallbackPolicyRule {
		t.Fatalf("unexpected defaults: %+v", settings)
	}
	for _, invalid := range []ServiceSettings{
		{Provider: "anthropic"},
		{BaseURL: "not-a-url"},
		{Timeout: 2 * time.Second},
		{Timeout: 301 * time.Second},
		{AnalysisEngine: "unknown"},
		{FallbackPolicy: "unknown"},
	} {
		if _, err := NormalizeServiceSettings(invalid); err == nil {
			t.Fatalf("expected invalid settings to fail: %+v", invalid)
		}
	}
}

func TestSettingsFromStoredUsesServerOnlyKeyWithoutExposingSecretInMetadata(t *testing.T) {
	stored := StoredServiceSettings{
		Provider:            ProviderOpenAI,
		BaseURL:             DefaultBaseURL,
		Model:               DefaultModel,
		TimeoutSeconds:      45,
		APIKey:              "sk-test-secret",
		APIKeyFingerprint:   APIKeyFingerprint("sk-test-secret"),
		APIKeyHint:          APIKeyHint("sk-test-secret"),
		VerificationStatus:  VerificationPending,
		LastVerifyLatencyMS: 12,
	}
	settings, err := SettingsFromStored(stored)
	if err != nil {
		t.Fatalf("open stored: %v", err)
	}
	if settings.APIKey != "sk-test-secret" || settings.Source != ConfigSourceWorkspace {
		t.Fatalf("unexpected effective settings: %+v", settings)
	}
	metadata := StoredMetadata(stored)
	if metadata.APIKeyHint != "****cret" || metadata.APIKeyConfigured != true || strings.Contains(metadata.APIKeyHint, "sk-test") {
		t.Fatalf("metadata exposed secret: %+v", metadata)
	}
	effectiveMetadata, err := MetadataFromEffective(ServiceSettings{
		Provider:       ProviderOpenAI,
		BaseURL:        DefaultBaseURL,
		Model:          DefaultModel,
		Timeout:        45 * time.Second,
		APIKey:         "sk-test-secret",
		OwnerID:        "owner-a",
		AnalysisEngine: AnalysisEngineOpenAI,
		FallbackPolicy: FallbackPolicyRule,
		Source:         ConfigSourceWorkspace,
	})
	if err != nil || effectiveMetadata.OwnerID != "owner-a" || !effectiveMetadata.ModelAvailable || effectiveMetadata.ConfigurationIssue != "" {
		t.Fatalf("effective model availability metadata was not preserved: %+v err=%v", effectiveMetadata, err)
	}
}

func TestMemoryServiceSettingsRepositoryIsolatesOwnersAndClearsOnlyOneKey(t *testing.T) {
	repo := NewMemoryServiceSettingsRepository()
	a := StoredServiceSettings{Provider: ProviderOpenAI, APIKey: "secret-a"}
	b := StoredServiceSettings{Provider: ProviderOpenAI, APIKey: "secret-b"}
	if err := repo.Save(context.Background(), "owner-a", a); err != nil {
		t.Fatalf("save a: %v", err)
	}
	if err := repo.Save(context.Background(), "owner-b", b); err != nil {
		t.Fatalf("save b: %v", err)
	}
	if err := repo.ClearKey(context.Background(), "owner-a"); err != nil {
		t.Fatalf("clear a: %v", err)
	}
	cleared, ok, err := repo.Load(context.Background(), "owner-a")
	if err != nil || !ok || cleared.APIKeyHint != "" || cleared.APIKeyFingerprint != "" {
		t.Fatalf("owner a key was not cleared: %+v %v %v", cleared, ok, err)
	}
	unchanged, ok, err := repo.Load(context.Background(), "owner-b")
	if err != nil || !ok || unchanged.APIKeyHint != APIKeyHint("secret-b") {
		t.Fatalf("owner b changed unexpectedly: %+v %v %v", unchanged, ok, err)
	}
}
