import {
  Bot,
  CircuitBoard,
  GitBranch,
  Layers,
  User,
  Wrench,
  type LucideIcon,
} from "lucide-react"
import { Card } from "@/components/ui/card"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import type { EditorNodeType } from "@/types/entities"

// Node palette — drag a tile onto the canvas to spawn a node of that
// type. Per-type accent colour is encoded in `accent` (border + soft
// bg + text) so the tile colours align with the canvas's node accents
// once the canvas adopts the same palette in a follow-up pass.

interface NodeKind {
  type: EditorNodeType
  label: string
  description: string
  icon: LucideIcon
  accent: string
}

const NODE_TYPES: NodeKind[] = [
  {
    type: "llm",
    label: "LLM",
    description: "Language model call",
    icon: Bot,
    accent:
      "border-violet-400/60 bg-violet-50 text-violet-700 dark:border-violet-400/40 dark:bg-violet-950/30 dark:text-violet-300",
  },
  {
    type: "function",
    label: "Function",
    description: "Custom logic",
    icon: CircuitBoard,
    accent:
      "border-sky-400/60 bg-sky-50 text-sky-700 dark:border-sky-400/40 dark:bg-sky-950/30 dark:text-sky-300",
  },
  {
    type: "tool",
    label: "Tool",
    description: "External tool call",
    icon: Wrench,
    accent:
      "border-emerald-400/60 bg-emerald-50 text-emerald-700 dark:border-emerald-400/40 dark:bg-emerald-950/30 dark:text-emerald-300",
  },
  {
    type: "router",
    label: "Router",
    description: "Conditional branch",
    icon: GitBranch,
    accent:
      "border-amber-400/60 bg-amber-50 text-amber-700 dark:border-amber-400/40 dark:bg-amber-950/30 dark:text-amber-300",
  },
  {
    type: "human",
    label: "Human",
    description: "Human-in-the-loop",
    icon: User,
    accent:
      "border-pink-400/60 bg-pink-50 text-pink-700 dark:border-pink-400/40 dark:bg-pink-950/30 dark:text-pink-300",
  },
  {
    type: "subgraph",
    label: "Subgraph",
    description: "Nested graph",
    icon: Layers,
    accent:
      "border-cyan-400/60 bg-cyan-50 text-cyan-700 dark:border-cyan-400/40 dark:bg-cyan-950/30 dark:text-cyan-300",
  },
]

export function NodePalette() {
  function handleDragStart(e: React.DragEvent, type: EditorNodeType) {
    e.dataTransfer.setData("node-type", type)
    e.dataTransfer.effectAllowed = "copy"
  }

  return (
    <aside className="flex w-56 shrink-0 flex-col border-r bg-card">
      <div className="border-b p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Node types
        </h2>
        <p className="mt-0.5 text-xs text-muted-foreground/70">
          Drag onto canvas
        </p>
      </div>

      <ScrollArea className="flex-1">
        <div className="space-y-2 p-3">
          {NODE_TYPES.map(({ type, label, description, icon: Icon, accent }) => (
            <Card
              key={type}
              draggable
              onDragStart={(e) => handleDragStart(e, type)}
              className={cn(
                "cursor-grab gap-1 border-2 p-2.5 shadow-none transition-shadow active:cursor-grabbing hover:shadow-sm",
                accent,
              )}
            >
              <div className="flex items-center gap-2">
                <Icon className="size-3.5" />
                <span className="font-mono text-xs font-semibold">{label}</span>
              </div>
              <p className="text-[10px] opacity-70">{description}</p>
            </Card>
          ))}
        </div>
      </ScrollArea>
    </aside>
  )
}
