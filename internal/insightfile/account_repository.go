package insightfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type persistedAccounts struct {
	Items []insight.Account `json:"items"`
}

type AccountRepository struct {
	mu    sync.RWMutex
	path  string
	items map[string]insight.Account
}

var _ insight.AccountRepository = (*AccountRepository)(nil)
var _ insight.AccountObservedProfileBulkRepository = (*AccountRepository)(nil)

func NewAccountRepository(path string) (*AccountRepository, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("account repository path is required")
	}
	r := &AccountRepository{path: path, items: map[string]insight.Account{}}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *AccountRepository) List(_ context.Context, workspaceID string) ([]insight.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return nil, err
	}
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return insight.Account{}, false, err
	}
	item, ok := r.items[accountID]
	if !ok || item.WorkspaceID != workspaceID {
		return insight.Account{}, false, nil
	}
	return insight.CloneAccount(item), true, nil
}

func (r *AccountRepository) UpsertObservedProfile(_ context.Context, workspaceID string, profile insight.AccountObservedProfile) (insight.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return insight.Account{}, err
	}
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
	if err := r.persistLocked(); err != nil {
		return insight.Account{}, err
	}
	return insight.CloneAccount(observed), nil
}

func (r *AccountRepository) SyncObservedProfiles(_ context.Context, workspaceID string, profiles []insight.AccountObservedProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
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
	return r.persistLocked()
}

func (r *AccountRepository) UpdateManualProfile(_ context.Context, workspaceID, accountID string, profile insight.AccountManualProfile) (insight.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return insight.Account{}, err
	}
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
	if err := r.persistLocked(); err != nil {
		return insight.Account{}, err
	}
	return insight.CloneAccount(item), nil
}

func (r *AccountRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *AccountRepository) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r.persistLocked()
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return r.persistLocked()
	}
	var payload persistedAccounts
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	r.items = map[string]insight.Account{}
	for _, item := range payload.Items {
		r.items[item.ID] = insight.CloneAccount(item)
	}
	return nil
}

func (r *AccountRepository) persistLocked() error {
	items := make([]insight.Account, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, insight.CloneAccount(item))
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	data, err := json.MarshalIndent(persistedAccounts{Items: items}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}
