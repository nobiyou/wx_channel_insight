package prompt

import (
	"context"
	"errors"
	"sync"
)

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]Settings
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]Settings)}
}

func (r *MemoryRepository) Load(_ context.Context, ownerID string) (Settings, error) {
	if r == nil {
		return Settings{}, errors.New("prompt memory repository is nil")
	}
	if ownerID == "" {
		return Settings{}, errors.New("prompt owner id is required")
	}
	r.mu.RLock()
	settings, ok := r.items[ownerID]
	r.mu.RUnlock()
	if !ok {
		return DefaultSettings(), nil
	}
	return NormalizeSettings(CloneSettings(settings)), nil
}

func (r *MemoryRepository) Save(_ context.Context, ownerID string, settings Settings) error {
	if r == nil {
		return errors.New("prompt memory repository is nil")
	}
	if ownerID == "" {
		return errors.New("prompt owner id is required")
	}
	settings = NormalizeSettings(settings)
	if err := ValidateSettings(settings); err != nil {
		return err
	}
	r.mu.Lock()
	r.items[ownerID] = CloneSettings(settings)
	r.mu.Unlock()
	return nil
}

func CloneSettings(settings Settings) Settings {
	clone := Settings{UpdatedAt: settings.UpdatedAt, Profiles: make(map[string]Profile, len(settings.Profiles))}
	for key, profile := range settings.Profiles {
		clone.Profiles[key] = profile
	}
	return clone
}
