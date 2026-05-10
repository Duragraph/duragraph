// Token + current-user persistence helpers backed by localStorage.
//
// The dashboard's API client (src/api/client.ts) reads "auth_token"
// directly via localStorage.getItem — keep that key name to avoid
// rewriting every request. The "auth_user" key is new in this PR and
// caches the user record returned by POST /api/auth/login so the UI can
// render the email + role badge without a separate /api/platform/me
// fetch on every page load.

const TOKEN_KEY = "auth_token"
const USER_KEY = "auth_user"

export interface AuthUser {
  user_id: string
  email: string
  role: string
  tenant_id?: string
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function getUser(): AuthUser | null {
  const raw = localStorage.getItem(USER_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw) as AuthUser
  } catch {
    // Corrupted localStorage value — treat as logged out so the user
    // bounces back to the login screen rather than seeing a blank UI.
    localStorage.removeItem(USER_KEY)
    return null
  }
}

export function setAuth(token: string, user: AuthUser): void {
  localStorage.setItem(TOKEN_KEY, token)
  localStorage.setItem(USER_KEY, JSON.stringify(user))
}

export function clearAuth(): void {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}

export function isAuthenticated(): boolean {
  return getToken() !== null
}
