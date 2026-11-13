<script setup>
import { computed, onMounted, ref } from "vue";
import { Activity, CheckCircle2, Clock3, Radio, RefreshCw, ShieldAlert, Video } from "lucide-vue-next";
import { NAlert, NButton, NIcon } from "naive-ui";
import MonitoringTasksPanel from "./MonitoringTasksPanel.vue";
import { fetchMonitoringHealth, fetchMonitoringTasks } from "../lib/api.js";
import {
  formatMonitoringTime,
  monitoringRunStatusLabel,
  monitoringStatusTone,
  monitoringTaskStatusLabel,
} from "../lib/monitoringTasks.js";

const props = defineProps({
  bindingStates: { type: Array, default: () => [] },
});

const taskPanel = ref(null);
const tasks = ref([]);
const health = ref(null);
const loading = ref(false);
const error = ref("");
const lastUpdated = ref("");

const enabledTasks = computed(() => tasks.value.filter((task) => task.status === "enabled"));
const runningTasks = computed(() => tasks.value.filter((task) => task.lastStatus === "running"));
const attentionCount = computed(() => {
  const fromHealth = Number(health.value?.failedTasks || 0) + Number(health.value?.overdueTasks || 0);
  if (fromHealth) return fromHealth;
  return tasks.value.filter((task) => ["failed", "partial", "abandoned"].includes(task.lastStatus)).length;
});
const targetCount = computed(() => tasks.value.reduce((total, task) => {
  if (task.scopeMode === "all_accounts" || task.scopeMode === "all_saved_bindings") return total + 1;
  return total + (Array.isArray(task.targets) ? task.targets.length : 0);
}, 0));
const recentTasks = computed(() => [...tasks.value]
  .filter((task) => task.lastRunAt || task.lastStatus)
  .sort((left, right) => new Date(right.lastRunAt || right.updatedAt || 0) - new Date(left.lastRunAt || left.updatedAt || 0))
  .slice(0, 5));

const schedulerLabel = computed(() => ({
  online: "调度器在线",
  offline: "调度器离线",
  unknown: "等待调度器",
}[health.value?.schedulerStatus] || (health.value?.schedulerStatus === "running"
  ? (runningTasks.value.length ? "正在采集" : enabledTasks.value.length ? "等待任务执行" : "调度器在线")
  : "等待调度器")));
const schedulerTone = computed(() => {
  if (health.value?.schedulerStatus === "offline") return "risk";
  if (health.value?.schedulerStatus === "online" || health.value?.schedulerStatus === "running") return "good";
  return "warn";
});
const coverageCount = computed(() => Array.isArray(health.value?.coverage) ? health.value.coverage.length : 0);
const schedulerNote = computed(() => {
  if (!tasks.value.length) return "还没有监控任务，创建任务后才会按计划采集数据。";
  if (attentionCount.value) return "失败或逾期任务会保留运行记录，处理后可重试。";
  return "调度器只领取到期任务，避免同一任务重复执行。";
});

function formatUpdated(value) {
  return value ? formatMonitoringTime(value, "刚刚") : "等待首次读取";
}

async function load() {
  if (loading.value) return;
  loading.value = true;
  error.value = "";
  const results = await Promise.allSettled([fetchMonitoringTasks(), fetchMonitoringHealth()]);
  if (results[0].status === "fulfilled") {
    tasks.value = Array.isArray(results[0].value?.items) ? results[0].value.items : [];
  } else {
    error.value = results[0].reason instanceof Error ? results[0].reason.message : "监控任务读取失败";
  }
  if (results[1].status === "fulfilled") {
    health.value = results[1].value;
  } else if (!error.value) {
    error.value = results[1].reason instanceof Error ? results[1].reason.message : "调度健康状态读取失败";
  }
  lastUpdated.value = new Date().toISOString();
  loading.value = false;
}

async function refreshAll() {
  await Promise.allSettled([load(), taskPanel.value?.refresh?.()]);
}

async function changed() {
  await load();
}

onMounted(load);
defineExpose({ refresh: refreshAll });
</script>

<template>
  <section class="monitoring-center-page" :aria-busy="loading">
    <div class="monitoring-center-hero">
      <div class="monitoring-center-hero-copy">
        <div class="monitoring-center-kicker"><Radio :size="14" aria-hidden="true" />视频号 · Insight 监控工作台</div>
        <h2>监控任务一览</h2>
        <p>集中查看调度状态、任务异常和运行记录；任务按计划采集账号、作品和评论数据。</p>
        <div class="monitoring-center-hero-actions">
          <n-button type="primary" @click="taskPanel?.openCreate?.()">新建监控任务</n-button>
          <n-button secondary :loading="loading" @click="refreshAll">
            <template #icon><n-icon><RefreshCw /></n-icon></template>
            刷新状态
          </n-button>
        </div>
      </div>
      <div class="monitoring-center-hero-status" :data-tone="schedulerTone">
        <div class="monitoring-center-status-icon"><n-icon size="22"><Activity /></n-icon></div>
        <div><strong>{{ schedulerLabel }}</strong><span>最后心跳 {{ formatUpdated(health?.lastHeartbeatAt) }}</span><span>状态更新 {{ formatUpdated(lastUpdated) }}</span></div>
      </div>
    </div>

    <n-alert v-if="error" type="warning" show-icon closable @close="error = ''">{{ error }}。点击“刷新状态”重试。</n-alert>

    <div class="monitoring-center-stat-grid" aria-label="监控摘要">
      <article class="monitoring-center-stat-card"><span><CheckCircle2 :size="16" aria-hidden="true" />启用任务</span><strong>{{ enabledTasks.length }}</strong><small>按周期自动采集</small></article>
      <article class="monitoring-center-stat-card"><span><Clock3 :size="16" aria-hidden="true" />正在运行</span><strong>{{ runningTasks.length }}</strong><small>避免重复执行</small></article>
      <article class="monitoring-center-stat-card" :data-tone="attentionCount ? 'warn' : 'good'"><span><ShieldAlert :size="16" aria-hidden="true" />需要处理</span><strong>{{ attentionCount }}</strong><small>{{ attentionCount ? `${health?.failedTasks || 0} 个失败 · ${health?.overdueTasks || 0} 个逾期` : '没有失败或逾期任务' }}</small></article>
      <article class="monitoring-center-stat-card"><span><Video :size="16" aria-hidden="true" />监控范围</span><strong>{{ targetCount }}</strong><small>{{ coverageCount }} 个来源有覆盖记录</small></article>
    </div>

    <div class="monitoring-center-info-grid">
      <section class="monitoring-center-info-card">
        <div class="monitoring-center-info-head"><div><div class="analysis-section-label">调度与覆盖</div><h3>采集链路</h3></div><span class="monitoring-center-live-dot" :data-tone="schedulerTone"></span></div>
        <dl class="monitoring-center-health-list">
          <div><dt>启用任务</dt><dd>{{ health?.enabledTasks ?? enabledTasks.length }}</dd></div>
          <div><dt>逾期任务</dt><dd :data-tone="health?.overdueTasks ? 'warn' : 'good'">{{ health?.overdueTasks || 0 }}</dd></div>
          <div><dt>失败任务</dt><dd :data-tone="health?.failedTasks ? 'risk' : 'good'">{{ health?.failedTasks || 0 }}</dd></div>
          <div><dt>状态读取</dt><dd>{{ formatUpdated(lastUpdated) }}</dd></div>
        </dl>
        <div class="monitoring-center-attention" :data-tone="attentionCount ? 'warn' : 'good'">
          <ShieldAlert :size="16" aria-hidden="true" />
          <div><strong>{{ attentionCount ? `${attentionCount} 个任务需要处理` : tasks.length ? '当前没有异常任务' : '等待创建第一个任务' }}</strong><span>{{ schedulerNote }}</span></div>
        </div>
      </section>
      <section class="monitoring-center-info-card">
        <div class="monitoring-center-info-head"><div><div class="analysis-section-label">最近活动</div><h3>任务运行状态</h3></div><span class="monitoring-center-info-count">{{ recentTasks.length }} 条</span></div>
        <ul v-if="recentTasks.length" class="monitoring-center-recent-list">
          <li v-for="task in recentTasks" :key="task.id">
            <span class="monitoring-center-recent-dot" :data-tone="monitoringStatusTone(task.lastStatus || task.status)"></span>
            <div><strong>{{ task.name }}</strong><span>{{ task.lastStatus ? monitoringRunStatusLabel(task.lastStatus) : monitoringTaskStatusLabel(task.status) }} · {{ formatUpdated(task.lastRunAt || task.updatedAt) }}</span></div>
          </li>
        </ul>
        <p v-else class="monitoring-center-empty">还没有运行记录。创建任务后，运行结果会出现在这里。</p>
      </section>
    </div>

    <MonitoringTasksPanel ref="taskPanel" scope="center" :binding-states="props.bindingStates" @changed="changed" />
  </section>
</template>
