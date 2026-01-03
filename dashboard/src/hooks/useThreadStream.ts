import { useEffect, useCallback, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import type { RunStatus } from "@/types/entities"

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || "http://localhost:8081/api/v1"

export interface ThreadStreamEvent {
  event: string
  data: {
    run_id?: string
    thread_id?: string
    status?: RunStatus
    message?: {
      role: string
      content: string
    }
    [key: string]: unknown
  }
}

interface UseThreadStreamOptions {
  threadId: string
  enabled?: boolean
  onEvent?: (event: ThreadStreamEvent) => void
  onMessage?: (message: { role: string; content: string }) => void
  onRunUpdate?: (runId: string, status: RunStatus) => void
}

export function useThreadStream({
  threadId,
  enabled = true,
  onEvent,
  onMessage,
  onRunUpdate,
}: UseThreadStreamOptions) {
  const queryClient = useQueryClient()
  const eventSourceRef = useRef<EventSource | null>(null)
  const [isConnected, setIsConnected] = useState(false)
  const [lastEvent, setLastEvent] = useState<ThreadStreamEvent | null>(null)

  const connect = useCallback(() => {
    if (!threadId || !enabled) return

    // Close existing connection
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
    }

    const url = `${API_BASE_URL}/threads/${threadId}/stream`
    const eventSource = new EventSource(url)
    eventSourceRef.current = eventSource

    eventSource.onopen = () => {
      setIsConnected(true)
    }

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        const streamEvent: ThreadStreamEvent = {
          event: data.event || "message",
          data,
        }

        setLastEvent(streamEvent)
        onEvent?.(streamEvent)

        const eventType = data.event || data.type

        // Handle message events
        if (eventType === "message" || eventType === "message.completed") {
          if (data.message) {
            onMessage?.(data.message)
          }
          // Refresh thread to get updated messages
          queryClient.invalidateQueries({ queryKey: ["thread", threadId] })
        }

        // Handle run status updates
        if (
          eventType === "run.started" ||
          eventType === "run.completed" ||
          eventType === "run.failed"
        ) {
          if (data.run_id && data.status) {
            onRunUpdate?.(data.run_id, data.status)
          }
          // Refresh thread runs
          queryClient.invalidateQueries({ queryKey: ["runs", { thread_id: threadId }] })
        }
      } catch (e) {
        console.error("Failed to parse SSE event:", e)
      }
    }

    eventSource.onerror = () => {
      setIsConnected(false)
      // Reconnect after a delay
      setTimeout(() => {
        if (enabled && threadId) {
          connect()
        }
      }, 3000)
    }
  }, [threadId, enabled, onEvent, onMessage, onRunUpdate, queryClient])

  const disconnect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
      eventSourceRef.current = null
      setIsConnected(false)
    }
  }, [])

  useEffect(() => {
    if (enabled && threadId) {
      connect()
    }

    return () => {
      disconnect()
    }
  }, [threadId, enabled, connect, disconnect])

  return {
    isConnected,
    lastEvent,
    connect,
    disconnect,
  }
}
