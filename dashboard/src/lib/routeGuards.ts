/**
 * Route guards for TanStack Router.
 *
 * These run synchronously in `beforeLoad`. They read engine capabilities from
 * the QueryClient cache populated by `<CapabilitiesProvider>` at app boot.
 * Attach them to admin-only routes to redirect users away from features that
 * are unavailable in the current engine mode.
 *
 * Usage in a route file:
 *   import { adminOnlyGuard } from '@/lib/routeGuards'
 *   export const Route = createFileRoute('/_app/admin')({
 *     beforeLoad: adminOnlyGuard,
 *     component: AdminLayout,
 *   })
 */

import { redirect } from "@tanstack/react-router"
import type { Capabilities } from "@/api/info"
import { CAPABILITIES_QUERY_KEY } from "@/hooks/useCapabilities"
import { queryClient } from "@/lib/queryClient"

/**
 * adminOnlyGuard redirects to "/" when the engine is not in multi-tenant mode.
 *
 * Reads from the QueryClient cache; does not fetch. Safe to use synchronously
 * because `<CapabilitiesProvider>` blocks rendering until data is populated.
 */
export function adminOnlyGuard() {
  const caps = queryClient.getQueryData<Capabilities>(CAPABILITIES_QUERY_KEY)
  if (!caps?.platformEnabled) {
    throw redirect({ to: "/" })
  }
}
