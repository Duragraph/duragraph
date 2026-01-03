const API_BASE = import.meta.env.VITE_API_URL || "/api/v1"

async function fetchWithAuth(url: string, options: RequestInit = {}) {
  const token = localStorage.getItem("auth_token")

  const response = await fetch(`${API_BASE}${url}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token && { Authorization: `Bearer ${token}` }),
      ...options.headers,
    },
  })

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
