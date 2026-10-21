package sqlite

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type AccountRepository struct{ c *Client }

func NewAccountRepository(c *Client) (*AccountRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &AccountRepository{c}, nil
}
func (r *AccountRepository) List(ctx context.Context, scope string) ([]insight.Account, error) {
	items, err := list[insight.Account](ctx, r.c.db, "accounts", scope)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, err
}
func (r *AccountRepository) Get(ctx context.Context, scope, id string) (insight.Account, bool, error) {
	return get[insight.Account](ctx, r.c.db, "accounts", scope, id)
}
func upsertObserved(ctx context.Context, q queryer, scope string, profile insight.AccountObservedProfile) (insight.Account, error) {
	now := profile.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	observed, ok := insight.BuildObservedAccount(scope, profile, now)
	if !ok {
		return insight.Account{}, insight.ErrAccountIdentityIncomplete
	}
	existing, found, err := get[insight.Account](ctx, q, "accounts", scope, observed.ID)
	if err != nil {
		return insight.Account{}, err
	}
	if found {
		observed = insight.MergeObservedAccount(existing, observed, now)
	}
	return observed, put(ctx, q, "accounts", scope, observed.ID, observed)
}
func (r *AccountRepository) UpsertObservedProfile(ctx context.Context, scope string, profile insight.AccountObservedProfile) (v insight.Account, err error) {
	err = r.c.update(ctx, func(tx *sql.Tx) error { var err error; v, err = upsertObserved(ctx, tx, scope, profile); return err })
	return
}
func (r *AccountRepository) SyncObservedProfiles(ctx context.Context, scope string, profiles []insight.AccountObservedProfile) error {
	return r.c.update(ctx, func(tx *sql.Tx) error {
		for _, profile := range profiles {
			if strings.TrimSpace(scope) == "" || strings.TrimSpace(profile.ChannelUsername) == "" {
				continue
			}
			if _, err := upsertObserved(ctx, tx, scope, profile); err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *AccountRepository) UpdateManualProfile(ctx context.Context, scope, id string, profile insight.AccountManualProfile) (item insight.Account, err error) {
	err = r.c.update(ctx, func(tx *sql.Tx) error {
		var found bool
		var err error
		item, found, err = get[insight.Account](ctx, tx, "accounts", scope, id)
		if err != nil {
			return err
		}
		if !found {
			return insight.ErrAccountNotFound
		}
		item.Region = strings.TrimSpace(profile.Region)
		item.Industry = strings.TrimSpace(profile.Industry)
		item.Tags = append([]string(nil), profile.Tags...)
		item.Note = strings.TrimSpace(profile.Note)
		item.IsPriority = profile.IsPriority
		item.ProfileSource = insight.AccountProfileSourceMixed
		item.UpdatedAt = time.Now().UTC()
		return put(ctx, tx, "accounts", scope, id, item)
	})
	return
}

var (
	_ insight.AccountRepository                    = (*AccountRepository)(nil)
	_ insight.AccountObservedProfileBulkRepository = (*AccountRepository)(nil)
)
