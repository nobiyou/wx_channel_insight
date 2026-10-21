import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  applyTrustedBridgeHeaders,
  resolveTrustedBridgeProxyConfig,
  trustedBridgeProxyHeaderNames
} from './trustedBridgeProxy.js'

function createProxyReq(seed = {}) {
  const headers = { ...seed }
  return {
    headers,
    setHeader(name, value) {
      headers[name] = value
    },
    removeHeader(name) {
      delete headers[name]
    }
  }
}

test('resolveTrustedBridgeProxyConfig trims env values', () => {
  assert.deepEqual(
    resolveTrustedBridgeProxyConfig({
      INSIGHT_WEB_TRUSTED_BRIDGE_TOKEN: ' bridge-secret ',
      INSIGHT_WEB_TRUSTED_HUB_USER_ID: ' hub:user:alice ',
      INSIGHT_WEB_TRUSTED_DEVICE_COUNT: ' 9 '
    }),
    {
      trustedBridgeToken: 'bridge-secret',
      trustedHubUserID: 'hub:user:alice',
      trustedDeviceCount: '9'
    }
  )
})

test('applyTrustedBridgeHeaders strips browser supplied bridge headers by default', () => {
  const proxyReq = createProxyReq({
    [trustedBridgeProxyHeaderNames.token]: 'browser-token',
    [trustedBridgeProxyHeaderNames.hubUserID]: 'hub:user:bob',
    [trustedBridgeProxyHeaderNames.deviceCount]: '4'
  })

  applyTrustedBridgeHeaders(proxyReq, {})

  assert.deepEqual(proxyReq.headers, {})
})

test('applyTrustedBridgeHeaders injects only trusted proxy env values', () => {
  const proxyReq = createProxyReq({
    [trustedBridgeProxyHeaderNames.token]: 'browser-token',
    [trustedBridgeProxyHeaderNames.hubUserID]: 'hub:user:bob',
    [trustedBridgeProxyHeaderNames.deviceCount]: '4'
  })

  applyTrustedBridgeHeaders(proxyReq, {
    INSIGHT_WEB_TRUSTED_BRIDGE_TOKEN: 'bridge-secret',
    INSIGHT_WEB_TRUSTED_HUB_USER_ID: 'hub:user:alice',
    INSIGHT_WEB_TRUSTED_DEVICE_COUNT: '9'
  })

  assert.deepEqual(proxyReq.headers, {
    [trustedBridgeProxyHeaderNames.token]: 'bridge-secret',
    [trustedBridgeProxyHeaderNames.hubUserID]: 'hub:user:alice',
    [trustedBridgeProxyHeaderNames.deviceCount]: '9'
  })
})
