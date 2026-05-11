import { useRef, useEffect, useCallback, useState } from "react"
import { createFileRoute } from "@tanstack/react-router"
import { useChatStore } from "@/stores/chat"
import { useAssistants } from "@/api/assistants"
import { useCreateThread } from "@/api/threads"
import { streamRun } from "@/lib/sse"
import { ChatMessage } from "@/components/playground/ChatMessage"
import { ChatInput } from "@/components/playground/ChatInput"
import { NodeExecutionPanel } from "@/components/playground/NodeExecutionPanel"
import { ApprovalDialog } from "@/components/playground/ApprovalDialog"
import type { Message } from "@/types/entities"

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

  // Inline assistant picker (studio originally drove this from its
  // sidebar; the dashboard sidebar is observability-shaped so we host
  // the selector inside the page).
  const { data: assistants } = useAssistants()
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
        {/* Assistant picker + SSE status bar */}
        <div className="flex items-center justify-between gap-3 border-b border-border bg-card px-4 py-2">
          <div className="flex items-center gap-2">
            <label htmlFor="assistant" className="text-xs text-muted-foreground">
              Assistant:
            </label>
            <select
              id="assistant"
              value={selectedAssistantId ?? ""}
              onChange={(e) => setAssistant(e.target.value || null)}
              className="border border-input bg-background px-2 py-1 text-xs focus:outline-none focus:ring-2 focus:ring-ring"
            >
              <option value="">— select —</option>
              {assistants?.map((a) => (
                <option key={a.assistant_id} value={a.assistant_id}>
                  {a.name || a.assistant_id.slice(0, 8)}
                </option>
              ))}
            </select>
          </div>
          {sseStatus !== "closed" && (
            <span className="flex items-center gap-2 text-xs">
              <span
                className={`inline-block h-2 w-2 ${
                  sseStatus === "open"
                    ? "bg-green-500"
                    : sseStatus === "connecting"
                      ? "bg-yellow-500 animate-pulse"
                      : "bg-red-500"
                }`}
              />
              <span className="text-muted-foreground">{sseStatus}</span>
            </span>
          )}
        </div>

        {/* Messages */}
        <div ref={scrollRef} className="flex-1 overflow-y-auto p-6">
          <div className="mx-auto max-w-3xl space-y-4">
            {messages.length === 0 && !isStreaming && (
              <div className="py-20 text-center text-muted-foreground">
                <p className="text-lg font-medium">DuraGraph Playground</p>
                <p className="mt-2 text-sm">
                  {selectedAssistantId
                    ? "Send a message to start a conversation"
                    : "Select an assistant from the sidebar to begin"}
                </p>
              </div>
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
