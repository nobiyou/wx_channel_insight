<script setup>
import { computed, onMounted, ref, useId, watch } from "vue";
import {
  NAlert,
  NButton,
  NEmpty,
  NIcon,
  NInput,
  NModal,
  NSelect,
  NSpin,
  NTag,
} from "naive-ui";
import {
  Archive,
  Check,
  Clock3,
  Pause,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  X,
} from "lucide-vue-next";
import PanelFrame from "./PanelFrame.vue";
import { monitoringTasksForScope } from "../lib/operationsNavigation.js";
import {
  archiveMonitoringTask,
  createMonitoringTask,
  fetchAccounts,
  fetchMonitoringTaskRuns,
  fetchMonitoringTasks,
  retryMonitoringTask,
  runMonitoringTask,
  updateMonitoringTask,
} from "../lib/api.js";
import {
  canRetryMonitoringTask,
  DEFAULT_MONITORING_INTERVAL,
  formatMonitoringInterval,
  formatMonitoringTime,
  isMonitoringTaskRunning,
  monitoringRunMessage,
  monitoringRunStatusLabel,
  monitoringIntervalOptions,
  monitoringSourceLabel,
  monitoringStatusTone,
  monitoringTargetLabel,
  monitoringTaskCollectsComments,
  monitoringTaskRunDetail,
  monitoringTaskStatusLabel,
  monitoringTaskTypeLabel,
  monitoringTaskTypeTone,
  normalizeMonitoringIntervalForTask,
  normalizeMonitoringTarget,
  taskTargetCount,
} from "../lib/monitoringTasks.js";

const props = defineProps({
  scope: { type: String, default: "diagnostics" },
  targetKey: { type: String, default: "" },
  targetAliases: { type: Array, default: () => [] },
  initialTaskId: { type: String, default: "" },
  bindingStates: {
    type: Array,
    default: () => [],
  },
});
const emit = defineEmits(["changed"]);
const titleID = useId();
let tasksRequest = 0;
let runsRequest = 0;

const tasks = ref([]);
const selectedTaskID = ref(props.initialTaskId);
const selectedRuns = ref([]);
const runsLoadedForTask = ref("");
const recentRunsByTaskID = ref({});
const loading = ref(false);
const runsLoading = ref(false);
const actionTaskID = ref("");
const errorMessage = ref("");
const noticeMessage = ref("");
const includeArchived = ref(false);
const editorOpen = ref(false);
const editorMode = ref("create");
const editorError = ref("");
const draft = ref(createEmptyDraft());
const activeRunTask = computed(() => tasks.value.find(isMonitoringTaskRunning) || null);
const newTargetValue = ref("");
const accounts = ref([]);
const accountsLoading = ref(false);

function createEmptyDraft() {
  return {
    id: "",
    name: "",
    taskType: props.scope === "keywords" ? "legacy" : "video_list",
    status: "enabled",
    scopeMode: "selected",
    interval: DEFAULT_MONITORING_INTERVAL,
    targets: [],
  };
}

const selectedTask = computed(
  () => tasks.value.find((task) => task.id === selectedTaskID.value) || null,
);

const draftCollectsComments = computed(() => monitoringTaskCollectsComments(draft.value));
const draftIntervalOptions = computed(() => monitoringIntervalOptions(draft.value));
const draftScopeOptions = computed(() => {
  if (draft.value.taskType === "legacy") {
    if (props.scope !== "diagnostics") return [{ label: "只运行选定监控对象", value: "selected" }];
    return [
      { label: "全部已保存的数据源", value: "all_saved_bindings" },
      { label: "只运行选定目标", value: "selected" },
    ];
  }
  if (draft.value.taskType === "account_profile") {
    return [
      { label: "全部账号", value: "all_accounts" },
      { label: "只运行选定账号", value: "selected" },
    ];
  }
  return [{ label: "只运行选定账号", value: "selected" }];
});

const availableTargets = computed(() => {
  const aliases = {
    wx_channel_comment_export: "wx_comment_export",
    wx_channel_feed_list: "wx_feed_list",
    wx_channel_video_search: "wx_video_search",
  };
  const seen = new Set();
  return props.bindingStates
    .map((binding) => {
      const source = aliases[binding?.source] || String(binding?.source || "").trim();
      const targetKey = String(
        binding?.targetKey || binding?.username || binding?.keyword || binding?.path || "",
      ).trim();
      const allowed = props.scope === "keywords" ? ["wx_video_search"] : props.scope === "accounts" ? ["wx_feed_list"] : ["wx_feed_list", "wx_video_search", "wx_comment_export"];
      if (!source || !targetKey || !allowed.includes(source)) {
        return null;
      }
      const key = `${source}\x00${targetKey}`;
      if (seen.has(key)) return null;
      seen.add(key);
      return normalizeMonitoringTarget({
        source,
        targetKey,
        label: monitoringSourceLabel(source),
        displayName:
          binding?.username || binding?.keyword || binding?.path || targetKey,
      });
    })
    .filter(Boolean);
});

const targetOptions = computed(() =>
  draft.value.taskType === "legacy"
    ? availableTargets.value.map((target) => ({
      label: `${monitoringSourceLabel(target.source)} · ${monitoringTargetLabel(target)}`,
      value: `${target.source}\x00${target.targetKey}`,
    }))
    : accounts.value.map((account) => ({
      label: `${account.channelName || account.channelUsername || "未命名账号"} · ${account.channelUsername || account.accountId}`,
      value: String(account.accountId || ""),
    })).filter((item) => item.value),
);

const selectedTargetKeys = computed(() =>
  new Set(
    draft.value.targets.map(
      (target) => `${target.source}\x00${target.targetKey}`,
    ),
  ),
);

const selectedTaskRunSummary = computed(() => {
  if (!selectedTask.value) return "选择一个任务查看最近运行结果";
  return `${selectedTask.value.name} · ${selectedRuns.value.length} 条历史记录`;
});

function taskTargetSummary(task) {
	if (task?.scopeMode === "all_accounts") {
		return "账号资料 · 执行全部账号";
	}
	if (task?.taskType && task.taskType !== "legacy") {
		const targets = Array.isArray(task?.targets) ? task.targets : [];
		return targets.length
			? `${monitoringTaskTypeLabel(task.taskType)} · ${targets.slice(0, 2).map((target) => monitoringTargetLabel(target)).join("、")}${targets.length > 2 ? `、另有 ${targets.length - 2} 个账号` : ""}`
			: `${monitoringTaskTypeLabel(task.taskType)} · 尚未配置账号`;
	}
  if (task?.scopeMode === "all_saved_bindings") {
    return "执行全部已保存的数据源";
  }
  const targets = Array.isArray(task?.targets) ? task.targets : [];
  if (!targets.length) return "尚未配置目标";
  return targets
    .slice(0, 2)
    .map((target) => `${monitoringSourceLabel(target.source)} · ${monitoringTargetLabel(target)}`)
    .concat(targets.length > 2 ? [`另有 ${targets.length - 2} 个目标`] : [])
    .join("、");
}

function taskStatusType(status) {
  return monitoringStatusTone(status) === "risk"
    ? "error"
    : monitoringStatusTone(status) === "warn"
      ? "warning"
      : monitoringStatusTone(status) === "good"
        ? "success"
        : "default";
}

function runStatusType(status) {
  const tone = monitoringStatusTone(status);
  return tone === "risk" ? "error" : tone === "warn" ? "warning" : tone === "good" ? "success" : "default";
}

function clearFeedback() {
  errorMessage.value = "";
  noticeMessage.value = "";
}

function resolveTaskPayload() {
	const taskShape = {
		taskType: draft.value.taskType,
		scopeMode: draft.value.scopeMode,
		targets: draft.value.scopeMode === "selected" ? draft.value.targets : [],
	};
  return {
    name: draft.value.name.trim(),
    taskType: draft.value.taskType,
    status: draft.value.status,
    scopeMode: draft.value.scopeMode,
    interval: normalizeMonitoringIntervalForTask(draft.value.interval, taskShape),
    targets:
      draft.value.scopeMode === "selected"
        ? draft.value.targets.map((target) => ({ ...target }))
        : [],
  };
}

async function loadAccounts() {
  if (accounts.value.length || accountsLoading.value) return;
  accountsLoading.value = true;
  try {
    const payload = await fetchAccounts({ page: 1, pageSize: 100, sort: "updated_at", direction: "desc" });
    accounts.value = Array.isArray(payload?.items) ? payload.items : [];
  } catch (error) {
    editorError.value = error instanceof Error ? error.message : "账户目标加载失败";
    accounts.value = [];
  } finally {
    accountsLoading.value = false;
  }
}

async function loadTasks({ keepSelection = true } = {}) {
  const request = ++tasksRequest;
  loading.value = true;
  try {
    const payload = await fetchMonitoringTasks(includeArchived.value);
    if (request !== tasksRequest) return;
    tasks.value = monitoringTasksForScope(Array.isArray(payload?.items) ? payload.items : [], props.scope, props.targetKey, props.targetAliases);
    const nextSelectedID = keepSelection && tasks.value.some((task) => task.id === selectedTaskID.value)
      ? selectedTaskID.value
      : tasks.value.find((task) => task.id === props.initialTaskId)?.id || tasks.value[0]?.id || "";
    selectedTaskID.value = nextSelectedID;
    recentRunsByTaskID.value = {};
    void loadRecentRunDetails(tasks.value, nextSelectedID, request);
    if (nextSelectedID) {
      await loadRuns(nextSelectedID);
    } else {
      runsRequest += 1;
      runsLoading.value = false;
      selectedRuns.value = [];
      runsLoadedForTask.value = "";
    }
  } catch (error) {
    if (request === tasksRequest) errorMessage.value = error instanceof Error ? error.message : "任务列表加载失败";
  } finally {
    if (request === tasksRequest) loading.value = false;
  }
}

async function loadRuns(taskID = selectedTaskID.value) {
  const request = ++runsRequest;
  runsLoadedForTask.value = "";
  if (!taskID) {
    selectedRuns.value = [];
    return;
  }
  runsLoading.value = true;
  try {
    const payload = await fetchMonitoringTaskRuns(taskID, 20);
    if (request !== runsRequest) return;
    selectedRuns.value = Array.isArray(payload?.items) ? payload.items : [];
    runsLoadedForTask.value = taskID;
  } catch (error) {
    if (request !== runsRequest) return;
    errorMessage.value = error instanceof Error ? error.message : "运行历史加载失败";
    selectedRuns.value = [];
  } finally {
    if (request === runsRequest) runsLoading.value = false;
  }
}

async function loadRecentRunDetails(taskItems, selectedID, request) {
  const candidates = taskItems.filter((task) =>
    task.id !== selectedID &&
    !String(task.lastError || "").trim() &&
    ["partial", "failed", "abandoned"].includes(String(task.lastStatus || "").trim()),
  );
  const entries = await Promise.all(candidates.map(async (task) => {
    try {
      const payload = await fetchMonitoringTaskRuns(task.id, 1);
      return [task.id, Array.isArray(payload?.items) ? payload.items[0] || null : null];
    } catch {
      return [task.id, null];
    }
  }));
  if (request !== tasksRequest) return;
  recentRunsByTaskID.value = Object.fromEntries(entries.filter(([, run]) => run));
}

async function selectTask(taskID) {
  selectedTaskID.value = taskID;
  await loadRuns(taskID);
}

function taskRunDetail(task) {
  const run = runsLoadedForTask.value === task.id ? selectedRuns.value[0] : recentRunsByTaskID.value[task.id];
  return monitoringTaskRunDetail(task, run);
}

function openCreateEditor() {
  editorMode.value = "create";
  draft.value = createEmptyDraft();
  newTargetValue.value = "";
  editorError.value = "";
  editorOpen.value = true;
  loadAccounts();
}

function openEditEditor(task) {
	if (task?.system) return;
	editorMode.value = "edit";
  draft.value = {
    id: task.id,
    name: task.name || "",
    taskType: task.taskType || "legacy",
    status: task.status || "enabled",
    scopeMode: task.scopeMode || "all_saved_bindings",
    interval: normalizeMonitoringIntervalForTask(task.interval, task),
    targets: (task.targets || []).map(normalizeMonitoringTarget),
  };
  newTargetValue.value = "";
  editorError.value = "";
  editorOpen.value = true;
  if (draft.value.taskType !== "legacy") loadAccounts();
}

function onTaskTypeChange(value) {
	if (value === "account_profile") {
		draft.value.scopeMode = "all_accounts";
		draft.value.targets = draft.value.targets.filter((target) => target.targetKind === "account");
		loadAccounts();
	} else if (value !== "legacy") {
		draft.value.scopeMode = "selected";
		draft.value.targets = draft.value.targets.filter((target) => target.targetKind === "account");
		loadAccounts();
	}
}

function addDraftTarget() {
  const value = newTargetValue.value;
  if (!value || selectedTargetKeys.value.has(value)) return;
  const target = draft.value.taskType === "legacy"
    ? availableTargets.value.find((item) => `${item.source}\x00${item.targetKey}` === value)
    : accounts.value.find((item) => String(item.accountId || "") === value);
  if (!target) return;
  draft.value.targets = [...draft.value.targets, draft.value.taskType === "legacy"
    ? { ...target }
    : { source: "account", targetKey: String(target.accountId), targetKind: "account", accountId: String(target.accountId), label: "账号", displayName: target.channelName || target.channelUsername || target.accountId }];
  newTargetValue.value = "";
}

function removeDraftTarget(target) {
  draft.value.targets = draft.value.targets.filter(
    (item) => draft.value.taskType === "legacy"
      ? !(item.source === target.source && item.targetKey === target.targetKey)
      : item.accountId !== target.accountId,
  );
}

async function saveDraft() {
  editorError.value = "";
  const payload = resolveTaskPayload();
  if (!payload.name) {
    editorError.value = "请填写任务名称";
    return;
  }
  if (payload.scopeMode === "selected" && !payload.targets.length) {
    editorError.value = draft.value.taskType === "legacy" ? "选定目标任务至少需要一个已保存的数据源目标" : "请选择至少一个账号";
    return;
  }
  actionTaskID.value = draft.value.id || "new";
  try {
    const saved = editorMode.value === "create"
      ? await createMonitoringTask(payload)
      : await updateMonitoringTask(draft.value.id, payload);
    editorOpen.value = false;
    noticeMessage.value = editorMode.value === "create" ? "监控任务已创建" : "监控任务已更新";
    selectedTaskID.value = saved?.id || draft.value.id || selectedTaskID.value;
    await loadTasks();
    emit("changed");
  } catch (error) {
    editorError.value = error instanceof Error ? error.message : "任务保存失败";
  } finally {
    actionTaskID.value = "";
  }
}

async function runTask(task, mode = "run") {
  if (!task?.id) return;
  clearFeedback();
  if (isMonitoringTaskRunning(task)) {
    selectedTaskID.value = task.id;
    noticeMessage.value = `${task.name} 正在执行中，请等待本次运行完成`;
    return;
  }
  if (activeRunTask.value) {
    noticeMessage.value = `已有任务“${activeRunTask.value.name}”正在执行，请等待完成后再操作`;
    return;
  }
  actionTaskID.value = task.id;
  try {
    const payload = mode === "retry"
      ? await retryMonitoringTask(task.id)
      : await runMonitoringTask(task.id);
    const run = payload?.run;
    noticeMessage.value = run
      ? `${task.name} 已完成${mode === "retry" ? "重试" : "执行"}：${monitoringRunStatusLabel(run.status)}`
      : `${task.name} 已提交执行`;
    selectedTaskID.value = task.id;
    await loadTasks();
    emit("changed");
  } catch (error) {
    errorMessage.value = monitoringRunMessage(
      error instanceof Error ? error.message : "",
      "任务执行失败",
    );
  } finally {
    actionTaskID.value = "";
  }
}

async function toggleTask(task) {
	if (!task?.id || task.system) return;
  clearFeedback();
  actionTaskID.value = task.id;
  try {
    await updateMonitoringTask(task.id, {
      status: task.status === "enabled" ? "paused" : "enabled",
    });
    noticeMessage.value = task.status === "enabled" ? "任务已暂停" : "任务已恢复运行";
    await loadTasks();
    emit("changed");
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "任务状态更新失败";
  } finally {
    actionTaskID.value = "";
  }
}

async function archiveTask(task) {
	if (!task?.id || task.system || !window.confirm(`确认归档“${task.name}”吗？历史记录会保留。`)) return;
  clearFeedback();
  actionTaskID.value = task.id;
  try {
    await archiveMonitoringTask(task.id);
    noticeMessage.value = "任务已归档，历史记录仍会保留";
    await loadTasks({ keepSelection: false });
    emit("changed");
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "任务归档失败";
  } finally {
    actionTaskID.value = "";
  }
}

function formatRunCounts(run) {
  const counts = run?.counts || {};
  return `目标 ${counts.attempted ?? 0} · 成功 ${counts.succeeded ?? 0} · 失败 ${counts.failed ?? 0} · 跳过 ${counts.skipped ?? 0}`;
}

function runTriggerLabel(trigger) {
  return { scheduled: "定时", manual: "手动", retry: "重试" }[trigger] || "执行";
}

watch(includeArchived, () => loadTasks({ keepSelection: false }));
watch(() => [props.scope, props.targetKey], () => loadTasks({ keepSelection: false }));
watch(() => props.initialTaskId, (id) => { if (id && tasks.value.some(task => task.id === id)) selectTask(id); });

watch(
  () => [draft.value.scopeMode, draft.value.targets.map((target) => `${target.targetKind}:${target.source}:${target.accountId}`).join("|")],
  () => {
    draft.value.interval = normalizeMonitoringIntervalForTask(draft.value.interval, draft.value);
  },
);

onMounted(() => loadTasks({ keepSelection: false }));
defineExpose({ refresh: loadTasks });
</script>

<template>
  <section class="monitoring-tasks-page" :aria-labelledby="titleID">
    <div class="monitoring-task-intro">
      <div>
        <div class="panel-eyebrow">采集计划与状态</div>
        <h2 :id="titleID">{{ scope === 'keywords' ? '关键词采集设置' : scope === 'accounts' ? '账号采集设置' : '采集运行诊断' }}</h2>
        <p>暂停、恢复和重试保留已有数据与运行历史。</p>
      </div>
      <div class="monitoring-task-intro-actions">
        <n-button secondary :loading="loading" @click="loadTasks()">
          <template #icon><n-icon><RefreshCw /></n-icon></template>
          刷新任务
        </n-button>
        <n-button type="primary" @click="openCreateEditor">
          <template #icon><n-icon><Plus /></n-icon></template>
          新建任务
        </n-button>
      </div>
    </div>

    <n-alert v-if="noticeMessage" type="success" show-icon closable @close="noticeMessage = ''">
      {{ noticeMessage }}
    </n-alert>
    <n-alert v-if="errorMessage" type="error" show-icon closable @close="errorMessage = ''">
      {{ errorMessage }}
    </n-alert>

    <div class="monitoring-task-layout">
      <PanelFrame title="监控任务" eyebrow="已保存计划" :action="`${tasks.length} 个任务`">
        <div class="monitoring-task-toolbar">
          <label class="monitoring-archived-toggle">
            <input v-model="includeArchived" type="checkbox" />
            <span>显示已归档</span>
          </label>
          <span class="monitoring-task-hint">定时器每轮只领取到期任务，避免重复执行。</span>
        </div>

        <div v-if="loading && !tasks.length" class="monitoring-task-loading">
          <n-spin size="small" />
          <span>正在读取任务…</span>
        </div>
        <div v-else-if="tasks.length" class="monitoring-task-grid">
          <article
            v-for="task in tasks"
            :key="task.id"
            class="monitoring-task-card"
            :class="{ selected: selectedTaskID === task.id }"
            @click="selectTask(task.id)"
          >
            <div class="monitoring-task-card-main">
              <div class="monitoring-task-card-head">
                <div class="monitoring-task-card-title">
                  <span class="monitoring-task-icon" :data-tone="monitoringStatusTone(task.status)">
                    <n-icon><Clock3 /></n-icon>
                  </span>
                  <div>
                    <h3><button class="monitoring-task-select" type="button" :aria-pressed="selectedTaskID === task.id" @click.stop="selectTask(task.id)">{{ task.name }}</button></h3>
                    <p>{{ task.system ? "系统默认任务" : "自定义任务" }}</p>
                  </div>
                </div>
                <div class="monitoring-task-badges">
                  <n-tag size="small" :type="taskStatusType(task.status)" :bordered="false">
                    {{ monitoringTaskStatusLabel(task.status) }}
                  </n-tag>
                  <n-tag v-if="task.system" size="small" type="info" :bordered="false">只读</n-tag>
                </div>
              </div>

              <div class="monitoring-task-type-line" :data-task-type="monitoringTaskTypeTone(task.taskType)">
                <span>{{ monitoringTaskTypeLabel(task.taskType) }}</span>
                <span v-if="task.system">每 30 分钟执行全部已保存且可运行的数据源</span>
                <span v-else-if="task.taskType === 'comment_collection'">使用已保存视频，不刷新视频列表</span>
                <span v-else-if="task.taskType === 'video_list'">只同步视频列表，不采集评论</span>
                <span v-else-if="task.taskType === 'account_profile'">只同步账号资料</span>
              </div>

              <p class="monitoring-task-targets" :title="taskTargetSummary(task)">
                {{ taskTargetSummary(task) }}
              </p>
            </div>
            <div class="monitoring-task-facts">
              <div><span>周期</span><strong>{{ formatMonitoringInterval(task.interval) }}</strong></div>
              <div><span>范围</span><strong>{{ taskTargetCount(task) }}</strong></div>
              <div><span>下次运行</span><strong>{{ task.status === 'paused' ? '恢复后按周期采集' : task.status === 'archived' ? '已停止采集' : formatMonitoringTime(task.nextRunAt) }}</strong></div>
            </div>
            <div class="monitoring-task-card-status">
              <div class="monitoring-task-last-run">
                <span>最近一次</span>
                <n-tag v-if="task.lastStatus" size="small" :type="runStatusType(task.lastStatus)" :bordered="false">
                  {{ monitoringRunStatusLabel(task.lastStatus) }}
                </n-tag>
                <span v-else>尚未运行</span>
                <span v-if="task.lastRunAt">· {{ formatMonitoringTime(task.lastRunAt) }}</span>
              </div>
              <p v-if="taskRunDetail(task)" class="monitoring-task-run-detail" :data-status="task.lastStatus">
                <strong>{{ task.lastStatus === 'partial' ? '部分完成说明' : '执行说明' }}</strong>
                <span>{{ taskRunDetail(task) }}</span>
              </p>
            </div>
            <div class="monitoring-task-actions" @click.stop>
              <n-button size="small" type="primary" :loading="actionTaskID === task.id" :disabled="task.status === 'archived' || Boolean(activeRunTask) || Boolean(actionTaskID)" :title="isMonitoringTaskRunning(task) ? '本次运行尚未完成' : activeRunTask ? '已有其他监控任务正在执行' : '立即运行'" @click="runTask(task)">
                <template #icon><n-icon><Play /></n-icon></template>
                {{ isMonitoringTaskRunning(task) ? "执行中" : activeRunTask ? "等待中" : "立即运行" }}
              </n-button>
              <n-button v-if="canRetryMonitoringTask(task)" size="small" secondary :loading="actionTaskID === task.id" @click="runTask(task, 'retry')">
                <template #icon><n-icon><RotateCcw /></n-icon></template>
                重试
              </n-button>
              <n-button size="small" secondary :disabled="task.system || task.status === 'archived' || Boolean(actionTaskID)" :loading="actionTaskID === task.id" @click="toggleTask(task)">
                <template #icon><n-icon><Pause v-if="task.status === 'enabled'" /><Play v-else /></n-icon></template>
                {{ task.status === "enabled" ? "暂停" : "恢复" }}
              </n-button>
              <n-button size="small" quaternary :disabled="task.status === 'archived' || task.system" :title="task.system ? '默认监控为系统只读任务' : '编辑任务'" aria-label="编辑任务" @click="openEditEditor(task)">
                <template #icon><n-icon><Pencil /></n-icon></template>
                编辑
              </n-button>
              <n-button size="small" quaternary title="归档任务" aria-label="归档任务" :disabled="task.status === 'archived' || task.system" @click="archiveTask(task)">
                <template #icon><n-icon><Archive /></n-icon></template>
                归档
              </n-button>
            </div>
            <div v-if="task.system" class="monitoring-default-task-guide">
              <div class="monitoring-default-task-guide-head">
                <n-icon><Check /></n-icon>
                <strong>默认监控范围（系统只读）</strong>
              </div>
              <p>每 30 分钟检查可运行的已保存来源。已配置的账号和关键词按各自任务采集；暂停的对象不会由默认任务代采。</p>
              <ul>
                <li><strong>评论导出</strong>：刷新已保存评论导出中的评论数据。</li>
                <li><strong>视频列表</strong>：同步已保存账号的视频列表和指标。</li>
                <li><strong>关键词视频</strong>：同步已保存关键词对应的视频数据。</li>
                <li><strong>视频分享</strong>：刷新已保存分享链接对应的视频数据。</li>
              </ul>
              <p class="monitoring-default-task-guide-note">账号资料、账号视频列表、账号评论采集属于独立任务，需要通过“新建任务”单独配置。</p>
            </div>
          </article>
        </div>
        <n-empty v-else description="还没有监控任务，请先新建一个任务" />
      </PanelFrame>

      <PanelFrame title="运行历史" eyebrow="执行记录" :action="selectedTaskRunSummary">
        <div v-if="selectedTask && runsLoading" class="monitoring-task-loading">
          <n-spin size="small" />
          <span>正在读取运行历史…</span>
        </div>
        <div v-else-if="selectedTask && selectedRuns.length" class="monitoring-run-list">
          <article v-for="run in selectedRuns" :key="run.id" class="monitoring-run-row">
            <div class="monitoring-run-head">
              <div>
                <strong>{{ runTriggerLabel(run.trigger) }} · 第 {{ run.attempt || 1 }} 次</strong>
                <span>{{ formatMonitoringTime(run.startedAt) }}</span>
              </div>
              <n-tag size="small" :type="runStatusType(run.status)" :bordered="false">
                {{ monitoringRunStatusLabel(run.status) }}
              </n-tag>
            </div>
            <div class="monitoring-run-counts">{{ formatRunCounts(run) }}</div>
            <p v-if="run.message" class="monitoring-run-message">{{ monitoringRunMessage(run.message) }}</p>
            <div v-if="run.operatorRunId" class="monitoring-run-id">Operator {{ run.operatorRunId }}</div>
            <div v-if="run.errors?.length" class="monitoring-run-errors">
              <span v-for="item in run.errors.slice(0, 3)" :key="`${run.id}-${item.source}-${item.targetKey}`">
                {{ monitoringSourceLabel(item.source) }} · {{ item.targetKey || "目标" }}：{{ item.message }}
              </span>
            </div>
          </article>
        </div>
        <n-empty v-else :description="selectedTask ? '这个任务还没有运行记录' : '先选择一个任务查看历史'" />
      </PanelFrame>
    </div>

    <n-modal v-model:show="editorOpen" preset="card" :title="editorMode === 'create' ? '新建监控任务' : '编辑监控任务'" style="width: min(620px, calc(100vw - 32px))">
      <div class="monitoring-task-form">
        <n-alert v-if="editorError" type="error" show-icon>
          {{ editorError }}
        </n-alert>
        <label class="form-field">
          <span>任务名称</span>
          <n-input v-model:value="draft.name" placeholder="例如：重点账号每 30 分钟" maxlength="60" />
        </label>
        <div class="monitoring-task-form-grid">
          <label class="form-field">
            <span>运行周期</span>
            <n-select
              v-model:value="draft.interval"
              :options="draftIntervalOptions"
            />
          </label>
          <label class="form-field">
            <span>任务状态</span>
            <n-select
              v-model:value="draft.status"
              :options="[
                { label: '创建后立即运行', value: 'enabled' },
                { label: '先创建，暂不运行', value: 'paused' },
              ]"
            />
          </label>
        </div>
        <label class="form-field">
          <span>任务类型</span>
          <n-select
            v-model:value="draft.taskType"
            :disabled="editorMode === 'edit' && draft.taskType === 'legacy'"
            :options="[
              ...(scope !== 'keywords' ? [{ label: '账号资料同步', value: 'account_profile' }, { label: '视频列表同步', value: 'video_list' }] : []),
              ...(scope === 'diagnostics' ? [{ label: '评论补采（辅助数据）', value: 'comment_collection' }] : []),
              ...(scope === 'keywords' ? [{ label: '关键词视频采集', value: 'legacy' }] : []),
              ...(editorMode === 'edit' && draft.taskType === 'legacy' ? [{ label: '兼容任务（历史）', value: 'legacy' }] : []),
            ]"
            @update:value="onTaskTypeChange"
          />
        </label>
        <label class="form-field">
          <span>监控范围</span>
            <n-select
              v-model:value="draft.scopeMode"
              :disabled="draftScopeOptions.length === 1"
              :options="draftScopeOptions"
            />
          </label>
          <div v-if="draft.scopeMode === 'all_accounts'" class="monitoring-scope-note">
            执行时读取当前账号主数据中的全部账号，只同步账号资料，不读取视频列表或评论。
          </div>
        <div v-if="draft.scopeMode === 'selected'" class="monitoring-target-picker">
          <div class="monitoring-target-picker-head">
            <span>{{ draft.taskType === 'legacy' ? '选择已保存目标' : '选择账号' }}</span>
            <span>{{ draft.targets.length }} 个</span>
          </div>
          <div v-if="targetOptions.length" class="monitoring-target-add">
            <n-select v-model:value="newTargetValue" :loading="accountsLoading" :options="targetOptions" :placeholder="draft.taskType === 'legacy' ? '选择数据源目标' : '选择账号'" />
            <n-button secondary :disabled="!newTargetValue" title="添加目标" aria-label="添加目标" @click="addDraftTarget">
              <template #icon><n-icon><Plus /></n-icon></template>
            </n-button>
          </div>
          <div v-else class="monitoring-target-empty">
            {{ draft.taskType === 'legacy' ? '请先在当前页面添加监控对象。' : '请先搜索账号并添加监控，采集后会建立账号主数据。' }}
          </div>
          <div v-if="draft.targets.length" class="monitoring-selected-targets">
            <div v-for="target in draft.targets" :key="`${target.source}-${target.targetKey}`" class="monitoring-selected-target">
              <span>{{ draft.taskType === 'legacy' ? `${monitoringSourceLabel(target.source)} · ${monitoringTargetLabel(target)}` : monitoringTargetLabel(target) }}</span>
              <button type="button" title="移除目标" aria-label="移除目标" @click="removeDraftTarget(target)">
                <n-icon><X /></n-icon>
              </button>
            </div>
          </div>
        </div>
        <p class="monitoring-form-note">
          {{ draft.taskType === 'account_profile'
            ? "只刷新账号资料，不读取视频列表，也不采集评论。建议串行或低频执行。"
            : draft.taskType === 'video_list'
              ? "只同步账号的视频列表，不采集评论；评论请单独创建评论采集任务。"
              : draft.taskType === 'comment_collection'
                ? "使用数据库中已经保存的视频，不重新刷新视频列表；评论采集最短 30 分钟，每轮最多处理 3 个视频。"
                : draftCollectsComments
                  ? "评论采集采用安全节奏：最短 30 分钟，每轮最多处理 3 个变化视频；命中平台频控后自动冷却。"
                  : "系统会把运行结果和失败原因写入任务历史；暂停任务不会删除历史。" }}
        </p>
        <div class="monitoring-task-form-actions">
          <n-button secondary @click="editorOpen = false">取消</n-button>
          <n-button type="primary" :loading="actionTaskID === (draft.id || 'new')" @click="saveDraft">
            <template #icon><n-icon><Check /></n-icon></template>
            {{ editorMode === "create" ? "创建任务" : "保存修改" }}
          </n-button>
        </div>
      </div>
    </n-modal>
  </section>
</template>
