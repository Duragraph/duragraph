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
    plugins: [
      react(),
      tailwindcss(),
      // `autoCodeSplitting` extracts each route file's `component`,
      // `loader`, etc. into its own chunk so the initial bundle only
      // ships the routes the user actually lands on. Without it every
      // route's transitive imports (xyflow + ELK from /traces/$id and
      // /runs/$id; React Flow CSS; the JsonView highlighter; …) were
      // bundled into one 2.5 MB index-*.js. Per-route splits drop
      // first-load size by ~60% in this app.
      TanStackRouterVite({ autoCodeSplitting: true }),
    ],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    // Bundle-shape config. Vendor chunks live separately so a route
    // chunk's only contribution is the route's own code; long-lived
    // libraries (react, tanstack, xyflow) get cached on their own
    // content hash and survive normal app churn.
    build: {
      rollupOptions: {
        output: {
          manualChunks: {
            // React core — bumped versions are rare, cache aggressively.
            "vendor-react": ["react", "react-dom"],
            // TanStack stack — owns routing + server state.
            "vendor-tanstack": [
              "@tanstack/react-query",
              "@tanstack/react-router",
            ],
            // xyflow + ELK — only loaded on routes that render the graph
            // visualizer (/runs/$id, /traces/$id, /builder). Splitting
            // them out is the single biggest first-load win because
            // ELK's WebWorker bundle alone is several hundred KB.
            "vendor-graph": ["@xyflow/react", "elkjs"],
          },
        },
      },
    },
    server: {
      port: 3001,
      proxy: (() => {
        // Distinct env name avoids clash with VITE_API_URL (which the
        // client uses as its API base path, often including /api/v1).
        const target = env.VITE_API_PROXY_TARGET || "http://localhost:18081"
        // Engine serves /info, /ok, /metrics, /mcp at the root (not under
        // /api). Without forwarding these, the dashboard's CapabilitiesProvider
        // hits the vite SPA fallback (index.html) for /info and shows
        // "Engine unreachable" because JSON.parse fails on HTML.
        return {
          "/api": { target, changeOrigin: true },
          "/info": { target, changeOrigin: true },
          "/ok": { target, changeOrigin: true },
          "/metrics": { target, changeOrigin: true },
          "/mcp": { target, changeOrigin: true },
        }
      })(),
    },
  }
})
