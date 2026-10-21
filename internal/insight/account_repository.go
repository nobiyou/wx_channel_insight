package insight

import (
	"context"
	"sort"
	"strings"
	"time"
)

// AccountObservedProfileBulkRepository is an optional repository capability
// used when ingest backfills a workspace with many previously observed
// accounts. Implementations keep the whole sync atomic and avoid one file
// rewrite or database round trip per account.
type AccountObservedProfileBulkRepository interface {
	SyncObservedProfiles(ctx context.Context, workspaceID string, profiles []AccountObservedProfile) error
}

func BuildObservedAccount(workspaceID string, profile AccountObservedProfile, now time.Time) (Account, bool) {
	workspaceID = strings.TrimSpace(workspaceID)
	username := strings.TrimSpace(profile.ChannelUsername)
	if workspaceID == "" || username == "" {
		return Account{}, false
	}
	platform := profile.Platform
	if platform == "" {
		platform = AccountPlatformWXChannels
	}
	observedAt := profile.ObservedAt
	if observedAt.IsZero() {
		observedAt = now
	}
	return Account{
		ID:                 StableAccountID(workspaceID, platform, username),
		WorkspaceID:        workspaceID,
		Platform:           platform,
		ChannelUsername:    username,
		NormalizedUsername: strings.ToLower(username),
		IdentityStatus:     AccountIdentityVerified,
		ChannelName:        strings.TrimSpace(profile.ChannelName),
		Signature:          strings.TrimSpace(profile.Signature),
		AvatarURL:          strings.TrimSpace(profile.AvatarURL),
		Region:             strings.TrimSpace(profile.Region),
		Industry:           strings.TrimSpace(profile.Industry),
		Tags:               normalizeAccountTags(profile.Tags),
		ProfileSource:      AccountProfileSourceUpstream,
		ProfileObservedAt:  observedAt,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, true
}

func NormalizeObservedProfiles(profiles []AccountObservedProfile, now time.Time) []AccountObservedProfile {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	byKey := make(map[string]AccountObservedProfile, len(profiles))
	for _, profile := range profiles {
		profile.ChannelUsername = strings.TrimSpace(profile.ChannelUsername)
		if profile.ChannelUsername == "" {
			continue
		}
		if profile.Platform == "" {
			profile.Platform = AccountPlatformWXChannels
		}
		profile.ChannelName = strings.TrimSpace(profile.ChannelName)
		profile.Signature = strings.TrimSpace(profile.Signature)
		profile.AvatarURL = strings.TrimSpace(profile.AvatarURL)
		profile.Region = strings.TrimSpace(profile.Region)
		profile.Industry = strings.TrimSpace(profile.Industry)
		profile.Tags = normalizeAccountTags(profile.Tags)
		if profile.ObservedAt.IsZero() {
			profile.ObservedAt = now
		}
		key := string(profile.Platform) + "\x00" + strings.ToLower(profile.ChannelUsername)
		current, exists := byKey[key]
		if !exists {
			byKey[key] = profile
			continue
		}
		if profile.ObservedAt.After(current.ObservedAt) {
			if profile.ChannelName == "" {
				profile.ChannelName = current.ChannelName
			}
			if profile.Signature == "" {
				profile.Signature = current.Signature
			}
			if profile.AvatarURL == "" {
				profile.AvatarURL = current.AvatarURL
			}
			if profile.Region == "" {
				profile.Region = current.Region
			}
			if profile.Industry == "" {
				profile.Industry = current.Industry
			}
			if len(profile.Tags) == 0 {
				profile.Tags = append([]string(nil), current.Tags...)
			}
			byKey[key] = profile
			continue
		}
		if profile.ObservedAt.Equal(current.ObservedAt) {
			if current.ChannelName == "" {
				current.ChannelName = profile.ChannelName
			}
			if current.Signature == "" {
				current.Signature = profile.Signature
			}
			if current.AvatarURL == "" {
				current.AvatarURL = profile.AvatarURL
			}
			if current.Region == "" {
				current.Region = profile.Region
			}
			if current.Industry == "" {
				current.Industry = profile.Industry
			}
			if len(current.Tags) == 0 {
				current.Tags = append([]string(nil), profile.Tags...)
			}
			byKey[key] = current
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]AccountObservedProfile, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}

func MergeObservedAccount(current Account, observed Account, now time.Time) Account {
	if current.ID == "" {
		return observed
	}
	current.ChannelUsername = observed.ChannelUsername
	current.NormalizedUsername = observed.NormalizedUsername
	current.IdentityStatus = AccountIdentityVerified
	current.Platform = observed.Platform
	if observed.ChannelName != "" {
		current.ChannelName = observed.ChannelName
	}
	if observed.Signature != "" {
		current.Signature = observed.Signature
	}
	if observed.AvatarURL != "" {
		current.AvatarURL = observed.AvatarURL
	}
	current.ProfileObservedAt = observed.ProfileObservedAt
	if current.ProfileSource == AccountProfileSourceManual || current.ProfileSource == AccountProfileSourceMixed {
		current.ProfileSource = AccountProfileSourceMixed
	} else {
		if observed.Region != "" {
			current.Region = observed.Region
		}
		if observed.Industry != "" {
			current.Industry = observed.Industry
		}
		if len(observed.Tags) > 0 {
			current.Tags = append([]string(nil), observed.Tags...)
		}
		current.ProfileSource = AccountProfileSourceUpstream
	}
	if current.CreatedAt.IsZero() {
		current.CreatedAt = now
	}
	current.UpdatedAt = now
	return current
}

func accountRepositoryContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
