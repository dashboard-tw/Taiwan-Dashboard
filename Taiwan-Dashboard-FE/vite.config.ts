import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

const backendUrl = process.env.DASHBOARD_BE_URL ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [vue({ template: { compilerOptions: { comments: false } } })],
  optimizeDeps: { exclude: ['maplibre-gl'] },
  server: {
    host: '127.0.0.1',
    port: 5173,
    proxy: {
      '/api': { target: backendUrl, changeOrigin: true },
      '/downloads': { target: backendUrl, changeOrigin: true },
    },
  },
})
