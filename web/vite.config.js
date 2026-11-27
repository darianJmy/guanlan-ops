import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { Agent } from 'node:http'

export default defineConfig({
  plugins: [vue()],
  server: {
    host: '0.0.0.0',
    port: 8080,
    strictPort: true,
    proxy: {
      '/api': {
        target: process.env.VITE_API_TARGET || 'http://127.0.0.1:8081',
        // Reuse API connections instead of forwarding Connection: close to browsers.
        agent: new Agent({ keepAlive: true }),
        changeOrigin: true,
        xfwd: true,
        ws: true,
      },
    },
  },
})
