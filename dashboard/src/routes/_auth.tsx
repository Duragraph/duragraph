import { createFileRoute, Outlet } from "@tanstack/react-router"
import { publicAuthGuard } from "@/lib/routeGuards"

// Public layout for unauthenticated routes (login, register). The
// _auth segment is the conventional TanStack Router pattern for routes
// that should NOT use the app shell. publicAuthGuard sends users away
// from here when (a) auth is disabled entirely, or (b) they're already
// signed in — both cases the login screen is useless to them.
export const Route = createFileRoute("/_auth")({
  beforeLoad: publicAuthGuard,
  component: AuthLayout,
})

function AuthLayout() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/30 p-4">
      <Outlet />
    </div>
  )
}
