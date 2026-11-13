import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  server: {
    port: 5173,
    strictPort: true,
    host: 'localhost',
    proxy: {
      '/ext/animevost': {
        target: 'https://api.animevost.org/v1',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/ext\/animevost/, '')
      }
    }
  }
})
