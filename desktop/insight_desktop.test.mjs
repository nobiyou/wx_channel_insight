import assert from 'node:assert/strict';
import { test } from 'node:test';
import vm from 'node:vm';
import { InsightDesktopBridge } from './insight_desktop.mjs';

test('login reuses and activates the existing official browser session', async () => {
  const bridge = new InsightDesktopBridge();
  const calls = [];
  bridge.findChannelPage = async () => ({
    navigate: async url => calls.push(url),
    command: async method => calls.push(method),
  });
  await bridge.openLogin();
  assert.deepEqual(calls, ['https://channels.weixin.qq.com/platform/login', 'Page.bringToFront']);
});

test('failed login navigation is reported as failure instead of accepted', async () => {
  const bridge = new InsightDesktopBridge();
  bridge.openLogin = async () => { throw new Error('page navigation failed'); };
  const replies = [];
  bridge.respond = (...args) => replies.push(args);
  await bridge.handleEdgeMessage(JSON.stringify({ id: 'request-1', type: 'command', payload: { action: 'channel_navigate' } }));
  assert.equal(replies[0][0], 'request-1');
  assert.equal(replies[0][1], false);
  assert.match(replies[0][3], /page navigation failed/);
});

function simulatedPage(path, username = '', text = '视频号ID：viewed-creator') {
  return {
    evaluate: async source => vm.runInNewContext(source, {
      window: { __wx_username: username },
      document: {
        hidden: false, body: { innerText: text },
        querySelectorAll() { throw new Error('QR extraction is retired'); },
      },
      location: new URL(`https://channels.weixin.qq.com${path}`),
      Date,
    }),
  };
}

test('official page state synchronizes account identity without extracting QR images', async () => {
  const bridge = new InsightDesktopBridge();
  bridge.findChannelPage = async () => simulatedPage('/platform/post/list', 'own-account');
  const state = await bridge.refreshPageState();
  assert.equal(state.finderUsername, 'own-account');
  assert.equal(state.status, 'authenticated');
  assert.ok(!('loginQRCode' in state));
  assert.ok(!('loginQRCodeURL' in state));
});

test('returning to login and visiting another creator cannot reuse an old identity', async () => {
  const bridge = new InsightDesktopBridge();
  bridge.lastState.finderUsername = 'old-account';
  bridge.findChannelPage = async () => simulatedPage('/platform/login', 'old-account');
  assert.equal((await bridge.refreshPageState()).finderUsername, '');
  bridge.findChannelPage = async () => simulatedPage('/web/pages/profile');
  assert.equal((await bridge.refreshPageState()).finderUsername, '');
});

test('CDP failure clears authentication rather than indefinitely publishing the old account', async () => {
  const bridge = new InsightDesktopBridge();
  bridge.lastState.finderUsername = 'old-account';
  bridge.findChannelPage = async () => { throw new Error('CDP unavailable'); };
  await assert.rejects(bridge.refreshPageState(), /CDP unavailable/);
  assert.equal(bridge.lastState.finderUsername, '');
  assert.equal(bridge.lastState.apiReady, false);
});
