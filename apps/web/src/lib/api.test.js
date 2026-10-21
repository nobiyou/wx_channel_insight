import { afterEach, test } from 'node:test'
import assert from 'node:assert/strict'
import {
  fetchDataCenter,
  fetchSavedSourceBindings,
  fetchAccounts,
  fetchAccount,
  fetchAccountVideos,
  fetchAccountComments,
  updateAccountProfile,
  startAccountMonitoring,
  backfillAccountComments,
  refreshAccountProfile,
  refreshVideoComments,
  fetchVideoDetail,
  fetchMonitoringTaskRuns,
  fetchMonitoringTasks,
  fetchRuntimeStatus,
  fetchWorkspaceContext,
  fetchEdgeNodes,
  runAIAnalysis,
  fetchAIAnalysisRuns,
  createMonitoringTask,
  archiveMonitoringTask,
  runMonitoringTask,
  retryMonitoringTask,
  searchWXFeedAccounts,
  saveWXFeedListBinding,
  setLiveRequestContextPolicy,
  fetchVideoComments,
  fetchPromptSettings,
  updatePromptSetting,
  resetPromptSetting,
  fetchAIServiceSettings,
  updateAIServiceSettings,
  verifyAIServiceSettings,
  clearAIServiceSettingsKey,
} from './api.js'

const originalFetch = globalThis.fetch
const originalWindow = globalThis.window

test('video workbench requests forward cancellation to fetch', async () => {
  const controller = new AbortController()
  let receivedSignal
  globalThis.fetch = async (_url, options) => {
    receivedSignal = options.signal
    return { ok: true, json: async () => ({ items: [] }) }
  }
  await fetchDataCenter('videos', { curation: 'selected' }, { signal: controller.signal })
  assert.equal(receivedSignal, controller.signal)
})

afterEach(() => {
  globalThis.fetch = originalFetch
  globalThis.window = originalWindow
  setLiveRequestContextPolicy('browser-simulated')
})

function setWindowSearch(search) {
  globalThis.window = {
    location: {
      search,
      href: `http://127.0.0.1:5176/${search}#dashboard`
    }
  }
}

test('fetchVideoDetail requests the encoded video object id without caching', async () => {
  setWindowSearch('')
  let capturedURL = ''
  let capturedOptions = null
  globalThis.fetch = async (url, options = {}) => {
    capturedURL = String(url)
    capturedOptions = options
    return {
      ok: true,
      async json() {
        return { objectId: 'object/1', playback: { status: 'unavailable' } }
      }
    }
  }

  const result = await fetchVideoDetail('object/1')

  assert.equal(capturedURL, '/api/v1/read-models/videos/object%2F1')
  assert.equal(capturedOptions.cache, 'no-store')
  assert.equal(result.objectId, 'object/1')
})

test('fetchVideoComments requests an encoded video id with server-side pagination', async () => {
  setWindowSearch('?hub_user_id=insight:operator:local&device_count=1')
  let capturedURL = ''
  globalThis.fetch = async (url) => {
    capturedURL = String(url)
    return {
      ok: true,
      async json() {
        return { items: [], page: 2, pageSize: 50, total: 0, totalPages: 1 }
      },
    }
  }

  const result = await fetchVideoComments('object/1', { page: 2, pageSize: 50, q: '问题' })

  assert.equal(
    capturedURL,
    '/api/v1/read-models/videos/object%2F1/comments?hub_user_id=insight%3Aoperator%3Alocal&device_count=1&page=2&pageSize=50&q=%E9%97%AE%E9%A2%98',
  )
  assert.equal(result.pageSize, 50)
})

test('fetchRuntimeStatus returns unavailable runtime state when API is down by default', async () => {
  setWindowSearch('')
  globalThis.fetch = async () => {
    throw new Error('runtime offline')
  }

  const result = await fetchRuntimeStatus()

  assert.equal(result.source, 'unavailable')
  assert.equal(result.data, null)
  assert.equal(result.error, 'runtime offline')
})

test('connection failures never become demo or local workspace successes', async () => {
  setWindowSearch('?demo_fallback=1&prototype_fallback=true')
  globalThis.fetch = async () => { throw new Error('API offline') }
  for (const result of [await fetchWorkspaceContext(), await fetchRuntimeStatus()]) {
    assert.equal(result.source, 'unavailable')
    assert.equal(result.data, null)
    assert.equal(result.error, 'API offline')
  }
})

test('connection reads accept cancellation without fabricating successful state', async () => {
  setWindowSearch('')
  const controller = new AbortController()
  const signals = []
  globalThis.fetch = async (_url, options) => {
    signals.push(options.signal)
    throw new DOMException('connection check canceled', 'AbortError')
  }
  const runtime = await fetchRuntimeStatus({ signal: controller.signal })
  const nodes = await fetchEdgeNodes({ signal: controller.signal })
  await assert.rejects(fetchSavedSourceBindings({ signal: controller.signal }), /connection check canceled/)
  assert.equal(runtime.source, 'unavailable')
  assert.equal(nodes.source, 'unavailable')
  assert.deepEqual(signals, [controller.signal, controller.signal, controller.signal])
})

test('prompt settings use the server-owned list and mutation endpoints', async () => {
  setWindowSearch('?hub_user_id=insight:operator:local&device_count=1')
  const requests = []
  globalThis.fetch = async (url, options = {}) => {
    requests.push({
      url: String(url),
      method: String(options.method || 'GET'),
      body: String(options.body || ''),
    })
    return {
      ok: true,
      async json() {
        return { items: [{ key: 'content-analysis', version: 'v2' }] }
      },
    }
  }

  await fetchPromptSettings()
  await updatePromptSetting('content-analysis', {
    systemPrompt: '规则',
    userPromptTemplate: '输入 {{data_json}}',
    enabled: true,
  })
  await resetPromptSetting('content-analysis')

  assert.deepEqual(requests, [
    {
      url: '/api/v1/settings/prompts?hub_user_id=insight%3Aoperator%3Alocal&device_count=1',
      method: 'GET',
      body: '',
    },
    {
      url: '/api/v1/settings/prompts/content-analysis?hub_user_id=insight%3Aoperator%3Alocal&device_count=1',
      method: 'PUT',
      body: JSON.stringify({
        systemPrompt: '规则',
        userPromptTemplate: '输入 {{data_json}}',
        enabled: true,
      }),
    },
    {
      url: '/api/v1/settings/prompts/content-analysis/reset?hub_user_id=insight%3Aoperator%3Alocal&device_count=1',
      method: 'POST',
      body: '',
    },
  ])
})

test('AI service settings use server-owned read, save, verify and clear endpoints', async () => {
  setWindowSearch('?hub_user_id=insight:operator:local&device_count=1')
  const requests = []
  globalThis.fetch = async (url, options = {}) => {
    requests.push({
      url: String(url),
      method: String(options.method || 'GET'),
      body: String(options.body || ''),
    })
    return {
      ok: true,
      async json() {
        return { provider: 'openai', apiKeyConfigured: true }
      },
    }
  }

  const draft = { provider: 'openai', baseUrl: 'https://api.openai.com/v1', model: 'gpt-5.6', timeoutSeconds: 45 }
  await fetchAIServiceSettings()
  await updateAIServiceSettings(draft, { verifyOnSave: true })
  await verifyAIServiceSettings({ ...draft, apiKey: 'draft-secret' })
  await clearAIServiceSettingsKey()

  const suffix = '?hub_user_id=insight%3Aoperator%3Alocal&device_count=1'
  assert.deepEqual(requests, [
    { url: `/api/v1/settings/ai-service${suffix}`, method: 'GET', body: '' },
    { url: `/api/v1/settings/ai-service${suffix}`, method: 'PUT', body: JSON.stringify({ ...draft, verifyOnSave: true }) },
    { url: `/api/v1/settings/ai-service/verify${suffix}`, method: 'POST', body: JSON.stringify({ ...draft, apiKey: 'draft-secret' }) },
    { url: `/api/v1/settings/ai-service/clear-key${suffix}`, method: 'POST', body: '' },
  ])
})

test('data center forwards account filters and video pagination', async () => {
  setWindowSearch('?hub_user_id=insight:operator:local&device_count=1')
  const capturedURLs = []
  globalThis.fetch = async (url) => {
    capturedURLs.push(String(url))
    return {
      ok: true,
      async json() {
        return { items: [], total: 0 }
      }
    }
  }

  await fetchDataCenter('accounts', { q: 'lillydaytoy', page: 1, pageSize: 20, sort: 'video_count', direction: 'desc' })
  await fetchDataCenter('videos', { account: 'lillydaytoy', page: 2, pageSize: 50, sort: 'likes', direction: 'asc' })

  assert.deepEqual(capturedURLs, [
    '/api/v1/read-models/data-center/accounts?hub_user_id=insight%3Aoperator%3Alocal&device_count=1&q=lillydaytoy&page=1&pageSize=20&sort=video_count&direction=desc',
    '/api/v1/read-models/data-center/videos?hub_user_id=insight%3Aoperator%3Alocal&device_count=1&account=lillydaytoy&page=2&pageSize=50&sort=likes&direction=asc',
  ])
})

test('account APIs keep stable account ids and explicit monitoring actions', async () => {
  setWindowSearch('?hub_user_id=insight:operator:local&device_count=1')
  const captured = []
  globalThis.fetch = async (url, options = {}) => {
    captured.push({ url: String(url), method: options.method || 'GET', body: options.body || '' })
    return {
      ok: true,
      async json() {
        return { items: [], account: { accountId: 'acct_1' } }
      },
    }
  }

  await fetchAccounts({ q: '科技', priority: 'true', page: 2, pageSize: 50 })
  await fetchAccount('acct/1')
  await updateAccountProfile('acct/1', { region: '华东', isPriority: true })
  await fetchAccountVideos('acct/1', { page: 2, pageSize: 50, sort: 'likes', direction: 'asc' })
  await fetchAccountComments('acct/1', { page: 1, q: '问题' })
  await startAccountMonitoring('acct/1', { commentMode: 'incremental' })
  await backfillAccountComments('acct/1')
  await refreshAccountProfile('acct/1')
  await refreshVideoComments('video/1')

  assert.deepEqual(captured, [
    {
      url: '/api/v1/accounts?hub_user_id=insight%3Aoperator%3Alocal&device_count=1&q=%E7%A7%91%E6%8A%80&priority=true&page=2&pageSize=50',
      method: 'GET',
      body: '',
    },
    { url: '/api/v1/accounts/acct%2F1?hub_user_id=insight%3Aoperator%3Alocal&device_count=1', method: 'GET', body: '' },
    { url: '/api/v1/accounts/acct%2F1?hub_user_id=insight%3Aoperator%3Alocal&device_count=1', method: 'PATCH', body: JSON.stringify({ region: '华东', isPriority: true }) },
    {
      url: '/api/v1/accounts/acct%2F1/videos?hub_user_id=insight%3Aoperator%3Alocal&device_count=1&page=2&pageSize=50&sort=likes&direction=asc',
      method: 'GET',
      body: '',
    },
    {
      url: '/api/v1/accounts/acct%2F1/comments?hub_user_id=insight%3Aoperator%3Alocal&device_count=1&page=1&q=%E9%97%AE%E9%A2%98',
      method: 'GET',
      body: '',
    },
    { url: '/api/v1/accounts/acct%2F1/monitoring?hub_user_id=insight%3Aoperator%3Alocal&device_count=1', method: 'POST', body: JSON.stringify({ commentMode: 'incremental' }) },
    { url: '/api/v1/accounts/acct%2F1/comments/backfill?hub_user_id=insight%3Aoperator%3Alocal&device_count=1', method: 'POST', body: '' },
    { url: '/api/v1/accounts/acct%2F1/profile?hub_user_id=insight%3Aoperator%3Alocal&device_count=1', method: 'POST', body: '' },
    { url: '/api/v1/read-models/videos/video%2F1/comments/refresh?hub_user_id=insight%3Aoperator%3Alocal&device_count=1', method: 'POST', body: '' },
  ])
})

test('saveWXFeedListBinding posts saved source payload to the dedicated endpoint', async () => {
  setWindowSearch('')

  let capturedURL = ''
  let capturedMethod = ''
  let capturedBody = ''
  globalThis.fetch = async (url, options = {}) => {
    capturedURL = String(url)
    capturedMethod = String(options.method || '')
    capturedBody = String(options.body || '')
    return {
      ok: true,
      async json() {
        return { state: 'idle', bindingStates: [] }
      }
    }
  }

  await saveWXFeedListBinding('http://127.0.0.1:2026', 'gh_saved')

  assert.equal(capturedURL, '/api/v1/ingest/wx-channel/feed-list/save')
  assert.equal(capturedMethod, 'POST')
  assert.equal(
    capturedBody,
    JSON.stringify({ baseUrl: 'http://127.0.0.1:2026', username: 'gh_saved' }),
  )
})

test('fetchSavedSourceBindings reads only persisted source bindings', async () => {
  setWindowSearch('')

  let capturedURL = ''
  globalThis.fetch = async (url) => {
    capturedURL = String(url)
    return {
      ok: true,
      async json() {
        return { items: [{ source: 'wx_feed_list', origin: 'manual' }] }
      }
    }
  }

  const items = await fetchSavedSourceBindings()

  assert.equal(capturedURL, '/api/v1/ingest/bindings')
  assert.deepEqual(items, [{ source: 'wx_feed_list', origin: 'manual' }])
})

test('searchWXFeedAccounts requests finder candidates through the dedicated endpoint', async () => {
  setWindowSearch('')

  let capturedURL = ''
  let capturedMethod = ''
  globalThis.fetch = async (url, options = {}) => {
    capturedURL = String(url)
    capturedMethod = String(options.method || 'GET')
    return {
      ok: true,
      async json() {
        return { items: [] }
      }
    }
  }

  await searchWXFeedAccounts('http://127.0.0.1:2026', 'teamlight')

  assert.equal(
    capturedURL,
    '/api/v1/ingest/wx-channel/feed-list/search?base_url=http%3A%2F%2F127.0.0.1%3A2026&keyword=teamlight',
  )
  assert.equal(capturedMethod, 'GET')
})

test('monitoring task list forwards the archive filter and returns task payload', async () => {
  setWindowSearch('?hub_user_id=insight:operator:tasks')

  let capturedURL = ''
  globalThis.fetch = async (url) => {
    capturedURL = String(url)
    return {
      ok: true,
      async json() {
        return { items: [{ id: 'task-1', name: '重点账号' }] }
      }
    }
  }

  const payload = await fetchMonitoringTasks(true)

  assert.equal(
    capturedURL,
    '/api/v1/monitoring/tasks?hub_user_id=insight%3Aoperator%3Atasks&includeArchived=true',
  )
  assert.equal(payload.items[0].id, 'task-1')
})

test('monitoring task mutations use JSON and encode task ids', async () => {
  setWindowSearch('')

  const requests = []
  globalThis.fetch = async (url, options = {}) => {
    requests.push({ url: String(url), method: String(options.method || 'GET'), body: String(options.body || '') })
    return {
      ok: true,
      async json() {
        return { run: { id: 'run-1', status: 'success' }, items: [] }
      }
    }
  }

  await createMonitoringTask({ name: '重点账号', interval: '15m' })
  await runMonitoringTask('task/with slash')
  await retryMonitoringTask('task/with slash')
  await archiveMonitoringTask('task/with slash')
  await fetchMonitoringTaskRuns('task/with slash', 3)

  assert.deepEqual(requests.slice(0, 4), [
    {
      url: '/api/v1/monitoring/tasks',
      method: 'POST',
      body: JSON.stringify({ name: '重点账号', interval: '15m' }),
    },
    {
      url: '/api/v1/monitoring/tasks/task%2Fwith%20slash/run',
      method: 'POST',
      body: '',
    },
    {
      url: '/api/v1/monitoring/tasks/task%2Fwith%20slash/retry',
      method: 'POST',
      body: '',
    },
    {
      url: '/api/v1/monitoring/tasks/task%2Fwith%20slash',
      method: 'DELETE',
      body: '',
    },
  ])
  assert.equal(
    requests[4].url,
    '/api/v1/monitoring/tasks/task%2Fwith%20slash/runs?limit=3',
  )
})

test('AI analysis helpers preserve the workspace context and bounded request shape', async () => {
  setWindowSearch('?hub_user_id=insight:operator:local&device_count=1')
  const requests = []
  globalThis.fetch = async (url, options = {}) => {
    requests.push({
      url: String(url),
      method: String(options.method || 'GET'),
      body: String(options.body || ''),
    })
    return {
      ok: true,
      async json() {
        return { items: [], status: 'completed' }
      },
    }
  }

  await runAIAnalysis('content-analysis', {
    scopeKey: 'videos',
    window: '24h',
    variables: { data_json: { count: 2 }, video_json: { id: 'video-1' } },
  })
  await fetchAIAnalysisRuns('content-analysis', { scopeKey: 'videos', window: '24h', limit: 99 })

  const suffix = '?hub_user_id=insight%3Aoperator%3Alocal&device_count=1'
  assert.deepEqual(requests, [
    {
      url: `/api/v1/ai-analysis/content-analysis${suffix}`,
      method: 'POST',
      body: JSON.stringify({
        scopeKey: 'videos',
        window: '24h',
        variables: { data_json: { count: 2 }, video_json: { id: 'video-1' } },
      }),
    },
    {
      url: `/api/v1/ai-analysis/content-analysis${suffix}&scopeKey=videos&window=24h&limit=20`,
      method: 'GET',
      body: '',
    },
  ])
})
