import { createFileRoute, Link, Outlet, useMatch } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/api/client"
import type { Run, Assistant, AssistantsResponse } from "@/types/entities"
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
import { RefreshCw, Search, Activity } from "lucide-react"
import { RunStatusBadge } from "@/components/runs/RunStatusBadge"
import { useState, useMemo } from "react"

export const Route = createFileRoute("/_app/traces")({
  component: TracesLayout,
})

function TracesLayout() {
  const childMatch = useMatch({ from: "/_app/traces/$traceId", shouldThrow: false })

  if (childMatch) {
    return <Outlet />
  }

  return <TracesPage />
}

function formatDuration(startedAt?: string, completedAt?: string): string {
  if (!startedAt) return "—"

  const start = new Date(startedAt).getTime()
  const end = completedAt ? new Date(completedAt).getTime() : Date.now()
  const duration = (end - start) / 1000

  if (duration < 1) return `${Math.round(duration * 1000)}ms`
  if (duration < 60) return `${duration.toFixed(1)}s`
  return `${Math.floor(duration / 60)}m ${Math.round(duration % 60)}s`
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
  const [statusFilter, setStatusFilter] = useState<string>("all")
  const [assistantFilter, setAssistantFilter] = useState<string>("all")

  const { data: runs, isLoading, error, refetch } = useQuery({
    queryKey: ["runs"],
    queryFn: () => api.get<Run[]>("/runs"),
  })

  const { data: assistantsData } = useQuery({
    queryKey: ["assistants"],
    queryFn: () => api.get<AssistantsResponse>("/assistants"),
  })

  const assistantMap = useMemo(() => {
    const map = new Map<string, Assistant>()
    if (assistantsData?.assistants) {
      assistantsData.assistants.forEach((a) => map.set(a.assistant_id, a))
    }
    return map
  }, [assistantsData])

  const uniqueAssistants = useMemo(() => {
    if (!runs) return []
    const ids = new Set(runs.map((r) => r.assistant_id))
    return Array.from(ids)
  }, [runs])

  const filteredRuns = useMemo(() => {
    if (!runs) return []
    return runs.filter((run) => {
      if (statusFilter !== "all" && run.status !== statusFilter) return false
      if (assistantFilter !== "all" && run.assistant_id !== assistantFilter) return false
      return true
    })
  }, [runs, statusFilter, assistantFilter])

  if (isLoading) {
    return (
      <div>
        <PageHeader
          title="Traces"
          description="Explore agent execution traces with LLM calls, tools, and costs"
        />
        <TracesTableSkeleton />
      </div>
    )
  }

  if (error) {
    return (
      <div>
        <PageHeader
          title="Traces"
          description="Explore agent execution traces with LLM calls, tools, and costs"
        />
        <Card className="p-6">
          <p className="text-sm text-red-600">
            Failed to load traces: {error.message}
          </p>
        </Card>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="Traces"
        description="Explore agent execution traces with LLM calls, tools, and costs"
        actions={
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            <RefreshCw className="h-4 w-4 mr-2" />
            Refresh
          </Button>
        }
      />

      {/* Filters */}
      <div className="flex gap-4 mb-6">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input placeholder="Search traces..." className="pl-9" />
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
          <SelectTrigger className="w-32">
            <SelectValue placeholder="Status" />
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

      {/* Traces Table */}
      {!runs || runs.length === 0 ? (
        <Card className="p-12 text-center">
          <Activity className="h-12 w-12 mx-auto text-muted-foreground/30 mb-4" />
          <h3 className="text-lg font-semibold mb-2">No traces yet</h3>
          <p className="text-muted-foreground">
            Run executions will appear here as traces
          </p>
        </Card>
      ) : (
        <>
          <div className="border rounded-lg">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Run ID</TableHead>
                  <TableHead>Assistant</TableHead>
                  <TableHead>Thread</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Duration</TableHead>
                  <TableHead>Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredRuns.map((run) => (
                  <TableRow
                    key={run.run_id}
                    className="cursor-pointer hover:bg-muted/50"
                  >
                    <TableCell>
                      <Link
                        to="/runs/$runId"
                        params={{ runId: run.run_id }}
                        className="font-mono text-sm hover:underline"
                      >
                        {run.run_id.slice(0, 12)}...
                      </Link>
                    </TableCell>
                    <TableCell>
                      <Link
                        to="/assistants/$assistantId"
                        params={{ assistantId: run.assistant_id }}
                        className="hover:underline"
                      >
                        {assistantMap.get(run.assistant_id)?.name || (
                          <span className="font-mono text-muted-foreground">
                            {run.assistant_id.slice(0, 12)}...
                          </span>
                        )}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <Link
                        to="/threads/$threadId"
                        params={{ threadId: run.thread_id }}
                        className="font-mono text-sm text-muted-foreground hover:underline"
                      >
                        {run.thread_id.slice(0, 8)}...
                      </Link>
                    </TableCell>
                    <TableCell>
                      <RunStatusBadge status={run.status} />
                    </TableCell>
                    <TableCell className="font-mono text-sm">
                      {formatDuration(run.started_at, run.completed_at)}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatRelativeTime(run.created_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          {/* Results count */}
          <div className="flex items-center justify-between mt-4">
            <div className="text-sm text-muted-foreground">
              Showing {filteredRuns.length} of {runs.length} traces
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
