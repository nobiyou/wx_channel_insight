package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type TrustedRuntimeEvidence struct {
	State                     string    `json:"state,omitempty"`
	VerifiedAt                time.Time `json:"verifiedAt,omitempty"`
	Source                    string    `json:"source,omitempty"`
	HubUserID                 string    `json:"hubUserId,omitempty"`
	APIURL                    string    `json:"apiURL,omitempty"`
	WebURL                    string    `json:"webURL,omitempty"`
	TrustedBoundaryMode       string    `json:"trustedBoundaryMode,omitempty"`
	APITrustedContextState    string    `json:"apiTrustedContextState,omitempty"`
	APIBrowserQueryState      string    `json:"apiBrowserQueryState,omitempty"`
	APIBrowserQueryStatusCode int       `json:"apiBrowserQueryStatusCode,omitempty"`
	WebTrustedContextState    string    `json:"webTrustedContextState,omitempty"`
	WebBrowserQueryState      string    `json:"webBrowserQueryState,omitempty"`
	Summary                   string    `json:"summary,omitempty"`
	Notes                     []string  `json:"notes,omitempty"`
}

type verifyRuntimeProofFile struct {
	Status    string                   `json:"status"`
	HubUserID string                   `json:"hubUserId"`
	APIPort   int                      `json:"apiPort"`
	WebPort   int                      `json:"webPort"`
	Results   []verifyRuntimeProofStep `json:"results"`
}

type verifyRuntimeProofStep struct {
	Name     string `json:"name"`
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
}

type verifiedRuntimeStatus struct {
	HubBridgeMode      string `json:"hubBridgeMode"`
	DemoContextEnabled bool   `json:"demoContextEnabled"`
}

type trustedProofContext struct {
	HubUserID     string `json:"hubUserId"`
	ContextSource string `json:"contextSource"`
}

type trustedBrowserProbe struct {
	StatusCode int                  `json:"statusCode"`
	JSON       *trustedProofContext `json:"json"`
}

func LoadTrustedRuntimeEvidence(root string) TrustedRuntimeEvidence {
	resolvedRoot := resolveTrustedRuntimeEvidenceRoot(root)
	if resolvedRoot == "" {
		return finalizeTrustedRuntimeEvidence(TrustedRuntimeEvidence{})
	}
	if evidence, ok := loadStoredTrustedRuntimeEvidence(resolvedRoot); ok {
		return evidence
	}
	return finalizeTrustedRuntimeEvidence(TrustedRuntimeEvidence{})
}

func resolveTrustedRuntimeEvidenceRoot(root string) string {
	if strings.TrimSpace(root) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return ""
		}
		root = cwd
	}
	resolved, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return resolved
}

func loadVerifyTrustedRuntimeEvidence(path string, verifiedAt time.Time) (TrustedRuntimeEvidence, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return TrustedRuntimeEvidence{}, false
	}

	var payload verifyRuntimeProofFile
	if err := json.Unmarshal(raw, &payload); err != nil {
		return TrustedRuntimeEvidence{}, false
	}
	if payload.Status != "success" {
		return TrustedRuntimeEvidence{}, false
	}

	evidence := TrustedRuntimeEvidence{
		VerifiedAt: verifiedAt,
		Source:     "verify-insight-docker-stack",
		HubUserID:  payload.HubUserID,
		APIURL:     buildLocalURL(payload.APIPort),
		WebURL:     buildLocalURL(payload.WebPort),
		Notes: []string{
			"bridge 验证由 verify-insight-docker-stack.ps1 显式保存。",
		},
	}

	steps := make(map[string]verifyRuntimeProofStep, len(payload.Results))
	for _, item := range payload.Results {
		steps[item.Name] = item
	}

	if step, ok := steps["check-runtime-status"]; ok {
		var status verifiedRuntimeStatus
		if parseJSONText(step.Output, &status) {
			evidence.TrustedBoundaryMode = deriveTrustedBoundaryMode(status.HubBridgeMode, status.DemoContextEnabled)
		}
	}
	if step, ok := steps["check-runtime-trusted-context"]; ok {
		var context trustedProofContext
		if parseJSONText(step.Output, &context) {
			evidence.APITrustedContextState = evaluateTrustedContextState(context)
			if evidence.HubUserID == "" {
				evidence.HubUserID = context.HubUserID
			}
		}
	}
	if step, ok := steps["check-runtime-browser-query-blocked"]; ok {
		var probe trustedBrowserProbe
		if parseJSONText(step.Output, &probe) {
			evidence.APIBrowserQueryStatusCode = probe.StatusCode
			evidence.APIBrowserQueryState = evaluateAPIBrowserQueryState(probe)
		}
	}
	if step, ok := steps["check-trusted-web-context"]; ok {
		var context trustedProofContext
		if parseJSONText(step.Output, &context) {
			evidence.WebTrustedContextState = evaluateTrustedContextState(context)
		}
	}
	if step, ok := steps["check-trusted-web-browser-query-isolated"]; ok {
		var context trustedProofContext
		if parseJSONText(step.Output, &context) {
			evidence.WebBrowserQueryState = evaluateWebBrowserQueryState(context, payload.HubUserID)
		}
	}
	return evidence, true
}

func hasTrustedRuntimeEvidence(evidence TrustedRuntimeEvidence) bool {
	return (strings.TrimSpace(evidence.TrustedBoundaryMode) != "" && evidence.TrustedBoundaryMode != "unknown") ||
		(evidence.APITrustedContextState != "" && evidence.APITrustedContextState != "missing") ||
		(evidence.APIBrowserQueryState != "" && evidence.APIBrowserQueryState != "missing") ||
		(evidence.WebTrustedContextState != "" && evidence.WebTrustedContextState != "missing") ||
		(evidence.WebBrowserQueryState != "" && evidence.WebBrowserQueryState != "missing")
}

func parseJSONText(raw string, target any) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	return json.Unmarshal([]byte(raw), target) == nil
}

func deriveTrustedBoundaryMode(hubBridgeMode string, demoContextEnabled bool) string {
	switch strings.TrimSpace(hubBridgeMode) {
	case "local-operator", "local-operator+trusted-header":
		if demoContextEnabled {
			return "local-operator-with-demo-fallback"
		}
		return "local-operator"
	case "trusted-header":
		if demoContextEnabled {
			return "trusted-with-demo-fallback"
		}
		return "trusted-only"
	case "hub-session", "hub-session+trusted-header":
		return "hub-session"
	case "demo-context":
		return "demo-context"
	default:
		return "unknown"
	}
}

func evaluateTrustedContextState(context trustedProofContext) string {
	if strings.TrimSpace(context.ContextSource) == "" && strings.TrimSpace(context.HubUserID) == "" {
		return "missing"
	}
	switch context.ContextSource {
	case "trusted_header", "local_operator", "hub_session":
		if strings.TrimSpace(context.HubUserID) != "" {
			return "verified"
		}
	}
	return "failed"
}

func evaluateAPIBrowserQueryState(probe trustedBrowserProbe) string {
	switch {
	case probe.StatusCode == 401:
		return "blocked"
	case probe.StatusCode == 200 && probe.JSON != nil && probe.JSON.ContextSource == "query":
		return "fallback"
	case probe.StatusCode == 200 && probe.JSON != nil &&
		(probe.JSON.ContextSource == "local_operator" || probe.JSON.ContextSource == "trusted_header" || probe.JSON.ContextSource == "hub_session") &&
		probe.JSON.HubUserID != "hub:user:browser-probe":
		// Hub-less local operator may answer without headers using the default operator id,
		// as long as browser query identity is not adopted.
		return "isolated"
	case probe.StatusCode == 0:
		return "missing"
	default:
		return "failed"
	}
}

func evaluateWebBrowserQueryState(context trustedProofContext, expectedHubUserID string) string {
	if strings.TrimSpace(context.ContextSource) == "" && strings.TrimSpace(context.HubUserID) == "" {
		return "missing"
	}
	switch context.ContextSource {
	case "trusted_header", "local_operator", "hub_session":
		if context.HubUserID == "hub:user:browser-probe" {
			return "failed"
		}
		if expectedHubUserID == "" || context.HubUserID == expectedHubUserID {
			return "isolated"
		}
	}
	return "failed"
}

func buildLocalURL(port int) string {
	if port <= 0 {
		return ""
	}
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

func hasTrustedRuntimeMetadataEvidence(evidence TrustedRuntimeEvidence) bool {
	return !evidence.VerifiedAt.IsZero() ||
		strings.TrimSpace(evidence.Source) != "" ||
		strings.TrimSpace(evidence.HubUserID) != "" ||
		strings.TrimSpace(evidence.APIURL) != "" ||
		strings.TrimSpace(evidence.WebURL) != "" ||
		len(evidence.Notes) > 0
}

func collectTrustedRuntimeVerifiedMetadataGaps(evidence TrustedRuntimeEvidence) []string {
	gaps := []string{}
	if evidence.VerifiedAt.IsZero() {
		gaps = append(gaps, "verifiedAt 缺失")
	}
	if strings.TrimSpace(evidence.Source) == "" {
		gaps = append(gaps, "source 缺失")
	}
	if strings.TrimSpace(evidence.HubUserID) == "" {
		gaps = append(gaps, "hubUserId 缺失")
	}
	return gaps
}

func finalizeTrustedRuntimeEvidence(evidence TrustedRuntimeEvidence) TrustedRuntimeEvidence {
	if evidence.TrustedBoundaryMode == "" {
		evidence.TrustedBoundaryMode = "unknown"
	}
	if evidence.APITrustedContextState == "" {
		evidence.APITrustedContextState = "missing"
	}
	if evidence.APIBrowserQueryState == "" {
		evidence.APIBrowserQueryState = "missing"
	}
	if evidence.WebTrustedContextState == "" {
		evidence.WebTrustedContextState = "missing"
	}
	if evidence.WebBrowserQueryState == "" {
		evidence.WebBrowserQueryState = "missing"
	}
	apiVerified := evidence.APITrustedContextState == "verified"
	apiBrowserIsolated := evidence.APIBrowserQueryState == "blocked" || evidence.APIBrowserQueryState == "isolated"
	webVerified := evidence.WebTrustedContextState == "verified"
	webIsolated := evidence.WebBrowserQueryState == "isolated"
	boundaryOK := evidence.TrustedBoundaryMode == "trusted-only" ||
		evidence.TrustedBoundaryMode == "local-operator" ||
		evidence.TrustedBoundaryMode == "hub-session"
	verifiedSemanticsComplete := apiVerified &&
		apiBrowserIsolated &&
		webVerified &&
		webIsolated &&
		boundaryOK
	verifiedMetadataGaps := collectTrustedRuntimeVerifiedMetadataGaps(evidence)
	verifiedMetadataComplete := len(verifiedMetadataGaps) == 0
	verifiedComplete := verifiedSemanticsComplete && verifiedMetadataComplete
	hasAnyProofEvidence := apiVerified || apiBrowserIsolated || webVerified || webIsolated ||
		evidence.APITrustedContextState != "missing" ||
		evidence.APIBrowserQueryState != "missing" ||
		evidence.WebTrustedContextState != "missing" ||
		evidence.WebBrowserQueryState != "missing"
	hasAnyEvidence := hasAnyProofEvidence || hasTrustedRuntimeMetadataEvidence(evidence)
	if evidence.State == "verified" && !verifiedComplete {
		if hasAnyEvidence {
			evidence.State = "partial"
		} else {
			evidence.State = "missing"
		}
		evidence.Summary = ""
	}
	if evidence.State == "" {
		switch {
		case verifiedComplete:
			evidence.State = "verified"
		case hasAnyEvidence:
			evidence.State = "partial"
		default:
			evidence.State = "missing"
		}
	}
	if evidence.Summary == "" {
		evidence.Summary = buildTrustedRuntimeSummary(evidence)
	}
	return evidence
}

func buildTrustedRuntimeSummary(evidence TrustedRuntimeEvidence) string {
	verifiedMetadataGaps := collectTrustedRuntimeVerifiedMetadataGaps(evidence)
	switch evidence.State {
	case "verified":
		return "最近一次验证已经同时覆盖 API trusted-only 边界和 Web proxy 隔离，当前这套联调链路具备可见证据。"
	case "partial":
		switch {
		case evidence.APITrustedContextState == "verified" &&
			evidence.APIBrowserQueryState == "blocked" &&
			evidence.WebTrustedContextState == "verified" &&
			evidence.WebBrowserQueryState == "isolated" &&
			len(verifiedMetadataGaps) > 0:
			return "最近一次 trusted runtime proof 仍缺少 " + strings.Join(verifiedMetadataGaps, " / ") + "，还不能把这条记录当成当前 live 的完整放行证据。"
		case evidence.APITrustedContextState == "verified" && evidence.APIBrowserQueryState == "blocked" && evidence.WebBrowserQueryState == "missing":
			return "已经记录到 API trusted-only 边界，但还没有看到最近一次浏览器侧 query-isolation 证明。"
		case evidence.TrustedBoundaryMode == "local-operator" || evidence.TrustedBoundaryMode == "local-operator-with-demo-fallback":
			return "当前是 Hub-less local-operator 证据，但 API / Web 两侧验证状态尚未同时闭合。"
		case evidence.TrustedBoundaryMode == "trusted-with-demo-fallback" || evidence.APIBrowserQueryState == "fallback":
			return "当前已经启用 trusted bridge，但最近一次验证仍保留 demo fallback，正式 live 边界还没有完全收紧。"
		default:
			return "已经有部分 trusted runtime 证据，但还不足以证明 API 和 Web 两侧都完成隔离。"
		}
	default:
		return "当前还没有找到最近一次 trusted runtime 验证记录，建议先运行 Docker 栈验证或可信 Web 开发检查，再让团队值班。"
	}
}
