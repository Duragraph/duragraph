import type { RunEvent } from '@/types/entities'

export type SSEStatus = 'connecting' | 'open' | 'closed' | 'error'

export interface SSECallbacks {
  onEvent: (event: RunEvent) => void
  onStatus: (status: SSEStatus) => void
  onError?: (error: Error) => void
}

export function createRunStream(
  threadId: string,
  runId: string,
  callbacks: SSECallbacks,
): () => void {
  const url = `/api/v1/threads/${threadId}/runs/${runId}/stream`
  callbacks.onStatus('connecting')

  const es = new EventSource(url)

  es.onopen = () => {
    callbacks.onStatus('open')
  }

  es.onmessage = (msg) => {
    try {
      const event: RunEvent = JSON.parse(msg.data)
      callbacks.onEvent(event)
    } catch {
      // non-JSON event, wrap it
      callbacks.onEvent({ event: 'message', data: { raw: msg.data } })
    }
  }

  // handle named event types the server sends
  for (const eventType of [
    'metadata',
    'run_started',
    'run_completed',
    'run_failed',
    'node_started',
    'node_completed',
    'node_failed',
    'output_chunk',
  ]) {
    es.addEventListener(eventType, ((evt: MessageEvent) => {
      try {
        const parsed = JSON.parse(evt.data)
        callbacks.onEvent({ event: eventType, data: parsed })
      } catch {
        callbacks.onEvent({ event: eventType, data: { raw: evt.data } })
      }
    }) as EventListener)
  }

  es.onerror = () => {
    if (es.readyState === EventSource.CLOSED) {
      callbacks.onStatus('closed')
    } else {
      callbacks.onStatus('error')
      callbacks.onError?.(new Error('SSE connection error'))
    }
  }

  return () => {
    es.close()
    callbacks.onStatus('closed')
  }
}

export function streamRun(
  threadId: string,
  assistantId: string,
  input: Record<string, unknown>,
  callbacks: SSECallbacks,
): () => void {
  const url = `/api/v1/threads/${threadId}/runs/stream`
  callbacks.onStatus('connecting')

  const controller = new AbortController()
  let pollTimer: ReturnType<typeof setInterval> | null = null
  let runIdCaptured: string | null = null
  let completedFired = false

  const stopPolling = () => {
    if (pollTimer !== null) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  // Worker-driven runs currently don't emit run_started/node_*/run_completed
  // through the streaming endpoint — only `metadata` events. The run does
  // complete on the control plane though, so once we know the run_id we
  // poll `/api/v1/runs/{id}` and synthesize `run_completed` (or `run_failed`)
  // when the run reaches a terminal state. This is a Studio-side fallback.
  const startPollingFor = (runId: string) => {
    if (pollTimer || completedFired) return
    pollTimer = setInterval(async () => {
      try {
        const r = await fetch(`/api/v1/runs/${runId}`)
        if (!r.ok) return
        const run = await r.json()
        const status = run.status as string

        // States where the polling fallback should stop. `requires_action`
        // is included because the run is paused waiting for human input
        // — it is not strictly terminal, but the *current* round trip is
        // done and continuing to poll would just spam the server until
        // the human resolves the interrupt.
        const stopStates = [
          'completed',
          'success',
          'failed',
          'error',
          'cancelled',
          'timeout',
          'requires_action',
          'interrupted',
        ]
        if (!stopStates.includes(status)) return

        // Idempotency guard: two setInterval iterations can be
        // in-flight on `await fetch(...)` concurrently. If both
        // resolve to terminal state in the same tick, both would
        // fire `onEvent({event: 'run_completed'})` before the first
        // could set `completedFired`. That's how the user saw the
        // same assistant message appended twice on a single turn.
        // Re-check after the awaits resolve.
        if (completedFired) return
        completedFired = true
        stopPolling()
        controller.abort()

        if (status === 'completed' || status === 'success') {
          callbacks.onEvent({
            event: 'run_completed',
            data: { run_id: runId, output: run.output ?? {} },
          })
        } else if (status === 'requires_action' || status === 'interrupted') {
          // ChatView listens for `run_requires_action` and opens the
          // ApprovalDialog. The dialog drives the resume RPC.
          callbacks.onEvent({
            event: 'run_requires_action',
            data: {
              run_id: runId,
              status,
              prompt:
                (run.output?.prompt as string | undefined) ??
                (run.metadata?.prompt as string | undefined),
              output: run.output ?? {},
            },
          })
        } else {
          callbacks.onEvent({
            event: 'run_failed',
            data: {
              run_id: runId,
              error: (run.error as string) || `run ${status}`,
            },
          })
        }
        callbacks.onStatus('closed')
      } catch {
        // ignore — keep polling until timeout-by-cancel
      }
    }, 750)
  }

  fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ assistant_id: assistantId, input }),
    signal: controller.signal,
  })
    .then(async (res) => {
      if (!res.ok || !res.body) {
        callbacks.onStatus('error')
        // Surface a useful message — especially for 409 multitask-reject so
        // the user knows what to do next.
        let msg = `Stream failed: ${res.status}`
        try {
          const body = await res.clone().json()
          if (typeof body?.message === 'string') {
            msg = body.message
          }
          if (res.status === 409) {
            msg +=
              " — resolve the pending approval dialog if any, or refresh to start a new thread."
          }
        } catch {
          // body wasn't JSON; keep the generic message
        }
        callbacks.onError?.(new Error(msg))
        return
      }
      callbacks.onStatus('open')
      // Synthesize a run_started so the chatStore flips isStreaming true
      // before we have any lifecycle events from the backend.
      callbacks.onEvent({ event: 'run_started', data: {} })

      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })

        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''

        let currentEvent = ''
        for (const line of lines) {
          if (line.startsWith('event: ')) {
            currentEvent = line.slice(7).trim()
          } else if (line.startsWith('data: ')) {
            const dataStr = line.slice(6)
            const evName = currentEvent || 'message'

            // Hard idempotency gate for terminal events. The
            // assistant-message duplication shipped because *two*
            // paths can deliver `run_completed`: the engine's own
            // SSE stream, and the polling fallback that hits
            // /api/v1/runs/{id}. Either can win the race. Whichever
            // arrives first sets `completedFired`; subsequent
            // terminal events (from the other path, or — racier
            // still — from another iteration of the same path)
            // skip the forward to the store. Non-terminal events
            // (output_chunk, node_started, metadata) still pass
            // through normally.
            const isTerminal =
              evName === 'run_completed' ||
              evName === 'run_failed' ||
              evName === 'run_requires_action'
            if (isTerminal) {
              if (completedFired) {
                currentEvent = ''
                continue
              }
              completedFired = true
              stopPolling()
            }

            try {
              const data = JSON.parse(dataStr)
              callbacks.onEvent({ event: evName, data })
              if (
                !runIdCaptured &&
                typeof data === 'object' &&
                data &&
                typeof data.run_id === 'string'
              ) {
                runIdCaptured = data.run_id
                startPollingFor(data.run_id)
              }
            } catch {
              callbacks.onEvent({
                event: evName,
                data: { raw: dataStr },
              })
            }
            currentEvent = ''
          }
        }
      }
      // Stream closed without giving us a run_completed — polling fallback
      // (if it started) will eventually synthesize one.
      if (!completedFired && !pollTimer) {
        callbacks.onStatus('closed')
      }
    })
    .catch((err: Error) => {
      if (err.name !== 'AbortError') {
        callbacks.onStatus('error')
        callbacks.onError?.(err)
      }
    })

  return () => {
    stopPolling()
    controller.abort()
    callbacks.onStatus('closed')
  }
}
