package promptpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/prompt"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(client *postgres.Client) (*Repository, error) {
	if client == nil || client.DB() == nil {
		return nil, errors.New("postgres client is required")
	}
	if err := postgres.EnsurePromptSettingsSchema(client.DB()); err != nil {
		return nil, err
	}
	return &Repository{db: client.DB()}, nil
}

func (r *Repository) Load(ctx context.Context, ownerID string) (prompt.Settings, error) {
	if r == nil || r.db == nil {
		return prompt.Settings{}, errors.New("prompt postgres repository is nil")
	}
	if ownerID == "" {
		return prompt.Settings{}, errors.New("prompt owner id is required")
	}
	var payload []byte
	err := r.db.QueryRowContext(ctx, `
SELECT payload
FROM insight_prompt_settings
WHERE hub_user_id = $1
`, ownerID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return prompt.DefaultSettings(), nil
	}
	if err != nil {
		return prompt.Settings{}, err
	}
	var settings prompt.Settings
	if err := json.Unmarshal(payload, &settings); err != nil {
		return prompt.Settings{}, err
	}
	settings = prompt.NormalizeSettings(settings)
	if err := prompt.ValidateSettings(settings); err != nil {
		return prompt.Settings{}, err
	}
	return settings, nil
}

func (r *Repository) Save(ctx context.Context, ownerID string, settings prompt.Settings) error {
	if r == nil || r.db == nil {
		return errors.New("prompt postgres repository is nil")
	}
	if ownerID == "" {
		return errors.New("prompt owner id is required")
	}
	settings = prompt.NormalizeSettings(settings)
	if err := prompt.ValidateSettings(settings); err != nil {
		return err
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_prompt_settings (hub_user_id, payload, updated_at)
VALUES ($1, $2::jsonb, now())
ON CONFLICT (hub_user_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = EXCLUDED.updated_at
`, ownerID, payload)
	return err
}
