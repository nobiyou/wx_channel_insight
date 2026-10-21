package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/prompt"
)

type promptSettingsResponse struct {
	Items     []promptProfileResponse `json:"items"`
	UpdatedAt time.Time               `json:"updatedAt"`
}

type promptProfileResponse struct {
	Key                       string    `json:"key"`
	Label                     string    `json:"label"`
	Description               string    `json:"description"`
	SystemPrompt              string    `json:"systemPrompt"`
	UserPromptTemplate        string    `json:"userPromptTemplate"`
	DefaultSystemPrompt       string    `json:"defaultSystemPrompt"`
	DefaultUserPromptTemplate string    `json:"defaultUserPromptTemplate"`
	Version                   string    `json:"version"`
	Enabled                   bool      `json:"enabled"`
	IntegrationState          string    `json:"integrationState"`
	Variables                 []string  `json:"variables"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}

func (s *server) handlePromptSettings(w http.ResponseWriter, r *http.Request) {
	if s.promptRepository == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "prompt repository is not configured"})
		return
	}
	const routePrefix = "/api/v1/settings/prompts"
	key := ""
	if strings.HasPrefix(r.URL.Path, routePrefix+"/") {
		key = strings.Trim(strings.TrimPrefix(r.URL.Path, routePrefix+"/"), "/")
	}
	switch {
	case r.Method == http.MethodGet && key == "":
		s.handlePromptSettingsList(w, r)
	case r.Method == http.MethodPut && key != "":
		s.handlePromptSettingsUpdate(w, r, key)
	case r.Method == http.MethodPost && key != "" && strings.HasSuffix(r.URL.Path, "/reset"):
		s.handlePromptSettingsReset(w, r, strings.TrimSuffix(key, "/reset"))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) handlePromptSettingsList(w http.ResponseWriter, r *http.Request) {
	settings, err := s.promptRepository.Load(r.Context(), s.promptOwnerID(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, mapPromptSettings(settings))
}

func (s *server) handlePromptSettingsUpdate(w http.ResponseWriter, r *http.Request, key string) {
	var patch prompt.ProfilePatch
	decoder := json.NewDecoder(io.LimitReader(r.Body, 96*1024))
	if err := decoder.Decode(&patch); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	settings, err := s.promptRepository.Load(r.Context(), s.promptOwnerID(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	settings, err = prompt.ApplyPatch(settings, key, patch, time.Now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.promptRepository.Save(r.Context(), s.promptOwnerID(r), settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, mapPromptSettings(settings))
}

func (s *server) handlePromptSettingsReset(w http.ResponseWriter, r *http.Request, key string) {
	settings, err := s.promptRepository.Load(r.Context(), s.promptOwnerID(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	settings, err = prompt.ResetProfile(settings, key, time.Now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.promptRepository.Save(r.Context(), s.promptOwnerID(r), settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, mapPromptSettings(settings))
}

func (s *server) promptOwnerID(r *http.Request) string {
	ownerID := strings.TrimSpace(hubContextFromRequest(r).HubUserID)
	if ownerID != "" {
		return ownerID
	}
	ownerID = strings.TrimSpace(s.runtimeStatus.OperatorIdentity)
	if ownerID != "" {
		return ownerID
	}
	return "insight:operator:local"
}

func mapPromptSettings(settings prompt.Settings) promptSettingsResponse {
	settings = prompt.NormalizeSettings(settings)
	items := make([]promptProfileResponse, 0, len(prompt.Descriptors()))
	for _, descriptor := range prompt.Descriptors() {
		profile := settings.Profiles[descriptor.Key]
		items = append(items, promptProfileResponse{
			Key:                       profile.Key,
			Label:                     profile.Label,
			Description:               profile.Description,
			SystemPrompt:              profile.SystemPrompt,
			UserPromptTemplate:        profile.UserPromptTemplate,
			DefaultSystemPrompt:       descriptor.DefaultSystemPrompt,
			DefaultUserPromptTemplate: descriptor.DefaultUserPromptTemplate,
			Version:                   profile.Version,
			Enabled:                   profile.Enabled,
			IntegrationState:          profile.IntegrationState,
			Variables:                 append([]string(nil), descriptor.Variables...),
			UpdatedAt:                 profile.UpdatedAt,
		})
	}
	return promptSettingsResponse{Items: items, UpdatedAt: settings.UpdatedAt}
}
