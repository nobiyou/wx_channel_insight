export const OPERATIONS_NAVIGATION = Object.freeze([
  { key: "dashboard", label: "运营概览" },
  { key: "accounts", label: "账号监控" },
  { key: "keywords", label: "关键词监控" },
  { key: "videos", label: "视频分析" },
  { key: "agent", label: "智能运营" },
  { key: "settings", label: "辅助设置" },
]);

export function buildOperationsHash(view, params = {}) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) if (value !== undefined && value !== null && value !== "") query.set(key, String(value));
  return `#${view}${query.size ? `?${query}` : ""}`;
}

export function parseOperationsRoute(hash = "") {
  const raw = String(hash).replace(/^#/, "");
  const [path, search = ""] = raw.split("?");
  const params = Object.fromEntries(new URLSearchParams(search));
  const key = path || "dashboard";
  if (key === "account" || key === "video") {
    const id = params.id || "";
    const fallback = key === "account" ? "#accounts" : "#videos";
    const requestedReturn = String(params.return || fallback);
    const returnHash = /^#(?:dashboard|accounts|keywords|videos|account|agent)(?:[/?]|$)/.test(requestedReturn) ? requestedReturn : fallback;
    return { view: key, id, params, returnHash };
  }
  return { view: OPERATIONS_NAVIGATION.some((item) => item.key === key) ? key : "dashboard", params, returnHash: "" };
}

export function videoFactsQuery(params = {}, view = "videos") {
  return {
    q: String(params.q || "").trim(),
    monitorKeyword: String(params.keyword || "").trim(),
    account: String(params.account || "").trim(),
    from: params.from || "", to: params.to || "",
    publishedFrom: params.publishedFrom || "", publishedTo: params.publishedTo || "",
    compareBy: params.compareBy === "keyword" ? "keyword" : "account",
	curation: ["all", "unreviewed", "selected", "watching", "excluded"].includes(params.curation) ? params.curation : "active",
	performance: ["standout", "typical", "underperforming", "insufficient_data"].includes(params.performance) ? params.performance : "",
	minInteractions: /^\d+$/.test(String(params.minInteractions ?? "")) ? String(params.minInteractions) : "",
	minLikes: /^\d+$/.test(String(params.minLikes ?? "")) ? String(params.minLikes) : "",
	freshOnly: params.freshOnly === true || params.freshOnly === "true",
	momentumOnly: params.momentumOnly === true || params.momentumOnly === "true",
	recentOnly: params.recentOnly === true || params.recentOnly === "true",
	hasKeyword: params.hasKeyword === true || params.hasKeyword === "true",
	tag: String(params.tag || "").trim(),
    page: Math.max(1, Number(params.page) || 1),
    pageSize: Number(params.pageSize) === 50 ? 50 : 20,
    sort: ["observed_at", "published_at", "likes", "comments", "favorites", "forwards", "growth_rate", "interactions", "growth_per_hour", "relative_performance", "recent_heat"].includes(params.sort) ? params.sort : view === "keywords" ? "interactions" : "observed_at",
    direction: params.direction === "asc" ? "asc" : "desc",
  };
}

export function monitoringTasksForScope(tasks = [], scope = "diagnostics", targetKey = "", targetAliases = []) {
  if (scope === "diagnostics") return tasks;
  return tasks.filter((task) => {
    if (task.system || task.scopeMode === "all_saved_bindings") return false;
    const targets = Array.isArray(task.targets) ? task.targets : [];
    if (scope === "keywords") return (task.taskType === "legacy" || !task.taskType) && targets.length > 0 && targets.every((target) => target.source === "wx_video_search") && (!targetKey || targets.some((target) => target.targetKey === targetKey));
    if (task.taskType === "comment_collection") return false;
    const belongsToAccounts = ["account_profile", "video_list"].includes(task.taskType) || targets.length > 0 && targets.every((target) => ["account", "wx_feed_list"].includes(target.source));
    const keys = [targetKey, ...targetAliases].filter(Boolean);
    return belongsToAccounts && (!keys.length || task.scopeMode === "all_accounts" || targets.some((target) => keys.includes(target.accountId) || keys.includes(target.targetKey)));
  });
}
