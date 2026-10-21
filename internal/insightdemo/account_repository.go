package insightdemo

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type AccountRepository struct {
	mu    sync.RWMutex
	items map[string]insight.Account
}

var _ insight.AccountRepository = (*AccountRepository)(nil)
var _ insight.AccountObservedProfileBulkRepository = (*AccountRepository)(nil)

func NewAccountRepository() *AccountRepository {
	return &AccountRepository{items: map[string]insight.Account{}}
}

func (r *AccountRepository) List(_ context.Context, workspaceID string) ([]insight.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]insight.Account, 0)
	for _, item := range r.items {
		if item.WorkspaceID == workspaceID {
			items = append(items, insight.CloneAccount(item))
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (r *AccountRepository) Get(_ context.Context, workspaceID, accountID string) (insight.Account, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[accountID]
	if !ok || item.WorkspaceID != workspaceID {
		return insight.Account{}, false, nil
	}
	return insight.CloneAccount(item), true, nil
}

func (r *AccountRepository) UpsertObservedProfile(_ context.Context, workspaceID string, profile insight.AccountObservedProfile) (insight.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := profile.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	observed, ok := insight.BuildObservedAccount(workspaceID, profile, now)
	if !ok {
		return insight.Account{}, insight.ErrAccountIdentityIncomplete
	}
	if existing, found := r.items[observed.ID]; found {
		observed = insight.MergeObservedAccount(existing, observed, now)
	}
	r.items[observed.ID] = insight.CloneAccount(observed)
	return insight.CloneAccount(observed), nil
}

func (r *AccountRepository) SyncObservedProfiles(_ context.Context, workspaceID string, profiles []insight.AccountObservedProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
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
		if existing, found := r.items[observed.ID]; found {
			observed = insight.MergeObservedAccount(existing, observed, now)
		}
		r.items[observed.ID] = insight.CloneAccount(observed)
	}
	return nil
}

func (r *AccountRepository) UpdateManualProfile(_ context.Context, workspaceID, accountID string, profile insight.AccountManualProfile) (insight.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[accountID]
	if !ok || item.WorkspaceID != workspaceID {
		return insight.Account{}, insight.ErrAccountNotFound
	}
	item.Region = strings.TrimSpace(profile.Region)
	item.Industry = strings.TrimSpace(profile.Industry)
	item.Tags = append([]string(nil), profile.Tags...)
	item.Note = strings.TrimSpace(profile.Note)
	item.IsPriority = profile.IsPriority
	item.ProfileSource = insight.AccountProfileSourceMixed
	item.UpdatedAt = time.Now().UTC()
	r.items[accountID] = insight.CloneAccount(item)
	return insight.CloneAccount(item), nil
}
