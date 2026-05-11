import {
  Bot,
  CircuitBoard,
  GitBranch,
  Layers,
  User as UserIcon,
  Wrench,
  type LucideIcon,
} from "lucide-react"
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { cn } from "@/lib/utils"
import type { NodeExecution } from "@/types/entities"

interface RunInspectorProps {
  executions: NodeExecution[]
  runOutput?: Record<string, unknown>
}

interface NodeStyle {
  icon: LucideIcon
  badge: string
}

const NODE_TYPE_STYLES: Record<string, NodeStyle> = {
  function: {
    icon: CircuitBoard,
    badge:
      "border-sky-500/40 bg-sky-500/10 text-sky-700 dark:text-sky-300",
  },
  llm: {
    icon: Bot,
    badge:
      "border-violet-500/40 bg-violet-500/10 text-violet-700 dark:text-violet-300",
  },
  tool: {
    icon: Wrench,
    badge:
      "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  },
  router: {
    icon: GitBranch,
    badge:
      "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300",
  },
  human: {
    icon: UserIcon,
    badge:
      "border-pink-500/40 bg-pink-500/10 text-pink-700 dark:text-pink-300",
  },
  subgraph: {
    icon: Layers,
    badge:
      "border-cyan-500/40 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300",
  },
}

function getNodeStyle(type: string): NodeStyle {
  return NODE_TYPE_STYLES[type] ?? NODE_TYPE_STYLES.function
}

function formatDuration(ms?: number): string {
  if (ms == null) return "-"
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function computeDuration(exec: NodeExecution): number | undefined {
  if (exec.duration_ms != null) return exec.duration_ms
  if (exec.started_at && exec.completed_at) {
    return (
      new Date(exec.completed_at).getTime() -
      new Date(exec.started_at).getTime()
    )
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
  const allKeys = Array.from(
    new Set([...Object.keys(prev), ...Object.keys(next)]),
  )
  const changes = allKeys.filter(
    (k) => JSON.stringify(prev[k]) !== JSON.stringify(next[k]),
  )

  if (changes.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">No state changes</p>
    )
  }

  return (
    <div className="space-y-1 font-mono text-xs">
      {changes.map((key) => (
        <div key={key} className="flex flex-wrap items-center gap-2">
          <span className="font-semibold">{key}:</span>
          {key in prev && (
            <span className="text-destructive line-through">
              {JSON.stringify(prev[key])}
            </span>
          )}
          {key in next && (
            <span className="text-emerald-600 dark:text-emerald-400">
              {JSON.stringify(next[key])}
            </span>
          )}
        </div>
      ))}
    </div>
  )
}

function StatusBadge({ status }: { status: NodeExecution["status"] }) {
  if (status === "started") {
    return (
      <Badge
        variant="outline"
        className="border-yellow-500/40 bg-yellow-500/10 text-yellow-700 dark:text-yellow-300"
      >
        <span className="mr-1 inline-block size-1.5 rounded-full bg-yellow-500 animate-pulse" />
        running
      </Badge>
    )
  }
  if (status === "failed") {
    return (
      <Badge variant="outline" className="border-destructive/40 bg-destructive/10 text-destructive">
        failed
      </Badge>
    )
  }
  return null
}

function CodeBlock({ value }: { value: unknown }) {
  return (
    <ScrollArea className="max-h-40 rounded-md border bg-muted">
      <pre className="p-3 font-mono text-xs">
        {JSON.stringify(value, null, 2)}
      </pre>
    </ScrollArea>
  )
}

export function RunInspector({ executions, runOutput }: RunInspectorProps) {
  const totalDuration = executions.reduce(
    (sum, e) => sum + (computeDuration(e) ?? 0),
    0,
  )
  const maxDuration = Math.max(
    ...executions.map((e) => computeDuration(e) ?? 0),
    1,
  )

  return (
    <div className="space-y-6">
      {/* Header + summary */}
      <Card className="p-4">
        <CardHeader className="p-0">
          <CardTitle className="flex items-center justify-between text-sm">
            Execution inspector
            <span className="font-mono text-xs text-muted-foreground">
              {executions.length} nodes · {formatDuration(totalDuration)}
            </span>
          </CardTitle>
        </CardHeader>

        {executions.length > 0 && (
          <>
            <Separator className="my-4" />
            <CardContent className="space-y-1.5 p-0">
              {executions.map((exec, i) => {
                const dur = computeDuration(exec) ?? 0
                const pct = maxDuration > 0 ? (dur / maxDuration) * 100 : 0
                const style = getNodeStyle(exec.node_type)
                return (
                  <div
                    key={`${exec.node_id}-${i}`}
                    className="flex items-center gap-2"
                  >
                    <span className="w-24 truncate font-mono text-xs">
                      {exec.node_id}
                    </span>
                    <div className="relative h-5 flex-1 rounded bg-muted">
                      <div
                        className={cn(
                          "h-full rounded border",
                          style.badge,
                        )}
                        style={{ width: `${Math.max(pct, 2)}%` }}
                      />
                    </div>
                    <span className="w-16 text-right font-mono text-xs text-muted-foreground">
                      {formatDuration(dur)}
                    </span>
                  </div>
                )
              })}
            </CardContent>
          </>
        )}
      </Card>

      {/* Per-node detail accordion */}
      {executions.length > 0 && (
        <Card className="p-0">
          <Accordion type="single" collapsible>
            {executions.map((exec, i) => {
              const style = getNodeStyle(exec.node_type)
              const Icon = style.icon
              const prevOutput = i > 0 ? (executions[i - 1].output ?? {}) : {}

              return (
                <AccordionItem
                  key={`detail-${exec.node_id}-${i}`}
                  value={`${exec.node_id}-${i}`}
                  className="border-b last:border-b-0"
                >
                  <AccordionTrigger className="px-4 py-3 hover:no-underline">
                    <div className="flex flex-1 items-center gap-3">
                      <span
                        className={cn(
                          "flex size-6 shrink-0 items-center justify-center rounded border",
                          style.badge,
                        )}
                      >
                        <Icon className="size-3.5" />
                      </span>
                      <span className="text-sm font-medium">
                        {exec.node_id}
                      </span>
                      <Badge variant="outline" className="font-mono text-[10px]">
                        {exec.node_type}
                      </Badge>
                      <div className="ml-auto flex items-center gap-2">
                        <StatusBadge status={exec.status} />
                        {exec.status === "completed" && (
                          <span className="font-mono text-xs text-muted-foreground">
                            {formatDuration(computeDuration(exec))}
                          </span>
                        )}
                      </div>
                    </div>
                  </AccordionTrigger>
                  <AccordionContent className="space-y-4 px-4 pb-4">
                    {exec.error && (
                      <Alert variant="destructive">
                        <AlertTitle>Error</AlertTitle>
                        <AlertDescription>
                          <pre className="mt-1 font-mono text-xs">
                            {exec.error}
                          </pre>
                        </AlertDescription>
                      </Alert>
                    )}

                    {exec.input && (
                      <div className="grid gap-1.5">
                        <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                          Input
                        </span>
                        <CodeBlock value={exec.input} />
                      </div>
                    )}

                    {exec.output && (
                      <div className="grid gap-1.5">
                        <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                          Output
                        </span>
                        <CodeBlock value={exec.output} />
                      </div>
                    )}

                    <div className="grid gap-1.5">
                      <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                        State changes
                      </span>
                      <StateDiff
                        prev={prevOutput as Record<string, unknown>}
                        next={(exec.output ?? {}) as Record<string, unknown>}
                      />
                    </div>
                  </AccordionContent>
                </AccordionItem>
              )
            })}
          </Accordion>
        </Card>
      )}

      {runOutput && (
        <Card className="p-4">
          <CardHeader className="p-0">
            <CardTitle className="text-xs uppercase tracking-wide text-muted-foreground">
              Final output
            </CardTitle>
          </CardHeader>
          <CardContent className="mt-2 p-0">
            <CodeBlock value={runOutput} />
          </CardContent>
        </Card>
      )}
    </div>
  )
}
