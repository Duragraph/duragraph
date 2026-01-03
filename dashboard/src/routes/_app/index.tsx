import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/api/client"
import type { AssistantsResponse, ThreadsResponse, Run } from "@/types/entities"
import { PageHeader } from "@/components/layout/PageHeader"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Play, Bot, MessageSquare, Activity, ArrowUpRight } from "lucide-react"

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
          title="Total Runs"
          value={totalRuns.toString()}
          icon={Play}
        />
        <StatsCard
          title="Active Runs"
          value={activeRuns.toString()}
          icon={Activity}
          trend={activeRuns > 0 ? "Running now" : undefined}
        />
        <StatsCard
          title="Assistants"
          value={totalAssistants.toString()}
          icon={Bot}
        />
        <StatsCard
          title="Threads"
          value={totalThreads.toString()}
          icon={MessageSquare}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center justify-between text-base">
              Recent Runs
              <Link
                to="/runs"
                className="text-sm text-primary hover:text-primary/80 font-medium inline-flex items-center gap-1"
              >
                View all
                <ArrowUpRight className="h-3 w-3" />
              </Link>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              No recent runs. Create an assistant and start a new run.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center justify-between text-base">
              Assistants
              <Link
                to="/assistants"
                className="text-sm text-primary hover:text-primary/80 font-medium inline-flex items-center gap-1"
              >
                View all
                <ArrowUpRight className="h-3 w-3" />
              </Link>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              No assistants yet. Create your first assistant to get started.
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

interface StatsCardProps {
  title: string
  value: string
  icon: React.ElementType
  trend?: string
  trendUp?: boolean
}

function StatsCard({ title, value, icon: Icon, trend, trendUp }: StatsCardProps) {
  return (
    <Card>
      <CardContent className="pt-6">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm text-muted-foreground">{title}</p>
            <p className="text-2xl font-bold font-mono">
              {value}
            </p>
            {trend && (
              <p
                className={`text-sm mt-1 ${
                  trendUp ? "text-emerald-600 dark:text-emerald-400" : "text-muted-foreground"
                }`}
              >
                {trend}
              </p>
            )}
          </div>
          <div className="h-12 w-12 rounded-lg bg-primary/10 flex items-center justify-center">
            <Icon className="h-6 w-6 text-primary" />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
