import { create } from 'zustand'
import { persist } from 'zustand/middleware'

// Engine response shape from POST /api/auth/login.
export interface AuthUser {
  user_id: string
  email: string
  role: string
  tenant_id?: string
}

interface AuthState {
  token: string | null
  user: AuthUser | null
  setAuth: (token: string, user: AuthUser) => void
  clearAuth: () => void
}

// Persisted to localStorage so a refresh keeps the user logged in until
// the JWT expires (24h default per engine config). The middleware in the
// engine clears the cookie on 401, but a stale token in localStorage
// would still attach Bearer until the next 401 — see lib/api.ts which
// calls clearAuth() on 401 to keep the two in sync.
export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      setAuth: (token, user) => set({ token, user }),
      clearAuth: () => set({ token: null, user: null }),
    }),
    { name: 'duragraph-auth' },
  ),
)
