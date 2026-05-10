import { useState } from 'react'
import { useAuthStore } from '@/stores/auth'
import { AuthError, login, register, toAuthUser } from '@/api/auth'

type Mode = 'login' | 'register'

interface FormState {
  email: string
  password: string
  displayName: string
}

const blank: FormState = { email: '', password: '', displayName: '' }

// Maps engine error codes (auth/password.yml § endpoints) to friendly
// help text. Anything not listed falls back to the engine's `message`.
function friendlyMessage(err: unknown): string {
  if (!(err instanceof AuthError)) {
    return 'Network error. Is the engine running?'
  }
  switch (err.code) {
    case 'invalid_credentials':
      return 'Invalid email or password.'
    case 'already_exists':
      return 'An account with this email already exists.'
    case 'invalid_input':
      return err.message || 'Please check your inputs.'
    case 'invalid_request':
      return 'Could not parse request. Refresh and try again.'
    default:
      return err.message || 'Login failed.'
  }
}

export function AuthView() {
  const setAuth = useAuthStore((s) => s.setAuth)

  const [mode, setMode] = useState<Mode>('login')
  const [form, setForm] = useState<FormState>(blank)
  const [error, setError] = useState<string | null>(null)
  const [info, setInfo] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const switchMode = (m: Mode) => {
    setMode(m)
    setError(null)
    setInfo(null)
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setInfo(null)
    setSubmitting(true)
    try {
      if (mode === 'register') {
        await register({
          email: form.email,
          password: form.password,
          display_name: form.displayName || form.email,
        })
        // Try auto-login. The first user (bootstrap) is admin+approved
        // so login succeeds immediately. Subsequent users are pending
        // and the login attempt returns 401 — we surface that as a
        // pending-approval message rather than a hard error so the user
        // knows the account WAS created.
        try {
          const resp = await login({
            email: form.email,
            password: form.password,
          })
          setAuth(resp.token, toAuthUser(resp))
        } catch (loginErr) {
          if (
            loginErr instanceof AuthError &&
            loginErr.code === 'invalid_credentials'
          ) {
            setInfo(
              'Account created. Awaiting admin approval before you can log in.',
            )
            setMode('login')
            setForm({ ...blank, email: form.email })
          } else {
            throw loginErr
          }
        }
      } else {
        const resp = await login({ email: form.email, password: form.password })
        setAuth(resp.token, toAuthUser(resp))
      }
    } catch (err) {
      setError(friendlyMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <div className="w-full max-w-sm border border-border bg-card p-8 shadow-md">
        <h1 className="mb-1 text-2xl font-semibold tracking-tight text-foreground">
          DuraGraph Studio
        </h1>
        <p className="mb-6 text-sm text-muted-foreground">
          {mode === 'login'
            ? 'Sign in to continue.'
            : 'Create your account.'}
        </p>

        <div className="mb-6 flex border border-border">
          <button
            type="button"
            onClick={() => switchMode('login')}
            className={`flex-1 px-3 py-2 text-sm font-medium transition-colors ${
              mode === 'login'
                ? 'bg-primary text-primary-foreground'
                : 'bg-background text-muted-foreground hover:text-foreground'
            }`}
          >
            Sign in
          </button>
          <button
            type="button"
            onClick={() => switchMode('register')}
            className={`flex-1 px-3 py-2 text-sm font-medium transition-colors ${
              mode === 'register'
                ? 'bg-primary text-primary-foreground'
                : 'bg-background text-muted-foreground hover:text-foreground'
            }`}
          >
            Register
          </button>
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <label className="flex flex-col gap-1 text-sm">
            <span className="text-foreground">Email</span>
            <input
              type="email"
              required
              autoComplete="email"
              value={form.email}
              onChange={(e) => setForm({ ...form, email: e.target.value })}
              className="border border-input bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              placeholder="you@example.com"
            />
          </label>

          <label className="flex flex-col gap-1 text-sm">
            <span className="text-foreground">Password</span>
            <input
              type="password"
              required
              minLength={mode === 'register' ? 8 : undefined}
              autoComplete={
                mode === 'register' ? 'new-password' : 'current-password' // gitleaks:allow
              }
              value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })}
              className="border border-input bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              placeholder={
                mode === 'register' ? 'At least 8 characters' : '••••••••'
              }
            />
          </label>

          {mode === 'register' && (
            <label className="flex flex-col gap-1 text-sm">
              <span className="text-foreground">
                Display name <span className="text-muted-foreground">(optional)</span>
              </span>
              <input
                type="text"
                autoComplete="name"
                value={form.displayName}
                onChange={(e) =>
                  setForm({ ...form, displayName: e.target.value })
                }
                className="border border-input bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                placeholder="How should we address you?"
              />
            </label>
          )}

          {error && (
            <div className="border border-destructive bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          )}
          {info && (
            <div className="border border-border bg-muted px-3 py-2 text-sm text-foreground">
              {info}
            </div>
          )}

          <button
            type="submit"
            disabled={submitting}
            className="bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {submitting
              ? 'Working...'
              : mode === 'login'
                ? 'Sign in'
                : 'Create account'}
          </button>
        </form>
      </div>
    </div>
  )
}
