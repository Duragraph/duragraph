import { createFileRoute } from "@tanstack/react-router"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

/**
 * Placeholder for the multi-tenant Metrics admin page. Phase 3 frontend work
 * will wire this to Mimir / cross-tenant metrics. The route exists now so the
 * sidebar `<Link to="/admin/metrics">` resolves under platform mode.
 */
export const Route = createFileRoute("/_app/admin/metrics")({
  component: AdminMetrics,
})

function AdminMetrics() {
  return (
    <div className="max-w-2xl">
      <Card>
        <CardHeader>
          <CardTitle>Metrics</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          Cross-tenant metrics UI is not yet implemented. This route exists to
          exercise the admin capability gating; the real screen lands in
          Phase 3.
        </CardContent>
      </Card>
    </div>
  )
}
