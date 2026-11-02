import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In dev, the Go server (token cookie, SSE) stays the single origin for /api.
export default defineConfig({
  plugins: [react()],
  server: { host: true, proxy: { '/api': { target: 'http://localhost:7777', changeOrigin: true } } },
  build: { outDir: '../backend/web/dist', emptyOutDir: true },
})
