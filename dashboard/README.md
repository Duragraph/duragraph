# DuraGraph Dashboard

The React dashboard for the DuraGraph control plane. Lets users view runs, manage assistants and threads, inspect traces, and monitor analytics.

## How it ships

In **production** the dashboard is **built into the same binary as the Go API server** via `go:embed`. There is no separate dashboard container. The flow:

1. `pnpm build` outputs static assets to `dashboard/dist/`
2. `go build ./cmd/server` picks them up via `//go:embed all:dashboard/dist` in `dashboard_embed.go` at the module root
3. The server registers a SPA-fallback handler at `/*` that serves `dist/` files and falls back to `index.html` for any path not under `/api/v1/`, `/health`, `/metrics`, `/ok`, or `/info`

`deploy/docker/Dockerfile.server` runs both build steps in a multi-stage image. See `internal/infrastructure/http/dashboard/handler.go` for the runtime serving logic.

> **Why is `dashboard/dist/index.html` committed?** It's a tiny placeholder so `go build` succeeds before anyone has run `pnpm build`. Real builds overwrite it. See `dashboard/.gitignore` — only `index.html` is whitelisted, every other file in `dist/` is ignored.

## Stack

- **React 19** + TypeScript (strict)
- **Vite 7** for build + HMR
- **TanStack Router** for type-safe routing (file-based)
- **TanStack Query** for server state
- **Zustand** for client state
- **shadcn/ui** + Radix + Tailwind for components
- **@xyflow/react** for graph visualization
- **next-themes** for dark mode

## Local development

```bash
# from the repo root
task dashboard:dev      # starts vite on :3303
task api:dev            # starts the Go API on :8080

# or directly:
cd dashboard && pnpm install && pnpm dev --port 3303 --host
```

Set `VITE_API_URL=http://localhost:8080/api/v1` in `dashboard/.env.local` if the API is on a different host. By default the client uses `/api/v1` (relative), which works when the dashboard is served from the same origin as the API.

## Building

```bash
task build:dashboard    # produces dashboard/dist/
task build:server       # rebuilds the Go binary with the new dist embedded
```

In CI both steps run on every PR — see `.github/workflows/ci.yml`.

## Layout

```
dashboard/
├── src/
│   ├── api/             # Thin fetch wrapper around the duragraph REST API
│   ├── components/
│   │   ├── assistants/  # Assistant management (list, form, card)
│   │   ├── chat/        # Chat-style interaction with threads
│   │   ├── graph/       # xyflow-based graph rendering
│   │   ├── layout/      # Sidebar, topbar, app shell
│   │   ├── runs/        # Run list + detail
│   │   ├── threads/     # Thread list + messages
│   │   └── ui/          # shadcn primitives
│   ├── routes/_app/     # File-based routes: runs, assistants, threads,
│   │                    # traces, analytics, costs, settings, profile
│   ├── stores/          # Zustand stores
│   └── lib/             # Utilities
├── dist/                # Build output (gitignored except for the placeholder)
└── public/
```

## Design system

- Zero border-radius, Space Grotesk display font, coral accent
- shadcn/ui primitives + sidebar-07 block; xyflow for graph viz
- React 19 + TypeScript strict, no `any`
- The visual workflow editor (formerly a standalone `studio/` app) is folded into this dashboard as of #190
