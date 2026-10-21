const queryHubUserIDKey = 'hub_user_id'
const queryDeviceCountKey = 'device_count'
const headerHubUserIDKey = 'header_hub_user_id'
const headerDeviceCountKey = 'header_device_count'
const liveRequestContextPolicies = {
  browserSimulated: 'browser-simulated',
  trustedBridgeOnly: 'trusted-bridge-only'
}

let liveRequestContextPolicy = liveRequestContextPolicies.browserSimulated

function currentSearch() {
  if (typeof window !== 'undefined') {
    return window.location.search
  }
  return ''
}

function normalizeText(value) {
  return String(value ?? '').trim()
}

function normalizeDeviceCount(value) {
  const normalized = normalizeText(value)
  return /^\d+$/.test(normalized) ? normalized : ''
}

function resolveRequestShape(pathOrSearch = '', search = currentSearch()) {
  const normalized = normalizeText(pathOrSearch)
  if (!normalized) {
    return { path: '', search }
  }
  if (normalized.startsWith('/')) {
    return { path: normalized, search }
  }
  if (normalized.startsWith('?')) {
    return { path: '', search: normalized }
  }
  return { path: '', search: normalized }
}

function shouldUseBrowserRequestContext(path = '') {
  return !(
    liveRequestContextPolicy === liveRequestContextPolicies.trustedBridgeOnly &&
    normalizeText(path).startsWith('/api/v1/')
  )
}

export function setLiveRequestContextPolicy(
  policy = liveRequestContextPolicies.browserSimulated
) {
  liveRequestContextPolicy =
    policy === liveRequestContextPolicies.trustedBridgeOnly
      ? liveRequestContextPolicies.trustedBridgeOnly
      : liveRequestContextPolicies.browserSimulated
}

export function normalizeContextForm(form = {}) {
  const queryHubUserID = normalizeText(form.queryHubUserID)
  let queryDeviceCount = normalizeDeviceCount(form.queryDeviceCount)
  let headerHubUserID = normalizeText(form.headerHubUserID)
  let headerDeviceCount = normalizeDeviceCount(form.headerDeviceCount)
  const headerEnabled = Boolean(form.headerEnabled)

  if (!queryHubUserID) {
    queryDeviceCount = ''
  }

  if (!headerEnabled) {
    headerHubUserID = ''
    headerDeviceCount = ''
  }

  if (!headerHubUserID) {
    headerDeviceCount = ''
  }

  return {
    queryHubUserID,
    queryDeviceCount,
    headerEnabled: headerEnabled && headerHubUserID !== '',
    headerHubUserID,
    headerDeviceCount
  }
}

export function readContextFormFromURL(search = currentSearch()) {
  const params = new URLSearchParams(search)
  const queryHubUserID = normalizeText(params.get(queryHubUserIDKey))
  const queryDeviceCount = normalizeDeviceCount(params.get(queryDeviceCountKey))
  const headerHubUserID = normalizeText(params.get(headerHubUserIDKey))
  const headerDeviceCount = normalizeDeviceCount(params.get(headerDeviceCountKey))

  return normalizeContextForm({
    queryHubUserID,
    queryDeviceCount,
    headerEnabled: headerHubUserID !== '' || headerDeviceCount !== '',
    headerHubUserID,
    headerDeviceCount
  })
}

export function buildAPIURL(path, search = currentSearch()) {
  if (!shouldUseBrowserRequestContext(path)) {
    return path
  }

  const context = readContextFormFromURL(search)
  const params = new URLSearchParams()

  if (context.queryHubUserID) {
    params.set(queryHubUserIDKey, context.queryHubUserID)
  }
  if (context.queryHubUserID && context.queryDeviceCount) {
    params.set(queryDeviceCountKey, context.queryDeviceCount)
  }

  const query = params.toString()
  return query ? `${path}?${query}` : path
}

export function buildRequestHeaders(pathOrSearch = '', search = currentSearch()) {
  const request = resolveRequestShape(pathOrSearch, search)
  if (!shouldUseBrowserRequestContext(request.path)) {
    return {}
  }

  const context = readContextFormFromURL(request.search)
  const headers = {}

  if (context.headerEnabled && context.headerHubUserID) {
    headers['X-Hub-User-ID'] = context.headerHubUserID
  }
  if (context.headerEnabled && context.headerHubUserID && context.headerDeviceCount) {
    headers['X-Hub-Device-Count'] = context.headerDeviceCount
  }

  return headers
}
