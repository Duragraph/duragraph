import { useState } from "react"
import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { AuthError, loginUser, registerUser, toAuthUser } from "@/api/auth"
import { setAuth } from "@/lib/auth"

export const Route = createFileRoute("/_auth/login")({
  component: LoginPage,
})

type Mode = "login" | "register"

interface FormState {
  email: string
  password: string
  displayName: string
}

const blank: FormState = { email: "", password: "", displayName: "" }

// Translates engine error codes into UI-facing copy. Anything not
// listed falls back to the engine's `message` so unfamiliar codes are
// at least surfaced rather than swallowed.
function friendlyMessage(err: unknown): string {
  if (!(err instanceof AuthError)) {
    return "Network error. Is the engine running?"
  }
  switch (err.code) {
    case "invalid_credentials":
      return "Invalid email or password."
    case "already_exists":
      return "An account with this email already exists."
    case "invalid_input":
      return err.message || "Please check your inputs."
    case "invalid_request":
      return "Could not parse request. Refresh and try again."
    default:
      return err.message || "Something went wrong."
  }
}

function LoginPage() {
  const navigate = useNavigate()
  const [mode, setMode] = useState<Mode>("login")
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
      if (mode === "register") {
        await registerUser({
          email: form.email,
          password: form.password,
          display_name: form.displayName || form.email,
        })
        // Auto-login after register. The first user (bootstrap) is
        // admin+approved so login succeeds immediately. Subsequent
        // users are pending and the login attempt returns 401 — we
        // surface that as a "wait for approval" message rather than a
        // hard error so the user sees their account WAS created.
        try {
          const resp = await loginUser({
            email: form.email,
            password: form.password,
          })
          setAuth(resp.token, toAuthUser(resp))
          navigate({ to: "/" })
        } catch (loginErr) {
          if (
            loginErr instanceof AuthError &&
            loginErr.code === "invalid_credentials"
          ) {
            setInfo(
              "Account created. Awaiting admin approval before you can log in.",
            )
            switchMode("login")
            setForm({ ...blank, email: form.email })
          } else {
            throw loginErr
          }
        }
      } else {
        const resp = await loginUser({
          email: form.email,
          password: form.password,
        })
        setAuth(resp.token, toAuthUser(resp))
        navigate({ to: "/" })
      }
    } catch (err) {
      setError(friendlyMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card className="w-full max-w-sm">
      <CardHeader>
        <CardTitle>DuraGraph Dashboard</CardTitle>
        <CardDescription>
          {mode === "login"
            ? "Sign in to your account."
            : "Create your account."}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs value={mode} onValueChange={(v) => switchMode(v as Mode)}>
          <TabsList className="mb-6 grid w-full grid-cols-2">
            <TabsTrigger value="login">Sign in</TabsTrigger>
            <TabsTrigger value="register">Register</TabsTrigger>
          </TabsList>

          <TabsContent value={mode} className="mt-0">
            <form onSubmit={handleSubmit} className="flex flex-col gap-4">
              <div className="grid gap-1.5">
                <Label htmlFor="email">Email</Label>
                <Input
                  id="email"
                  type="email"
                  required
                  autoComplete="email"
                  value={form.email}
                  onChange={(e) => setForm({ ...form, email: e.target.value })}
                  placeholder="you@example.com"
                />
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  required
                  minLength={mode === "register" ? 8 : undefined}
                  autoComplete={
                    mode === "register" ? "new-password" : "current-password" // gitleaks:allow
                  }
                  value={form.password}
                  onChange={(e) =>
                    setForm({ ...form, password: e.target.value })
                  }
                  placeholder={
                    mode === "register" ? "At least 8 characters" : "••••••••"
                  }
                />
              </div>

              {mode === "register" && (
                <div className="grid gap-1.5">
                  <Label htmlFor="display_name">
                    Display name{" "}
                    <span className="text-muted-foreground">(optional)</span>
                  </Label>
                  <Input
                    id="display_name"
                    type="text"
                    autoComplete="name"
                    value={form.displayName}
                    onChange={(e) =>
                      setForm({ ...form, displayName: e.target.value })
                    }
                    placeholder="How should we address you?"
                  />
                </div>
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

              <Button type="submit" disabled={submitting} className="mt-2">
                {submitting
                  ? "Working..."
                  : mode === "login"
                    ? "Sign in"
                    : "Create account"}
              </Button>
            </form>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  )
}
