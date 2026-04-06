import type { NodeExecution } from '@/types/entities'

const NODE_ICONS: Record<string, string> = {
  function: 'fn',
  llm: 'AI',
  tool: 'T',
  router: 'R',
  human: 'H',
}

interface NodeExecutionPanelProps {
  executions: NodeExecution[]
}

export function NodeExecutionPanel({ executions }: NodeExecutionPanelProps) {
  return (
    <div className="w-72 border-l border-border overflow-y-auto bg-card">
      <div className="border-b border-border p-3">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Execution
        </h3>
      </div>
      <div className="p-3 space-y-2">
        {executions.map((exec, i) => (
          <div
            key={`${exec.node_id}-${i}`}
            className="flex items-start gap-2 border border-border p-2"
          >
            <span className="flex h-6 w-6 shrink-0 items-center justify-center bg-muted text-[10px] font-mono font-bold">
              {NODE_ICONS[exec.node_type] ?? 'fn'}
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5">
                <span className="text-xs font-medium truncate">
                  {exec.node_id}
                </span>
                <span
                  className={`inline-block h-1.5 w-1.5 shrink-0 ${
                    exec.status === 'completed'
                      ? 'bg-green-500'
                      : exec.status === 'started'
                        ? 'bg-yellow-500 animate-pulse'
                        : 'bg-red-500'
                  }`}
                />
              </div>
              {exec.error && (
                <p className="mt-0.5 text-[10px] text-red-600 truncate">
                  {exec.error}
                </p>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
