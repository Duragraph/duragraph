import { clearAuth, getToken } from "@/lib/auth"

const API_BASE = import.meta.env.VITE_API_URL || "/api/v1"

async function fetchWithAuth(url: string, options: RequestInit = {}) {
  const token = getToken()

  const response = await fetch(`${API_BASE}${url}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token && { Authorization: `Bearer ${token}` }),
      ...options.headers,
    },
  })

  // Token expired or revoked → clear localStorage and bounce to login.
  // We use window.location instead of the router's navigate because
  // this fetch helper isn't running inside a React component (no hook
  // context); a full reload also discards in-flight TanStack Query
  // state that would otherwise re-fire and 401 again in a loop.
  if (response.status === 401) {
    clearAuth()
    if (
      typeof window !== "undefined" &&
      !window.location.pathname.startsWith("/login")
    ) {
      window.location.href = "/login"
    }
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({}))
    throw new Error(error.message || `HTTP ${response.status}`)
  }

  return response.json()
}

export const api = {
  get: <T>(url: string): Promise<T> => fetchWithAuth(url),
  post: <T>(url: string, data: unknown): Promise<T> =>
    fetchWithAuth(url, { method: "POST", body: JSON.stringify(data) }),
  patch: <T>(url: string, data: unknown): Promise<T> =>
    fetchWithAuth(url, { method: "PATCH", body: JSON.stringify(data) }),
  delete: <T>(url: string): Promise<T> =>
    fetchWithAuth(url, { method: "DELETE" }),
}
