import { useState } from 'react'
import { useAllRuns } from '@/api/runs'
import type { Run, RunStatus, NodeExecution } from '@/types/entities'
import { createRunStream } from '@/lib/sse'
import type { RunEvent } from '@/types/entities'

const STATUS_COLORS: Record<RunStatus, string> = {
  queued: 'bg-muted-foreground',
  in_progress: 'bg-yellow-500 animate-pulse',
  completed: 'bg-green-500',
  failed: 'bg-red-500',
  cancelled: 'bg-muted-foreground',
  requires_action: 'bg-orange-500',
}

const NODE_TYPE_STYLES: Record<string, { icon: string; color: string }> = {
  function: { icon: 'fn', color: 'bg-blue-500/10 text-blue-600 border-blue-200' },
  llm: { icon: 'AI', color: 'bg-purple-500/10 text-purple-600 border-purple-200' },
  tool: { icon: 'T', color: 'bg-green-500/10 text-green-600 border-green-200' },
  router: { icon: 'R', color: 'bg-orange-500/10 text-orange-600 border-orange-200' },
  human: { icon: 'H', color: 'bg-pink-500/10 text-pink-600 border-pink-200' },
}

function getNodeStyle(type: string) {
  return NODE_TYPE_STYLES[type] ?? NODE_TYPE_STYLES['function']
}

export function TracesView() {
  const { data: runs, isLoading } = useAllRuns()
  const [selectedRun, setSelectedRun] = useState<Run | null>(null)
  const [nodeExecutions, setNodeExecutions] = useState<NodeExecution[]>([])
  const [traceLoading, setTraceLoading] = useState(false)

  function loadTrace(run: Run) {
    setSelectedRun(run)
    setNodeExecutions([])

    if (run.status === 'completed' || run.status === 'failed') {
      // for completed runs, fetch from output metadata if available
      const nodes = (run.output?.nodes_executed as string[]) ?? []
      if (nodes.length > 0) {
        setNodeExecutions(
          nodes.map((n) => ({
            node_id: n,
            node_type: 'function',
            status: 'completed',
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
        if (event.event === 'node_started') {
          setNodeExecutions((prev) => [
            ...prev,
            {
              node_id: (event.data.node_id as string) ?? 'unknown',
              node_type: (event.data.node_type as string) ?? 'function',
              status: 'started',
              started_at: new Date().toISOString(),
            },
          ])
        } else if (event.event === 'node_completed') {
          setNodeExecutions((prev) => {
            const idx = prev.findIndex(
              (n) => n.node_id === event.data.node_id && n.status === 'started',
            )
            if (idx >= 0) {
              const updated = [...prev]
              updated[idx] = {
                ...updated[idx],
                status: 'completed',
                output: event.data.output as Record<string, unknown>,
                completed_at: new Date().toISOString(),
              }
              return updated
            }
            return [
              ...prev,
              {
                node_id: (event.data.node_id as string) ?? 'unknown',
                node_type: (event.data.node_type as string) ?? 'function',
                status: 'completed',
                output: event.data.output as Record<string, unknown>,
                completed_at: new Date().toISOString(),
              },
            ]
          })
        } else if (
          event.event === 'run_completed' ||
          event.event === 'run_failed'
        ) {
          setTraceLoading(false)
          cleanup()
        }
      },
      onStatus: (status) => {
        if (status === 'closed' || status === 'error') {
          setTraceLoading(false)
        }
      },
    })
  }

  return (
    <div className="flex h-full">
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
              selectedRun?.run_id === run.run_id ? 'bg-accent' : ''
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="text-xs font-mono text-muted-foreground">
                {run.run_id.slice(0, 8)}
              </span>
              <span className="flex items-center gap-1.5">
                <span className={`inline-block h-2 w-2 ${STATUS_COLORS[run.status]}`} />
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
            No runs yet. Send a message in Chat to create one.
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
          <div className="mx-auto max-w-3xl">
            {/* Run header */}
            <div className="mb-6">
              <div className="flex items-center gap-3">
                <h2 className="text-lg font-semibold">Run {selectedRun.run_id.slice(0, 8)}</h2>
                <span className="flex items-center gap-1.5 text-sm">
                  <span
                    className={`inline-block h-2 w-2 ${STATUS_COLORS[selectedRun.status]}`}
                  />
                  {selectedRun.status}
                </span>
              </div>
              <div className="mt-1 text-sm text-muted-foreground">
                Thread: {selectedRun.thread_id.slice(0, 8)} | Assistant:{' '}
                {selectedRun.assistant_id.slice(0, 8)} |{' '}
                {new Date(selectedRun.created_at).toLocaleString()}
              </div>
            </div>

            {/* Timeline */}
            {traceLoading && nodeExecutions.length === 0 && (
              <div className="text-sm text-muted-foreground animate-pulse">
                Loading trace...
              </div>
            )}

            <div className="space-y-0">
              {nodeExecutions.map((exec, i) => {
                const style = getNodeStyle(exec.node_type)
                return (
                  <div key={`${exec.node_id}-${i}`} className="flex gap-3">
                    {/* Timeline connector */}
                    <div className="flex flex-col items-center">
                      <div
                        className={`flex h-8 w-8 items-center justify-center border text-xs font-mono font-bold ${style.color}`}
                      >
                        {style.icon}
                      </div>
                      {i < nodeExecutions.length - 1 && (
                        <div className="w-px flex-1 bg-border" />
                      )}
                    </div>

                    {/* Node detail */}
                    <div className="flex-1 pb-4">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-sm">{exec.node_id}</span>
                        <span className="text-xs text-muted-foreground">
                          {exec.node_type}
                        </span>
                        {exec.status === 'started' && (
                          <span className="text-xs text-yellow-600 animate-pulse">
                            running...
                          </span>
                        )}
                        {exec.status === 'failed' && (
                          <span className="text-xs text-red-600">failed</span>
                        )}
                      </div>

                      {exec.error && (
                        <div className="mt-1 text-xs text-red-600 font-mono bg-red-50 p-2">
                          {exec.error}
                        </div>
                      )}

                      {exec.output && (
                        <details className="mt-1">
                          <summary className="cursor-pointer text-xs text-muted-foreground hover:text-foreground">
                            Output
                          </summary>
                          <pre className="mt-1 overflow-x-auto bg-muted p-2 text-xs font-mono">
                            {JSON.stringify(exec.output, null, 2)}
                          </pre>
                        </details>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>

            {/* Run output */}
            {selectedRun.output && (
              <div className="mt-6">
                <h3 className="mb-2 text-sm font-semibold">Output</h3>
                <pre className="overflow-x-auto bg-muted p-3 text-xs font-mono">
                  {JSON.stringify(selectedRun.output, null, 2)}
                </pre>
              </div>
            )}

            {selectedRun.error && (
              <div className="mt-6">
                <h3 className="mb-2 text-sm font-semibold text-red-600">Error</h3>
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
