// Auth endpoints live under /api/auth (not /api/v1), so they don't go
// through the api client in client.ts. This file is the single source
// of truth for hitting POST /api/auth/{register,login}.

import type { AuthUser } from "@/lib/auth"

const AUTH_BASE = "/api/auth"

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

// AuthError carries the engine's `error` code so the form can render
// branch-specific copy (e.g. "already_exists" → "use a different email").
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
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    credentials: "include",
  })
  if (!res.ok) {
    let code = "unknown_error"
    let message = res.statusText
    try {
      const data = await res.json()
      if (typeof data === "object" && data !== null) {
        code = (data as { error?: string }).error ?? code
        message = (data as { message?: string }).message ?? message
      }
    } catch {
      // Non-JSON error body — fall back to statusText.
    }
    throw new AuthError(res.status, code, message)
  }
  return res.json() as Promise<T>
}

export function registerUser(req: RegisterRequest) {
  return authFetch<RegisterResponse>("/register", req)
}

export function loginUser(req: LoginRequest) {
  return authFetch<LoginResponse>("/login", req)
}

export function toAuthUser(resp: LoginResponse): AuthUser {
  return {
    user_id: resp.user_id,
    email: resp.email,
    role: resp.role,
    tenant_id: resp.tenant_id,
  }
}
