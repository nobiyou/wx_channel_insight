package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/prompt"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestPromptSettingsAPIListsUpdatesAndResetsPerOwner(t *testing.T) {
	srv := newServerWithOperator(nil, nil, nil, nil, nil, nil, runtime.Status{OperatorIdentity: "insight:operator:local"})
	srv.promptRepository = prompt.NewMemoryRepository()
	handler := srv.routes()

	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/settings/prompts?hub_user_id=owner-a", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("list prompt settings: status=%d body=%s", get.Code, get.Body.String())
	}
	if strings.Contains(get.Body.String(), "OPENAI_API_KEY") {
		t.Fatal("prompt settings response must not contain the API key name")
	}

	updateBody := bytes.NewBufferString(`{"systemPrompt":"自定义系统提示","userPromptTemplate":"{{data_json}}"}`)
	update := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/v1/settings/prompts/content-analysis?hub_user_id=owner-a", updateBody)
	handler.ServeHTTP(update, updateRequest)
	if update.Code != http.StatusOK || !strings.Contains(update.Body.String(), "自定义系统提示") {
		t.Fatalf("update prompt settings: status=%d body=%s", update.Code, update.Body.String())
	}

	otherOwner := httptest.NewRecorder()
	handler.ServeHTTP(otherOwner, httptest.NewRequest(http.MethodGet, "/api/v1/settings/prompts?hub_user_id=owner-b", nil))
	if otherOwner.Code != http.StatusOK || strings.Contains(otherOwner.Body.String(), "自定义系统提示") {
		t.Fatalf("expected owner isolation: status=%d body=%s", otherOwner.Code, otherOwner.Body.String())
	}

	reset := httptest.NewRecorder()
	resetRequest := httptest.NewRequest(http.MethodPost, "/api/v1/settings/prompts/content-analysis/reset?hub_user_id=owner-a", nil)
	handler.ServeHTTP(reset, resetRequest)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset prompt settings: status=%d body=%s", reset.Code, reset.Body.String())
	}
	var payload promptSettingsResponse
	if err := json.Unmarshal(reset.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode reset response: %v", err)
	}
	if len(payload.Items) != 4 || payload.Items[0].SystemPrompt == "自定义系统提示" {
		t.Fatalf("unexpected reset response: %+v", payload)
	}
}

func TestPromptSettingsAPIRejectsUnknownTemplateVariable(t *testing.T) {
	srv := newServerWithOperator(nil, nil, nil, nil, nil, nil, runtime.Status{OperatorIdentity: "owner-a"})
	srv.promptRepository = prompt.NewMemoryRepository()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/settings/prompts/content-analysis?hub_user_id=owner-a", bytes.NewBufferString(`{"systemPrompt":"x","userPromptTemplate":"{{unknown}}"}`))
	srv.routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected validation error, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
