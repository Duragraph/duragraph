import { createFileRoute, Outlet, redirect } from "@tanstack/react-router"
import { AppSidebar } from "@/components/app-sidebar"
import { Topbar } from "@/components/layout/Topbar"
import { ErrorBoundary } from "@/components/common/ErrorBoundary"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { isAuthenticated } from "@/lib/auth"

// Auth gate for the app shell. Any route under /_app/* runs through
// this beforeLoad — without a token, bounce to /login. Public routes
// live under /_auth/* (login, register) and are not gated.
//
// Shell layout uses shadcn's <SidebarProvider> + <Sidebar> block
// (sidebar-05) so collapse/keyboard/persistence behaviours come from
// the canonical implementation rather than hand-rolled state.
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
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <Topbar />
        <main className="flex-1 overflow-y-auto p-6">
          <ErrorBoundary>
            <Outlet />
          </ErrorBoundary>
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
