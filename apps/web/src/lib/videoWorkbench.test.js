import test from 'node:test';
import assert from 'node:assert/strict';
import { discoveryPreset, activeDiscoveryPreset, discoveryDays, assessmentPresentation, keywordRankPresentation } from './videoWorkbench.js';
import { videoFactsQuery, buildOperationsHash, parseOperationsRoute } from './operationsNavigation.js';

test('changing discovery intent resets contradictory quality and library filters', () => {
  const query = discoveryPreset({ account: 'a', monitorKeyword: '秋天', page: 4, curation: 'excluded', performance: 'underperforming', minLikes: '500' }, 'rising');
  assert.equal(query.page, 1);
  assert.equal(query.account, 'a');
  assert.equal(query.monitorKeyword, '秋天');
  assert.equal(query.curation, 'active');
  assert.equal(query.performance, '');
  assert.equal(query.minLikes, '');
  assert.equal(query.freshOnly, true);
  assert.equal(activeDiscoveryPreset(query), 'rising');
});

test('decisions, thresholds and discovery intent survive a detail return route', () => {
  const hash = buildOperationsHash('videos', { curation: 'selected', minInteractions: '100', minLikes: '30', tag: '可复用', freshOnly: true, momentumOnly: false, sort: 'relative_performance', performance: 'standout' });
  const query = videoFactsQuery(parseOperationsRoute(hash).params);
  assert.equal(query.curation, 'selected');
  assert.equal(query.sort, 'relative_performance');
  assert.equal(query.freshOnly, true);
  assert.equal(query.momentumOnly, false);
  assert.equal(query.tag, '可复用');
  assert.equal(query.minLikes, '30');
  assert.equal(videoFactsQuery({ minInteractions: '-1' }).minInteractions, '');
});

test('discovery chart uses Shanghai dates, fills gaps and counts real observations only', () => {
  const days = discoveryDays([{ date: '2026-10-17', newVideos: 2, observedVideos: 9 }], '2026-10-17T17:00:00Z', 3);
  assert.deepEqual(days.map(day => day.date), ['2026-10-16', '2026-10-17', '2026-10-18']);
  assert.deepEqual(days.map(day => day.newVideos), [0, 2, 0]);
  assert.deepEqual(discoveryDays([], 'invalid'), []);
});

test('assessment displays zero only when qualified samples were actually assessed', () => {
  const stale = assessmentPresentation({ state: 'stale', eligibleVideos: 0, reason: '指标超过 24 小时未更新' }, 0);
  assert.equal(stale.value, '待更新');
  assert.equal(stale.unavailable, true);
  assert.match(stale.hint, /24 小时/);
  assert.equal(assessmentPresentation({ state: 'insufficient_data', reason: '同组样本不足' }, 0).value, '依据不足');
  assert.equal(assessmentPresentation({ state: 'empty', reason: '当前范围没有视频' }, 0).value, '暂无样本');
  assert.equal(assessmentPresentation(undefined, 0).value, '待评估');
  const zero = assessmentPresentation({ state: 'available', eligibleVideos: 8, reason: '8 条视频可评估' }, 0);
  assert.equal(zero.value, '0');
  assert.equal(zero.unavailable, false);
  assert.match(zero.hint, /没有符合项/);
});

test('historical standout remains a historical reference without implying current performance', () => {
  const result = assessmentPresentation({ state: 'historical', eligibleVideos: 7000, reason: '指标已过期，仅供历史复盘' }, 6568);
  assert.equal(result.value, '6,568');
  assert.equal(result.historical, true);
  assert.match(result.hint, /历史/);
  assert.equal(assessmentPresentation({ state: 'historical', reason: '历史参考' }, 0).value, '0');
});

test('recent and keyword discovery preserve scope and survive returning from details', () => {
  const recent = discoveryPreset({ monitorKeyword: '秋天', performance: 'standout', curation: 'selected', minInteractions: '500' }, 'recent');
  assert.equal(recent.recentOnly, true);
  assert.equal(recent.sort, 'recent_heat');
  assert.equal(recent.performance, '');
  const hash = buildOperationsHash('videos', { ...recent, keyword: recent.monitorKeyword });
  const restored = videoFactsQuery(parseOperationsRoute(hash).params);
  assert.equal(restored.recentOnly, true);
  assert.equal(restored.sort, 'recent_heat');
  assert.equal(activeDiscoveryPreset(restored), 'recent');
  const keyword = discoveryPreset(recent, 'keyword_hot');
  assert.equal(keyword.recentOnly, false);
  assert.equal(keyword.monitorKeyword, '秋天');
  assert.equal(keyword.sort, 'interactions');
  assert.equal(activeDiscoveryPreset(keyword), 'keyword_hot');
});

test('keyword rank presentation keeps historical, tied-zero and insufficient samples explicit', () => {
  const rank = { keyword: '秋天', rank: 3, sampleCount: 1247, percentile: 99.6, state: 'historical', reason: '样本含过期数据' };
  const display = keywordRankPresentation(rank);
  assert.match(display.label, /3.*1,247/);
  assert.equal(display.historical, true);
  assert.match(display.detail, /99.6%/);
  assert.match(keywordRankPresentation({ ...rank, rank: 1, percentile: 0 }).detail, /0%/);
  const small = keywordRankPresentation({ keyword: '小词', rank: null, sampleCount: 2, state: 'insufficient_data', reason: '至少 5 条才排名' });
  assert.match(small.label, /依据不足/);
  assert.match(small.detail, /5/);
});
