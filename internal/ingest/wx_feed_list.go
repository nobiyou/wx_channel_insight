package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// FeedListTransport optionally fetches search/feed without local HTTP
// (multi-LAN edge gateway on the Insight server).
type FeedListTransport interface {
	SearchAccounts(ctx context.Context, keyword string) (json.RawMessage, error)
	SearchVideos(ctx context.Context, keyword string) (json.RawMessage, error)
	FetchFeedList(ctx context.Context, username string) (json.RawMessage, error)
}

// FeedProfileTransport optionally fetches an exact video profile through a
// local wx_channel client or a remote edge node.
type FeedProfileTransport interface {
	FetchVideoProfile(ctx context.Context, objectID, nonceID string) (json.RawMessage, error)
}

// FeedShareTransport resolves one wx_channel share URL through a local or
// remote edge client. The returned payload is parsed before it enters the
// persisted event and observation pipeline.
type FeedShareTransport interface {
	ResolveVideoShare(ctx context.Context, shareURL string) (json.RawMessage, error)
}

type WXFeedListClient struct {
	StartMarker string
	MaxPages    int
	BaseURL     string
	Client      *http.Client
	Transport   FeedListTransport
}

type WXFeedListResult struct {
	// Latest-page refreshes are observations, not additional historical coverage.
	refreshedHeadItems int
	refreshedHeadPages int
	PageCount          int
	Complete           bool
	Notice             string
	Username           string
	NextMarker         string
	Items              []WXFeedListItem
}

type WXFeedListItem struct {
	MissingMetrics  MetricMask
	ID              string
	ObjectNonceID   string
	Title           string
	ChannelName     string
	ChannelUsername string
	PublishedAt     time.Time
	Tags            []string
	CoverURL        string
	VideoURL        string
	// DecryptKey is request-time media metadata. It is intentionally not
	// persisted with observations because wx_channel video URLs and keys expire.
	DecryptKey   string
	PlayCount    int
	LikeCount    int
	CommentCount int
	FavCount     int
	ForwardCount int
	Signal       string
}

func (c WXFeedListClient) SearchAccounts(ctx context.Context, keyword string) ([]WXFeedAccount, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}

	if c.Transport != nil {
		raw, err := c.Transport.SearchAccounts(ctx, keyword)
		if err == nil {
			return parseWXFeedAccountsFromSearchPayload(raw)
		}
		if strings.TrimSpace(c.BaseURL) == "" {
			return nil, err
		}
	}

	baseURL := normalizeWXFeedListBaseURL(c.BaseURL)

	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}

	endpoint := baseURL + "/api/channels/contact/search?keyword=" + url.QueryEscape(keyword)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, normalizeWXFeedListFetchError(baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("wx_channel api returned HTTP %d", resp.StatusCode)
		var payload struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		if readErr == nil && len(body) > 0 {
			if err := json.Unmarshal(body, &payload); err == nil {
				switch {
				case strings.TrimSpace(payload.Message) != "":
					message = strings.TrimSpace(payload.Message)
				case strings.TrimSpace(payload.Error) != "":
					message = strings.TrimSpace(payload.Error)
				}
			}
		}
		return nil, normalizeWXFeedListMessage(baseURL, message)
	}

	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ErrCode int    `json:"errCode"`
			ErrMsg  string `json:"errMsg"`
			Data    struct {
				BaseResponse struct {
					Ret    int `json:"Ret"`
					ErrMsg struct {
						String string `json:"String"`
					} `json:"ErrMsg"`
				} `json:"BaseResponse"`
				InfoList []map[string]any `json:"infoList"`
			} `json:"data"`
			InfoList []map[string]any `json:"infoList"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("wx_channel api error: %s", payload.Message)
	}
	if payload.Data.ErrCode != 0 {
		message := strings.TrimSpace(payload.Data.ErrMsg)
		if message == "" {
			message = fmt.Sprintf("wx_channel api error code %d", payload.Data.ErrCode)
		}
		return nil, normalizeWXFeedListMessage(baseURL, message)
	}
	if payload.Data.Data.BaseResponse.Ret != 0 {
		message := strings.TrimSpace(payload.Data.Data.BaseResponse.ErrMsg.String)
		if message == "" {
			message = fmt.Sprintf("wx_channel api error code %d", payload.Data.Data.BaseResponse.Ret)
		}
		return nil, normalizeWXFeedListMessage(baseURL, message)
	}

	infoList := payload.Data.Data.InfoList
	if len(infoList) == 0 {
		infoList = payload.Data.InfoList
	}

	results := make([]WXFeedAccount, 0, len(infoList))
	seen := make(map[string]struct{}, len(infoList))
	for _, item := range infoList {
		account := parseWXFeedAccount(item)
		if account.Username == "" {
			continue
		}
		if _, exists := seen[account.Username]; exists {
			continue
		}
		seen[account.Username] = struct{}{}
		results = append(results, account)
	}
	return results, nil
}

// ResolveAccount verifies and loads one exact video account username. The
// contact search API is intentionally fuzzy, so a full @finder username must
// use the feed-list API instead of being sent back as a keyword.
func (c WXFeedListClient) ResolveAccount(ctx context.Context, username string) (WXFeedAccount, error) {
	username = strings.TrimSpace(username)
	if !looksLikeWXFeedFinderUsername(username) {
		return WXFeedAccount{}, fmt.Errorf("当前填写的 %q 不是完整的 @finder username", username)
	}

	if c.Transport != nil {
		raw, err := c.Transport.FetchFeedList(ctx, username)
		if err == nil {
			account, parseErr := parseWXFeedAccountFromFeedListPayload(username, raw)
			if parseErr == nil {
				return account, nil
			}
			if strings.TrimSpace(c.BaseURL) == "" {
				return WXFeedAccount{}, parseErr
			}
		} else if strings.TrimSpace(c.BaseURL) == "" {
			return WXFeedAccount{}, err
		}
	}

	baseURL := normalizeWXFeedListBaseURL(c.BaseURL)
	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}

	endpoint := baseURL + "/api/channels/contact/feed/list?username=" + url.QueryEscape(username)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return WXFeedAccount{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return WXFeedAccount{}, normalizeWXFeedListFetchError(baseURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return WXFeedAccount{}, err
	}
	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("wx_channel api returned HTTP %d", resp.StatusCode)
		var payload struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if len(body) > 0 {
			if decodeErr := json.Unmarshal(body, &payload); decodeErr == nil {
				switch {
				case strings.TrimSpace(payload.Message) != "":
					message = strings.TrimSpace(payload.Message)
				case strings.TrimSpace(payload.Error) != "":
					message = strings.TrimSpace(payload.Error)
				}
			}
		}
		return WXFeedAccount{}, normalizeWXFeedListMessage(baseURL, message)
	}

	return parseWXFeedAccountFromFeedListPayload(username, body)
}

func (c WXFeedListClient) SearchVideos(ctx context.Context, keyword string) (WXFeedListResult, error) {
	return c.collectPages(ctx, keyword, true)
}

func (c WXFeedListClient) SearchVideosPage(ctx context.Context, keyword, marker string) (WXFeedListResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return WXFeedListResult{}, fmt.Errorf("keyword is required")
	}

	if c.Transport != nil {
		var raw json.RawMessage
		var err error
		if transport, ok := c.Transport.(FeedPageTransport); ok {
			raw, err = transport.SearchVideosPage(ctx, keyword, marker)
		} else if marker == "" {
			raw, err = c.Transport.SearchVideos(ctx, keyword)
		} else {
			return WXFeedListResult{}, errors.New("采集连接不支持分页，请更新采集连接")
		}
		if err == nil {
			return parseWXFeedListResultFromPayload(keyword, raw)
		}
		if strings.TrimSpace(c.BaseURL) == "" {
			return WXFeedListResult{}, err
		}
	}

	baseURL := normalizeWXFeedListBaseURL(c.BaseURL)

	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}

	endpoint := baseURL + "/api/channels/feed/search?keyword=" + url.QueryEscape(keyword)
	if marker != "" {
		endpoint += "&next_marker=" + url.QueryEscape(marker)
	}
	var resp *http.Response
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if reqErr != nil {
			return WXFeedListResult{}, reqErr
		}

		resp, err = httpClient.Do(req)
		if err == nil {
			break
		}
		if shouldRetryWXFeedListTimeout(ctx, err, attempt) {
			continue
		}
		return WXFeedListResult{}, normalizeWXFeedListFetchError(baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("wx_channel api returned HTTP %d", resp.StatusCode)
		var payload struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		if readErr == nil && len(body) > 0 {
			if err := json.Unmarshal(body, &payload); err == nil {
				switch {
				case strings.TrimSpace(payload.Message) != "":
					message = strings.TrimSpace(payload.Message)
				case strings.TrimSpace(payload.Error) != "":
					message = strings.TrimSpace(payload.Error)
				}
			}
		}
		return WXFeedListResult{}, normalizeWXFeedListMessage(baseURL, message)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return WXFeedListResult{}, err
	}
	return parseWXFeedListResultFromPayload(keyword, body)
}

// FetchVideoProfile loads one exact video by object_id and nonce_id. Search
// results are intentionally bounded by wx_channel, so historical Insight
// rows may need this exact lookup when keyword search no longer returns them.
func (c WXFeedListClient) FetchVideoProfile(ctx context.Context, objectID, nonceID string) (WXFeedListItem, error) {
	objectID = strings.TrimSpace(objectID)
	nonceID = strings.TrimSpace(nonceID)
	if objectID == "" {
		return WXFeedListItem{}, fmt.Errorf("object_id is required")
	}

	if transport, ok := c.Transport.(FeedProfileTransport); ok {
		raw, err := transport.FetchVideoProfile(ctx, objectID, nonceID)
		if err == nil {
			item, parseErr := parseWXFeedProfileItem(objectID, raw)
			if parseErr == nil {
				return item, nil
			}
			if strings.TrimSpace(c.BaseURL) == "" {
				return WXFeedListItem{}, parseErr
			}
		} else if strings.TrimSpace(c.BaseURL) == "" {
			return WXFeedListItem{}, err
		}
	}

	baseURL := normalizeWXFeedListBaseURL(c.BaseURL)
	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	endpoint := baseURL + "/api/channels/feed/profile?object_id=" + url.QueryEscape(objectID)
	if nonceID != "" {
		endpoint += "&nonce_id=" + url.QueryEscape(nonceID)
	}

	var resp *http.Response
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if reqErr != nil {
			return WXFeedListItem{}, reqErr
		}
		resp, err = httpClient.Do(req)
		if err == nil {
			break
		}
		if shouldRetryWXFeedListTimeout(ctx, err, attempt) {
			continue
		}
		return WXFeedListItem{}, normalizeWXFeedListFetchError(baseURL, err)
	}
	if resp == nil {
		return WXFeedListItem{}, normalizeWXFeedListFetchError(baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("wx_channel api returned HTTP %d", resp.StatusCode)
		var payload struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		if readErr == nil && len(body) > 0 {
			if unmarshalErr := json.Unmarshal(body, &payload); unmarshalErr == nil {
				switch {
				case strings.TrimSpace(payload.Message) != "":
					message = strings.TrimSpace(payload.Message)
				case strings.TrimSpace(payload.Error) != "":
					message = strings.TrimSpace(payload.Error)
				}
			}
		}
		return WXFeedListItem{}, normalizeWXFeedListMessage(baseURL, message)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return WXFeedListItem{}, err
	}
	return parseWXFeedProfileItem(objectID, body)
}

// ResolveVideoShare resolves a supported wx_channel share URL into the same
// video item shape used by feed-list and video-search imports.
func (c WXFeedListClient) ResolveVideoShare(ctx context.Context, shareURL string) (WXFeedListItem, error) {
	shareURL, err := normalizeWXSharedFeedURL(shareURL)
	if err != nil {
		return WXFeedListItem{}, err
	}

	if transport, ok := c.Transport.(FeedShareTransport); ok {
		raw, transportErr := transport.ResolveVideoShare(ctx, shareURL)
		if transportErr == nil {
			item, parseErr := parseWXSharedFeedProfileItem(raw)
			if parseErr == nil {
				return item, nil
			}
			if strings.TrimSpace(c.BaseURL) == "" {
				return WXFeedListItem{}, parseErr
			}
		} else if strings.TrimSpace(c.BaseURL) == "" {
			return WXFeedListItem{}, transportErr
		}
	}

	baseURL := normalizeWXFeedListBaseURL(c.BaseURL)
	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	endpoint := baseURL + "/api/channels/feed/profile?url=" + url.QueryEscape(shareURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return WXFeedListItem{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return WXFeedListItem{}, normalizeWXFeedListFetchError(baseURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return WXFeedListItem{}, err
	}
	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("wx_channel api returned HTTP %d", resp.StatusCode)
		var payload struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if len(body) > 0 {
			if decodeErr := json.Unmarshal(body, &payload); decodeErr == nil {
				switch {
				case strings.TrimSpace(payload.Message) != "":
					message = strings.TrimSpace(payload.Message)
				case strings.TrimSpace(payload.Error) != "":
					message = strings.TrimSpace(payload.Error)
				}
			}
		}
		return WXFeedListItem{}, normalizeWXFeedListMessage(baseURL, message)
	}
	return parseWXSharedFeedProfileItem(body)
}

func (c WXFeedListClient) Fetch(ctx context.Context, username string) (WXFeedListResult, error) {
	return c.collectPages(ctx, username, false)
}

func (c WXFeedListClient) FetchPage(ctx context.Context, username, marker string) (WXFeedListResult, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return WXFeedListResult{}, fmt.Errorf("username is required")
	}

	if c.Transport != nil {
		var raw json.RawMessage
		var err error
		if transport, ok := c.Transport.(FeedPageTransport); ok {
			raw, err = transport.FetchFeedListPage(ctx, username, marker)
		} else if marker == "" {
			raw, err = c.Transport.FetchFeedList(ctx, username)
		} else {
			return WXFeedListResult{}, errors.New("采集连接不支持分页，请更新采集连接")
		}
		if err == nil {
			return parseWXFeedListResultFromPayload(username, raw)
		}
		if strings.TrimSpace(c.BaseURL) == "" {
			return WXFeedListResult{}, err
		}
	}

	baseURL := normalizeWXFeedListBaseURL(c.BaseURL)

	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}

	endpoint := baseURL + "/api/channels/contact/feed/list?username=" + url.QueryEscape(username)
	if marker != "" {
		endpoint += "&next_marker=" + url.QueryEscape(marker)
	}
	var resp *http.Response
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return WXFeedListResult{}, err
		}

		resp, err = httpClient.Do(req)
		if err == nil {
			break
		}
		if shouldRetryWXFeedListTimeout(ctx, err, attempt) {
			continue
		}
		return WXFeedListResult{}, normalizeWXFeedListFetchError(baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("wx_channel api returned HTTP %d", resp.StatusCode)
		var payload struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		if readErr == nil && len(body) > 0 {
			if err := json.Unmarshal(body, &payload); err == nil {
				switch {
				case strings.TrimSpace(payload.Message) != "":
					message = strings.TrimSpace(payload.Message)
				case strings.TrimSpace(payload.Error) != "":
					message = strings.TrimSpace(payload.Error)
				}
			}
		}
		return WXFeedListResult{}, normalizeWXFeedListMessage(baseURL, message)
	}

	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Object     []map[string]any `json:"object"`
			ObjectList []map[string]any `json:"objectList"`
			LastBuffer string           `json:"lastBuffer"`
			ErrCode    int              `json:"errCode"`
			ErrMsg     string           `json:"errMsg"`
			Data       struct {
				BaseResponse struct {
					Ret    int `json:"Ret"`
					ErrMsg struct {
						String string `json:"String"`
					} `json:"ErrMsg"`
				} `json:"BaseResponse"`
				Object     []map[string]any `json:"object"`
				ObjectList []map[string]any `json:"objectList"`
				LastBuffer string           `json:"lastBuffer"`
			} `json:"data"`
			Payload struct {
				Username       string `json:"username"`
				FinderUsername string `json:"finderUsername"`
			} `json:"payload"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return WXFeedListResult{}, err
	}
	if payload.Code != 0 {
		return WXFeedListResult{}, fmt.Errorf("wx_channel api error: %s", payload.Message)
	}
	if payload.Data.ErrCode != 0 {
		message := strings.TrimSpace(payload.Data.ErrMsg)
		if message == "" {
			message = fmt.Sprintf("wx_channel api error code %d", payload.Data.ErrCode)
		}
		return WXFeedListResult{}, normalizeWXFeedListMessage(baseURL, message)
	}
	if payload.Data.Data.BaseResponse.Ret != 0 {
		return WXFeedListResult{}, buildWXFeedListFinderUsernameError(username, payload.Data.Data.BaseResponse.Ret)
	}

	objects := payload.Data.ObjectList
	if len(objects) == 0 {
		objects = payload.Data.Object
	}
	if len(objects) == 0 {
		objects = payload.Data.Data.ObjectList
	}
	if len(objects) == 0 {
		objects = payload.Data.Data.Object
	}

	nextMarker := payload.Data.LastBuffer
	if nextMarker == "" {
		nextMarker = payload.Data.Data.LastBuffer
	}

	result := WXFeedListResult{
		Username:   username,
		NextMarker: nextMarker,
		Items:      make([]WXFeedListItem, 0, len(objects)),
	}
	for _, item := range objects {
		parsed := parseWXFeedListItem(username, item)
		if parsed.ID == "" {
			continue
		}
		result.Items = append(result.Items, parsed)
	}
	return result, nil
}

func parseWXFeedProfileItem(objectID string, raw json.RawMessage) (WXFeedListItem, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return WXFeedListItem{}, err
	}
	if code := intAny(root["code"]); code != 0 {
		message := stringAny(root["message"])
		if message == "" {
			message = fmt.Sprintf("wx_channel api error code %d", code)
		}
		return WXFeedListItem{}, fmt.Errorf("%s", message)
	}
	object := findWXFeedProfileObject(root, 0)
	if object == nil {
		return WXFeedListItem{}, fmt.Errorf("wx_channel profile response did not contain video object %s", objectID)
	}
	item := parseWXFeedListItem("", object)
	if item.ID == "" {
		item.ID = objectID
	}
	if item.ID != objectID {
		return WXFeedListItem{}, fmt.Errorf("wx_channel profile returned object %s instead of %s", item.ID, objectID)
	}
	return item, nil
}

func parseWXSharedFeedProfileItem(raw json.RawMessage) (WXFeedListItem, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return WXFeedListItem{}, err
	}
	if code := intAny(firstNonNilAny(root["code"], root["errCode"])); code != 0 {
		message := firstNonEmptyWXFeedString(root["message"], root["errMsg"])
		if message == "" {
			message = fmt.Sprintf("wx_channel api error code %d", code)
		}
		return WXFeedListItem{}, fmt.Errorf("%s", message)
	}
	object := findWXFeedProfileObject(root, 0)
	if object == nil {
		return WXFeedListItem{}, fmt.Errorf("wx_channel shared profile response did not contain a video object")
	}
	item := parseWXFeedListItem("", object)
	if item.ID == "" {
		return WXFeedListItem{}, fmt.Errorf("wx_channel shared profile response did not contain a video id")
	}
	return item, nil
}

func findWXFeedProfileObject(value any, depth int) map[string]any {
	if depth > 6 {
		return nil
	}
	switch cast := value.(type) {
	case map[string]any:
		// feed/profile may return the video object directly, while older
		// responses wrap it under data.data.object.
		if wxFeedObjectID(cast) != "" {
			if _, hasDescription := cast["objectDesc"]; hasDescription {
				return cast
			}
		}
		if object, ok := cast["object"].(map[string]any); ok && wxFeedObjectID(object) != "" {
			return object
		}
		for _, key := range []string{"data", "payload"} {
			if nested, ok := cast[key]; ok {
				if object := findWXFeedProfileObject(nested, depth+1); object != nil {
					return object
				}
			}
		}
	}
	return nil
}

func normalizeWXFeedListBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:2026"
	}
	return baseURL
}

func normalizeWXSharedFeedURL(raw string) (string, error) {
	raw = strings.TrimSpace(strings.Trim(raw, "\"'"))
	if raw == "" {
		return "", fmt.Errorf("分享链接不能为空")
	}
	if len(raw) > 4096 || strings.ContainsAny(raw, "\r\n") {
		return "", fmt.Errorf("分享链接格式无效")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" || parsed.Port() != "" {
		return "", fmt.Errorf("分享链接格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("分享链接必须使用 HTTP 或 HTTPS")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	path := strings.TrimRight(parsed.Path, "/")
	supported := (host == "weixin.qq.com" && strings.HasPrefix(path, "/sph/") && len(strings.TrimPrefix(path, "/sph/")) > 0) ||
		(host == "channels.weixin.qq.com" && strings.HasPrefix(path, "/finder-preview/pages/sph") && len(path) > len("/finder-preview/pages/sph"))
	if !supported {
		return "", fmt.Errorf("仅支持微信视频号分享链接")
	}
	return parsed.String(), nil
}

func wxFeedObjectID(item map[string]any) string {
	return firstNonEmptyWXFeedString(item["id"], item["objectId"], item["objectID"])
}

func BuildEventsFromWXFeedList(hubUserID, machineID string, result WXFeedListResult, now time.Time) []Event {
	return buildEventsFromWXFeedListSource(hubUserID, machineID, result, now, "wx_channel_feed_list", "")
}

func BuildEventsFromWXVideoSearch(hubUserID, machineID string, result WXFeedListResult, keyword string, now time.Time) []Event {
	return buildEventsFromWXFeedListSource(hubUserID, machineID, result, now, RunSourceWXVideoSearch, strings.TrimSpace(keyword))
}

func buildEventsFromWXFeedListSource(hubUserID, machineID string, result WXFeedListResult, now time.Time, source string, keyword string) []Event {
	events := make([]Event, 0, len(result.Items))
	for _, item := range result.Items {
		channelUsername := strings.TrimSpace(item.ChannelUsername)
		// A feed-list binding identifies the requested account. When an item
		// omits its contact object, that saved identity is still authoritative
		// for this feed source. Search results do not get this fallback.
		if channelUsername == "" && source == "wx_channel_feed_list" {
			channelUsername = strings.TrimSpace(result.Username)
		}
		events = append(events, Event{
			MissingMetrics:  item.MissingMetrics,
			ID:              buildWXFeedListEventID(source, item.ID),
			HubUserID:       hubUserID,
			MachineID:       machineID,
			EventType:       "video_snapshot",
			Source:          source,
			CollectedAt:     now,
			PublishedAt:     item.PublishedAt,
			ObjectID:        item.ID,
			ObjectNonceID:   item.ObjectNonceID,
			ChannelUsername: channelUsername,
			CoverURL:        item.CoverURL,
			VideoCount:      1,
			PlayCount:       maxInt(0, item.PlayCount),
			LikeCount:       maxInt(0, item.LikeCount),
			CommentCount:    maxInt(0, item.CommentCount),
			FavCount:        maxInt(0, item.FavCount),
			ForwardCount:    maxInt(0, item.ForwardCount),
			TopicCount:      maxInt(1, len(item.Tags)),
			Title:           item.Title,
			ChannelName:     item.ChannelName,
			Keyword:         keyword,
			Signal:          item.Signal,
			Tags:            append([]string(nil), item.Tags...),
		})
	}
	return events
}

func buildWXFeedListEventID(source, objectID string) string {
	if source == RunSourceWXVideoSearch {
		return fmt.Sprintf("wx-video-search-%s", objectID)
	}
	return fmt.Sprintf("wx-feed-video-%s", objectID)
}

func BuildEventFromWXManualShare(hubUserID, machineID string, item WXFeedListItem, now time.Time) Event {
	return Event{
		MissingMetrics:  item.MissingMetrics,
		ID:              fmt.Sprintf("wx-manual-share-%s", strings.TrimSpace(item.ID)),
		HubUserID:       hubUserID,
		MachineID:       machineID,
		EventType:       "video_snapshot",
		Source:          RunSourceWXManualShare,
		CollectedAt:     now,
		PublishedAt:     item.PublishedAt,
		ObjectID:        item.ID,
		ObjectNonceID:   item.ObjectNonceID,
		ChannelUsername: item.ChannelUsername,
		CoverURL:        item.CoverURL,
		VideoCount:      1,
		PlayCount:       maxInt(0, item.PlayCount),
		LikeCount:       maxInt(0, item.LikeCount),
		CommentCount:    maxInt(0, item.CommentCount),
		FavCount:        maxInt(0, item.FavCount),
		ForwardCount:    maxInt(0, item.ForwardCount),
		TopicCount:      maxInt(1, len(item.Tags)),
		Title:           item.Title,
		ChannelName:     item.ChannelName,
		Signal:          item.Signal,
		Tags:            append([]string(nil), item.Tags...),
	}
}

func parseWXFeedListItem(username string, item map[string]any) WXFeedListItem {
	result := WXFeedListItem{
		ID:              wxFeedObjectID(item),
		ObjectNonceID:   firstNonEmptyWXFeedString(item["objectNonceId"], item["nonceId"], item["object_nonce_id"]),
		ChannelName:     username,
		ChannelUsername: extractWXFeedChannelUsername(item),
		CoverURL:        extractWXFeedCoverURL(item),
		VideoURL:        extractWXFeedVideoURL(item),
		DecryptKey:      extractWXFeedDecryptKey(item),
		PublishedAt:     parseWXFeedListTime(firstNonNilAny(item["createtime"], item["createTime"], item["publishedAt"])),
	}

	desc, _ := item["objectDesc"].(map[string]any)
	if title := stringAny(desc["description"]); title != "" {
		result.Title = sanitizeWXText(title)
	}

	if contact := extractWXFeedContact(item); contact != nil {
		if nickname := stringAny(contact["nickname"]); nickname != "" {
			result.ChannelName = nickname
		}
	}

	if mediaList, ok := desc["media"].([]any); ok && len(mediaList) > 0 {
		if media, ok := mediaList[0].(map[string]any); ok {
			if result.CoverURL == "" {
				result.CoverURL = firstNonEmptyWXFeedString(media["thumbUrl"], media["coverUrl"], media["fullThumbUrl"])
			}
			if result.VideoURL == "" {
				result.VideoURL = joinWXFeedMediaURL(media)
			}
			if result.DecryptKey == "" {
				result.DecryptKey = firstNonEmptyWXFeedKey(media["decodeKey"], media["decryptKey"])
			}
			if result.PublishedAt.IsZero() {
				result.PublishedAt = parseWXFeedListTime(firstNonNilAny(media["createtime"], media["createTime"], media["publishedAt"]))
			}
		}
	}

	metricSources := []map[string]any{item}
	if extend, ok := item["objectExtend"].(map[string]any); ok {
		if monotonic, ok := extend["monotonicData"].(map[string]any); ok {
			if counts, ok := monotonic["countInfo"].(map[string]any); ok {
				metricSources = append(metricSources, counts)
			}
		}
	}
	if mediaList, ok := desc["media"].([]any); ok && len(mediaList) > 0 {
		if media, ok := mediaList[0].(map[string]any); ok {
			metricSources = append(metricSources, media)
		}
	}
	for _, metric := range []struct {
		key  string
		flag MetricMask
		dest *int
	}{
		{"readCount", MetricPlay, &result.PlayCount}, {"likeCount", MetricLike, &result.LikeCount},
		{"commentCount", MetricComment, &result.CommentCount}, {"favCount", MetricFavorite, &result.FavCount}, {"forwardCount", MetricForward, &result.ForwardCount},
	} {
		result.MissingMetrics |= metric.flag
		for _, source := range metricSources {
			if value, ok := source[metric.key]; ok && value != nil {
				count, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
				if err == nil && count >= 0 && int64(int(count)) == count {
					*metric.dest = int(count)
					result.MissingMetrics &^= metric.flag
					break
				}
			}
		}
	}
	result.Tags = deriveFeedTags(result.Title)
	result.Signal = deriveFeedSignal(result.CommentCount, result.Tags)
	return result
}

func extractWXFeedVideoURL(item map[string]any) string {
	if item == nil {
		return ""
	}
	if value := joinWXFeedMediaURL(item); value != "" {
		return value
	}
	for _, key := range []string{"videoUrl", "videoURL", "playUrl", "playURL"} {
		if value := stringAny(item[key]); value != "" {
			return value
		}
	}
	if desc, ok := item["objectDesc"].(map[string]any); ok {
		if value := joinWXFeedMediaURL(desc); value != "" {
			return value
		}
		if mediaList, ok := desc["media"].([]any); ok && len(mediaList) > 0 {
			if media, ok := mediaList[0].(map[string]any); ok {
				return joinWXFeedMediaURL(media)
			}
		}
	}
	return ""
}

func joinWXFeedMediaURL(value map[string]any) string {
	if value == nil {
		return ""
	}
	base := firstNonEmptyWXFeedString(value["url"], value["videoUrl"], value["videoURL"], value["playUrl"], value["playURL"])
	if base == "" {
		return ""
	}
	videoURL := base + stringAny(value["urlToken"])
	return appendWXFeedVideoSpec(videoURL, lowestWXFeedSpecFormat(value["spec"]))
}

func lowestWXFeedSpecFormat(value any) string {
	specs, ok := value.([]any)
	if !ok {
		return ""
	}

	lowestBitRate := 0
	lowestFormat := ""
	for _, rawSpec := range specs {
		spec, ok := rawSpec.(map[string]any)
		if !ok {
			continue
		}
		format := stringAny(spec["fileFormat"])
		bitRate := intAny(spec["bitRate"])
		if format == "" || bitRate <= 0 {
			continue
		}
		if lowestBitRate == 0 || bitRate < lowestBitRate {
			lowestBitRate = bitRate
			lowestFormat = format
		}
	}
	return lowestFormat
}

func appendWXFeedVideoSpec(videoURL, format string) string {
	videoURL = strings.TrimSpace(videoURL)
	format = strings.TrimSpace(format)
	if videoURL == "" || format == "" || strings.Contains(strings.ToLower(videoURL), "x-snsvideoflag=") {
		return videoURL
	}

	separator := "?"
	if strings.Contains(videoURL, "?") {
		separator = "&"
	}
	if strings.HasSuffix(videoURL, "?") || strings.HasSuffix(videoURL, "&") {
		separator = ""
	}
	return videoURL + separator + "X-snsvideoflag=" + url.QueryEscape(format)
}

func extractWXFeedDecryptKey(item map[string]any) string {
	if item == nil {
		return ""
	}
	if value := firstNonEmptyWXFeedKey(item["decodeKey"], item["decryptKey"]); value != "" {
		return value
	}
	if desc, ok := item["objectDesc"].(map[string]any); ok {
		if value := firstNonEmptyWXFeedKey(desc["decodeKey"], desc["decryptKey"]); value != "" {
			return value
		}
		if mediaList, ok := desc["media"].([]any); ok && len(mediaList) > 0 {
			if media, ok := mediaList[0].(map[string]any); ok {
				return firstNonEmptyWXFeedKey(media["decodeKey"], media["decryptKey"])
			}
		}
	}
	return ""
}

func firstNonEmptyWXFeedKey(values ...any) string {
	for _, value := range values {
		if result := decimalStringAny(value); result != "" {
			return result
		}
	}
	return ""
}

func decimalStringAny(value any) string {
	if value == nil {
		return ""
	}
	switch cast := value.(type) {
	case json.Number:
		return strings.TrimSpace(cast.String())
	case float64:
		return strconv.FormatFloat(cast, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(cast), 'f', -1, 32)
	default:
		return stringAny(value)
	}
}

func extractWXFeedCoverURL(item map[string]any) string {
	if item == nil {
		return ""
	}
	if value := firstNonEmptyWXFeedString(item["coverUrl"], item["thumbUrl"], item["fullThumbUrl"]); value != "" {
		return value
	}
	if desc, ok := item["objectDesc"].(map[string]any); ok {
		if value := firstNonEmptyWXFeedString(desc["coverUrl"], desc["thumbUrl"], desc["fullThumbUrl"]); value != "" {
			return value
		}
		if mediaList, ok := desc["media"].([]any); ok && len(mediaList) > 0 {
			if media, ok := mediaList[0].(map[string]any); ok {
				return firstNonEmptyWXFeedString(media["thumbUrl"], media["coverUrl"], media["fullThumbUrl"])
			}
		}
	}
	return ""
}

func firstNonEmptyWXFeedString(values ...any) string {
	for _, value := range values {
		if result := stringAny(value); result != "" {
			return result
		}
	}
	return ""
}

func extractWXFeedContact(item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	if contact, ok := item["contact"].(map[string]any); ok {
		return contact
	}
	if desc, ok := item["objectDesc"].(map[string]any); ok {
		for _, key := range []string{"contact", "finderContact", "author"} {
			if contact, ok := desc[key].(map[string]any); ok {
				return contact
			}
		}
	}
	return nil
}

func extractWXFeedChannelUsername(item map[string]any) string {
	contact := extractWXFeedContact(item)
	if contact != nil {
		if value := stringAny(contact["username"]); value != "" {
			return value
		}
	}
	for _, key := range []string{"channelUsername", "finderUsername"} {
		if value := stringAny(item[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstNonNilAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func parseWXFeedListTime(value any) time.Time {
	if value == nil {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339, stringAny(value)); err == nil {
		return parsed
	}

	var seconds int64
	var numeric bool
	switch typed := value.(type) {
	case float64:
		seconds = int64(typed)
		numeric = true
	case float32:
		seconds = int64(typed)
		numeric = true
	case int:
		seconds = int64(typed)
		numeric = true
	case int32:
		seconds = int64(typed)
		numeric = true
	case int64:
		seconds = typed
		numeric = true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			seconds = parsed
			numeric = true
		}
	}
	if !numeric {
		raw := strings.TrimSpace(stringAny(value))
		if raw == "" {
			return time.Time{}
		}
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			parsedFloat, floatErr := strconv.ParseFloat(raw, 64)
			if floatErr != nil {
				return time.Time{}
			}
			seconds = int64(parsedFloat)
		} else {
			seconds = parsed
		}
	}
	if seconds <= 0 {
		return time.Time{}
	}
	if seconds >= 1_000_000_000_000 {
		seconds /= 1000
	}
	return time.Unix(seconds, 0).UTC()
}

func parseWXFeedAccount(item map[string]any) WXFeedAccount {
	contact := extractWXFeedContact(item)
	if contact == nil {
		contact = map[string]any{}
	}
	account := WXFeedAccount{
		Username:  stringAny(contact["username"]),
		Nickname:  stringAny(contact["nickname"]),
		Signature: stringAny(contact["signature"]),
		HeadURL:   stringAny(contact["headUrl"]),
		Region:    parseWXFeedAccountRegion(contact, item),
		Industry:  parseWXFeedAccountIndustry(contact, item),
		Tags:      parseWXFeedAccountTags(contact, item),
	}
	if account.Nickname == "" {
		account.Nickname = stringAny(item["nickname"])
	}
	if account.Signature == "" {
		account.Signature = stringAny(item["signature"])
	}
	return account
}

func parseWXFeedAccountRegion(contact, item map[string]any) string {
	region := firstNonEmptyWXFeedString(contact["region"], contact["location"])
	if extInfo, ok := contact["extInfo"].(map[string]any); ok {
		province := firstNonEmptyWXFeedString(extInfo["province"], extInfo["state"])
		city := firstNonEmptyWXFeedString(extInfo["city"], extInfo["district"])
		region = joinWXFeedAccountRegion(province, city, region)
	}
	return firstNonEmptyWXFeedString(region, item["region"], item["location"])
}

func joinWXFeedAccountRegion(province, city, fallback string) string {
	province = strings.TrimSpace(province)
	city = strings.TrimSpace(city)
	if province != "" && city != "" {
		return province + " " + city
	}
	if province != "" {
		return province
	}
	if city != "" {
		return city
	}
	return strings.TrimSpace(fallback)
}

func parseWXFeedAccountIndustry(contact, item map[string]any) string {
	if authInfo, ok := contact["authInfo"].(map[string]any); ok {
		if industry := firstNonEmptyWXFeedString(authInfo["authProfession"], authInfo["industry"], authInfo["profession"]); industry != "" {
			return industry
		}
	}
	return firstNonEmptyWXFeedString(contact["industry"], contact["profession"], item["industry"], item["profession"])
}

func parseWXFeedAccountTags(contact, item map[string]any) []string {
	sources := []any{
		contact["tags"], contact["tagList"], contact["labels"], contact["labelList"],
		item["tags"], item["tagList"], item["labels"], item["labelList"],
	}
	if extInfo, ok := contact["extInfo"].(map[string]any); ok {
		sources = append(sources, extInfo["tags"], extInfo["tagList"], extInfo["labels"], extInfo["labelList"])
	}
	if authInfo, ok := contact["authInfo"].(map[string]any); ok {
		sources = append(sources, authInfo["tags"], authInfo["tagList"], authInfo["labels"], authInfo["labelList"])
	}

	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, source := range sources {
		for _, tag := range stringListAny(source) {
			key := strings.ToLower(tag)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, tag)
		}
	}
	return result
}

func stringListAny(value any) []string {
	switch typed := value.(type) {
	case string:
		parts := strings.FieldsFunc(typed, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == '，' || r == '、'
		})
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if normalized := strings.TrimSpace(part); normalized != "" {
				result = append(result, normalized)
			}
		}
		return result
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			result = append(result, stringListAny(item)...)
		}
		return result
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			result = append(result, stringListAny(item)...)
		}
		return result
	case map[string]any:
		for _, key := range []string{"name", "label", "tagName", "value"} {
			if nested, ok := typed[key]; ok {
				return stringListAny(nested)
			}
		}
	}
	return nil
}

func parseWXFeedAccountFromFeedListPayload(username string, raw json.RawMessage) (WXFeedAccount, error) {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return WXFeedAccount{}, err
	}
	contact := findWXFeedAccountContact(root, username, 0)
	if contact == nil {
		return WXFeedAccount{}, fmt.Errorf(
			"无法按完整 @finder username %q 确认账号。请检查账号 ID 是否完整，或改用账号名称搜索",
			username,
		)
	}
	account := parseWXFeedAccount(map[string]any{"contact": contact})
	if account.Username == "" {
		return WXFeedAccount{}, fmt.Errorf(
			"wx_channel 返回的账号缺少可用的 @finder username %q",
			username,
		)
	}
	return account, nil
}

func findWXFeedAccountContact(value any, username string, depth int) map[string]any {
	if depth > 8 {
		return nil
	}
	switch cast := value.(type) {
	case map[string]any:
		if strings.EqualFold(strings.TrimSpace(stringAny(cast["username"])), username) {
			return cast
		}
		for _, key := range []string{"contact", "finderContact", "author"} {
			if nested, ok := cast[key].(map[string]any); ok {
				if strings.EqualFold(strings.TrimSpace(stringAny(nested["username"])), username) {
					return nested
				}
			}
		}
		for _, nested := range cast {
			if contact := findWXFeedAccountContact(nested, username, depth+1); contact != nil {
				return contact
			}
		}
	case []any:
		for _, nested := range cast {
			if contact := findWXFeedAccountContact(nested, username, depth+1); contact != nil {
				return contact
			}
		}
	}
	return nil
}

func deriveFeedTags(title string) []string {
	tokens := tokenizeWXText(title)
	if len(tokens) == 0 {
		return nil
	}
	if len(tokens) > 3 {
		tokens = tokens[:3]
	}
	return tokens
}

func deriveFeedSignal(commentCount int, tags []string) string {
	switch {
	case commentCount >= 100:
		return "高评论密度"
	case commentCount >= 30:
		return "评论活跃"
	case len(tags) >= 2:
		return "主题清晰"
	default:
		return "新热视频"
	}
}

func looksLikeWXFeedFinderUsername(username string) bool {
	username = strings.ToLower(strings.TrimSpace(username))
	return strings.HasSuffix(username, "@finder")
}

func buildWXFeedListFinderUsernameError(username string, retCode int) error {
	username = strings.TrimSpace(username)
	switch {
	case username == "":
		return fmt.Errorf("请先填写真实的 @finder username，再导入视频列表")
	case looksLikeWXFeedFinderUsername(username):
		return fmt.Errorf("当前 @finder username 暂时没有返回可用视频列表，请重新搜索并确认该账号当前可访问")
	default:
		return fmt.Errorf("当前填写的 %q 不是可直接拉取视频列表的 @finder username。请先搜索账号并选择真实的 @finder username，再重新导入", username)
	}
}

func buildWXFeedListEmptyResultError(username string) error {
	if !looksLikeWXFeedFinderUsername(username) {
		return buildWXFeedListFinderUsernameError(username, 0)
	}
	return fmt.Errorf("当前 @finder username 没有返回可用视频列表，请确认该账号下存在可访问的视频内容后再重试")
}

func buildWXVideoSearchEmptyResultError(keyword string) error {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return fmt.Errorf("请先填写视频搜索关键词，再导入关键词视频来源")
	}
	return fmt.Errorf("当前关键词 %q 没有返回可用视频，请换一个关键词后再重试", keyword)
}

func stringAny(value any) string {
	if value == nil {
		return ""
	}
	switch cast := value.(type) {
	case string:
		return strings.TrimSpace(cast)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", cast))
	}
}

func intAny(value any) int {
	switch cast := value.(type) {
	case int:
		return cast
	case int32:
		return int(cast)
	case int64:
		return int(cast)
	case float64:
		return int(cast)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(cast), 10, 64)
		if err == nil {
			return int(parsed)
		}
	}
	return 0
}

func normalizeWXFeedListFetchError(baseURL string, err error) error {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "connectex"),
		strings.Contains(message, "connection refused"),
		strings.Contains(message, "actively refused"):
		return fmt.Errorf("无法连接到 wx_channel API。请先启动 wx_channel 或 wx_channel_radar，并确认 %s 可访问", baseURL)
	case isWXFeedListTimeoutError(err):
		return fmt.Errorf("连接 wx_channel API 超时。请确认 %s 已启动，并且视频号独立客户端已经完成 API 初始化", baseURL)
	default:
		return err
	}
}

func shouldRetryWXFeedListTimeout(ctx context.Context, err error, attempt int) bool {
	if attempt > 0 || err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return isWXFeedListTimeoutError(err)
}

func isWXFeedListTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "deadline exceeded") || strings.Contains(message, "client.timeout")
}

func normalizeWXFeedListMessage(baseURL, message string) error {
	normalized := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(normalized, "no ready wechat page"),
		strings.Contains(normalized, "no available client"),
		strings.Contains(normalized, "no ready client"):
		return fmt.Errorf("wx_channel 已启动，但当前没有就绪的视频号客户端。请先确认视频号独立客户端正在运行，并且请求已被 wx_channel 代理拦截后，再导入视频列表")
	case strings.Contains(normalized, "http 503"):
		return fmt.Errorf("wx_channel API 当前不可用。请确认 %s 已启动，并检查视频号客户端采集链路是否在线", baseURL)
	default:
		return fmt.Errorf("%s", message)
	}
}

func parseWXFeedAccountsFromSearchPayload(raw json.RawMessage) ([]WXFeedAccount, error) {
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ErrCode int    `json:"errCode"`
			ErrMsg  string `json:"errMsg"`
			Data    struct {
				BaseResponse struct {
					Ret    int `json:"Ret"`
					ErrMsg struct {
						String string `json:"String"`
					} `json:"ErrMsg"`
				} `json:"BaseResponse"`
				InfoList []map[string]any `json:"infoList"`
			} `json:"data"`
			InfoList []map[string]any `json:"infoList"`
		} `json:"data"`
		InfoList []map[string]any `json:"infoList"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.Code != 0 && envelope.Message != "" && envelope.Data.Data.InfoList == nil && envelope.Data.InfoList == nil && envelope.InfoList == nil {
		return nil, fmt.Errorf("wx_channel api error: %s", envelope.Message)
	}
	if envelope.Data.ErrCode != 0 {
		message := strings.TrimSpace(envelope.Data.ErrMsg)
		if message == "" {
			message = fmt.Sprintf("wx_channel api error code %d", envelope.Data.ErrCode)
		}
		return nil, fmt.Errorf("%s", message)
	}
	if envelope.Data.Data.BaseResponse.Ret != 0 {
		message := strings.TrimSpace(envelope.Data.Data.BaseResponse.ErrMsg.String)
		if message == "" {
			message = fmt.Sprintf("wx_channel api error code %d", envelope.Data.Data.BaseResponse.Ret)
		}
		return nil, fmt.Errorf("%s", message)
	}
	infoList := envelope.Data.Data.InfoList
	if len(infoList) == 0 {
		infoList = envelope.Data.InfoList
	}
	if len(infoList) == 0 {
		infoList = envelope.InfoList
	}
	results := make([]WXFeedAccount, 0, len(infoList))
	seen := map[string]struct{}{}
	for _, item := range infoList {
		account := parseWXFeedAccount(item)
		if account.Username == "" {
			continue
		}
		if _, exists := seen[account.Username]; exists {
			continue
		}
		seen[account.Username] = struct{}{}
		results = append(results, account)
	}
	return results, nil
}

func parseWXFeedListResultFromPayload(username string, raw json.RawMessage) (WXFeedListResult, error) {
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Object     []map[string]any `json:"object"`
			ObjectList []map[string]any `json:"objectList"`
			LastBuffer string           `json:"lastBuffer"`
			ErrCode    int              `json:"errCode"`
			ErrMsg     string           `json:"errMsg"`
			Data       struct {
				BaseResponse struct {
					Ret    int `json:"Ret"`
					ErrMsg struct {
						String string `json:"String"`
					} `json:"ErrMsg"`
				} `json:"BaseResponse"`
				Object     []map[string]any `json:"object"`
				ObjectList []map[string]any `json:"objectList"`
				LastBuffer string           `json:"lastBuffer"`
			} `json:"data"`
		} `json:"data"`
		Object     []map[string]any `json:"object"`
		ObjectList []map[string]any `json:"objectList"`
		LastBuffer string           `json:"lastBuffer"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return WXFeedListResult{}, err
	}
	if envelope.Code != 0 && envelope.Message != "" && len(envelope.Data.Object) == 0 && len(envelope.Object) == 0 {
		return WXFeedListResult{}, fmt.Errorf("wx_channel api error: %s", envelope.Message)
	}
	if envelope.Data.ErrCode != 0 {
		message := strings.TrimSpace(envelope.Data.ErrMsg)
		if message == "" {
			message = fmt.Sprintf("wx_channel api error code %d", envelope.Data.ErrCode)
		}
		return WXFeedListResult{}, fmt.Errorf("%s", message)
	}
	if envelope.Data.Data.BaseResponse.Ret != 0 {
		return WXFeedListResult{}, buildWXFeedListFinderUsernameError(username, envelope.Data.Data.BaseResponse.Ret)
	}
	objects := envelope.Data.ObjectList
	if len(objects) == 0 {
		objects = envelope.Data.Object
	}
	if len(objects) == 0 {
		objects = envelope.Data.Data.ObjectList
	}
	if len(objects) == 0 {
		objects = envelope.Data.Data.Object
	}
	if len(objects) == 0 {
		objects = envelope.ObjectList
	}
	if len(objects) == 0 {
		objects = envelope.Object
	}
	nextMarker := envelope.Data.LastBuffer
	if nextMarker == "" {
		nextMarker = envelope.Data.Data.LastBuffer
	}
	if nextMarker == "" {
		nextMarker = envelope.LastBuffer
	}
	result := WXFeedListResult{
		Username:   username,
		NextMarker: nextMarker,
		Items:      make([]WXFeedListItem, 0, len(objects)),
	}
	for _, item := range objects {
		parsed := parseWXFeedListItem(username, item)
		if parsed.ID == "" {
			continue
		}
		result.Items = append(result.Items, parsed)
	}
	return result, nil
}
