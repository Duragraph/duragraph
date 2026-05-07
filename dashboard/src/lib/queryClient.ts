import { QueryClient } from "@tanstack/react-query"

/**
 * Shared QueryClient instance.
 *
 * Exported as a module singleton so route guards (which run outside React)
 * can read cached data via `queryClient.getQueryData(...)` synchronously.
 * The same instance is provided to React via `<QueryClientProvider>` in
 * `src/routes/__root.tsx`.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60, // 1 minute
      gcTime: 1000 * 60 * 10, // 10 minutes
      retry: 3,
      refetchOnWindowFocus: false,
    },
  },
})
