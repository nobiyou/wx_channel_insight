import path from 'node:path'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { applyTrustedBridgeHeaders } from './src/lib/trustedBridgeProxy.js'

const apiProxyTarget = process.env.INSIGHT_API_PROXY_TARGET || 'http://127.0.0.1:18081'

export default defineConfig({
  plugins: [vue()],
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          const normalizedID = id.split(path.sep).join('/')
          if (!normalizedID.includes('/node_modules/')) return undefined

          if (
            normalizedID.includes('/naive-ui/') ||
            normalizedID.includes('/@css-render/') ||
            normalizedID.includes('/vueuc/') ||
            normalizedID.includes('/vooks/') ||
            normalizedID.includes('/vdirs/') ||
            normalizedID.includes('/treemate/') ||
            normalizedID.includes('/seemly/') ||
            normalizedID.includes('/evtd/') ||
            normalizedID.includes('/async-validator/') ||
            normalizedID.includes('/date-fns/') ||
            normalizedID.includes('/date-fns-tz/') ||
            normalizedID.includes('/highlight.js/') ||
            normalizedID.includes('/lodash')
          ) {
            return 'naive-ui'
          }

          if (normalizedID.includes('/lucide-vue-next/')) {
            return 'icons'
          }

          return 'vendor'
        },
      },
    },
  },
  server: {
    host: '127.0.0.1',
    port: 5176,
    proxy: {
      '/api': {
        target: apiProxyTarget,
        changeOrigin: true,
        configure(proxy) {
          proxy.on('proxyReq', (proxyReq) => {
            applyTrustedBridgeHeaders(proxyReq, process.env)
          })
        }
      }
    }
  }
})
