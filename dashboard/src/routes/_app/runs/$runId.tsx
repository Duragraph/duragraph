import { useState, useCallback } from "react"
import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/api/client"
import type { Run, Assistant, Graph } from "@/types/entities"
import { PageHeader } from "@/components/layout/PageHeader"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Skeleton } from "@/components/ui/skeleton"
import { RunStatusBadge } from "@/components/runs/RunStatusBadge"
import { GraphVisualizer, type ExecutionStatus } from "@/components/graph/GraphVisualizer"
import { useRunStream } from "@/hooks/useRunStream"
import {
  ArrowLeft,
  Clock,
  Play,
  GitBranch,
  AlertCircle,
  MessageSquare,
  Bot,
  Radio,
} from "lucide-react"

export const Route = createFileRoute("/_app/runs/$runId")({
  component: RunDetailPage,
})

function RunDetailPage() {
  const { runId } = Route.useParams()
  const [nodeStatuses, setNodeStatuses] = useState<Record<string, ExecutionStatus>>({})

  const {
    data: run,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["run", runId],
    queryFn: () => api.get<Run>(`/runs/${runId}`),
    // Refetch more frequently when run is in progress
    refetchInterval: (query) => {
      const data = query.state.data
      if (data?.status === "in_progress" || data?.status === "queued") {
        return 2000
      }
      return false
    },
  })

  // Handle node status updates from stream
  const handleNodeUpdate = useCallback(
    (nodeId: string, status: "started" | "completed") => {
      setNodeStatuses((prev) => ({
        ...prev,
        [nodeId]: status === "started" ? "running" : "completed",
      }))
    },
    []
  )

  // Connect to SSE stream when run is in progress
  const { isConnected } = useRunStream({
    runId,
    enabled: run?.status === "in_progress" || run?.status === "queued",
    onNodeUpdate: handleNodeUpdate,
  })

  // Fetch assistant info
  const { data: assistant } = useQuery({
    queryKey: ["assistant", run?.assistant_id],
    queryFn: () => api.get<Assistant>(`/assistants/${run?.assistant_id}`),
    enabled: !!run?.assistant_id,
  })

  // Fetch graph for visualization
  const { data: graph, isLoading: isGraphLoading } = useQuery({
    queryKey: ["assistant-graph", run?.assistant_id],
    queryFn: () => api.get<Graph>(`/assistants/${run?.assistant_id}/graph`),
    enabled: !!assistant?.graph_id,
  })

  if (isLoading) {
    return <RunDetailSkeleton />
  }

  if (error || !run) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <AlertCircle className="h-12 w-12 text-red-500 mb-4" />
        <h2 className="text-xl font-semibold mb-2">Failed to load run</h2>
        <p className="text-muted-foreground">{error?.message || "Run not found"}</p>
        <Link to="/runs" className="mt-4">
          <Button variant="outline">Back to Runs</Button>
        </Link>
      </div>
    )
  }

  // Calculate duration if we have start and completion times
  const duration =
    run.started_at && run.completed_at
      ? `${((new Date(run.completed_at).getTime() - new Date(run.started_at).getTime()) / 1000).toFixed(2)}s`
      : run.started_at
        ? "Running..."
        : "Pending"

  return (
    <div>
      <div className="mb-4">
        <Link
          to="/runs"
          className="text-sm text-muted-foreground hover:text-foreground inline-flex items-center gap-1"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Runs
        </Link>
      </div>

      <PageHeader
        title={
          <div className="flex items-center gap-3">
            <span className="font-mono">{runId.slice(0, 16)}...</span>
            <RunStatusBadge status={run.status} />
            {isConnected && (
              <Badge variant="outline" className="text-green-600 border-green-600 gap-1">
                <Radio className="h-3 w-3 animate-pulse" />
                Live
              </Badge>
            )}
          </div>
        }
        description={`Assistant: ${assistant?.name || run.assistant_id.slice(0, 12)} · Duration: ${duration}`}
        actions={
          <Button variant="outline" size="sm">
            <Play className="h-4 w-4 mr-2" />
            Replay
          </Button>
        }
      />

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <Clock className="h-4 w-4" />
              Duration
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{duration}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <Bot className="h-4 w-4" />
              Assistant
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Link
              to="/assistants/$assistantId"
              params={{ assistantId: run.assistant_id }}
              className="text-lg font-medium hover:underline truncate block"
            >
              {assistant?.name || run.assistant_id.slice(0, 12)}
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <MessageSquare className="h-4 w-4" />
              Thread
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Link
              to="/threads/$threadId"
              params={{ threadId: run.thread_id }}
              className="text-sm font-mono hover:underline truncate block"
            >
              {run.thread_id.slice(0, 12)}...
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <GitBranch className="h-4 w-4" />
              Created
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-sm">{new Date(run.created_at).toLocaleString()}</div>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="details">
        <TabsList>
          <TabsTrigger value="details">Details</TabsTrigger>
          <TabsTrigger value="input-output">Input/Output</TabsTrigger>
          <TabsTrigger value="graph">Graph</TabsTrigger>
          <TabsTrigger value="metadata">Metadata</TabsTrigger>
        </TabsList>

        <TabsContent value="details" className="mt-4">
          <div className="grid grid-cols-2 gap-4">
            <Card>
              <CardHeader>
                <CardTitle>Run Details</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Run ID</span>
                  <span className="font-mono text-sm">{run.run_id}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Status</span>
                  <RunStatusBadge status={run.status} />
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Created</span>
                  <span>{new Date(run.created_at).toLocaleString()}</span>
                </div>
                {run.started_at && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Started</span>
                    <span>{new Date(run.started_at).toLocaleString()}</span>
                  </div>
                )}
                {run.completed_at && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Completed</span>
                    <span>{new Date(run.completed_at).toLocaleString()}</span>
                  </div>
                )}
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Duration</span>
                  <span>{duration}</span>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Linked Resources</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex justify-between items-center">
                  <span className="text-muted-foreground">Assistant</span>
                  <Link
                    to="/assistants/$assistantId"
                    params={{ assistantId: run.assistant_id }}
                    className="hover:underline"
                  >
                    {assistant?.name || run.assistant_id.slice(0, 12)}...
                  </Link>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-muted-foreground">Thread</span>
                  <Link
                    to="/threads/$threadId"
                    params={{ threadId: run.thread_id }}
                    className="font-mono text-sm hover:underline"
                  >
                    {run.thread_id.slice(0, 12)}...
                  </Link>
                </div>
                {run.error && (
                  <div className="mt-4 p-3 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800">
                    <h4 className="text-sm font-medium text-red-800 dark:text-red-200 mb-1">
                      Error
                    </h4>
                    <p className="text-sm text-red-700 dark:text-red-300">{run.error}</p>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="input-output" className="mt-4">
          <div className="grid grid-cols-2 gap-4">
            <Card>
              <CardHeader>
                <CardTitle>Input</CardTitle>
              </CardHeader>
              <CardContent>
                <pre className="bg-muted p-4 rounded-lg text-sm overflow-auto max-h-96">
                  {JSON.stringify(run.input || {}, null, 2)}
                </pre>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Output</CardTitle>
              </CardHeader>
              <CardContent>
                {run.output ? (
                  <pre className="bg-muted p-4 rounded-lg text-sm overflow-auto max-h-96">
                    {JSON.stringify(run.output, null, 2)}
                  </pre>
                ) : (
                  <p className="text-sm text-muted-foreground">No output yet</p>
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
                Execution Graph
              </CardTitle>
            </CardHeader>
            <CardContent>
              {isGraphLoading ? (
                <div className="h-[500px] flex items-center justify-center">
                  <Skeleton className="h-full w-full" />
                </div>
              ) : graph ? (
                <div className="h-[500px] border rounded-lg overflow-hidden">
                  <GraphVisualizer
                    graph={graph}
                    nodeStatuses={
                      // Use streaming node statuses if available, otherwise fall back to static status
                      Object.keys(nodeStatuses).length > 0
                        ? nodeStatuses
                        : run.status === "success"
                          ? Object.fromEntries(graph.nodes.map((n) => [n.id, "completed" as ExecutionStatus]))
                          : run.status === "error"
                            ? Object.fromEntries(graph.nodes.map((n) => [n.id, "error" as ExecutionStatus]))
                            : undefined
                    }
                  />
                </div>
              ) : (
                <div className="h-[400px] flex items-center justify-center bg-muted/20 rounded-lg border-2 border-dashed">
                  <div className="text-center text-muted-foreground">
                    <GitBranch className="w-12 h-12 mx-auto mb-4 opacity-50" />
                    <div className="text-lg font-medium mb-2">No Graph Available</div>
                    <div className="text-sm">
                      Graph data could not be loaded for this run
                    </div>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="metadata" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Metadata</CardTitle>
            </CardHeader>
            <CardContent>
              {run.metadata && Object.keys(run.metadata).length > 0 ? (
                <pre className="bg-muted p-4 rounded-lg text-sm overflow-auto max-h-96">
                  {JSON.stringify(run.metadata, null, 2)}
                </pre>
              ) : (
                <p className="text-sm text-muted-foreground">No metadata available.</p>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function RunDetailSkeleton() {
  return (
    <div>
      <Skeleton className="h-4 w-32 mb-4" />
      <Skeleton className="h-8 w-64 mb-2" />
      <Skeleton className="h-4 w-48 mb-6" />
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
