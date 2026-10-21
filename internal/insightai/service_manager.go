package insightai

import (
	"context"
	"errors"
	"strings"
	"time"
)

type ServiceSettingsManager struct {
	repository  ServiceSettingsRepository
	environment ServiceSettings
}

func NewServiceSettingsManager(repository ServiceSettingsRepository, environment ServiceSettings) (*ServiceSettingsManager, error) {
	normalized, err := NormalizeServiceSettings(environment)
	if err != nil {
		return nil, err
	}
	if normalized.APIKey != "" {
		normalized.Source = ConfigSourceEnvironment
	} else {
		normalized.Source = ConfigSourceNone
	}
	return &ServiceSettingsManager{repository: repository, environment: normalized}, nil
}

func (m *ServiceSettingsManager) Resolve(ctx context.Context, ownerID string) (ServiceSettings, error) {
	if m == nil {
		return ServiceSettings{}, errors.New("AI service settings manager is nil")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return ServiceSettings{}, ErrOwnerRequired
	}
	effective := m.environment
	effective.OwnerID = ownerID
	if m.repository == nil {
		return effective, nil
	}
	stored, ok, err := m.repository.Load(ctx, ownerID)
	if err != nil {
		return ServiceSettings{}, err
	}
	if !ok {
		return effective, nil
	}
	workspace, err := SettingsFromStored(stored)
	if err != nil {
		return ServiceSettings{}, err
	}
	effective.Provider = workspace.Provider
	effective.BaseURL = workspace.BaseURL
	effective.Model = workspace.Model
	effective.Timeout = workspace.Timeout
	effective.AnalysisEngine = workspace.AnalysisEngine
	effective.FallbackPolicy = workspace.FallbackPolicy
	effective.Verification = workspace.Verification
	effective.LastVerifiedAt = workspace.LastVerifiedAt
	effective.LatencyMS = workspace.LatencyMS
	effective.LastError = workspace.LastError
	effective.OwnerID = ownerID
	if workspace.APIKey != "" {
		effective.APIKey = workspace.APIKey
		effective.Source = ConfigSourceWorkspace
	} else if effective.APIKey != "" {
		effective.Source = ConfigSourceEnvironment
	} else {
		effective.Source = ConfigSourceNone
	}
	return NormalizeServiceSettings(effective)
}

func (m *ServiceSettingsManager) Metadata(ctx context.Context, ownerID string) (ServiceMetadata, error) {
	effective, err := m.Resolve(ctx, ownerID)
	if err != nil {
		return ServiceMetadata{}, err
	}
	metadata, err := MetadataFromEffective(effective)
	if err != nil {
		return ServiceMetadata{}, err
	}
	metadata.APIKeyPersistenceAvailable = m.repository != nil
	metadata.OwnerID = strings.TrimSpace(ownerID)
	if m.repository == nil {
		return metadata, nil
	}
	stored, ok, err := m.repository.Load(ctx, strings.TrimSpace(ownerID))
	if err != nil || !ok {
		return metadata, err
	}
	metadata.UpdatedAt = stored.UpdatedAt
	if effective.Source == ConfigSourceWorkspace {
		metadata.APIKeyHint = stored.APIKeyHint
		metadata.APIKeyConfigured = stored.APIKeyHint != ""
	}
	return metadata, nil
}

func (m *ServiceSettingsManager) Save(ctx context.Context, ownerID string, patch ServiceSettingsPatch, now time.Time) (ServiceMetadata, error) {
	return m.save(ctx, ownerID, patch, now, nil)
}

// SaveAndVerify rechecks the exact draft on the server before persisting its
// verification result. The browser result is only a prompt to use this path;
// it is never trusted as proof of connectivity.
func (m *ServiceSettingsManager) SaveAndVerify(ctx context.Context, ownerID string, patch ServiceSettingsPatch, now time.Time) (ServiceMetadata, error) {
	if m == nil || m.repository == nil {
		return ServiceMetadata{}, errors.New("AI service settings repository is not configured")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return ServiceMetadata{}, ErrOwnerRequired
	}
	base, err := m.Resolve(ctx, ownerID)
	if err != nil {
		return ServiceMetadata{}, err
	}
	draft, err := ApplyServiceSettingsPatch(base, patch)
	if err != nil {
		return ServiceMetadata{}, err
	}
	verification := VerifyConnection(ctx, draft, nil)
	return m.save(ctx, ownerID, patch, now, &verification)
}

func (m *ServiceSettingsManager) save(ctx context.Context, ownerID string, patch ServiceSettingsPatch, now time.Time, verification *VerificationResult) (ServiceMetadata, error) {
	if m == nil || m.repository == nil {
		return ServiceMetadata{}, errors.New("AI service settings repository is not configured")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return ServiceMetadata{}, ErrOwnerRequired
	}
	base, err := m.Resolve(ctx, ownerID)
	if err != nil {
		return ServiceMetadata{}, err
	}
	draft, err := ApplyServiceSettingsPatch(base, patch)
	if err != nil {
		return ServiceMetadata{}, err
	}
	stored, ok, err := m.repository.Load(ctx, ownerID)
	if err != nil {
		return ServiceMetadata{}, err
	}
	if !ok {
		stored = StoredServiceSettings{}
	}
	stored.Provider = draft.Provider
	stored.BaseURL = draft.BaseURL
	stored.Model = draft.Model
	stored.TimeoutSeconds = int(draft.Timeout / time.Second)
	stored.AnalysisEngine = draft.AnalysisEngine
	stored.FallbackPolicy = draft.FallbackPolicy
	if patch.APIKey != nil && strings.TrimSpace(*patch.APIKey) != "" {
		apiKey := strings.TrimSpace(*patch.APIKey)
		stored.APIKey = apiKey
		stored.APIKeyFingerprint = APIKeyFingerprint(apiKey)
		stored.APIKeyHint = APIKeyHint(apiKey)
	}
	applyVerificationState(&stored, verification)
	stored.UpdatedAt = now.UTC()
	stored, err = NormalizeStoredServiceSettings(stored)
	if err != nil {
		return ServiceMetadata{}, err
	}
	if err := m.repository.Save(ctx, ownerID, stored); err != nil {
		return ServiceMetadata{}, err
	}
	return m.Metadata(ctx, ownerID)
}

func applyVerificationState(stored *StoredServiceSettings, verification *VerificationResult) {
	if stored == nil {
		return
	}
	if verification == nil {
		stored.VerificationStatus = VerificationPending
		stored.LastVerifiedAt = time.Time{}
		stored.LastVerifyLatencyMS = 0
		stored.LastVerifyError = ""
		return
	}
	status := strings.TrimSpace(verification.Status)
	if status == "" {
		if verification.OK {
			status = VerificationVerified
		} else {
			status = VerificationFailed
		}
	}
	stored.VerificationStatus = status
	stored.LastVerifyLatencyMS = maxInt64(0, verification.LatencyMS)
	stored.LastVerifyError = ""
	stored.LastVerifiedAt = time.Time{}
	if verification.OK {
		if verifiedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(verification.VerifiedAt)); err == nil {
			stored.LastVerifiedAt = verifiedAt.UTC()
		}
		return
	}
	stored.LastVerifyError = strings.TrimSpace(verification.Message)
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func (m *ServiceSettingsManager) ClearKey(ctx context.Context, ownerID string) (ServiceMetadata, error) {
	if m == nil || m.repository == nil {
		return ServiceMetadata{}, errors.New("AI service settings repository is not configured")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return ServiceMetadata{}, ErrOwnerRequired
	}
	if err := m.repository.ClearKey(ctx, ownerID); err != nil {
		return ServiceMetadata{}, err
	}
	return m.Metadata(ctx, ownerID)
}

func ApplyServiceSettingsPatch(base ServiceSettings, patch ServiceSettingsPatch) (ServiceSettings, error) {
	if strings.TrimSpace(patch.Provider) != "" {
		base.Provider = patch.Provider
	}
	if strings.TrimSpace(patch.BaseURL) != "" {
		base.BaseURL = patch.BaseURL
	}
	if strings.TrimSpace(patch.Model) != "" {
		base.Model = patch.Model
	}
	if strings.TrimSpace(patch.AnalysisEngine) != "" {
		base.AnalysisEngine = patch.AnalysisEngine
	}
	if strings.TrimSpace(patch.FallbackPolicy) != "" {
		base.FallbackPolicy = patch.FallbackPolicy
	}
	if patch.TimeoutSeconds > 0 {
		base.Timeout = time.Duration(patch.TimeoutSeconds) * time.Second
	}
	if patch.APIKey != nil && strings.TrimSpace(*patch.APIKey) != "" {
		base.APIKey = strings.TrimSpace(*patch.APIKey)
		base.Source = ConfigSourceWorkspace
	}
	return NormalizeServiceSettings(base)
}
