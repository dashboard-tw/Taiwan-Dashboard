import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

const backendUrl = process.env.DASHBOARD_BE_URL ?? 'http://127.0.0.1:8080'

const apexchartsRem = {
  postcssPlugin: 'apexcharts-rem',
  Declaration: {
    'font-size': (decl: { value: string; source?: { input: { file?: string } } }) => {
      if (!decl.source?.input.file?.includes('apexcharts')) return
      decl.value = decl.value.replace(/([\d.]+)px/g, (_, n: string) => `${Number(n) / 16}rem`)
    },
  },
}

export default defineConfig({
  plugins: [vue({ template: { compilerOptions: { comments: false } } })],
  optimizeDeps: { exclude: ['maplibre-gl'] },
  css: { postcss: { plugins: [apexchartsRem] } },
  server: {
    host: '127.0.0.1',
    port: 5173,
    proxy: {
      '/api': { target: backendUrl, changeOrigin: true },
      '/downloads': { target: backendUrl, changeOrigin: true },
    },
  },
})
