export const AI_SERVICE_DEFAULTS = Object.freeze({
  ownerId: "",
  provider: "openai",
  baseUrl: "https://api.openai.com/v1",
  model: "gpt-5.6",
  timeoutSeconds: 45,
  analysisEngine: "openai",
  fallbackPolicy: "rule-engine",
  source: "none",
  apiKeyConfigured: false,
  apiKeyHint: "",
  apiKeyPersistenceAvailable: false,
  verificationStatus: "never",
  lastVerifiedAt: "",
  lastVerifyLatencyMs: 0,
  lastVerifyError: "",
  updatedAt: "",
});

export function normalizeAIServiceSettings(payload = {}) {
  const timeout = Number(payload?.timeoutSeconds);
  return {
    ...AI_SERVICE_DEFAULTS,
    ownerId: String(payload?.ownerId || "").trim(),
    provider: String(payload?.provider || AI_SERVICE_DEFAULTS.provider).trim().toLowerCase(),
    baseUrl: String(payload?.baseUrl || AI_SERVICE_DEFAULTS.baseUrl).trim(),
    model: String(payload?.model || AI_SERVICE_DEFAULTS.model).trim(),
    timeoutSeconds: Number.isFinite(timeout) ? timeout : AI_SERVICE_DEFAULTS.timeoutSeconds,
    analysisEngine: String(payload?.analysisEngine || AI_SERVICE_DEFAULTS.analysisEngine).trim().toLowerCase(),
    fallbackPolicy: String(payload?.fallbackPolicy || AI_SERVICE_DEFAULTS.fallbackPolicy).trim().toLowerCase(),
    source: String(payload?.source || "none").trim().toLowerCase(),
    apiKeyConfigured: payload?.apiKeyConfigured === true,
    modelAvailable: payload?.modelAvailable === true,
    configurationIssue: String(payload?.configurationIssue || "").trim(),
    apiKeyHint: String(payload?.apiKeyHint || "").trim(),
    apiKeyPersistenceAvailable: payload?.apiKeyPersistenceAvailable === true,
    verificationStatus: String(payload?.verificationStatus || "never").trim().toLowerCase(),
    lastVerifiedAt: String(payload?.lastVerifiedAt || "").trim(),
    lastVerifyLatencyMs: Math.max(0, Number(payload?.lastVerifyLatencyMs) || 0),
    lastVerifyError: String(payload?.lastVerifyError || "").trim(),
    updatedAt: String(payload?.updatedAt || "").trim(),
  };
}

export function validateAIServiceDraft(draft = {}) {
  if (String(draft.provider || "").trim().toLowerCase() !== "openai") {
    return "当前仅支持 OpenAI";
  }
  const baseUrl = String(draft.baseUrl || "").trim();
  try {
    const parsed = new URL(baseUrl);
    if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
      return "Base URL 必须使用 HTTP 或 HTTPS";
    }
  } catch {
    return "请输入完整的 Base URL";
  }
  if (!String(draft.model || "").trim()) {
    return "模型名称不能为空";
  }
  const timeout = Number(draft.timeoutSeconds);
  if (!Number.isInteger(timeout) || timeout < 5 || timeout > 300) {
    return "请求超时必须是 5–300 秒之间的整数";
  }
  if (!["openai", "rule-engine"].includes(String(draft.analysisEngine || "openai").trim().toLowerCase())) {
    return "请选择有效的分析引擎";
  }
  if (!["rule-engine", "error"].includes(String(draft.fallbackPolicy || "rule-engine").trim().toLowerCase())) {
    return "请选择有效的失败策略";
  }
  return "";
}

export function buildAIServiceDraftPayload(draft = {}, apiKey = "") {
  const payload = {
    provider: String(draft.provider || "openai").trim().toLowerCase(),
    baseUrl: String(draft.baseUrl || "").trim().replace(/\/+$/, ""),
    model: String(draft.model || "").trim(),
    timeoutSeconds: Number(draft.timeoutSeconds),
  };
  if (draft.analysisEngine !== undefined) {
    payload.analysisEngine = String(draft.analysisEngine || "openai").trim().toLowerCase();
  }
  if (draft.fallbackPolicy !== undefined) {
    payload.fallbackPolicy = String(draft.fallbackPolicy || "rule-engine").trim().toLowerCase();
  }
  const normalizedKey = String(apiKey || "").trim();
  if (normalizedKey) {
    payload.apiKey = normalizedKey;
  }
  return payload;
}

export function aiServiceSourceLabel(source) {
  return {
    workspace: "当前工作区",
    environment: "服务器环境变量",
    none: "尚未配置",
  }[String(source || "").trim().toLowerCase()] || "未知来源";
}

export function verificationStatusMeta(status) {
  return {
    verified: { label: "已验证", tone: "success" },
    pending: { label: "待验证", tone: "warning" },
    failed: { label: "验证失败", tone: "error" },
    never: { label: "未验证", tone: "neutral" },
  }[String(status || "").trim().toLowerCase()] || { label: "未验证", tone: "neutral" };
}

export function verificationResultMeta(result = {}) {
  if (result?.ok === true) {
    return {
      tone: "success",
      title: "连接验证通过",
      detail: `${String(result.model || "OpenAI").trim()} · ${Math.max(0, Number(result.latencyMs) || 0)} ms`,
    };
  }
  const code = String(result?.errorCode || "upstream").trim();
  const labels = {
    authentication: "API Key 无效或权限不足",
    rate_limit: "请求受到限流或账户额度不足",
    timeout: "连接验证超时",
    network: "无法连接到服务地址",
    invalid_config: "配置格式不正确",
    invalid_response: "服务返回了无法识别的响应",
    not_configured: "尚未提供可用的 API Key",
    upstream: "OpenAI 服务返回异常",
  };
  return {
    tone: "error",
    title: labels[code] || "连接验证失败",
    detail: String(result?.message || "请检查配置后重试").trim(),
  };
}
