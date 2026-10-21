<script setup>
import { ref } from "vue";
import { NAlert, NSpin } from "naive-ui";
import { Search, Plus } from "lucide-vue-next";
import { createMonitoringTask, fetchMonitoringTasks, updateMonitoringTask, runMonitoringTask, saveWXFeedListBinding, saveWXVideoSearchBinding, searchWXFeedAccounts } from "../lib/api.js";
import { findMonitoringObjectTask, MONITORING_INTERVAL_OPTIONS, monitoringRunStatusLabel } from "../lib/monitoringTasks.js";

const props = defineProps({ mode: { type: String, required: true }, baseUrl: { type: String, default: "" }, clientId: { type: String, default: "" } });
const emit = defineEmits(["saved"]);
const input = ref("");
const interval = ref("10m");
const candidates = ref([]);
const busy = ref(false);
const error = ref("");
const notice = ref("");

async function search() {
  if (!input.value.trim() || busy.value) return;
  busy.value = true; error.value = ""; notice.value = "";
  candidates.value = [];
  try {
    const payload = await searchWXFeedAccounts(props.baseUrl, input.value.trim(), props.clientId);
    candidates.value = (Array.isArray(payload?.items) ? payload.items : []).filter((item) => item.username);
    if (!candidates.value.length) notice.value = "没有找到账号，请检查账号 ID、名称或来源连接。";
  } catch (cause) { error.value = `搜索账号失败：${cause instanceof Error ? cause.message : cause}`; }
  finally { busy.value = false; }
}

async function save(target = input.value.trim(), runNow = false, displayName = target) {
  const key = String(target || "").trim();
  if (!key || busy.value) return;
  busy.value = true; error.value = ""; notice.value = "";
  let saved = false;
  try {
    const source = props.mode === "keywords" ? "wx_video_search" : "wx_feed_list";
    const existing = findMonitoringObjectTask((await fetchMonitoringTasks()).items, source, key);
    if (props.mode === "keywords") await saveWXVideoSearchBinding(props.baseUrl, key);
    else await saveWXFeedListBinding(props.baseUrl, key);
    const task = existing
      ? await updateMonitoringTask(existing.id, { interval: interval.value })
      : await createMonitoringTask({ name: `${props.mode === "keywords" ? "关键词" : "账号"} · ${displayName}`, taskType: "legacy", status: "enabled", scopeMode: "selected", interval: interval.value, targets: [{ source, targetKey: key, displayName }] });
    saved = true;
    emit("saved", { key, task });
    if (runNow) {
      const result = await runMonitoringTask(task.id);
      notice.value = `已保存监控，本次采集：${monitoringRunStatusLabel(result?.run?.status || "running")}。`;
      emit("saved", { key, task });
    } else notice.value = task.status === "paused" ? "已更新监控周期，任务仍处于暂停状态。可在下方恢复或立即采集。" : "已保存监控周期，可在下方暂停、恢复或立即采集。";
  } catch (cause) { error.value = `${saved ? "监控已保存，采集失败；可用原参数重试" : "保存监控失败"}：${cause instanceof Error ? cause.message : cause}`; }
  finally { busy.value = false; }
}
</script>

<template>
  <section class="analysis-surface operations-object-form" :aria-label="mode === 'keywords' ? '添加关键词监控' : '添加账号监控'">
    <div class="analysis-surface-head"><div><div class="analysis-section-label">持续采集</div><h3>{{ mode === 'keywords' ? '添加关键词监控' : '添加账号监控' }}</h3><p>{{ mode === 'keywords' ? '保存搜索关键词，持续采集该关键词实际返回的视频样本。' : '搜索并核对真实账号 ID，或直接填写完整的 @finder username。' }}</p></div></div>
    <n-alert v-if="error" type="error" show-icon>{{ error }}</n-alert>
    <n-alert v-if="notice" type="info" show-icon>{{ notice }}</n-alert>
    <form class="operations-filter-form" @submit.prevent="mode === 'keywords' ? save(input, false) : search()">
      <label><span>{{ mode === 'keywords' ? '搜索关键词' : '账号 ID 或名称' }}</span><input v-model="input" required :placeholder="mode === 'keywords' ? '如 家居收纳' : '@finder username 或账号名称'" /></label>
      <label><span>采集周期</span><select v-model="interval"><option v-for="option in MONITORING_INTERVAL_OPTIONS" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <button class="tool-button primary" type="submit" :disabled="busy || !input.trim()"><Search v-if="mode === 'accounts'" :size="15" /><Plus v-else :size="15" />{{ mode === 'keywords' ? '保存监控' : '搜索账号' }}</button>
      <button v-if="mode === 'keywords' || input.includes('@finder')" class="tool-button secondary" type="button" :disabled="busy || !input.trim()" @click="save(input, true)">保存并立即采集</button>
      <n-spin v-if="busy" size="small" />
    </form>
    <div v-if="mode === 'accounts' && candidates.length" class="analysis-table-wrap"><table class="analysis-table"><thead><tr><th>账号</th><th>真实账号 ID</th><th>操作</th></tr></thead><tbody><tr v-for="item in candidates" :key="item.username"><td>{{ item.nickname || item.username }}</td><td>{{ item.username }}</td><td><button type="button" class="tool-button primary" :disabled="busy" @click="save(item.username, true, item.nickname || item.username)">添加监控并采集</button></td></tr></tbody></table></div>
  </section>
</template>
