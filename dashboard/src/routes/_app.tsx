import { createFileRoute, Outlet } from "@tanstack/react-router"
import { AppSidebar } from "@/components/app-sidebar"
import { Topbar } from "@/components/layout/Topbar"
import { ErrorBoundary } from "@/components/common/ErrorBoundary"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { authGuard } from "@/lib/routeGuards"

// Auth gate for the app shell. authGuard reads GET /info: if the backend
// has AUTH_ENABLED=false the user goes straight in; otherwise it
// requires a token in localStorage and bounces to /login when missing.
//
// Shell layout uses shadcn's <SidebarProvider> + <Sidebar> block
// (sidebar-05) so collapse/keyboard/persistence behaviours come from
// the canonical implementation rather than hand-rolled state.
export const Route = createFileRoute("/_app")({
  beforeLoad: authGuard,
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
