import { useState } from "react"
import { createFileRoute } from "@tanstack/react-router"
import { useAllRuns } from "@/api/runs"
import type { Run, RunStatus, NodeExecution, RunEvent } from "@/types/entities"
import { createRunStream } from "@/lib/sse"
import { RunInspector } from "@/components/debug/RunInspector"
import { GraphTopology, StatePanel } from "@/components/debug/GraphView"

// Inspector — run-level reasoning trace ported from studio's TracesView.
// Distinct from dashboard's /traces (which is the high-level run
// listing): /inspector drills into per-node execution detail, edge
// topology, and run state. Streams node_started / node_completed events
// for in-progress runs; falls back to cached output.nodes_executed for
// completed runs.

export const Route = createFileRoute("/_app/inspector")({
  component: InspectorPage,
})

const STATUS_COLORS: Record<RunStatus, string> = {
  queued: "bg-muted-foreground",
  in_progress: "bg-yellow-500 animate-pulse",
  completed: "bg-green-500",
  failed: "bg-red-500",
  cancelled: "bg-muted-foreground",
  requires_action: "bg-orange-500",
}

type DebugTab = "inspector" | "graph" | "state"

function InspectorPage() {
  const { data: runs, isLoading } = useAllRuns()
  const [selectedRun, setSelectedRun] = useState<Run | null>(null)
  const [nodeExecutions, setNodeExecutions] = useState<NodeExecution[]>([])
  const [traceLoading, setTraceLoading] = useState(false)
  const [debugTab, setDebugTab] = useState<DebugTab>("inspector")

  function loadTrace(run: Run) {
    setSelectedRun(run)
    setNodeExecutions([])

    if (run.status === "completed" || run.status === "failed") {
      // for completed runs, fetch from output metadata if available
      const nodes = (run.output?.nodes_executed as string[]) ?? []
      if (nodes.length > 0) {
        setNodeExecutions(
          nodes.map((n) => ({
            node_id: n,
            node_type: "function",
            status: "completed",
          })),
        )
        return
      }
    }

    // for in-progress or runs without cached traces, stream events
    if (!run.thread_id) return
    setTraceLoading(true)
    const cleanup = createRunStream(run.thread_id, run.run_id, {
      onEvent: (event: RunEvent) => {
        if (event.event === "node_started") {
          setNodeExecutions((prev) => [
            ...prev,
            {
              node_id: (event.data.node_id as string) ?? "unknown",
              node_type: (event.data.node_type as string) ?? "function",
              status: "started",
              started_at: new Date().toISOString(),
            },
          ])
        } else if (event.event === "node_completed") {
          setNodeExecutions((prev) => {
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
        } else if (
          event.event === "run_completed" ||
          event.event === "run_failed"
        ) {
          setTraceLoading(false)
          cleanup()
        }
      },
      onStatus: (status) => {
        if (status === "closed" || status === "error") {
          setTraceLoading(false)
        }
      },
    })
  }

  return (
    <div className="flex h-full -m-6">
      {/* Run list */}
      <div className="w-80 border-r border-border overflow-y-auto">
        <div className="p-4 border-b border-border">
          <h2 className="text-sm font-semibold">Runs</h2>
        </div>

        {isLoading && (
          <div className="p-4 text-sm text-muted-foreground">Loading...</div>
        )}

        {runs?.map((run) => (
          <button
            key={run.run_id}
            onClick={() => loadTrace(run)}
            className={`w-full border-b border-border p-3 text-left hover:bg-accent/50 transition-colors ${
              selectedRun?.run_id === run.run_id ? "bg-accent" : ""
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="text-xs font-mono text-muted-foreground">
                {run.run_id.slice(0, 8)}
              </span>
              <span className="flex items-center gap-1.5">
                <span
                  className={`inline-block h-2 w-2 ${STATUS_COLORS[run.status]}`}
                />
                <span className="text-xs">{run.status}</span>
              </span>
            </div>
            <div className="mt-1 text-xs text-muted-foreground">
              {new Date(run.created_at).toLocaleString()}
            </div>
          </button>
        )) ?? null}

        {!isLoading && (!runs || runs.length === 0) && (
          <div className="p-4 text-sm text-muted-foreground">
            No runs yet. Send a message in Playground to create one.
          </div>
        )}
      </div>

      {/* Trace detail */}
      <div className="flex-1 overflow-y-auto p-6">
        {!selectedRun ? (
          <div className="flex h-full items-center justify-center text-muted-foreground">
            <p>Select a run to view its execution trace</p>
          </div>
        ) : (
          <div className="mx-auto max-w-4xl">
            {/* Run header */}
            <div className="mb-6">
              <div className="flex items-center gap-3">
                <h2 className="text-lg font-semibold">
                  Run {selectedRun.run_id.slice(0, 8)}
                </h2>
                <span className="flex items-center gap-1.5 text-sm">
                  <span
                    className={`inline-block h-2 w-2 ${STATUS_COLORS[selectedRun.status]}`}
                  />
                  {selectedRun.status}
                </span>
              </div>
              <div className="mt-1 text-sm text-muted-foreground">
                Thread: {selectedRun.thread_id.slice(0, 8)} | Assistant:{" "}
                {selectedRun.assistant_id.slice(0, 8)} |{" "}
                {new Date(selectedRun.created_at).toLocaleString()}
              </div>
            </div>

            {/* Debug tabs */}
            <div className="mb-4 flex gap-0 border-b border-border">
              {(["inspector", "graph", "state"] as const).map((tab) => (
                <button
                  key={tab}
                  onClick={() => setDebugTab(tab)}
                  className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                    debugTab === tab
                      ? "border-foreground text-foreground"
                      : "border-transparent text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {tab === "inspector" && "Inspector"}
                  {tab === "graph" && "Graph"}
                  {tab === "state" && "State"}
                </button>
              ))}
            </div>

            {traceLoading && nodeExecutions.length === 0 && (
              <div className="text-sm text-muted-foreground animate-pulse">
                Loading trace...
              </div>
            )}

            {debugTab === "inspector" && (
              <RunInspector
                executions={nodeExecutions}
                runOutput={selectedRun.output}
              />
            )}

            {debugTab === "graph" && (
              <GraphTopology
                nodes={nodeExecutions.map((e) => ({
                  id: e.node_id,
                  type: e.node_type,
                }))}
                edges={nodeExecutions.slice(1).map((e, i) => ({
                  source: nodeExecutions[i].node_id,
                  target: e.node_id,
                }))}
                activeNodeId={
                  nodeExecutions.find((e) => e.status === "started")?.node_id
                }
                completedNodeIds={nodeExecutions
                  .filter((e) => e.status === "completed")
                  .map((e) => e.node_id)}
              />
            )}

            {debugTab === "state" && (
              <StatePanel
                state={(selectedRun.output ?? {}) as Record<string, unknown>}
                previousState={
                  (selectedRun.input ?? {}) as Record<string, unknown>
                }
              />
            )}

            {selectedRun.error && (
              <div className="mt-6">
                <h3 className="mb-2 text-sm font-semibold text-red-600">
                  Error
                </h3>
                <pre className="overflow-x-auto bg-red-50 p-3 text-xs font-mono text-red-700">
                  {selectedRun.error}
                </pre>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
