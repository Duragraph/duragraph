import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/api/client"
import { runsPollInterval } from "@/api/runs"
import type { V2Run } from "@/types/entities"
import { RunStatusBadge } from "@/components/runs/RunStatusBadge"
import { PageHeader } from "@/components/layout/PageHeader"
import { Card, CardContent } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { JsonView } from "@/components/common/JsonView"
import { ArrowLeft } from "lucide-react"

interface ThreadStateSnapshot {
  checkpoint: { checkpoint_id?: string }
  created_at: string
  values: unknown
  next: string[]
}

export const Route = createFileRoute("/_app/traces/$traceId")({ component: SessionDetailPage })

// A session is a thread's run history. Execution payloads/spans are not part
// of the v2 Run contract; each row links to the run's documented detail view.
function SessionDetailPage() {
  const { traceId: threadId } = Route.useParams()
  const { data: runs, isLoading, error } = useQuery({
    queryKey: ["runs", threadId],
    queryFn: () => api.get<V2Run[]>(`/threads/${threadId}/runs`),
    refetchInterval: (q) => runsPollInterval(q.state.data),
  })
  const { data: latestState } = useQuery({
    queryKey: ["thread-state", threadId],
    queryFn: () => api.get<ThreadStateSnapshot>(`/threads/${threadId}/state`),
    retry: false, // threads without snapshots return 404
  })
  const { data: history } = useQuery({
    queryKey: ["thread-history", threadId],
    queryFn: () => api.get<ThreadStateSnapshot[]>(`/threads/${threadId}/history?limit=10`),
  })

  return <div className="space-y-6">
    <Link to="/traces" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"><ArrowLeft className="h-4 w-4" /> Back to sessions</Link>
    <PageHeader title="Session runs" description={`Thread ${threadId}`} />
    {isLoading ? <Skeleton className="h-40 w-full" /> : error ? <p role="alert">Failed to load runs: {error.message}</p> : !runs?.length ? <p className="text-muted-foreground">No runs in this thread.</p> :
      [...runs].sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime()).map(run =>
        <Card key={run.run_id}><CardContent className="flex flex-wrap items-center justify-between gap-3 py-4">
          <Link to="/runs/$runId" params={{ runId: run.run_id }} className="font-mono text-sm hover:underline">{run.run_id}</Link>
          <RunStatusBadge status={run.status} />
          <span className="text-sm text-muted-foreground">{new Date(run.created_at).toLocaleString()}</span>
        </CardContent></Card>)}
    <Card><CardContent className="space-y-3 py-4">
      <h2 className="font-semibold">Latest thread state</h2>
      <p className="text-sm text-muted-foreground">Thread-level checkpoint, not the input or output of a selected run.</p>
      {latestState ? <><p className="text-sm">Checkpoint {latestState.checkpoint?.checkpoint_id || "unknown"} · {latestState.created_at}</p><JsonView value={latestState.values} /></> : <p className="text-sm text-muted-foreground">No thread checkpoint available.</p>}
      <h3 className="font-medium">Recent checkpoints</h3>
      {history?.length ? <ul className="space-y-1 text-sm">{history.map((state, i) => <li key={state.checkpoint?.checkpoint_id || i}>#{state.checkpoint?.checkpoint_id || "unknown"} · {state.created_at}</li>)}</ul> : <p className="text-sm text-muted-foreground">No checkpoint history available.</p>}
    </CardContent></Card>
  </div>
}
