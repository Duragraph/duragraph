import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/api/client"
import type { Assistant, Run, Graph } from "@/types/entities"
import { PageHeader } from "@/components/layout/PageHeader"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  ArrowLeft,
  Edit,
  GitBranch,
  Play,
  Settings,
  Trash2,
  AlertCircle,
} from "lucide-react"
import { RunStatusBadge } from "@/components/runs/RunStatusBadge"
import { GraphVisualizer } from "@/components/graph/GraphVisualizer"

export const Route = createFileRoute("/_app/assistants/$assistantId")({
  component: AssistantDetailPage,
})

function AssistantDetailPage() {
  const { assistantId } = Route.useParams()

  const {
    data: assistant,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["assistant", assistantId],
    queryFn: () => api.get<Assistant>(`/assistants/${assistantId}`),
  })

  // Fetch runs for this assistant
  const { data: runs } = useQuery({
    queryKey: ["runs", { assistant_id: assistantId }],
    queryFn: () => api.get<Run[]>(`/runs?assistant_id=${assistantId}`),
    enabled: !!assistant,
  })

  // Fetch graph for this assistant
  const { data: graph, isLoading: isGraphLoading } = useQuery({
    queryKey: ["assistant-graph", assistantId],
    queryFn: () => api.get<Graph>(`/assistants/${assistantId}/graph`),
    enabled: !!assistant?.graph_id,
  })

  if (isLoading) {
    return <AssistantDetailSkeleton />
  }

  if (error || !assistant) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <AlertCircle className="h-12 w-12 text-red-500 mb-4" />
        <h2 className="text-xl font-semibold mb-2">Failed to load assistant</h2>
        <p className="text-muted-foreground">{error?.message || "Assistant not found"}</p>
        <Link to="/assistants" className="mt-4">
          <Button variant="outline">Back to Assistants</Button>
        </Link>
      </div>
    )
  }

  const recentRuns = runs?.slice(0, 5) || []
  const totalRuns = runs?.length || 0
  const successfulRuns = runs?.filter((r) => r.status === "success").length || 0
  const successRate = totalRuns > 0 ? ((successfulRuns / totalRuns) * 100).toFixed(1) : "0"

  return (
    <div>
      <div className="mb-4">
        <Link
          to="/assistants"
          className="text-sm text-muted-foreground hover:text-foreground inline-flex items-center gap-1"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Assistants
        </Link>
      </div>

      <PageHeader
        title={
          <div className="flex items-center gap-3">
            <span>{assistant.name}</span>
            <Badge variant="outline">v{assistant.version}</Badge>
          </div>
        }
        description={assistant.description || "No description"}
        actions={
          <>
            <Button variant="outline" size="sm">
              <Play className="h-4 w-4 mr-2" />
              Test Run
            </Button>
            <Button variant="outline" size="sm">
              <Edit className="h-4 w-4 mr-2" />
              Edit
            </Button>
            <Button variant="outline" size="sm" className="text-destructive">
              <Trash2 className="h-4 w-4 mr-2" />
              Delete
            </Button>
          </>
        }
      />

      {/* Stats */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Total Runs
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {totalRuns.toLocaleString()}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Success Rate
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">
              {successRate}%
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Graph ID
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-lg font-mono truncate">{assistant.graph_id || "N/A"}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Model
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-lg font-mono truncate">{assistant.model || "N/A"}</div>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="graph">Graph</TabsTrigger>
          <TabsTrigger value="config">Configuration</TabsTrigger>
          <TabsTrigger value="versions">Versions</TabsTrigger>
          <TabsTrigger value="runs">Recent Runs</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="mt-4">
          <div className="grid grid-cols-2 gap-4">
            <Card>
              <CardHeader>
                <CardTitle>Details</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">ID</span>
                  <span className="font-mono text-sm">{assistant.assistant_id}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Graph</span>
                  <span>{assistant.graph_id || "N/A"}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Model</span>
                  <span>{assistant.model || "N/A"}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Version</span>
                  <span>v{assistant.version}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Created</span>
                  <span>{new Date(assistant.created_at * 1000).toLocaleDateString()}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Updated</span>
                  <span>{new Date(assistant.updated_at * 1000).toLocaleDateString()}</span>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Recent Runs</CardTitle>
              </CardHeader>
              <CardContent>
                {recentRuns.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No runs yet</p>
                ) : (
                  <div className="space-y-3">
                    {recentRuns.map((run) => (
                      <Link
                        key={run.run_id}
                        to="/runs/$runId"
                        params={{ runId: run.run_id }}
                        className="flex items-center justify-between hover:bg-muted/50 p-2 rounded -mx-2"
                      >
                        <div>
                          <div className="font-mono text-sm">{run.run_id.slice(0, 12)}...</div>
                          <div className="text-xs text-muted-foreground">
                            {new Date(run.created_at).toLocaleString()}
                          </div>
                        </div>
                        <RunStatusBadge status={run.status} />
                      </Link>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="graph" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <GitBranch className="h-5 w-5" />
                Graph: {assistant.graph_id || "N/A"}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {isGraphLoading ? (
                <div className="h-[500px] flex items-center justify-center">
                  <Skeleton className="h-full w-full" />
                </div>
              ) : graph ? (
                <div className="h-[500px] border rounded-lg overflow-hidden">
                  <GraphVisualizer graph={graph} />
                </div>
              ) : (
                <div className="h-[400px] flex items-center justify-center bg-muted/20 rounded-lg border-2 border-dashed">
                  <div className="text-center text-muted-foreground">
                    <GitBranch className="w-12 h-12 mx-auto mb-4 opacity-50" />
                    <div className="text-lg font-medium mb-2">No Graph Available</div>
                    <div className="text-sm">
                      {assistant.graph_id
                        ? "Graph data could not be loaded"
                        : "This assistant has no associated graph"}
                    </div>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="config" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Settings className="h-5 w-5" />
                  Configuration
                </div>
                <Button variant="outline" size="sm">
                  <Edit className="h-4 w-4 mr-2" />
                  Edit
                </Button>
              </CardTitle>
            </CardHeader>
            <CardContent>
              <pre className="bg-muted p-4 rounded-lg text-sm overflow-auto max-h-96">
                {JSON.stringify(assistant.config || {}, null, 2)}
              </pre>
              {assistant.metadata && Object.keys(assistant.metadata).length > 0 && (
                <>
                  <h4 className="font-medium mt-6 mb-2">Metadata</h4>
                  <pre className="bg-muted p-4 rounded-lg text-sm overflow-auto max-h-96">
                    {JSON.stringify(assistant.metadata, null, 2)}
                  </pre>
                </>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="versions" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Version History</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Version</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow>
                    <TableCell>
                      <Badge variant="default">v{assistant.version}</Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {new Date(assistant.created_at * 1000).toLocaleDateString()}
                    </TableCell>
                    <TableCell>
                      <span className="text-xs text-muted-foreground">Current</span>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
              <p className="text-sm text-muted-foreground mt-4">
                Version history will be available when assistant versioning is enabled.
              </p>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="runs" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>All Runs</CardTitle>
            </CardHeader>
            <CardContent>
              {runs && runs.length > 0 ? (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Run ID</TableHead>
                      <TableHead>Thread</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Created</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {runs.map((run) => (
                      <TableRow key={run.run_id}>
                        <TableCell>
                          <Link
                            to="/runs/$runId"
                            params={{ runId: run.run_id }}
                            className="font-mono text-sm hover:underline"
                          >
                            {run.run_id.slice(0, 12)}...
                          </Link>
                        </TableCell>
                        <TableCell className="font-mono text-sm">
                          {run.thread_id.slice(0, 12)}...
                        </TableCell>
                        <TableCell>
                          <RunStatusBadge status={run.status} />
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {new Date(run.created_at).toLocaleString()}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              ) : (
                <p className="text-sm text-muted-foreground">No runs yet for this assistant.</p>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function AssistantDetailSkeleton() {
  return (
    <div>
      <Skeleton className="h-4 w-32 mb-4" />
      <Skeleton className="h-8 w-64 mb-2" />
      <Skeleton className="h-4 w-96 mb-6" />
      <div className="grid grid-cols-4 gap-4 mb-6">
        {[...Array(4)].map((_, i) => (
          <Card key={i}>
            <CardHeader className="pb-2">
              <Skeleton className="h-4 w-20" />
            </CardHeader>
            <CardContent>
              <Skeleton className="h-8 w-16" />
            </CardContent>
          </Card>
        ))}
      </div>
      <Skeleton className="h-10 w-80 mb-4" />
      <Skeleton className="h-64 w-full" />
    </div>
  )
}
