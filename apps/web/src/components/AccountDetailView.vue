<script setup>
import { computed, onMounted, ref, watch } from "vue";
import {
  ArrowDown,
  ArrowUp,
  ArrowUpRight,
  Bookmark,
  CalendarDays,
  Check,
  ChevronLeft,
  ChevronRight,
  Database,
  Heart,
  History,
  MessageCircle,
  Radio,
  RefreshCw,
  Save,
  Share2,
  Star,
  UserRound,
} from "lucide-vue-next";
import { NAlert, NEmpty, NSpin } from "naive-ui";
import MonitoringTasksPanel from "./MonitoringTasksPanel.vue";
import VideoAnalysisList from "./VideoAnalysisList.vue";
import { monitoringTaskStatusLabel } from "../lib/monitoringTasks.js";
import {
  backfillAccountComments,
  fetchAccount,
  fetchAccountComments,
  fetchAccounts,
  fetchAccountVideos,
  fetchDataCenterHistory,
  refreshAccountProfile,
  refreshMonitoringMetrics,
  startAccountMonitoring,
  updateAccountProfile,
} from "../lib/api.js";
import { metricsAreStale } from "../lib/autoRefresh.js";

const props = defineProps({
  accountKey: { type: String, default: "" },
  initialParams: { type: Object, default: () => ({}) },
  bindingStates: { type: Array, default: () => [] },
  initialAccount: { type: Object, default: null },
  formatNumber: { type: Function, required: true },
  formatDateTime: { type: Function, required: true },
});

const emit = defineEmits(["back", "open-video", "refresh"]);
const taskPanel = ref(null);
const workbench = ref(null);
const workbenchAccount = computed(() => account.value?.channelUsername || account.value?.channelName || props.accountKey);
const workbenchParams = computed(() => ({ ...props.initialParams, account: workbenchAccount.value }));
const workbenchRoute = computed(() => ({ id: props.accountKey, return: props.initialParams.return }));
function openWorkbenchVideo(payload) { emit("open-video", { ...payload, returnHash: payload.returnHash || window.location.hash }); }
const taskSection = ref(null);

const account = ref(props.initialAccount || null);
const accountID = ref("");
const monitoring = ref({ enabled: false, taskCount: 0, tasks: [] });
const videos = ref([]);
const comments = ref([]);
const history = ref([]);
const activeTab = ref("analysis");
const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const totalPages = ref(1);
const commentPage = ref(1);
const commentPageSize = ref(20);
const commentTotal = ref(0);
const commentTotalPages = ref(1);
const commentKeyword = ref("");
const historyPage = ref(1);
const historyPageSize = ref(20);
const historyTotal = ref(0);
const historyTotalPages = ref(1);
const sort = ref("published_at");
const direction = ref("desc");
const loading = ref(false);
const liveCollecting = ref(false);
const liveNotice = ref("");
const tabLoading = ref(false);
const actionID = ref("");
const error = ref("");
const notice = ref("");
const dataNotice = ref("");
const editing = ref(false);
const draft = ref({ region: "", industry: "", tags: "", note: "", isPriority: false });
const requestSerial = ref(0);
const videosRequestSerial = ref(0);
const commentsRequestSerial = ref(0);
const historyRequestSerial = ref(0);

const sortOptions = [
  { value: "observed_at", label: "最近采集" },
  { value: "published_at", label: "发布时间" },
  { value: "likes", label: "点赞" },
  { value: "comments", label: "评论" },
  { value: "favorites", label: "收藏" },
  { value: "forwards", label: "转发" },
];

const accountName = computed(() => account.value?.channelName || account.value?.channelUsername || "未命名账号");
const accountUsername = computed(() => account.value?.channelUsername || props.accountKey || "账户标识待确认");
const accountMetrics = computed(() => [
  ["视频数", metricValue("videoCount")],
  ["点赞", metricValue("likes")],
  ["评论", metricValue("comments")],
  ["收藏", metricValue("favorites")],
  ["转发", metricValue("forwards")],
  ["最近采集", account.value?.metrics?.recentObservedAt || account.value?.profileObservedAt || ""],
]);
const pageStart = computed(() => (total.value ? (page.value - 1) * pageSize.value + 1 : 0));
const pageEnd = computed(() => Math.min(total.value, page.value * pageSize.value));
const commentPageStart = computed(() => (commentTotal.value ? (commentPage.value - 1) * commentPageSize.value + 1 : 0));
const commentPageEnd = computed(() => Math.min(commentTotal.value, commentPage.value * commentPageSize.value));
const historyPageStart = computed(() => (historyTotal.value ? (historyPage.value - 1) * historyPageSize.value + 1 : 0));
const historyPageEnd = computed(() => Math.min(historyTotal.value, historyPage.value * historyPageSize.value));
const profileSourceLabel = computed(() => ({ upstream: "上游画像", manual: "人工维护", mixed: "混合来源" }[account.value?.profileSource] || "来源待确认"));
const monitoringLabel = computed(() => monitoring.value.enabled ? `监控中 · ${monitoring.value.taskCount || 0} 个任务` : monitoring.value.taskCount > 0 ? "已暂停" : "未加入监控");

function metricValue(key) {
  return account.value?.metrics?.[key] ?? null;
}

function formatMetric(value) {
  if (value === null || value === undefined || value === "") return "—";
  if (typeof value === "string" && value.includes("T")) return props.formatDateTime(value);
  return props.formatNumber(value);
}

function videoTime(video) {
  return video?.publishedAt ? props.formatDateTime(video.publishedAt) : "未设置";
}

function videoSubtitle(video) {
  return [video?.channelUsername || video?.channelName, video?.source].filter(Boolean).join(" · ") || "来源待确认";
}

function commentContent(comment) {
  return comment?.content || "未提供评论正文";
}

function clearFeedback() {
  error.value = "";
  notice.value = "";
}

function resetDraft() {
  draft.value = {
    region: account.value?.region || "",
    industry: account.value?.industry || "",
    tags: Array.isArray(account.value?.tags) ? account.value.tags.join(", ") : "",
    note: account.value?.note || "",
    isPriority: Boolean(account.value?.isPriority),
  };
}

function applyAccountPayload(payload) {
  const nextAccount = payload?.account || payload;
  if (nextAccount && typeof nextAccount === "object") {
    account.value = nextAccount;
    accountID.value = String(nextAccount.accountId || props.accountKey || "").trim();
  }
  if (payload?.monitoring) monitoring.value = payload.monitoring;
  if (payload?.dataNotice) dataNotice.value = payload.dataNotice;
  resetDraft();
}

async function resolveAccountPayload() {
  const key = String(props.accountKey || "").trim();
  if (!key) throw new Error("缺少账户标识，暂时无法读取账户详情");
  try {
    return await fetchAccount(key);
  } catch (directError) {
    const list = await fetchAccounts({ q: key, page: 1, pageSize: 20 });
    const candidate = (list?.items || []).find((item) =>
      [item?.accountId, item?.channelUsername, item?.channelName].some((value) => String(value || "").toLowerCase() === key.toLowerCase()),
    ) || list?.items?.[0];
    if (!candidate?.accountId) throw directError;
    return fetchAccount(candidate.accountId);
  }
}

async function load({ background = false, collect = true } = {}) {
  if (background && (loading.value || tabLoading.value || editing.value || actionID.value)) return;
  const serial = requestSerial.value + 1;
  requestSerial.value = serial;
  if (!background) { loading.value = true; clearFeedback(); }
  try {
    const payload = await resolveAccountPayload();
    if (serial !== requestSerial.value) return;
    applyAccountPayload(payload);
    if (!accountID.value) throw new Error("账户响应缺少稳定 accountId");
    await loadVideos();
    if (activeTab.value === "comments") await loadComments();
    if (activeTab.value === "history") await loadHistory();
    if (activeTab.value === "analysis") await workbench.value?.refresh({ background });
    if (collect && metricsAreStale(account.value?.metrics?.recentObservedAt)) void collectLiveMetrics();
  } catch (loadError) {
    if (serial !== requestSerial.value) return;
    error.value = loadError instanceof Error ? loadError.message : "账户详情加载失败";
  } finally {
    if (serial === requestSerial.value) loading.value = false;
  }
}

async function collectLiveMetrics(force = false) {
  if (liveCollecting.value || !accountID.value) return;
  const id = accountID.value;
  liveCollecting.value = true;
  liveNotice.value = "正在采集账号最新视频与互动指标，当前数据可继续查看…";
  try {
    const result = await refreshMonitoringMetrics("account", id, force);
    if (accountID.value !== id) return;
    liveNotice.value = result.message || "指标仍在 10 分钟有效期内";
    if (result.status === "updated") await load({ background: true, collect: false });
  } catch (cause) {
    if (accountID.value === id) liveNotice.value = `实时采集失败，保留上次有效数据：${cause instanceof Error ? cause.message : cause}`;
  } finally { liveCollecting.value = false; }
}

async function loadVideos() {
  if (!accountID.value) return;
  const serial = videosRequestSerial.value + 1;
  videosRequestSerial.value = serial;
  tabLoading.value = true;
  try {
    const payload = await fetchAccountVideos(accountID.value, { page: page.value, pageSize: pageSize.value, sort: sort.value, direction: direction.value });
    if (serial !== videosRequestSerial.value) return;
    videos.value = Array.isArray(payload?.items) ? payload.items : [];
    page.value = Number(payload?.page) || page.value;
    pageSize.value = Number(payload?.pageSize) || pageSize.value;
    total.value = Number(payload?.total) || 0;
    totalPages.value = Number(payload?.totalPages) || 1;
  } catch (loadError) {
    if (serial !== videosRequestSerial.value) return;
    error.value = loadError instanceof Error ? loadError.message : "账户视频加载失败";
  } finally {
    if (serial === videosRequestSerial.value && activeTab.value === "videos") tabLoading.value = false;
  }
}

async function loadComments() {
  if (!accountID.value) return;
  const serial = commentsRequestSerial.value + 1;
  commentsRequestSerial.value = serial;
  tabLoading.value = true;
  try {
    const payload = await fetchAccountComments(accountID.value, { page: commentPage.value, pageSize: commentPageSize.value, q: commentKeyword.value.trim() });
    if (serial !== commentsRequestSerial.value) return;
    comments.value = Array.isArray(payload?.items) ? payload.items : [];
    commentPage.value = Number(payload?.page) || commentPage.value;
    commentPageSize.value = Number(payload?.pageSize) || commentPageSize.value;
    commentTotal.value = Number(payload?.total) || 0;
    commentTotalPages.value = Number(payload?.totalPages) || 1;
  } catch (loadError) {
    if (serial !== commentsRequestSerial.value) return;
    error.value = loadError instanceof Error ? loadError.message : "账户评论加载失败";
    comments.value = [];
  } finally {
    if (serial === commentsRequestSerial.value && activeTab.value === "comments") tabLoading.value = false;
  }
}

async function loadHistory() {
  if (!accountID.value) return;
  const serial = historyRequestSerial.value + 1;
  historyRequestSerial.value = serial;
  tabLoading.value = true;
  try {
    const payload = await fetchDataCenterHistory("accounts", accountID.value, { page: historyPage.value, pageSize: historyPageSize.value });
    if (serial !== historyRequestSerial.value) return;
    history.value = Array.isArray(payload?.items) ? payload.items : [];
    historyPage.value = Number(payload?.page) || historyPage.value;
    historyPageSize.value = Number(payload?.pageSize) || historyPageSize.value;
    historyTotal.value = Number(payload?.total) || 0;
    historyTotalPages.value = Number(payload?.totalPages) || 1;
  } catch (loadError) {
    if (serial !== historyRequestSerial.value) return;
    error.value = loadError instanceof Error ? loadError.message : "账户历史加载失败";
    history.value = [];
    historyTotal.value = 0;
    historyTotalPages.value = 1;
  } finally {
    if (serial === historyRequestSerial.value && activeTab.value === "history") tabLoading.value = false;
  }
}

function changePage(nextPage) {
  if (tabLoading.value || nextPage < 1 || nextPage > totalPages.value) return;
  page.value = nextPage;
  loadVideos();
}

function changeCommentPage(nextPage) {
  if (tabLoading.value || nextPage < 1 || nextPage > commentTotalPages.value) return;
  commentPage.value = nextPage;
  loadComments();
}

function changeSort(event) {
  sort.value = event.target.value;
  page.value = 1;
  loadVideos();
}

function toggleDirection() {
  direction.value = direction.value === "asc" ? "desc" : "asc";
  page.value = 1;
  loadVideos();
}

function changePageSize(event) {
  pageSize.value = Number(event.target.value) === 50 ? 50 : 20;
  page.value = 1;
  loadVideos();
}

function changeCommentPageSize(event) {
  commentPageSize.value = Number(event.target.value) === 50 ? 50 : 20;
  commentPage.value = 1;
  loadComments();
}

function changeHistoryPage(nextPage) {
  if (tabLoading.value || nextPage < 1 || nextPage > historyTotalPages.value) return;
  historyPage.value = nextPage;
  loadHistory();
}

function changeHistoryPageSize(event) {
  historyPageSize.value = Number(event.target.value) === 50 ? 50 : 20;
  historyPage.value = 1;
  loadHistory();
}

function submitCommentSearch() {
  commentPage.value = 1;
  loadComments();
}

function selectTab(tab) {
  activeTab.value = tab;
  if (tab === "comments") loadComments();
  if (tab === "history") loadHistory();
}

function startEditing() {
  resetDraft();
  editing.value = true;
}

function cancelEditing() {
  editing.value = false;
  resetDraft();
}

async function saveProfile() {
  if (!accountID.value) return;
  actionID.value = "profile";
  clearFeedback();
  try {
    const payload = await updateAccountProfile(accountID.value, {
      region: draft.value.region.trim(),
      industry: draft.value.industry.trim(),
      tags: draft.value.tags.split(",").map((tag) => tag.trim()).filter(Boolean),
      note: draft.value.note.trim(),
      isPriority: Boolean(draft.value.isPriority),
    });
    applyAccountPayload(payload);
    editing.value = false;
    notice.value = "账户画像已保存，上游刷新不会覆盖人工字段。";
  } catch (saveError) {
    error.value = saveError instanceof Error ? saveError.message : "账户画像保存失败";
  } finally {
    actionID.value = "";
  }
}

async function togglePriority() {
  if (!account.value) return;
  actionID.value = "priority";
  clearFeedback();
  try {
    const payload = await updateAccountProfile(accountID.value, { isPriority: !account.value.isPriority });
    applyAccountPayload(payload);
    notice.value = account.value.isPriority ? "已加入重点账号" : "已取消重点账号标记";
  } catch (actionError) {
    error.value = actionError instanceof Error ? actionError.message : "重点标记更新失败";
  } finally {
    actionID.value = "";
  }
}

async function refreshProfile() {
  if (!accountID.value) return;
  actionID.value = "profile-refresh";
  clearFeedback();
  try {
    applyAccountPayload(await refreshAccountProfile(accountID.value));
    notice.value = "账号资料已从上游刷新，人工维护字段保持不变。";
  } catch (actionError) {
    error.value = actionError instanceof Error ? actionError.message : "账号资料刷新失败";
  } finally {
    actionID.value = "";
  }
}

async function monitorAccount() {
  if (!accountID.value || actionID.value) return;
  if (monitoring.value.taskCount > 0) { taskSection.value?.scrollIntoView({ behavior: "smooth", block: "start" }); return; }
  actionID.value = "monitor";
  clearFeedback();
  try {
    const payload = await startAccountMonitoring(accountID.value, { taskType: "video_list", commentMode: "incremental" });
    if (payload?.task) monitoring.value = { ...monitoring.value, enabled: true, taskCount: (monitoring.value.taskCount || 0) + 1, tasks: [...(monitoring.value.tasks || []), payload.task] };
    notice.value = "视频列表同步任务已创建，可在下方采集设置查看并运行。";
    await taskPanel.value?.refresh();
    emit("refresh");
  } catch (actionError) {
    error.value = actionError instanceof Error ? actionError.message : "账号监控创建失败";
  } finally {
    actionID.value = "";
  }
}

async function monitoringChanged() { await load(); emit("refresh"); }

async function backfillComments() {
  if (!accountID.value || !window.confirm("确认启动评论回采吗？系统每轮最多处理 3 个视频，剩余视频可通过后续运行继续采集。")) return;
  actionID.value = "backfill";
  clearFeedback();
  try {
    const payload = await backfillAccountComments(accountID.value);
    notice.value = `本轮评论回采完成：成功 ${payload?.succeeded ?? 0} 个视频，失败 ${payload?.failed ?? 0} 个，延后 ${payload?.skipped ?? 0} 项。`;
    if (activeTab.value === "comments") await loadComments();
  } catch (actionError) {
    error.value = actionError instanceof Error ? actionError.message : "全量评论回采失败";
  } finally {
    actionID.value = "";
  }
}

watch(
  () => [props.accountKey, props.initialAccount],
  () => {
    account.value = props.initialAccount || null;
    liveNotice.value = "";
    accountID.value = "";
    page.value = 1;
    commentPage.value = 1;
    historyPage.value = 1;
    historyTotal.value = 0;
    historyTotalPages.value = 1;
    videosRequestSerial.value += 1;
    commentsRequestSerial.value += 1;
    historyRequestSerial.value += 1;
    load();
  },
  { deep: true },
);

onMounted(load);

defineExpose({ refresh: load });
</script>

<template>
  <section class="account-detail-page" aria-labelledby="account-detail-title">
    <div class="account-detail-back-row">
      <button class="account-detail-back-button" type="button" @click="emit('back')"><ChevronLeft :size="16" aria-hidden="true" />返回账户列表</button>
      <span class="account-detail-route-label">账户详情 / 主数据与采集证据</span>
    </div>

    <div class="account-detail-heading">
      <div class="account-detail-heading-main">
        <div class="account-detail-avatar"><img v-if="account?.avatarUrl" :src="account.avatarUrl" :alt="`${accountName}头像`" /><UserRound v-else :size="22" aria-hidden="true" /></div>
        <div><div class="analysis-page-kicker">账户主数据</div><h2 id="account-detail-title">{{ accountName }}</h2><p>{{ accountUsername }} · {{ profileSourceLabel }}</p></div>
      </div>
      <div class="account-detail-heading-actions"><span class="account-library-status" :data-tone="monitoring.enabled ? 'active' : 'idle'"><Radio :size="14" aria-hidden="true" />{{ monitoringLabel }}</span><button class="account-library-flag" :class="{ active: account?.isPriority }" type="button" :disabled="actionID" :title="account?.isPriority ? '取消重点账号' : '设为重点账号'" :aria-label="account?.isPriority ? '取消重点账号' : '设为重点账号'" @click="togglePriority"><Star :size="16" :fill="account?.isPriority ? 'currentColor' : 'none'" aria-hidden="true" /></button><button class="account-detail-refresh" type="button" title="刷新账户详情" aria-label="刷新账户详情" :disabled="loading || actionID" @click="load"><RefreshCw :size="15" aria-hidden="true" /></button></div>
    </div>

    <div class="account-detail-profile-actions"><span class="analysis-muted-cell">指标超过 10 分钟时自动采集 · 评论任务至少间隔 30 分钟</span><button class="tool-button secondary" type="button" :disabled="liveCollecting || !accountID" @click="collectLiveMetrics(true)"><RefreshCw :size="14" />{{ liveCollecting ? '正在采集…' : '立即采集指标' }}</button></div>
    <n-alert v-if="liveNotice" type="info" :show-icon>{{ liveNotice }}</n-alert>
    <n-alert v-if="dataNotice" type="warning" :show-icon>{{ dataNotice }}
    </n-alert>
    <n-alert v-if="notice" type="success" :show-icon closable @close="notice = ''">{{ notice }}
    </n-alert>
    <n-alert v-if="error" type="error" :show-icon class="account-detail-error"><div class="account-detail-error-content"><span>{{ error }}</span><button class="account-detail-retry" type="button" @click="load"><RefreshCw :size="14" aria-hidden="true" />重试</button></div></n-alert>

    <section class="analysis-surface account-detail-profile-surface" aria-labelledby="account-profile-title">
      <div class="analysis-surface-head"><div><div class="analysis-section-label">账户画像</div><h3 id="account-profile-title">身份与人工维护字段</h3></div><div class="account-detail-profile-actions"><span class="account-detail-source-label">画像证据 {{ formatDateTime(account?.profileObservedAt) }}</span><button v-if="!editing" class="tool-button secondary" type="button" :disabled="actionID" @click="refreshProfile"><RefreshCw :size="14" aria-hidden="true" />刷新资料</button><button v-if="!editing" class="tool-button secondary" type="button" @click="startEditing"><UserRound :size="14" aria-hidden="true" />编辑画像</button><template v-else><button class="tool-button secondary" type="button" @click="cancelEditing">取消</button><button class="tool-button primary" type="button" :disabled="actionID === 'profile'" @click="saveProfile"><Save :size="14" aria-hidden="true" />保存</button></template></div></div>
      <div v-if="!editing" class="account-detail-profile-grid"><div><span>账号 ID</span><strong>{{ account?.accountId || "—" }}</strong></div><div><span>昵称</span><strong>{{ account?.channelName || "未设置" }}</strong></div><div><span>地区</span><strong>{{ account?.region || "未设置" }}</strong></div><div><span>行业</span><strong>{{ account?.industry || "未设置" }}</strong></div><div class="account-detail-profile-wide"><span>简介</span><p :title="account?.signature || '未设置'">{{ account?.signature || "未设置简介" }}</p></div><div><span>标签</span><p>{{ account?.tags?.length ? account.tags.join("、") : "未设置" }}</p></div><div class="account-detail-profile-wide"><span>备注</span><p :title="account?.note || '未设置'">{{ account?.note || "未设置" }}</p></div></div>
      <form v-else class="account-detail-profile-grid account-detail-profile-edit" @submit.prevent="saveProfile"><label><span>地区</span><input v-model="draft.region" type="text" placeholder="未设置" /></label><label><span>行业</span><input v-model="draft.industry" type="text" placeholder="未设置" /></label><label class="account-detail-profile-wide"><span>标签</span><input v-model="draft.tags" type="text" placeholder="多个标签用逗号分隔" /></label><label class="account-detail-profile-wide"><span>备注</span><textarea v-model="draft.note" rows="3" placeholder="记录账号维护备注"></textarea></label><label class="account-detail-priority-input"><input v-model="draft.isPriority" type="checkbox" /><span>设为重点账号</span></label></form>
    </section>

    <div class="analysis-stat-strip account-detail-stat-strip"><div v-for="metric in accountMetrics" :key="metric[0]"><span>{{ metric[0] }}</span><strong>{{ formatMetric(metric[1]) }}</strong></div></div>

    <section class="analysis-surface account-detail-monitor-surface" aria-labelledby="account-monitor-title"><div class="account-detail-monitor-main"><div><div class="analysis-section-label">采集关系</div><h3 id="account-monitor-title">账号监控</h3><p>重点标记只代表业务关注；监控状态由真实任务目标派生。</p></div><div class="account-detail-monitor-actions"><button class="tool-button primary" type="button" :disabled="Boolean(actionID)" @click="monitorAccount"><Radio :size="14" aria-hidden="true" />{{ monitoring.taskCount ? "管理监控" : "加入监控" }}</button><button class="tool-button secondary" type="button" :disabled="Boolean(actionID) || !account?.channelUsername" @click="backfillComments"><RefreshCw :size="14" aria-hidden="true" />全量回采评论</button></div></div><div v-if="monitoring.tasks?.length" class="account-detail-task-summary"><div v-for="task in monitoring.tasks" :key="task.taskId || task.id" class="account-detail-task-row"><strong>{{ task.taskName || task.name || "账号监控任务" }}</strong><span>{{ monitoringTaskStatusLabel(task.status) }} · 最近运行 {{ formatDateTime(task.lastRunAt) }}</span><span v-if="task.lastError" class="account-detail-task-error">{{ task.lastError }}</span></div></div><div v-else class="account-detail-monitor-empty">当前账号还没有关联监控任务。</div></section>

    <div class="analysis-toolbar account-detail-tabs" role="tablist" aria-label="账户详情内容"><div class="analysis-tabs"><button v-for="tab in [{ key: 'analysis', label: '运营分析与筛选' }, { key: 'videos', label: '视频列表' }, { key: 'comments', label: '评论库' }, { key: 'history', label: '历史证据' }]" :key="tab.key" class="analysis-tab" :class="{ active: activeTab === tab.key }" type="button" role="tab" :aria-selected="activeTab === tab.key" @click="selectTab(tab.key)">{{ tab.label }}</button></div><span class="account-detail-tab-note">所有列表均为服务端分页，评论正文保留原始采集证据。</span></div>

    <VideoAnalysisList v-if="activeTab === 'analysis'" ref="workbench" mode="account" route-view="account" :route-params="workbenchRoute" :scope-account="workbenchAccount" :initial-params="workbenchParams" @open-video="openWorkbenchVideo" />
    <section v-else-if="activeTab === 'videos'" class="analysis-surface account-detail-video-surface" aria-labelledby="account-videos-title"><div class="analysis-surface-head account-detail-list-head"><div><div class="analysis-section-label">关联内容</div><h3 id="account-videos-title">账户视频列表</h3><p>展示该账户已采集的视频，不受榜单 Top N 限制；上游分页未完成时仍可能存在未采集视频。</p></div><div class="account-detail-list-meta"><span>{{ tabLoading ? "读取中…" : `${formatNumber(total)} 条视频` }}</span><button class="account-detail-refresh" type="button" title="刷新账户视频" aria-label="刷新账户视频" :disabled="tabLoading" @click="loadVideos"><RefreshCw :size="15" aria-hidden="true" /></button></div></div><div class="account-detail-list-toolbar"><span class="account-detail-list-note">点击视频标题查看视频详情和互动历史。</span><div class="account-detail-list-controls"><label class="analysis-limit-select"><span>排序</span><select :value="sort" @change="changeSort"><option v-for="option in sortOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label><button class="account-detail-direction" type="button" :aria-label="direction === 'asc' ? '升序，点击切换为降序' : '降序，点击切换为升序'" :title="direction === 'asc' ? '升序' : '降序'" @click="toggleDirection"><ArrowUp v-if="direction === 'asc'" :size="15" aria-hidden="true" /><ArrowDown v-else :size="15" aria-hidden="true" /></button><label class="analysis-limit-select"><span>每页</span><select :value="pageSize" @change="changePageSize"><option :value="20">20</option><option :value="50">50</option></select></label></div></div><div v-if="tabLoading && !videos.length" class="account-detail-loading"><n-spin size="medium" /><span>正在读取账户视频…</span></div><div v-else-if="!videos.length" class="analysis-empty"><n-empty description="当前账户暂无可展示的视频记录" /></div><div v-else class="analysis-table-wrap"><table class="analysis-table account-detail-video-table"><thead><tr><th>视频</th><th>发布时间</th><th class="analysis-number-col">点赞</th><th class="analysis-number-col">评论</th><th class="analysis-number-col">收藏</th><th class="analysis-number-col">转发</th><th>来源</th><th class="analysis-action-col"><span class="sr-only">操作</span></th></tr></thead><tbody><tr v-for="video in videos" :key="video.id || video.objectId"><td class="analysis-title-cell account-detail-video-title-cell"><button class="analysis-title-button" type="button" :disabled="!video.objectId" :title="video.objectId ? '打开视频详情' : '缺少视频标识'" @click="emit('open-video', video)">{{ video.title || "未命名视频" }}</button><span>{{ videoSubtitle(video) }}</span></td><td class="account-detail-time-cell"><strong>{{ videoTime(video) }}</strong><small>采集 {{ formatDateTime(video.observedAt) }}</small></td><td class="analysis-number-cell"><span class="account-detail-metric"><Heart :size="13" aria-hidden="true" />{{ formatMetric(video.likes) }}</span></td><td class="analysis-number-cell"><span class="account-detail-metric"><MessageCircle :size="13" aria-hidden="true" />{{ formatMetric(video.comments) }}</span></td><td class="analysis-number-cell"><span class="account-detail-metric"><Bookmark :size="13" aria-hidden="true" />{{ formatMetric(video.favorites) }}</span></td><td class="analysis-number-cell"><span class="account-detail-metric"><Share2 :size="13" aria-hidden="true" />{{ formatMetric(video.forwards) }}</span></td><td class="analysis-muted-cell">{{ video.source || "来源待确认" }}</td><td class="analysis-action-col"><button class="analysis-icon-button" type="button" :disabled="!video.objectId" aria-label="查看视频详情" title="查看视频详情" @click="emit('open-video', video)"><ArrowUpRight :size="16" aria-hidden="true" /></button></td></tr></tbody></table></div><footer v-if="total" class="data-center-pagination account-detail-pagination"><span>显示 {{ pageStart }}–{{ pageEnd }}，共 {{ formatNumber(total) }} 条</span><div class="data-center-pagination-controls"><button class="analysis-icon-button" type="button" aria-label="上一页" title="上一页" :disabled="page <= 1 || tabLoading" @click="changePage(page - 1)"><ChevronLeft :size="16" aria-hidden="true" /></button><strong>第 {{ page }} / {{ totalPages }} 页</strong><button class="analysis-icon-button" type="button" aria-label="下一页" title="下一页" :disabled="page >= totalPages || tabLoading" @click="changePage(page + 1)"><ChevronRight :size="16" aria-hidden="true" /></button></div></footer></section>

    <section v-else-if="activeTab === 'comments'" class="analysis-surface account-detail-comments-surface" aria-labelledby="account-comments-title"><div class="analysis-surface-head account-detail-list-head"><div><div class="analysis-section-label">原始事实</div><h3 id="account-comments-title">逐条评论库</h3><p>顶层评论与回复均保留父子关系，不替代评论洞察排行榜。</p></div><div class="account-detail-list-meta"><span>{{ tabLoading ? "读取中…" : `${formatNumber(commentTotal)} 条评论` }}</span><button class="account-detail-refresh" type="button" title="刷新评论库" aria-label="刷新评论库" :disabled="tabLoading" @click="loadComments"><RefreshCw :size="15" aria-hidden="true" /></button></div></div><form class="account-detail-comment-search" @submit.prevent="submitCommentSearch"><label><MessageCircle :size="14" aria-hidden="true" /><input v-model="commentKeyword" type="search" placeholder="搜索评论正文、评论人或视频" /></label><button class="tool-button secondary" type="submit">查询</button><label class="analysis-limit-select"><span>每页</span><select :value="commentPageSize" @change="changeCommentPageSize"><option :value="20">20</option><option :value="50">50</option></select></label></form><div v-if="tabLoading && !comments.length" class="account-detail-loading"><n-spin size="medium" /><span>正在读取原始评论…</span></div><div v-else-if="!comments.length" class="analysis-empty"><n-empty description="当前账户暂无逐条评论记录" /></div><div v-else class="analysis-table-wrap"><table class="analysis-table account-detail-comment-table"><thead><tr><th>评论内容</th><th>视频</th><th>评论人</th><th>IP 地区</th><th class="analysis-number-col">点赞</th><th class="analysis-number-col">回复</th><th>评论时间</th><th>采集时间</th></tr></thead><tbody><tr v-for="comment in comments" :key="comment.id || comment.commentId"><td class="account-detail-comment-content"><span v-if="comment.parentCommentId" class="account-detail-reply-mark">回复</span><span :title="commentContent(comment)">{{ commentContent(comment) }}</span></td><td class="analysis-muted-cell" :title="comment.videoTitle">{{ comment.videoTitle || comment.videoObjectId || "未标记视频" }}</td><td class="account-detail-comment-author"><strong>{{ comment.authorNickname || comment.authorUsername || "匿名用户" }}</strong><small>{{ comment.authorUsername || "未提供账号" }}</small></td><td class="analysis-muted-cell">{{ comment.ipRegion || "未设置" }}</td><td class="analysis-number-cell">{{ formatMetric(comment.likeCount) }}</td><td class="analysis-number-cell">{{ formatMetric(comment.replyCount) }}</td><td class="analysis-muted-cell">{{ formatDateTime(comment.createdAt) }}</td><td class="analysis-muted-cell">{{ formatDateTime(comment.collectedAt) }}</td></tr></tbody></table></div><footer v-if="commentTotal" class="data-center-pagination account-detail-pagination"><span>显示 {{ commentPageStart }}–{{ commentPageEnd }}，共 {{ formatNumber(commentTotal) }} 条</span><div class="data-center-pagination-controls"><button class="analysis-icon-button" type="button" aria-label="上一页" title="上一页" :disabled="commentPage <= 1 || tabLoading" @click="changeCommentPage(commentPage - 1)"><ChevronLeft :size="16" aria-hidden="true" /></button><strong>第 {{ commentPage }} / {{ commentTotalPages }} 页</strong><button class="analysis-icon-button" type="button" aria-label="下一页" title="下一页" :disabled="commentPage >= commentTotalPages || tabLoading" @click="changeCommentPage(commentPage + 1)"><ChevronRight :size="16" aria-hidden="true" /></button></div></footer></section>

    <section v-else class="analysis-surface account-detail-history-surface" aria-labelledby="account-history-title"><div class="analysis-surface-head account-detail-list-head"><div><div class="analysis-section-label">观察证据</div><h3 id="account-history-title">账户历史</h3><p>只展示实际账户观察或指标观察，不补造缺失历史。</p></div><div class="account-detail-list-meta"><span>{{ tabLoading ? "读取中…" : `${formatNumber(historyTotal)} 条证据` }}</span><button class="account-detail-refresh" type="button" title="刷新历史证据" aria-label="刷新历史证据" :disabled="tabLoading" @click="loadHistory"><RefreshCw :size="15" aria-hidden="true" /></button></div></div><div v-if="tabLoading && !history.length" class="account-detail-loading"><n-spin size="medium" /><span>正在读取历史证据…</span></div><div v-else-if="!history.length" class="analysis-empty"><n-empty description="当前账户暂无可展示的历史证据" /></div><div v-else class="analysis-table-wrap"><table class="analysis-table account-detail-history-table"><thead><tr><th>时间</th><th>证据类型</th><th>来源</th><th>内容</th><th class="analysis-number-col">视频数</th><th class="analysis-number-col">评论</th><th>说明</th></tr></thead><tbody><tr v-for="item in history" :key="item.id || item.observedAt"><td class="analysis-muted-cell">{{ formatDateTime(item.observedAt) }}</td><td>{{ item.evidenceType || "观察" }}</td><td class="analysis-muted-cell">{{ item.source || "来源待确认" }}</td><td class="account-detail-history-title">{{ item.title || item.channelName || item.channelUsername || "账户观察" }}</td><td class="analysis-number-cell">{{ formatMetric(item.videoCount) }}</td><td class="analysis-number-cell">{{ formatMetric(item.comments) }}</td><td class="analysis-muted-cell">{{ item.signal || "真实采集证据" }}</td></tr></tbody></table></div><footer v-if="historyTotal" class="data-center-pagination account-detail-pagination"><span>显示 {{ historyPageStart }}–{{ historyPageEnd }}，共 {{ formatNumber(historyTotal) }} 条</span><div class="data-center-pagination-controls"><button class="analysis-icon-button" type="button" aria-label="上一页" title="上一页" :disabled="historyPage <= 1 || tabLoading" @click="changeHistoryPage(historyPage - 1)"><ChevronLeft :size="16" aria-hidden="true" /></button><strong>第 {{ historyPage }} / {{ historyTotalPages }} 页</strong><button class="analysis-icon-button" type="button" aria-label="下一页" title="下一页" :disabled="historyPage >= historyTotalPages || tabLoading" @click="changeHistoryPage(historyPage + 1)"><ChevronRight :size="16" aria-hidden="true" /></button><label class="analysis-limit-select"><span>每页</span><select :value="historyPageSize" @change="changeHistoryPageSize"><option :value="20">20</option><option :value="50">50</option></select></label></div></footer></section>
    <div v-if="accountID" ref="taskSection"><MonitoringTasksPanel ref="taskPanel" scope="accounts" :target-key="accountID" :target-aliases="[account?.channelUsername, accountKey].filter(Boolean)" :binding-states="bindingStates" @changed="monitoringChanged" /></div>
  </section>
</template>
