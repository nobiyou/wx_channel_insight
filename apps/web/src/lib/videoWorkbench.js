import { formatDecimal, formatInteger } from './numberFormatting.js';

export const VIDEO_CURATION_STATES = Object.freeze([
  { value: 'unreviewed', label: '未处理', tone: 'neutral' },
  { value: 'selected', label: '已精选', tone: 'success' },
  { value: 'watching', label: '待观察', tone: 'warning' },
  { value: 'excluded', label: '已排除', tone: 'muted' },
]);

export function curationLabel(value) {
  return VIDEO_CURATION_STATES.find(state => state.value === value)?.label || '未处理';
}

export function performanceLabel(tier) {
  return { standout: '表现突出', typical: '常规表现', underperforming: '低表现待复核', insufficient_data: '待积累依据' }[tier] || '待积累依据';
}

export function keywordRankPresentation(rank) {
  const number = formatDecimal;
  const available = ['available', 'historical'].includes(rank?.state) && rank.rank != null;
  return {
    label: available ? `#${rank.keyword} · 第 ${number(rank.rank)} / ${number(rank.sampleCount)} 名` : `#${rank?.keyword || ''} · 排名依据不足`,
    detail: available ? `超过 ${number(rank.percentile)}% 样本 · 并列同名次` : rank?.reason || '等待完整样本',
    historical: rank?.state === 'historical',
    reason: rank?.reason || '',
  };
}

// Eligibility and freshness rules belong to the analysis API. Presentation only
// maps its explicit states, so missing evidence never silently becomes a zero.
export function assessmentPresentation(assessment, count) {
  const state = assessment?.state;
  const unavailable = !['available', 'historical'].includes(state);
  return {
    value: unavailable ? ({ stale: '待更新', insufficient_data: '依据不足', empty: '暂无样本' }[state] || '待评估') : formatInteger(count),
    unavailable,
    historical: state === 'historical',
    hint: `${assessment?.reason || '等待评估状态，稍后刷新再查看'}${!unavailable && count === 0 ? '；当前没有符合项' : ''}`,
  };
}

export const VIDEO_DISCOVERY_PRESETS = Object.freeze([
  { key: 'all', label: '全部内容', query: {} },
  { key: 'hot', label: '累计热门', query: { sort: 'interactions', minInteractions: '100' } },
  { key: 'recent', label: '近期热门', query: { sort: 'recent_heat', recentOnly: true } },
  { key: 'keyword_hot', label: '关键词内热门', query: { sort: 'interactions' } },
  { key: 'rising', label: '正在增长', query: { sort: 'growth_per_hour', momentumOnly: true, freshOnly: true } },
  { key: 'standout', label: '表现突出', query: { sort: 'relative_performance', performance: 'standout' } },
  { key: 'review', label: '低表现复核', query: { sort: 'relative_performance', direction: 'asc', performance: 'underperforming' } },
  { key: 'selected', label: '精选内容库', query: { curation: 'selected' } },
]);

export function discoveryPreset(query, key) {
  const preset = VIDEO_DISCOVERY_PRESETS.find(item => item.key === key);
  return { ...query, curation: 'active', performance: '', minInteractions: '', minLikes: '', momentumOnly: false, freshOnly: false, recentOnly: false, sort: 'observed_at', direction: 'desc', page: 1, ...(preset?.query || {}) };
}

export function activeDiscoveryPreset(query) {
  if (query.curation === 'selected') return 'selected';
  if (query.performance === 'underperforming') return 'review';
  if (query.performance === 'standout') return 'standout';
  if (query.momentumOnly) return 'rising';
  if (query.recentOnly && query.sort === 'recent_heat') return 'recent';
  if (query.sort === 'interactions' && Number(query.minInteractions) >= 100) return 'hot';
  if (query.sort === 'interactions' && query.monitorKeyword) return 'keyword_hot';
  return 'all';
}

// Fill calendar gaps without fabricating observations or metric increments.
export function discoveryDays(points = [], reference, count = 14) {
  const valid = points.filter(point => /^\d{4}-\d{2}-\d{2}$/.test(point.date));
  const ref = reference && !String(reference).startsWith('0001-') ? new Date(reference) : new Date();
  if (!Number.isFinite(ref.getTime())) return [];
  const day = ref.toLocaleDateString('en-CA', { timeZone: 'Asia/Shanghai' });
  const end = new Date(`${day}T00:00:00+08:00`).getTime();
  const byDate = new Map(valid.map(point => [point.date, point]));
  return Array.from({ length: count }, (_, index) => {
    const date = new Date(end - (count - index - 1) * 86400000).toLocaleDateString('en-CA', { timeZone: 'Asia/Shanghai' });
    return byDate.get(date) || { date, newVideos: 0, observedVideos: 0 };
  });
}
