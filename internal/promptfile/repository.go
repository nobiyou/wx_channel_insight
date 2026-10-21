package promptfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/prompt"
)

type Repository struct {
	mu    sync.Mutex
	path  string
	items map[string]prompt.Settings
}

func NewRepository(path string) (*Repository, error) {
	if path == "" {
		return nil, errors.New("prompt repository path is required")
	}
	repo := &Repository{path: path, items: make(map[string]prompt.Settings)}
	if err := repo.loadLocked(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Load(_ context.Context, ownerID string) (prompt.Settings, error) {
	if r == nil {
		return prompt.Settings{}, errors.New("prompt repository is nil")
	}
	if ownerID == "" {
		return prompt.Settings{}, errors.New("prompt owner id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return prompt.Settings{}, err
	}
	settings, ok := r.items[ownerID]
	if !ok {
		return prompt.DefaultSettings(), nil
	}
	return prompt.NormalizeSettings(prompt.CloneSettings(settings)), nil
}

func (r *Repository) Save(_ context.Context, ownerID string, settings prompt.Settings) error {
	if r == nil {
		return errors.New("prompt repository is nil")
	}
	if ownerID == "" {
		return errors.New("prompt owner id is required")
	}
	settings = prompt.NormalizeSettings(settings)
	if err := prompt.ValidateSettings(settings); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	r.items[ownerID] = prompt.CloneSettings(settings)
	return r.persistLocked()
}

func (r *Repository) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var items map[string]prompt.Settings
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	if items == nil {
		items = make(map[string]prompt.Settings)
	}
	r.items = items
	return nil
}

func (r *Repository) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r.items, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}
