import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  // base controls how absolute asset URLs are emitted in index.html.
  // When the bundle is served by the engine binary at /studio/* (via
  // //go:embed studio/dist), assets like /assets/index-X.js must be
  // requested as /studio/assets/index-X.js. Default '/' produces a
  // blank page in the embedded path because the script tags 404.
  // VITE_BASE_PATH lets the standalone vite dev server keep '/'.
  base: process.env.VITE_BASE_PATH ?? '/studio/',
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target:
          process.env.VITE_DURAGRAPH_API_URL ||
          process.env.VITE_API_PROXY_TARGET ||
          'http://localhost:18081',
        changeOrigin: true,
      },
    },
  },
})
