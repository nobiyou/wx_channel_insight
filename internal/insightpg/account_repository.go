package insightpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type AccountRepository struct {
	db *sql.DB
}

var _ insight.AccountRepository = (*AccountRepository)(nil)
var _ insight.AccountObservedProfileBulkRepository = (*AccountRepository)(nil)
var _ insight.AccountPageRepository = (*AccountRepository)(nil)

func NewAccountRepository(client *postgres.Client) (*AccountRepository, error) {
	if client == nil || client.DB() == nil {
		return nil, errors.New("postgres client is required")
	}
	if err := postgres.EnsureAccountSchema(client.DB()); err != nil {
		return nil, err
	}
	return &AccountRepository{db: client.DB()}, nil
}

func (r *AccountRepository) List(ctx context.Context, workspaceID string) ([]insight.Account, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := r.db.QueryContext(ctx, accountSelect+`
WHERE workspace_id = $1
ORDER BY updated_at DESC, account_id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []insight.Account{}
	for rows.Next() {
		item, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *AccountRepository) ListPage(ctx context.Context, workspaceID string, query insight.AccountQuery) (insight.AccountPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	where, args := accountPageWhere(workspaceID, query)
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM insight_accounts WHERE "+where, args...).Scan(&total); err != nil {
		return insight.AccountPage{}, err
	}
	totalPages := (total + query.PageSize - 1) / query.PageSize
	if totalPages == 0 {
		totalPages = 1
	}
	page := query.Page
	if page > totalPages {
		page = totalPages
	}
	orderColumn := "updated_at"
	switch query.Sort {
	case "channel_name":
		orderColumn = "channel_name"
	case "profile_observed_at":
		orderColumn = "profile_observed_at"
	}
	direction := "DESC"
	if query.Direction == "asc" {
		direction = "ASC"
	}
	listArgs := append(append([]any(nil), args...), query.PageSize, (page-1)*query.PageSize)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("%s\nWHERE %s\nORDER BY %s %s NULLS LAST, account_id %s\nLIMIT $%d OFFSET $%d", accountSelect, where, orderColumn, direction, direction, len(args)+1, len(args)+2), listArgs...)
	if err != nil {
		return insight.AccountPage{}, err
	}
	defer rows.Close()
	items := make([]insight.Account, 0, query.PageSize)
	for rows.Next() {
		item, scanErr := scanAccount(rows)
		if scanErr != nil {
			return insight.AccountPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return insight.AccountPage{}, err
	}
	return insight.AccountPage{Items: items, Page: page, PageSize: query.PageSize, Total: total, TotalPages: totalPages}, nil
}

func accountPageWhere(workspaceID string, query insight.AccountQuery) (string, []any) {
	clauses := []string{"workspace_id = $1"}
	args := []any{workspaceID}
	if value := strings.ToLower(strings.TrimSpace(query.Keyword)); value != "" {
		pattern := "%" + value + "%"
		start := len(args) + 1
		placeholders := make([]string, 8)
		for index := range placeholders {
			placeholders[index] = fmt.Sprintf("$%d", start+index)
			args = append(args, pattern)
		}
		clauses = append(clauses, fmt.Sprintf("(LOWER(account_id) LIKE %s OR LOWER(channel_username) LIKE %s OR LOWER(channel_name) LIKE %s OR LOWER(signature) LIKE %s OR LOWER(region) LIKE %s OR LOWER(industry) LIKE %s OR LOWER(note) LIKE %s OR LOWER(tags_json::text) LIKE %s)", placeholders[0], placeholders[1], placeholders[2], placeholders[3], placeholders[4], placeholders[5], placeholders[6], placeholders[7]))
	}
	if value := strings.ToLower(strings.TrimSpace(query.Region)); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("LOWER(region) = $%d", len(args)))
	}
	if value := strings.ToLower(strings.TrimSpace(query.Industry)); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("LOWER(industry) = $%d", len(args)))
	}
	if value := strings.ToLower(strings.TrimSpace(query.Source)); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("LOWER(profile_source) = $%d", len(args)))
	}
	if query.Priority != nil {
		args = append(args, *query.Priority)
		clauses = append(clauses, fmt.Sprintf("is_priority = $%d", len(args)))
	}
	return strings.Join(clauses, " AND "), args
}

func (r *AccountRepository) Get(ctx context.Context, workspaceID, accountID string) (insight.Account, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, accountSelect+`
WHERE workspace_id = $1 AND account_id = $2`, workspaceID, accountID)
	item, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return insight.Account{}, false, nil
	}
	if err != nil {
		return insight.Account{}, false, err
	}
	return item, true, nil
}

func (r *AccountRepository) UpsertObservedProfile(ctx context.Context, workspaceID string, profile insight.AccountObservedProfile) (insight.Account, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := profile.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	observed, ok := insight.BuildObservedAccount(workspaceID, profile, now)
	if !ok {
		return insight.Account{}, insight.ErrAccountIdentityIncomplete
	}
	tagsJSON, err := json.Marshal(observed.Tags)
	if err != nil {
		return insight.Account{}, err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_accounts (
  account_id, workspace_id, platform, channel_username, normalized_username,
  identity_status, channel_name, signature, avatar_url, region, industry, tags_json, profile_source,
  profile_observed_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13, $14, $15, $15)
ON CONFLICT (workspace_id, platform, normalized_username) DO UPDATE SET
  account_id = EXCLUDED.account_id,
  channel_username = EXCLUDED.channel_username,
  identity_status = EXCLUDED.identity_status,
  channel_name = COALESCE(NULLIF(EXCLUDED.channel_name, ''), insight_accounts.channel_name),
  signature = COALESCE(NULLIF(EXCLUDED.signature, ''), insight_accounts.signature),
  avatar_url = COALESCE(NULLIF(EXCLUDED.avatar_url, ''), insight_accounts.avatar_url),
  region = CASE
    WHEN insight_accounts.profile_source IN ('manual', 'mixed') THEN insight_accounts.region
    ELSE COALESCE(NULLIF(EXCLUDED.region, ''), insight_accounts.region)
  END,
  industry = CASE
    WHEN insight_accounts.profile_source IN ('manual', 'mixed') THEN insight_accounts.industry
    ELSE COALESCE(NULLIF(EXCLUDED.industry, ''), insight_accounts.industry)
  END,
  tags_json = CASE
    WHEN insight_accounts.profile_source IN ('manual', 'mixed') THEN insight_accounts.tags_json
    WHEN jsonb_typeof(EXCLUDED.tags_json) = 'array' AND jsonb_array_length(EXCLUDED.tags_json) > 0 THEN EXCLUDED.tags_json
    ELSE insight_accounts.tags_json
  END,
  profile_source = CASE WHEN insight_accounts.profile_source IN ('manual', 'mixed') THEN 'mixed' ELSE 'upstream' END,
  profile_observed_at = EXCLUDED.profile_observed_at,
  updated_at = EXCLUDED.updated_at`,
		observed.ID, observed.WorkspaceID, observed.Platform, observed.ChannelUsername, observed.NormalizedUsername,
		observed.IdentityStatus, observed.ChannelName, observed.Signature, observed.AvatarURL,
		observed.Region, observed.Industry, string(tagsJSON), observed.ProfileSource, observed.ProfileObservedAt, now)
	if err != nil {
		return insight.Account{}, err
	}
	item, found, err := r.Get(ctx, workspaceID, observed.ID)
	if err != nil {
		return insight.Account{}, err
	}
	if !found {
		return insight.Account{}, errors.New("account upsert returned no row")
	}
	return item, nil
}

func (r *AccountRepository) SyncObservedProfiles(ctx context.Context, workspaceID string, profiles []insight.AccountObservedProfile) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for _, profile := range profiles {
		observedAt := profile.ObservedAt
		if observedAt.IsZero() {
			observedAt = now
		}
		profile.ObservedAt = observedAt
		observed, ok := insight.BuildObservedAccount(workspaceID, profile, now)
		if !ok {
			continue
		}
		tagsJSON, err := json.Marshal(observed.Tags)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO insight_accounts (
  account_id, workspace_id, platform, channel_username, normalized_username,
  identity_status, channel_name, signature, avatar_url, region, industry, tags_json, profile_source,
  profile_observed_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13, $14, $15, $15)
ON CONFLICT (workspace_id, platform, normalized_username) DO UPDATE SET
  account_id = EXCLUDED.account_id,
  channel_username = EXCLUDED.channel_username,
  identity_status = EXCLUDED.identity_status,
  channel_name = COALESCE(NULLIF(EXCLUDED.channel_name, ''), insight_accounts.channel_name),
  signature = COALESCE(NULLIF(EXCLUDED.signature, ''), insight_accounts.signature),
  avatar_url = COALESCE(NULLIF(EXCLUDED.avatar_url, ''), insight_accounts.avatar_url),
  region = CASE
    WHEN insight_accounts.profile_source IN ('manual', 'mixed') THEN insight_accounts.region
    ELSE COALESCE(NULLIF(EXCLUDED.region, ''), insight_accounts.region)
  END,
  industry = CASE
    WHEN insight_accounts.profile_source IN ('manual', 'mixed') THEN insight_accounts.industry
    ELSE COALESCE(NULLIF(EXCLUDED.industry, ''), insight_accounts.industry)
  END,
  tags_json = CASE
    WHEN insight_accounts.profile_source IN ('manual', 'mixed') THEN insight_accounts.tags_json
    WHEN jsonb_typeof(EXCLUDED.tags_json) = 'array' AND jsonb_array_length(EXCLUDED.tags_json) > 0 THEN EXCLUDED.tags_json
    ELSE insight_accounts.tags_json
  END,
  profile_source = CASE WHEN insight_accounts.profile_source IN ('manual', 'mixed') THEN 'mixed' ELSE 'upstream' END,
  profile_observed_at = EXCLUDED.profile_observed_at,
  updated_at = EXCLUDED.updated_at`,
			observed.ID, observed.WorkspaceID, observed.Platform, observed.ChannelUsername, observed.NormalizedUsername,
			observed.IdentityStatus, observed.ChannelName, observed.Signature, observed.AvatarURL,
			observed.Region, observed.Industry, string(tagsJSON), observed.ProfileSource, observed.ProfileObservedAt, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *AccountRepository) UpdateManualProfile(ctx context.Context, workspaceID, accountID string, profile insight.AccountManualProfile) (insight.Account, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	tagsJSON, err := json.Marshal(profile.Tags)
	if err != nil {
		return insight.Account{}, err
	}
	row := r.db.QueryRowContext(ctx, `
UPDATE insight_accounts
SET region = $1, industry = $2, tags_json = $3::jsonb, note = $4,
    is_priority = $5, profile_source = CASE WHEN profile_source = 'upstream' THEN 'mixed' ELSE profile_source END,
    updated_at = now()
WHERE workspace_id = $6 AND account_id = $7
RETURNING account_id, workspace_id, platform, channel_username, normalized_username,
          identity_status, channel_name, signature, avatar_url, region, industry,
          tags_json, note, is_priority, profile_source, profile_observed_at, created_at, updated_at`,
		strings.TrimSpace(profile.Region), strings.TrimSpace(profile.Industry), string(tagsJSON), strings.TrimSpace(profile.Note),
		profile.IsPriority, workspaceID, accountID)
	item, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return insight.Account{}, insight.ErrAccountNotFound
	}
	return item, err
}

const accountSelect = `SELECT account_id, workspace_id, platform, channel_username, normalized_username,
  identity_status, channel_name, signature, avatar_url, region, industry,
  tags_json, note, is_priority, profile_source, profile_observed_at, created_at, updated_at
FROM insight_accounts`

type accountScanner interface {
	Scan(dest ...any) error
}

func scanAccount(scanner accountScanner) (insight.Account, error) {
	var item insight.Account
	var platform, identityStatus, profileSource string
	var tagsJSON []byte
	if err := scanner.Scan(
		&item.ID, &item.WorkspaceID, &platform, &item.ChannelUsername, &item.NormalizedUsername,
		&identityStatus, &item.ChannelName, &item.Signature, &item.AvatarURL, &item.Region, &item.Industry,
		&tagsJSON, &item.Note, &item.IsPriority, &profileSource, &item.ProfileObservedAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return insight.Account{}, err
	}
	item.Platform = insight.AccountPlatform(platform)
	item.IdentityStatus = insight.AccountIdentityStatus(identityStatus)
	item.ProfileSource = insight.AccountProfileSource(profileSource)
	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
			return insight.Account{}, err
		}
	}
	return item, nil
}
