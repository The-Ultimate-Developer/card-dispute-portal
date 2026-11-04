import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// In development the browser talks only to Vite; /api is proxied to the Go API,
// so the session cookie is same-origin exactly as it is behind Netlify in production.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: { '/api': 'http://localhost:8080' },
  },
  test: {
    environment: 'jsdom',
    environmentOptions: { jsdom: { url: 'http://localhost:5173' } },
    setupFiles: ['./src/test/setup.ts'],
    restoreMocks: true,
  },
})
