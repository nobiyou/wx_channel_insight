import assert from 'node:assert/strict';
import { test } from 'node:test';
import fs from 'node:fs';
import vm from 'node:vm';

function loadAccountView() {
  const source = fs.readFileSync(new URL('../components/PlatformAccountView.vue', import.meta.url), 'utf8');
  const setup = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import .*;\r?$/gm, '');
  const timers = new Map();
  let nextTimer = 0;
  let unmount;
  let nextSession = { status: 'authenticated', finderUsername: 'account-a', clientId: 'device-1' };
  let accountReads = 0;
  let sessionReads = 0;
  let collectResult;
  let lastSignal;
  const sandbox = {
    ref: value => ({ value }), computed: fn => ({ get value() { return fn(); } }),
    defineProps: () => ({}), defineEmits: () => () => {}, defineExpose() {}, onMounted() {},
    onBeforeUnmount(fn) { unmount = fn; }, AbortController, Intl, Date,
    window: {
      setTimeout(fn, delay) { const id = ++nextTimer; timers.set(id, { fn, delay }); return id; },
      clearTimeout(id) { timers.delete(id); },
    },
    async fetchChannelAccountSession({ signal }) { sessionReads++; lastSignal = signal; return nextSession; },
    async fetchAccounts() { accountReads++; return { items: [] }; },
    async openChannelAccountLogin() {},
    collectChannelAccount: async () => collectResult,
  };
  vm.runInNewContext(`${setup}\nglobalThis.state = { session, collection, accountData, selectedVideoId, load, collect };`, sandbox);
  return {
    state: sandbox.state, timers,
    get accountReads() { return accountReads; }, get sessionReads() { return sessionReads; },
    get lastSignal() { return lastSignal; },
    setSession(value) { nextSession = value; }, setCollect(value) { collectResult = value; },
    unmount: () => unmount(),
    tick: async () => { const timer = [...timers.values()][0]; timers.clear(); await timer.fn(); await Promise.resolve(); },
  };
}

test('authenticated sessions continue syncing logout without repeatedly reading saved account data', async () => {
  const env = loadAccountView();
  await env.state.load();
  await env.tick();
  assert.equal(env.sessionReads, 2);
  assert.equal(env.accountReads, 1, 'a successful empty lookup must not be repeated each session poll');
  env.state.collection.value = { itemCount: 12 };
  env.setSession({ status: 'login_required', clientId: 'device-1' });
  await env.tick();
  assert.equal(env.state.session.value.status, 'login_required');
  assert.equal(env.state.collection.value, null);
  assert.equal([...env.timers.values()][0].delay, 5000);
});

test('unmount aborts the session request and retires the polling timer', async () => {
  const env = loadAccountView();
  await env.state.load();
  env.unmount();
  assert.equal(env.lastSignal.aborted, true);
  assert.equal(env.timers.size, 0);
});

test('collection completion for a previous account cannot populate the new account', async () => {
  const env = loadAccountView();
  await env.state.load();
  let resolve;
  env.setCollect(new Promise(done => { resolve = done; }));
  const pending = env.state.collect();
  env.setSession({ status: 'authenticated', finderUsername: 'account-b', clientId: 'device-1' });
  await env.state.load({ background: true });
  resolve({ itemCount: 18 });
  await pending;
  assert.equal(env.state.collection.value, null);
});
