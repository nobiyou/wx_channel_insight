package insightai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	ProviderOpenAI = "openai"

	AnalysisEngineOpenAI = "openai"
	AnalysisEngineRule   = "rule-engine"

	FallbackPolicyRule  = "rule-engine"
	FallbackPolicyError = "error"

	ConfigSourceWorkspace   = "workspace"
	ConfigSourceEnvironment = "environment"
	ConfigSourceNone        = "none"

	VerificationVerified = "verified"
	VerificationPending  = "pending"
	VerificationFailed   = "failed"
	VerificationNever    = "never"

	DefaultTimeoutSeconds = 45
	MinTimeoutSeconds     = 5
	MaxTimeoutSeconds     = 300
	MaxAPIKeyLength       = 4096
	MaxBaseURLLength      = 2048
	MaxModelLength        = 200
)

var (
	ErrOwnerRequired = errors.New("ai service owner id is required")
)

// ServiceSettings is the server-side effective configuration. APIKey must never
// be copied into an HTTP response.
type ServiceSettings struct {
	Provider       string
	BaseURL        string
	Model          string
	Timeout        time.Duration
	APIKey         string
	OwnerID        string
	AnalysisEngine string
	FallbackPolicy string
	Source         string
	Verification   string
	LastVerifiedAt time.Time
	LatencyMS      int64
	LastError      string
}

// StoredServiceSettings is the only representation written to file/Postgres.
// APIKey stays on the server and must never be mapped to an HTTP response.
type StoredServiceSettings struct {
	Provider            string    `json:"provider"`
	BaseURL             string    `json:"baseUrl"`
	Model               string    `json:"model"`
	TimeoutSeconds      int       `json:"timeoutSeconds"`
	AnalysisEngine      string    `json:"analysisEngine,omitempty"`
	FallbackPolicy      string    `json:"fallbackPolicy,omitempty"`
	APIKey              string    `json:"apiKey,omitempty"`
	APIKeyCiphertext    string    `json:"apiKeyCiphertext,omitempty"`
	APIKeyNonce         string    `json:"apiKeyNonce,omitempty"`
	APIKeyFingerprint   string    `json:"apiKeyFingerprint,omitempty"`
	APIKeyHint          string    `json:"apiKeyHint,omitempty"`
	VerificationStatus  string    `json:"verificationStatus"`
	LastVerifiedAt      time.Time `json:"lastVerifiedAt,omitempty"`
	LastVerifyLatencyMS int64     `json:"lastVerifyLatencyMs,omitempty"`
	LastVerifyError     string    `json:"lastVerifyError,omitempty"`
	UpdatedAt           time.Time `json:"updatedAt,omitempty"`
}

type ServiceSettingsPatch struct {
	Provider       string
	BaseURL        string
	Model          string
	TimeoutSeconds int
	AnalysisEngine string
	FallbackPolicy string
	APIKey         *string
}

type ServiceMetadata struct {
	OwnerID                    string    `json:"ownerId"`
	Provider                   string    `json:"provider"`
	BaseURL                    string    `json:"baseUrl"`
	Model                      string    `json:"model"`
	TimeoutSeconds             int       `json:"timeoutSeconds"`
	AnalysisEngine             string    `json:"analysisEngine"`
	FallbackPolicy             string    `json:"fallbackPolicy"`
	Source                     string    `json:"source"`
	APIKeyConfigured           bool      `json:"apiKeyConfigured"`
	ModelAvailable             bool      `json:"modelAvailable"`
	ConfigurationIssue         string    `json:"configurationIssue,omitempty"`
	APIKeyHint                 string    `json:"apiKeyHint,omitempty"`
	APIKeyPersistenceAvailable bool      `json:"apiKeyPersistenceAvailable"`
	VerificationStatus         string    `json:"verificationStatus"`
	LastVerifiedAt             time.Time `json:"lastVerifiedAt,omitempty"`
	LastVerifyLatencyMS        int64     `json:"lastVerifyLatencyMs,omitempty"`
	LastVerifyError            string    `json:"lastVerifyError,omitempty"`
	UpdatedAt                  time.Time `json:"updatedAt,omitempty"`
}

type ServiceSettingsRepository interface {
	Load(context.Context, string) (StoredServiceSettings, bool, error)
	Save(context.Context, string, StoredServiceSettings) error
	ClearKey(context.Context, string) error
}

func DefaultServiceSettings() ServiceSettings {
	return ServiceSettings{
		Provider:       ProviderOpenAI,
		BaseURL:        DefaultBaseURL,
		Model:          DefaultModel,
		Timeout:        DefaultTimeoutSeconds * time.Second,
		AnalysisEngine: AnalysisEngineOpenAI,
		FallbackPolicy: FallbackPolicyRule,
		Source:         ConfigSourceNone,
		Verification:   VerificationNever,
	}
}

func NormalizeServiceSettings(settings ServiceSettings) (ServiceSettings, error) {
	settings.OwnerID = strings.TrimSpace(settings.OwnerID)
	settings.Provider = strings.ToLower(strings.TrimSpace(settings.Provider))
	if settings.Provider == "" {
		settings.Provider = ProviderOpenAI
	}
	if settings.Provider != ProviderOpenAI {
		return ServiceSettings{}, fmt.Errorf("unsupported AI provider %q", settings.Provider)
	}
	settings.AnalysisEngine = strings.ToLower(strings.TrimSpace(settings.AnalysisEngine))
	if settings.AnalysisEngine == "" {
		settings.AnalysisEngine = AnalysisEngineOpenAI
	}
	switch settings.AnalysisEngine {
	case AnalysisEngineOpenAI, AnalysisEngineRule:
	default:
		return ServiceSettings{}, fmt.Errorf("unsupported analysis engine %q", settings.AnalysisEngine)
	}
	settings.FallbackPolicy = strings.ToLower(strings.TrimSpace(settings.FallbackPolicy))
	if settings.FallbackPolicy == "" {
		settings.FallbackPolicy = FallbackPolicyRule
	}
	switch settings.FallbackPolicy {
	case FallbackPolicyRule, FallbackPolicyError:
	default:
		return ServiceSettings{}, fmt.Errorf("unsupported fallback policy %q", settings.FallbackPolicy)
	}

	settings.BaseURL = strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	if settings.BaseURL == "" {
		settings.BaseURL = DefaultBaseURL
	}
	if len(settings.BaseURL) > MaxBaseURLLength {
		return ServiceSettings{}, errors.New("AI base URL is too long")
	}
	parsed, err := url.Parse(settings.BaseURL)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return ServiceSettings{}, errors.New("AI base URL must be an absolute HTTP or HTTPS URL")
	}

	settings.Model = strings.TrimSpace(settings.Model)
	if settings.Model == "" {
		settings.Model = DefaultModel
	}
	if len(settings.Model) > MaxModelLength {
		return ServiceSettings{}, errors.New("AI model name is too long")
	}

	if settings.Timeout <= 0 {
		settings.Timeout = DefaultTimeoutSeconds * time.Second
	}
	timeoutSeconds := int(settings.Timeout / time.Second)
	if timeoutSeconds < MinTimeoutSeconds || timeoutSeconds > MaxTimeoutSeconds {
		return ServiceSettings{}, fmt.Errorf("AI timeout must be between %d and %d seconds", MinTimeoutSeconds, MaxTimeoutSeconds)
	}
	settings.Timeout = time.Duration(timeoutSeconds) * time.Second
	settings.APIKey = strings.TrimSpace(settings.APIKey)
	if len(settings.APIKey) > MaxAPIKeyLength {
		return ServiceSettings{}, errors.New("AI API key is too long")
	}
	if settings.Source == "" {
		if settings.APIKey != "" {
			settings.Source = ConfigSourceWorkspace
		} else {
			settings.Source = ConfigSourceNone
		}
	}
	if settings.Verification == "" {
		settings.Verification = VerificationNever
	}
	return settings, nil
}

func SettingsFromStored(stored StoredServiceSettings) (ServiceSettings, error) {
	settings := ServiceSettings{
		Provider:       stored.Provider,
		BaseURL:        stored.BaseURL,
		Model:          stored.Model,
		Timeout:        time.Duration(stored.TimeoutSeconds) * time.Second,
		AnalysisEngine: stored.AnalysisEngine,
		FallbackPolicy: stored.FallbackPolicy,
		Source:         ConfigSourceWorkspace,
		Verification:   stored.VerificationStatus,
		LastVerifiedAt: stored.LastVerifiedAt,
		LatencyMS:      stored.LastVerifyLatencyMS,
		LastError:      stored.LastVerifyError,
	}
	settings.APIKey = strings.TrimSpace(stored.APIKey)
	return NormalizeServiceSettings(settings)
}

func StoredMetadata(stored StoredServiceSettings) ServiceMetadata {
	return ServiceMetadata{
		Provider:            stored.Provider,
		BaseURL:             stored.BaseURL,
		Model:               stored.Model,
		TimeoutSeconds:      stored.TimeoutSeconds,
		AnalysisEngine:      firstNonEmpty(stored.AnalysisEngine, AnalysisEngineOpenAI),
		FallbackPolicy:      firstNonEmpty(stored.FallbackPolicy, FallbackPolicyRule),
		Source:              ConfigSourceWorkspace,
		APIKeyConfigured:    strings.TrimSpace(stored.APIKey) != "" || stored.APIKeyFingerprint != "" || stored.APIKeyHint != "",
		APIKeyHint:          firstNonEmpty(stored.APIKeyHint, APIKeyHint(stored.APIKey)),
		VerificationStatus:  stored.VerificationStatus,
		LastVerifiedAt:      stored.LastVerifiedAt,
		LastVerifyLatencyMS: stored.LastVerifyLatencyMS,
		LastVerifyError:     stored.LastVerifyError,
		UpdatedAt:           stored.UpdatedAt,
	}
}

func NormalizeStoredServiceSettings(stored StoredServiceSettings) (StoredServiceSettings, error) {
	normalized, err := NormalizeServiceSettings(ServiceSettings{
		Provider:       stored.Provider,
		BaseURL:        stored.BaseURL,
		Model:          stored.Model,
		Timeout:        time.Duration(stored.TimeoutSeconds) * time.Second,
		AnalysisEngine: stored.AnalysisEngine,
		FallbackPolicy: stored.FallbackPolicy,
	})
	if err != nil {
		return StoredServiceSettings{}, err
	}
	stored.Provider = normalized.Provider
	stored.BaseURL = normalized.BaseURL
	stored.Model = normalized.Model
	stored.TimeoutSeconds = int(normalized.Timeout / time.Second)
	stored.AnalysisEngine = normalized.AnalysisEngine
	stored.FallbackPolicy = normalized.FallbackPolicy
	stored.APIKey = strings.TrimSpace(stored.APIKey)
	if len(stored.APIKey) > MaxAPIKeyLength {
		return StoredServiceSettings{}, errors.New("AI API key is too long")
	}
	stored.APIKeyFingerprint = strings.TrimSpace(stored.APIKeyFingerprint)
	stored.APIKeyHint = strings.TrimSpace(stored.APIKeyHint)
	if stored.APIKey == "" && strings.TrimSpace(stored.APIKeyCiphertext) == "" && strings.TrimSpace(stored.APIKeyNonce) == "" {
		stored.APIKeyFingerprint = ""
		stored.APIKeyHint = ""
	} else if stored.APIKey != "" {
		stored.APIKeyFingerprint = APIKeyFingerprint(stored.APIKey)
		stored.APIKeyHint = APIKeyHint(stored.APIKey)
	} else if strings.TrimSpace(stored.APIKeyCiphertext) == "" || strings.TrimSpace(stored.APIKeyNonce) == "" {
		return StoredServiceSettings{}, errors.New("AI config ciphertext and nonce must be provided together")
	}
	switch stored.VerificationStatus {
	case "":
		stored.VerificationStatus = VerificationNever
	case VerificationVerified, VerificationPending, VerificationFailed, VerificationNever:
	default:
		return StoredServiceSettings{}, fmt.Errorf("invalid AI verification status %q", stored.VerificationStatus)
	}
	stored.LastVerifyError = strings.TrimSpace(stored.LastVerifyError)
	return stored, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func MetadataFromEffective(settings ServiceSettings) (ServiceMetadata, error) {
	normalized, err := NormalizeServiceSettings(settings)
	if err != nil {
		return ServiceMetadata{}, err
	}
	hint := APIKeyHint(normalized.APIKey)
	modelAvailable := normalized.AnalysisEngine == AnalysisEngineOpenAI && normalized.APIKey != ""
	configurationIssue := ""
	if normalized.AnalysisEngine == AnalysisEngineRule {
		configurationIssue = "analysis engine is configured to rule-engine"
	} else if normalized.APIKey == "" {
		configurationIssue = ErrNotConfigured.Error()
	}
	return ServiceMetadata{
		OwnerID:             normalized.OwnerID,
		Provider:            normalized.Provider,
		BaseURL:             normalized.BaseURL,
		Model:               normalized.Model,
		TimeoutSeconds:      int(normalized.Timeout / time.Second),
		AnalysisEngine:      normalized.AnalysisEngine,
		FallbackPolicy:      normalized.FallbackPolicy,
		Source:              normalized.Source,
		APIKeyConfigured:    normalized.APIKey != "",
		ModelAvailable:      modelAvailable,
		ConfigurationIssue:  configurationIssue,
		APIKeyHint:          hint,
		VerificationStatus:  normalized.Verification,
		LastVerifiedAt:      normalized.LastVerifiedAt,
		LastVerifyLatencyMS: normalized.LatencyMS,
		LastVerifyError:     normalized.LastError,
	}, nil
}

func APIKeyHint(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ""
	}
	if len(apiKey) <= 4 {
		return "****"
	}
	return "****" + apiKey[len(apiKey)-4:]
}

func APIKeyFingerprint(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(digest[:8])
}

type MemoryServiceSettingsRepository struct {
	mu    sync.RWMutex
	items map[string]StoredServiceSettings
}

func NewMemoryServiceSettingsRepository() *MemoryServiceSettingsRepository {
	return &MemoryServiceSettingsRepository{items: make(map[string]StoredServiceSettings)}
}

func (r *MemoryServiceSettingsRepository) Load(_ context.Context, ownerID string) (StoredServiceSettings, bool, error) {
	if r == nil {
		return StoredServiceSettings{}, false, errors.New("AI service settings repository is nil")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return StoredServiceSettings{}, false, ErrOwnerRequired
	}
	r.mu.RLock()
	settings, ok := r.items[ownerID]
	r.mu.RUnlock()
	return settings, ok, nil
}

func (r *MemoryServiceSettingsRepository) Save(_ context.Context, ownerID string, settings StoredServiceSettings) error {
	if r == nil {
		return errors.New("AI service settings repository is nil")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return ErrOwnerRequired
	}
	settings, err := NormalizeStoredServiceSettings(settings)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.items[ownerID] = settings
	r.mu.Unlock()
	return nil
}

func (r *MemoryServiceSettingsRepository) ClearKey(_ context.Context, ownerID string) error {
	if r == nil {
		return errors.New("AI service settings repository is nil")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return ErrOwnerRequired
	}
	r.mu.Lock()
	settings, ok := r.items[ownerID]
	if ok {
		settings.APIKey = ""
		settings.APIKeyFingerprint = ""
		settings.APIKeyHint = ""
		settings.VerificationStatus = VerificationNever
		settings.LastVerifiedAt = time.Time{}
		settings.LastVerifyLatencyMS = 0
		settings.LastVerifyError = ""
		settings.UpdatedAt = time.Now().UTC()
		r.items[ownerID] = settings
	}
	r.mu.Unlock()
	return nil
}
