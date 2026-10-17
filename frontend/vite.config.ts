import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// In dev the Go server runs on :8090; proxy REST and the simulation WebSocket to it.
const backend = process.env.BACKEND_URL ?? 'http://localhost:8090'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: backend, changeOrigin: true, ws: true },
    },
  },
  build: { outDir: 'dist', sourcemap: true },
  test: { environment: 'jsdom', include: ['src/**/*.test.ts'] },
})
