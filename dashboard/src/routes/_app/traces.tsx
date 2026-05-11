import { createFileRoute, Link, Outlet, useMatch, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useMemo, useState } from "react"
import { api } from "@/api/client"
import { runsPollInterval } from "@/api/runs"
import type { Run, RunStatus, Assistant, AssistantsResponse } from "@/types/entities"
import { PageHeader } from "@/components/layout/PageHeader"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { RunStatusBadge } from "@/components/runs/RunStatusBadge"
import { RefreshCw, Search, Activity, MessageSquare } from "lucide-react"

export const Route = createFileRoute("/_app/traces")({
  component: TracesLayout,
})

// /traces is a Langfuse-style **Sessions** index — one row per thread,
// not one row per run. The trace detail (`$traceId`) renders the chain
// of runs inside that thread. Mapping:
//   Langfuse Session   ⇄  DuraGraph Thread
//   Langfuse Trace     ⇄  DuraGraph Run
//   Langfuse Span      ⇄  DuraGraph Node execution within a run
// This route only renders when there's no child match; otherwise the
// nested `$traceId` route renders inside the Outlet.
function TracesLayout() {
  const childMatch = useMatch({ from: "/_app/traces/$traceId", shouldThrow: false })
  if (childMatch) return <Outlet />
  return <TracesPage />
}

type ThreadRollup = {
  threadId: string
  runCount: number
  lastActivity: string
  latestStatus: RunStatus
  latestAssistantId: string
  latestRunId: string
  errorCount: number
}

function rollupThreads(runs: Run[]): ThreadRollup[] {
  const byThread = new Map<string, Run[]>()
  for (const r of runs) {
    const list = byThread.get(r.thread_id) ?? []
    list.push(r)
    byThread.set(r.thread_id, list)
  }

  const rollups: ThreadRollup[] = []
  for (const [threadId, list] of byThread) {
    // Sort runs newest-first by created_at so list[0] is the latest.
    const sorted = [...list].sort(
      (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    )
    const latest = sorted[0]
    rollups.push({
      threadId,
      runCount: sorted.length,
      lastActivity: latest.updated_at || latest.created_at,
      latestStatus: latest.status as RunStatus,
      latestAssistantId: latest.assistant_id,
      latestRunId: latest.run_id,
      errorCount: sorted.filter((r) => r.status === "failed").length,
    })
  }
  return rollups.sort(
    (a, b) => new Date(b.lastActivity).getTime() - new Date(a.lastActivity).getTime(),
  )
}

function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  if (diffMins < 1) return "just now"
  if (diffMins < 60) return `${diffMins} min ago`
  const diffHours = Math.floor(diffMins / 60)
  if (diffHours < 24) return `${diffHours} hour${diffHours > 1 ? "s" : ""} ago`
  const diffDays = Math.floor(diffHours / 24)
  return `${diffDays} day${diffDays > 1 ? "s" : ""} ago`
}

function TracesPage() {
  const navigate = useNavigate()
  const [statusFilter, setStatusFilter] = useState<string>("all")
  const [assistantFilter, setAssistantFilter] = useState<string>("all")
  const [search, setSearch] = useState<string>("")

  const {
    data: runs,
    isLoading,
    error,
    refetch,
  } = useQuery({
    queryKey: ["runs"],
    queryFn: () => api.get<Run[]>("/runs"),
    // Adaptive polling: only refresh aggressively when something is
    // actually in flight. The previous 5 s blanket interval refetched
    // even after every run had been completed for hours, causing
    // pointless re-renders of the whole threads table. Now a fast
    // 1.5 s tick kicks in only while any run is non-terminal, and the
    // page idles at 15 s the rest of the time.
    refetchInterval: (q) => runsPollInterval(q.state.data as Run[] | undefined),
  })

  const { data: assistantsData } = useQuery({
    queryKey: ["assistants"],
    queryFn: () => api.get<AssistantsResponse>("/assistants"),
  })

  const assistantMap = useMemo(() => {
    const map = new Map<string, Assistant>()
    assistantsData?.assistants?.forEach((a) => map.set(a.assistant_id, a))
    return map
  }, [assistantsData])

  const threads = useMemo(() => (runs ? rollupThreads(runs) : []), [runs])

  const uniqueAssistants = useMemo(() => {
    const ids = new Set(threads.map((t) => t.latestAssistantId))
    return Array.from(ids)
  }, [threads])

  const filteredThreads = useMemo(() => {
    return threads.filter((t) => {
      if (statusFilter !== "all" && t.latestStatus !== statusFilter) return false
      if (assistantFilter !== "all" && t.latestAssistantId !== assistantFilter) return false
      if (search && !t.threadId.toLowerCase().includes(search.toLowerCase())) return false
      return true
    })
  }, [threads, statusFilter, assistantFilter, search])

  if (isLoading) {
    return (
      <div>
        <PageHeader
          title="Traces"
          description="Sessions of runs grouped by thread. Drill into any session to see its run chain."
        />
        <TracesTableSkeleton />
      </div>
    )
  }

  if (error) {
    return (
      <div>
        <PageHeader title="Traces" description="Sessions of runs grouped by thread." />
        <Card className="p-6">
          <p className="text-sm text-red-600">Failed to load traces: {error.message}</p>
        </Card>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="Traces"
        description="Sessions of runs grouped by thread. Drill into any session to see its run chain."
        actions={
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            <RefreshCw className="h-4 w-4 mr-2" />
            Refresh
          </Button>
        }
      />

      <div className="flex gap-4 mb-6">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search by thread id..."
            className="pl-9"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <Select value={assistantFilter} onValueChange={setAssistantFilter}>
          <SelectTrigger className="w-48">
            <SelectValue placeholder="All Assistants" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Assistants</SelectItem>
            {uniqueAssistants.map((id) => (
              <SelectItem key={id} value={id}>
                {assistantMap.get(id)?.name || id.slice(0, 12) + "..."}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-40">
            <SelectValue placeholder="Latest status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Status</SelectItem>
            <SelectItem value="completed">Completed</SelectItem>
            <SelectItem value="failed">Failed</SelectItem>
            <SelectItem value="in_progress">Running</SelectItem>
            <SelectItem value="queued">Queued</SelectItem>
            <SelectItem value="requires_action">Action Required</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {threads.length === 0 ? (
        <Card className="p-12 text-center">
          <Activity className="h-12 w-12 mx-auto text-muted-foreground/30 mb-4" />
          <h3 className="text-lg font-semibold mb-2">No traces yet</h3>
          <p className="text-muted-foreground">
            Run a workflow to see its trace appear here, grouped by thread.
          </p>
        </Card>
      ) : (
        <>
          <div className="border rounded-lg">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Thread</TableHead>
                  <TableHead>Latest Assistant</TableHead>
                  <TableHead>Runs</TableHead>
                  <TableHead>Latest Status</TableHead>
                  <TableHead>Last Activity</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredThreads.map((t) => (
                  <TableRow
                    key={t.threadId}
                    className="cursor-pointer hover:bg-muted/50"
                    onClick={() =>
                      navigate({ to: "/traces/$traceId", params: { traceId: t.threadId } })
                    }
                  >
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <MessageSquare className="h-4 w-4 text-muted-foreground" />
                        <span className="font-mono text-sm">{t.threadId.slice(0, 16)}...</span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Link
                        to="/assistants/$assistantId"
                        params={{ assistantId: t.latestAssistantId }}
                        className="hover:underline"
                        onClick={(e) => e.stopPropagation()}
                      >
                        {assistantMap.get(t.latestAssistantId)?.name || (
                          <span className="font-mono text-muted-foreground text-sm">
                            {t.latestAssistantId.slice(0, 12)}...
                          </span>
                        )}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <span className="font-medium">{t.runCount}</span>
                      {t.errorCount > 0 && (
                        <span className="ml-2 text-xs text-red-600">
                          ({t.errorCount} failed)
                        </span>
                      )}
                    </TableCell>
                    <TableCell>
                      <RunStatusBadge status={t.latestStatus} />
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatRelativeTime(t.lastActivity)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          <div className="flex items-center justify-between mt-4">
            <div className="text-sm text-muted-foreground">
              Showing {filteredThreads.length} of {threads.length} sessions ({runs?.length ?? 0}{" "}
              total runs)
            </div>
          </div>
        </>
      )}
    </div>
  )
}

function TracesTableSkeleton() {
  return (
    <Card className="p-4 space-y-4">
      {[...Array(5)].map((_, i) => (
        <Skeleton key={i} className="h-12 w-full" />
      ))}
    </Card>
  )
}
