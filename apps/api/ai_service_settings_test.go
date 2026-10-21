package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestAIServiceSettingsAPISavesMaskedConfigPerOwnerAndClearsKey(t *testing.T) {
	repo := insightai.NewMemoryServiceSettingsRepository()
	manager, err := insightai.NewServiceSettingsManager(repo, insightai.DefaultServiceSettings())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := newServerWithOperator(nil, nil, nil, nil, nil, nil, runtime.Status{OperatorIdentity: "insight:operator:local"})
	srv.aiServiceManager = manager
	handler := srv.routes()

	secret := "sk-owner-a-secret"
	save := httptest.NewRecorder()
	handler.ServeHTTP(save, httptest.NewRequest(http.MethodPut, aiServiceSettingsRoute+"?hub_user_id=owner-a", bytes.NewBufferString(`{"provider":"openai","baseUrl":"https://example.test/v1","model":"model-a","timeoutSeconds":45,"apiKey":"`+secret+`"}`)))
	if save.Code != http.StatusOK || strings.Contains(save.Body.String(), secret) {
		t.Fatalf("unsafe save response: status=%d body=%s", save.Code, save.Body.String())
	}

	getA := httptest.NewRecorder()
	handler.ServeHTTP(getA, httptest.NewRequest(http.MethodGet, aiServiceSettingsRoute+"?hub_user_id=owner-a", nil))
	var metadataA insightai.ServiceMetadata
	if err := json.Unmarshal(getA.Body.Bytes(), &metadataA); err != nil {
		t.Fatalf("decode owner a: %v", err)
	}
	if metadataA.OwnerID != "owner-a" || !metadataA.APIKeyConfigured || !metadataA.ModelAvailable || metadataA.Source != insightai.ConfigSourceWorkspace || metadataA.Model != "model-a" {
		t.Fatalf("unexpected owner a metadata: %+v", metadataA)
	}

	getB := httptest.NewRecorder()
	handler.ServeHTTP(getB, httptest.NewRequest(http.MethodGet, aiServiceSettingsRoute+"?hub_user_id=owner-b", nil))
	var metadataB insightai.ServiceMetadata
	if err := json.Unmarshal(getB.Body.Bytes(), &metadataB); err != nil {
		t.Fatalf("decode owner b: %v", err)
	}
	if metadataB.OwnerID != "owner-b" || metadataB.APIKeyConfigured || metadataB.ModelAvailable || metadataB.Source != insightai.ConfigSourceNone {
		t.Fatalf("owner b observed owner a config: %+v", metadataB)
	}

	clear := httptest.NewRecorder()
	handler.ServeHTTP(clear, httptest.NewRequest(http.MethodPost, aiServiceSettingsRoute+"/clear-key?hub_user_id=owner-a", nil))
	if clear.Code != http.StatusOK || strings.Contains(clear.Body.String(), secret) {
		t.Fatalf("clear failed: status=%d body=%s", clear.Code, clear.Body.String())
	}
}

func TestAIServiceSettingsAPIVerifiesDraftWithoutSaving(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer draft-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":"verification","output":[]}`))
	}))
	defer upstream.Close()
	repo := insightai.NewMemoryServiceSettingsRepository()
	manager, err := insightai.NewServiceSettingsManager(repo, insightai.DefaultServiceSettings())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := newServerWithOperator(nil, nil, nil, nil, nil, nil, runtime.Status{OperatorIdentity: "owner-a"})
	srv.aiServiceManager = manager
	payload := `{"provider":"openai","baseUrl":"` + upstream.URL + `","model":"test-model","timeoutSeconds":5,"apiKey":"draft-secret"}`
	recorder := httptest.NewRecorder()
	srv.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, aiServiceSettingsRoute+"/verify?hub_user_id=owner-a", bytes.NewBufferString(payload)))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "draft-secret") {
		t.Fatalf("verify failed or leaked secret: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result insightai.VerificationResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || !result.OK {
		t.Fatalf("unexpected verify result: %+v %v", result, err)
	}
	if _, ok, err := repo.Load(context.Background(), "owner-a"); err != nil || ok {
		t.Fatalf("draft verification persisted settings: ok=%v err=%v", ok, err)
	}
}

func TestAIServiceSettingsAPISavesVerifiedDraftWhenRequested(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer saved-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":"verification"}`))
	}))
	defer upstream.Close()

	manager, err := insightai.NewServiceSettingsManager(insightai.NewMemoryServiceSettingsRepository(), insightai.DefaultServiceSettings())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := newServerWithOperator(nil, nil, nil, nil, nil, nil, runtime.Status{OperatorIdentity: "owner-a"})
	srv.aiServiceManager = manager
	recorder := httptest.NewRecorder()
	payload := `{"provider":"openai","baseUrl":"` + upstream.URL + `","model":"verified-model","timeoutSeconds":5,"apiKey":"saved-secret","verifyOnSave":true}`
	srv.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, aiServiceSettingsRoute+"?hub_user_id=owner-a", bytes.NewBufferString(payload)))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "saved-secret") {
		t.Fatalf("verified save failed or leaked secret: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var metadata insightai.ServiceMetadata
	if err := json.Unmarshal(recorder.Body.Bytes(), &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata.VerificationStatus != insightai.VerificationVerified || metadata.LastVerifiedAt.IsZero() {
		t.Fatalf("expected saved verification metadata, got %+v", metadata)
	}
}

func TestAIServiceSettingsAPISavesKeyWithoutEncryptionConfiguration(t *testing.T) {
	manager, err := insightai.NewServiceSettingsManager(insightai.NewMemoryServiceSettingsRepository(), insightai.DefaultServiceSettings())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := newServerWithOperator(nil, nil, nil, nil, nil, nil, runtime.Status{OperatorIdentity: "owner-a"})
	srv.aiServiceManager = manager
	recorder := httptest.NewRecorder()
	srv.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, aiServiceSettingsRoute, bytes.NewBufferString(`{"apiKey":"secret"}`)))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "\"secret\"") {
		t.Fatalf("unexpected direct persistence response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAIServiceSettingsAPIRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	manager, err := insightai.NewServiceSettingsManager(insightai.NewMemoryServiceSettingsRepository(), insightai.DefaultServiceSettings())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := newServerWithOperator(nil, nil, nil, nil, nil, nil, runtime.Status{OperatorIdentity: "owner-a"})
	srv.aiServiceManager = manager
	handler := srv.routes()

	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, httptest.NewRequest(http.MethodPost, aiServiceSettingsRoute+"/verify", bytes.NewBufferString(`{"provider":"openai","ownerId":"owner-b"}`)))
	if unknown.Code != http.StatusBadRequest || !strings.Contains(unknown.Body.String(), "unknown field") {
		t.Fatalf("unexpected unknown-field response: status=%d body=%s", unknown.Code, unknown.Body.String())
	}

	trailing := httptest.NewRecorder()
	handler.ServeHTTP(trailing, httptest.NewRequest(http.MethodPost, aiServiceSettingsRoute+"/verify", bytes.NewBufferString(`{} {}`)))
	if trailing.Code != http.StatusBadRequest || !strings.Contains(trailing.Body.String(), "single JSON object") {
		t.Fatalf("unexpected trailing-json response: status=%d body=%s", trailing.Code, trailing.Body.String())
	}
}
