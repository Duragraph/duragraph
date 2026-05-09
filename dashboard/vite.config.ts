import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { TanStackRouterVite } from "@tanstack/router-vite-plugin"
import { defineConfig, loadEnv } from "vite"

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  // Read .env files so VITE_*_PROXY_TARGET overrides take effect.
  // vite.config.ts does NOT have access to import.meta.env nor are .env
  // values loaded into process.env automatically — loadEnv is required.
  const env = loadEnv(mode, process.cwd(), "")
  return {
    plugins: [react(), tailwindcss(), TanStackRouterVite()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      port: 3001,
      proxy: {
        "/api": {
          // Distinct env name avoids clash with VITE_API_URL (which the
          // client uses as its API base path, often including /api/v1).
          target: env.VITE_API_PROXY_TARGET || "http://localhost:18081",
          changeOrigin: true,
        },
      },
    },
  }
})
