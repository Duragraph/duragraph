import { useRef, useEffect, useCallback, useState } from 'react'
import { useChatStore } from '@/stores/chat'
import { useCreateRun } from '@/api/runs'
import { useCreateThread } from '@/api/threads'
import { streamRun } from '@/lib/sse'
import { ChatMessage } from '@/components/chat/ChatMessage'
import { ChatInput } from '@/components/chat/ChatInput'
import { NodeExecutionPanel } from '@/components/chat/NodeExecutionPanel'
import { ApprovalDialog } from '@/components/chat/ApprovalDialog'
import type { Message } from '@/types/entities'

export function ChatView() {
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
    handleEvent,
    setSSEStatus,
  } = useChatStore()

  const createRun = useCreateRun()
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

      const userMessage: Message = { role: 'user', content }
      addMessage(userMessage)

      let threadId = selectedThreadId
      if (!threadId) {
        try {
          const thread = await createThread.mutateAsync({})
          threadId = thread.thread_id
          setThread(threadId)
        } catch {
          addMessage({
            role: 'assistant',
            content: 'Error: Failed to create thread',
          })
          return
        }
      }

      const allMessages = [...messages, userMessage]

      cleanupRef.current?.()
      cleanupRef.current = streamRun(
        threadId,
        selectedAssistantId,
        { messages: allMessages },
        {
          onEvent: (event) => {
            handleEvent(event)
            if (
              event.event === 'run_requires_action' ||
              (event.event === 'run_started' &&
                event.data.status === 'requires_action')
            ) {
              setPendingApproval({
                runId: (event.data.run_id as string) ?? '',
                threadId: threadId!,
                prompt: event.data.prompt as string | undefined,
              })
            }
          },
          onStatus: setSSEStatus,
          onError: (err) => {
            addMessage({
              role: 'assistant',
              content: `Error: ${err.message}`,
            })
          },
        },
      )

      createRun.mutate({
        thread_id: threadId,
        assistant_id: selectedAssistantId,
        input: { messages: allMessages },
      })
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
      createRun,
      createThread,
    ],
  )

  return (
    <div className="flex h-full">
      <div className="flex flex-1 flex-col">
        {/* SSE status indicator */}
        {sseStatus !== 'closed' && (
          <div className="flex items-center gap-2 border-b border-border px-4 py-1.5 text-xs">
            <span
              className={`inline-block h-2 w-2 ${
                sseStatus === 'open'
                  ? 'bg-green-500'
                  : sseStatus === 'connecting'
                    ? 'bg-yellow-500 animate-pulse'
                    : 'bg-red-500'
              }`}
            />
            <span className="text-muted-foreground">{sseStatus}</span>
          </div>
        )}

        {/* Messages */}
        <div ref={scrollRef} className="flex-1 overflow-y-auto p-6">
          <div className="mx-auto max-w-3xl space-y-4">
            {messages.length === 0 && !isStreaming && (
              <div className="py-20 text-center text-muted-foreground">
                <p className="text-lg font-medium">DuraGraph Studio</p>
                <p className="mt-2 text-sm">
                  {selectedAssistantId
                    ? 'Send a message to start a conversation'
                    : 'Select an assistant from the sidebar to begin'}
                </p>
              </div>
            )}

            {messages.map((msg, i) => (
              <ChatMessage key={i} message={msg} />
            ))}

            {streamingContent && (
              <ChatMessage
                message={{ role: 'assistant', content: streamingContent }}
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

      {/* Node execution panel - shows during/after runs */}
      {nodeExecutions.length > 0 && (
        <NodeExecutionPanel executions={nodeExecutions} />
      )}
    </div>
  )
}
