package insightaipg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type Repository struct {
	db    *sql.DB
	codec *insightai.SecretCodec
}

func NewRepository(client *postgres.Client, codecs ...*insightai.SecretCodec) (*Repository, error) {
	if client == nil || client.DB() == nil {
		return nil, errors.New("postgres client is required")
	}
	if err := postgres.EnsureAIServiceSettingsSchema(client.DB()); err != nil {
		return nil, err
	}
	var codec *insightai.SecretCodec
	if len(codecs) > 0 {
		codec = codecs[0]
	}
	return &Repository{db: client.DB(), codec: codec}, nil
}

func (r *Repository) Load(ctx context.Context, ownerID string) (insightai.StoredServiceSettings, bool, error) {
	if r == nil || r.db == nil {
		return insightai.StoredServiceSettings{}, false, errors.New("AI service settings postgres repository is nil")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return insightai.StoredServiceSettings{}, false, insightai.ErrOwnerRequired
	}
	var payload []byte
	err := r.db.QueryRowContext(ctx, `
SELECT payload
FROM insight_ai_service_settings
WHERE hub_user_id = $1
`, ownerID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return insightai.StoredServiceSettings{}, false, nil
	}
	if err != nil {
		return insightai.StoredServiceSettings{}, false, err
	}
	var settings insightai.StoredServiceSettings
	if err := json.Unmarshal(payload, &settings); err != nil {
		return insightai.StoredServiceSettings{}, false, err
	}
	normalized, err := r.decode(settings)
	return normalized, true, err
}

func (r *Repository) Save(ctx context.Context, ownerID string, settings insightai.StoredServiceSettings) error {
	if r == nil || r.db == nil {
		return errors.New("AI service settings postgres repository is nil")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return insightai.ErrOwnerRequired
	}
	normalized, err := r.encode(settings)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_ai_service_settings (hub_user_id, payload, updated_at)
VALUES ($1, $2::jsonb, now())
ON CONFLICT (hub_user_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = EXCLUDED.updated_at
`, ownerID, payload)
	return err
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
