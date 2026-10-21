package insightai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/prompt"
)

func TestModuleDescriptorListsAllActiveModules(t *testing.T) {
	descriptors := ModuleDescriptors()
	if len(descriptors) != 4 {
		t.Fatalf("expected four module descriptors, got %d", len(descriptors))
	}
	for _, descriptor := range descriptors {
		if descriptor.SchemaName == "" || len(descriptor.Variables) == 0 || len(descriptor.RequiredVariables) == 0 {
			t.Fatalf("descriptor is incomplete: %+v", descriptor)
		}
		profile, ok := prompt.DefaultProfile(descriptor.Key)
		if !ok || profile.IntegrationState != prompt.IntegrationActive || !profile.Enabled {
			t.Fatalf("module profile is not active: %+v", descriptor)
		}
	}
}

func TestNormalizeModuleInputRejectsUnknownVariablesAndEmptyEvidence(t *testing.T) {
	valid := json.RawMessage("{\"items\":[{\"title\":\"视频\"}]}")
	if _, err := NormalizeModuleInput(ModuleInput{
		Module:   prompt.KeyContentAnalysis,
		ScopeKey: "dashboard",
		Window:   "24h",
		Variables: map[string]json.RawMessage{
			"data_json":  valid,
			"video_json": valid,
			"unknown":    valid,
		},
	}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected unknown variable rejection, got %v", err)
	}
	if _, err := NormalizeModuleInput(ModuleInput{
		Module:   prompt.KeyTopicTrends,
		ScopeKey: "dashboard",
		Window:   "24h",
		Variables: map[string]json.RawMessage{
			"data_json": json.RawMessage("[]"),
		},
	}); err == nil || !strings.Contains(err.Error(), "insufficient") {
		t.Fatalf("expected empty evidence rejection, got %v", err)
	}
}

func TestAnalyzeModuleUsesModulePromptAndStructuredSchema(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request["model"] != "gpt-5.6" {
			t.Fatalf("unexpected model: %+v", request["model"])
		}
		textConfig, _ := request["text"].(map[string]any)
		format, _ := textConfig["format"].(map[string]any)
		if format["type"] != "json_schema" || format["name"] != "content_analysis" {
			t.Fatalf("unexpected structured request: %+v", format)
		}
		if !strings.Contains(request["input"].(string), "内容分析") {
			t.Fatalf("expected rendered content prompt, got %q", request["input"])
		}
		output := map[string]any{
			"summary":         "内容概览",
			"strengths":       []string{"结构清晰"},
			"weaknesses":      []string{"证据较少"},
			"audience":        "运营人员",
			"recommendations": []string{"补充案例"},
			"risks":           []string{"样本有限"},
			"evidenceRefs":    []string{"video-1"},
		}
		outputJSON, _ := json.Marshal(output)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": string(outputJSON)})
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-5.6",
		Timeout: time.Second,
	}, server.Client())
	execution, err := client.AnalyzeModule(context.Background(), ModuleInput{
		Module:   prompt.KeyContentAnalysis,
		ScopeKey: "dashboard",
		Window:   "24h",
		Variables: map[string]json.RawMessage{
			"data_json":  json.RawMessage("{\"title\":\"内容分析\",\"window\":\"24h\"}"),
			"video_json": json.RawMessage("{\"id\":\"video-1\",\"title\":\"标题\"}"),
		},
	})
	if err != nil {
		t.Fatalf("analyze module: %v", err)
	}
	if execution.Status != ModuleStatusCompleted || execution.RunID == "" || !execution.ModelAttempted {
		t.Fatalf("unexpected execution: %+v", execution)
	}
	if execution.PromptVersion != prompt.DefaultVersion || !strings.HasPrefix(execution.InputFingerprint, "sha256:") {
		t.Fatalf("unexpected trace metadata: %+v", execution)
	}
	if !strings.Contains(string(execution.Result), "内容概览") || requestCount.Load() != 1 {
		t.Fatalf("expected validated result and one request, got %+v count=%d", execution, requestCount.Load())
	}
}

func TestAnalyzeModuleHonorsDisabledPromptWithoutProviderCall(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
	}))
	defer server.Close()

	repository := prompt.NewMemoryRepository()
	settings := prompt.DefaultSettings()
	profile := settings.Profiles[prompt.KeyTopicTrends]
	profile.Enabled = false
	settings.Profiles[prompt.KeyTopicTrends] = profile
	if err := repository.Save(context.Background(), "owner-a", settings); err != nil {
		t.Fatalf("save prompt settings: %v", err)
	}
	client := NewClient(Config{
		APIKey:           "test-key",
		BaseURL:          server.URL,
		Timeout:          time.Second,
		PromptRepository: repository,
		PromptOwnerID:    "owner-a",
	}, server.Client())
	execution, err := client.AnalyzeModule(WithPromptOwner(context.Background(), "owner-a"), ModuleInput{
		Module:   prompt.KeyTopicTrends,
		ScopeKey: "dashboard",
		Window:   "24h",
		Variables: map[string]json.RawMessage{
			"data_json": json.RawMessage("{\"topics\":[{\"keyword\":\"AI\"}]}"),
		},
	})
	if err != nil {
		t.Fatalf("disabled prompt should be an execution result: %v", err)
	}
	if execution.Status != ModuleStatusDisabled || requestCount.Load() != 0 {
		t.Fatalf("unexpected disabled execution: %+v count=%d", execution, requestCount.Load())
	}
}

func TestAnalyzeModuleRejectsInvalidStructuredResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": "{\"summary\":\"缺字段\"}"})
	}))
	defer server.Close()
	client := NewClient(Config{APIKey: "test-key", BaseURL: server.URL, Timeout: time.Second}, server.Client())
	execution, err := client.AnalyzeModule(context.Background(), ModuleInput{
		Module:   prompt.KeyCommentInsights,
		ScopeKey: "dashboard",
		Window:   "24h",
		Variables: map[string]json.RawMessage{
			"data_json":     json.RawMessage("{\"window\":\"24h\"}"),
			"comments_json": json.RawMessage("{\"items\":[{\"theme\":\"体验\"}]}"),
		},
	})
	if err != nil {
		t.Fatalf("invalid provider output should be an execution result: %v", err)
	}
	if execution.Status != ModuleStatusFallback || !strings.Contains(execution.FallbackReason, "missing result field") {
		t.Fatalf("unexpected invalid-result execution: %+v", execution)
	}
}

func TestRetiredModulesCannotCallProvider(t *testing.T) {
	client := NewClient(Config{}, nil)
	for _, module := range []string{prompt.KeyTopicAnalysis, prompt.KeyTopicScript, prompt.KeyDailyReport} {
		_, err := client.AnalyzeModule(context.Background(), ModuleInput{Module: module, ScopeKey: "old", Window: "all"})
		if err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("retired module %s: %v", module, err)
		}
	}
}
