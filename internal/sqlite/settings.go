package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	"github.com/nobiyou/wx_channel_insight/internal/prompt"
)

type PromptRepository struct{ c *Client }

func NewPromptRepository(c *Client) (*PromptRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &PromptRepository{c}, nil
}
func (r *PromptRepository) Load(ctx context.Context, owner string) (prompt.Settings, error) {
	if strings.TrimSpace(owner) == "" {
		return prompt.Settings{}, errors.New("prompt owner id is required")
	}
	v, found, err := get[prompt.Settings](ctx, r.c.db, "prompt", owner, "settings")
	if !found && err == nil {
		return prompt.DefaultSettings(), nil
	}
	return prompt.NormalizeSettings(v), err
}
func (r *PromptRepository) Save(ctx context.Context, owner string, v prompt.Settings) error {
	if strings.TrimSpace(owner) == "" {
		return errors.New("prompt owner id is required")
	}
	v = prompt.NormalizeSettings(v)
	if err := prompt.ValidateSettings(v); err != nil {
		return err
	}
	return put(ctx, r.c.db, "prompt", owner, "settings", v)
}

type AIServiceRepository struct {
	c     *Client
	codec *insightai.SecretCodec
}

func NewAIServiceRepository(c *Client, codec *insightai.SecretCodec) (*AIServiceRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &AIServiceRepository{c, codec}, nil
}
func (r *AIServiceRepository) Load(ctx context.Context, owner string) (insightai.StoredServiceSettings, bool, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return insightai.StoredServiceSettings{}, false, insightai.ErrOwnerRequired
	}
	v, found, err := get[insightai.StoredServiceSettings](ctx, r.c.db, "ai_service", owner, "settings")
	if err != nil || !found {
		return v, found, err
	}
	if v.APIKeyCiphertext != "" {
		if r.codec == nil {
			return insightai.StoredServiceSettings{}, false, errors.New("AI config encryption key is required to read persisted API keys")
		}
		v.APIKey, err = r.codec.Decrypt(v.APIKeyCiphertext, v.APIKeyNonce)
		if err != nil {
			return insightai.StoredServiceSettings{}, false, err
		}
	}
	v, err = insightai.NormalizeStoredServiceSettings(v)
	return v, true, err
}
func (r *AIServiceRepository) Save(ctx context.Context, owner string, v insightai.StoredServiceSettings) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return insightai.ErrOwnerRequired
	}
	v, err := insightai.NormalizeStoredServiceSettings(v)
	if err != nil {
		return err
	}
	if v.APIKey != "" {
		if r.codec == nil {
			return errors.New("AI config encryption key is required to persist API keys")
		}
		v.APIKeyCiphertext, v.APIKeyNonce, err = r.codec.Encrypt(v.APIKey)
		if err != nil {
			return err
		}
		v.APIKey = ""
	}
	return put(ctx, r.c.db, "ai_service", owner, "settings", v)
}
func (r *AIServiceRepository) ClearKey(ctx context.Context, owner string) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return insightai.ErrOwnerRequired
	}
	return r.c.update(ctx, func(tx *sql.Tx) error {
		v, found, err := get[insightai.StoredServiceSettings](ctx, tx, "ai_service", owner, "settings")
		if err != nil || !found {
			return err
		}
		v.APIKey = ""
		v.APIKeyCiphertext = ""
		v.APIKeyNonce = ""
		v.APIKeyFingerprint = ""
		v.APIKeyHint = ""
		v.VerificationStatus = insightai.VerificationNever
		v.LastVerifiedAt = time.Time{}
		v.LastVerifyLatencyMS = 0
		v.LastVerifyError = ""
		v.UpdatedAt = time.Now().UTC()
		return put(ctx, tx, "ai_service", owner, "settings", v)
	})
}

var (
	_ prompt.Repository                   = (*PromptRepository)(nil)
	_ insightai.ServiceSettingsRepository = (*AIServiceRepository)(nil)
)
