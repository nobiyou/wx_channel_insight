package promptfile

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/prompt"
)

func TestRepositoryRoundTripsPerOwner(t *testing.T) {
	repo, err := NewRepository(filepath.Join(t.TempDir(), "prompt-settings.json"))
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	ctx := context.Background()
	settings, err := repo.Load(ctx, "owner-a")
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	settings, err = prompt.ApplyPatch(settings, prompt.KeyContentAnalysis, prompt.ProfilePatch{
		SystemPrompt:       "custom system",
		UserPromptTemplate: "{{data_json}}",
	}, time.Now())
	if err != nil {
		t.Fatalf("patch settings: %v", err)
	}
	settings.Profiles[prompt.KeyTopicAnalysis] = prompt.Profile{Key:prompt.KeyTopicAnalysis,SystemPrompt:"historical",UserPromptTemplate:"{{legacy_json}}",Version:"v8"}
	if err := repo.Save(ctx, "owner-a", settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	reloaded, err := NewRepository(repo.path)
	if err != nil {
		t.Fatalf("reload repository: %v", err)
	}
	got, err := reloaded.Load(ctx, "owner-a")
	if err != nil || got.Profiles[prompt.KeyContentAnalysis].SystemPrompt != "custom system" {
		t.Fatalf("unexpected owner settings: %+v err=%v", got, err)
	}
	if got.Profiles[prompt.KeyTopicAnalysis].SystemPrompt != "historical" { t.Fatal("archived prompt lost during save and reload") }
	other, err := reloaded.Load(ctx, "owner-b")
	if err != nil || other.Profiles[prompt.KeyContentAnalysis].SystemPrompt == "custom system" {
		t.Fatalf("expected owner isolation, got %+v err=%v", other, err)
	}
}
