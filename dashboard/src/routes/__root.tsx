import { createRootRoute, Outlet } from "@tanstack/react-router"
import { TanStackRouterDevtools } from "@tanstack/router-devtools"
import { QueryClientProvider } from "@tanstack/react-query"
import { ReactQueryDevtools } from "@tanstack/react-query-devtools"
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
      <ReactQueryDevtools initialIsOpen={false} />
      <TanStackRouterDevtools />
    </QueryClientProvider>
  ),
})
