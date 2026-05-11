import { createFileRoute, Link } from "@tanstack/react-router"
import { runsPollInterval } from "@/api/runs"
import { useQuery } from "@tanstack/react-query"
import { useMemo, useState, useEffect } from "react"
import { api } from "@/api/client"
import type {
  Run,
  RunStatus,
  Assistant,
  AssistantsResponse,
  Graph,
  Thread,
  NodeExecution,
  RunEvent,
} from "@/types/entities"
import { PageHeader } from "@/components/layout/PageHeader"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"
import { Separator } from "@/components/ui/separator"
import { RunStatusBadge } from "@/components/runs/RunStatusBadge"
import { RunInspector } from "@/components/runs/RunInspector"
import { StatePanel } from "@/components/runs/StatePanel"
import { JsonView } from "@/components/common/JsonView"
import {
  GraphVisualizer,
  type ExecutionStatus,
} from "@/components/graph/GraphVisualizer"
import { createRunStream } from "@/lib/sse"
import {
  ArrowLeft,
  Clock,
  AlertCircle,
  GitBranch,
  ExternalLink,
  Activity,
} from "lucide-react"

export const Route = createFileRoute("/_app/traces/$traceId")({
  component: SessionDetailPage,
})

// /traces/$traceId renders the **session** view — a vertical chain of
// all runs that share this thread, plus a detail pane for the
// currently-selected run. `traceId` is treated as the thread id; the
// route name stays `$traceId` so the Langfuse mental model (a trace =
// a session of related runs) holds at the URL layer. In DuraGraph
// terms this is `/threads/{id}/runs` plus a right-pane run inspector.

function durationOf(run: Run): string {
  if (!run.started_at) return "—"
  const start = new Date(run.started_at).getTime()
  const end = run.completed_at ? new Date(run.completed_at).getTime() : Date.now()
  const ms = end - start
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)}s`
  return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`
}

function inputPreview(run: Run): string {
  const raw = run.input
  if (!raw) return ""
  const messages = (raw as { messages?: unknown }).messages
  if (Array.isArray(messages) && messages.length > 0) {
    const last = messages[messages.length - 1] as { content?: unknown; role?: unknown }
    if (typeof last.content === "string") return last.content
  }
  if (typeof (raw as { message?: unknown }).message === "string") {
    return (raw as { message: string }).message
  }
  try {
    return JSON.stringify(raw)
  } catch {
    return ""
  }
}

function outputPreview(run: Run): string {
  const raw = run.output
  if (!raw) return ""
  const messages = (raw as { messages?: unknown }).messages
  if (Array.isArray(messages) && messages.length > 0) {
    const last = messages[messages.length - 1] as { content?: unknown }
    if (typeof last.content === "string") return last.content
  }
  if (typeof (raw as { response?: unknown }).response === "string") {
    return (raw as { response: string }).response
  }
  try {
    return JSON.stringify(raw)
  } catch {
    return ""
  }
}

function SessionDetailPage() {
  const { traceId: threadId } = Route.useParams()

  const { data: thread } = useQuery({
    queryKey: ["thread", threadId],
    queryFn: () => api.get<Thread>(`/threads/${threadId}`),
  })

  const {
    data: runs,
    isLoading: runsLoading,
    error: runsError,
  } = useQuery({
    queryKey: ["runs", threadId],
    queryFn: () => api.get<Run[]>(`/threads/${threadId}/runs`),
    // Same adaptive cadence as /traces — 1.5 s while a run is mid-
    // flight, 15 s when the session is idle. Without this we
    // refetched the thread's whole run history every 3 s forever.
    refetchInterval: (q) => runsPollInterval(q.state.data as Run[] | undefined),
  })

  // Chain ordered oldest-first so the "first message" sits at the top
  // and conversation reads top-to-bottom, matching the Langfuse session
  // replay layout. The connector line drops from each card to the next.
  const chain = useMemo(() => {
    if (!runs) return []
    return [...runs].sort(
      (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
    )
  }, [runs])

  const { data: assistantsData } = useQuery({
    queryKey: ["assistants"],
    queryFn: () => api.get<AssistantsResponse>("/assistants"),
  })

  const assistantMap = useMemo(() => {
    const map = new Map<string, Assistant>()
    assistantsData?.assistants?.forEach((a) => map.set(a.assistant_id, a))
    return map
  }, [assistantsData])

  // Right-pane selection. `null` means "show the latest" — we don't
  // commit a default into state because the chain mutates as new runs
  // arrive (poll-driven), and a stale id would either pin the user to
  // an old run or require an effect to keep it fresh. Deriving the
  // fallback at render time keeps the latest run selected by default
  // and lets explicit clicks pin a different one.
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null)
  const effectiveSelectedId =
    selectedRunId ?? (chain.length > 0 ? chain[chain.length - 1].run_id : null)
  const selectedRun = useMemo(
    () => chain.find((r) => r.run_id === effectiveSelectedId) ?? null,
    [chain, effectiveSelectedId],
  )

  return (
    <div>
      <div className="mb-4">
        <Link
          to="/traces"
          className="text-sm text-muted-foreground hover:text-foreground inline-flex items-center gap-1"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Traces
        </Link>
      </div>

      <PageHeader
        title={
          <div className="flex items-center gap-3">
            <span className="font-mono text-base">{threadId.slice(0, 16)}...</span>
            <Badge variant="outline">
              {chain.length} {chain.length === 1 ? "run" : "runs"}
            </Badge>
          </div>
        }
        description={
          thread?.created_at
            ? `Session started ${new Date(thread.created_at).toLocaleString()}`
            : "Loading session metadata..."
        }
        actions={
          <Link to="/threads/$threadId" params={{ threadId }}>
            <Button variant="outline" size="sm">
              <ExternalLink className="h-4 w-4 mr-2" />
              View thread
            </Button>
          </Link>
        }
      />

      {runsLoading ? (
        <SessionSkeleton />
      ) : runsError ? (
        <Card className="p-6">
          <p className="text-sm text-red-600">
            Failed to load session: {runsError.message}
          </p>
        </Card>
      ) : chain.length === 0 ? (
        <Card className="p-12 text-center">
          <Activity className="h-12 w-12 mx-auto text-muted-foreground/30 mb-4" />
          <h3 className="text-lg font-semibold mb-2">No runs in this session</h3>
          <p className="text-muted-foreground">
            Once a run is created for this thread it will appear here.
          </p>
        </Card>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-[380px_1fr] gap-6">
          <RunChainRail
            chain={chain}
            selectedRunId={effectiveSelectedId}
            onSelect={setSelectedRunId}
            assistantMap={assistantMap}
          />
          <RunDetailPane
            run={selectedRun}
            assistant={selectedRun ? assistantMap.get(selectedRun.assistant_id) : undefined}
          />
        </div>
      )}
    </div>
  )
}

// RunChainRail — the vertical chain on the left. Each run is rendered
// as a card connected by a continuous vertical line (a true "chain"
// metaphor). The latest run is at the BOTTOM; this matches both the
// chronological "scroll down to read forward" affordance and the
// Langfuse session replay layout where you read top-to-bottom.
function RunChainRail({
  chain,
  selectedRunId,
  onSelect,
  assistantMap,
}: {
  chain: Run[]
  selectedRunId: string | null
  onSelect: (id: string) => void
  assistantMap: Map<string, Assistant>
}) {
  return (
    <Card className="overflow-hidden">
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Run chain</CardTitle>
        <CardDescription>Oldest first — click a run to inspect</CardDescription>
      </CardHeader>
      <Separator />
      <ScrollArea className="h-[calc(100vh-280px)]">
        <div className="relative px-4 py-4">
          {/* The vertical chain spine. Sits behind the cards so each
              card's left margin reveals the line + a centered dot. */}
          <div className="absolute left-7 top-6 bottom-6 w-px bg-border" aria-hidden />
          <ol className="space-y-3">
            {chain.map((run, i) => {
              const isSelected = run.run_id === selectedRunId
              const assistant = assistantMap.get(run.assistant_id)
              return (
                <li key={run.run_id} className="relative pl-8">
                  {/* Dot on the spine. Coloured by status so the chain
                      reads as a stream of green/red/amber pearls. */}
                  <span
                    className={
                      "absolute left-[18px] top-3 h-3 w-3 rounded-full ring-4 ring-background " +
                      dotColorForStatus(run.status as RunStatus)
                    }
                    aria-hidden
                  />
                  <button
                    type="button"
                    onClick={() => onSelect(run.run_id)}
                    className={
                      "w-full text-left rounded-md border p-3 transition-colors " +
                      (isSelected
                        ? "border-primary bg-primary/5"
                        : "border-border hover:bg-muted/50")
                    }
                  >
                    <div className="flex items-center justify-between gap-2 mb-1">
                      <span className="text-xs font-medium text-muted-foreground">
                        Run {i + 1} ·{" "}
                        <span className="font-mono">{run.run_id.slice(0, 8)}</span>
                      </span>
                      <RunStatusBadge status={run.status as RunStatus} />
                    </div>
                    <div className="text-xs text-muted-foreground mb-1">
                      {assistant?.name || run.assistant_id.slice(0, 12)} ·{" "}
                      {durationOf(run)} · {new Date(run.created_at).toLocaleTimeString()}
                    </div>
                    {inputPreview(run) && (
                      <p className="text-sm line-clamp-2 mt-1">{inputPreview(run)}</p>
                    )}
                  </button>
                </li>
              )
            })}
          </ol>
        </div>
      </ScrollArea>
    </Card>
  )
}

function dotColorForStatus(status: RunStatus): string {
  switch (status) {
    case "completed":
      return "bg-emerald-500"
    case "failed":
      return "bg-red-500"
    case "in_progress":
      return "bg-primary animate-pulse"
    case "requires_action":
      return "bg-amber-500"
    case "cancelled":
      return "bg-muted-foreground"
    case "queued":
    default:
      return "bg-muted-foreground/50"
  }
}

// useNodeExecutions — derives a `NodeExecution[]` for a run from two
// sources:
//
//   1. Live runs (`in_progress` / `queued`) subscribe to the SSE
//      `/threads/{id}/runs/{id}/stream` and append a row on every
//      `node_started` event, then close it on the matching
//      `node_completed`. Resubscribes when the user selects a
//      different in-progress run.
//
//   2. Terminal runs (`completed` / `failed` / `cancelled`) have no
//      live stream to replay, so we fall back to whatever the engine
//      cached in `run.output.nodes_executed` (an array of node ids).
//      The engine currently writes this only for some run types;
//      where it isn't written, the Spans tab will show "no recorded
//      executions" — that's a known gap to close with a proper
//      per-run spans REST endpoint.
//
// This hook used to live in the now-deleted `/inspector` route; it's
// the unified source of truth for span data inside `/traces/$traceId`.
function useNodeExecutions(run: Run | null): NodeExecution[] {
  const isLive = run?.status === "in_progress" || run?.status === "queued"

  // Terminal runs are pure-derive (no state needed) — the engine has
  // already settled the run.output.nodes_executed array by the time we
  // open the page.
  const cached = useMemo<NodeExecution[]>(() => {
    if (!run || isLive) return []
    const nodes = (run.output?.nodes_executed as string[] | undefined) ?? []
    return nodes.map((n) => ({
      node_id: n,
      node_type: "function",
      status: "completed" as const,
    }))
  }, [run, isLive])

  // Live runs need state so SSE callbacks can append. We reset the
  // accumulator using React's "store-information-from-previous-renders"
  // pattern (setState during render after a prev-id mismatch) instead
  // of putting setState in the effect body — that keeps the effect a
  // pure subscription with no cascading renders.
  const [live, setLive] = useState<NodeExecution[]>([])
  const [trackedRunId, setTrackedRunId] = useState<string | null>(null)
  const liveRunId = isLive && run ? run.run_id : null
  if (liveRunId !== trackedRunId) {
    setTrackedRunId(liveRunId)
    setLive([])
  }

  useEffect(() => {
    if (!run || !isLive) return
    const cleanup = createRunStream(run.thread_id, run.run_id, {
      onEvent: (event: RunEvent) => {
        if (event.event === "node_started") {
          setLive((prev) => [
            ...prev,
            {
              node_id: (event.data.node_id as string) ?? "unknown",
              node_type: (event.data.node_type as string) ?? "function",
              status: "started",
              started_at: new Date().toISOString(),
            },
          ])
        } else if (event.event === "node_completed") {
          setLive((prev) => {
            const idx = prev.findIndex(
              (n) =>
                n.node_id === event.data.node_id && n.status === "started",
            )
            if (idx >= 0) {
              const updated = [...prev]
              updated[idx] = {
                ...updated[idx],
                status: "completed",
                output: event.data.output as Record<string, unknown>,
                completed_at: new Date().toISOString(),
              }
              return updated
            }
            // Race: completed fired without us seeing started. Just
            // append a fully-formed completed row.
            return [
              ...prev,
              {
                node_id: (event.data.node_id as string) ?? "unknown",
                node_type: (event.data.node_type as string) ?? "function",
                status: "completed",
                output: event.data.output as Record<string, unknown>,
                completed_at: new Date().toISOString(),
              },
            ]
          })
        }
      },
      onStatus: () => {
        // Status transitions are owned by the parent run query's
        // polling; nothing to do here.
      },
    })
    return cleanup
  }, [run, isLive])

  return isLive ? live : cached
}

// RunDetailPane — the right-pane inspector for the currently-selected
// run. Mirrors /runs/$runId's tab layout but compacted for the
// session-view split. The Graph tab uses GraphVisualizer with derived
// per-node statuses; Spans + State tabs render the per-node accordion
// + run-level state diff previously hosted in the standalone
// /inspector route.
function RunDetailPane({
  run,
  assistant,
}: {
  run: Run | null
  assistant: Assistant | undefined
}) {
  const executions = useNodeExecutions(run)

  const { data: graph, isLoading: graphLoading } = useQuery({
    queryKey: ["assistant-graph", run?.assistant_id],
    queryFn: () => api.get<Graph>(`/assistants/${run?.assistant_id}/graph`),
    enabled: !!run?.assistant_id,
  })

  if (!run) {
    return (
      <Card className="p-12 text-center">
        <p className="text-sm text-muted-foreground">Select a run to inspect</p>
      </Card>
    )
  }

  // Per-node Graph-tab statuses. When real per-node execution data is
  // present (live SSE or cached `nodes_executed`), prefer that — paint
  // started nodes as `running`, completed as `completed`, leave the
  // rest idle. Otherwise fall back to the coarse run-level paint
  // (everything green/red/yellow depending on run.status).
  const nodeStatuses: Record<string, ExecutionStatus> | undefined = (() => {
    if (!graph) return undefined

    if (executions.length > 0) {
      const map: Record<string, ExecutionStatus> = {}
      for (const exec of executions) {
        if (exec.status === "started") map[exec.node_id] = "running"
        else if (exec.status === "completed") map[exec.node_id] = "completed"
        else if (exec.status === "failed") map[exec.node_id] = "error"
      }
      return map
    }

    if (run.status === "completed") {
      return Object.fromEntries(graph.nodes.map((n) => [n.id, "completed" as ExecutionStatus]))
    }
    if (run.status === "failed") {
      return Object.fromEntries(graph.nodes.map((n) => [n.id, "error" as ExecutionStatus]))
    }
    if (run.status === "in_progress") {
      return Object.fromEntries(graph.nodes.map((n) => [n.id, "running" as ExecutionStatus]))
    }
    return undefined
  })()

  return (
    <div className="space-y-4">
      {/* Compact run header — the Langfuse trace-detail "stats bar". */}
      <Card>
        <CardContent className="pt-6">
          <div className="flex flex-wrap items-center gap-6">
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Run</span>
              <Link
                to="/runs/$runId"
                params={{ runId: run.run_id }}
                className="font-mono text-sm hover:underline inline-flex items-center gap-1"
              >
                {run.run_id.slice(0, 16)}...
                <ExternalLink className="h-3 w-3" />
              </Link>
            </div>
            <RunStatusBadge status={run.status as RunStatus} />
            <div className="flex items-center gap-2 text-sm">
              <Clock className="h-4 w-4 text-muted-foreground" />
              <span className="font-mono">{durationOf(run)}</span>
            </div>
            <div className="text-sm text-muted-foreground">
              {assistant?.name || run.assistant_id.slice(0, 12)} ·{" "}
              {new Date(run.created_at).toLocaleString()}
            </div>
          </div>
          {run.error && (
            <div className="mt-4 p-3 rounded-md border border-red-200 bg-red-50 dark:bg-red-950/30 dark:border-red-900">
              <div className="flex items-start gap-2">
                <AlertCircle className="h-4 w-4 text-red-600 dark:text-red-400 mt-0.5" />
                <div>
                  <div className="text-sm font-medium text-red-800 dark:text-red-200">
                    Run failed
                  </div>
                  <div className="text-sm text-red-700 dark:text-red-300 mt-1">
                    {run.error}
                  </div>
                </div>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Tabs defaultValue="conversation">
        <TabsList>
          <TabsTrigger value="conversation">Conversation</TabsTrigger>
          <TabsTrigger value="spans">Spans</TabsTrigger>
          <TabsTrigger value="graph">Graph</TabsTrigger>
          <TabsTrigger value="state">State</TabsTrigger>
          <TabsTrigger value="io">I/O</TabsTrigger>
          <TabsTrigger value="metadata">Metadata</TabsTrigger>
        </TabsList>

        <TabsContent value="conversation" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Conversation</CardTitle>
              <CardDescription>
                The user message that initiated this run and the assistant's response.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <ConversationBubble role="user" content={inputPreview(run)} />
              {run.status === "in_progress" ? (
                <ConversationBubble role="assistant" content="…" pending />
              ) : (
                <ConversationBubble role="assistant" content={outputPreview(run)} />
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="spans" className="mt-4">
          {executions.length === 0 ? (
            <Card className="p-12 text-center">
              <p className="text-sm text-muted-foreground">
                {run.status === "in_progress" || run.status === "queued"
                  ? "Waiting for the first node to start…"
                  : "No per-node execution data was recorded for this run. Re-run from Playground to capture live spans."}
              </p>
            </Card>
          ) : (
            <RunInspector executions={executions} runOutput={run.output} />
          )}
        </TabsContent>

        <TabsContent value="graph" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <GitBranch className="h-5 w-5" />
                Execution graph
              </CardTitle>
              <CardDescription>
                Each node is one step in the workflow. Per-node status comes from
                live SSE events when available; otherwise it falls back to the
                run's terminal status.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {graphLoading ? (
                <Skeleton className="h-[460px] w-full" />
              ) : graph ? (
                <div className="h-[460px] border rounded-lg overflow-hidden">
                  <GraphVisualizer
                    graph={graph}
                    nodeStatuses={nodeStatuses}
                    showMiniMap={false}
                  />
                </div>
              ) : (
                <div className="h-[300px] flex items-center justify-center bg-muted/20 rounded-lg border border-dashed">
                  <p className="text-sm text-muted-foreground">
                    No graph topology available for this assistant
                  </p>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="state" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>State diff</CardTitle>
              <CardDescription>
                Keys present in the run's final state, highlighted by what the
                input did or didn't contain. <span className="font-medium">New</span>{" "}
                = key didn't exist on input. <span className="font-medium">Changed</span>{" "}
                = key existed but the value moved.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <StatePanel
                state={(run.output ?? {}) as Record<string, unknown>}
                previousState={(run.input ?? {}) as Record<string, unknown>}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="io" className="mt-4">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <Card>
              <CardHeader>
                <CardTitle>Input payload</CardTitle>
              </CardHeader>
              <CardContent>
                <JsonView value={run.input ?? {}} />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Output payload</CardTitle>
              </CardHeader>
              <CardContent>
                {run.output ? (
                  <JsonView value={run.output} />
                ) : (
                  <p className="text-sm text-muted-foreground">No output yet.</p>
                )}
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="metadata" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Metadata</CardTitle>
              <CardDescription>
                Caller-attached key/value bag (correlation ids, A/B tags, etc.).
                Round-tripped unchanged by the engine.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {run.metadata && Object.keys(run.metadata).length > 0 ? (
                <JsonView value={run.metadata} />
              ) : (
                <p className="text-sm text-muted-foreground">
                  No metadata was attached to this run.
                </p>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function ConversationBubble({
  role,
  content,
  pending,
}: {
  role: "user" | "assistant"
  content: string
  pending?: boolean
}) {
  const isUser = role === "user"
  return (
    <div className={"flex " + (isUser ? "justify-end" : "justify-start")}>
      <div
        className={
          "max-w-[80%] rounded-lg px-4 py-2 text-sm " +
          (isUser
            ? "bg-primary text-primary-foreground"
            : "bg-muted text-foreground border")
        }
      >
        <div className="text-xs font-medium opacity-70 mb-1">
          {isUser ? "User" : "Assistant"}
        </div>
        <div className={"whitespace-pre-wrap " + (pending ? "animate-pulse" : "")}>
          {content || (
            <span className="opacity-60 italic">(empty)</span>
          )}
        </div>
      </div>
    </div>
  )
}

function SessionSkeleton() {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-[380px_1fr] gap-6">
      <Card className="p-4 space-y-3">
        {[...Array(4)].map((_, i) => (
          <Skeleton key={i} className="h-20 w-full" />
        ))}
      </Card>
      <div className="space-y-4">
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-10 w-80" />
        <Skeleton className="h-64 w-full" />
      </div>
    </div>
  )
}
