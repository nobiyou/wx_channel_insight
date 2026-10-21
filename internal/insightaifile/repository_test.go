package insightaifile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insightai"
)

func TestRepositoryPersistsAndIsolatesOwners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai-service-settings.json")
	codec, err := insightai.NewSecretCodec("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("new codec: %v", err)
	}
	repo, err := NewRepository(path, codec)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	a := insightai.StoredServiceSettings{Provider: insightai.ProviderOpenAI, BaseURL: insightai.DefaultBaseURL, Model: "model-a", TimeoutSeconds: 45, APIKey: "key-a", VerificationStatus: insightai.VerificationNever}
	b := insightai.StoredServiceSettings{Provider: insightai.ProviderOpenAI, BaseURL: insightai.DefaultBaseURL, Model: "model-b", TimeoutSeconds: 60, VerificationStatus: insightai.VerificationPending}
	if err := repo.Save(context.Background(), "owner-a", a); err != nil {
		t.Fatalf("save owner a: %v", err)
	}
	if err := repo.Save(context.Background(), "owner-b", b); err != nil {
		t.Fatalf("save owner b: %v", err)
	}
	reloaded, err := NewRepository(path, codec)
	if err != nil {
		t.Fatalf("reload repository: %v", err)
	}
	gotA, ok, err := reloaded.Load(context.Background(), "owner-a")
	if err != nil || !ok || gotA.Model != "model-a" || gotA.APIKey != "key-a" {
		t.Fatalf("unexpected owner a: %+v %v %v", gotA, ok, err)
	}
	gotB, ok, err := reloaded.Load(context.Background(), "owner-b")
	if err != nil || !ok || gotB.Model != "model-b" {
		t.Fatalf("unexpected owner b: %+v %v %v", gotB, ok, err)
	}
}

func TestRepositoryPersistsServerOnlyKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai-service-settings.json")
	codec, err := insightai.NewSecretCodec("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("new codec: %v", err)
	}
	repo, err := NewRepository(path, codec)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	settings := insightai.StoredServiceSettings{
		Provider:       insightai.ProviderOpenAI,
		BaseURL:        insightai.DefaultBaseURL,
		Model:          insightai.DefaultModel,
		TimeoutSeconds: 45,
		APIKey:         "server-only-secret",
	}
	if err := repo.Save(context.Background(), "owner-a", settings); err != nil {
		t.Fatalf("save key: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(raw), "server-only-secret") || !strings.Contains(string(raw), `"apiKeyCiphertext"`) || !strings.Contains(string(raw), `"apiKeyNonce"`) {
		t.Fatalf("expected encrypted JSON store without plaintext key, err=%v", err)
	}
}
