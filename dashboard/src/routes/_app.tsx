import { createFileRoute, Outlet, redirect } from "@tanstack/react-router"
import { Sidebar } from "@/components/layout/Sidebar"
import { Topbar } from "@/components/layout/Topbar"
import { ErrorBoundary } from "@/components/common/ErrorBoundary"
import { isAuthenticated } from "@/lib/auth"

// Auth gate for the app shell. Any route under /_app/* runs through
// this beforeLoad — without a token, bounce to /login. Public routes
// live under /_auth/* (login, register) and are not gated.
export const Route = createFileRoute("/_app")({
  beforeLoad: () => {
    if (!isAuthenticated()) {
      throw redirect({ to: "/login" })
    }
  },
  component: AppLayout,
})

function AppLayout() {
  return (
    <div className="flex h-screen bg-gray-50 dark:bg-gray-950">
      <Sidebar />
      <div className="flex-1 flex flex-col overflow-hidden">
        <Topbar />
        <main className="flex-1 overflow-y-auto p-6">
          <ErrorBoundary>
            <Outlet />
          </ErrorBoundary>
        </main>
      </div>
    </div>
  )
}
