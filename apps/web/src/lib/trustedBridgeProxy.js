export const trustedBridgeProxyHeaderNames = {
  token: 'X-Insight-Bridge-Token',
  hubUserID: 'X-Hub-User-ID',
  deviceCount: 'X-Hub-Device-Count'
}

function readEnv(env, key) {
  return String(env?.[key] ?? '').trim()
}

export function resolveTrustedBridgeProxyConfig(env = {}) {
  return {
    trustedBridgeToken: readEnv(env, 'INSIGHT_WEB_TRUSTED_BRIDGE_TOKEN'),
    trustedHubUserID: readEnv(env, 'INSIGHT_WEB_TRUSTED_HUB_USER_ID'),
    trustedDeviceCount: readEnv(env, 'INSIGHT_WEB_TRUSTED_DEVICE_COUNT')
  }
}

export function applyTrustedBridgeHeaders(proxyReq, env = {}) {
  const config = resolveTrustedBridgeProxyConfig(env)

  proxyReq.removeHeader(trustedBridgeProxyHeaderNames.token)
  proxyReq.removeHeader(trustedBridgeProxyHeaderNames.hubUserID)
  proxyReq.removeHeader(trustedBridgeProxyHeaderNames.deviceCount)

  if (config.trustedBridgeToken) {
    proxyReq.setHeader(trustedBridgeProxyHeaderNames.token, config.trustedBridgeToken)
  }

  if (!config.trustedHubUserID) {
    return
  }

  proxyReq.setHeader(trustedBridgeProxyHeaderNames.hubUserID, config.trustedHubUserID)
  if (config.trustedDeviceCount) {
    proxyReq.setHeader(trustedBridgeProxyHeaderNames.deviceCount, config.trustedDeviceCount)
  }
}
