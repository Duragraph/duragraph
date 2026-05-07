import type { ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"
import { fetchInfo } from "@/api/info"
import { CAPABILITIES_QUERY_KEY } from "@/hooks/useCapabilities"

interface CapabilitiesProviderProps {
  children: ReactNode
}

/**
 * CapabilitiesProvider performs the boot-time fetch of GET /info and gates
 * the rest of the app behind it. Once data is in the QueryClient cache,
 * `useCapabilities()` and route `beforeLoad` guards (via
 * `queryClient.getQueryData`) can read it synchronously.
 */
export function CapabilitiesProvider({ children }: CapabilitiesProviderProps) {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: CAPABILITIES_QUERY_KEY,
    queryFn: fetchInfo,
    staleTime: Infinity,
    retry: 1,
  })

  if (isError) {
    return (
      <div className="min-h-screen flex items-center justify-center p-6">
        <div className="max-w-md text-center space-y-3">
          <h1 className="text-lg font-semibold">Engine unreachable</h1>
          <p className="text-sm text-muted-foreground">
            Could not load engine capabilities from <code>/info</code>.
            {error instanceof Error ? ` ${error.message}` : ""}
          </p>
          <button
            type="button"
            onClick={() => {
              void refetch()
            }}
            className="px-3 py-1.5 text-sm font-medium bg-primary text-primary-foreground hover:bg-primary/90"
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

  if (isLoading || !data) {
    // Brief blank — the fetch is local to the same origin and typically
    // resolves in a few ms. A spinner would flash worse than nothing.
    return null
  }

  return <>{children}</>
}
