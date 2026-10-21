package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insightai"
)

const aiServiceSettingsRoute = "/api/v1/settings/ai-service"

type aiServiceSettingsRequest struct {
	Provider       string `json:"provider"`
	BaseURL        string `json:"baseUrl"`
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	AnalysisEngine string `json:"analysisEngine"`
	FallbackPolicy string `json:"fallbackPolicy"`
	APIKey         string `json:"apiKey"`
	VerifyOnSave   bool   `json:"verifyOnSave"`
}

func (s *server) handleAIServiceSettings(w http.ResponseWriter, r *http.Request) {
	if s.aiServiceManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AI service settings are not configured"})
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, aiServiceSettingsRoute), "/")
	switch {
	case r.Method == http.MethodGet && action == "":
		s.handleAIServiceSettingsGet(w, r)
	case r.Method == http.MethodPut && action == "":
		s.handleAIServiceSettingsSave(w, r)
	case r.Method == http.MethodPost && action == "verify":
		s.handleAIServiceSettingsVerify(w, r)
	case r.Method == http.MethodPost && action == "clear-key":
		s.handleAIServiceSettingsClearKey(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *server) handleAIServiceSettingsGet(w http.ResponseWriter, r *http.Request) {
	metadata, err := s.aiServiceManager.Metadata(r.Context(), s.promptOwnerID(r))
	if err != nil {
		writeAIServiceSettingsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, metadata)
}

func (s *server) handleAIServiceSettingsSave(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeAIServiceSettingsRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	patch := payload.patch()
	var metadata insightai.ServiceMetadata
	if payload.VerifyOnSave {
		metadata, err = s.aiServiceManager.SaveAndVerify(r.Context(), s.promptOwnerID(r), patch, time.Now())
	} else {
		metadata, err = s.aiServiceManager.Save(r.Context(), s.promptOwnerID(r), patch, time.Now())
	}
	if err != nil {
		writeAIServiceSettingsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, metadata)
}

func (s *server) handleAIServiceSettingsVerify(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeAIServiceSettingsRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	effective, err := s.aiServiceManager.Resolve(r.Context(), s.promptOwnerID(r))
	if err != nil {
		writeAIServiceSettingsError(w, err)
		return
	}
	draft, err := insightai.ApplyServiceSettingsPatch(effective, payload.patch())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, insightai.VerifyConnection(r.Context(), draft, nil))
}

func (s *server) handleAIServiceSettingsClearKey(w http.ResponseWriter, r *http.Request) {
	metadata, err := s.aiServiceManager.ClearKey(r.Context(), s.promptOwnerID(r))
	if err != nil {
		writeAIServiceSettingsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, metadata)
}

func decodeAIServiceSettingsRequest(r *http.Request) (aiServiceSettingsRequest, error) {
	var payload aiServiceSettingsRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		return aiServiceSettingsRequest{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return aiServiceSettingsRequest{}, errors.New("request body must contain a single JSON object")
		}
		return aiServiceSettingsRequest{}, err
	}
	return payload, nil
}

func (r aiServiceSettingsRequest) patch() insightai.ServiceSettingsPatch {
	patch := insightai.ServiceSettingsPatch{
		Provider:       r.Provider,
		BaseURL:        r.BaseURL,
		Model:          r.Model,
		TimeoutSeconds: r.TimeoutSeconds,
		AnalysisEngine: r.AnalysisEngine,
		FallbackPolicy: r.FallbackPolicy,
	}
	if apiKey := strings.TrimSpace(r.APIKey); apiKey != "" {
		patch.APIKey = &apiKey
	}
	return patch
}

func writeAIServiceSettingsError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, insightai.ErrOwnerRequired), strings.Contains(err.Error(), "unsupported AI provider"), strings.Contains(err.Error(), "unsupported analysis engine"), strings.Contains(err.Error(), "unsupported fallback policy"), strings.Contains(err.Error(), "AI base URL"), strings.Contains(err.Error(), "AI timeout"), strings.Contains(err.Error(), "AI model"):
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
