import { useEffect, useCallback, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import type { Run, RunStatus } from "@/types/entities"

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || "http://localhost:8081/api/v1"

export interface RunStreamEvent {
  event: string
  data: {
    run_id?: string
    thread_id?: string
    status?: RunStatus
    output?: Record<string, unknown>
    error?: string
    node_id?: string
    node_output?: unknown
    [key: string]: unknown
  }
}

interface UseRunStreamOptions {
  runId: string
  enabled?: boolean
  onEvent?: (event: RunStreamEvent) => void
  onNodeUpdate?: (nodeId: string, status: "started" | "completed", output?: unknown) => void
  onComplete?: (run: Partial<Run>) => void
  onError?: (error: string) => void
}

export function useRunStream({
  runId,
  enabled = true,
  onEvent,
  onNodeUpdate,
  onComplete,
  onError,
}: UseRunStreamOptions) {
  const queryClient = useQueryClient()
  const eventSourceRef = useRef<EventSource | null>(null)
  const [isConnected, setIsConnected] = useState(false)
  const [lastEvent, setLastEvent] = useState<RunStreamEvent | null>(null)

  const connect = useCallback(() => {
    if (!runId || !enabled) return

    // Close existing connection
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
    }

    const url = `${API_BASE_URL}/stream?run_id=${runId}`
    const eventSource = new EventSource(url)
    eventSourceRef.current = eventSource

    eventSource.onopen = () => {
      setIsConnected(true)
    }

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        const streamEvent: RunStreamEvent = {
          event: data.event || "message",
          data,
        }

        setLastEvent(streamEvent)
        onEvent?.(streamEvent)

        // Handle different event types
        const eventType = data.event || data.type

        if (eventType === "node.started" && data.node_id) {
          onNodeUpdate?.(data.node_id, "started")
        }

        if (eventType === "node.completed" && data.node_id) {
          onNodeUpdate?.(data.node_id, "completed", data.output)
        }

        if (eventType === "run.completed" || eventType === "values") {
          // Invalidate run query to refresh data
          queryClient.invalidateQueries({ queryKey: ["run", runId] })
          queryClient.invalidateQueries({ queryKey: ["runs"] })

          if (data.status === "success" || data.status === "completed") {
            onComplete?.(data)
          }
        }

        if (eventType === "run.failed" || eventType === "error") {
          queryClient.invalidateQueries({ queryKey: ["run", runId] })
          queryClient.invalidateQueries({ queryKey: ["runs"] })
          onError?.(data.error || "Run failed")
        }

        if (eventType === "end") {
          eventSource.close()
          setIsConnected(false)
        }
      } catch (e) {
        console.error("Failed to parse SSE event:", e)
      }
    }

    eventSource.onerror = () => {
      setIsConnected(false)
      // Reconnect after a delay if still enabled
      setTimeout(() => {
        if (enabled && runId) {
          connect()
        }
      }, 3000)
    }
  }, [runId, enabled, onEvent, onNodeUpdate, onComplete, onError, queryClient])

  const disconnect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
      eventSourceRef.current = null
      setIsConnected(false)
    }
  }, [])

  useEffect(() => {
    if (enabled && runId) {
      connect()
    }

    return () => {
      disconnect()
    }
  }, [runId, enabled, connect, disconnect])

  return {
    isConnected,
    lastEvent,
    connect,
    disconnect,
  }
}

// Hook for streaming all runs (for lists)
interface UseRunsStreamOptions {
  enabled?: boolean
  onRunUpdate?: (runId: string, status: RunStatus) => void
}

export function useRunsStream({ enabled = true, onRunUpdate: _onRunUpdate }: UseRunsStreamOptions = {}) {
  const queryClient = useQueryClient()
  const eventSourceRef = useRef<EventSource | null>(null)
  const [isConnected, setIsConnected] = useState(false)

  const connect = useCallback(() => {
    if (!enabled) return

    if (eventSourceRef.current) {
      eventSourceRef.current.close()
    }

    // Subscribe to all run events using a wildcard topic
    // The backend would need to support this - for now we'll poll instead
    // This is a placeholder for when backend supports broadcast streams

    setIsConnected(true)
  }, [enabled])

  const disconnect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
      eventSourceRef.current = null
      setIsConnected(false)
    }
  }, [])

  // For now, use polling as fallback since broadcast stream isn't available
  useEffect(() => {
    if (!enabled) return

    const interval = setInterval(() => {
      queryClient.invalidateQueries({ queryKey: ["runs"] })
    }, 5000) // Poll every 5 seconds

    return () => clearInterval(interval)
  }, [enabled, queryClient])

  return {
    isConnected,
    connect,
    disconnect,
  }
}
