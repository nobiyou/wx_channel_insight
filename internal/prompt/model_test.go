package prompt

import (
	"testing"
	"time"
)

func TestApplyPatchValidatesVariablesAndIncrementsVersion(t *testing.T) {
	settings := DefaultSettings()
	updated, err := ApplyPatch(settings, KeyContentAnalysis, ProfilePatch{
		SystemPrompt:       "自定义系统 Prompt",
		UserPromptTemplate: "输入：{{ data_json }}",
	}, time.Date(2026, 8, 18, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	profile := updated.Profiles[KeyContentAnalysis]
	if profile.Version != "v2" || profile.SystemPrompt != "自定义系统 Prompt" || profile.UpdatedAt.IsZero() {
		t.Fatalf("unexpected patched profile: %+v", profile)
	}

	_, err = ApplyPatch(settings, KeyContentAnalysis, ProfilePatch{
		SystemPrompt:       "系统",
		UserPromptTemplate: "{{unknown}}",
	}, time.Now())
	if err == nil {
		t.Fatal("expected unknown template variable to be rejected")
	}
}

func TestNormalizeSettingsFillsMissingProfilesWithoutChangingCustomText(t *testing.T) {
	settings := Settings{Profiles: map[string]Profile{
		KeyContentAnalysis: {
			Key:                KeyContentAnalysis,
			SystemPrompt:       "自定义",
			UserPromptTemplate: "{{data_json}}",
			Version:            "v4",
		},
	}}
	normalized := NormalizeSettings(settings)
	if normalized.Profiles[KeyContentAnalysis].SystemPrompt != "自定义" || normalized.Profiles[KeyContentAnalysis].Version != "v4" {
		t.Fatalf("custom profile was not preserved: %+v", normalized.Profiles[KeyContentAnalysis])
	}
	if _, ok := normalized.Profiles[KeyCommentInsights]; !ok {
		t.Fatal("expected missing profile to be filled")
	}
}

func TestRenderReplacesControlledVariables(t *testing.T) {
	rendered, err := Render("标题\n{{data_json}}\n{{ data_json }}", map[string]string{"data_json": "{\"title\":\"测试\"}"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if rendered != "标题\n{\"title\":\"测试\"}\n{\"title\":\"测试\"}" {
		t.Fatalf("unexpected rendered prompt: %q", rendered)
	}
}

func TestOnlyAnalyticalPromptsExecuteWhileHistoricalProfilesSurvive(t *testing.T) {
	if len(Descriptors()) != 4 {
		t.Fatalf("expected four analytical profiles, got %d", len(Descriptors()))
	}
	for _, key := range []string{KeyTopicAnalysis, KeyTopicScript, KeyDailyReport} {
		legacy := Profile{Key: key, SystemPrompt: "historical", UserPromptTemplate: "{{legacy_json}}", Version: "v9"}
		settings := Settings{Profiles: map[string]Profile{key: legacy}}
		normalized := NormalizeSettings(settings)
		if normalized.Profiles[key] != legacy {
			t.Fatalf("legacy profile changed: %+v", normalized.Profiles[key])
		}
		if err := ValidateSettings(normalized); err != nil {
			t.Fatal(err)
		}
		if _, ok := Descriptor(key); ok {
			t.Fatalf("retired profile is executable: %s", key)
		}
		if _, err := ApplyPatch(normalized, key, ProfilePatch{SystemPrompt: "x", UserPromptTemplate: "x"}, time.Now()); err == nil {
			t.Fatal("retired profile update accepted")
		}
	}
}
