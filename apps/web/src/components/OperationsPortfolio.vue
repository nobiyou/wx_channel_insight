<script setup>
import { onMounted, ref, shallowRef } from 'vue';
import { NAlert, NSpin } from 'naive-ui';
import { fetchDataCenter } from '../lib/api.js';
import OperationsInsights from './OperationsInsights.vue';
const props = defineProps({ groupBy: { type: String, default: 'account' } });
const emit = defineEmits(['filter', 'discover']);
const data = shallowRef(null); const loading = ref(false); const error = ref('');
async function load() {
  if (loading.value) return;
  loading.value = true; error.value = '';
  try { data.value = await fetchDataCenter('videos', { compareBy: props.groupBy, hasKeyword: props.groupBy === 'keyword' }); }
  catch (cause) { error.value = `运营对比暂不可用：${cause.message}。可继续管理监控对象。`; }
  finally { loading.value = false; }
}
onMounted(load); defineExpose({ refresh: load });
</script>
<template><section class="analysis-surface ops-portfolio" :aria-busy="loading"><div v-if="loading && !data" class="account-detail-loading"><n-spin size="small" /><span>正在分析已采集内容…</span></div><n-alert v-if="error" type="warning">{{ error }}<button type="button" class="operations-text-button" @click="load">重试</button></n-alert><OperationsInsights v-if="data" :analysis="data.analysis" :generated-at="data.generatedAt" compact @filter="emit('filter', $event)" @discover="emit('discover', $event)" /></section></template>
