import { afterEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { exportVideoTable, setLiveRequestContextPolicy } from './api.js';

const originalFetch = globalThis.fetch;
const originalWindow = globalThis.window;
afterEach(() => {
  globalThis.fetch = originalFetch;
  globalThis.window = originalWindow;
  setLiveRequestContextPolicy('browser-simulated');
});

test('video CSV export forwards applied filters, selected IDs, context and cancellation', async () => {
  globalThis.window = { location: { search: '?hub_user_id=operator&device_count=1' } };
  const controller = new AbortController();
  const blob = new Blob(['\ufeff标题,点赞\r\n示例,0\r\n'], { type: 'text/csv' });
  let request;
  globalThis.fetch = async (url, options) => {
    request = { url: String(url), ...options };
    return {
      ok: true,
      headers: new Headers({ 'content-type': 'text/csv; charset=utf-8', 'content-disposition': 'attachment; filename="insight-videos-demo.csv"' }),
      blob: async () => blob,
    };
  };
  const result = await exportVideoTable({ account: '账号A', q: 'AI', sort: 'likes', direction: 'desc', page: 2, refresh: true }, { scope: 'selected', ids: ['video:one'], signal: controller.signal });
  const url = new URL(request.url, 'http://localhost');
  assert.equal(url.pathname, '/api/v1/read-models/data-center/videos/export');
  assert.equal(url.searchParams.get('hub_user_id'), 'operator');
  assert.equal(url.searchParams.get('account'), '账号A');
  assert.equal(url.searchParams.get('sort'), 'likes');
  assert.equal(url.searchParams.has('page'), false);
  assert.equal(url.searchParams.has('refresh'), false);
  assert.equal(request.method, 'POST');
  assert.equal(request.cache, 'no-store');
  assert.equal(request.signal, controller.signal);
  assert.deepEqual(JSON.parse(request.body), { scope: 'selected', ids: ['video:one'] });
  assert.equal(result.blob, blob);
  assert.equal(result.filename, 'insight-videos-demo.csv');
});

test('video export uses trusted context policy and returns API failures instead of files', async () => {
  globalThis.window = { location: { search: '?hub_user_id=untrusted' } };
  setLiveRequestContextPolicy('trusted-bridge-only');
  globalThis.fetch = async (url, options) => {
    assert.equal(new URL(url, 'http://localhost').searchParams.has('hub_user_id'), false);
    assert.deepEqual(JSON.parse(options.body), { scope: 'filtered', ids: [] });
    return { ok: false, status: 403, json: async () => ({ error: '没有导出权限' }) };
  };
  await assert.rejects(exportVideoTable({}, { scope: 'filtered' }), /没有导出权限/);
});

test('video export rejects unexpected successful HTML responses', async () => {
  globalThis.fetch = async () => ({ ok: true, headers: new Headers({ 'content-type': 'text/html' }) });
  await assert.rejects(exportVideoTable({}), /导出.*格式/);
});
