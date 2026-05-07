import { createFileRoute } from "@tanstack/react-router"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

/**
 * Admin landing page. Shown if the user lands on /admin directly. The actual
 * admin features (Users, Metrics, etc.) are Phase 3 frontend work — this page
 * is a placeholder so the route resolves cleanly when platform mode is on.
 */
export const Route = createFileRoute("/_app/admin/")({
  component: AdminIndex,
})

function AdminIndex() {
  return (
    <div className="max-w-2xl">
      <Card>
        <CardHeader>
          <CardTitle>Admin</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground space-y-2">
          <p>Multi-tenant admin features will appear here.</p>
          <p>
            If you reached this page in single-tenant mode, restart the engine
            with <code>MIGRATOR_PLATFORM_ENABLED=true</code> to enable admin
            APIs.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
