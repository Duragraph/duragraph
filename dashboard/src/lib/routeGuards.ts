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

/**
 * adminOnlyGuard redirects to "/" when the engine is not in multi-tenant mode.
 *
 * Async: uses `ensureQueryData` so it works on cold load (deep link / refresh)
 * before `<CapabilitiesProvider>` mounts. TanStack Router's `beforeLoad`
 * accepts async functions and awaits them.
 */
export async function adminOnlyGuard() {
  const caps = await queryClient.ensureQueryData<Capabilities>({
    queryKey: CAPABILITIES_QUERY_KEY,
    queryFn: fetchInfo,
    staleTime: Infinity,
  })
  if (!caps.platformEnabled) {
    throw redirect({ to: "/" })
  }
}
