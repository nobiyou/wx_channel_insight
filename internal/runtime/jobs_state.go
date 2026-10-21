package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type JobsRuntimeState struct {
	LastMode           string    `json:"lastMode"`
	LastStatus         string    `json:"lastStatus"`
	LastHubUserID      string    `json:"lastHubUserId"`
	LastStartedAt      time.Time `json:"lastStartedAt"`
	LastFinishedAt     time.Time `json:"lastFinishedAt"`
	LastIteration      int       `json:"lastIteration"`
	LastIterations     int       `json:"lastIterations"`
	LastInterval       string    `json:"lastInterval"`
	LastError          string    `json:"lastError"`
	LastRequestedCount int       `json:"lastRequestedCount"`
	LastRequestedSources []string `json:"lastRequestedSources,omitempty"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func LoadJobsRuntimeState(path string) (JobsRuntimeState, error) {
	if path == "" {
		return JobsRuntimeState{}, nil
	}
	resolved, err := ResolveStorePath(path)
	if err != nil {
		return JobsRuntimeState{}, err
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return JobsRuntimeState{}, nil
		}
		return JobsRuntimeState{}, err
	}
	var state JobsRuntimeState
	if len(raw) == 0 {
		return JobsRuntimeState{}, nil
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return JobsRuntimeState{}, err
	}
	return state, nil
}

func SaveJobsRuntimeState(path string, state JobsRuntimeState) error {
	if path == "" {
		return nil
	}
	resolved, err := ResolveStorePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tempPath := resolved + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, resolved)
}
