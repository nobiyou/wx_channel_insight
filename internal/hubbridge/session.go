package hubbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Identity is the Hub-authenticated identity mapped into Insight workspace ownership.
type Identity struct {
	HubUserID   string
	DeviceCount int
	Email       string
	Role        string
	Source      string
}

// SessionResolver resolves a Hub Bearer JWT into an Insight identity.
type SessionResolver interface {
	Resolve(ctx context.Context, bearerToken string) (Identity, error)
}

// HTTPClient validates Hub JWTs by calling Hub's own profile / device APIs.
// Insight does not share Hub's JWT secret; Hub remains the auth owner.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
	now        func() time.Time
	cacheTTL   time.Duration

	mu    sync.Mutex
	cache map[string]cachedIdentity
}

type cachedIdentity struct {
	identity Identity
	expires  time.Time
}

// NewHTTPClient builds a Hub session client. baseURL is the Hub origin, e.g. http://127.0.0.1:8080.
func NewHTTPClient(baseURL string, httpClient *http.Client) (*HTTPClient, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("hub base URL is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &HTTPClient{
		baseURL:    trimmed,
		httpClient: httpClient,
		now:        time.Now,
		cacheTTL:   30 * time.Second,
		cache:      map[string]cachedIdentity{},
	}, nil
}

func (c *HTTPClient) SetNow(now func() time.Time) {
	if now != nil {
		c.now = now
	}
}

func (c *HTTPClient) SetCacheTTL(ttl time.Duration) {
	if ttl >= 0 {
		c.cacheTTL = ttl
	}
}

func (c *HTTPClient) Resolve(ctx context.Context, bearerToken string) (Identity, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return Identity{}, fmt.Errorf("hub bearer token is required")
	}

	if identity, ok := c.lookupCache(token); ok {
		return identity, nil
	}

	profile, err := c.fetchProfile(ctx, token)
	if err != nil {
		return Identity{}, err
	}
	deviceCount, err := c.fetchDeviceCount(ctx, token)
	if err != nil {
		// Profile is enough to establish ownership; device count is best-effort.
		deviceCount = 0
	}

	identity := Identity{
		HubUserID:   FormatHubUserID(profile.ID),
		DeviceCount: deviceCount,
		Email:       strings.TrimSpace(profile.Email),
		Role:        strings.TrimSpace(profile.Role),
		Source:      "hub_session",
	}
	if identity.HubUserID == "" {
		return Identity{}, fmt.Errorf("hub profile did not include a user id")
	}
	c.storeCache(token, identity)
	return identity, nil
}

// FormatHubUserID maps Hub numeric user IDs onto the existing Insight owner key space.
func FormatHubUserID(userID uint) string {
	if userID == 0 {
		return ""
	}
	return "hub:user:" + strconv.FormatUint(uint64(userID), 10)
}

type hubProfile struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (c *HTTPClient) fetchProfile(ctx context.Context, token string) (hubProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/auth/profile", nil)
	if err != nil {
		return hubProfile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return hubProfile{}, fmt.Errorf("hub profile request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return hubProfile{}, fmt.Errorf("hub session is invalid or expired")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return hubProfile{}, fmt.Errorf("hub profile returned status %d", resp.StatusCode)
	}

	var profile hubProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return hubProfile{}, fmt.Errorf("hub profile response is not valid JSON: %w", err)
	}
	if profile.ID == 0 {
		return hubProfile{}, fmt.Errorf("hub profile missing user id")
	}
	return profile, nil
}

type deviceListResponse struct {
	Code    int             `json:"code"`
	Devices json.RawMessage `json:"devices"`
	Data    json.RawMessage `json:"data"`
}

func (c *HTTPClient) fetchDeviceCount(ctx context.Context, token string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/device/list", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("hub device list request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("hub device list returned status %d", resp.StatusCode)
	}

	var payload deviceListResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("hub device list response is not valid JSON: %w", err)
	}

	if len(payload.Devices) > 0 {
		var devices []json.RawMessage
		if err := json.Unmarshal(payload.Devices, &devices); err == nil {
			return len(devices), nil
		}
	}
	if len(payload.Data) > 0 {
		var data struct {
			DeviceCount int `json:"device_count"`
			Devices     []json.RawMessage
		}
		if err := json.Unmarshal(payload.Data, &data); err == nil {
			if data.DeviceCount > 0 {
				return data.DeviceCount, nil
			}
			if data.Devices != nil {
				return len(data.Devices), nil
			}
		}
	}
	return 0, nil
}

func (c *HTTPClient) lookupCache(token string) (Identity, bool) {
	if c.cacheTTL <= 0 {
		return Identity{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[token]
	if !ok || !c.now().Before(entry.expires) {
		if ok {
			delete(c.cache, token)
		}
		return Identity{}, false
	}
	return entry.identity, true
}

func (c *HTTPClient) storeCache(token string, identity Identity) {
	if c.cacheTTL <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[token] = cachedIdentity{
		identity: identity,
		expires:  c.now().Add(c.cacheTTL),
	}
}
