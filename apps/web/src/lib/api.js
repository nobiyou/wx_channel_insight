import {
  buildAPIURL,
  buildRequestHeaders,
  setLiveRequestContextPolicy,
} from "./requestContext.js";

async function buildAPIError(response) {
  let message = `HTTP ${response.status}`;
  try {
    const payload = await response.json();
    if (
      payload &&
      typeof payload.message === "string" &&
      payload.message.trim()
    ) {
      message = payload.message.trim();
    } else if (
      payload &&
      typeof payload.error === "string" &&
      payload.error.trim()
    ) {
      message = payload.error.trim();
    }
  } catch {
    // ignore non-JSON error bodies
  }
  return new Error(message);
}

function buildAPIURLWithQuery(path, query = {}) {
  const resolved = buildAPIURL(path);
  const [pathname, search = ""] = resolved.split("?");
  const params = new URLSearchParams(search);

  Object.entries(query).forEach(([key, value]) => {
    const normalized = String(value ?? "").trim();
    if (normalized) {
      params.set(key, normalized);
    }
  });

  const nextQuery = params.toString();
  return nextQuery ? `${pathname}?${nextQuery}` : pathname;
}

export async function fetchWorkspaceContext() {
  try {
    const path = "/api/v1/workspace/context";
    const response = await fetch(buildAPIURL(path), {
      cache: "no-store",
      headers: buildRequestHeaders(path),
    });
    if (!response.ok) {
      throw await buildAPIError(response);
    }
    const payload = await response.json();
    return {
      data: payload,
      source: "api",
      error: null,
    };
  } catch (error) {
    return {
      data: null,
      source: "unavailable",
      error: error instanceof Error ? error.message : "unknown error",
    };
  }
}

export async function fetchPromptSettings() {
  return accountJSON("/api/v1/settings/prompts");
}

export async function updatePromptSetting(key, patch) {
  if (!["content-analysis", "topic-trends", "account-performance", "comment-insights"].includes(String(key).trim())) throw new Error("This prompt has been removed from operations analysis");
  const normalizedKey = String(key ?? "").trim();
  if (!normalizedKey) {
    throw new Error("Prompt profile key is required");
  }
  return accountJSON(`/api/v1/settings/prompts/${encodeURIComponent(normalizedKey)}`, {
    method: "PUT",
    body: patch,
  });
}

export async function resetPromptSetting(key) {
  if (!["content-analysis", "topic-trends", "account-performance", "comment-insights"].includes(String(key).trim())) throw new Error("This prompt has been removed from operations analysis");
  const normalizedKey = String(key ?? "").trim();
  if (!normalizedKey) {
    throw new Error("Prompt profile key is required");
  }
  return accountJSON(`/api/v1/settings/prompts/${encodeURIComponent(normalizedKey)}/reset`, {
    method: "POST",
  });
}

export async function fetchAIServiceSettings() {
  return accountJSON("/api/v1/settings/ai-service");
}

export async function updateAIServiceSettings(patch, options = {}) {
  const body = { ...(patch || {}) };
  if (options?.verifyOnSave === true) {
    body.verifyOnSave = true;
  }
  return accountJSON("/api/v1/settings/ai-service", {
    method: "PUT",
    body,
  });
}

export async function verifyAIServiceSettings(patch) {
  return accountJSON("/api/v1/settings/ai-service/verify", {
    method: "POST",
    body: patch,
  });
}

export async function clearAIServiceSettingsKey() {
  return accountJSON("/api/v1/settings/ai-service/clear-key", {
    method: "POST",
  });
}

export async function fetchEdgeNodes(options = {}) {
  try {
    const path = "/api/v1/edge/nodes";
    const response = await fetch(buildAPIURL(path), {
      ...(options.signal ? { signal: options.signal } : {}),
      cache: "no-store",
      headers: buildRequestHeaders(path),
    });
    if (!response.ok) {
      throw await buildAPIError(response);
    }
    const payload = await response.json();
    return {
      data: Array.isArray(payload?.nodes) ? payload.nodes : [],
      count: Number(payload?.count || 0),
      source: "api",
      error: null,
    };
  } catch (error) {
    return {
      data: [],
      count: 0,
      source: "unavailable",
      error: error instanceof Error ? error.message : "unknown error",
    };
  }
}

export async function fetchRuntimeStatus(options = {}) {
  try {
    const path = "/api/v1/runtime/status";
    const response = await fetch(buildAPIURL(path), {
      ...(options.signal ? { signal: options.signal } : {}),
      cache: "no-store",
      headers: buildRequestHeaders(path),
    });
    if (!response.ok) {
      throw await buildAPIError(response);
    }
    const payload = await response.json();
    return {
      data: payload,
      source: "api",
      error: null,
    };
  } catch (error) {

    return {
      data: null,
      source: "unavailable",
      error: error instanceof Error ? error.message : "unknown error",
    };
  }
}

export async function runAIAnalysis(module, payload = {}) {
  if (!["content-analysis", "topic-trends", "account-performance", "comment-insights"].includes(String(module).trim())) throw new Error("This AI module has been removed from operations analysis");
  const normalizedModule = String(module ?? "").trim();
  if (!normalizedModule) {
    throw new Error("AI 分析模块不能为空");
  }
  const values = payload && typeof payload === "object" ? payload : {};
  return accountJSON(`/api/v1/ai-analysis/${encodeURIComponent(normalizedModule)}`, {
    method: "POST",
    body: {
      scopeKey: String(values.scopeKey ?? "").trim(),
      window: String(values.window ?? "all").trim() || "all",
      variables: values.variables && typeof values.variables === "object" ? values.variables : {},
    },
  });
}

export async function fetchAIAnalysisRuns(module, options = {}) {
  if (!["content-analysis", "topic-trends", "account-performance", "comment-insights"].includes(String(module).trim())) throw new Error("This AI module has been removed from operations analysis");
  const normalizedModule = String(module ?? "").trim();
  if (!normalizedModule) {
    throw new Error("AI 分析模块不能为空");
  }
  const values = options && typeof options === "object" ? options : {};
  const rawLimit = Number(values.limit);
  const limit = Number.isFinite(rawLimit) && rawLimit > 0
    ? Math.min(20, Math.max(1, Math.floor(rawLimit)))
    : 0;
  return accountJSON(`/api/v1/ai-analysis/${encodeURIComponent(normalizedModule)}`, {
    query: {
      ...(String(values.scopeKey ?? "").trim() ? { scopeKey: String(values.scopeKey).trim() } : {}),
      ...(String(values.window ?? "").trim() ? { window: String(values.window).trim() } : {}),
      ...(limit ? { limit } : {}),
    },
  });
}

export async function fetchVideoPlayback(objectID, signal) {
  const normalized = String(objectID ?? "").trim();
  if (!normalized) throw new Error("视频标识不能为空");
  const path = `/api/v1/read-models/videos/${encodeURIComponent(normalized)}/playback`;
  const response = await fetch(buildAPIURL(path), { cache: "no-store", headers: buildRequestHeaders(path), signal });
  if (!response.ok) throw await buildAPIError(response);
  return response.json();
}

export async function fetchMonitoringHealth() {
  const path = "/api/v1/monitoring/health";
  const response = await fetch(buildAPIURL(path), { cache: "no-store", headers: buildRequestHeaders(path) });
  if (!response.ok) throw await buildAPIError(response);
  return response.json();
}

export async function fetchVideoDetail(objectID) {
  const normalized = String(objectID ?? "").trim();
  if (!normalized) {
    throw new Error("视频标识不能为空");
  }
  const path = `/api/v1/read-models/videos/${encodeURIComponent(normalized)}`;
  const response = await fetch(buildAPIURL(path), {
    cache: "no-store",
    headers: buildRequestHeaders(path),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

export async function fetchVideoComments(objectID, query = {}) {
  const normalized = String(objectID ?? "").trim();
  if (!normalized) {
    throw new Error("视频标识不能为空");
  }
  return accountJSON(`/api/v1/read-models/videos/${encodeURIComponent(normalized)}/comments`, { query });
}

export async function fetchDataCenter(entity, query = {}, options = {}) {
  const normalizedEntity = String(entity ?? "").trim();
  if (!normalizedEntity) {
    throw new Error("数据中心实体不能为空");
  }
  const path = buildAPIURLWithQuery(
    `/api/v1/read-models/data-center/${encodeURIComponent(normalizedEntity)}`,
    query,
  );
  const response = await fetch(path, {
    ...(options.signal ? { signal: options.signal } : {}),
    cache: "no-store",
    headers: buildRequestHeaders(path),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

export async function curateVideos(patch) {
	return accountJSON("/api/v1/read-models/data-center/videos/curation", {method:"POST",body:patch});
}

export async function fetchVideoCuration(videoID) {
  return accountJSON(`/api/v1/read-models/data-center/videos/${encodeURIComponent(videoID)}/curation`);
}

export async function fetchOperationsAgent() {
  return accountJSON('/api/v1/operations-agent');
}
export async function inspectOperationsAgent(scope = {}) {
  return accountJSON('/api/v1/operations-agent/inspect', {method:'POST',body:scope});
}
export async function decideOperationsAgent(action, ids) {
  if (!['apply','dismiss','undo','refresh'].includes(action)) throw new Error('不支持的智能运营操作');
  return accountJSON(`/api/v1/operations-agent/${action}`, {method:'POST',body:{ids}});
}

export async function fetchSavedSourceBindings(options = {}) {
  const path = "/api/v1/ingest/bindings";
  const response = await fetch(buildAPIURL(path), {
    ...(options.signal ? { signal: options.signal } : {}),
    cache: "no-store",
    headers: buildRequestHeaders(path),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  const payload = await response.json();
  return Array.isArray(payload?.items) ? payload.items : [];
}

export async function fetchDataCenterHistory(entity, entityID, query = {}) {
  const normalizedEntity = String(entity ?? "").trim();
  const normalizedID = String(entityID ?? "").trim();
  if (!normalizedEntity || !normalizedID) {
    throw new Error("数据中心历史记录参数不能为空");
  }
  const path = buildAPIURLWithQuery(
    `/api/v1/read-models/data-center/${encodeURIComponent(normalizedEntity)}/${encodeURIComponent(normalizedID)}/history`,
    query,
  );
  const response = await fetch(path, {
    cache: "no-store",
    headers: buildRequestHeaders(path),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

async function accountJSON(path, { method = "GET", body, query } = {}) {
  const resolvedPath = query ? buildAPIURLWithQuery(path, query) : buildAPIURL(path);
  const response = await fetch(resolvedPath, {
    cache: "no-store",
    method,
    headers: {
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...buildRequestHeaders(resolvedPath),
    },
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

export async function fetchAccounts(query = {}) {
  return accountJSON("/api/v1/accounts", { query });
}

export async function fetchAccount(accountID) {
  const normalizedID = String(accountID ?? "").trim();
  if (!normalizedID) {
    throw new Error("账户标识不能为空");
  }
  return accountJSON(`/api/v1/accounts/${encodeURIComponent(normalizedID)}`);
}

export async function updateAccountProfile(accountID, patch) {
  const normalizedID = String(accountID ?? "").trim();
  if (!normalizedID) {
    throw new Error("账户标识不能为空");
  }
  return accountJSON(`/api/v1/accounts/${encodeURIComponent(normalizedID)}`, {
    method: "PATCH",
    body: patch,
  });
}

export async function fetchAccountVideos(accountID, query = {}) {
  const normalizedID = String(accountID ?? "").trim();
  if (!normalizedID) {
    throw new Error("账户标识不能为空");
  }
  return accountJSON(`/api/v1/accounts/${encodeURIComponent(normalizedID)}/videos`, { query });
}

export async function fetchAccountComments(accountID, query = {}) {
  const normalizedID = String(accountID ?? "").trim();
  if (!normalizedID) {
    throw new Error("账户标识不能为空");
  }
  return accountJSON(`/api/v1/accounts/${encodeURIComponent(normalizedID)}/comments`, { query });
}

export async function startAccountMonitoring(accountID, options = {}) {
  const normalizedID = String(accountID ?? "").trim();
  if (!normalizedID) {
    throw new Error("账户标识不能为空");
  }
  return accountJSON(`/api/v1/accounts/${encodeURIComponent(normalizedID)}/monitoring`, {
    method: "POST",
    body: options,
  });
}

export async function backfillAccountComments(accountID) {
  const normalizedID = String(accountID ?? "").trim();
  if (!normalizedID) {
    throw new Error("账户标识不能为空");
  }
  return accountJSON(`/api/v1/accounts/${encodeURIComponent(normalizedID)}/comments/backfill`, {
    method: "POST",
  });
}

export async function refreshAccountProfile(accountID) {
  const normalizedID = String(accountID ?? "").trim();
  if (!normalizedID) {
    throw new Error("账户标识不能为空");
  }
  return accountJSON(`/api/v1/accounts/${encodeURIComponent(normalizedID)}/profile`, {
    method: "POST",
  });
}

export async function refreshVideoComments(objectID) {
  const normalizedID = String(objectID ?? "").trim();
  if (!normalizedID) {
    throw new Error("视频标识不能为空");
  }
  return accountJSON(`/api/v1/read-models/videos/${encodeURIComponent(normalizedID)}/comments/refresh`, {
    method: "POST",
  });
}

export async function refreshMonitoringMetrics(kind, id, force = false) {
  return accountJSON("/api/v1/monitoring/refresh-metrics", {
    method: "POST", body: { kind, id: String(id || "").trim(), force },
  });
}

export async function importWXCommentExport(path) {
  const apiPath = "/api/v1/ingest/wx-channel/comment-export/import";
  const response = await fetch(
    buildAPIURL(apiPath),
    {
      cache: "no-store",
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...buildRequestHeaders(apiPath),
      },
      body: JSON.stringify({ path }),
    },
  );
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

export async function searchWXFeedAccounts(baseURL, keyword, clientId = "") {
  const apiPath = buildAPIURLWithQuery(
    "/api/v1/ingest/wx-channel/feed-list/search",
    {
      base_url: baseURL,
      keyword,
      ...(clientId ? { client_id: clientId } : {}),
    },
  );
  const response = await fetch(apiPath, {
    cache: "no-store",
    headers: buildRequestHeaders(apiPath),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

export async function importWXVideoShare(baseURL, shareURL, clientId = "") {
  const apiPath = "/api/v1/ingest/wx-channel/video-share/import";
  const response = await fetch(buildAPIURL(apiPath), {
    cache: "no-store",
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...buildRequestHeaders(apiPath),
    },
    body: JSON.stringify({
      baseUrl: baseURL,
      shareUrl: shareURL,
      ...(clientId ? { clientId } : {}),
    }),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

export async function saveWXFeedListBinding(baseURL, username) {
  const apiPath = "/api/v1/ingest/wx-channel/feed-list/save";
  const response = await fetch(
    buildAPIURL(apiPath),
    {
      cache: "no-store",
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...buildRequestHeaders(apiPath),
      },
      body: JSON.stringify({ baseUrl: baseURL, username }),
    },
  );
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

export async function saveWXVideoSearchBinding(baseURL, keyword) {
  const apiPath = "/api/v1/ingest/wx-channel/video-search/save";
  const response = await fetch(buildAPIURL(apiPath), {
    cache: "no-store",
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...buildRequestHeaders(apiPath),
    },
    body: JSON.stringify({ baseUrl: baseURL, keyword }),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

async function monitoringJSON(path, { method = "GET", body } = {}) {
  const response = await fetch(buildAPIURL(path), {
    cache: "no-store",
    method,
    headers: {
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...buildRequestHeaders(path),
    },
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

export async function fetchMonitoringTasks(includeArchived = false) {
  const path = buildAPIURLWithQuery("/api/v1/monitoring/tasks", {
    ...(includeArchived ? { includeArchived: "true" } : {}),
  });
  const response = await fetch(path, {
    cache: "no-store",
    headers: buildRequestHeaders(path),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

export async function createMonitoringTask(task) {
  return monitoringJSON("/api/v1/monitoring/tasks", {
    method: "POST",
    body: task,
  });
}

export async function updateMonitoringTask(taskID, task) {
  return monitoringJSON(`/api/v1/monitoring/tasks/${encodeURIComponent(taskID)}`, {
    method: "PATCH",
    body: task,
  });
}

export async function archiveMonitoringTask(taskID) {
  return monitoringJSON(`/api/v1/monitoring/tasks/${encodeURIComponent(taskID)}`, {
    method: "DELETE",
  });
}

export async function fetchMonitoringTaskRuns(taskID, limit = 20) {
  const path = buildAPIURLWithQuery(
    `/api/v1/monitoring/tasks/${encodeURIComponent(taskID)}/runs`,
    limit > 0 ? { limit } : {},
  );
  const response = await fetch(path, {
    cache: "no-store",
    headers: buildRequestHeaders(path),
  });
  if (!response.ok) {
    throw await buildAPIError(response);
  }
  return response.json();
}

export async function runMonitoringTask(taskID) {
  return monitoringJSON(`/api/v1/monitoring/tasks/${encodeURIComponent(taskID)}/run`, {
    method: "POST",
  });
}

export async function retryMonitoringTask(taskID) {
  return monitoringJSON(`/api/v1/monitoring/tasks/${encodeURIComponent(taskID)}/retry`, {
    method: "POST",
  });
}

export { setLiveRequestContextPolicy };
