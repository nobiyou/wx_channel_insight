package insight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type AccountPlatform string

const AccountPlatformWXChannels AccountPlatform = "wx_channels"

type AccountIdentityStatus string

const (
	AccountIdentityVerified   AccountIdentityStatus = "verified"
	AccountIdentityIncomplete AccountIdentityStatus = "incomplete"
)

type AccountProfileSource string

const (
	AccountProfileSourceUpstream AccountProfileSource = "upstream"
	AccountProfileSourceManual   AccountProfileSource = "manual"
	AccountProfileSourceMixed    AccountProfileSource = "mixed"
)

var (
	ErrAccountServiceUnavailable = errors.New("account service is not configured")
	ErrAccountNotFound           = errors.New("account was not found")
	ErrAccountIdentityIncomplete = errors.New("account identity is incomplete")
)

// Account is the canonical workspace-scoped account master record. Metrics are
// intentionally kept out of this record; they are derived from observations.
type Account struct {
	ID                 string                `json:"accountId"`
	WorkspaceID        string                `json:"workspaceId"`
	Platform           AccountPlatform       `json:"platform"`
	ChannelUsername    string                `json:"channelUsername"`
	NormalizedUsername string                `json:"normalizedUsername"`
	IdentityStatus     AccountIdentityStatus `json:"identityStatus"`
	ChannelName        string                `json:"channelName"`
	Signature          string                `json:"signature"`
	AvatarURL          string                `json:"avatarUrl"`
	Region             string                `json:"region"`
	Industry           string                `json:"industry"`
	Tags               []string              `json:"tags"`
	Note               string                `json:"note"`
	IsPriority         bool                  `json:"isPriority"`
	ProfileSource      AccountProfileSource  `json:"profileSource"`
	ProfileObservedAt  time.Time             `json:"profileObservedAt,omitempty"`
	CreatedAt          time.Time             `json:"createdAt"`
	UpdatedAt          time.Time             `json:"updatedAt"`
}

type AccountObservedProfile struct {
	Platform        AccountPlatform
	ChannelUsername string
	ChannelName     string
	Signature       string
	AvatarURL       string
	Region          string
	Industry        string
	Tags            []string
	ObservedAt      time.Time
}

type AccountManualProfile struct {
	Region     string
	Industry   string
	Tags       []string
	Note       string
	IsPriority bool
}

type AccountProfilePatch struct {
	Region     *string  `json:"region"`
	Industry   *string  `json:"industry"`
	Tags       []string `json:"tags"`
	Note       *string  `json:"note"`
	IsPriority *bool    `json:"isPriority"`
}

type AccountQuery struct {
	HubUserID     string
	DeviceCount   int
	ContextSource string
	Keyword       string
	Region        string
	Industry      string
	Source        string
	Priority      *bool
	Page          int
	PageSize      int
	Sort          string
	Direction     string
}

type AccountPage struct {
	Items      []Account `json:"items"`
	Page       int       `json:"page"`
	PageSize   int       `json:"pageSize"`
	Total      int       `json:"total"`
	TotalPages int       `json:"totalPages"`
}

type AccountRepository interface {
	List(ctx context.Context, workspaceID string) ([]Account, error)
	Get(ctx context.Context, workspaceID, accountID string) (Account, bool, error)
	UpsertObservedProfile(ctx context.Context, workspaceID string, profile AccountObservedProfile) (Account, error)
	UpdateManualProfile(ctx context.Context, workspaceID, accountID string, profile AccountManualProfile) (Account, error)
}

// AccountPageRepository is an optional bounded-read implementation. File and
// demo repositories keep using the List fallback, while PostgreSQL can apply
// filters, ordering and pagination before scanning rows into memory.
type AccountPageRepository interface {
	ListPage(ctx context.Context, workspaceID string, query AccountQuery) (AccountPage, error)
}

type AccountServiceConfig struct {
	Repository               AccountRepository
	WorkspaceContextResolver WorkspaceContextResolver
	Now                      func() time.Time
}

type AccountService struct {
	repo            AccountRepository
	contextResolver WorkspaceContextResolver
	now             func() time.Time
}

func NewAccountService(config AccountServiceConfig) *AccountService {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &AccountService{
		repo:            config.Repository,
		contextResolver: config.WorkspaceContextResolver,
		now:             now,
	}
}

func (s *AccountService) List(ctx context.Context, query AccountQuery) (AccountPage, error) {
	if s == nil || s.repo == nil {
		return AccountPage{}, ErrAccountServiceUnavailable
	}
	query, err := normalizeAccountQuery(query)
	if err != nil {
		return AccountPage{}, err
	}
	workspaceID, err := s.resolveWorkspace(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return AccountPage{}, err
	}
	if paged, ok := s.repo.(AccountPageRepository); ok {
		return paged.ListPage(ctx, workspaceID, query)
	}
	items, err := s.repo.List(ctx, workspaceID)
	if err != nil {
		return AccountPage{}, err
	}
	filtered := make([]Account, 0, len(items))
	for _, item := range items {
		if matchesAccountQuery(item, query) {
			filtered = append(filtered, CloneAccount(item))
		}
	}
	sortAccounts(filtered, query.Sort, query.Direction)
	page, pageSize, totalPages := paginateAccounts(len(filtered), query.Page, query.PageSize)
	start := (page - 1) * pageSize
	end := minAccountInt(start+pageSize, len(filtered))
	pageItems := []Account{}
	if start < len(filtered) {
		pageItems = append(pageItems, filtered[start:end]...)
	}
	return AccountPage{Items: pageItems, Page: page, PageSize: pageSize, Total: len(filtered), TotalPages: totalPages}, nil
}

func (s *AccountService) Get(ctx context.Context, query AccountQuery, accountID string) (Account, error) {
	if s == nil || s.repo == nil {
		return Account{}, ErrAccountServiceUnavailable
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return Account{}, ErrAccountNotFound
	}
	workspaceID, err := s.resolveWorkspace(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return Account{}, err
	}
	item, found, err := s.repo.Get(ctx, workspaceID, accountID)
	if err != nil {
		return Account{}, err
	}
	if !found {
		return Account{}, ErrAccountNotFound
	}
	return CloneAccount(item), nil
}

// FindByChannelUsername resolves the stable upstream username without using a
// paginated UI query. It is used by single-video actions that only carry the
// video observation's channel username.
func (s *AccountService) FindByChannelUsername(ctx context.Context, query AccountQuery, username string) (Account, error) {
	if s == nil || s.repo == nil {
		return Account{}, ErrAccountServiceUnavailable
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return Account{}, ErrAccountNotFound
	}
	workspaceID, err := s.resolveWorkspace(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return Account{}, err
	}
	items, err := s.repo.List(ctx, workspaceID)
	if err != nil {
		return Account{}, err
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.ChannelUsername), username) {
			return CloneAccount(item), nil
		}
	}
	return Account{}, ErrAccountNotFound
}

func (s *AccountService) UpsertObservedProfile(ctx context.Context, query AccountQuery, profile AccountObservedProfile) (Account, error) {
	if s == nil || s.repo == nil {
		return Account{}, ErrAccountServiceUnavailable
	}
	profile.ChannelUsername = strings.TrimSpace(profile.ChannelUsername)
	if profile.ChannelUsername == "" {
		return Account{}, ErrAccountIdentityIncomplete
	}
	if profile.Platform == "" {
		profile.Platform = AccountPlatformWXChannels
	}
	if profile.ObservedAt.IsZero() {
		profile.ObservedAt = s.now().UTC()
	}
	workspaceID, err := s.resolveWorkspace(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return Account{}, err
	}
	return s.repo.UpsertObservedProfile(ctx, workspaceID, profile)
}

// SyncObservedProfiles promotes account identities found in ingest evidence
// into the canonical account master while leaving manual fields untouched.
func (s *AccountService) SyncObservedProfiles(ctx context.Context, query AccountQuery, profiles []AccountObservedProfile) (int, error) {
	if s == nil || s.repo == nil {
		return 0, ErrAccountServiceUnavailable
	}
	workspaceID, err := s.resolveWorkspace(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return 0, err
	}
	normalized := NormalizeObservedProfiles(profiles, s.now().UTC())
	if len(normalized) == 0 {
		return 0, nil
	}
	if bulk, ok := s.repo.(AccountObservedProfileBulkRepository); ok {
		if err := bulk.SyncObservedProfiles(ctx, workspaceID, normalized); err != nil {
			return 0, err
		}
		return len(normalized), nil
	}
	for _, profile := range normalized {
		if _, err := s.repo.UpsertObservedProfile(ctx, workspaceID, profile); err != nil {
			return 0, err
		}
	}
	return len(normalized), nil
}

func (s *AccountService) UpdateManualProfile(ctx context.Context, query AccountQuery, accountID string, patch AccountProfilePatch) (Account, error) {
	if s == nil || s.repo == nil {
		return Account{}, ErrAccountServiceUnavailable
	}
	current, err := s.Get(ctx, query, accountID)
	if err != nil {
		return Account{}, err
	}
	manual := AccountManualProfile{
		Region:     current.Region,
		Industry:   current.Industry,
		Tags:       append([]string(nil), current.Tags...),
		Note:       current.Note,
		IsPriority: current.IsPriority,
	}
	if patch.Region != nil {
		manual.Region = strings.TrimSpace(*patch.Region)
	}
	if patch.Industry != nil {
		manual.Industry = strings.TrimSpace(*patch.Industry)
	}
	if patch.Tags != nil {
		manual.Tags = normalizeAccountTags(patch.Tags)
	}
	if patch.Note != nil {
		manual.Note = strings.TrimSpace(*patch.Note)
	}
	if patch.IsPriority != nil {
		manual.IsPriority = *patch.IsPriority
	}
	workspaceID, err := s.resolveWorkspace(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return Account{}, err
	}
	return s.repo.UpdateManualProfile(ctx, workspaceID, current.ID, manual)
}

func (s *AccountService) SetPriority(ctx context.Context, query AccountQuery, accountID string, priority bool) (Account, error) {
	return s.UpdateManualProfile(ctx, query, accountID, AccountProfilePatch{IsPriority: &priority})
}

func (s *AccountService) resolveWorkspace(ctx context.Context, hubUserID string, deviceCount int, contextSource string) (string, error) {
	if s.contextResolver == nil {
		return strings.TrimSpace(hubUserID), nil
	}
	resolved, err := s.contextResolver.Resolve(ctx, workspace.ContextQuery{
		HubUserID:     hubUserID,
		DeviceCount:   deviceCount,
		ContextSource: contextSource,
	})
	if err != nil {
		return "", err
	}
	return resolved.Workspace.ID, nil
}

func normalizeAccountQuery(query AccountQuery) (AccountQuery, error) {
	if query.Page == 0 {
		query.Page = 1
	}
	if query.Page < 1 {
		return AccountQuery{}, errors.New("account page must be at least 1")
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.PageSize != 20 && query.PageSize != 50 && query.PageSize != 100 {
		return AccountQuery{}, errors.New("account pageSize must be 20, 50 or 100")
	}
	query.Sort = strings.ToLower(strings.TrimSpace(query.Sort))
	if query.Sort == "" {
		query.Sort = "updated_at"
	}
	if query.Sort != "updated_at" && query.Sort != "channel_name" && query.Sort != "profile_observed_at" {
		return AccountQuery{}, fmt.Errorf("account sort %q is not supported", query.Sort)
	}
	query.Direction = strings.ToLower(strings.TrimSpace(query.Direction))
	if query.Direction == "" {
		query.Direction = "desc"
	}
	if query.Direction != "asc" && query.Direction != "desc" {
		return AccountQuery{}, errors.New("account direction must be asc or desc")
	}
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Region = strings.TrimSpace(query.Region)
	query.Industry = strings.TrimSpace(query.Industry)
	query.Source = strings.TrimSpace(query.Source)
	return query, nil
}

func matchesAccountQuery(item Account, query AccountQuery) bool {
	if query.Keyword != "" {
		needle := strings.ToLower(query.Keyword)
		matched := false
		for _, value := range []string{item.ID, item.ChannelUsername, item.ChannelName, item.Signature, item.Region, item.Industry, item.Note} {
			if strings.Contains(strings.ToLower(value), needle) {
				matched = true
				break
			}
		}
		if !matched {
			for _, tag := range item.Tags {
				if strings.Contains(strings.ToLower(tag), needle) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return false
		}
	}
	if query.Region != "" && !strings.EqualFold(item.Region, query.Region) {
		return false
	}
	if query.Industry != "" && !strings.EqualFold(item.Industry, query.Industry) {
		return false
	}
	if query.Source != "" && !strings.EqualFold(string(item.ProfileSource), query.Source) {
		return false
	}
	if query.Priority != nil && item.IsPriority != *query.Priority {
		return false
	}
	return true
}

func sortAccounts(items []Account, sortKey, direction string) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		var cmp int
		switch sortKey {
		case "channel_name":
			cmp = strings.Compare(strings.ToLower(left.ChannelName), strings.ToLower(right.ChannelName))
		case "profile_observed_at":
			cmp = compareAccountTime(left.ProfileObservedAt, right.ProfileObservedAt)
		default:
			cmp = compareAccountTime(left.UpdatedAt, right.UpdatedAt)
		}
		if cmp == 0 {
			cmp = strings.Compare(left.ID, right.ID)
		}
		if direction == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})
}

func compareAccountTime(left, right time.Time) int {
	if left.Before(right) {
		return -1
	}
	if left.After(right) {
		return 1
	}
	return 0
}

func paginateAccounts(total, page, pageSize int) (int, int, int) {
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	return page, pageSize, totalPages
}

func minAccountInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func normalizeAccountTags(tags []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, tag)
	}
	return result
}

func StableAccountID(workspaceID string, platform AccountPlatform, normalizedUsername string) string {
	value := strings.Join([]string{strings.TrimSpace(workspaceID), string(platform), strings.ToLower(strings.TrimSpace(normalizedUsername))}, "\x00")
	digest := sha256.Sum256([]byte(value))
	return "acct_" + hex.EncodeToString(digest[:12])
}

func CloneAccount(source Account) Account {
	source.Tags = append([]string(nil), source.Tags...)
	return source
}
