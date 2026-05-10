import type { AuthUser } from '@/stores/auth'

// Auth endpoints live under /api/auth (NOT /api/v1/), so they have their
// own raw fetch wrapper rather than going through lib/api.ts (which
// prefixes /api/v1).
const AUTH_BASE = '/api/auth'

export interface RegisterRequest {
  email: string
  password: string
  display_name: string
}

export interface RegisterResponse {
  user_id: string
  status: string
}

export interface LoginRequest {
  email: string
  password: string
}

export interface LoginResponse {
  user_id: string
  email: string
  role: string
  tenant_id?: string
  token: string
}

// AuthError carries the HTTP status + machine-readable error code from
// the engine (e.g. "invalid_credentials", "already_exists",
// "invalid_input"). The form components branch on `code` so the same
// 400 from a short password and a malformed email render different
// help text.
export class AuthError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

async function authFetch<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${AUTH_BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    credentials: 'include', // engine sets duragraph_session cookie on /login
  })
  if (!res.ok) {
    let code = 'unknown_error'
    let message = res.statusText
    try {
      const data = await res.json()
      if (typeof data === 'object' && data !== null) {
        code = (data as { error?: string }).error ?? code
        message = (data as { message?: string }).message ?? message
      }
    } catch {
      // Non-JSON body (proxy 502, etc.); fall back to statusText.
    }
    throw new AuthError(res.status, code, message)
  }
  return res.json() as Promise<T>
}

export function register(req: RegisterRequest) {
  return authFetch<RegisterResponse>('/register', req)
}

export function login(req: LoginRequest) {
  return authFetch<LoginResponse>('/login', req)
}

export function toAuthUser(resp: LoginResponse): AuthUser {
  return {
    user_id: resp.user_id,
    email: resp.email,
    role: resp.role,
    tenant_id: resp.tenant_id,
  }
}
