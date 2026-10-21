<script setup>
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { NAlert } from "naive-ui";
import MonitoringTasksPanel from "./MonitoringTasksPanel.vue";
import AIServiceSettingsPanel from "./AIServiceSettingsPanel.vue";
import PromptSettingsPanel from "./PromptSettingsPanel.vue";
import { fetchMonitoringHealth, importWXCommentExport } from "../lib/api.js";
import { buildOperationsHash } from "../lib/operationsNavigation.js";
import { monitoringRunStatusLabel, monitoringSourceLabel } from "../lib/monitoringTasks.js";
const props = defineProps({ initialParams: { type: Object, default: () => ({}) }, bindingStates: { type: Array, default: () => [] }, runtime: { type: Object, default: null }, edgeNodes: { type: Array, default: () => [] }, baseUrl: { type: String, default: "" }, clientId: { type: String, default: "" } });
const emit = defineEmits(["update:baseUrl", "update:clientId", "refresh"]);
const tab = ref("sources"); const commentPath = ref(""); const error = ref(""); const notice = ref(""); const busy = ref(false);
const tasks = ref(null);
const health = ref(null);
const healthError = ref("");
const healthBusy = ref(false);
const schedulerLabel = computed(() => ({ unknown: "未收到调度心跳", offline: "调度已离线", online: "调度在线", running: "调度正在执行任务" }[health.value?.schedulerStatus] || "正在读取调度状态…"));
const healthDate = (value) => value && !String(value).startsWith("0001-") ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "暂无记录";
async function loadHealth() {
  if (healthBusy.value) return;
  healthBusy.value = true;
  try { health.value = await fetchMonitoringHealth(); healthError.value = ""; }
  catch (cause) { healthError.value = `运行诊断更新失败：${cause instanceof Error ? cause.message : cause}。请重试。`; }
  finally { healthBusy.value = false; }
}
watch(tab, (value) => { if (value === "runs") void loadHealth(); }, { immediate: true });
const healthTimer = setInterval(() => { if (tab.value === "runs") void loadHealth(); }, 30000);
onBeforeUnmount(() => clearInterval(healthTimer));
watch(() => props.initialParams, (params) => { tab.value = ["sources", "runs", "ai"].includes(params.tab) ? params.tab : "sources"; }, { immediate: true });
function selectTab(value) { tab.value = value; window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}${buildOperationsHash("settings", { tab: value, task: value === "runs" ? props.initialParams.task : "" })}`); }
async function importComments() { if (!commentPath.value.trim() || busy.value) return; busy.value = true; error.value = ""; notice.value = ""; try { const payload = await importWXCommentExport(commentPath.value.trim()); notice.value = `评论事实导入：${monitoringRunStatusLabel(payload.status || 'running')}`; emit("refresh"); } catch (cause) { error.value = `评论导入失败：${cause instanceof Error ? cause.message : cause}`; } finally { busy.value = false; } }
defineExpose({ refresh: () => tasks.value?.refresh() });
</script>
<template>
  <section class="analysis-page">
    <div class="analysis-page-head"><div><div class="analysis-page-kicker">辅助设置</div><h2>来源连接与运行诊断</h2><p>三个核心流程使用采集事实和已有分析。AI 数据解释可选，未配置时监控与视频分析仍可用。</p></div><button type="button" class="tool-button secondary" @click="emit('refresh')">刷新连接状态</button></div>
    <div class="analysis-tabs" aria-label="辅助设置分类"><button v-for="item in [{ key: 'sources', label: '来源连接' }, { key: 'runs', label: '运行诊断' }, { key: 'ai', label: '可选 AI 配置' }]" :key="item.key" type="button" class="analysis-tab" :class="{ active: tab === item.key }" :aria-pressed="tab === item.key" @click="selectTab(item.key)">{{ item.label }}</button></div>
    <div v-if="tab === 'sources'" class="operations-core-page">
      <section class="analysis-surface operations-object-form"><h3>采集连接</h3><p>在线边缘节点优先使用已有连接；没有边缘节点时填写当前采集器 HTTP 地址。</p><div class="operations-filter-form"><label><span>采集器地址（边缘模式可留空）</span><input :value="baseUrl" type="url" placeholder="http://127.0.0.1:2026" @input="emit('update:baseUrl', $event.target.value)" /></label><label><span>指定边缘节点</span><select :value="clientId" @change="emit('update:clientId', $event.target.value)"><option value="">自动选择可用节点</option><option v-for="node in edgeNodes" :key="node.clientId" :value="node.clientId">{{ node.name || node.clientId }} · {{ node.status || '已连接' }}</option></select></label></div></section>
      <section class="analysis-surface operations-object-form"><h3>已保存来源</h3><p v-if="!bindingStates.length">没有来源绑定，请在账号或关键词页面添加监控。</p><div v-else class="analysis-table-wrap"><table class="analysis-table"><thead><tr><th>采集类型</th><th>对象</th><th>采集器</th></tr></thead><tbody><tr v-for="binding in bindingStates" :key="`${binding.source}-${binding.targetKey || binding.username || binding.keyword || binding.path}`"><td>{{ monitoringSourceLabel(binding.source) }}</td><td>{{ binding.username || binding.keyword || binding.path || binding.targetKey }}</td><td>{{ binding.baseURL || binding.baseUrl || '边缘连接 / 服务端配置' }}</td></tr></tbody></table></div></section>
      <section class="analysis-surface operations-object-form"><h3>辅助导入原始评论</h3><p>评论补采服务于视频分析。此处导入已有导出文件，并保留原视频 ID。</p><n-alert v-if="error" type="error" show-icon>{{ error }}</n-alert><n-alert v-if="notice" type="info" show-icon>{{ notice }}</n-alert><form class="operations-filter-form" @submit.prevent="importComments"><label><span>服务端可读取的评论 JSON 路径</span><input v-model="commentPath" required placeholder="评论导出文件路径" /></label><button class="tool-button primary" type="submit" :disabled="busy">导入评论事实</button></form></section>
    </div>
    <div v-else-if="tab === 'runs'" class="operations-core-page">
      <section class="analysis-surface operations-object-form" aria-labelledby="scheduler-health-title">
        <div class="analysis-page-head"><div><h3 id="scheduler-health-title">运行状态</h3><p>{{ schedulerLabel }} · 最近心跳 {{ healthDate(health?.lastHeartbeatAt) }}</p></div><button class="tool-button secondary" type="button" :disabled="healthBusy" @click="loadHealth">刷新诊断</button></div>
        <n-alert v-if="healthError" type="error" show-icon>{{ healthError }}</n-alert>
        <n-alert v-if="health && ['unknown', 'offline'].includes(health.schedulerStatus)" type="warning" show-icon>自动采集尚未确认运行。请启动或检查调度服务；API 可连接并不代表任务会按时执行。</n-alert>
        <div v-if="health" class="analysis-stat-strip"><div><span>启用任务</span><strong>{{ health.enabledTasks }}</strong></div><div><span>到期未执行</span><strong>{{ health.overdueTasks }}</strong></div><div><span>数据待更新</span><strong>{{ health.staleTasks }}</strong></div><div><span>最近执行异常</span><strong>{{ health.failedTasks }}</strong></div></div>
        <p>数据待更新：启用任务尚未成功采集，或最近成功时间已超过两个采集周期。暂停任务不计入。</p>
      </section>
      <section class="analysis-surface operations-object-form" aria-labelledby="collection-coverage-title"><h3 id="collection-coverage-title">采集覆盖范围</h3><p>仅表示上游本轮返回的分页范围；关键词搜索样本不代表全站视频。较早的记录未保存分页状态，覆盖范围未知。</p><p v-if="!health?.coverage?.length">暂无分页覆盖记录，下次采集后更新。</p><div v-else class="analysis-table-wrap"><table class="analysis-table"><thead><tr><th>对象</th><th>覆盖状态</th><th>已采集页数 / 条目</th><th>最近更新</th></tr></thead><tbody><tr v-for="item in health.coverage" :key="`${item.source}-${item.targetKey}`"><td class="health-target">{{ item.targetKey }}</td><td><strong>{{ item.complete ? '本轮分页已完成' : '待续采' }}</strong><p v-if="item.notice">{{ item.notice }}</p></td><td>{{ item.pageCount }} 页 / {{ item.videoCount }} 条目</td><td>{{ healthDate(item.updatedAt) }}</td></tr></tbody></table></div></section>
      <MonitoringTasksPanel ref="tasks" scope="diagnostics" :initial-task-id="initialParams.task || ''" :binding-states="bindingStates" @changed="emit('refresh'); loadHealth()" />
    </div>
    <div v-else class="operations-core-page"><AIServiceSettingsPanel /><PromptSettingsPanel /></div>
  </section>
</template>
<style scoped>
.health-target { max-width: 280px; overflow-wrap: anywhere; }
</style>
