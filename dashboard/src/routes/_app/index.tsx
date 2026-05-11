import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/api/client"
import type {
  AssistantsResponse,
  ThreadsResponse,
  Run,
  RunStatus,
} from "@/types/entities"
import { PageHeader } from "@/components/layout/PageHeader"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableRow,
} from "@/components/ui/table"
import {
  Activity,
  ArrowUpRight,
  Bot,
  MessageSquare,
  Play,
} from "lucide-react"
import { cn } from "@/lib/utils"

const STATUS_BADGE: Record<RunStatus, string> = {
  queued: "border-muted-foreground/40 text-muted-foreground",
  in_progress:
    "border-yellow-500/40 bg-yellow-500/10 text-yellow-700 dark:text-yellow-300",
  completed:
    "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  failed: "border-destructive/40 bg-destructive/10 text-destructive",
  cancelled: "border-muted-foreground/40 text-muted-foreground",
  requires_action:
    "border-orange-500/40 bg-orange-500/10 text-orange-700 dark:text-orange-300",
}

export const Route = createFileRoute("/_app/")({
  component: Dashboard,
})

function Dashboard() {
  const { data: assistantsData } = useQuery({
    queryKey: ["assistants"],
    queryFn: () => api.get<AssistantsResponse>("/assistants"),
  })

  const { data: threadsData } = useQuery({
    queryKey: ["threads"],
    queryFn: () => api.get<ThreadsResponse>("/threads"),
  })

  const { data: runs } = useQuery({
    queryKey: ["runs"],
    queryFn: () => api.get<Run[]>("/runs"),
  })

  const totalRuns = runs?.length ?? 0
  const activeRuns = runs?.filter(r => r.status === "in_progress" || r.status === "queued").length ?? 0
  const totalAssistants = assistantsData?.total ?? 0
  const totalThreads = threadsData?.total ?? 0

  return (
    <div>
      <PageHeader
        title="Dashboard"
        description="Overview of your workflow executions"
      />

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
        <StatsCard
          to="/runs"
          title="Total runs"
          value={totalRuns.toString()}
          icon={Play}
        />
        <StatsCard
          to="/runs"
          title="Active runs"
          value={activeRuns.toString()}
          icon={Activity}
          trend={activeRuns > 0 ? "Running now" : undefined}
        />
        <StatsCard
          to="/assistants"
          title="Assistants"
          value={totalAssistants.toString()}
          icon={Bot}
        />
        <StatsCard
          to="/threads"
          title="Threads"
          value={totalThreads.toString()}
          icon={MessageSquare}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card className="p-0">
          <CardHeader className="border-b py-4">
            <CardTitle className="flex items-center justify-between text-base">
              Recent runs
              <Button
                asChild
                variant="ghost"
                size="sm"
                className="text-primary hover:text-primary"
              >
                <Link to="/runs">
                  View all
                  <ArrowUpRight className="size-3" />
                </Link>
              </Button>
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <RecentRunsList runs={runs ?? []} />
          </CardContent>
        </Card>

        <Card className="p-0">
          <CardHeader className="border-b py-4">
            <CardTitle className="flex items-center justify-between text-base">
              Assistants
              <Button
                asChild
                variant="ghost"
                size="sm"
                className="text-primary hover:text-primary"
              >
                <Link to="/assistants">
                  View all
                  <ArrowUpRight className="size-3" />
                </Link>
              </Button>
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <AssistantsList
              assistants={assistantsData?.assistants ?? []}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

// RecentRunsList — five most recent runs, newest first, as clickable
// rows in a shadcn <Table>. Each row navigates to /runs/$runId.
function RecentRunsList({ runs }: { runs: Run[] }) {
  const navigate = useNavigate()

  if (runs.length === 0) {
    return (
      <p className="p-6 text-center text-sm text-muted-foreground">
        No recent runs. Create an assistant and start a new run.
      </p>
    )
  }
  const recent = [...runs]
    .sort(
      (a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    )
    .slice(0, 5)
  return (
    <Table>
      <TableBody>
        {recent.map((run) => (
          <TableRow
            key={run.run_id}
            onClick={() =>
              navigate({ to: "/runs/$runId", params: { runId: run.run_id } })
            }
            className="cursor-pointer"
          >
            <TableCell>
              <div className="font-mono text-xs">
                {run.run_id.slice(0, 12)}
              </div>
              <div className="text-xs text-muted-foreground">
                {new Date(run.created_at).toLocaleString()}
              </div>
            </TableCell>
            <TableCell className="text-right">
              <Badge
                variant="outline"
                className={cn(
                  "font-mono text-[10px]",
                  STATUS_BADGE[run.status as RunStatus],
                )}
              >
                {run.status}
              </Badge>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

// AssistantsList — first five assistants in creation order, as
// clickable rows that route into /assistants/$assistantId.
function AssistantsList({
  assistants,
}: {
  assistants: AssistantsResponse["assistants"]
}) {
  const navigate = useNavigate()

  if (assistants.length === 0) {
    return (
      <p className="p-6 text-center text-sm text-muted-foreground">
        No assistants yet. Create your first assistant to get started.
      </p>
    )
  }
  const top = assistants.slice(0, 5)
  return (
    <Table>
      <TableBody>
        {top.map((a) => (
          <TableRow
            key={a.assistant_id}
            onClick={() =>
              navigate({
                to: "/assistants/$assistantId",
                params: { assistantId: a.assistant_id },
              })
            }
            className="cursor-pointer"
          >
            <TableCell>
              <div className="text-sm font-medium">
                {a.name || a.assistant_id.slice(0, 12)}
              </div>
              <div className="font-mono text-xs text-muted-foreground">
                {a.graph_id}
              </div>
            </TableCell>
            <TableCell className="w-8 text-right">
              <ArrowUpRight className="size-4 text-muted-foreground" />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

interface StatsCardProps {
  to: string
  title: string
  value: string
  icon: React.ElementType
  trend?: string
  trendUp?: boolean
}

// StatsCard renders as a Link (via the parent <Link asChild>): the
// entire card surface is the click target, the icon hints at the
// destination, and hover state mirrors the row pattern below. Each
// card navigates to its section so the icon isn't just decoration.
function StatsCard({
  to,
  title,
  value,
  icon: Icon,
  trend,
  trendUp,
}: StatsCardProps) {
  return (
    <Link to={to} className="group">
      <Card className="transition-colors group-hover:bg-accent">
        <CardContent className="pt-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-muted-foreground">{title}</p>
              <p className="text-2xl font-bold font-mono">{value}</p>
              {trend && (
                <p
                  className={cn(
                    "text-sm mt-1",
                    trendUp
                      ? "text-emerald-600 dark:text-emerald-400"
                      : "text-muted-foreground",
                  )}
                >
                  {trend}
                </p>
              )}
            </div>
            <div className="h-12 w-12 rounded-lg bg-primary/10 flex items-center justify-center transition-colors group-hover:bg-primary/20">
              <Icon className="h-6 w-6 text-primary" />
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
