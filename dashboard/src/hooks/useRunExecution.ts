import { useEffect, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { applyExecutionEvent, emptyExecution } from "@/api/runExecution"

const eventTypes = ["run.created", "run.started", "run.completed", "run.failed", "run.cancelled", "run.requires_action", "execution.node_started", "execution.node_completed", "execution.node_failed"]

export function useRunExecution(threadId: string | null, runId: string) {
  const [state, setState] = useState(() => ({ runId, execution: emptyExecution(), connected: false, unavailable: false, received: false }))
  const queryClient = useQueryClient()
  const current = state.runId === runId ? state : { runId, execution: emptyExecution(), connected: false, unavailable: false, received: false }

  useEffect(() => {
    if (!threadId) return
    const source = new EventSource(`/api/v1/threads/${encodeURIComponent(threadId)}/runs/${encodeURIComponent(runId)}/stream`)
    source.onopen = () => setState(s => ({ ...(s.runId === runId ? s : { runId, execution: emptyExecution(), received: false }), connected: true, unavailable: false }))
    source.onerror = () => {
      setState(s => ({ ...s, connected: false, unavailable: !s.received }))
      // A finished stream closes normally. Status remains available via GET.
      source.close()
    }
    for (const type of eventTypes) {
      source.addEventListener(type, (message) => {
        try {
          const payload: unknown = JSON.parse((message as MessageEvent).data)
          setState(s => ({ ...s, runId, received: true, execution: applyExecutionEvent(s.runId === runId ? s.execution : emptyExecution(), type, payload) }))
          if (type === "run.completed" || type === "run.failed" || type === "run.cancelled" || type === "run.requires_action") {
            queryClient.invalidateQueries({ queryKey: ["run", runId] })
            queryClient.invalidateQueries({ queryKey: ["runs"] })
          }
        } catch { /* malformed event: ignore, status polling remains authoritative */ }
      })
    }
    return () => { source.close() }
  }, [threadId, runId, queryClient])

  return current
}
