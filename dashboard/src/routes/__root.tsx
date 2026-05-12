import { createRootRoute, Outlet } from "@tanstack/react-router"
import { QueryClientProvider } from "@tanstack/react-query"
import { Toaster } from "@/components/ui/sonner"
import { queryClient } from "@/lib/queryClient"
import { CapabilitiesProvider } from "@/components/CapabilitiesProvider"

export const Route = createRootRoute({
  component: () => (
    <QueryClientProvider client={queryClient}>
      <CapabilitiesProvider>
        <Outlet />
      </CapabilitiesProvider>
      <Toaster />
    </QueryClientProvider>
  ),
})
