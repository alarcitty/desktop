import { Readable } from 'node:stream'
import { defineConfig, type Plugin } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

const PLAY_DC_UPSTREAM = 'https://play.dreamerscast.com'

function dreamCastMediaProxyPlugin(): Plugin {
  return {
    name: 'dreamcast-media-proxy',
    configureServer(server) {
      server.middlewares.use('/ext/play-dc', async (req, res, next) => {
        if (!req.url) return next()
        if (req.method === 'OPTIONS') {
          res.statusCode = 204
          res.setHeader('Access-Control-Allow-Origin', '*')
          res.end()
          return
        }
        try {
          const suffix = req.url.startsWith('/') ? req.url : `/${req.url}`
          if (suffix.startsWith('//')) {
            res.statusCode = 400
            res.end('invalid path')
            return
          }
          const targetUrl = `${PLAY_DC_UPSTREAM}${suffix}`
          const headers = new Headers()
          if (req.headers.range && typeof req.headers.range === 'string') {
            headers.set('Range', req.headers.range)
          }
          const upstream = await fetch(targetUrl, { method: req.method || 'GET', headers })
          const ct = upstream.headers.get('content-type') || ''
          res.statusCode = upstream.status
          res.setHeader('Access-Control-Allow-Origin', '*')
          if (ct) res.setHeader('Content-Type', ct)
          const cc = upstream.headers.get('cache-control')
          if (cc) res.setHeader('Cache-Control', cc)
          const cr = upstream.headers.get('content-range')
          if (cr) res.setHeader('Content-Range', cr)
          const ar = upstream.headers.get('accept-ranges')
          if (ar) res.setHeader('Accept-Ranges', ar)

          const isM3U8 =
            suffix.toLowerCase().includes('.m3u8') || ct.toLowerCase().includes('mpegurl')
          if (isM3U8 && upstream.ok) {
            const text = (await upstream.text())
              .replaceAll(`${PLAY_DC_UPSTREAM}/`, '/ext/play-dc/')
              .replaceAll('http://play.dreamerscast.com/', '/ext/play-dc/')
            res.end(text)
            return
          }
          if (!upstream.body) {
            res.end()
            return
          }
          Readable.fromWeb(upstream.body as never).pipe(res)
        } catch (err) {
          res.statusCode = 502
          res.end(err instanceof Error ? err.message : 'proxy error')
        }
      })
    }
  }
}

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => ({
  plugins: [svelte(), dreamCastMediaProxyPlugin()],
  esbuild: {
    legalComments: 'none',
    drop: mode === 'production' ? ['console', 'debugger'] : []
  },
  build: {
    target: 'esnext',
    minify: 'esbuild',
    cssMinify: true,
    sourcemap: false,
    reportCompressedSize: false,
    chunkSizeWarningLimit: 700,
    modulePreload: {
      polyfill: false
    },
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/hls.js')) {
            return 'hls'
          }
          if (
            id.includes('node_modules/lucide') ||
            id.includes('node_modules/@lucide') ||
            id.includes('node_modules/morphicons')
          ) {
            return 'icons'
          }
          if (id.includes('node_modules/svelte')) {
            return 'vendor-svelte'
          }
        }
      }
    }
  },
  server: {
    port: 5173,
    strictPort: true,
    host: 'localhost',
    proxy: {
      '/ext/animevost': {
        target: 'https://api.animevost.org/v1',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/ext\/animevost/, '')
      },
      '/ext/dreamerscast': {
        target: 'https://dreamerscast.com',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/ext\/dreamerscast/, '') || '/'
      },
      '/ext/anilibria': {
        target: 'https://anilibria.top/api/v1',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/ext\/anilibria/, '') || '/'
      },
      '/api/v1': {
        target: 'http://localhost:8080',
        changeOrigin: true
      },
      '/health': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  }
}))
