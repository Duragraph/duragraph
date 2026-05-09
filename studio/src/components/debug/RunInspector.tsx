import { useState } from 'react'
import type { NodeExecution } from '@/types/entities'

interface RunInspectorProps {
  executions: NodeExecution[]
  runOutput?: Record<string, unknown>
}

const NODE_TYPE_STYLES: Record<string, { icon: string; color: string }> = {
  function: { icon: 'fn', color: 'bg-blue-500/10 text-blue-600 border-blue-200' },
  llm: { icon: 'AI', color: 'bg-purple-500/10 text-purple-600 border-purple-200' },
  tool: { icon: 'T', color: 'bg-green-500/10 text-green-600 border-green-200' },
  router: { icon: 'R', color: 'bg-orange-500/10 text-orange-600 border-orange-200' },
  human: { icon: 'H', color: 'bg-pink-500/10 text-pink-600 border-pink-200' },
  subgraph: { icon: 'SG', color: 'bg-cyan-500/10 text-cyan-600 border-cyan-200' },
}

function getNodeStyle(type: string) {
  return NODE_TYPE_STYLES[type] ?? NODE_TYPE_STYLES['function']
}

function formatDuration(ms?: number): string {
  if (ms == null) return '-'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function computeDuration(exec: NodeExecution): number | undefined {
  if (exec.duration_ms != null) return exec.duration_ms
  if (exec.started_at && exec.completed_at) {
    return new Date(exec.completed_at).getTime() - new Date(exec.started_at).getTime()
  }
  return undefined
}

function StateDiff({
  prev,
  next,
}: {
  prev: Record<string, unknown>
  next: Record<string, unknown>
}) {
  const allKeys = Array.from(new Set([...Object.keys(prev), ...Object.keys(next)]))
  const changes = allKeys.filter(
    (k) => JSON.stringify(prev[k]) !== JSON.stringify(next[k]),
  )

  if (changes.length === 0) {
    return <span className="text-xs text-muted-foreground">No state changes</span>
  }

  return (
    <div className="space-y-1">
      {changes.map((key) => (
        <div key={key} className="text-xs font-mono">
          <span className="font-semibold text-foreground">{key}:</span>{' '}
          {key in prev && (
            <span className="text-red-500 line-through">
              {JSON.stringify(prev[key])}
            </span>
          )}{' '}
          {key in next && (
            <span className="text-green-600">{JSON.stringify(next[key])}</span>
          )}
        </div>
      ))}
    </div>
  )
}

export function RunInspector({ executions, runOutput }: RunInspectorProps) {
  const [expandedIdx, setExpandedIdx] = useState<number | null>(null)

  const totalDuration = executions.reduce(
    (sum, e) => sum + (computeDuration(e) ?? 0),
    0,
  )
  const maxDuration = Math.max(
    ...executions.map((e) => computeDuration(e) ?? 0),
    1,
  )

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h3 className="text-sm font-semibold">Execution Inspector</h3>
        <span className="text-xs text-muted-foreground">
          {executions.length} nodes | {formatDuration(totalDuration)}
        </span>
      </div>

      {/* Timing waterfall */}
      <div className="mb-6 space-y-1">
        {executions.map((exec, i) => {
          const dur = computeDuration(exec) ?? 0
          const pct = maxDuration > 0 ? (dur / maxDuration) * 100 : 0
          const style = getNodeStyle(exec.node_type)

          return (
            <div key={`${exec.node_id}-${i}`} className="flex items-center gap-2">
              <span className="w-24 truncate text-xs font-mono">{exec.node_id}</span>
              <div className="flex-1 h-5 bg-muted relative">
                <div
                  className={`h-full ${style.color} border`}
                  style={{ width: `${Math.max(pct, 2)}%` }}
                />
              </div>
              <span className="w-16 text-right text-xs font-mono text-muted-foreground">
                {formatDuration(dur)}
              </span>
            </div>
          )
        })}
      </div>

      {/* Expandable node details */}
      <div className="space-y-0">
        {executions.map((exec, i) => {
          const style = getNodeStyle(exec.node_type)
          const expanded = expandedIdx === i
          const prevOutput =
            i > 0 ? (executions[i - 1].output ?? {}) : {}

          return (
            <div key={`detail-${exec.node_id}-${i}`}>
              <button
                onClick={() => setExpandedIdx(expanded ? null : i)}
                className="flex w-full items-center gap-3 border-b border-border p-2 text-left hover:bg-accent/50 transition-colors"
              >
                <div
                  className={`flex h-6 w-6 items-center justify-center border text-xs font-mono font-bold ${style.color}`}
                >
                  {style.icon}
                </div>
                <span className="flex-1 text-sm font-medium">{exec.node_id}</span>
                <span className="text-xs text-muted-foreground">
                  {exec.node_type}
                </span>
                {exec.status === 'started' && (
                  <span className="text-xs text-yellow-600 animate-pulse">
                    running
                  </span>
                )}
                {exec.status === 'failed' && (
                  <span className="text-xs text-red-600">failed</span>
                )}
                {exec.status === 'completed' && (
                  <span className="text-xs text-green-600">
                    {formatDuration(computeDuration(exec))}
                  </span>
                )}
                <span className="text-xs text-muted-foreground">
                  {expanded ? '▼' : '▶'}
                </span>
              </button>

              {expanded && (
                <div className="border-b border-border bg-muted/30 p-3 space-y-3">
                  {exec.error && (
                    <div>
                      <div className="text-xs font-semibold text-red-600 mb-1">Error</div>
                      <pre className="text-xs font-mono bg-red-50 p-2 text-red-700">
                        {exec.error}
                      </pre>
                    </div>
                  )}

                  {exec.input && (
                    <div>
                      <div className="text-xs font-semibold mb-1">Input</div>
                      <pre className="text-xs font-mono bg-muted p-2 overflow-x-auto max-h-40 overflow-y-auto">
                        {JSON.stringify(exec.input, null, 2)}
                      </pre>
                    </div>
                  )}

                  {exec.output && (
                    <div>
                      <div className="text-xs font-semibold mb-1">Output</div>
                      <pre className="text-xs font-mono bg-muted p-2 overflow-x-auto max-h-40 overflow-y-auto">
                        {JSON.stringify(exec.output, null, 2)}
                      </pre>
                    </div>
                  )}

                  <div>
                    <div className="text-xs font-semibold mb-1">State Changes</div>
                    <StateDiff
                      prev={prevOutput as Record<string, unknown>}
                      next={(exec.output ?? {}) as Record<string, unknown>}
                    />
                  </div>
                </div>
              )}
            </div>
          )
        })}
      </div>

      {runOutput && (
        <div className="mt-4">
          <h4 className="mb-1 text-xs font-semibold">Final Output</h4>
          <pre className="text-xs font-mono bg-muted p-2 overflow-x-auto max-h-60 overflow-y-auto">
            {JSON.stringify(runOutput, null, 2)}
          </pre>
        </div>
      )}
    </div>
  )
}
