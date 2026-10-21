<script setup>
import { computed, ref } from 'vue';
import { ArrowUpRight, BarChart3, BookmarkCheck, Clock3, Sparkles, TrendingUp } from 'lucide-vue-next';
import { discoveryDays, assessmentPresentation } from '../lib/videoWorkbench.js';
import { formatDecimal } from '../lib/numberFormatting.js';

const props = defineProps({ analysis: { type: Object, default: null }, generatedAt: { type: String, default: '' }, compact: Boolean, collapsible: Boolean });
const emit = defineEmits(['discover', 'filter']);
const groupSort = ref('average');
const groupSearch = ref('');
const expanded = ref(false);
const number = value => value === null || value === undefined ? '—' : formatDecimal(value);
const date = value => value && !value.startsWith('0001-') ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '暂无观察';
const days = computed(() => discoveryDays(props.analysis?.discoveryTrend, props.analysis?.to && !props.analysis.to.startsWith('0001-') ? props.analysis.to : props.generatedAt));
const maxDay = computed(() => Math.max(1, ...days.value.map(day => day.newVideos)));
const discovered = computed(() => days.value.reduce((sum, day) => sum + day.newVideos, 0));
const coverage = computed(() => props.analysis?.observedVideos ? `${Math.round(1000 * props.analysis.completeVideos / props.analysis.observedVideos) / 10}%` : '暂无样本');
const standout = computed(() => assessmentPresentation(props.analysis?.readiness?.standout, props.analysis?.standoutVideos));
const growth = computed(() => assessmentPresentation(props.analysis?.readiness?.growth, props.analysis?.risingVideos));
const review = computed(() => assessmentPresentation(props.analysis?.readiness?.review, props.analysis?.underperformingVideos));
const allGroups = computed(() => (props.analysis?.groups || []).filter(group => group.key !== '__unknown' && group.name.toLowerCase().includes(groupSearch.value.trim().toLowerCase())).sort((a, b) => {
  const key = { average: 'averageInteractions', selected: 'selectedVideos', rising: 'risingVideos', videos: 'videos' }[groupSort.value];
  return (b[key] ?? -1) - (a[key] ?? -1) || b.videos - a.videos || a.key.localeCompare(b.key);
}));
const groups = computed(() => allGroups.value.slice(0, expanded.value ? 100 : props.compact ? 5 : 8).map(group => ({ ...group,
  standout: assessmentPresentation(group.readiness?.standout, group.standoutVideos),
  growth: assessmentPresentation(group.readiness?.growth, group.risingVideos),
})));
const groupFilter = group => emit('filter', props.analysis?.groupBy === 'keyword' ? { monitorKeyword: group.key } : { account: group.key });
const discover = (key, status) => emit('discover', status.unavailable ? 'all' : key);
</script>

<template>
  <section v-if="analysis" class="ops-insights" aria-label="运营发现与分析">
    <div class="ops-opportunity-grid">
      <button type="button" class="ops-opportunity" @click="discover('standout', standout)"><span><Sparkles :size="16" />表现突出<span v-if="standout.historical" class="ops-assessment-badge">历史参考</span></span><strong :class="{ 'ops-assessment-pending': standout.unavailable }">{{ standout.value }}</strong><p class="ops-assessment-reason">{{ standout.hint }}</p><small>{{ standout.unavailable ? '查看已采集样本' : '寻找可参考的内容' }}<ArrowUpRight :size="13" /></small></button>
      <button type="button" class="ops-opportunity" @click="discover('rising', growth)"><span><TrendingUp :size="16" />正在增长</span><strong :class="{ 'ops-assessment-pending': growth.unavailable }">{{ growth.value }}</strong><p class="ops-assessment-reason">{{ growth.hint }}</p><small>{{ growth.unavailable ? '查看样本与采集时间' : '查看增长内容' }}<ArrowUpRight :size="13" /></small></button>
      <button type="button" class="ops-opportunity" @click="emit('discover', analysis.selectedVideos ? 'selected' : 'all')"><span><BookmarkCheck :size="16" />精选内容</span><strong>{{ number(analysis.selectedVideos) }}</strong><p class="ops-assessment-reason">{{ analysis.selectedVideos ? '已由人工确认并加入精选库' : '尚未人工精选，可从热门或突出内容中挑选' }}</p><small>{{ analysis.selectedVideos ? '查看精选内容库' : '去挑选优秀内容' }}<ArrowUpRight :size="13" /></small></button>
      <button type="button" class="ops-opportunity ops-opportunity-review" @click="discover('review', review)"><span><BarChart3 :size="16" />低表现待复核</span><strong :class="{ 'ops-assessment-pending': review.unavailable }">{{ review.value }}</strong><p class="ops-assessment-reason">{{ review.hint }}</p><small>{{ review.unavailable ? '查看样本与采集时间' : '人工复核后决定是否排除' }}<ArrowUpRight :size="13" /></small></button>
    </div>
    <div class="ops-evidence-line"><span><Clock3 :size="14" />最近采集 {{ date(analysis.latestObservedAt) }}</span><span>完整互动指标 {{ number(analysis.completeVideos) }} / {{ number(analysis.observedVideos) }}（{{ coverage }}）</span><span v-if="analysis.staleVideos">{{ number(analysis.staleVideos) }} 条超过 24 小时未更新；完整度不代表实时性</span></div>
    <details class="ops-assessment-guide"><summary>评估标准与“0”的含义</summary><ul><li>表现突出：同账号、同发布龄至少 5 条完整样本且互动中位数大于 0；互动达到中位数 2 倍且不少于 100。历史参考包含过期指标，不代表当前表现。</li><li>正在增长：两次完整观察间隔 10 分钟至 72 小时，最新指标在 24 小时内，且每小时新增互动大于 0。</li><li>低表现复核：互动不超过同组中位数的 30%，发布满 3 天，具备可比历史且指标新鲜。只有满足评估条件才显示数量，是否排除由人工决定。</li><li>近期热门：近 7 天发布、24 小时内更新、互动完整且大于 0，按对数互动和每 3 天减半的时间衰减排序。关键词排名按命中样本的累计互动比较，至少 5 条，并列同名次；样本含过期数据时标注历史参考。</li><li>依据不足 / 待更新表示暂时无法判断；数量 0 表示可评估样本中没有符合项。精选由人工标记，0 表示当前范围尚未精选。</li></ul></details>
    <component :is="collapsible ? 'details' : 'div'" class="ops-analysis-detail">
    <summary v-if="collapsible"><strong>来源对比与采集趋势</strong><span>展开查看图表、账号或关键词表现</span></summary>
    <div class="ops-insights-detail" :class="{ compact }">
      <section class="ops-discovery-chart">
        <div class="ops-section-heading"><div><h3>采集发现趋势</h3><p>近 14 天新发现 {{ number(discovered) }} 条视频</p></div><span class="ops-chart-legend"><i></i>首次发现</span></div>
        <svg viewBox="0 0 520 148" role="img" :aria-label="`近14天首次发现${discovered}条视频；这是采集趋势，不是平台流量趋势`">
          <line x1="8" y1="120" x2="512" y2="120" stroke="#dbe4ef" />
          <g v-for="(day, index) in days" :key="day.date"><title>{{ day.date }}：首次发现 {{ day.newVideos }} 条，观察 {{ day.observedVideos }} 条</title><rect :x="index * 36 + 14" :y="120 - day.newVideos / maxDay * 94" width="22" :height="day.newVideos ? Math.max(2, day.newVideos / maxDay * 94) : 0" rx="4" fill="#3977df" /><text v-if="index % 3 === 0 || index === 13" :x="index * 36 + 25" y="141" text-anchor="middle" fill="#64748b" font-size="10">{{ day.date.slice(5) }}</text></g>
        </svg>
        <p class="ops-chart-note">按首次采集时间统计，用于判断样本扩充节奏。视频互动走势请进入详情查看。</p>
        <details class="ops-trend-details"><summary>查看每日数字与统计口径</summary><p>首次发现按视频首次被采集的日期统计，观察视频按当日有采集记录的视频去重统计；均为已采集样本。</p><table class="ops-trend-numbers"><thead><tr><th>日期</th><th>首次发现</th><th>观察视频</th></tr></thead><tbody><tr v-for="day in days" :key="day.date"><td>{{ day.date }}</td><td>{{ number(day.newVideos) }}</td><td>{{ number(day.observedVideos) }}</td></tr></tbody></table></details>
      </section>
      <section class="ops-source-comparison">
        <div class="ops-section-heading"><div><h3>{{ analysis.groupBy === 'keyword' ? '关键词表现对比' : '账号内容表现对比' }}</h3><p>均互动只计算指标完整的视频，同时展示样本量</p></div></div>
        <div class="ops-comparison-controls"><label><span class="sr-only">查找对比对象</span><input v-model="groupSearch" type="search" :placeholder="analysis.groupBy === 'keyword' ? '查找关键词' : '查找账号'" /></label><label><span class="sr-only">对比排序</span><select v-model="groupSort"><option value="average">按均互动</option><option value="videos">按视频数</option><option value="rising">按增长视频数</option><option value="selected">按精选视频数</option></select></label></div>
        <div class="analysis-table-wrap"><table class="analysis-table ops-comparison-table"><thead><tr><th>{{ analysis.groupBy === 'keyword' ? '关键词' : '账号' }}</th><th>完整 / 视频数</th><th>均互动</th><th>内容评估</th></tr></thead><tbody><tr v-for="group in groups" :key="group.key"><td><button type="button" class="analysis-title-button" @click="groupFilter(group)">{{ group.name }}<ArrowUpRight :size="12" /></button></td><td>{{ number(group.completeVideos) }} / {{ number(group.videos) }}</td><td>{{ number(group.averageInteractions) }}</td><td><div class="ops-group-assessments"><span :title="group.standout.hint" :aria-label="`突出：${group.standout.value}；${group.standout.hint}`">突出 {{ group.standout.value }}<em v-if="group.standout.historical">历史</em></span><span :title="group.selectedVideos ? '人工精选内容数' : '当前范围尚未人工精选'">精选 {{ number(group.selectedVideos) }}</span><span :title="group.growth.hint" :aria-label="`增长：${group.growth.value}；${group.growth.hint}`">增长 {{ group.growth.value }}</span></div></td></tr></tbody></table></div>
        <p v-if="groups.length" class="ops-chart-note">依据不足：尚无有效同组对比；待更新：指标过期；精选 0：尚未人工精选。具体门槛见上方评估标准。</p>
        <p v-if="!groups.length" class="operations-empty">当前范围没有可对比的来源，调整筛选后再查看。</p>
        <button v-if="allGroups.length > (compact ? 5 : 8)" type="button" class="operations-text-button" @click="expanded = !expanded">{{ expanded ? '收起' : `展开更多（${allGroups.length} 组）` }}</button>
        <p v-if="analysis.groupBy === 'keyword'" class="ops-chart-note">同一视频可命中多个关键词，各组不可直接相加；搜索命中样本不代表关键词全站热度。</p>
      </section>
    </div>
    </component>
  </section>
</template>
