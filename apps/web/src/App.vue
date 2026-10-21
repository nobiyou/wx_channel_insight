<script setup>
import { computed, defineAsyncComponent, nextTick, onMounted, onUnmounted, ref } from "vue";
import { NAlert, NButton, NConfigProvider, NIcon, NSpin } from "naive-ui";
import { BarChart3, Bot, Hash, Home, RefreshCw, Settings, Users } from "lucide-vue-next";
import { fetchEdgeNodes, fetchRuntimeStatus, fetchSavedSourceBindings, fetchWorkspaceContext, setLiveRequestContextPolicy } from "./lib/api.js";
import { buildOperationsHash, OPERATIONS_NAVIGATION, parseOperationsRoute } from "./lib/operationsNavigation.js";
import { startAutoRefresh } from "./lib/autoRefresh.js";
import { formatInteger } from "./lib/numberFormatting.js";

const AccountMonitoringView = defineAsyncComponent(() => import("./components/AccountMonitoringView.vue"));
const KeywordMonitoringView = defineAsyncComponent(() => import("./components/KeywordMonitoringView.vue"));
const VideoAnalysisList = defineAsyncComponent(() => import("./components/VideoAnalysisList.vue"));
const OperationsOverview = defineAsyncComponent(() => import("./components/OperationsOverview.vue"));
const OperationsAgentView = defineAsyncComponent(() => import("./components/OperationsAgentView.vue"));
const SourceDiagnosticsView = defineAsyncComponent(() => import("./components/SourceDiagnosticsView.vue"));
const AccountDetailView = defineAsyncComponent(() => import("./components/AccountDetailView.vue"));
const VideoDetailView = defineAsyncComponent(() => import("./components/VideoDetailView.vue"));

setLiveRequestContextPolicy("trusted-bridge-only");
const route = ref(parseOperationsRoute(window.location.hash));
const visited = ref(new Set([route.value.view]));
const listParams = ref({ keywords: route.value.view === "keywords" ? route.value.params : {}, videos: route.value.view === "videos" ? route.value.params : {}, settings: route.value.view === "settings" ? route.value.params : {} });
const workspace = ref(null); const ready = ref(false); const loading = ref(true); const contextError = ref(""); const sourcesError = ref("");
const bindings = ref([]); const edgeNodes = ref([]); const runtime = ref(null); const baseUrl = ref(""); const clientId = ref("");
const accountKey = ref(route.value.view === "account" ? route.value.id : "");
const accountParams = ref(route.value.view === "account" ? route.value.params : {});
const accountReturn = ref(route.value.view === "account" ? route.value.returnHash : "#accounts");
const overview = ref(null); const accounts = ref(null); const keywords = ref(null); const videos = ref(null); const settings = ref(null); const accountDetail = ref(null); const videoDetail = ref(null);
const refreshing = ref(false);
const agent = ref(null);
let sourcesPromise = null;
let autoRefresh;
const icons = { dashboard: Home, accounts: Users, keywords: Hash, videos: BarChart3, agent: Bot, settings: Settings };
const themeOverrides = { common: { primaryColor: '#2f6fec', primaryColorHover: '#225bd0', primaryColorPressed: '#1c4fb8', primaryColorSuppl: '#2f6fec', borderRadius: '8px' } };
const activeKey = computed(() => route.value.view === "account" ? "accounts" : route.value.view === "video" ? "videos" : route.value.view);
const title = computed(() => OPERATIONS_NAVIGATION.find((view) => view.key === activeKey.value)?.label || "运营概览");
const collectorURL = computed(() => baseUrl.value.trim() || (edgeNodes.value.length ? "" : "http://127.0.0.1:2026"));
const formatNumber = (value) => formatInteger(Number(value) || 0);
const formatDateTime = (value) => value && Number.isFinite(new Date(value).getTime()) ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "不可用";

async function syncRoute() {
  const previousView = route.value.view;
  route.value = parseOperationsRoute(window.location.hash);
  const wasVisited = visited.value.has(route.value.view);
  visited.value = new Set([...visited.value, route.value.view]);
  if (["keywords", "videos", "settings"].includes(route.value.view)) listParams.value = { ...listParams.value, [route.value.view]: route.value.params };
  if (route.value.view === "account") { accountKey.value = route.value.id; accountParams.value = route.value.params; accountReturn.value = route.value.returnHash; }
  await nextTick();
  if (ready.value && wasVisited && previousView !== route.value.view) await refreshPage();
}
function navigate(view, params) {
  const current = parseOperationsRoute(window.location.hash);
  if (["keywords", "videos", "settings"].includes(current.view)) listParams.value = { ...listParams.value, [current.view]: current.params };
  window.location.hash = buildOperationsHash(view, params || listParams.value[view] || {});
}
function returnFromVideo() { window.location.hash = route.value.returnHash; }
function returnFromAccount() { window.location.hash = accountReturn.value; }
function openVideo(payload) {
  const video = payload?.video || payload;
  const id = String(video?.objectId || "").trim();
  if (!id) return;
  const returnHash = payload?.returnHash || window.location.hash || "#videos";
  window.location.hash = buildOperationsHash("video", { id, return: returnHash });
}
function openAccount(item) {
  const id = String(item?.accountId || item?.channelUsername || item?.username || "").trim();
  if (id) window.location.hash = buildOperationsHash("account", { id, return: window.location.hash || "#accounts" });
}
async function refreshSources() {
  if (!sourcesPromise) sourcesPromise = loadSources().finally(() => { sourcesPromise = null; });
  return sourcesPromise;
}
async function loadSources() {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), 7000);
  try {
    const options = { signal: controller.signal };
    const results = await Promise.allSettled([
      fetchSavedSourceBindings(options).then(value => { bindings.value = value; return value; }),
      fetchEdgeNodes(options).then(value => { if (value.source === "api") edgeNodes.value = value.data || []; return value; }),
      fetchRuntimeStatus(options).then(value => { if (value.source === "api") runtime.value = value.data; return value; }),
    ]);
    const failures = [];
    if (results[0].status !== "fulfilled") failures.push("来源绑定");
    if (results[1].status !== "fulfilled" || results[1].value.source !== "api") failures.push("边缘连接");
    if (results[2].status !== "fulfilled" || results[2].value.source !== "api") failures.push("运行状态");
    sourcesError.value = failures.length ? `${failures.join("、")}读取${controller.signal.aborted ? "超时" : "失败"}，请稍后更新数据` : "";
  } finally { window.clearTimeout(timer); }
}
async function bootstrap() {
  loading.value = true; contextError.value = "";
  const result = await fetchWorkspaceContext();
  if (result.source !== "api") { contextError.value = `工作区尚未由服务端确认：${result.error || "API 不可用"}`; ready.value = false; loading.value = false; return; }
  workspace.value = result.data; ready.value = true; loading.value = false;
  refreshSources();
}
async function refreshActive() {
  if (refreshing.value || loading.value) return;
  refreshing.value = true;
  try {
  if (!ready.value) return await bootstrap();
  await Promise.allSettled([refreshSources(), refreshPage()]);
  } finally { refreshing.value = false; }
}
async function refreshPage(options) {
  const component = { dashboard: overview, accounts, keywords, videos, agent, settings, account: accountDetail, video: videoDetail }[route.value.view];
  await component?.value?.refresh?.(options);
}
onMounted(() => {
  window.addEventListener("hashchange", syncRoute); bootstrap();
  autoRefresh = startAutoRefresh(async () => {
    if (!ready.value || refreshing.value || loading.value || document.activeElement?.matches("input, textarea, select, [contenteditable='true']") || !["dashboard", "accounts", "keywords", "videos", "agent", "account", "video"].includes(route.value.view)) return;
    await refreshPage({ background: true });
  });
});
onUnmounted(() => { window.removeEventListener("hashchange", syncRoute); autoRefresh?.stop(); });
</script>

<template>
  <n-config-provider :theme-overrides="themeOverrides">
    <div class="shell operations-shell">
      <aside class="sidebar">
        <div class="brand-block"><div class="brand-mark" aria-hidden="true">IN</div><div><div class="brand-name">Insight</div><div class="brand-subtitle">运营数据分析</div></div></div>
        <nav class="nav-list" aria-label="主导航"><button v-for="view in OPERATIONS_NAVIGATION" :key="view.key" class="nav-item" :class="{ active: activeKey === view.key }" type="button" :aria-label="view.label" :title="view.label" :aria-current="activeKey === view.key ? 'page' : undefined" @click="navigate(view.key)"><span class="nav-icon-shell" aria-hidden="true"><n-icon size="17"><component :is="icons[view.key]" /></n-icon></span><span class="nav-item-label">{{ view.label }}</span></button></nav>
        <div class="sidebar-foot"><div class="workspace-status-line"><span class="status-dot" :data-tone="ready ? 'good' : 'warn'"></span><span>{{ ready ? '工作区已连接' : '等待连接' }}</span></div><div class="status-title">{{ workspace?.workspaceName || '默认运营工作区' }}</div><div class="status-meta">监控对象 → 采集历史 → 数据分析</div></div>
      </aside>
      <main class="app-content">
        <header class="topbar"><div><div class="page-kicker">运营数据分析</div><h1 class="page-title">{{ title }}</h1><p class="page-summary">账号监控、关键词热门视频监控与视频数据分析</p></div><div class="topbar-meta"><div class="workspace-context-label"><span>服务端工作区</span><strong>{{ workspace?.workspaceName || '等待确认' }}</strong></div><n-button size="small" secondary :loading="loading || refreshing" @click="refreshActive"><template #icon><n-icon><RefreshCw /></n-icon></template>更新数据</n-button></div></header>
        <div v-if="loading" class="loading-state"><n-spin size="medium" /><span>正在确认工作区与数据连接…</span></div>
        <n-alert v-else-if="contextError" type="error" show-icon>{{ contextError }}。连接恢复后点击“更新数据”。</n-alert>
        <template v-else-if="ready">
          <n-alert v-if="sourcesError" type="warning" show-icon>{{ sourcesError }}。已保存的分析数据仍可查看。</n-alert>
          <OperationsOverview v-if="visited.has('dashboard')" v-show="route.view === 'dashboard'" ref="overview" @navigate="navigate" @open-video="openVideo" />
          <OperationsAgentView v-if="visited.has('agent')" v-show="route.view === 'agent'" ref="agent" @navigate="navigate" @open-video="openVideo" />
          <AccountMonitoringView v-if="visited.has('accounts')" v-show="route.view === 'accounts'" ref="accounts" :binding-states="bindings" :base-url="collectorURL" :client-id="clientId" @open-detail="openAccount" @analyze="navigate('videos', $event)" @refresh="refreshSources" />
          <KeywordMonitoringView v-if="visited.has('keywords')" v-show="route.view === 'keywords'" ref="keywords" :binding-states="bindings" :initial-params="listParams.keywords" :base-url="collectorURL" :client-id="clientId" @open-video="openVideo" @open-account="openAccount" @analyze="navigate('videos', $event)" @refresh="refreshSources" />
          <VideoAnalysisList v-if="visited.has('videos')" v-show="route.view === 'videos'" ref="videos" :initial-params="listParams.videos" :base-url="collectorURL" :client-id="clientId" @open-video="openVideo" @open-account="openAccount" />
          <AccountDetailView v-if="accountKey" v-show="route.view === 'account'" ref="accountDetail" :account-key="accountKey" :initial-params="accountParams" :binding-states="bindings" :format-number="formatNumber" :format-date-time="formatDateTime" @back="returnFromAccount" @open-video="openVideo" @refresh="refreshSources" />
          <VideoDetailView v-if="route.view === 'video'" ref="videoDetail" :object-id="route.id" @back="returnFromVideo" />
          <SourceDiagnosticsView v-if="visited.has('settings')" v-show="route.view === 'settings'" ref="settings" :initial-params="listParams.settings" :binding-states="bindings" :runtime="runtime" :edge-nodes="edgeNodes" v-model:base-url="baseUrl" v-model:client-id="clientId" @refresh="refreshSources" />
        </template>
      </main>
    </div>
  </n-config-provider>
</template>
