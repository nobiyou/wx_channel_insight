package workspace

import (
	"context"
	"strings"
)

type ContextQuery struct {
	HubUserID     string
	DeviceCount   int
	ContextSource string
}

type ResolvedContext struct {
	Workspace     DefaultWorkspace
	ContextSource string
	Resolution    string
}

type ContextResolverConfig struct {
	DefaultHubUserID   string
	DefaultDeviceCount int
}

type ContextBootstrapper interface {
	EnsureDefaultWorkspace(ctx context.Context, input EnsureDefaultWorkspaceInput) (EnsureDefaultWorkspaceResult, error)
}

type ContextResolver struct {
	bootstrap          ContextBootstrapper
	defaultHubUserID   string
	defaultDeviceCount int
}

func NewContextResolver(bootstrap ContextBootstrapper, config ContextResolverConfig) *ContextResolver {
	return &ContextResolver{
		bootstrap:          bootstrap,
		defaultHubUserID:   config.DefaultHubUserID,
		defaultDeviceCount: config.DefaultDeviceCount,
	}
}

func (r *ContextResolver) Resolve(ctx context.Context, query ContextQuery) (ResolvedContext, error) {
	hubUserID := strings.TrimSpace(query.HubUserID)
	if hubUserID == "" {
		hubUserID = strings.TrimSpace(r.defaultHubUserID)
	}

	deviceCount := query.DeviceCount
	if deviceCount <= 0 {
		deviceCount = r.defaultDeviceCount
	}

	result, err := r.bootstrap.EnsureDefaultWorkspace(ctx, EnsureDefaultWorkspaceInput{
		HubUserID:   hubUserID,
		DeviceCount: deviceCount,
	})
	if err != nil {
		return ResolvedContext{}, err
	}

	return ResolvedContext{
		Workspace:     result.Workspace,
		ContextSource: defaultContextSource(query.ContextSource),
		Resolution:    result.Resolution,
	}, nil
}

func defaultContextSource(source string) string {
	if strings.TrimSpace(source) == "" {
		return "default"
	}
	return source
}
