import { createFileRoute, Outlet } from "@tanstack/react-router"
import { adminOnlyGuard } from "@/lib/routeGuards"

/**
 * Admin layout route. Hidden when the engine is not in multi-tenant mode.
 *
 * The `beforeLoad` guard reads engine capabilities from the QueryClient cache
 * (populated by `<CapabilitiesProvider>` at boot) and redirects to "/" when
 * MIGRATOR_PLATFORM_ENABLED is not set on the engine. This is defence-in-depth:
 * the sidebar already hides admin links in single-tenant mode.
 */
export const Route = createFileRoute("/_app/admin")({
  beforeLoad: adminOnlyGuard,
  component: AdminLayout,
})

function AdminLayout() {
  return <Outlet />
}
