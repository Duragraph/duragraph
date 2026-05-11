import {
  Activity,
  Bot,
  CircuitBoard,
  GitBranch,
  Wrench,
  User as UserIcon,
  type LucideIcon,
} from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import type { NodeExecution } from "@/types/entities"

// Maps the engine's node_type string to a lucide icon. Falls back to
// `Activity` for anything unrecognised so the panel never shows a
// blank tile when a new node type lands before the icon map updates.
const NODE_ICON: Record<string, LucideIcon> = {
  function: CircuitBoard,
  llm: Bot,
  tool: Wrench,
  router: GitBranch,
  human: UserIcon,
}

const STATUS_STYLES: Record<NodeExecution["status"], string> = {
  started:
    "border-yellow-500/40 bg-yellow-500/10 text-yellow-700 dark:text-yellow-300",
  completed:
    "border-green-500/40 bg-green-500/10 text-green-700 dark:text-green-300",
  failed: "border-destructive/40 bg-destructive/10 text-destructive",
}

interface NodeExecutionPanelProps {
  executions: NodeExecution[]
}

export function NodeExecutionPanel({ executions }: NodeExecutionPanelProps) {
  return (
    <Card className="w-72 rounded-none border-l-0 border-y-0 border-r-0 p-0 gap-0 shadow-none">
      <CardHeader className="border-b py-3 px-4">
        <CardTitle className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          <Activity className="size-3.5" />
          Execution
        </CardTitle>
      </CardHeader>
      <ScrollArea className="flex-1">
        <CardContent className="space-y-2 p-3">
          {executions.map((exec, i) => {
            const Icon = NODE_ICON[exec.node_type] ?? Activity
            return (
              <Card
                key={`${exec.node_id}-${i}`}
                className="gap-1.5 p-2.5 shadow-none"
              >
                <div className="flex items-center gap-2">
                  <span className="flex size-6 shrink-0 items-center justify-center rounded bg-muted">
                    <Icon className="size-3.5" />
                  </span>
                  <span className="flex-1 truncate font-mono text-xs">
                    {exec.node_id}
                  </span>
                  <Badge
                    variant="outline"
                    className={cn("text-[10px]", STATUS_STYLES[exec.status])}
                  >
                    <span
                      className={cn(
                        "mr-1 inline-block size-1.5 rounded-full",
                        exec.status === "started" &&
                          "bg-yellow-500 animate-pulse",
                        exec.status === "completed" && "bg-green-500",
                        exec.status === "failed" && "bg-destructive",
                      )}
                    />
                    {exec.status}
                  </Badge>
                </div>
                {exec.error && (
                  <p className="line-clamp-2 text-[10px] text-destructive">
                    {exec.error}
                  </p>
                )}
              </Card>
            )
          })}

          {executions.length === 0 && (
            <p className="px-2 py-6 text-center text-xs text-muted-foreground">
              No nodes executed yet
            </p>
          )}
        </CardContent>
      </ScrollArea>
    </Card>
  )
}
