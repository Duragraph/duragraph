import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import {
  Clock,
  Loader2,
  CheckCircle,
  XCircle,
  Ban,
  AlertTriangle,
} from "lucide-react"
import type { RunStatus } from "@/types/entities"

const statusConfig: Record<
  RunStatus,
  {
    label: string
    icon: React.ElementType
    className: string
  }
> = {
  queued: {
    label: "Queued",
    icon: Clock,
    className: "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300",
  },
  in_progress: {
    label: "Running",
    icon: Loader2,
    className: "bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300",
  },
  completed: {
    label: "Completed",
    icon: CheckCircle,
    className: "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300",
  },
  failed: {
    label: "Failed",
    icon: XCircle,
    className: "bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300",
  },
  cancelled: {
    label: "Cancelled",
    icon: Ban,
    className: "bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400",
  },
  requires_action: {
    label: "Action Required",
    icon: AlertTriangle,
    className: "bg-amber-100 text-amber-700 dark:bg-amber-900 dark:text-amber-300",
  },
}

interface RunStatusBadgeProps {
  status: RunStatus
  className?: string
}

export function RunStatusBadge({ status, className }: RunStatusBadgeProps) {
  const config = statusConfig[status]
  const Icon = config.icon

  return (
    <Badge
      variant="outline"
      className={cn(
        "inline-flex items-center gap-1.5 px-2.5 py-0.5 border-0",
        config.className,
        className
      )}
    >
      <Icon
        className={cn(
          "h-3.5 w-3.5",
          status === "in_progress" && "animate-spin"
        )}
      />
      {config.label}
    </Badge>
  )
}
