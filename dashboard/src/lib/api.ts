// apiFetch is a thin fetch wrapper compatible with the studio-style
// hook API (assistants.ts, runs.ts, threads.ts). It's a different shape
// from `api.get/post` in `@/api/client` but both target the same
// `/api/v1` base and read the same Bearer token from localStorage.
//
// New code should prefer the hook style (`useAssistants()` etc. — see
// `@/api/assistants`); the bare `api.get()` shape from `@/api/client`
// is retained for routes that already use it.

import { clearAuth, getToken } from "@/lib/auth"

const API_BASE = "/api/v1"

export async function apiFetch<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...((options?.headers as Record<string, string> | undefined) ?? {}),
  }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
    credentials: "include",
  })

  // Token expired or revoked → clear and bounce. Matches the behaviour
  // of `api/client.ts` so the two paths cannot drift on auth handling.
  if (res.status === 401) {
    clearAuth()
    if (
      typeof window !== "undefined" &&
      !window.location.pathname.startsWith("/login")
    ) {
      window.location.href = "/login"
    }
  }

  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    throw new Error(`${res.status}: ${text}`)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}
