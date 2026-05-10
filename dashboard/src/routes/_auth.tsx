import { createFileRoute, Outlet, redirect } from "@tanstack/react-router"
import { isAuthenticated } from "@/lib/auth"

// Public layout for unauthenticated routes (login, register). The
// _auth segment is the conventional TanStack Router pattern for routes
// that should NOT use the app shell. If the user is already
// authenticated, bounce them straight to the app — no point letting
// someone with a valid token land on /login and re-enter creds.
export const Route = createFileRoute("/_auth")({
  beforeLoad: () => {
    if (isAuthenticated()) {
      throw redirect({ to: "/" })
    }
  },
  component: AuthLayout,
})

function AuthLayout() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/30 p-4">
      <Outlet />
    </div>
  )
}
