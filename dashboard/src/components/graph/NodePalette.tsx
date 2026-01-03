import { Brain, Wrench, GitBranch, User, Play, Square } from "lucide-react"
import { Card } from "@/components/ui/card"
import { cn } from "@/lib/utils"

const nodeItems = [
  {
    type: "start",
    label: "Start",
    icon: Play,
    description: "Entry point",
    color: "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300",
  },
  {
    type: "end",
    label: "End",
    icon: Square,
    description: "Exit point",
    color: "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300",
  },
  {
    type: "llm",
    label: "LLM",
    icon: Brain,
    description: "Call an LLM model",
    color: "bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300",
  },
  {
    type: "tool",
    label: "Tool",
    icon: Wrench,
    description: "Execute a function",
    color: "bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300",
  },
  {
    type: "conditional",
    label: "Conditional",
    icon: GitBranch,
    description: "Branch logic",
    color: "bg-amber-100 text-amber-700 dark:bg-amber-900 dark:text-amber-300",
  },
  {
    type: "human",
    label: "Human",
    icon: User,
    description: "Await human input",
    color: "bg-pink-100 text-pink-700 dark:bg-pink-900 dark:text-pink-300",
  },
]

export function NodePalette() {
  const onDragStart = (
    event: React.DragEvent,
    nodeType: string
  ) => {
    event.dataTransfer.setData("application/reactflow", nodeType)
    event.dataTransfer.effectAllowed = "move"
  }

  return (
    <Card className="w-[180px] p-2">
      <div className="mb-2 px-2 text-xs font-semibold text-muted-foreground uppercase tracking-wide">
        Nodes
      </div>
      <div className="space-y-1">
        {nodeItems.map((item) => (
          <div
            key={item.type}
            className={cn(
              "flex cursor-grab items-center gap-2 rounded-md px-2 py-1.5 transition-colors",
              "hover:bg-muted active:cursor-grabbing"
            )}
            draggable
            onDragStart={(e) => onDragStart(e, item.type)}
          >
            <div
              className={cn(
                "flex h-7 w-7 items-center justify-center rounded-md",
                item.color
              )}
            >
              <item.icon className="h-3.5 w-3.5" />
            </div>
            <div className="flex-1 min-w-0">
              <div className="text-sm font-medium truncate">{item.label}</div>
              <div className="text-[10px] text-muted-foreground truncate">
                {item.description}
              </div>
            </div>
          </div>
        ))}
      </div>
    </Card>
  )
}
