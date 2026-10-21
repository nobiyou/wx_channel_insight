<script setup>
import { computed, onMounted, ref, shallowRef } from 'vue';
import { NAlert, NSpin } from 'naive-ui';
import { ArrowUpRight, Clock3, Hash, Users, Video } from 'lucide-vue-next';
import { fetchAccounts, fetchDataCenter, fetchMonitoringTasks } from '../lib/api.js';
import { buildOperationsSummary } from '../lib/operationsOverview.js';
import { discoveryPreset, assessmentPresentation } from '../lib/videoWorkbench.js';
import { formatInteger } from '../lib/numberFormatting.js';
import OperationsInsights from './OperationsInsights.vue';
import { formatMonitoringTime, monitoringRunStatusLabel, monitoringStatusTone, monitoringTaskStatusLabel } from '../lib/monitoringTasks.js';
const emit = defineEmits(['navigate', 'open-video']);
const accounts = ref(null); const videoPage = shallowRef(null); const tasks = shallowRef(null);
const error = ref(''); const loading = ref(false);
const summary = computed(() => buildOperationsSummary(tasks.value, accounts.value, videoPage.value?.total ?? null));
const recentRuns = computed(() => (tasks.value || []).filter(task => task.lastRunAt).sort((a,b) => new Date(b.lastRunAt) - new Date(a.lastRunAt)).slice(0,5));
const highlights = computed(() => (videoPage.value?.items || []).slice(0,4));
const rising = computed(() => videoPage.value?.analysis?.risingTop || []);
const growthAssessment = computed(() => assessmentPresentation(videoPage.value?.analysis?.readiness?.growth, videoPage.value?.analysis?.risingVideos));
function discover(key) { emit('navigate','videos',discoveryPreset({}, key)); }
function filterSource(filters) { const { monitorKeyword, ...rest } = filters; emit('navigate','videos',{...rest,keyword:monitorKeyword}); }
const number = value => value === null || value === undefined ? '—' : formatInteger(value);
async function load() {
  if (loading.value) return;
  loading.value = true;
  try {
    const results = await Promise.allSettled([fetchAccounts({page:1,pageSize:20}), fetchDataCenter('videos',{page:1,pageSize:20,sort:'interactions',direction:'desc',compareBy:'keyword'}), fetchMonitoringTasks()]);
    if (results[0].status === 'fulfilled') accounts.value = results[0].value.total;
    if (results[1].status === 'fulfilled') videoPage.value = results[1].value;
    if (results[2].status === 'fulfilled') tasks.value = results[2].value.items || [];
    error.value = results.filter(result => result.status === 'rejected').map(result => result.reason?.message || '摘要暂不可用').join('；');
  } finally { loading.value = false; }
}
onMounted(load); defineExpose({refresh:load});
</script>
<template>
  <section class="analysis-page operations-overview" :aria-busy="loading">
    <div class="analysis-page-head"><div><h2>运营机会概览</h2><p>先看内容机会与变化，再决定关注、精选和排除。</p></div><span class="operations-updated"><n-spin v-if="loading" size="small" /><Clock3 v-else :size="14" />{{ loading ? '正在读取数据…' : `读取于 ${formatMonitoringTime(videoPage?.generatedAt, '等待首次采集')}` }}</span></div>
    <n-alert v-if="error" type="warning" show-icon>部分数据暂不可用，已保留上次结果：{{ error }}</n-alert>
    <section class="analysis-surface ops-overview-insights"><OperationsInsights :analysis="videoPage?.analysis" :generated-at="videoPage?.generatedAt" compact @discover="discover" @filter="filterSource" /></section>
    <div class="operations-overview-grid">
      <button class="analysis-surface operations-summary-card" type="button" @click="emit('navigate','accounts')"><span class="operations-summary-label"><Users :size="18" />账号监控计划<ArrowUpRight :size="16" /></span><strong>{{ number(summary.accountPlans) }}<small>个计划</small></strong><span class="operations-summary-state" v-if="summary.accountPlans !== null"><span>{{ summary.accountEnabled }} 个启用</span><span>{{ summary.accountPaused }} 个暂停</span></span><span v-else>监控计划暂不可用</span><small>已收录 {{ number(summary.savedAccounts) }} 个账号，含搜索样本作者</small></button>
      <button class="analysis-surface operations-summary-card" type="button" @click="emit('navigate','keywords')"><span class="operations-summary-label"><Hash :size="18" />关键词监控<ArrowUpRight :size="16" /></span><strong>{{ number(summary.keywords) }}<small>个关键词</small></strong><span class="operations-summary-state" v-if="summary.keywords !== null"><span>{{ summary.keywordEnabled }} 个启用</span><span>{{ summary.keywordPaused }} 个暂停</span></span><span v-else>关键词计划暂不可用</span><small>查看真实搜索样本和互动表现</small></button>
      <button class="analysis-surface operations-summary-card" type="button" @click="emit('navigate','videos')"><span class="operations-summary-label"><Video :size="18" />已采集视频<ArrowUpRight :size="16" /></span><strong>{{ number(summary.videos) }}<small>条视频</small></strong><span class="operations-summary-state"><span>按真实视频 ID 去重</span></span><small>查看指标趋势、来源与原始评论</small></button>
    </div>
    <section class="analysis-surface operations-highlights ops-rising-board"><div class="analysis-surface-head"><div><h3>正在增长的内容</h3><p>最近两次可比观察的每小时新增互动，最新指标在 24 小时内</p></div><button class="operations-text-button" type="button" @click="discover('rising')">增长榜<ArrowUpRight :size="14" /></button></div><ol v-if="rising.length" class="operations-highlight-list"><li v-for="(video,index) in rising" :key="video.objectId"><span class="operations-rank">{{ index + 1 }}</span><button type="button" class="operations-highlight-title" @click="emit('open-video',{video,returnHash:'#dashboard'})"><strong>{{ video.title || '未命名视频' }}</strong><span>{{ video.channelName || '账号待确认' }}</span></button><div class="operations-highlight-metric"><strong>+{{ number(Math.round(video.performance.growthPerHour)) }}</strong><span>互动 / 小时</span></div></li></ol><div v-else class="operations-empty">{{ growthAssessment.hint }}。可查看历史热门，或在任务中检查指标采集进度。</div></section>
    <div class="operations-overview-detail-grid">
      <section class="analysis-surface operations-recent"><div class="analysis-surface-head"><div><h3>最近采集</h3><p>执行结果和自动采集状态分开显示</p></div><button class="operations-text-button" type="button" @click="emit('navigate','settings',{tab:'runs'})">全部记录<ArrowUpRight :size="14" /></button></div><ul v-if="recentRuns.length" class="operations-status-list"><li v-for="task in recentRuns" :key="task.id"><div class="operations-run-title"><span class="operations-status-dot" :data-tone="monitoringStatusTone(task.lastStatus)"></span><button type="button" @click="emit('navigate','settings',{tab:'runs',task:task.id})">{{ task.name }}</button><span class="operations-badge" :data-tone="monitoringStatusTone(task.lastStatus)">{{ monitoringRunStatusLabel(task.lastStatus) }}</span></div><div class="operations-run-meta"><span>{{ monitoringTaskStatusLabel(task.status) }}</span><time>{{ formatMonitoringTime(task.lastRunAt) }}</time></div><p v-if="task.lastError" class="operations-run-error">{{ task.lastError }}</p></li></ul><div v-else class="operations-empty">暂无采集记录。添加账号或关键词监控后，执行首次采集。</div></section>
      <section class="analysis-surface operations-highlights"><div class="analysis-surface-head"><div><h3>互动热门 · 已采集样本</h3><p>仅比较已保存的样本，不代表全站热榜</p></div><button class="operations-text-button" type="button" @click="emit('navigate','videos',{sort:'interactions'})">查看全部<ArrowUpRight :size="14" /></button></div><ol v-if="highlights.length" class="operations-highlight-list"><li v-for="(video,index) in highlights" :key="video.objectId"><span class="operations-rank">{{ index + 1 }}</span><button type="button" class="operations-highlight-title" @click="emit('open-video',{video,returnHash:'#dashboard'})"><strong>{{ video.title || '未命名视频' }}</strong><span>{{ video.channelName || '账号待确认' }}</span></button><div class="operations-highlight-metric"><strong>{{ number(video.performance?.interactions) }}</strong><span>总互动</span></div></li></ol><div v-else class="operations-empty">采集视频后，这里会展示样本中的互动表现。</div></section>
    </div>
  </section>
</template>
