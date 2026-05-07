import { useQuery } from "@tanstack/react-query"
import { fetchInfo, type Capabilities } from "@/api/info"

/** Stable query key for the engine capabilities query. */
export const CAPABILITIES_QUERY_KEY = ["capabilities"] as const

/**
 * useCapabilities returns engine capabilities loaded by `<CapabilitiesProvider>`.
 *
 * The provider performs the initial fetch at app boot and only renders
 * children once data is available — so within the app tree this hook is
 * guaranteed to return a populated `Capabilities`. Outside the provider it
 * would throw.
 */
export function useCapabilities(): Capabilities {
  const { data } = useQuery({
    queryKey: CAPABILITIES_QUERY_KEY,
    queryFn: fetchInfo,
    staleTime: Infinity,
    retry: 1,
  })
  if (!data) {
    throw new Error(
      "useCapabilities() called before <CapabilitiesProvider> populated the cache",
    )
  }
  return data
}
