import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/api/client"
import { useRun } from "@/api/runs"
import { graphTopology } from "@/api/runExecution"
import { hasRunThread, isActiveRun, type Assistant } from "@/types/entities"
import { useRunExecution } from "@/hooks/useRunExecution"
import { PageHeader } from "@/components/layout/PageHeader"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { JsonView } from "@/components/common/JsonView"
import { RunStatusBadge } from "@/components/runs/RunStatusBadge"
import { GraphVisualizer, type ExecutionStatus } from "@/components/graph/GraphVisualizer"
import { ArrowLeft, AlertCircle, RefreshCw } from "lucide-react"

export const Route = createFileRoute("/_app/runs/$runId")({ component: RunDetailPage })

function RunDetailPage() {
  const { runId } = Route.useParams()
  // Poll the documented run resource while active. The legacy /stream event
  // endpoint does not provide a reliable execution history for v2 runs.
  const { data: run, isLoading, error, refetch } = useRun(runId)
  const hasThread = !!run && hasRunThread(run)
  const { execution, connected, unavailable, received } = useRunExecution(hasThread ? run.thread_id : null, runId)
  const { data: assistant } = useQuery({
    queryKey: ["assistant", run?.assistant_id],
    queryFn: () => api.get<Assistant>(`/assistants/${run?.assistant_id}`),
    enabled: !!run?.assistant_id,
  })
  const { data: graphSchema, isLoading: graphLoading } = useQuery({
    queryKey: ["assistant-graph", run?.assistant_id],
    queryFn: () => api.get<unknown>(`/assistants/${run?.assistant_id}/graph`),
    enabled: !!run?.assistant_id,
    retry: false,
  })
  const graph = graphTopology(graphSchema)
  const nodeStatuses = Object.fromEntries(Object.values(execution.nodes).map(n => [n.nodeId, n.status])) as Record<string, ExecutionStatus>

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
          <div><span className="text-muted-foreground">Thread: </span>{hasThread
            ? <Link className="font-mono hover:underline" to="/threads/$threadId" params={{ threadId: run.thread_id }}>{run.thread_id}</Link>
            : <span>Stateless run</span>}</div>
        </CardContent></Card>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <Card><CardHeader><CardTitle>Metadata</CardTitle></CardHeader><CardContent>{Object.keys(run.metadata ?? {}).length ? <JsonView value={run.metadata} /> : <p className="text-sm text-muted-foreground">No metadata attached.</p>}</CardContent></Card>
        <Card><CardHeader><CardTitle>Run options</CardTitle></CardHeader><CardContent><JsonView value={run.kwargs ?? {}} /></CardContent></Card>
      </div>
      <Card><CardHeader><CardTitle>Execution details</CardTitle></CardHeader><CardContent>
        {hasThread ? <div className="space-y-4">
          <p className="text-sm text-muted-foreground">{connected ? "Receiving run events" : received ? "Recorded run events" : unavailable ? "Event stream unavailable; run status still refreshes from the API." : "Loading run events…"} Recorded events are replayed when the stream is available.</p>
          {execution.error && <p role="alert" className="text-sm text-destructive">{execution.error}</p>}
          <div className="grid gap-4 md:grid-cols-2">
            <div><h3 className="mb-2 font-medium">Input (run event)</h3>{execution.input === undefined ? <p className="text-sm text-muted-foreground">Not available from run events.</p> : <JsonView value={execution.input} />}</div>
            <div><h3 className="mb-2 font-medium">Output (run event)</h3>{execution.output === undefined ? <p className="text-sm text-muted-foreground">Not available from run events.</p> : <JsonView value={execution.output} />}</div>
          </div>
          <h3 className="font-medium">Node executions</h3>
          {Object.values(execution.nodes).length ? Object.values(execution.nodes).map(node =>
            <div key={node.nodeId} className="border p-3 text-sm space-y-2">
              <div className="flex items-center justify-between"><span className="font-mono">{node.nodeId}</span><span>{node.status}{node.durationMs !== undefined ? ` · ${node.durationMs} ms` : ""}</span></div>
              {node.error && <p className="text-destructive">{node.error}</p>}
              {node.input !== undefined && <div><p>Input</p><JsonView value={node.input} /></div>}
              {node.output !== undefined && <div><p>Output</p><JsonView value={node.output} /></div>}
            </div>) : <p className="text-sm text-muted-foreground">No node events received.</p>}
        </div> : <p className="text-sm text-muted-foreground">Stateless runs have no documented per-run replay endpoint. Input, output, timings, errors and node history are not available in the Run response.</p>}
      </CardContent></Card>
      <Card><CardHeader><CardTitle>Assistant graph</CardTitle></CardHeader><CardContent>
        {graphLoading ? <Skeleton className="h-[400px] w-full" /> : graph && graph.nodes.length ? <div className="h-[500px]"><GraphVisualizer graph={graph} nodeStatuses={nodeStatuses} /></div> : <p className="text-sm text-muted-foreground">Graph topology is not available from this assistant's graph schema.</p>}
        <p className="mt-3 text-sm text-muted-foreground">Topology belongs to the assistant. Only nodes with recorded run events show an execution status.</p>
      </CardContent></Card>
    </div>
  )
}
