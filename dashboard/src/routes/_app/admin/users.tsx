import { createFileRoute } from "@tanstack/react-router"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

/**
 * Placeholder for the multi-tenant Users admin page. Phase 3 frontend work
 * will replace this with the real UI; the route exists now so the sidebar
 * `<Link to="/admin/users">` resolves and the capability gating mechanism is
 * exercisable end-to-end.
 */
export const Route = createFileRoute("/_app/admin/users")({
  component: AdminUsers,
})

function AdminUsers() {
  return (
    <div className="max-w-2xl">
      <Card>
        <CardHeader>
          <CardTitle>Users</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          User management UI is not yet implemented. This route exists to
          exercise the admin capability gating; the real screen lands in
          Phase 3.
        </CardContent>
      </Card>
    </div>
  )
}
