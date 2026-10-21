package insightai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServiceSettingsManagerSavesWorkspaceConfigAndFallsBackAfterClear(t *testing.T) {
	repo := NewMemoryServiceSettingsRepository()
	manager, err := NewServiceSettingsManager(repo, ServiceSettings{
		Provider: ProviderOpenAI,
		BaseURL:  "https://environment.example/v1",
		Model:    "environment-model",
		Timeout:  30 * time.Second,
		APIKey:   "environment-secret",
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	workspaceKey := "workspace-secret"
	metadata, err := manager.Save(context.Background(), "owner-a", ServiceSettingsPatch{
		Provider: ProviderOpenAI, BaseURL: "https://workspace.example/v1", Model: "workspace-model", TimeoutSeconds: 60, APIKey: &workspaceKey,
	}, time.Date(2026, 8, 18, 1, 2, 3, 0, time.UTC))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if metadata.Source != ConfigSourceWorkspace || metadata.APIKeyHint != "****cret" || metadata.VerificationStatus != VerificationPending || !metadata.APIKeyPersistenceAvailable {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	stored, ok, err := repo.Load(context.Background(), "owner-a")
	if err != nil || !ok || stored.APIKey != workspaceKey {
		t.Fatalf("workspace secret was not stored: %+v %v %v", stored, ok, err)
	}
	effective, err := manager.Resolve(context.Background(), "owner-a")
	if err != nil || effective.APIKey != workspaceKey || effective.Model != "workspace-model" {
		t.Fatalf("unexpected effective workspace config: %+v %v", effective, err)
	}
	cleared, err := manager.ClearKey(context.Background(), "owner-a")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared.Source != ConfigSourceEnvironment || cleared.APIKeyHint != "****cret" {
		t.Fatalf("expected environment key fallback after clear: %+v", cleared)
	}
}

func TestServiceSettingsManagerSavesKeyWithoutEncryptionConfig(t *testing.T) {
	manager, err := NewServiceSettingsManager(NewMemoryServiceSettingsRepository(), DefaultServiceSettings())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	apiKey := "secret"
	metadata, err := manager.Save(context.Background(), "owner-a", ServiceSettingsPatch{
		Provider: ProviderOpenAI, BaseURL: DefaultBaseURL, Model: DefaultModel, TimeoutSeconds: 45, APIKey: &apiKey,
	}, time.Now())
	if err != nil || !metadata.APIKeyPersistenceAvailable || !metadata.APIKeyConfigured {
		t.Fatalf("expected key to persist without encryption configuration, got metadata=%+v err=%v", metadata, err)
	}
}

func TestServiceSettingsManagerSaveAndVerifyPersistsSuccessfulVerification(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer workspace-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":"verification"}`))
	}))
	defer upstream.Close()

	manager, err := NewServiceSettingsManager(NewMemoryServiceSettingsRepository(), DefaultServiceSettings())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	apiKey := "workspace-secret"
	metadata, err := manager.SaveAndVerify(context.Background(), "owner-a", ServiceSettingsPatch{
		Provider: ProviderOpenAI, BaseURL: upstream.URL, Model: "verified-model", TimeoutSeconds: 5, APIKey: &apiKey,
	}, time.Now())
	if err != nil {
		t.Fatalf("save and verify: %v", err)
	}
	if metadata.VerificationStatus != VerificationVerified || metadata.LastVerifiedAt.IsZero() || metadata.LastVerifyLatencyMS < 0 {
		t.Fatalf("expected verified metadata, got %+v", metadata)
	}
	stored, ok, err := manager.repository.Load(context.Background(), "owner-a")
	if err != nil || !ok || stored.VerificationStatus != VerificationVerified || stored.LastVerifiedAt.IsZero() {
		t.Fatalf("expected verified state to persist, stored=%+v ok=%v err=%v", stored, ok, err)
	}
}
