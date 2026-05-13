/**
 * Route guards for TanStack Router.
 *
 * These run in `beforeLoad`, BEFORE child components mount. They cannot rely
 * on `<CapabilitiesProvider>` having already populated the query cache —
 * that provider only runs once the route's component subtree mounts, which
 * is *after* `beforeLoad`. Cold loads / deep links / page refreshes hit
 * empty cache.
 *
 * We use `queryClient.ensureQueryData` so the guard:
 *   - returns cached capabilities if present (warm in-app navigation), or
 *   - fires the query and awaits it (cold load), or
 *   - rethrows on fetch failure (TanStack Router surfaces it).
 *
 * The query key + queryFn + staleTime: Infinity must match `useCapabilities`
 * so both code paths share the same query identity in the cache.
 *
 * Usage in a route file:
 *   import { adminOnlyGuard } from '@/lib/routeGuards'
 *   export const Route = createFileRoute('/_app/admin')({
 *     beforeLoad: adminOnlyGuard,
 *     component: AdminLayout,
 *   })
 */

import { redirect } from "@tanstack/react-router"
import { fetchInfo, type Capabilities } from "@/api/info"
import { CAPABILITIES_QUERY_KEY } from "@/hooks/useCapabilities"
import { queryClient } from "@/lib/queryClient"
import { isAuthenticated } from "@/lib/auth"

/** Shared capability fetcher for guards. Same identity as `useCapabilities`. */
async function ensureCapabilities(): Promise<Capabilities> {
  return queryClient.ensureQueryData<Capabilities>({
    queryKey: CAPABILITIES_QUERY_KEY,
    queryFn: fetchInfo,
    staleTime: Infinity,
  })
}

/**
 * authGuard protects the authenticated app shell (/_app/*).
 *
 * Decision matrix driven by GET /info:
 *   - authEnabled=false  → no gate, let the user straight in
 *   - authEnabled=true, no token  → bounce to /login
 *   - authEnabled=true, token present → let through (JWT middleware
 *     enforces validity on each /api/* call)
 *
 * Replaces the previous bare `isAuthenticated()` check, which gated the
 * dashboard even when the backend wasn't enforcing auth — causing the
 * "login form submits to a 404" bug in `duragraph dev` defaults.
 */
export async function authGuard() {
  const caps = await ensureCapabilities()
  if (!caps.authEnabled) {
    return
  }
  if (!isAuthenticated()) {
    throw redirect({ to: "/login" })
  }
}

/**
 * publicAuthGuard protects the /_auth/* routes (login, register).
 *
 *   - authEnabled=false  → redirect to /  (no auth to configure, the
 *     login screen is meaningless)
 *   - authEnabled=true, already authenticated → redirect to /
 *   - otherwise → render (the page itself can branch on
 *     passwordAuthEnabled vs OAuth providers to pick its UI)
 */
export async function publicAuthGuard() {
  const caps = await ensureCapabilities()
  if (!caps.authEnabled) {
    throw redirect({ to: "/" })
  }
  if (isAuthenticated()) {
    throw redirect({ to: "/" })
  }
}

/**
 * adminOnlyGuard redirects to "/" when the engine is not in multi-tenant mode.
 */
export async function adminOnlyGuard() {
  const caps = await ensureCapabilities()
  if (!caps.platformEnabled) {
    throw redirect({ to: "/" })
  }
}
