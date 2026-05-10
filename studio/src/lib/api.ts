import { useAuthStore } from '@/stores/auth'

const API_BASE = '/api/v1'

export async function apiFetch<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  // Read the token straight from the store rather than via a hook so
  // this helper stays usable from non-React contexts (action callbacks,
  // tanstack-query queryFns, etc.). getState() returns the latest state
  // without subscribing to changes.
  const token = useAuthStore.getState().token

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options?.headers as Record<string, string> | undefined),
  }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
    credentials: 'include',
  })

  // Token expired or revoked → clear the auth store so App.tsx bounces
  // back to the login screen on the next render. Without this the user
  // would see opaque "401: ..." errors from every subsequent query.
  if (res.status === 401) {
    useAuthStore.getState().clearAuth()
  }

  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    throw new Error(`${res.status}: ${text}`)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}
