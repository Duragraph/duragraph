import { createFileRoute } from "@tanstack/react-router"
import { LoginPage } from "./login"

export const Route = createFileRoute("/_auth/register")({
  component: () => <LoginPage initialMode="register" />,
})
