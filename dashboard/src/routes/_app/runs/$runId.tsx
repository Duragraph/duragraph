import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/api/client"
import { useRun } from "@/api/runs"
import { hasRunThread, isActiveRun, type Assistant } from "@/types/entities"
import { PageHeader } from "@/components/layout/PageHeader"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { JsonView } from "@/components/common/JsonView"
import { RunStatusBadge } from "@/components/runs/RunStatusBadge"
import { ArrowLeft, AlertCircle, RefreshCw } from "lucide-react"

export const Route = createFileRoute("/_app/runs/$runId")({ component: RunDetailPage })

function RunDetailPage() {
  const { runId } = Route.useParams()
  // Poll the documented run resource while active. The legacy /stream event
  // endpoint does not provide a reliable execution history for v2 runs.
  const { data: run, isLoading, error, refetch } = useRun(runId)
  const { data: assistant } = useQuery({
    queryKey: ["assistant", run?.assistant_id],
    queryFn: () => api.get<Assistant>(`/assistants/${run?.assistant_id}`),
    enabled: !!run?.assistant_id,
  })

  if (isLoading) return <Skeleton className="h-64 w-full" />
  if (error || !run) return (
    <div className="py-12 text-center">
      <AlertCircle className="h-12 w-12 text-destructive mx-auto mb-4" />
      <h2 className="text-xl font-semibold">Failed to load run</h2>
      <p className="text-muted-foreground">{error?.message || "Run not found"}</p>
      <Button asChild variant="outline" className="mt-4"><Link to="/runs">Back to Runs</Link></Button>
    </div>
  )

  return (
    <div className="space-y-6">
      <Link to="/runs" className="text-sm text-muted-foreground hover:text-foreground inline-flex items-center gap-1">
        <ArrowLeft className="h-4 w-4" /> Back to Runs
      </Link>
      <PageHeader
        title={<span className="flex items-center gap-3 font-mono">{run.run_id.slice(0, 16)}… <RunStatusBadge status={run.status} /></span>}
        description={`Assistant: ${assistant?.name || run.assistant_id}${isActiveRun(run) ? " · Auto-refreshing status" : ""}`}
        actions={<Button variant="outline" size="sm" onClick={() => refetch()}><RefreshCw className="mr-2 h-4 w-4" /> Refresh</Button>}
      />
      <div className="grid gap-4 md:grid-cols-2">
        <Card><CardHeader><CardTitle>Run details</CardTitle></CardHeader><CardContent className="space-y-3 text-sm">
          <div><span className="text-muted-foreground">Run ID: </span><span className="font-mono break-all">{run.run_id}</span></div>
          <div><span className="text-muted-foreground">Created: </span>{new Date(run.created_at).toLocaleString()}</div>
          <div><span className="text-muted-foreground">Updated: </span>{new Date(run.updated_at).toLocaleString()}</div>
          <div><span className="text-muted-foreground">Strategy: </span>{run.multitask_strategy}</div>
        </CardContent></Card>
        <Card><CardHeader><CardTitle>Linked resources</CardTitle></CardHeader><CardContent className="space-y-3 text-sm">
          <div><span className="text-muted-foreground">Assistant: </span><Link className="hover:underline" to="/assistants/$assistantId" params={{ assistantId: run.assistant_id }}>{assistant?.name || run.assistant_id}</Link></div>
          <div><span className="text-muted-foreground">Thread: </span>{hasRunThread(run)
            ? <Link className="font-mono hover:underline" to="/threads/$threadId" params={{ threadId: run.thread_id }}>{run.thread_id}</Link>
            : <span>Stateless run</span>}</div>
        </CardContent></Card>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <Card><CardHeader><CardTitle>Metadata</CardTitle></CardHeader><CardContent>{Object.keys(run.metadata ?? {}).length ? <JsonView value={run.metadata} /> : <p className="text-sm text-muted-foreground">No metadata attached.</p>}</CardContent></Card>
        <Card><CardHeader><CardTitle>Run options</CardTitle></CardHeader><CardContent><JsonView value={run.kwargs ?? {}} /></CardContent></Card>
      </div>
      <Card><CardHeader><CardTitle>Execution details</CardTitle></CardHeader><CardContent>
        <p className="text-sm text-muted-foreground">The run API does not expose input, output, timings, errors or per-node execution details. Assistant graph topology is not an execution trace.</p>
      </CardContent></Card>
    </div>
  )
}
