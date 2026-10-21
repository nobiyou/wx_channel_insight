package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var zeroClientsPattern = regexp.MustCompile(`(?i)"clients"\s*:\s*0|clients\s*[=:]\s*0`)

const (
	defaultLocalWXFeedListBaseURL          = "http://127.0.0.1:2026"
	wxFeedListNoReadyClientReason          = "feed list has no ready page client"
	wxFeedListHealthUnreachableReason      = "feed list ws/health is unreachable"
	wxVideoSearchNoReadyClientReason       = "video search has no ready page client"
	wxVideoSearchHealthUnreachableReason   = "video search ws/health is unreachable"
	wxCommentExportNoReadyClientReason     = "comment export has no ready page client"
	wxCommentExportHealthUnreachableReason = "comment export ws/health is unreachable"
	wxCommentExportContextMismatchCode     = "wx_comment_export_context_mismatch"
	wxCommentExportProxyMismatchCode       = "wx_comment_export_proxy_mismatch"
	maxLiveBindingHealthProbeTimeout       = 1 * time.Second
)

type wxFeedListHealthPayload struct {
	Clients *int   `json:"clients"`
	Status  string `json:"status"`
}

type wxFeedListStatusEnvelope struct {
	Code    int                     `json:"code"`
	Message string                  `json:"message"`
	Data    wxFeedListStatusPayload `json:"data"`
}

type wxFeedListStatusPayload struct {
	Clients            *int                     `json:"clients"`
	ReadyClients       *int                     `json:"ready_clients"`
	SearchReadyClients *int                     `json:"search_ready_clients"`
	FeedReadyClients   *int                     `json:"feed_ready_clients"`
	ClientList         []wxFeedListStatusClient `json:"client_list"`
	Runtime            wxChannelRuntimeStatus   `json:"runtime"`
}

type wxChannelRuntimeStatus struct {
	Proxy       wxChannelRuntimeProxy       `json:"proxy"`
	Certificate wxChannelRuntimeCertificate `json:"certificate"`
	Injection   wxChannelRuntimeInjection   `json:"injection"`
}

type wxChannelRuntimeProxy struct {
	Mode    string `json:"mode"`
	Address string `json:"address"`
	Port    int    `json:"port"`
}

type wxChannelRuntimeCertificate struct {
	Installed bool   `json:"installed"`
	Error     string `json:"error"`
}

type wxChannelRuntimeInjection struct {
	Checked   bool   `json:"checked"`
	Started   bool   `json:"started"`
	LastError string `json:"last_error"`
}

type wxFeedListStatusClient struct {
	PagePath       string `json:"page_path"`
	Href           string `json:"href"`
	APIReady       bool   `json:"api_ready"`
	SupportsSearch bool   `json:"supports_search"`
	SupportsFeed   bool   `json:"supports_feed"`
}

type wxFeedListProxyMismatch struct {
	CurrentProxyAddress  string
	ExpectedProxyAddress string
}

type wxChannelStatusProbeFailure struct {
	URL   string
	Error string
}

type bindingHealthCache map[string]SourceBindingAssessment

func (s *Service) assessBinding(ctx context.Context, source string, binding SourceBinding) SourceBindingAssessment {
	return s.assessBindingWithCache(ctx, source, binding, nil)
}

// assessBindingWithCache keeps live endpoint probes request-scoped. A saved
// workspace may contain many comment exports that all use the same
// wx_channel endpoint; probing that endpoint once is enough to describe the
// endpoint health while each binding still performs its own file validation.
func (s *Service) assessBindingWithCache(ctx context.Context, source string, binding SourceBinding, healthCache bindingHealthCache) SourceBindingAssessment {
	binding = s.resolveRuntimeSourceBinding(binding)
	assessment := s.assessBindingForExecution(source, binding)
	if !assessment.Runnable || !s.enableLiveChecks {
		return assessment
	}

	switch canonicalSource(source) {
	case RunSourceWXFeedList:
		baseURL := firstNonEmpty(assessment.ResolvedBaseURL, binding.BaseURL)
		if strings.TrimSpace(baseURL) == "" {
			return assessment
		}
		return s.assessBindingHealthWithCache(
			ctx,
			bindingHealthCacheKey(RunSourceWXFeedList, baseURL),
			assessment,
			func() SourceBindingAssessment {
				return assessWXFeedListBindingHealth(ctx, baseURL, s.wxFeedListClient.Client)
			},
			healthCache,
		)
	case RunSourceWXVideoSearch:
		baseURL := firstNonEmpty(assessment.ResolvedBaseURL, binding.BaseURL)
		if strings.TrimSpace(baseURL) == "" {
			return assessment
		}
		return s.assessBindingHealthWithCache(
			ctx,
			bindingHealthCacheKey(RunSourceWXVideoSearch, baseURL),
			assessment,
			func() SourceBindingAssessment {
				return assessWXVideoSearchBindingHealth(ctx, baseURL, s.wxFeedListClient.Client)
			},
			healthCache,
		)
	case RunSourceWXCommentExport:
		baseURL := strings.TrimSpace(s.wxCommentClient.BaseURL)
		if baseURL == "" {
			return assessment
		}
		return s.assessBindingHealthWithCache(
			ctx,
			bindingHealthCacheKey(RunSourceWXCommentExport, baseURL),
			assessment,
			func() SourceBindingAssessment {
				return assessWXCommentExportBindingHealth(ctx, baseURL, s.wxCommentClient.Client, SourceBindingAssessment{})
			},
			healthCache,
		)
	default:
		return assessment
	}
}

// assessBindingForExecution keeps the existing full validation and its
// fingerprint cache on source-management and collection paths.
func (s *Service) assessBindingForExecution(source string, binding SourceBinding) SourceBindingAssessment {
	if canonicalSource(source) != RunSourceWXCommentExport {
		return s.AssessBinding(source, binding)
	}

	resolution, err := s.resolveWXCommentExportPath(binding.Path)
	if err != nil || strings.TrimSpace(resolution.ResolvedPath) == "" {
		return s.AssessBinding(source, binding)
	}
	info, err := os.Stat(resolution.ResolvedPath)
	if err != nil {
		return s.AssessBinding(source, binding)
	}
	key := normalizeBindingPath(resolution.ResolvedPath)

	s.commentCacheMu.Lock()
	if cached, ok := s.commentCache[key]; ok && cached.validated && cached.size == info.Size() && cached.modifiedAt.Equal(info.ModTime()) {
		assessment := cached.assessment
		assessment.ResolvedPath = resolution.ResolvedPath
		assessment.PersistPath = resolution.PersistPath
		assessment.Target = cloneSourceBindingTarget(cached.assessment.Target)
		assessment.Diagnostic = cloneSourceBindingDiagnostic(cached.assessment.Diagnostic)
		s.commentCacheMu.Unlock()
		return assessment
	}
	s.commentCacheMu.Unlock()

	assessment := s.AssessBinding(source, binding)
	s.commentCacheMu.Lock()
	s.commentCache[key] = cachedCommentBindingAssessment{
		size:       info.Size(),
		modifiedAt: info.ModTime(),
		validated:  true,
		assessment: assessment,
	}
	s.commentCacheMu.Unlock()
	return assessment
}

// assessBindingForRead keeps dashboard reads local and bounded. Comment export
// validity still belongs to import/refresh and the live execution path; a
// readable export may additionally provide a cached business-facing target.
func (s *Service) assessBindingForRead(source string, binding SourceBinding) SourceBindingAssessment {
	if canonicalSource(source) != RunSourceWXCommentExport {
		return s.AssessBinding(source, binding)
	}

	paths := []string{normalizeBindingPath(binding.Path)}
	if mappedPath := mapWXCommentExportPathToRoot(binding.Path, s.commentDataRoot); mappedPath != "" && !strings.EqualFold(mappedPath, paths[0]) {
		paths = append(paths, mappedPath)
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			return SourceBindingAssessment{
				Runnable:     true,
				ResolvedPath: path,
				PersistPath:  path,
			}
		}

		key := normalizeBindingPath(path)
		s.commentCacheMu.Lock()
		if cached, ok := s.commentCache[key]; ok && cached.size == info.Size() && cached.modifiedAt.Equal(info.ModTime()) {
			assessment := cached.assessment
			assessment.ResolvedPath = path
			assessment.PersistPath = path
			assessment.Target = cloneSourceBindingTarget(cached.assessment.Target)
			assessment.Diagnostic = cloneSourceBindingDiagnostic(cached.assessment.Diagnostic)
			s.commentCacheMu.Unlock()
			return assessment
		}
		s.commentCacheMu.Unlock()

		assessment := SourceBindingAssessment{
			Runnable:     true,
			ResolvedPath: path,
			PersistPath:  path,
		}
		// Keep a malformed export non-blocking for read-only pages, but use the
		// same file as the source of truth when it contains usable metadata.
		if payload, loadErr := LoadWXCommentExportFile(path); loadErr == nil {
			assessment.Target = buildSourceBindingTargetFromCommentExport(payload)
		}
		s.commentCacheMu.Lock()
		s.commentCache[key] = cachedCommentBindingAssessment{
			size:       info.Size(),
			modifiedAt: info.ModTime(),
			assessment: assessment,
		}
		s.commentCacheMu.Unlock()
		return assessment
	}

	return SourceBindingAssessment{
		Runnable: false,
		Reason:   "评论导出文件不存在，请重新导入评论导出文件或更新路径",
	}
}

func (s *Service) assessBindingHealthWithCache(
	ctx context.Context,
	key string,
	baseAssessment SourceBindingAssessment,
	probe func() SourceBindingAssessment,
	healthCache bindingHealthCache,
) SourceBindingAssessment {
	if healthCache != nil {
		if cached, ok := healthCache[key]; ok {
			return mergeBindingAssessments(baseAssessment, cached)
		}
	}
	health := probe()
	if healthCache != nil {
		healthCache[key] = SourceBindingAssessment{
			Runnable:        health.Runnable,
			Reason:          health.Reason,
			Detail:          health.Detail,
			ResolvedBaseURL: health.ResolvedBaseURL,
			Diagnostic:      cloneSourceBindingDiagnostic(health.Diagnostic),
		}
	}
	return mergeBindingAssessments(baseAssessment, health)
}

func mergeBindingAssessments(base, health SourceBindingAssessment) SourceBindingAssessment {
	if health.ResolvedPath == "" {
		health.ResolvedPath = base.ResolvedPath
	}
	if health.PersistPath == "" {
		health.PersistPath = base.PersistPath
	}
	if health.ResolvedBaseURL == "" {
		health.ResolvedBaseURL = base.ResolvedBaseURL
	}
	if health.Target == nil {
		health.Target = cloneSourceBindingTarget(base.Target)
	}
	return health
}

func bindingHealthCacheKey(source, baseURL string) string {
	return canonicalSource(source) + "\x00" + strings.TrimRight(normalizeWXFeedListBaseURL(baseURL), "/")
}

func assessWXVideoSearchBindingHealth(ctx context.Context, baseURL string, client *http.Client) SourceBindingAssessment {
	primaryBaseURL := normalizeWXFeedListBaseURL(baseURL)
	assessment := probeWXVideoSearchHealth(ctx, primaryBaseURL, client)
	if assessment.Runnable || assessment.Reason == wxVideoSearchNoReadyClientReason {
		return assessment
	}

	fallbackBaseURL := resolveFallbackWXFeedListBaseURL(primaryBaseURL)
	if fallbackBaseURL == "" {
		return assessment
	}

	fallbackAssessment := probeWXVideoSearchHealth(ctx, fallbackBaseURL, client)
	if fallbackAssessment.Runnable || fallbackAssessment.Reason == wxVideoSearchNoReadyClientReason {
		return fallbackAssessment
	}
	return assessment
}

func assessWXCommentExportBindingHealth(ctx context.Context, baseURL string, client *http.Client, baseAssessment SourceBindingAssessment) SourceBindingAssessment {
	resolvedBaseURL := normalizeWXFeedListBaseURL(baseURL)
	assessment, ok, statusFailure := probeWXCommentExportStatusHealth(ctx, resolvedBaseURL, client)
	if ok {
		if assessment.ResolvedPath == "" {
			assessment.ResolvedPath = baseAssessment.ResolvedPath
		}
		if assessment.PersistPath == "" {
			assessment.PersistPath = baseAssessment.PersistPath
		}
		return assessment
	}

	assessment = probeWXCommentExportWSHealth(ctx, resolvedBaseURL, client, statusFailure)
	if assessment.ResolvedPath == "" {
		assessment.ResolvedPath = baseAssessment.ResolvedPath
	}
	if assessment.PersistPath == "" {
		assessment.PersistPath = baseAssessment.PersistPath
	}
	return assessment
}

func assessWXFeedListBindingHealth(ctx context.Context, baseURL string, client *http.Client) SourceBindingAssessment {
	primaryBaseURL := normalizeWXFeedListBaseURL(baseURL)
	assessment := probeWXFeedListHealth(ctx, primaryBaseURL, client)
	if assessment.Runnable || assessment.Reason == wxFeedListNoReadyClientReason {
		return assessment
	}

	fallbackBaseURL := resolveFallbackWXFeedListBaseURL(primaryBaseURL)
	if fallbackBaseURL == "" {
		return assessment
	}

	fallbackAssessment := probeWXFeedListHealth(ctx, fallbackBaseURL, client)
	if fallbackAssessment.Runnable || fallbackAssessment.Reason == wxFeedListNoReadyClientReason {
		return fallbackAssessment
	}
	return assessment
}

func probeWXFeedListHealth(ctx context.Context, baseURL string, client *http.Client) SourceBindingAssessment {
	if assessment, ok, statusFailure := probeWXFeedListStatusHealth(ctx, baseURL, client); ok {
		return assessment
	} else {
		return probeWXFeedListWSHealth(ctx, baseURL, client, statusFailure)
	}
}

func probeWXVideoSearchHealth(ctx context.Context, baseURL string, client *http.Client) SourceBindingAssessment {
	assessment, ok, statusFailure := probeWXVideoSearchStatusHealth(ctx, baseURL, client)
	if ok {
		return assessment
	}
	assessment = probeWXFeedListWSHealth(ctx, baseURL, client, statusFailure)
	if assessment.Reason == wxFeedListHealthUnreachableReason {
		assessment.Reason = wxVideoSearchHealthUnreachableReason
	}
	if strings.HasPrefix(assessment.Detail, wxFeedListHealthUnreachableReason) {
		assessment.Detail = strings.Replace(assessment.Detail, wxFeedListHealthUnreachableReason, wxVideoSearchHealthUnreachableReason, 1)
	}
	if strings.Contains(assessment.Detail, "视频列表来源") {
		assessment.Detail = strings.Replace(assessment.Detail, "视频列表来源", "关键词视频来源", 1)
	}
	if assessment.Reason == wxFeedListNoReadyClientReason {
		assessment.Reason = wxVideoSearchNoReadyClientReason
	}
	if assessment.Diagnostic != nil {
		switch assessment.Diagnostic.Code {
		case "wx_feed_list_health_unreachable":
			assessment.Diagnostic.Code = "wx_video_search_health_unreachable"
		case "wx_feed_list_no_ready_page_client":
			assessment.Diagnostic.Code = "wx_video_search_no_ready_page_client"
		case "wx_feed_list_proxy_mismatch":
			assessment.Diagnostic.Code = "wx_video_search_proxy_mismatch"
		}
	}
	return assessment
}

func probeWXVideoSearchStatusHealth(ctx context.Context, baseURL string, client *http.Client) (SourceBindingAssessment, bool, *wxChannelStatusProbeFailure) {
	resolvedBaseURL := normalizeWXFeedListBaseURL(baseURL)
	statusURL := strings.TrimRight(strings.TrimSpace(resolvedBaseURL), "/") + "/api/channels/status"
	if statusURL == "/api/channels/status" {
		return SourceBindingAssessment{}, false, nil
	}

	body, ok, failure := fetchWXFeedListHealthBody(ctx, statusURL, client, 16*1024)
	if !ok {
		return SourceBindingAssessment{}, false, buildWXChannelStatusProbeFailure(statusURL, failure)
	}

	payload, ok := parseWXFeedListStatusPayload(body)
	if !ok {
		return SourceBindingAssessment{}, false, nil
	}

	connectedCount := firstKnownCount(payload.Clients, len(payload.ClientList))
	readyCount := firstKnownCount(payload.ReadyClients, countAPIReadyClients(payload.ClientList))
	searchReadyCount := firstKnownCount(payload.SearchReadyClients, countSearchReadyClients(payload.ClientList))
	if searchReadyCount > 0 || readyCount > 0 {
		return SourceBindingAssessment{
			Runnable:        true,
			ResolvedBaseURL: resolvedBaseURL,
		}, true, nil
	}

	if connectedCount > 0 || readyCount > 0 || len(payload.ClientList) > 0 || countsWereReported(payload) {
		detail, diagnostic := buildNoReadyWXVideoSearchStatusDetail(resolvedBaseURL, statusURL, payload, connectedCount, readyCount, searchReadyCount)
		return SourceBindingAssessment{
			Runnable:        false,
			Reason:          wxVideoSearchNoReadyClientReason,
			Detail:          detail,
			ResolvedBaseURL: resolvedBaseURL,
			Diagnostic:      diagnostic,
		}, true, nil
	}

	return SourceBindingAssessment{}, false, nil
}

func probeWXFeedListStatusHealth(ctx context.Context, baseURL string, client *http.Client) (SourceBindingAssessment, bool, *wxChannelStatusProbeFailure) {
	resolvedBaseURL := normalizeWXFeedListBaseURL(baseURL)
	statusURL := strings.TrimRight(strings.TrimSpace(resolvedBaseURL), "/") + "/api/channels/status"
	if statusURL == "/api/channels/status" {
		return SourceBindingAssessment{}, false, nil
	}

	body, ok, failure := fetchWXFeedListHealthBody(ctx, statusURL, client, 16*1024)
	if !ok {
		return SourceBindingAssessment{}, false, buildWXChannelStatusProbeFailure(statusURL, failure)
	}

	payload, ok := parseWXFeedListStatusPayload(body)
	if !ok {
		return SourceBindingAssessment{}, false, nil
	}

	connectedCount := firstKnownCount(payload.Clients, len(payload.ClientList))
	readyCount := firstKnownCount(payload.ReadyClients, countAPIReadyClients(payload.ClientList))
	feedReadyCount := firstKnownCount(payload.FeedReadyClients, countFeedReadyClients(payload.ClientList))
	if feedReadyCount > 0 {
		return SourceBindingAssessment{
			Runnable:        true,
			ResolvedBaseURL: resolvedBaseURL,
		}, true, nil
	}

	if connectedCount > 0 || readyCount > 0 || len(payload.ClientList) > 0 || countsWereReported(payload) {
		detail, diagnostic := buildNoReadyWXFeedListStatusDetail(resolvedBaseURL, statusURL, payload, connectedCount, readyCount, feedReadyCount)
		return SourceBindingAssessment{
			Runnable:        false,
			Reason:          wxFeedListNoReadyClientReason,
			Detail:          detail,
			ResolvedBaseURL: resolvedBaseURL,
			Diagnostic:      diagnostic,
		}, true, nil
	}

	return SourceBindingAssessment{}, false, nil
}

func probeWXFeedListWSHealth(ctx context.Context, baseURL string, client *http.Client, statusFailure *wxChannelStatusProbeFailure) SourceBindingAssessment {
	resolvedBaseURL := normalizeWXFeedListBaseURL(baseURL)
	healthURL := strings.TrimRight(strings.TrimSpace(resolvedBaseURL), "/") + "/ws/health"
	if healthURL == "/ws/health" {
		return SourceBindingAssessment{
			Runnable:        false,
			Reason:          wxFeedListHealthUnreachableReason,
			Detail:          buildWXChannelHealthUnreachableDetail(wxFeedListHealthUnreachableReason, "视频列表来源", statusFailure),
			ResolvedBaseURL: resolvedBaseURL,
			Diagnostic:      buildWXFeedListHealthUnreachableDiagnostic(resolvedBaseURL, healthURL, statusFailure),
		}
	}

	body, ok, _ := fetchWXFeedListHealthBody(ctx, healthURL, client, 4096)
	if !ok {
		return SourceBindingAssessment{
			Runnable:        false,
			Reason:          wxFeedListHealthUnreachableReason,
			Detail:          buildWXChannelHealthUnreachableDetail(wxFeedListHealthUnreachableReason, "视频列表来源", statusFailure),
			ResolvedBaseURL: resolvedBaseURL,
			Diagnostic:      buildWXFeedListHealthUnreachableDiagnostic(resolvedBaseURL, healthURL, statusFailure),
		}
	}

	if payload, ok := parseWXFeedListHealthPayload(body); ok {
		if payload.Clients != nil && *payload.Clients <= 0 {
			detail, diagnostic := buildNoReadyWXFeedListDetail(resolvedBaseURL, healthURL, payload.Clients, payload.Status)
			return SourceBindingAssessment{
				Runnable:        false,
				Reason:          wxFeedListNoReadyClientReason,
				Detail:          detail,
				ResolvedBaseURL: resolvedBaseURL,
				Diagnostic:      diagnostic,
			}
		}
		if payload.Clients != nil && *payload.Clients > 0 {
			return SourceBindingAssessment{
				Runnable:        true,
				ResolvedBaseURL: resolvedBaseURL,
			}
		}
	}
	if zeroClientsPattern.Match(body) {
		zero := 0
		detail, diagnostic := buildNoReadyWXFeedListDetail(resolvedBaseURL, healthURL, &zero, "")
		return SourceBindingAssessment{
			Runnable:        false,
			Reason:          wxFeedListNoReadyClientReason,
			Detail:          detail,
			ResolvedBaseURL: resolvedBaseURL,
			Diagnostic:      diagnostic,
		}
	}

	return SourceBindingAssessment{
		Runnable:        true,
		ResolvedBaseURL: resolvedBaseURL,
	}
}

func probeWXCommentExportStatusHealth(ctx context.Context, baseURL string, client *http.Client) (SourceBindingAssessment, bool, *wxChannelStatusProbeFailure) {
	resolvedBaseURL := normalizeWXFeedListBaseURL(baseURL)
	statusURL := strings.TrimRight(strings.TrimSpace(resolvedBaseURL), "/") + "/api/channels/status"
	if statusURL == "/api/channels/status" {
		return SourceBindingAssessment{}, false, nil
	}

	body, ok, failure := fetchWXFeedListHealthBody(ctx, statusURL, client, 16*1024)
	if !ok {
		return SourceBindingAssessment{}, false, buildWXChannelStatusProbeFailure(statusURL, failure)
	}

	payload, ok := parseWXFeedListStatusPayload(body)
	if !ok {
		return SourceBindingAssessment{}, false, nil
	}

	connectedCount := firstKnownCount(payload.Clients, len(payload.ClientList))
	readyCount := firstKnownCount(payload.ReadyClients, countAPIReadyClients(payload.ClientList))
	if readyCount > 0 {
		return SourceBindingAssessment{
			Runnable:        true,
			ResolvedBaseURL: resolvedBaseURL,
			Diagnostic:      buildWXCommentExportStatusSnapshotDiagnostic(resolvedBaseURL, statusURL, payload, connectedCount, readyCount),
		}, true, nil
	}

	if connectedCount > 0 || len(payload.ClientList) > 0 || countsWereReported(payload) {
		detail, diagnostic := buildNoReadyWXCommentExportStatusDetail(resolvedBaseURL, statusURL, payload, connectedCount, readyCount)
		return SourceBindingAssessment{
			Runnable:        false,
			Reason:          wxCommentExportNoReadyClientReason,
			Detail:          detail,
			ResolvedBaseURL: resolvedBaseURL,
			Diagnostic:      diagnostic,
		}, true, nil
	}

	return SourceBindingAssessment{}, false, nil
}

func probeWXCommentExportWSHealth(ctx context.Context, baseURL string, client *http.Client, statusFailure *wxChannelStatusProbeFailure) SourceBindingAssessment {
	resolvedBaseURL := normalizeWXFeedListBaseURL(baseURL)
	healthURL := strings.TrimRight(strings.TrimSpace(resolvedBaseURL), "/") + "/ws/health"
	if healthURL == "/ws/health" {
		return SourceBindingAssessment{
			Runnable:        false,
			Reason:          wxCommentExportHealthUnreachableReason,
			Detail:          buildWXChannelHealthUnreachableDetail(wxCommentExportHealthUnreachableReason, "评论导出来源", statusFailure),
			ResolvedBaseURL: resolvedBaseURL,
			Diagnostic:      buildWXCommentExportHealthUnreachableDiagnostic(resolvedBaseURL, healthURL, statusFailure),
		}
	}

	body, ok, _ := fetchWXFeedListHealthBody(ctx, healthURL, client, 4096)
	if !ok {
		return SourceBindingAssessment{
			Runnable:        false,
			Reason:          wxCommentExportHealthUnreachableReason,
			Detail:          buildWXChannelHealthUnreachableDetail(wxCommentExportHealthUnreachableReason, "评论导出来源", statusFailure),
			ResolvedBaseURL: resolvedBaseURL,
			Diagnostic:      buildWXCommentExportHealthUnreachableDiagnostic(resolvedBaseURL, healthURL, statusFailure),
		}
	}

	if payload, ok := parseWXFeedListHealthPayload(body); ok {
		if payload.Clients != nil && *payload.Clients <= 0 {
			detail, diagnostic := buildNoReadyWXCommentExportDetail(resolvedBaseURL, healthURL, payload.Clients, payload.Status)
			return SourceBindingAssessment{
				Runnable:        false,
				Reason:          wxCommentExportNoReadyClientReason,
				Detail:          detail,
				ResolvedBaseURL: resolvedBaseURL,
				Diagnostic:      diagnostic,
			}
		}
		if payload.Clients != nil && *payload.Clients > 0 {
			return SourceBindingAssessment{
				Runnable:        true,
				ResolvedBaseURL: resolvedBaseURL,
			}
		}
	}
	if zeroClientsPattern.Match(body) {
		zero := 0
		detail, diagnostic := buildNoReadyWXCommentExportDetail(resolvedBaseURL, healthURL, &zero, "")
		return SourceBindingAssessment{
			Runnable:        false,
			Reason:          wxCommentExportNoReadyClientReason,
			Detail:          detail,
			ResolvedBaseURL: resolvedBaseURL,
			Diagnostic:      diagnostic,
		}
	}

	return SourceBindingAssessment{
		Runnable:        true,
		ResolvedBaseURL: resolvedBaseURL,
	}
}

func fetchWXFeedListHealthBody(ctx context.Context, endpoint string, client *http.Client, limit int64) ([]byte, bool, string) {
	httpClient := cloneHealthProbeClient(client)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, err.Error()
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, false, describeWXChannelHealthProbeError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, false, "HTTP " + strconv.Itoa(resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, false, err.Error()
	}
	return body, true, ""
}

func cloneHealthProbeClient(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{Timeout: maxLiveBindingHealthProbeTimeout}
	}

	copy := *client
	if copy.Timeout <= 0 || copy.Timeout > maxLiveBindingHealthProbeTimeout {
		copy.Timeout = maxLiveBindingHealthProbeTimeout
	}
	return &copy
}

func describeWXChannelHealthProbeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout after " + maxLiveBindingHealthProbeTimeout.String()
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout after " + maxLiveBindingHealthProbeTimeout.String()
	}
	return strings.TrimSpace(err.Error())
}

func buildWXChannelStatusProbeFailure(statusURL, failure string) *wxChannelStatusProbeFailure {
	statusURL = strings.TrimSpace(statusURL)
	failure = strings.TrimSpace(failure)
	if statusURL == "" && failure == "" {
		return nil
	}
	return &wxChannelStatusProbeFailure{
		URL:   statusURL,
		Error: failure,
	}
}

func attachWXChannelStatusProbeFailure(diagnostic *SourceBindingDiagnostic, failure *wxChannelStatusProbeFailure) {
	if diagnostic == nil || failure == nil {
		return
	}
	if statusURL := strings.TrimSpace(failure.URL); statusURL != "" {
		diagnostic.StatusURL = statusURL
	}
	if errText := strings.TrimSpace(failure.Error); errText != "" {
		diagnostic.WXChannelStatusError = errText
	}
}

func buildWXChannelHealthUnreachableDetail(reason, sourceLabel string, statusFailure *wxChannelStatusProbeFailure) string {
	detail := reason + "：" + sourceLabel + "当前无法确认 wx_channel 客户端状态"
	if statusFailure != nil && strings.TrimSpace(statusFailure.URL) != "" {
		detail += "，" + strings.TrimSpace(statusFailure.URL) + " 未正常响应"
	}
	if statusFailure != nil && strings.TrimSpace(statusFailure.Error) != "" {
		detail += "（" + strings.TrimSpace(statusFailure.Error) + "）"
	}
	detail += "。请先确认 wx_channel API 进程可响应 /api/channels/status；如果 2025/2026 端口已监听但请求超时，先重启 wx_channel，再打开视频号独立客户端进入目标页面。"
	return detail
}

func parseWXFeedListHealthPayload(body []byte) (wxFeedListHealthPayload, bool) {
	var payload wxFeedListHealthPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return wxFeedListHealthPayload{}, false
	}
	if payload.Clients == nil && strings.TrimSpace(payload.Status) == "" {
		return wxFeedListHealthPayload{}, false
	}
	return payload, true
}

func parseWXFeedListStatusPayload(body []byte) (wxFeedListStatusPayload, bool) {
	var envelope wxFeedListStatusEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return wxFeedListStatusPayload{}, false
	}
	if envelope.Code != 0 {
		return wxFeedListStatusPayload{}, false
	}
	if !countsWereReported(envelope.Data) && len(envelope.Data.ClientList) == 0 {
		return wxFeedListStatusPayload{}, false
	}
	return envelope.Data, true
}

func countsWereReported(payload wxFeedListStatusPayload) bool {
	return payload.Clients != nil || payload.ReadyClients != nil || payload.SearchReadyClients != nil || payload.FeedReadyClients != nil
}

func runtimeReportsProcessProxy(payload wxFeedListStatusPayload) bool {
	return strings.EqualFold(strings.TrimSpace(payload.Runtime.Proxy.Mode), "process_proxy")
}

func attachWXChannelRuntimeDiagnostic(diagnostic *SourceBindingDiagnostic, payload wxFeedListStatusPayload) {
	if diagnostic == nil {
		return
	}
	if mode := strings.TrimSpace(payload.Runtime.Proxy.Mode); mode != "" {
		diagnostic.WXChannelProxyMode = mode
	}
	if address := strings.TrimSpace(payload.Runtime.Proxy.Address); address != "" {
		diagnostic.WXChannelProxyAddress = address
	}
	if payload.Runtime.Injection.Checked {
		diagnostic.WXChannelInjectionChecked = boolPtr(true)
		diagnostic.WXChannelInjectionStarted = boolPtr(payload.Runtime.Injection.Started)
	}
	if errText := strings.TrimSpace(payload.Runtime.Injection.LastError); errText != "" {
		diagnostic.WXChannelInjectionError = errText
	}
	if payload.Runtime.Certificate.Installed || strings.TrimSpace(payload.Runtime.Certificate.Error) != "" {
		diagnostic.WXChannelCertificateInstalled = boolPtr(payload.Runtime.Certificate.Installed)
	}
	if errText := strings.TrimSpace(payload.Runtime.Certificate.Error); errText != "" {
		diagnostic.WXChannelCertificateError = errText
	}
}

func buildNoReadyProcessProxyDetail(sourceLabel string, payload wxFeedListStatusPayload) string {
	detail := sourceLabel + " has no ready page client：wx_channel 当前使用进程代理模式"
	if address := strings.TrimSpace(payload.Runtime.Proxy.Address); address != "" {
		detail += "（" + address + "）"
	}
	if payload.Runtime.Injection.Checked && !payload.Runtime.Injection.Started {
		detail += "，但视频号进程注入引擎未启动成功"
		if errText := strings.TrimSpace(payload.Runtime.Injection.LastError); errText != "" {
			detail += "：" + errText
		}
		detail += "。请用管理员权限运行 wx_channel，或确认视频号独立客户端启动后已被进程代理接管。"
	} else {
		detail += "，但当前 /api/channels/status 仍是 clients=0、ready_clients=0，说明视频号独立客户端还没有通过 wx_channel 进程代理连进来。请确认客户端已打开并重进视频号页面后再检查 ready client。"
	}
	if payload.Runtime.Certificate.Installed == false && strings.TrimSpace(payload.Runtime.Certificate.Error) != "" {
		detail += " 证书检查也有异常：" + strings.TrimSpace(payload.Runtime.Certificate.Error) + "。"
	}
	return detail
}

func firstKnownCount(value *int, fallback int) int {
	if value != nil {
		return *value
	}
	return fallback
}

func countAPIReadyClients(items []wxFeedListStatusClient) int {
	count := 0
	for _, item := range items {
		if item.APIReady {
			count++
		}
	}
	return count
}

func countFeedReadyClients(items []wxFeedListStatusClient) int {
	count := 0
	for _, item := range items {
		if item.SupportsFeed {
			count++
		}
	}
	return count
}

func countSearchReadyClients(items []wxFeedListStatusClient) int {
	count := 0
	for _, item := range items {
		if item.SupportsSearch || item.APIReady {
			count++
		}
	}
	return count
}

func pickRepresentativeFeedClient(items []wxFeedListStatusClient) *wxFeedListStatusClient {
	for _, item := range items {
		if item.SupportsFeed {
			copy := item
			return &copy
		}
	}
	for _, item := range items {
		if item.APIReady {
			copy := item
			return &copy
		}
	}
	if len(items) == 0 {
		return nil
	}
	copy := items[0]
	return &copy
}

func buildWXFeedListHealthUnreachableDiagnostic(baseURL, healthURL string, statusFailure *wxChannelStatusProbeFailure) *SourceBindingDiagnostic {
	diagnostic := &SourceBindingDiagnostic{
		Code:            "wx_feed_list_health_unreachable",
		ResolvedBaseURL: normalizeWXFeedListBaseURL(baseURL),
		WSHealthURL:     strings.TrimSpace(healthURL),
	}
	attachWXChannelStatusProbeFailure(diagnostic, statusFailure)
	return diagnostic
}

func buildWXCommentExportHealthUnreachableDiagnostic(baseURL, healthURL string, statusFailure *wxChannelStatusProbeFailure) *SourceBindingDiagnostic {
	diagnostic := &SourceBindingDiagnostic{
		Code:            "wx_comment_export_health_unreachable",
		ResolvedBaseURL: normalizeWXFeedListBaseURL(baseURL),
		WSHealthURL:     strings.TrimSpace(healthURL),
	}
	attachWXChannelStatusProbeFailure(diagnostic, statusFailure)
	return diagnostic
}

func buildWXCommentExportStatusSnapshotDiagnostic(baseURL, statusURL string, payload wxFeedListStatusPayload, connectedCount, readyCount int) *SourceBindingDiagnostic {
	diagnostic := &SourceBindingDiagnostic{
		Code:                 "wx_comment_export_ready_page_client",
		ResolvedBaseURL:      normalizeWXFeedListBaseURL(baseURL),
		StatusURL:            strings.TrimSpace(statusURL),
		ConnectedClientCount: intPtr(connectedCount),
		ReadyClientCount:     intPtr(readyCount),
	}
	if client := pickRepresentativeFeedClient(payload.ClientList); client != nil {
		diagnostic.CurrentPagePath = strings.TrimSpace(client.PagePath)
		diagnostic.CurrentPageHref = strings.TrimSpace(client.Href)
	}
	return diagnostic
}

func buildNoReadyWXFeedListStatusDetail(baseURL, statusURL string, payload wxFeedListStatusPayload, connectedCount, readyCount, feedReadyCount int) (string, *SourceBindingDiagnostic) {
	diagnostic := &SourceBindingDiagnostic{
		Code:                 "wx_feed_list_no_ready_page_client",
		ResolvedBaseURL:      normalizeWXFeedListBaseURL(baseURL),
		StatusURL:            strings.TrimSpace(statusURL),
		ConnectedClientCount: intPtr(connectedCount),
		ReadyClientCount:     intPtr(readyCount),
		FeedReadyClientCount: intPtr(feedReadyCount),
	}

	if client := pickRepresentativeFeedClient(payload.ClientList); client != nil {
		diagnostic.CurrentPagePath = strings.TrimSpace(client.PagePath)
		diagnostic.CurrentPageHref = strings.TrimSpace(client.Href)
	}
	attachWXChannelRuntimeDiagnostic(diagnostic, payload)

	if connectedCount <= 0 && readyCount <= 0 {
		if runtimeReportsProcessProxy(payload) {
			diagnostic.Code = "wx_feed_list_process_proxy_no_client"
			return buildNoReadyProcessProxyDetail("feed list", payload), diagnostic
		}
		if hint := detectLocalWXFeedListProxyMismatch(baseURL); hint != nil {
			diagnostic.Code = "wx_feed_list_proxy_mismatch"
			diagnostic.CurrentBrowserProxyAddress = hint.CurrentProxyAddress
			diagnostic.ExpectedProxyAddress = hint.ExpectedProxyAddress
			return "feed list has no ready page client：当前 Windows 代理配置是 " + hint.CurrentProxyAddress + "，不是 wx_channel 预期的 " + hint.ExpectedProxyAddress + "；当前 /api/channels/status 还是 clients=0、ready_clients=0，说明视频号独立客户端还没有经过 wx_channel 代理连进来。请先确认客户端确实走 " + hint.ExpectedProxyAddress + "，再检查 /api/channels/status 是否出现 ready client。", diagnostic
		}
		return "feed list has no ready page client：wx_channel 已启动，但当前没有就绪的视频号客户端。请先确认视频号独立客户端仍在运行，并且请求已经被 wx_channel 代理拦截；然后检查 /api/channels/status 是否出现 ready client。", diagnostic
	}

	detail := "feed list has no ready page client：wx_channel 当前已有 " + strconv.Itoa(maxInt(0, connectedCount)) + " 个客户端连接"
	if readyCount > 0 {
		detail += "，其中 " + strconv.Itoa(readyCount) + " 个 API 已就绪"
	}
	detail += "，但支持视频列表的客户端仍然是 0。"
	if diagnostic.CurrentPagePath != "" {
		detail += " 当前客户端上下文 " + diagnostic.CurrentPagePath + "。"
	}
	detail += " 请在视频号独立客户端切回首页或支持视频列表的界面，并确认请求仍经过 wx_channel 代理后再刷新。"
	return detail, diagnostic
}

func buildNoReadyWXVideoSearchStatusDetail(baseURL, statusURL string, payload wxFeedListStatusPayload, connectedCount, readyCount, searchReadyCount int) (string, *SourceBindingDiagnostic) {
	diagnostic := &SourceBindingDiagnostic{
		Code:                 "wx_video_search_no_ready_page_client",
		ResolvedBaseURL:      normalizeWXFeedListBaseURL(baseURL),
		StatusURL:            strings.TrimSpace(statusURL),
		ConnectedClientCount: intPtr(connectedCount),
		ReadyClientCount:     intPtr(readyCount),
		FeedReadyClientCount: intPtr(searchReadyCount),
	}

	if client := pickRepresentativeFeedClient(payload.ClientList); client != nil {
		diagnostic.CurrentPagePath = strings.TrimSpace(client.PagePath)
		diagnostic.CurrentPageHref = strings.TrimSpace(client.Href)
	}
	attachWXChannelRuntimeDiagnostic(diagnostic, payload)

	if connectedCount <= 0 && readyCount <= 0 {
		if runtimeReportsProcessProxy(payload) {
			diagnostic.Code = "wx_video_search_process_proxy_no_client"
			return buildNoReadyProcessProxyDetail("video search", payload), diagnostic
		}
		if hint := detectLocalWXFeedListProxyMismatch(baseURL); hint != nil {
			diagnostic.Code = "wx_video_search_proxy_mismatch"
			diagnostic.CurrentBrowserProxyAddress = hint.CurrentProxyAddress
			diagnostic.ExpectedProxyAddress = hint.ExpectedProxyAddress
			return "video search has no ready page client：当前 Windows 代理配置是 " + hint.CurrentProxyAddress + "，不是 wx_channel 预期的 " + hint.ExpectedProxyAddress + "；当前 /api/channels/status 还是 clients=0、ready_clients=0，说明视频号独立客户端还没有经过 wx_channel 代理连进来。请先确认客户端确实走 " + hint.ExpectedProxyAddress + "，再检查 /api/channels/status 是否出现 ready client。", diagnostic
		}
		return "video search has no ready page client：wx_channel 已启动，但当前没有就绪的视频号客户端。请先确认视频号独立客户端正在运行，并且请求已经被 wx_channel 代理拦截；然后检查 /api/channels/status 是否出现 ready client。", diagnostic
	}

	detail := "video search has no ready page client：wx_channel 当前已有 " + strconv.Itoa(maxInt(0, connectedCount)) + " 个客户端连接"
	if readyCount > 0 {
		detail += "，其中 " + strconv.Itoa(readyCount) + " 个 API 已就绪"
	}
	detail += "，但支持视频搜索的客户端仍然是 0。"
	if diagnostic.CurrentPagePath != "" {
		detail += " 当前客户端上下文 " + diagnostic.CurrentPagePath + "。"
	}
	detail += " 请确认视频号独立客户端仍经过 wx_channel 代理，并等待搜索 API 初始化后再刷新。"
	return detail, diagnostic
}

func buildNoReadyWXCommentExportStatusDetail(baseURL, statusURL string, payload wxFeedListStatusPayload, connectedCount, readyCount int) (string, *SourceBindingDiagnostic) {
	diagnostic := &SourceBindingDiagnostic{
		Code:                 "wx_comment_export_no_ready_page_client",
		ResolvedBaseURL:      normalizeWXFeedListBaseURL(baseURL),
		StatusURL:            strings.TrimSpace(statusURL),
		ConnectedClientCount: intPtr(connectedCount),
		ReadyClientCount:     intPtr(readyCount),
	}

	if client := pickRepresentativeFeedClient(payload.ClientList); client != nil {
		diagnostic.CurrentPagePath = strings.TrimSpace(client.PagePath)
		diagnostic.CurrentPageHref = strings.TrimSpace(client.Href)
	}
	attachWXChannelRuntimeDiagnostic(diagnostic, payload)

	if connectedCount <= 0 && readyCount <= 0 {
		if runtimeReportsProcessProxy(payload) {
			diagnostic.Code = "wx_comment_export_process_proxy_no_client"
			return buildNoReadyProcessProxyDetail("comment export", payload), diagnostic
		}
		if hint := detectLocalWXFeedListProxyMismatch(baseURL); hint != nil {
			diagnostic.Code = wxCommentExportProxyMismatchCode
			diagnostic.CurrentBrowserProxyAddress = hint.CurrentProxyAddress
			diagnostic.ExpectedProxyAddress = hint.ExpectedProxyAddress
			return "comment export has no ready page client：当前 Windows 代理配置是 " + hint.CurrentProxyAddress + "，不是 wx_channel 预期的 " + hint.ExpectedProxyAddress + "；当前 /api/channels/status 还是 clients=0、ready_clients=0，说明视频号独立客户端还没有经过 wx_channel 代理连进来。请先确认客户端确实走 " + hint.ExpectedProxyAddress + "，再切回目标视频重试评论刷新。", diagnostic
		}
		return "comment export has no ready page client：wx_channel 已启动，但当前没有就绪的视频号客户端。请先在视频号独立客户端切回目标视频，等待 API 初始化完成后再重试评论刷新。", diagnostic
	}

	detail := "comment export has no ready page client：wx_channel 当前已有 " + strconv.Itoa(maxInt(0, connectedCount)) + " 个客户端连接"
	if readyCount > 0 {
		detail += "，其中 " + strconv.Itoa(readyCount) + " 个 API 已就绪"
	}
	detail += "，但当前还没有可用于评论导出的就绪客户端上下文。"
	if diagnostic.CurrentPagePath != "" {
		detail += " 当前客户端上下文 " + diagnostic.CurrentPagePath + "。"
	}
	detail += " 请先在视频号独立客户端切回目标视频，并确认请求继续经过 wx_channel 代理后再重试。"
	return detail, diagnostic
}

func buildNoReadyWXFeedListDetail(baseURL, healthURL string, clientCount *int, healthStatus string) (string, *SourceBindingDiagnostic) {
	diagnostic := &SourceBindingDiagnostic{
		Code:            "wx_feed_list_no_ready_page_client",
		ResolvedBaseURL: normalizeWXFeedListBaseURL(baseURL),
		WSHealthURL:     strings.TrimSpace(healthURL),
		HealthStatus:    strings.TrimSpace(healthStatus),
	}
	if clientCount != nil {
		count := *clientCount
		diagnostic.ReadyClientCount = &count
	}

	if hint := detectLocalWXFeedListProxyMismatch(baseURL); hint != nil {
		diagnostic.Code = "wx_feed_list_proxy_mismatch"
		diagnostic.CurrentBrowserProxyAddress = hint.CurrentProxyAddress
		diagnostic.ExpectedProxyAddress = hint.ExpectedProxyAddress
		return "feed list has no ready page client：当前 Windows 代理配置是 " + hint.CurrentProxyAddress + "，不是 wx_channel 预期的 " + hint.ExpectedProxyAddress + "；视频号独立客户端只有在请求经过 wx_channel 代理拦截时，才会向 API 暴露可抓取能力。请先确认客户端确实走 " + hint.ExpectedProxyAddress + "，再检查 /api/channels/status 是否出现 ready client。", diagnostic
	}
	return "feed list has no ready page client：wx_channel 已启动，但当前没有就绪的视频号客户端。请先确认视频号独立客户端仍在运行，并且请求已经被 wx_channel 代理拦截；然后检查 /api/channels/status 是否出现 ready client。", diagnostic
}

func buildNoReadyWXCommentExportDetail(baseURL, healthURL string, clientCount *int, healthStatus string) (string, *SourceBindingDiagnostic) {
	diagnostic := &SourceBindingDiagnostic{
		Code:            "wx_comment_export_no_ready_page_client",
		ResolvedBaseURL: normalizeWXFeedListBaseURL(baseURL),
		WSHealthURL:     strings.TrimSpace(healthURL),
		HealthStatus:    strings.TrimSpace(healthStatus),
	}
	if clientCount != nil {
		count := *clientCount
		diagnostic.ReadyClientCount = &count
	}
	if hint := detectLocalWXFeedListProxyMismatch(baseURL); hint != nil {
		diagnostic.Code = wxCommentExportProxyMismatchCode
		diagnostic.CurrentBrowserProxyAddress = hint.CurrentProxyAddress
		diagnostic.ExpectedProxyAddress = hint.ExpectedProxyAddress
		return "comment export has no ready page client：当前 Windows 代理配置是 " + hint.CurrentProxyAddress + "，不是 wx_channel 预期的 " + hint.ExpectedProxyAddress + "；视频号独立客户端只有在请求经过 wx_channel 代理拦截时，才会向 API 暴露评论导出能力。请先确认客户端确实走 " + hint.ExpectedProxyAddress + "，再切回目标视频重试评论刷新。", diagnostic
	}
	return "comment export has no ready page client：wx_channel 已启动，但当前没有就绪的视频号客户端。请先在视频号独立客户端切回目标视频，等待 API 初始化完成后再重试评论刷新。", diagnostic
}

func intPtr(value int) *int {
	next := value
	return &next
}

func boolPtr(value bool) *bool {
	next := value
	return &next
}

func resolveFallbackWXFeedListBaseURL(baseURL string) string {
	normalizedBaseURL := normalizeWXFeedListBaseURL(baseURL)
	if sameWXFeedListBaseURL(normalizedBaseURL, defaultLocalWXFeedListBaseURL) {
		return ""
	}
	if !isLoopbackWXFeedListBaseURL(normalizedBaseURL) {
		return ""
	}
	return defaultLocalWXFeedListBaseURL
}

func sameWXFeedListBaseURL(left, right string) bool {
	return strings.EqualFold(normalizeWXFeedListBaseURL(left), normalizeWXFeedListBaseURL(right))
}

func isLoopbackWXFeedListBaseURL(baseURL string) bool {
	parsed, err := url.Parse(normalizeWXFeedListBaseURL(baseURL))
	if err != nil {
		return false
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isHostDockerInternalWXFeedListBaseURL(baseURL string) bool {
	parsed, err := url.Parse(normalizeWXFeedListBaseURL(baseURL))
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(parsed.Hostname()), "host.docker.internal")
}

func expectedWXChannelProxyAddress(baseURL string) string {
	parsed, err := url.Parse(normalizeWXFeedListBaseURL(baseURL))
	if err != nil {
		return ""
	}
	port := parsed.Port()
	if port == "" {
		return ""
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber <= 1 {
		return ""
	}
	return net.JoinHostPort(parsed.Hostname(), strconv.Itoa(portNumber-1))
}
