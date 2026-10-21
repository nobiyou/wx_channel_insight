<script setup>
import { computed, ref, watch } from "vue";
import MonitoringObjectForm from "./MonitoringObjectForm.vue";
import MonitoringTasksPanel from "./MonitoringTasksPanel.vue";
import VideoAnalysisList from "./VideoAnalysisList.vue";
import OperationsPortfolio from "./OperationsPortfolio.vue";
import { buildOperationsHash } from "../lib/operationsNavigation.js";
import { discoveryPreset } from "../lib/videoWorkbench.js";
const props = defineProps({ bindingStates: { type: Array, default: () => [] }, initialParams: { type: Object, default: () => ({}) }, baseUrl: { type: String, default: "" }, clientId: { type: String, default: "" } });
const emit = defineEmits(["refresh", "open-video", "open-account", "analyze"]);
const selected = ref(props.initialParams.keyword || "");
const tasks = ref(null); const list = ref(null);
const portfolio = ref(null);
const keywords = computed(() => [...new Set(props.bindingStates.filter((binding) => ["wx_video_search", "wx_channel_video_search"].includes(binding.source)).map((binding) => binding.keyword || binding.targetKey).filter(Boolean))]);
watch(keywords, (values) => { if (!selected.value && values.length) selected.value = values[0]; }, { immediate: true });
watch(() => props.initialParams.keyword, (value) => { if (value) selected.value = value; });
function select() { window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}${buildOperationsHash("keywords", { keyword: selected.value })}`); }
async function refresh(options) { await Promise.allSettled([tasks.value?.refresh(), list.value?.refresh(options), portfolio.value?.refresh()]); }
function analyzeKeyword(filters) { if(filters.monitorKeyword) { selected.value = filters.monitorKeyword; select(); } }
defineExpose({ refresh });
async function saved({ key }) { selected.value = key; select(); emit("refresh"); await refresh(); }
async function tasksChanged() { emit("refresh"); await list.value?.refresh(); }
</script>
<template>
  <div class="operations-core-page">
    <div class="analysis-page-head"><div><div class="analysis-page-kicker">关键词持续观察</div><h2>关键词热门视频监控</h2><p>每个关键词保存真实搜索命中关系。同一视频命中多个关键词时保留各关系，互动指标按视频去重。</p></div></div>
    <OperationsPortfolio ref="portfolio" group-by="keyword" @filter="analyzeKeyword" @discover="emit('analyze', { ...discoveryPreset({}, $event), hasKeyword: true, compareBy: 'keyword' })" />
    <MonitoringObjectForm mode="keywords" :base-url="baseUrl" :client-id="clientId" @saved="saved" />
    <section class="analysis-surface operations-object-form"><label class="operations-keyword-select"><span>查看关键词</span><select v-model="selected" @change="select"><option value="">请选择监控关键词</option><option v-if="selected && !keywords.includes(selected)" :value="selected">{{ selected }}</option><option v-for="keyword in keywords" :key="keyword" :value="keyword">{{ keyword }}</option></select></label><p v-if="!keywords.length">暂无关键词监控，请先保存关键词和采集周期。</p></section>
    <details class="analysis-surface operations-task-disclosure"><summary><strong>采集设置与运行记录</strong><span>暂停、恢复、修改周期和查看记录</span></summary><MonitoringTasksPanel ref="tasks" scope="keywords" :target-key="selected" :binding-states="bindingStates" @changed="tasksChanged" /></details>
    <VideoAnalysisList v-if="selected" ref="list" mode="keywords" :keyword="selected" :initial-params="initialParams" @open-video="emit('open-video', $event)" @open-account="emit('open-account', $event)" />
  </div>
</template>
