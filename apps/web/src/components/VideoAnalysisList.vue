<script setup>
import { computed, onUnmounted, ref, shallowRef, useId, watch } from 'vue';
import { NAlert, NEmpty, NSpin } from 'naive-ui';
import { ArrowUpRight, BookmarkCheck, ChevronLeft, ChevronRight, RefreshCw, SlidersHorizontal } from 'lucide-vue-next';
import { curateVideos, fetchDataCenter, importWXVideoShare } from '../lib/api.js';
import { buildOperationsHash, videoFactsQuery } from '../lib/operationsNavigation.js';
import { videoGrowthLabel } from '../lib/videoFactsPresentation.js';
import { formatDecimal } from '../lib/numberFormatting.js';
import { activeDiscoveryPreset, curationLabel, discoveryPreset, keywordRankPresentation, performanceLabel, VIDEO_DISCOVERY_PRESETS } from '../lib/videoWorkbench.js';
import OperationsInsights from './OperationsInsights.vue';
import VideoCurationEditor from './VideoCurationEditor.vue';

const props = defineProps({ mode: { type: String, default: 'videos' }, routeView: { type: String, default: '' }, routeParams: { type: Object, default: () => ({}) }, scopeAccount: { type: String, default: '' }, keyword: { type: String, default: '' }, initialParams: { type: Object, default: () => ({}) }, baseUrl: { type: String, default: '' }, clientId: { type: String, default: '' } });
const emit = defineEmits(['open-video', 'open-account']);
const titleID = useId();
const query = ref(videoFactsQuery(props.initialParams, props.mode));
const pageData = shallowRef({ items: [], total: 0, totalPages: 1 });
const loading = ref(false); const error = ref(''); const notice = ref('');
const backgroundLoading = ref(false);
const shareURL = ref(''); const importBusy = ref(false); const mutationBusy = ref(false);
const selectedIDs = ref([]); const editorOpen = ref(false); const editorVideos = ref([]);
let requestID = 0;
let requestController;
const rows = computed(() => pageData.value.items || []);
const selectedSet = computed(() => new Set(selectedIDs.value));
const selectedRows = computed(() => rows.value.filter(row => selectedSet.value.has(row.id)));
const allSelected = computed(() => rows.value.length > 0 && selectedRows.value.length === rows.value.length);
const busy = computed(() => loading.value || mutationBusy.value);
const currentPreset = computed(() => activeDiscoveryPreset(query.value));
const discoveryPresets = computed(() => VIDEO_DISCOVERY_PRESETS.filter(preset => preset.key !== 'keyword_hot' || query.value.monitorKeyword || props.keyword));
const ranksByRow = computed(() => {
  const keyword = props.keyword || query.value.monitorKeyword;
  return new Map(rows.value.map(row => {
    const ranks = row.performance?.keywordRanks || [];
    return [row.id, (keyword ? ranks.filter(rank => rank.keyword === keyword) : ranks).map(keywordRankPresentation)];
  }));
});
const rankDisplays = row => ranksByRow.value.get(row.id) || [];
const libraryStates = computed(() => {
  const m = pageData.value.management;
  return [
    { value: 'active', label: '有效内容', count: m ? m.total - m.excluded : null },
    { value: 'unreviewed', label: '未处理', count: m?.unreviewed },
    { value: 'selected', label: '精选', count: m?.selected },
    { value: 'watching', label: '待观察', count: m?.watching },
    { value: 'excluded', label: '排除列表', count: m?.excluded },
    { value: 'all', label: '全部含排除', count: m?.total },
  ];
});
const pageStart = computed(() => pageData.value.total ? ((pageData.value.page || 1) - 1) * (pageData.value.pageSize || 20) + 1 : 0);
const pageEnd = computed(() => Math.min(pageData.value.total, (pageData.value.page || 1) * (pageData.value.pageSize || 20)));
const sortOptions = [
  { value: 'observed_at', label: '最近采集' }, { value: 'published_at', label: '发布时间' },
  { value: 'interactions', label: '互动总量' }, { value: 'growth_per_hour', label: '每小时新增互动' },
  { value: 'recent_heat', label: '近期热度（近 7 天发布）' },
  { value: 'relative_performance', label: '相对同组表现' }, { value: 'growth_rate', label: '历史增长率' },
  { value: 'likes', label: '点赞' }, { value: 'comments', label: '评论' }, { value: 'favorites', label: '收藏' }, { value: 'forwards', label: '转发' },
];
const metric = value => value === null || value === undefined || value === '' ? '—' : formatDecimal(Number(value));
const date = value => value && !value.startsWith('0001-') && Number.isFinite(new Date(value).getTime()) ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '未记录';
const growthWindow = row => row.growthWindow?.baselineObservedAt ? `${date(row.growthWindow.baselineObservedAt)} → ${date(row.growthWindow.latestObservedAt)}` : '等待可比历史';
function currentHash() {
  const { monitorKeyword, ...params } = query.value;
  return buildOperationsHash(props.routeView || props.mode, { ...props.routeParams, ...params, keyword: monitorKeyword });
}
async function load({ updateHash = false, refresh = false, background = false } = {}) {
  if (props.mode === 'keywords' && !props.keyword) return;
  if (query.value.from && query.value.to && query.value.from > query.value.to) { error.value = '采集起始日期不能晚于截止日期。'; return; }
  if (query.value.publishedFrom && query.value.publishedTo && query.value.publishedFrom > query.value.publishedTo) { error.value = '发布时间起始日期不能晚于截止日期。'; return; }
  const id = ++requestID;
  requestController?.abort();
  requestController = new AbortController();
  query.value.monitorKeyword = props.mode === 'keywords' ? props.keyword : query.value.monitorKeyword;
  if (props.scopeAccount) query.value.account = props.scopeAccount;
  if (updateHash) window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}${currentHash()}`);
  loading.value = !background; backgroundLoading.value = background; error.value = '';
  try {
    const payload = await fetchDataCenter('videos', { ...query.value, refresh, from: query.value.from ? `${query.value.from}T00:00:00+08:00` : '', to: query.value.to ? `${query.value.to}T23:59:59.999999999+08:00` : '', publishedFrom: query.value.publishedFrom ? `${query.value.publishedFrom}T00:00:00+08:00` : '', publishedTo: query.value.publishedTo ? `${query.value.publishedTo}T23:59:59.999999999+08:00` : '' }, { signal: requestController.signal });
    if (id !== requestID) return;
    pageData.value = payload;
    query.value.page = Number(payload.page) || query.value.page;
    selectedIDs.value = selectedIDs.value.filter(key => payload.items?.some(row => row.id === key));
  } catch (cause) {
    if (id === requestID && cause?.name !== 'AbortError') error.value = `视频查询失败：${cause instanceof Error ? cause.message : cause}。上次有效数据仍保留，请重试。`;
  } finally { if (id === requestID) { loading.value = false; backgroundLoading.value = false; } }
}
function search() { query.value.page = 1; selectedIDs.value = []; load({ updateHash: true }); }
function choosePreset(key) { query.value = discoveryPreset(query.value, key); search(); }
function chooseLibrary(value) { query.value = { ...discoveryPreset(query.value, 'all'), curation: value }; search(); }
function filterSource(filters) { query.value = { ...query.value, ...filters }; search(); }
function selectWindow(days) {
  const day = value => new Date(value).toLocaleDateString('en-CA', { timeZone: 'Asia/Shanghai' });
  query.value.from = days ? day(Date.now() - (days - 1) * 86400000) : '';
  query.value.to = days ? day(Date.now()) : ''; search();
}
function resetFilters() { query.value = videoFactsQuery({ keyword: props.mode === 'keywords' ? props.keyword : '' }, props.mode); search(); }
function page(value) { query.value.page = value; selectedIDs.value = []; load({ updateHash: true }); }
function togglePage() { selectedIDs.value = allSelected.value ? [] : rows.value.map(row => row.id); }
function openVideo(video) { emit('open-video', { video, returnHash: currentHash() }); }
function edit(videos) { editorVideos.value = videos; editorOpen.value = true; }
async function edited(count) { notice.value = `已保存 ${count} 条视频的运营标记。`; await load(); }
async function mark(videos, status) {
  if (busy.value || !videos.length) return;
  mutationBusy.value = true; error.value = ''; notice.value = '';
  try {
    await curateVideos({ ids: videos.map(row => row.id), status });
    notice.value = `已将 ${videos.length} 条视频设为「${curationLabel(status)}」。${status === 'excluded' ? '原始数据已保留，可到排除列表恢复。' : ''}`;
    selectedIDs.value = []; await load();
  } catch (cause) { error.value = `操作失败：${cause.message}。所选视频已保留，请重试。`; }
  finally { mutationBusy.value = false; }
}
async function importVideo() {
  if (!shareURL.value.trim() || importBusy.value) return;
  importBusy.value = true; error.value = ''; notice.value = '';
  try { const payload = await importWXVideoShare(props.baseUrl, shareURL.value.trim(), props.clientId); notice.value = `已提交原视频采集：${payload.status || '请查看采集状态'}`; await load({ refresh: true }); }
  catch (cause) { error.value = `视频采集失败：${cause.message}`; }
  finally { importBusy.value = false; }
}
watch([() => props.keyword, () => props.initialParams], () => {
  query.value = videoFactsQuery(props.initialParams, props.mode);
  if (props.mode === 'keywords' && props.initialParams.keyword !== props.keyword) query.value.page = 1;
  selectedIDs.value = []; load();
}, { immediate: true });
onUnmounted(() => { requestID++; requestController?.abort(); });
defineExpose({ refresh: (options = {}) => {
  if (busy.value || backgroundLoading.value || editorOpen.value || selectedIDs.value.length) return;
  return load({ refresh: !options.background, background: !!options.background });
}, discover: choosePreset });
</script>

<template>
  <section class="analysis-page ops-video-workbench" :aria-labelledby="titleID">
    <div class="analysis-page-head"><div><div class="analysis-page-kicker">发现 · 判断 · 筛选 · 复盘</div><h2 :id="titleID">{{ mode === 'keywords' ? `「${keyword}」内容分析` : '视频运营工作台' }}</h2><p>从已采集样本发现机会，把值得研究的内容加入精选库。标记与原始数据分别保存。</p></div><button class="tool-button secondary" type="button" :disabled="busy" @click="load({ refresh: true })"><RefreshCw :size="15" />更新分析</button></div>
    <n-alert v-if="error" type="error" show-icon role="alert">{{ error }}</n-alert><n-alert v-if="notice" type="success" show-icon role="status">{{ notice }}</n-alert>
    <div class="ops-discovery-nav" aria-label="内容发现方式"><button v-for="preset in discoveryPresets" :key="preset.key" type="button" :class="{ active: currentPreset === preset.key }" :aria-pressed="currentPreset === preset.key" :disabled="busy" @click="choosePreset(preset.key)">{{ preset.label }}</button></div>
    <section class="analysis-surface ops-content-surface" :aria-busy="loading || backgroundLoading">
      <form class="ops-main-search" @submit.prevent="search"><label><span class="sr-only">标题或备注搜索</span><input v-model="query.q" type="search" placeholder="搜索视频标题、账号、标签或复盘备注" /></label><label v-if="!scopeAccount"><span class="sr-only">来源账号</span><input v-model="query.account" placeholder="筛选来源账号" /></label><button class="tool-button primary" type="submit" :disabled="busy">筛选内容</button><button class="tool-button secondary" type="button" :disabled="busy" @click="resetFilters">重置</button></form>
      <details class="ops-advanced-filters"><summary><SlidersHorizontal :size="14" />更多筛选与排序<span>时间、阈值、标签、指标完整性</span></summary><form class="operations-filter-form operations-video-filters" @submit.prevent="search"><label v-if="mode !== 'keywords'"><span>来源关键词（精确）</span><input v-model="query.monitorKeyword" placeholder="例如：秋天" /></label><label><span>采集起始日期</span><input v-model="query.from" type="date" /></label><label><span>采集截止日期</span><input v-model="query.to" type="date" /></label><label><span>发布起始日期</span><input v-model="query.publishedFrom" type="date" /></label><label><span>发布截止日期</span><input v-model="query.publishedTo" type="date" /></label><label><span>最低互动总量</span><input v-model="query.minInteractions" type="number" min="0" step="1" placeholder="不限；缺失数据不参与" /></label><label><span>最低点赞</span><input v-model="query.minLikes" type="number" min="0" step="1" placeholder="不限" /></label><label><span>内容标签</span><input v-model="query.tag" placeholder="精确标签" /></label><label><span>表现分层</span><select v-model="query.performance"><option value="">全部表现</option><option value="standout">表现突出</option><option value="typical">常规表现</option><option value="underperforming">低表现待复核</option><option value="insufficient_data">待积累依据</option></select></label><label><span>对比维度</span><select v-model="query.compareBy"><option value="account">来源账号</option><option value="keyword">来源关键词</option></select></label><label><span>排序依据</span><select v-model="query.sort"><option v-for="option in sortOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label><label><span>顺序</span><select v-model="query.direction"><option value="desc">从高到低 / 从新到旧</option><option value="asc">从低到高 / 从旧到新</option></select></label><label class="ops-check-filter"><input v-model="query.recentOnly" type="checkbox" /><span>仅看近期热门合格样本</span></label><label class="ops-check-filter"><input v-model="query.freshOnly" type="checkbox" /><span>仅看 24 小时内更新</span></label><label class="ops-check-filter"><input v-model="query.momentumOnly" type="checkbox" /><span>仅看有可靠正增长</span></label><button class="tool-button primary" type="submit" :disabled="busy">应用筛选</button></form></details>
      <div class="ops-window-actions"><span>采集窗口</span><button type="button" :disabled="busy" @click="selectWindow(7)">近 7 天</button><button type="button" :disabled="busy" @click="selectWindow(30)">近 30 天</button><button type="button" :disabled="busy" @click="selectWindow(0)">全部时间</button><span>未采全指标显示「—」，不当作零值</span></div>
      <div v-if="query.to" class="ops-ranking-context" role="status"><strong>采集时间范围截至 {{ query.to }}</strong><p>本页采用所选范围内的采集记录。历史日期下的“新鲜”、增长和近期热度均相对于所选截止时点评估，用于运营复盘。</p></div>
      <OperationsInsights collapsible :analysis="pageData.analysis" :generated-at="pageData.generatedAt" @discover="choosePreset" @filter="filterSource" />
      <div class="ops-library-nav" aria-label="运营内容库"><button v-for="state in libraryStates" :key="state.value" type="button" :class="{ active: query.curation === state.value }" :aria-pressed="query.curation === state.value" :disabled="busy" @click="chooseLibrary(state.value)">{{ state.label }}<span>{{ metric(state.count) }}</span></button></div>
      <div v-if="query.recentOnly" class="ops-ranking-context" role="status">
        <strong>近期热门 · 时间衰减热度 v1</strong>
        <p>近 7 天发布，24 小时内更新，互动指标完整；综合互动量与发布时间发现值得关注的内容。</p><details><summary>查看排序口径</summary><p>热度 = ln(1 + 总互动) × 2^(-发布小时数 / 72)，每 3 天衰减一半，只展示正互动视频。用于内容发现，不代表播放量或质量评分。</p></details>
        <p>{{ pageData.analysis?.recentHot?.reason || '正在核对当前来源范围的样本…' }}<span v-if="query.to"> · 以所选采集截止时间回看</span></p>
        <button type="button" class="operations-text-button" :disabled="busy" @click="choosePreset('hot')">查看累计热门<ArrowUpRight :size="13" /></button>
      </div>
      <div v-if="query.monitorKeyword" class="ops-ranking-context">
        <strong># {{ query.monitorKeyword }} · 关键词样本内排名</strong>
        <p>按该词命中视频的累计互动比较，显示名次、完整样本数和历史状态。<span v-if="query.to">当前为截至 {{ query.to }} 的样本回看。</span></p><details><summary>查看比较范围</summary><p>采用所选采集时间范围内的完整样本，至少 5 条才排名。名次在搜索、账号、发布日期、运营标记筛选和翻页前计算；并列同名次。累计互动未校正账号规模和发布龄，不代表关键词全站热度。</p></details>
        <button v-if="currentPreset !== 'keyword_hot'" type="button" class="operations-text-button" :disabled="busy" @click="choosePreset('keyword_hot')">按关键词累计互动查看<ArrowUpRight :size="13" /></button>
      </div>
      <div class="ops-result-head"><div><h3>{{ query.curation === 'excluded' ? '已排除内容 · 随时可恢复' : query.curation === 'selected' ? '精选内容库' : '内容评估与管理' }}</h3><p>匹配 {{ metric(pageData.total) }} 条 · 分析覆盖全部匹配结果，列表每页 {{ query.pageSize }} 条<span v-if="backgroundLoading" class="operations-updated" role="status">正在后台更新…</span></p></div><label><span class="sr-only">每页显示条数</span><select v-model.number="query.pageSize" :disabled="busy" @change="search"><option :value="20">20 条 / 页</option><option :value="50">50 条 / 页</option></select></label></div>
      <div v-if="loading && !rows.length" class="account-detail-loading"><n-spin size="medium" /><span>正在读取并分析已采集视频…</span></div>
      <n-empty v-else-if="!rows.length" description="当前条件下没有内容，可调整筛选或切换内容库"><template #extra><button type="button" class="tool-button secondary" :disabled="busy" @click="resetFilters">清除筛选</button></template></n-empty>
      <div v-else class="analysis-table-wrap"><table class="analysis-table ops-video-table"><thead><tr><th class="ops-select-cell"><input type="checkbox" :checked="allSelected" :indeterminate="selectedRows.length > 0 && !allSelected" :disabled="busy" aria-label="选择本页全部视频" @change="togglePage" /></th><th>视频与运营标记</th><th>互动与增长</th><th>表现判断</th><th>发布与采集</th><th>运营操作</th></tr></thead><tbody><tr v-for="row in rows" :key="row.id" :class="{ 'ops-row-selected': selectedIDs.includes(row.id), 'ops-row-excluded': row.curation?.status === 'excluded' }"><td class="ops-select-cell"><input v-model="selectedIDs" type="checkbox" :value="row.id" :disabled="busy" :aria-label="`选择视频：${row.title || row.objectId}`" /></td><td class="ops-video-title-cell"><div class="ops-video-identity"><img v-if="row.coverUrl" :src="row.coverUrl" alt="" loading="lazy" referrerpolicy="no-referrer" @error="$event.target.style.display = 'none'" /><div><button type="button" class="analysis-title-button ops-video-title" :title="row.title" :disabled="!row.objectId" @click="openVideo(row)">{{ row.title || '未命名视频' }}</button><button type="button" class="operations-text-button ops-video-author" :disabled="!row.channelUsername" @click="emit('open-account', row)">{{ row.channelName || row.channelUsername || '账号待确认' }}<ArrowUpRight :size="12" /></button></div></div><div class="ops-video-tags"><span class="ops-curation-badge" :data-status="row.curation?.status">{{ curationLabel(row.curation?.status) }}</span><span v-for="tag in row.curation?.tags" :key="tag" class="ops-content-tag">{{ tag }}</span><span v-for="sourceKeyword in row.sourceKeywords" :key="`source-${sourceKeyword}`" class="ops-source-tag"># {{ sourceKeyword }}</span></div><p v-if="row.curation?.note" class="ops-video-note">{{ row.curation.note }}</p><small class="ops-video-id">ID {{ row.objectId || '未记录' }}</small></td><td><strong class="ops-interaction-total">{{ metric(row.performance?.interactions) }}<small> 总互动</small></strong><div class="ops-metric-pairs"><span>赞 {{ metric(row.likes) }}</span><span>评 {{ metric(row.comments) }}</span><span>藏 {{ metric(row.favorites) }}</span><span>转 {{ metric(row.forwards) }}</span></div><div class="ops-growth-evidence" :title="growthWindow(row)"><strong v-if="row.performance?.growthPerHour !== null && row.performance?.growthPerHour !== undefined">+{{ metric(row.performance.growthPerHour) }} / 小时</strong><span v-else>{{ videoGrowthLabel(row) }}</span><small>{{ row.observationCount || 0 }} 次观察 · {{ row.performance?.growthPerHour != null ? '最近两次可比观察' : '查看详情了解增长依据' }}</small></div></td><td class="ops-performance-cell">
        <div v-for="rank in rankDisplays(row)" :key="rank.label" class="ops-keyword-rank" :title="rank.reason"><strong>{{ rank.label }}</strong><small>{{ rank.detail }}</small><span v-if="rank.historical" class="ops-stale-label">历史参考 · 样本含过期数据</span></div>
        <div v-if="row.performance?.recentHeat?.score != null" class="ops-recent-heat" :title="row.performance.recentHeat.reason"><strong>{{ metric(row.performance.recentHeat.score) }}</strong><span>近期热度 · v1</span></div>
        <details class="ops-account-assessment" :open="!rankDisplays(row).length"><summary>同账号表现：<span class="ops-performance-badge" :data-tier="row.performance?.tier">{{ performanceLabel(row.performance?.tier) }}</span></summary><strong v-if="row.performance?.relativeToMedian != null">同组 {{ metric(row.performance.relativeToMedian) }} 倍</strong><p>{{ row.performance?.reason || '正在形成分析依据' }}</p></details>
        </td><td class="ops-video-time"><span>发布 {{ date(row.publishedAt) }}</span><span>采集 {{ date(row.observedAt) }}</span><span v-if="row.performance?.freshness === 'stale'" class="ops-stale-label">数据待更新</span></td><td><div class="ops-row-actions"><button class="ops-mark-button" :class="{ active: row.curation?.status === 'selected' }" type="button" :disabled="busy" :aria-label="`精选视频：${row.title || row.objectId}`" @click="mark([row], 'selected')"><BookmarkCheck :size="13" />精选</button><button class="ops-mark-button" type="button" :disabled="busy" @click="mark([row], 'watching')">观察</button><button v-if="row.curation?.status !== 'excluded'" class="ops-mark-button" type="button" :disabled="busy" @click="mark([row], 'excluded')">排除</button><button v-else class="ops-mark-button" type="button" :disabled="busy" @click="mark([row], 'unreviewed')">恢复</button><button class="ops-mark-button" type="button" :disabled="busy" @click="edit([row])">标签备注</button><button class="operations-text-button" type="button" :disabled="!row.objectId" @click="openVideo(row)">趋势与评论<ArrowUpRight :size="13" /></button></div></td></tr></tbody></table></div>
      <footer v-if="pageData.total" class="data-center-pagination"><span>显示 {{ pageStart }}–{{ pageEnd }}，共 {{ metric(pageData.total) }} 条</span><div class="data-center-pagination-controls"><button class="analysis-icon-button" type="button" aria-label="上一页" :disabled="query.page <= 1 || busy" @click="page(query.page - 1)"><ChevronLeft :size="16" /></button><strong>第 {{ query.page }} / {{ pageData.totalPages || 1 }} 页</strong><button class="analysis-icon-button" type="button" aria-label="下一页" :disabled="query.page >= pageData.totalPages || busy" @click="page(query.page + 1)"><ChevronRight :size="16" /></button></div></footer>
    </section>
    <div v-if="selectedRows.length" class="ops-batch-bar" role="region" aria-label="批量管理所选视频"><strong>已选 {{ selectedRows.length }} 条</strong><button type="button" :disabled="busy" @click="mark(selectedRows, 'selected')">加入精选</button><button type="button" :disabled="busy" @click="mark(selectedRows, 'watching')">待观察</button><button type="button" :disabled="busy" @click="mark(selectedRows, 'excluded')">排除</button><button type="button" :disabled="busy" @click="mark(selectedRows, 'unreviewed')">恢复未处理</button><button type="button" :disabled="busy" @click="edit(selectedRows)">批量标签</button><button type="button" :disabled="busy" @click="selectedIDs = []">取消选择</button><span v-if="mutationBusy" role="status">正在保存…</span></div>
    <VideoCurationEditor v-model:show="editorOpen" :videos="editorVideos" @saved="edited" />
    <details v-if="mode === 'videos'" class="analysis-surface ops-import-video"><summary>采集单条视频数据</summary><p>填写原视频分享链接，采集互动数据后进入分析。</p><form class="operations-filter-form" @submit.prevent="importVideo"><label><span>原视频分享链接</span><input v-model="shareURL" type="url" required placeholder="https://…" /></label><button class="tool-button primary" :disabled="importBusy" type="submit">{{ importBusy ? '采集中…' : '采集视频数据' }}</button></form></details>
  </section>
</template>
