import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Signet frontend — Vue 3 SPA consuming the Go backend's /api/v1/* REST API.
// Dev server proxies /api and /storage to the Go backend so cookies/session
// auth work without CORS gymnastics in local dev.
export default defineConfig({
  plugins: [vue(), {
    name: 'local-registration-preview',
    apply: 'serve',
    configureServer(server) {
      if (process.env.LOCAL_REGISTRATION_PREVIEW !== 'true') return
      server.middlewares.use((req, res, next) => {
        if (!req.url?.startsWith('/api/')) return next()
        res.setHeader('Content-Type', 'application/json')
        res.setHeader('Cache-Control', 'no-store')
        const url = new URL(req.url, 'http://localhost')
        if (req.method === 'GET' && url.pathname === '/api/v1/register/step2/local-demo') {
          res.end(JSON.stringify({ status: 'success', blockchain_enabled: true, id: 'local-demo',
            packages: [1000, 2500, 5000, 10000, 25000, 50000, 100000, 500000, 1000000, 500, 100]
              .map(price => ({ id: `demo-${price}`, name: `Demo package ${price}`, price })) }))
          return
        }
        if (req.method === 'GET' && url.pathname === '/api/v1/register/referral' && url.searchParams.get('ref') === 'LOCAL-DEMO') {
          res.end(JSON.stringify({ status: 'success', blockchain_enabled: true,
            countries: [{ id: 1, code: 'AE', name: 'United Arab Emirates' }, { id: 2, code: 'LK', name: 'Sri Lanka' }], leaders: [] }))
          return
        }
        res.statusCode = url.pathname.endsWith('/me') ? 401 : 403
        res.end(JSON.stringify({ message: 'Local visual preview only. No account is created, no email is sent and no blockchain transaction is submitted.' }))
      })
    },
  }],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
      '/storage': {
        target: process.env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
})
