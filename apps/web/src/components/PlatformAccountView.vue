<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { NAlert, NButton, NSpin } from "naive-ui";
import { ExternalLink, RefreshCw, UserRound } from "lucide-vue-next";
import AccountDetailView from "./AccountDetailView.vue";
import VideoDetailView from "./VideoDetailView.vue";
import { collectChannelAccount, fetchAccounts, fetchChannelAccountSession, openChannelAccountLogin } from "../lib/api.js";

const props = defineProps({ edgeNodes: { type: Array, default: () => [] } });
const emit = defineEmits(["navigate", "refresh"]);

const session = ref(null);
const loading = ref(true);
const refreshing = ref(false);
const busy = ref(false);
const error = ref("");
const notice = ref("");
const collection = ref(null);
const accountData = ref(null);
const accountLoading = ref(false);
const dataSection = ref(null);
const selectedVideoId = ref("");
let timer;
let sessionLoading = false;
let disposed = false;
let sessionController;
let accountLookupUsername = "";

const node = computed(() => session.value?.node || null);
const username = computed(() => session.value?.status === "authenticated" ? String(session.value?.finderUsername || "").trim() : "");
const authenticated = computed(() => session.value?.status === "authenticated" && username.value);
const sessionLabel = computed(() => {
  if (authenticated.value) return "已登录，可查看本账号数据";
  if (session.value?.status === "login_required") return "等待登录视频号助手";
  return "等待 wx_channel 连接";
});
const sessionTone = computed(() => authenticated.value ? "good" : session.value?.status === "login_required" ? "warn" : "muted");

async function load(options = {}) {
  if (disposed || sessionLoading) return;
  sessionLoading = true;
  window.clearTimeout(timer);
  sessionController = new AbortController();
  if (options.background) refreshing.value = true;
  else loading.value = true;
  try {
    const nextSession = await fetchChannelAccountSession({ clientId: node.value?.id || node.value?.clientId, signal: sessionController.signal });
    if (disposed) return;
    const previousUsername = username.value;
    const previousClientId = session.value?.clientId;
    session.value = nextSession;
    error.value = "";
    const nextUsername = username.value;
    if (!nextUsername || previousUsername !== nextUsername || previousClientId !== nextSession.clientId) {
      collection.value = null;
      accountLookupUsername = "";
    }
    const sameAccount = String(accountData.value?.channelUsername || "").toLowerCase() === nextUsername.toLowerCase();
    if (!nextUsername || !sameAccount) {
      accountData.value = null;
      selectedVideoId.value = "";
    }
    if (authenticated.value && (!options.background || accountLookupUsername !== nextUsername)) void loadAccountData(options);
  } catch (cause) {
    if (disposed) return;
    session.value = null;
    accountData.value = null;
    collection.value = null;
    accountLookupUsername = "";
    selectedVideoId.value = "";
    error.value = `账号连接状态读取失败：${cause instanceof Error ? cause.message : cause}`;
  } finally {
    loading.value = false;
    refreshing.value = false;
    sessionLoading = false;
    if (!disposed) timer = window.setTimeout(() => void load({ background: true }), authenticated.value ? 15000 : 5000);
  }
}

async function loadAccountData(options = {}) {
  const currentUsername = username.value;
  if (!currentUsername || accountLoading.value) return;
  accountLoading.value = true;
  try {
    const payload = await fetchAccounts({ q: currentUsername, page: 1, pageSize: 20 });
    const match = (payload?.items || []).find((item) => String(item?.channelUsername || "").toLowerCase() === currentUsername.toLowerCase());
    if (!disposed && currentUsername === username.value) {
      accountData.value = match || null;
      accountLookupUsername = currentUsername;
    }
  } catch (cause) {
    if (!options.background) error.value = `本账号数据读取失败：${cause instanceof Error ? cause.message : cause}`;
  } finally {
    accountLoading.value = false;
  }
}

async function login() {
  if (busy.value || !session.value || session.value.status === "offline") return;
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await openChannelAccountLogin(session.value.clientId || node.value?.id || "");
    notice.value = "已请求在内置浏览器中打开官方登录页，请在该页面用手机微信扫码并确认。";
    await load({ background: true });
  } catch (cause) {
    error.value = `打开 Insight 内置浏览器失败：${cause instanceof Error ? cause.message : cause}`;
  } finally {
    busy.value = false;
  }
}

async function collect() {
  if (busy.value || !authenticated.value) return;
  busy.value = true;
  error.value = "";
  notice.value = "";
  const currentUsername = username.value;
  const currentClientId = session.value.clientId || node.value?.id || "";
  try {
    const result = await collectChannelAccount(currentClientId, currentUsername);
    if (disposed || currentUsername !== username.value || currentClientId !== (session.value?.clientId || node.value?.id || "")) return;
    collection.value = result;
    await loadAccountData();
    if (disposed || currentUsername !== username.value || currentClientId !== (session.value?.clientId || node.value?.id || "")) return;
    notice.value = `已采集 ${result.itemCount || 0} 条本账号作品，数据已写入工作区。`;
    emit("refresh");
  } catch (cause) {
    if (disposed || currentUsername !== username.value) return;
    error.value = `本账号作品采集失败：${cause instanceof Error ? cause.message : cause}`;
  } finally {
    busy.value = false;
  }
}

function openData() {
  dataSection.value?.scrollIntoView({ behavior: "smooth", block: "start" });
}

function formatNumber(value) {
  const number = Number(value);
  return Number.isFinite(number) ? new Intl.NumberFormat("zh-CN").format(number) : "—";
}

function formatDateTime(value) {
  if (!value || String(value).startsWith("0001-")) return "未记录";
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? date.toLocaleString("zh-CN", { hour12: false }) : "未记录";
}

function handleAccountVideo(payload) {
  const video = payload?.video || payload;
  if (!video?.objectId) return;
  selectedVideoId.value = String(video.objectId);
}

onMounted(() => {
  void load();
});
onBeforeUnmount(() => {
  disposed = true;
  window.clearTimeout(timer);
  sessionController?.abort();
});

defineExpose({ refresh: () => load({ background: true }) });
</script>

<template>
  <section class="platform-account-page">
    <div class="analysis-page-head">
      <div>
        <div class="analysis-page-kicker">视频号 · 本账号工作台</div>
        <h2>平台账号</h2>
        <p>登录后查看本账号作品和数据。</p>
      </div>
      <n-button secondary :loading="loading || refreshing" @click="load()">
        <template #icon><RefreshCw /></template>
        刷新连接
      </n-button>
    </div>

    <n-alert v-if="error" type="error" show-icon>{{ error }}</n-alert>
    <n-alert v-if="notice" type="info" show-icon>{{ notice }}</n-alert>

    <div v-if="loading" class="platform-account-loading"><n-spin size="medium" /><span>正在读取视频号助手连接…</span></div>
    <template v-else>
      <section class="platform-account-hero" :data-tone="sessionTone">
        <div class="platform-account-hero-icon" aria-hidden="true"><UserRound /></div>
        <div class="platform-account-hero-copy">
          <span class="platform-account-eyebrow">视频号助手</span>
          <h3>{{ sessionLabel }}</h3>
          <p v-if="authenticated">当前登录账号：<strong>{{ username }}</strong></p>
          <p v-else-if="session?.status === 'login_required'">请在内置浏览器中扫码登录。</p>
          <p v-else>请先启动 Insight 桌面版，内置浏览器会自动连接并提供登录入口。</p>
        </div>
        <div class="platform-account-hero-actions">
          <n-button v-if="!authenticated" type="primary" :disabled="session?.status === 'offline'" :loading="busy" @click="login">
            <template #icon><ExternalLink /></template>
            在内置浏览器中登录
          </n-button>
          <n-button v-else type="primary" :loading="busy" @click="collect">采集本账号作品</n-button>
          <n-button v-if="authenticated" secondary :disabled="busy" @click="openData">查看本账号数据</n-button>
        </div>
      </section>

      <section class="platform-account-summary" aria-label="账号摘要">
        <div class="platform-account-summary-item">
          <span class="platform-account-eyebrow">连接状态</span>
          <div class="platform-account-summary-value"><span class="platform-account-status-dot" :data-tone="authenticated ? 'good' : 'muted'" aria-hidden="true"></span><strong>{{ authenticated ? '已连接' : sessionLabel }}</strong></div>
          <small>{{ node?.hostname || 'Insight 内置 Edge' }} · {{ node?.pagePath || '等待页面' }}</small>
        </div>
        <div class="platform-account-summary-item">
          <span class="platform-account-eyebrow">本账号作品</span>
          <div class="platform-account-summary-value"><strong>{{ accountData?.metrics?.videoCount ?? collection?.itemCount ?? '—' }}</strong><span>条</span></div>
          <button class="platform-account-link" type="button" :disabled="!authenticated" @click="openData">查看作品 <span aria-hidden="true">→</span></button>
        </div>
      </section>

      <section v-if="authenticated" ref="dataSection" class="platform-account-data-section" aria-labelledby="platform-account-data-title">
        <div class="platform-account-data-heading">
          <div>
            <span class="platform-account-eyebrow">独立数据空间</span>
            <h3 id="platform-account-data-title">本账号数据</h3>
            <p>仅展示当前登录账号的指标、作品、评论和监控任务。</p>
          </div>
          <n-button secondary :loading="accountLoading" @click="loadAccountData()">
            <template #icon><RefreshCw /></template>
            刷新本账号数据
          </n-button>
        </div>
        <div v-if="accountLoading && !accountData" class="platform-account-data-loading"><n-spin size="medium" /><span>正在读取本账号数据…</span></div>
        <div v-else-if="accountData" class="platform-account-data-detail">
          <VideoDetailView
            v-if="selectedVideoId"
            :object-id="selectedVideoId"
            :auto-collect="false"
            @back="selectedVideoId = ''"
          />
          <AccountDetailView
            v-else
            :account-key="accountData.channelUsername || username"
            :initial-account="accountData"
            :format-number="formatNumber"
            :format-date-time="formatDateTime"
            @open-video="handleAccountVideo"
            @refresh="loadAccountData"
          />
        </div>
        <div v-else class="platform-account-data-empty">
          <strong>还没有本账号数据</strong>
          <span>先点击“采集本账号作品”，采集完成后这里会显示独立的账号指标和作品列表。</span>
        </div>
      </section>
    </template>
  </section>
</template>
