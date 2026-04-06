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

  fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ assistant_id: assistantId, input }),
    signal: controller.signal,
  })
    .then(async (res) => {
      if (!res.ok || !res.body) {
        callbacks.onStatus('error')
        callbacks.onError?.(new Error(`Stream failed: ${res.status}`))
        return
      }
      callbacks.onStatus('open')
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
            try {
              const data = JSON.parse(dataStr)
              callbacks.onEvent({
                event: currentEvent || 'message',
                data,
              })
            } catch {
              callbacks.onEvent({
                event: currentEvent || 'message',
                data: { raw: dataStr },
              })
            }
            currentEvent = ''
          }
        }
      }
      callbacks.onStatus('closed')
    })
    .catch((err: Error) => {
      if (err.name !== 'AbortError') {
        callbacks.onStatus('error')
        callbacks.onError?.(err)
      }
    })

  return () => {
    controller.abort()
    callbacks.onStatus('closed')
  }
}
