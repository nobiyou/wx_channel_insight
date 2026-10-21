package insightaifile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insightai"
)

type Repository struct {
	mu    sync.Mutex
	path  string
	items map[string]insightai.StoredServiceSettings
	codec *insightai.SecretCodec
}

func NewRepository(path string, codecs ...*insightai.SecretCodec) (*Repository, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("AI service settings repository path is required")
	}
	var codec *insightai.SecretCodec
	if len(codecs) > 0 {
		codec = codecs[0]
	}
	repo := &Repository{path: path, items: make(map[string]insightai.StoredServiceSettings), codec: codec}
	if err := repo.loadLocked(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Load(_ context.Context, ownerID string) (insightai.StoredServiceSettings, bool, error) {
	if r == nil {
		return insightai.StoredServiceSettings{}, false, errors.New("AI service settings file repository is nil")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return insightai.StoredServiceSettings{}, false, insightai.ErrOwnerRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return insightai.StoredServiceSettings{}, false, err
	}
	settings, ok := r.items[ownerID]
	if !ok {
		return insightai.StoredServiceSettings{}, false, nil
	}
	normalized, err := r.decode(settings)
	return normalized, true, err
}

func (r *Repository) Save(_ context.Context, ownerID string, settings insightai.StoredServiceSettings) error {
	if r == nil {
		return errors.New("AI service settings file repository is nil")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return insightai.ErrOwnerRequired
	}
	normalized, err := r.decode(settings)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	r.items[ownerID] = normalized
	return r.persistLocked()
}

func (r *Repository) ClearKey(ctx context.Context, ownerID string) error {
	settings, ok, err := r.Load(ctx, ownerID)
	if err != nil || !ok {
		return err
	}
	settings.APIKey = ""
	settings.APIKeyFingerprint = ""
	settings.APIKeyHint = ""
	settings.VerificationStatus = insightai.VerificationNever
	settings.LastVerifiedAt = time.Time{}
	settings.LastVerifyLatencyMS = 0
	settings.LastVerifyError = ""
	settings.UpdatedAt = time.Now().UTC()
	return r.Save(ctx, ownerID, settings)
}

func (r *Repository) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		r.items = make(map[string]insightai.StoredServiceSettings)
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		r.items = make(map[string]insightai.StoredServiceSettings)
		return nil
	}
	var items map[string]insightai.StoredServiceSettings
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	if items == nil {
		items = make(map[string]insightai.StoredServiceSettings)
	}
	for ownerID, settings := range items {
		normalized, err := r.decode(settings)
		if err != nil {
			return errors.New("invalid AI service settings for owner " + ownerID + ": " + err.Error())
		}
		items[ownerID] = normalized
	}
	r.items = items
	return nil
}

func (r *Repository) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	persisted := make(map[string]insightai.StoredServiceSettings, len(r.items))
	for ownerID, settings := range r.items {
		encoded, err := r.encode(settings)
		if err != nil {
			return errors.New("cannot persist AI service settings for owner " + ownerID + ": " + err.Error())
		}
		persisted[ownerID] = encoded
		r.items[ownerID] = encoded
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}

func (r *Repository) decode(settings insightai.StoredServiceSettings) (insightai.StoredServiceSettings, error) {
	if strings.TrimSpace(settings.APIKeyCiphertext) != "" {
		if r.codec == nil {
			return insightai.StoredServiceSettings{}, errors.New("AI config encryption key is required to read persisted API keys")
		}
		apiKey, err := r.codec.Decrypt(settings.APIKeyCiphertext, settings.APIKeyNonce)
		if err != nil {
			return insightai.StoredServiceSettings{}, err
		}
		settings.APIKey = apiKey
	}
	return insightai.NormalizeStoredServiceSettings(settings)
}

func (r *Repository) encode(settings insightai.StoredServiceSettings) (insightai.StoredServiceSettings, error) {
	normalized, err := insightai.NormalizeStoredServiceSettings(settings)
	if err != nil {
		return insightai.StoredServiceSettings{}, err
	}
	if normalized.APIKey == "" {
		return normalized, nil
	}
	if r.codec == nil {
		return insightai.StoredServiceSettings{}, errors.New("AI config encryption key is required to persist API keys")
	}
	ciphertext, nonce, err := r.codec.Encrypt(normalized.APIKey)
	if err != nil {
		return insightai.StoredServiceSettings{}, err
	}
	normalized.APIKey = ""
	normalized.APIKeyCiphertext = ciphertext
	normalized.APIKeyNonce = nonce
	return normalized, nil
}
