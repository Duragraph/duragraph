import { create } from 'zustand'
import type { Message, RunEvent, NodeExecution } from '@/types/entities'
import type { SSEStatus } from '@/lib/sse'

interface ChatState {
  selectedThreadId: string | null
  selectedAssistantId: string | null
  messages: Message[]
  streamingContent: string
  isStreaming: boolean
  sseStatus: SSEStatus
  nodeExecutions: NodeExecution[]

  setThread: (id: string | null) => void
  setAssistant: (id: string | null) => void
  addMessage: (msg: Message) => void
  setMessages: (msgs: Message[]) => void
  setStreaming: (streaming: boolean) => void
  setSSEStatus: (status: SSEStatus) => void
  appendStreamChunk: (chunk: string) => void
  finalizeStream: () => void
  handleEvent: (event: RunEvent) => void
  addNodeExecution: (exec: NodeExecution) => void
  clearNodeExecutions: () => void
  reset: () => void
}

export const useChatStore = create<ChatState>((set, get) => ({
  selectedThreadId: null,
  selectedAssistantId: null,
  messages: [],
  streamingContent: '',
  isStreaming: false,
  sseStatus: 'closed',
  nodeExecutions: [],

  setThread: (id) => set({ selectedThreadId: id, messages: [], nodeExecutions: [] }),
  setAssistant: (id) => set({ selectedAssistantId: id }),
  addMessage: (msg) => set((s) => ({ messages: [...s.messages, msg] })),
  setMessages: (msgs) => set({ messages: msgs }),
  setStreaming: (streaming) => set({ isStreaming: streaming }),
  setSSEStatus: (status) => set({ sseStatus: status }),

  appendStreamChunk: (chunk) =>
    set((s) => ({ streamingContent: s.streamingContent + chunk })),

  finalizeStream: () => {
    const content = get().streamingContent
    if (content) {
      set((s) => ({
        messages: [...s.messages, { role: 'assistant', content }],
        streamingContent: '',
        isStreaming: false,
      }))
    } else {
      set({ isStreaming: false })
    }
  },

  handleEvent: (event) => {
    const { addNodeExecution, appendStreamChunk, finalizeStream } = get()

    switch (event.event) {
      case 'run_started':
        set({ isStreaming: true, streamingContent: '', nodeExecutions: [] })
        break

      case 'node_started':
        addNodeExecution({
          node_id: (event.data.node_id as string) ?? 'unknown',
          node_type: (event.data.node_type as string) ?? 'function',
          status: 'started',
          started_at: new Date().toISOString(),
        })
        break

      case 'node_completed':
        addNodeExecution({
          node_id: (event.data.node_id as string) ?? 'unknown',
          node_type: (event.data.node_type as string) ?? 'function',
          status: 'completed',
          output: event.data.output as Record<string, unknown>,
          completed_at: new Date().toISOString(),
        })
        break

      case 'node_failed':
        addNodeExecution({
          node_id: (event.data.node_id as string) ?? 'unknown',
          node_type: (event.data.node_type as string) ?? 'function',
          status: 'failed',
          error: event.data.error as string,
          completed_at: new Date().toISOString(),
        })
        break

      case 'output_chunk': {
        const chunk = (event.data.content as string) ?? (event.data.chunk as string) ?? ''
        if (chunk) appendStreamChunk(chunk)
        break
      }

      case 'run_completed': {
        const output = event.data.output as Record<string, unknown> | undefined
        if (output) {
          const messages = output.messages as Message[] | undefined
          if (messages && messages.length > 0) {
            const lastMsg = messages[messages.length - 1]
            if (lastMsg.role === 'assistant') {
              set((s) => ({
                messages: [...s.messages, lastMsg],
                streamingContent: '',
                isStreaming: false,
              }))
              return
            }
          }
          const response = (output.response as string) ?? ''
          if (response) {
            set((s) => ({
              messages: [...s.messages, { role: 'assistant', content: response }],
              streamingContent: '',
              isStreaming: false,
            }))
            return
          }
        }
        finalizeStream()
        break
      }

      case 'run_failed':
        set((s) => ({
          messages: [
            ...s.messages,
            {
              role: 'assistant',
              content: `Error: ${(event.data.error as string) ?? 'Run failed'}`,
            },
          ],
          streamingContent: '',
          isStreaming: false,
        }))
        break

      default:
        break
    }
  },

  addNodeExecution: (exec) =>
    set((s) => {
      const existing = s.nodeExecutions.findIndex(
        (n) => n.node_id === exec.node_id && n.status === 'started',
      )
      if (existing >= 0 && exec.status !== 'started') {
        const updated = [...s.nodeExecutions]
        updated[existing] = { ...updated[existing], ...exec }
        return { nodeExecutions: updated }
      }
      return { nodeExecutions: [...s.nodeExecutions, exec] }
    }),

  clearNodeExecutions: () => set({ nodeExecutions: [] }),

  reset: () =>
    set({
      selectedThreadId: null,
      selectedAssistantId: null,
      messages: [],
      streamingContent: '',
      isStreaming: false,
      sseStatus: 'closed',
      nodeExecutions: [],
    }),
}))
