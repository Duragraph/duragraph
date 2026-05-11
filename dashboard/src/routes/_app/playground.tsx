import { useRef, useEffect, useCallback, useState } from "react"
import { createFileRoute } from "@tanstack/react-router"
import { useChatStore } from "@/stores/chat"
import { useAssistants } from "@/api/assistants"
import { useCreateThread, useThreads } from "@/api/threads"
import { streamRun } from "@/lib/sse"
import { ChatMessage } from "@/components/playground/ChatMessage"
import { ChatInput } from "@/components/playground/ChatInput"
import { NodeExecutionPanel } from "@/components/playground/NodeExecutionPanel"
import { ApprovalDialog } from "@/components/playground/ApprovalDialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import type { Message } from "@/types/entities"

// Sentinel value for the "new thread" option. Radix's Select disallows
// an empty-string value because it conflicts with the cleared state;
// we serialise the picker's "new thread" intent as this constant and
// translate at the boundary before calling setThread.
const NEW_THREAD_VALUE = "__new__"
const NO_ASSISTANT_VALUE = "__none__"

// Playground — the chat playground ported from studio. Distinct from
// `/threads/:threadId` (which inspects a persisted thread): playground
// is an ephemeral session — pick an assistant from the sidebar (left),
// send a message, watch the run stream lifecycle events in real time
// via SSE, with node-by-node execution detail in the right panel and
// approval dialogs for human-in-the-loop nodes.
//
// Cancels the active SSE stream on unmount; threads created during a
// session persist on the engine but their thread_id is discarded once
// the user navigates away (matches studio's behaviour).

export const Route = createFileRoute("/_app/playground")({
  component: PlaygroundPage,
})

function PlaygroundPage() {
  const {
    selectedThreadId,
    selectedAssistantId,
    messages,
    streamingContent,
    isStreaming,
    sseStatus,
    nodeExecutions,
    addMessage,
    setThread,
    setAssistant,
    handleEvent,
    setSSEStatus,
  } = useChatStore()

  // Inline assistant + thread pickers (studio originally drove these
  // from its sidebar; the dashboard sidebar is observability-shaped so
  // we host the selectors inside the page).
  //
  // Thread selection: "" = new thread (created lazily on first message
  // via createThread.mutateAsync). An existing thread_id resumes that
  // conversation; the chat-store's setThread() clears local messages
  // so the page re-renders empty — re-fetching prior messages from the
  // server would need a /threads/{id}/messages call which is out of
  // scope for the picker bring-up.
  const { data: assistants } = useAssistants()
  const { data: threadsResp } = useThreads()
  const threads = threadsResp ?? []
  const createThread = useCreateThread()
  const scrollRef = useRef<HTMLDivElement>(null)
  const cleanupRef = useRef<(() => void) | null>(null)
  const [pendingApproval, setPendingApproval] = useState<{
    runId: string
    threadId: string
    prompt?: string
  } | null>(null)

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages, streamingContent])

  useEffect(() => {
    return () => {
      cleanupRef.current?.()
    }
  }, [])

  const sendMessage = useCallback(
    async (content: string) => {
      if (!selectedAssistantId || isStreaming) return

      const userMessage: Message = { role: "user", content }
      addMessage(userMessage)

      let threadId = selectedThreadId
      if (!threadId) {
        try {
          const thread = await createThread.mutateAsync({})
          threadId = thread.thread_id
          setThread(threadId)
        } catch {
          addMessage({
            role: "assistant",
            content: "Error: Failed to create thread",
          })
          return
        }
      }

      const allMessages = [...messages, userMessage]

      // streamRun does BOTH: creates the run on the control plane AND
      // streams its lifecycle events back. Do not also call createRun
      // — that would create a second run on the same thread, which the
      // control plane's default multitask_strategy: "reject" 409s.
      cleanupRef.current?.()
      cleanupRef.current = streamRun(
        threadId,
        selectedAssistantId,
        { messages: allMessages },
        {
          onEvent: (event) => {
            handleEvent(event)
            if (
              event.event === "run_requires_action" ||
              (event.event === "run_started" &&
                event.data.status === "requires_action")
            ) {
              setPendingApproval({
                runId: (event.data.run_id as string) ?? "",
                threadId: threadId!,
                prompt: event.data.prompt as string | undefined,
              })
            }
          },
          onStatus: setSSEStatus,
          onError: (err) => {
            addMessage({
              role: "assistant",
              content: `Error: ${err.message}`,
            })
          },
        },
      )
    },
    [
      selectedAssistantId,
      selectedThreadId,
      isStreaming,
      messages,
      addMessage,
      setThread,
      handleEvent,
      setSSEStatus,
      createThread,
    ],
  )

  return (
    <div className="flex h-full -m-6">
      <div className="flex flex-1 flex-col">
        {/* Assistant + Thread pickers + SSE status bar */}
        <div className="flex items-center justify-between gap-3 border-b bg-card px-4 py-3">
          <div className="flex items-center gap-6">
            <div className="flex items-center gap-2">
              <Label htmlFor="assistant" className="text-xs">
                Assistant
              </Label>
              <Select
                value={selectedAssistantId ?? NO_ASSISTANT_VALUE}
                onValueChange={(v) =>
                  setAssistant(v === NO_ASSISTANT_VALUE ? null : v)
                }
              >
                <SelectTrigger id="assistant" size="sm" className="w-[200px]">
                  <SelectValue placeholder="Select assistant" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_ASSISTANT_VALUE}>
                    — select —
                  </SelectItem>
                  {assistants?.map((a) => (
                    <SelectItem key={a.assistant_id} value={a.assistant_id}>
                      {a.name || a.assistant_id.slice(0, 8)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <Separator orientation="vertical" className="h-6" />

            <div className="flex items-center gap-2">
              <Label htmlFor="thread" className="text-xs">
                Thread
              </Label>
              <Select
                value={selectedThreadId ?? NEW_THREAD_VALUE}
                onValueChange={(v) =>
                  setThread(v === NEW_THREAD_VALUE ? null : v)
                }
              >
                <SelectTrigger id="thread" size="sm" className="w-[240px]">
                  <SelectValue placeholder="New thread" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NEW_THREAD_VALUE}>
                    — new thread —
                  </SelectItem>
                  {threads.map((t) => (
                    <SelectItem key={t.thread_id} value={t.thread_id}>
                      {t.thread_id.slice(0, 8)}
                      {t.updated_at &&
                        ` · ${new Date(t.updated_at).toLocaleString()}`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          {sseStatus !== "closed" && (
            <Badge
              variant="outline"
              className={
                sseStatus === "open"
                  ? "border-green-500/50 text-green-600 dark:text-green-400"
                  : sseStatus === "connecting"
                    ? "border-yellow-500/50 text-yellow-600 dark:text-yellow-400"
                    : "border-destructive/50 text-destructive"
              }
            >
              <span
                className={`mr-1.5 inline-block h-1.5 w-1.5 rounded-full ${
                  sseStatus === "open"
                    ? "bg-green-500"
                    : sseStatus === "connecting"
                      ? "bg-yellow-500 animate-pulse"
                      : "bg-destructive"
                }`}
              />
              {sseStatus}
            </Badge>
          )}
        </div>

        {/* Messages */}
        <div ref={scrollRef} className="flex-1 overflow-y-auto p-6">
          <div className="mx-auto max-w-3xl space-y-4">
            {messages.length === 0 && !isStreaming && (
              <Card className="mx-auto mt-20 max-w-md border-dashed bg-transparent shadow-none">
                <CardHeader className="text-center">
                  <CardTitle className="text-lg">
                    DuraGraph Playground
                  </CardTitle>
                  <CardDescription>
                    {selectedAssistantId
                      ? "Send a message to start a conversation."
                      : "Pick an assistant above to begin."}
                  </CardDescription>
                </CardHeader>
              </Card>
            )}

            {messages.map((msg, i) => (
              <ChatMessage key={i} message={msg} />
            ))}

            {streamingContent && (
              <ChatMessage
                message={{ role: "assistant", content: streamingContent }}
                isStreaming
              />
            )}

            {pendingApproval && (
              <ApprovalDialog
                runId={pendingApproval.runId}
                threadId={pendingApproval.threadId}
                prompt={pendingApproval.prompt}
                onResolved={() => setPendingApproval(null)}
              />
            )}
          </div>
        </div>

        {/* Input */}
        <ChatInput
          onSend={sendMessage}
          disabled={!selectedAssistantId || isStreaming}
          isStreaming={isStreaming}
        />
      </div>

      {/* Node execution panel — shows during/after runs */}
      {nodeExecutions.length > 0 && (
        <NodeExecutionPanel executions={nodeExecutions} />
      )}
    </div>
  )
}
