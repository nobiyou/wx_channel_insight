const MODULE_LABELS = Object.freeze({
  "content-analysis": "内容分析",
  "topic-trends": "关键词样本解释",
  "account-performance": "账号表现",
  "comment-insights": "评论洞察",
});

const STATUS_META = Object.freeze({
  completed: { label: "模型完成", tone: "good", detail: "已生成结构化建议", hasResult: true },
  fallback: { label: "已降级", tone: "warn", detail: "本次未保存模型结果", hasResult: false },
  failed: { label: "分析失败", tone: "risk", detail: "本次未保存模型结果", hasResult: false },
  disabled: { label: "未启用", tone: "warn", detail: "当前 Prompt profile 未启用", hasResult: false },
  insufficient_evidence: { label: "证据不足", tone: "warn", detail: "当前输入不足以发起可靠分析", hasResult: false },
});

const RESULT_FIELD_LABELS = Object.freeze({
  title: "标题",
  summary: "摘要",
  headline: "标题",
  strengths: "优势",
  weaknesses: "短板",
  audience: "受众判断",
  positioning: "定位判断",
  trendStage: "趋势阶段",
  drivers: "驱动因素",
  opportunities: "机会",
  recommendations: "建议动作",
  monitoringAdvice: "监控建议",
  themes: "评论主题",
  needs: "受众需求",
  sentiment: "情绪判断",
  sampleBoundary: "样本边界",
  caveats: "注意事项",
  limitations: "局限",
  keyFindings: "关键发现",
  recommendedActions: "推荐动作",
  watchouts: "风险提醒",
  facts: "事实",
  judgments: "判断",
});

const RESULT_FIELD_ORDER = Object.freeze([
  "title", "headline", "summary", "strengths", "weaknesses", "audience", "positioning",
  "trendStage", "drivers", "themes", "needs", "sentiment", "sampleBoundary",
  "opportunities", "recommendations", "monitoringAdvice", "keyFindings",
  "recommendedActions", "watchouts", "facts", "judgments", "caveats", "limitations",
]);

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function asText(value) {
  return typeof value === "string" ? value.trim() : "";
}

function normalizeStringList(value) {
  if (!Array.isArray(value)) return [];
  return value.filter((item) => typeof item === "string").map((item) => item.trim()).filter(Boolean);
}

function parseResult(value) {
  if (isRecord(value)) return value;
  if (typeof value !== "string" || !value.trim()) return null;
  try {
    const parsed = JSON.parse(value);
    return isRecord(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

export function getAIAnalysisModuleLabel(module) {
  const key = String(module ?? "").trim();
  return MODULE_LABELS[key] || key || "AI 分析";
}

export function getAIAnalysisStatusMeta(status) {
  const key = String(status ?? "").trim().toLowerCase();
  return STATUS_META[key] || {
    label: "状态未知",
    tone: "risk",
    detail: "运行状态无法识别",
    hasResult: false,
  };
}

export function normalizeAIAnalysisResult(module, value) {
  const parsed = parseResult(value);
  if (!parsed) return null;
  const result = {};
  const keys = [
    ...RESULT_FIELD_ORDER,
    ...Object.keys(parsed).filter((key) => !RESULT_FIELD_ORDER.includes(key)),
  ];
  for (const key of keys) {
    if (!(key in parsed) || key === "evidenceRefs") continue;
    const value = parsed[key];
    if (Array.isArray(value)) {
      const items = normalizeStringList(value);
      if (items.length) result[key] = items;
      continue;
    }
    const text = asText(value);
    if (text) result[key] = text;
  }
  return Object.keys(result).length ? result : null;
}

export function buildAIAnalysisResultSections(module, value) {
  const result = normalizeAIAnalysisResult(module, value);
  if (!result) return [];
  return Object.entries(result).map(([key, sectionValue]) => ({
    key,
    label: RESULT_FIELD_LABELS[key] || key,
    type: Array.isArray(sectionValue) ? "list" : "text",
    value: sectionValue,
  }));
}

export function buildAIAnalysisEvidenceRefs(value) {
  return normalizeStringList(parseResult(value)?.evidenceRefs);
}

export function formatAIAnalysisTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString("zh-CN", { hour12: false });
}

export function buildAIAnalysisTrace(run) {
  if (!isRecord(run)) return [];
  const duration = Number(run.durationMs);
  const trace = [
    { key: "scope", label: "分析范围", value: asText(run.scopeKey) || "—" },
    { key: "window", label: "时间窗", value: asText(run.window) || "全部" },
    { key: "model", label: "模型", value: [asText(run.modelProvider), asText(run.modelName)].filter(Boolean).join(" · ") || "未调用" },
    { key: "prompt", label: "Prompt 版本", value: asText(run.promptVersion) || "—" },
    { key: "engine", label: "分析引擎", value: asText(run.analysisEngine) || "—" },
    { key: "source", label: "配置来源", value: asText(run.configSource) || "—" },
    { key: "duration", label: "耗时", value: Number.isFinite(duration) && duration >= 0 ? `${Math.round(duration)} ms` : "—" },
    { key: "run-at", label: "运行时间", value: formatAIAnalysisTime(run.runAt) },
    { key: "fingerprint", label: "输入指纹", value: asText(run.inputFingerprint) || "—" },
  ];
  return trace.filter((item) => item.value !== "—" || ["scope", "window", "run-at"].includes(item.key));
}

export function normalizeAIAnalysisRun(module, run) {
  if (!isRecord(run)) return null;
  const status = String(run.status ?? "").trim().toLowerCase();
  const statusMeta = getAIAnalysisStatusMeta(status);
  const result = statusMeta.hasResult ? normalizeAIAnalysisResult(module || run.module, run.result) : null;
  return {
    ...run,
    module: asText(run.module) || String(module ?? "").trim(),
    status,
    result,
    resultSections: result ? buildAIAnalysisResultSections(module || run.module, result) : [],
    evidenceRefs: result ? buildAIAnalysisEvidenceRefs(run.result) : [],
    statusMeta,
    trace: buildAIAnalysisTrace(run),
  };
}

export function buildAIAnalysisVariables(module, source = {}) {
  const values = isRecord(source) ? source : {};
  if (isRecord(values.variables)) return { ...values.variables };
  const data = values.data ?? values.dataJSON ?? values.dashboard ?? {};
  switch (String(module ?? "").trim()) {
    case "content-analysis": return { data_json: data, video_json: values.video ?? values.videoJSON ?? {} };
    case "topic-trends": return { data_json: data };
    case "account-performance": return { data_json: data, account_json: values.account ?? values.accountJSON ?? {} };
    case "comment-insights": return { data_json: data, comments_json: values.comments ?? values.commentsJSON ?? [] };
    default: return {};
  }
}

export function buildAIAnalysisPayload(module, options = {}) {
  const values = isRecord(options) ? options : {};
  return {
    scopeKey: asText(values.scopeKey) || "dashboard",
    window: asText(values.window) || "all",
    variables: buildAIAnalysisVariables(module, values),
  };
}
