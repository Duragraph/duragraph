import { useState, useRef, useEffect, useCallback } from "react"
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/api/client"
import type { Thread, Run, Message } from "@/types/entities"
import { PageHeader } from "@/components/layout/PageHeader"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Skeleton } from "@/components/ui/skeleton"
import { ScrollArea } from "@/components/ui/scroll-area"
import { DeleteConfirmationDialog } from "@/components/ui/delete-confirmation-dialog"
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
  MessageSquare,
  Play,
  Trash2,
  AlertCircle,
  Settings,
  Radio,
} from "lucide-react"
import { RunStatusBadge } from "@/components/runs/RunStatusBadge"
import { NewRunDialog } from "@/components/runs/NewRunDialog"
import { ChatInput, ChatMessage } from "@/components/chat"
import { useThreadStream } from "@/hooks/useThreadStream"
import { toast } from "sonner"

export const Route = createFileRoute("/_app/threads/$threadId")({
  component: ThreadDetailPage,
})

function ThreadDetailPage() {
  const { threadId } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const scrollRef = useRef<HTMLDivElement>(null)
  const [streamingMessage, setStreamingMessage] = useState<Partial<Message> | null>(null)
  const [showNewRunDialog, setShowNewRunDialog] = useState(false)
  const [showDeleteDialog, setShowDeleteDialog] = useState(false)

  // Delete thread mutation
  const deleteMutation = useMutation({
    mutationFn: () => api.delete(`/threads/${threadId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["threads"] })
      toast.success("Thread deleted successfully")
      navigate({ to: "/threads" })
    },
    onError: (error: Error) => {
      toast.error(`Failed to delete thread: ${error.message}`)
    },
  })

  const {
    data: thread,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["thread", threadId],
    queryFn: () => api.get<Thread>(`/threads/${threadId}`),
  })

  // Fetch runs for this thread
  const { data: runs } = useQuery({
    queryKey: ["runs", { thread_id: threadId }],
    queryFn: () => api.get<Run[]>(`/threads/${threadId}/runs`),
    enabled: !!thread,
  })

  // Check if there are active runs
  const hasActiveRuns = runs?.some(
    (r) => r.status === "in_progress" || r.status === "queued"
  )

  // Handle incoming messages from stream
  const handleMessage = useCallback((message: { role: string; content: string }) => {
    setStreamingMessage({
      id: "streaming",
      role: message.role as Message["role"],
      content: message.content,
      created_at: Date.now() / 1000,
    })
  }, [])

  // Handle run updates from stream
  const handleRunUpdate = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["runs", { thread_id: threadId }] })
  }, [queryClient, threadId])

  // Connect to thread stream when there are active runs
  const { isConnected } = useThreadStream({
    threadId,
    enabled: hasActiveRuns,
    onMessage: handleMessage,
    onRunUpdate: handleRunUpdate,
    onEvent: (event) => {
      // Clear streaming message when complete
      if (event.event === "message.completed" || event.event === "run.completed") {
        setStreamingMessage(null)
        queryClient.invalidateQueries({ queryKey: ["thread", threadId] })
      }
    },
  })

  // Add message mutation
  const addMessageMutation = useMutation({
    mutationFn: (content: string) =>
      api.post(`/threads/${threadId}/messages`, {
        role: "user",
        content,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["thread", threadId] })
    },
  })

  // Auto-scroll to bottom when messages change
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [thread?.messages, streamingMessage])

  const handleSendMessage = (content: string) => {
    addMessageMutation.mutate(content)
  }

  if (isLoading) {
    return <ThreadDetailSkeleton />
  }

  if (error || !thread) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <AlertCircle className="h-12 w-12 text-red-500 mb-4" />
        <h2 className="text-xl font-semibold mb-2">Failed to load thread</h2>
        <p className="text-muted-foreground">{error?.message || "Thread not found"}</p>
        <Link to="/threads" className="mt-4">
          <Button variant="outline">Back to Threads</Button>
        </Link>
      </div>
    )
  }

  const totalRuns = runs?.length || 0
  const activeRuns = runs?.filter((r) => r.status === "in_progress" || r.status === "queued").length || 0

  return (
    <div>
      <div className="mb-4">
        <Link
          to="/threads"
          className="text-sm text-muted-foreground hover:text-foreground inline-flex items-center gap-1"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Threads
        </Link>
      </div>

      <PageHeader
        title={
          <div className="flex items-center gap-3">
            <MessageSquare className="h-6 w-6" />
            <span className="font-mono">{threadId.slice(0, 16)}...</span>
            {isConnected && (
              <Badge variant="outline" className="text-green-600 border-green-600 gap-1">
                <Radio className="h-3 w-3 animate-pulse" />
                Live
              </Badge>
            )}
          </div>
        }
        description={`Created ${new Date(thread.created_at * 1000).toLocaleString()}`}
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => setShowNewRunDialog(true)}>
              <Play className="h-4 w-4 mr-2" />
              New Run
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="text-destructive"
              onClick={() => setShowDeleteDialog(true)}
            >
              <Trash2 className="h-4 w-4 mr-2" />
              Delete
            </Button>
          </>
        }
      />

      <NewRunDialog
        open={showNewRunDialog}
        onOpenChange={setShowNewRunDialog}
        defaultThreadId={threadId}
      />

      <DeleteConfirmationDialog
        open={showDeleteDialog}
        onOpenChange={setShowDeleteDialog}
        title="Delete Thread"
        description="Are you sure you want to delete this thread? This will also delete all messages and runs associated with it. This action cannot be undone."
        onConfirm={async () => { await deleteMutation.mutateAsync() }}
        isPending={deleteMutation.isPending}
      />

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Messages
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{thread.messages?.length || 0}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Total Runs
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{totalRuns}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Active Runs
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-blue-600">{activeRuns}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Last Updated
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-lg">
              {new Date(thread.updated_at * 1000).toLocaleDateString()}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="messages">
        <TabsList>
          <TabsTrigger value="messages">Messages</TabsTrigger>
          <TabsTrigger value="runs">Runs</TabsTrigger>
          <TabsTrigger value="state">State</TabsTrigger>
          <TabsTrigger value="metadata">Metadata</TabsTrigger>
        </TabsList>

        <TabsContent value="messages" className="mt-4">
          <Card className="flex flex-col h-[600px]">
            <CardHeader className="flex-shrink-0 border-b">
              <CardTitle className="flex items-center justify-between">
                <span>Conversation</span>
                {hasActiveRuns && (
                  <Badge variant="secondary" className="font-normal">
                    Run in progress...
                  </Badge>
                )}
              </CardTitle>
            </CardHeader>
            <CardContent className="flex-1 p-0 flex flex-col min-h-0">
              {thread.messages && thread.messages.length > 0 ? (
                <ScrollArea className="flex-1 px-4" ref={scrollRef}>
                  <div className="py-4">
                    {thread.messages.map((message) => (
                      <ChatMessage key={message.id} message={message} />
                    ))}
                    {streamingMessage && (
                      <ChatMessage
                        message={streamingMessage as Message}
                        isStreaming
                      />
                    )}
                  </div>
                </ScrollArea>
              ) : (
                <div className="flex-1 flex flex-col items-center justify-center py-12 text-center">
                  <MessageSquare className="h-12 w-12 text-muted-foreground/30 mb-4" />
                  <h3 className="text-lg font-medium mb-2">No messages yet</h3>
                  <p className="text-sm text-muted-foreground">
                    Send a message to start the conversation.
                  </p>
                </div>
              )}
              <ChatInput
                onSend={handleSendMessage}
                isLoading={addMessageMutation.isPending}
                disabled={hasActiveRuns}
                placeholder={
                  hasActiveRuns
                    ? "Wait for the current run to complete..."
                    : "Type a message..."
                }
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="runs" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Run History</CardTitle>
            </CardHeader>
            <CardContent>
              {runs && runs.length > 0 ? (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Run ID</TableHead>
                      <TableHead>Assistant</TableHead>
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
                          {run.assistant_id.slice(0, 12)}...
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
                <div className="flex flex-col items-center justify-center py-12 text-center">
                  <Play className="h-12 w-12 text-muted-foreground/30 mb-4" />
                  <h3 className="text-lg font-medium mb-2">No runs yet</h3>
                  <p className="text-sm text-muted-foreground">
                    Start a run to execute a workflow on this thread.
                  </p>
                  <Button className="mt-4" onClick={() => setShowNewRunDialog(true)}>
                    <Play className="h-4 w-4 mr-2" />
                    Start New Run
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="state" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Settings className="h-5 w-5" />
                Thread State
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <Settings className="h-12 w-12 text-muted-foreground/30 mb-4" />
                <h3 className="text-lg font-medium mb-2">State Management</h3>
                <p className="text-sm text-muted-foreground">
                  Thread state and checkpoints will be available here.
                  <br />
                  <span className="text-xs">(Coming soon)</span>
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="metadata" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Metadata</CardTitle>
            </CardHeader>
            <CardContent>
              {thread.metadata && Object.keys(thread.metadata).length > 0 ? (
                <pre className="bg-muted p-4 rounded-lg text-sm overflow-auto max-h-96">
                  {JSON.stringify(thread.metadata, null, 2)}
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

function ThreadDetailSkeleton() {
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
