import { useState } from "react"
import { createFileRoute, Outlet, useMatch } from "@tanstack/react-router"
import { useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/layout/PageHeader"
import { Button } from "@/components/ui/button"
import { Plus, RefreshCw } from "lucide-react"
import { RunTable } from "@/components/runs/RunTable"
import { NewRunDialog } from "@/components/runs/NewRunDialog"

export const Route = createFileRoute("/_app/runs")({
  component: RunsLayout,
})

function RunsLayout() {
  // Check if we're on a child route (e.g., /runs/$runId)
  const childMatch = useMatch({ from: "/_app/runs/$runId", shouldThrow: false })

  if (childMatch) {
    // Render child route (run detail page)
    return <Outlet />
  }

  // Render runs list
  return <RunsPage />
}

function RunsPage() {
  const queryClient = useQueryClient()
  const [showNewRunDialog, setShowNewRunDialog] = useState(false)

  const handleRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ["runs"] })
  }

  return (
    <div>
      <PageHeader
        title="Runs"
        description="View and manage workflow executions"
        actions={
          <>
            <Button variant="outline" size="sm" onClick={handleRefresh}>
              <RefreshCw className="h-4 w-4 mr-2" />
              Refresh
            </Button>
            <Button size="sm" onClick={() => setShowNewRunDialog(true)}>
              <Plus className="h-4 w-4 mr-2" />
              New Run
            </Button>
          </>
        }
      />

      <RunTable />

      <NewRunDialog
        open={showNewRunDialog}
        onOpenChange={setShowNewRunDialog}
      />
    </div>
  )
}
