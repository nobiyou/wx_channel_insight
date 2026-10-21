import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  buildRequestHeaders,
  readContextFormFromURL,
  setLiveRequestContextPolicy
} from './requestContext.js'

test.afterEach?.(() => {
  setLiveRequestContextPolicy('browser-simulated')
})

test('readContextFormFromURL parses query and header simulation parameters', () => {
  const result = readContextFormFromURL('?hub_user_id=hub:user:bob&device_count=4&header_hub_user_id=hub:user:carol&header_device_count=11')

  assert.deepEqual(result, {
    queryHubUserID: 'hub:user:bob',
    queryDeviceCount: '4',
    headerEnabled: true,
    headerHubUserID: 'hub:user:carol',
    headerDeviceCount: '11'
  })
})

test('buildRequestHeaders only emits enabled header values', () => {
  assert.deepEqual(
    buildRequestHeaders('?hub_user_id=hub:user:bob&device_count=4&header_hub_user_id=hub:user:carol&header_device_count=11'),
    {
      'X-Hub-User-ID': 'hub:user:carol',
      'X-Hub-Device-Count': '11'
    }
  )

  assert.deepEqual(buildRequestHeaders('?hub_user_id=hub:user:bob&device_count=4'), {})
})

test('trusted bridge only mode suppresses browser simulated context for /api/v1/*', () => {
  setLiveRequestContextPolicy('trusted-bridge-only')

  assert.deepEqual(buildRequestHeaders('/api/v1/dashboard', '?header_hub_user_id=hub:user:carol&header_device_count=11'), {})
  assert.deepEqual(
    buildRequestHeaders('/diagnostics', '?header_hub_user_id=hub:user:carol&header_device_count=11'),
    {
      'X-Hub-User-ID': 'hub:user:carol',
      'X-Hub-Device-Count': '11'
    }
  )
})
