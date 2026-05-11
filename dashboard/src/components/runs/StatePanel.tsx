import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"

// StatePanel — run-level state inspector. Renders the run's terminal
// `state` (= run.output) keyed-by-key with badges marking keys that
// are new vs. changed relative to `previousState` (= run.input).
//
// Distinct from the per-node `StateDiff` rendered inside RunInspector:
// that one shows the delta from the previous node's output to the
// current node's output (one step). This one shows the delta across
// the entire run.

interface StatePanelProps {
  state: Record<string, unknown>
  previousState?: Record<string, unknown>
}

export function StatePanel({ state, previousState }: StatePanelProps) {
  const keys = Object.keys(state)

  return (
    <Card className="overflow-hidden p-0">
      <CardHeader className="border-b py-3 px-4">
        <CardTitle className="text-xs uppercase tracking-wide text-muted-foreground">
          State inspector
        </CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        {keys.length === 0 ? (
          <p className="py-8 text-center text-xs text-muted-foreground">
            Empty state
          </p>
        ) : (
          <ScrollArea className="max-h-[480px]">
            <div className="divide-y">
              {keys.map((key) => {
                const changed =
                  previousState !== undefined &&
                  JSON.stringify(state[key]) !==
                    JSON.stringify(previousState[key])
                const isNew =
                  previousState !== undefined && !(key in previousState)

                return (
                  <div
                    key={key}
                    className={cn(
                      "flex items-start gap-3 px-4 py-2",
                      isNew &&
                        "border-l-2 border-emerald-500/60 bg-emerald-500/5",
                      !isNew &&
                        changed &&
                        "border-l-2 border-amber-500/60 bg-amber-500/5",
                    )}
                  >
                    <span className="min-w-[80px] shrink-0 font-mono text-xs font-semibold">
                      {key}
                    </span>
                    <span className="break-all font-mono text-xs text-muted-foreground">
                      {typeof state[key] === "string"
                        ? `"${state[key]}"`
                        : JSON.stringify(state[key])}
                    </span>
                    {isNew && (
                      <Badge
                        variant="outline"
                        className="ml-auto border-emerald-500/40 text-emerald-700 dark:text-emerald-300 text-[10px]"
                      >
                        new
                      </Badge>
                    )}
                    {!isNew && changed && (
                      <Badge
                        variant="outline"
                        className="ml-auto border-amber-500/40 text-amber-700 dark:text-amber-300 text-[10px]"
                      >
                        changed
                      </Badge>
                    )}
                  </div>
                )
              })}
            </div>
          </ScrollArea>
        )}
      </CardContent>
    </Card>
  )
}
