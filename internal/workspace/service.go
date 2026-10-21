package workspace

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type DefaultWorkspaceService struct {
	repo DefaultWorkspaceRepository
	now  func() time.Time
}

func NewDefaultWorkspaceService(repo DefaultWorkspaceRepository) *DefaultWorkspaceService {
	return &DefaultWorkspaceService{
		repo: repo,
		now:  time.Now,
	}
}

func (s *DefaultWorkspaceService) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *DefaultWorkspaceService) EnsureDefaultWorkspace(ctx context.Context, input EnsureDefaultWorkspaceInput) (EnsureDefaultWorkspaceResult, error) {
	hubUserID := strings.TrimSpace(input.HubUserID)
	if hubUserID == "" {
		return EnsureDefaultWorkspaceResult{}, fmt.Errorf("hub user id is required")
	}

	deviceCount := input.DeviceCount
	if deviceCount < 0 {
		deviceCount = 0
	}

	now := s.now()
	workspaceItem, resolution, err := s.repo.UpsertDefaultWorkspace(ctx, hubUserID, func(existing DefaultWorkspace, found bool) (DefaultWorkspace, string, error) {
		if found {
			if existing.DeviceCount != deviceCount {
				existing.DeviceCount = deviceCount
				existing.LastSyncedAt = now
				return existing, "updated", nil
			}
			return existing, "reused", nil
		}

		created := DefaultWorkspace{
			ID:            fmt.Sprintf("ws-default-%s", normalizeIDFragment(hubUserID)),
			Name:          "默认运营工作区",
			OwnerUserID:   hubUserID,
			DeviceCount:   deviceCount,
			BootstrapMode: "lazy_create",
			Source:        "insight_default_workspace",
			State:         "ready",
			CreatedAt:     now,
			LastSyncedAt:  now,
		}
		return created, "created", nil
	})
	if err != nil {
		return EnsureDefaultWorkspaceResult{}, err
	}

	return EnsureDefaultWorkspaceResult{
		Workspace:  workspaceItem,
		Resolution: resolution,
	}, nil
}

func normalizeIDFragment(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char + ('a' - 'A'))
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		default:
			builder.WriteRune('-')
		}
	}

	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "workspace"
	}
	return result
}
