<script setup>
import { computed, onMounted, ref } from "vue";
import {
  ArrowUpRight,
  Check,
  ChevronLeft,
  ChevronRight,
  Database,
  Flag,
  Radio,
  RefreshCw,
  Search,
  Star,
  UserRound,
} from "lucide-vue-next";
import { NAlert, NEmpty, NSpin } from "naive-ui";
import {
  fetchAccounts,
  startAccountMonitoring,
  updateAccountProfile,
} from "../lib/api.js";

const props = defineProps({
  formatNumber: { type: Function, required: true },
  formatDateTime: { type: Function, required: true },
});

const emit = defineEmits(["open-detail", "changed"]);

const keyword = ref("");
const region = ref("");
const industry = ref("");
const source = ref("");
const priority = ref("");
const monitoringStatus = ref("");
const collectedFrom = ref("");
const collectedTo = ref("");
const dataNotice = ref("");
const displayedPage = ref(1);
const displayedPageSize = ref(20);
const page = ref(1);
const pageSize = ref(20);
const items = ref([]);
const total = ref(0);
const totalPages = ref(1);
const loading = ref(false);
const actionID = ref("");
const error = ref("");
const notice = ref("");
const requestSerial = ref(0);

const summary = computed(() => ({
  total: total.value,
  priority: items.value.filter((item) => item?.isPriority).length,
  monitored: items.value.filter((item) => item?.monitoring?.enabled).length,
}));

const pageStart = computed(() => (total.value ? (displayedPage.value - 1) * displayedPageSize.value + 1 : 0));
const pageEnd = computed(() => Math.min(total.value, displayedPage.value * displayedPageSize.value));

function metric(account, key) {
  return account?.metrics?.[key] ?? null;
}

function displayCount(value) {
  return value === null || value === undefined ? "—" : props.formatNumber(value);
}

function accountName(account) {
  return account?.channelName || account?.channelUsername || "未命名账号";
}

function accountIdentity(account) {
  return account?.channelUsername || account?.accountId || "账号标识待确认";
}

function monitoringLabel(account) {
  if (account?.monitoring?.error) return "状态暂不可用";
  if (account?.monitoring?.enabled) {
    return `监控中 · ${account.monitoring.taskCount || 0} 个任务`;
  }
  if (account?.monitoring?.taskCount > 0) return "已暂停";
  return "未加入监控";
}

function monitoringTone(account) {
  return account?.monitoring?.enabled ? "active" : "idle";
}

function clearFeedback() {
  error.value = "";
  notice.value = "";
}

function queryPayload() {
  return {
    q: keyword.value.trim(),
    region: region.value.trim(),
    industry: industry.value.trim(),
    source: source.value,
    monitoringStatus: monitoringStatus.value,
    from: collectedFrom.value ? `${collectedFrom.value}T00:00:00+08:00` : "",
    to: collectedTo.value ? `${collectedTo.value}T23:59:59.999999999+08:00` : "",
    ...(priority.value ? { priority: priority.value } : {}),
    page: page.value,
    pageSize: pageSize.value,
    sort: "updated_at",
    direction: "desc",
  };
}

async function load({ keepFeedback = false } = {}) {
  if (collectedFrom.value && collectedTo.value && collectedFrom.value > collectedTo.value) {
    error.value = "最近采集起始日期不能晚于截止日期。";
    return;
  }
  const serial = requestSerial.value + 1;
  requestSerial.value = serial;
  loading.value = true;
  if (!keepFeedback) clearFeedback();
  try {
    const payload = await fetchAccounts(queryPayload());
    if (serial !== requestSerial.value) return;
    items.value = Array.isArray(payload?.items) ? payload.items : [];
    page.value = Number(payload?.page) || page.value;
    pageSize.value = Number(payload?.pageSize) || pageSize.value;
    total.value = Number(payload?.total) || 0;
    totalPages.value = Number(payload?.totalPages) || 1;
    displayedPage.value = page.value;
    displayedPageSize.value = pageSize.value;
    dataNotice.value = payload?.dataNotice || "";
  } catch (loadError) {
    if (serial !== requestSerial.value) return;
    error.value = `${loadError instanceof Error ? loadError.message : "账号列表加载失败"}。上次有效数据仍保留。`;
    page.value = displayedPage.value;
    pageSize.value = displayedPageSize.value;
  } finally {
    if (serial === requestSerial.value) loading.value = false;
  }
}

function submitSearch() {
  page.value = 1;
  load();
}

function resetSearch() {
  keyword.value = "";
  region.value = "";
  industry.value = "";
  source.value = "";
  priority.value = "";
  monitoringStatus.value = "";
  collectedFrom.value = "";
  collectedTo.value = "";
  page.value = 1;
  load();
}

function changePageSize(event) {
  pageSize.value = Number(event.target.value) === 50 ? 50 : 20;
  page.value = 1;
  load();
}

function changePage(nextPage) {
  if (loading.value || nextPage < 1 || nextPage > totalPages.value) return;
  page.value = nextPage;
  load();
}

async function togglePriority(account) {
  const accountID = String(account?.accountId || "").trim();
  if (!accountID) return;
  actionID.value = `${accountID}:priority`;
  clearFeedback();
  try {
    await updateAccountProfile(accountID, { isPriority: !account.isPriority });
    notice.value = account.isPriority ? "已取消重点账号标记" : "已加入重点账号";
    await load({ keepFeedback: true });
  } catch (actionError) {
    error.value = actionError instanceof Error ? actionError.message : "重点标记更新失败";
  } finally {
    actionID.value = "";
  }
}

async function monitorAccount(account) {
  const accountID = String(account?.accountId || "").trim();
  if (!accountID || actionID.value) return;
  if (account?.monitoring?.taskCount > 0) { emit("open-detail", account); return; }
  actionID.value = `${accountID}:monitor`;
  clearFeedback();
  try {
    await startAccountMonitoring(accountID, { commentMode: "incremental" });
    notice.value = "账号监控任务已创建，可在下方采集设置查看并运行。";
    await load({ keepFeedback: true });
    emit("changed");
  } catch (actionError) {
    error.value = actionError instanceof Error ? actionError.message : "账号监控创建失败";
  } finally {
    actionID.value = "";
  }
}

onMounted(load);

defineExpose({ refresh: load });
</script>

<template>
  <section class="analysis-page account-library-page" aria-labelledby="account-library-title">
    <div class="analysis-page-head">
      <div>
        <div class="analysis-page-kicker">账号监控与表现</div>
        <h2 id="account-library-title">已收录账号</h2>
        <p>账号身份、监控关系与已采集视频表现集中查看；指标只展示真实采集证据。</p>
      </div>
      <div class="analysis-page-meta"><Database :size="15" aria-hidden="true" /><span>共 {{ formatNumber(summary.total) }} 个账号</span></div>
    </div>

    <n-alert v-if="notice" type="success" :show-icon closable @close="notice = ''">{{ notice }}
    </n-alert>
    <n-alert v-if="error" type="error" :show-icon closable @close="error = ''">{{ error }}
    </n-alert>
    <n-alert v-if="dataNotice" type="warning" :show-icon>{{ dataNotice }}</n-alert>

    <section class="analysis-surface account-library-filter-surface" aria-label="账户筛选">
      <div class="account-library-filter-head">
        <div><div class="analysis-section-label">检索条件</div><h3>筛选账号</h3></div>
        <span class="account-library-filter-hint"><Flag :size="14" aria-hidden="true" />本页重点 {{ summary.priority }} · 本页监控中 {{ summary.monitored }}</span>
      </div>
      <form class="account-library-filters" @submit.prevent="submitSearch">
        <label class="account-library-field account-library-field-wide">
          <span>账号、昵称或简介</span>
          <div class="account-library-input-wrap"><Search :size="15" aria-hidden="true" /><input v-model="keyword" type="search" placeholder="搜索账号标识、昵称、简介" /></div>
        </label>
        <label class="account-library-field"><span>地区</span><input v-model="region" type="text" placeholder="如 华东" /></label>
        <label class="account-library-field"><span>行业</span><input v-model="industry" type="text" placeholder="如 教育" /></label>
        <label class="account-library-field"><span>画像来源</span><select v-model="source"><option value="">全部来源</option><option value="upstream">上游画像</option><option value="manual">人工维护</option><option value="mixed">混合来源</option></select></label>
        <label class="account-library-field"><span>重点状态</span><select v-model="priority"><option value="">全部账户</option><option value="true">重点账号</option><option value="false">普通账号</option></select></label>
        <label class="account-library-field"><span>监控状态</span><select v-model="monitoringStatus"><option value="">全部状态</option><option value="enabled">监控中</option><option value="paused">已暂停</option><option value="unmonitored">未加入监控</option></select></label>
        <label class="account-library-field"><span>最近采集起始</span><input v-model="collectedFrom" type="date" /></label>
        <label class="account-library-field"><span>最近采集截止</span><input v-model="collectedTo" type="date" /></label>
        <div class="account-library-filter-actions"><button class="tool-button primary" type="submit"><Search :size="14" aria-hidden="true" />查询</button><button class="tool-button secondary" type="button" @click="resetSearch"><RefreshCw :size="14" aria-hidden="true" />重置</button></div>
      </form>
      <p class="account-library-filter-note">采集日期用于筛选最近采集过视频数据的账号；列表指标为已采集视频的最新汇总。</p>
    </section>

    <section class="analysis-surface account-library-table-surface" aria-labelledby="account-library-table-title">
      <div class="analysis-surface-head account-library-table-head">
        <div><div class="analysis-section-label">账户清单</div><h3 id="account-library-table-title">全部账户</h3></div>
        <div class="account-library-table-meta"><span>{{ loading ? "正在同步…" : `显示 ${pageStart}–${pageEnd} / 共 ${formatNumber(total)}` }}</span><button class="analysis-icon-button" type="button" title="刷新账户库" aria-label="刷新账户库" :disabled="loading" @click="load"><RefreshCw :size="15" aria-hidden="true" /></button><label class="analysis-limit-select"><span>每页</span><select :value="pageSize" @change="changePageSize"><option :value="20">20</option><option :value="50">50</option></select></label></div>
      </div>

      <div v-if="loading && !items.length" class="account-library-loading"><n-spin size="medium" /><span>正在读取账户主数据…</span></div>
      <div v-else-if="!items.length" class="analysis-empty"><n-empty description="当前筛选范围暂无账户记录" /></div>
      <div v-else class="analysis-table-wrap">
        <table class="analysis-table account-library-table">
          <thead><tr><th>账号</th><th>昵称 / 简介</th><th>地区</th><th>行业</th><th>重点</th><th>监控状态</th><th class="analysis-number-col">视频</th><th class="analysis-number-col">评论</th><th>最近采集</th><th class="analysis-action-col"><span class="sr-only">操作</span></th></tr></thead>
          <tbody>
            <tr v-for="account in items" :key="account.accountId">
              <td class="analysis-title-cell account-library-identity-cell"><button class="analysis-title-button" type="button" @click="emit('open-detail', account)"><UserRound :size="14" aria-hidden="true" />{{ accountName(account) }}</button><span>{{ accountIdentity(account) }}</span></td>
              <td class="account-library-profile-cell"><strong>{{ account.channelName || "未设置" }}</strong><span :title="account.signature || '未设置'">{{ account.signature || "未设置简介" }}</span></td>
              <td class="analysis-muted-cell">{{ account.region || "未设置" }}</td>
              <td class="analysis-muted-cell">{{ account.industry || "未设置" }}</td>
              <td><button class="account-library-flag" :class="{ active: account.isPriority }" type="button" :disabled="actionID" :aria-label="account.isPriority ? '取消重点账号' : '设为重点账号'" :title="account.isPriority ? '取消重点账号' : '设为重点账号'" @click="togglePriority(account)"><Star :size="15" :fill="account.isPriority ? 'currentColor' : 'none'" aria-hidden="true" /></button></td>
              <td><span class="account-library-status" :data-tone="monitoringTone(account)"><Radio :size="13" aria-hidden="true" />{{ monitoringLabel(account) }}</span></td>
              <td class="analysis-number-cell">{{ displayCount(metric(account, "videoCount")) }}</td>
              <td class="analysis-number-cell">{{ displayCount(metric(account, "comments")) }}</td>
              <td class="analysis-muted-cell">{{ account.metrics?.recentObservedAt && !account.metrics.recentObservedAt.startsWith('0001-') ? formatDateTime(account.metrics.recentObservedAt) : "未采集" }}</td>
              <td class="analysis-action-col"><div class="account-library-actions"><button class="analysis-icon-button" type="button" title="查看账号详情" aria-label="查看账号详情" @click="emit('open-detail', account)"><ArrowUpRight :size="16" aria-hidden="true" /></button><button class="account-library-monitor-button" type="button" :disabled="Boolean(actionID)" :title="account.monitoring?.taskCount ? '管理已有监控' : '加入监控'" @click="monitorAccount(account)"><Check :size="13" aria-hidden="true" />{{ account.monitoring?.taskCount ? '管理监控' : '加入监控' }}</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
      <footer v-if="total" class="data-center-pagination account-library-pagination"><span>显示 {{ pageStart }}–{{ pageEnd }}，共 {{ formatNumber(total) }} 条</span><div class="data-center-pagination-controls"><button class="analysis-icon-button" type="button" aria-label="上一页" title="上一页" :disabled="page <= 1 || loading" @click="changePage(page - 1)"><ChevronLeft :size="16" aria-hidden="true" /></button><strong>第 {{ page }} / {{ totalPages }} 页</strong><button class="analysis-icon-button" type="button" aria-label="下一页" title="下一页" :disabled="page >= totalPages || loading" @click="changePage(page + 1)"><ChevronRight :size="16" aria-hidden="true" /></button></div></footer>
    </section>
  </section>
</template>
<style scoped>
.account-library-filter-note { margin: 0; padding: 0 16px 16px; color: var(--analytics-muted); font-size: 12px; line-height: 1.6; }
</style>
