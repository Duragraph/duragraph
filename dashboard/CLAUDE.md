# CLAUDE.md — duragraph/dashboard

Guidance for working on the React dashboard. The repo-wide guide is one level up at `duragraph/CLAUDE.md`.

## What this is

The dashboard is a single-page React app served by the Go duragraph server from the same binary. It reads the duragraph REST API at `/api/v1`. There is no separate dashboard container or origin.

## Critical: how it ships

- `pnpm build` writes static assets to `dashboard/dist/`.
- `dashboard_embed.go` at the **module root** uses `//go:embed all:dashboard/dist` to bundle that directory into the Go binary.
- The runtime handler lives in `internal/infrastructure/http/dashboard/handler.go`. It serves files from the embedded FS and falls back to `index.html` for SPA routes.
- `dashboard/dist/index.html` is committed as a placeholder so `go build` works before `pnpm build` ever runs. **Don't delete it.** Vite will overwrite it during real builds; that's fine and expected.
- `dashboard/.gitignore` keeps `dist/*` out of git but whitelists `dist/index.html`. Don't whitelist anything else there.
- After running `pnpm build` locally, `git status` will show `dashboard/dist/index.html` as modified (real Vite output) plus other dist files as untracked-but-ignored. Don't commit them.

## Stack constraints

- **React 19**, TypeScript strict mode. Don't add `any`.
- **TanStack Router** is file-based — routes are `src/routes/_app/<page>.tsx`. The `_app` segment is the auth-gated layout. The route tree (`routeTree.gen.ts`) is generated; don't edit by hand.
- **TanStack Query** owns server state. Use `useQuery` for reads, `useMutation` for writes. Don't store API responses in Zustand.
- **Zustand** is for client-only state (sidebar collapsed, theme, etc.).
- **shadcn/ui** primitives live in `src/components/ui/`. Don't reach for new component libraries — extend shadcn.
- **Tailwind** for styling. The design spec calls for **zero border-radius** and **Space Grotesk** typography (see `duragraph-spec/frontend/design-system.yml`). Match those tokens.
- **xyflow/react** for the graph viz. Components live in `src/components/graph/`.

## API client

`src/api/client.ts` is a thin fetch wrapper. It reads `VITE_API_URL` (default `/api/v1`) and pulls a Bearer token from `localStorage`. If you need streaming (SSE), don't go through this client — open an `EventSource` directly.

## Don't

- Don't introduce a new state library, router, or component library.
- Don't add a separate Docker image for the dashboard. Production = embedded in the server binary.
- Don't commit anything in `dashboard/dist/` other than the placeholder `index.html`.
- Don't put API endpoint URLs at component level — put them in `src/api/` so the codebase has one source of truth.
- Don't fetch in components. Use TanStack Query hooks.

## Spec sources

When wiring a new screen, check the spec first:

- `duragraph-spec/frontend/wireframes.yml` — ASCII wireframe for each screen
- `duragraph-spec/frontend/components.yml` — component inventory
- `duragraph-spec/frontend/design-system.yml` — design tokens
- `duragraph-spec/frontend/user-flows.yml` — task flows

The graph-builder spec (`duragraph-spec/frontend/graph-builder.yml`) is **deferred**. Don't implement it without confirmation.

## Common commands

```bash
pnpm dev --port 3303 --host    # vite dev server (matches Taskfile DASHBOARD_PORT)
pnpm build                      # production build → dist/
pnpm lint                       # eslint
tsc -b --noEmit                 # typecheck without emitting
```

CI runs install + lint + typecheck + build on every PR — see `.github/workflows/ci.yml` `dashboard` job.
