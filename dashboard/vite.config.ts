import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'

// Served at /dashboard/ in production via nginx; dev runs at the same
// subpath so links and the router behave identically.
export default defineConfig({
  base: '/dashboard/',
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    port: 5173,
    proxy: {
      // Proxy backend calls to prod during local dev so we don't need a
      // local Postgres + sms-server to click around. `secure: false` lets
      // us hit https endpoints with self-signed/test certs if applicable.
      '/admin': {
        target: 'https://sms.aipanel.cl',
        changeOrigin: true,
        secure: true,
      },
      '/v1': {
        target: 'https://sms.aipanel.cl',
        changeOrigin: true,
        secure: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
  },
})
