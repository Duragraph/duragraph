import { apiFetch } from '@/lib/api'
import type { Message, Run } from '@/types/entities'

/**
 * Rebuild the chat message list for a thread.
 *
 * Note: the control plane's `/threads/{tid}/runs` LIST endpoint returns a
 * truncated projection (`run_id, thread_id, assistant_id, status, created_at,
 * updated_at`) WITHOUT the `output` field. To get `output.messages` we have
 * to call the DETAIL endpoint `/runs/{id}` for the run we care about.
 *
 * Strategy: list runs (newest first), find the latest COMPLETED run, fetch
 * its detail, and read `output.messages` (which the chatbot example writes
 * as the full conversation). Falls back to walking earlier runs if the
 * latest one doesn't carry messages.
 */
export async function hydrateThreadMessages(threadId: string): Promise<Message[]> {
  const runs = await apiFetch<Run[]>(`/threads/${threadId}/runs`).catch(() => [] as Run[])
  if (!Array.isArray(runs) || runs.length === 0) return []

  // Sort newest first (the API may already do this, but don't trust it).
  const sortedNewestFirst = [...runs].sort((a, b) =>
    (b.created_at ?? '').localeCompare(a.created_at ?? ''),
  )

  // The control plane's LIST endpoint currently returns a stale `status`
  // (always "queued") regardless of actual state, so we cannot pre-filter
  // here. Walk newest first and trust only the DETAIL endpoint's status.
  // Stop at the first run whose detail has output.messages populated.
  for (const r of sortedNewestFirst) {
    const detail = await apiFetch<Run>(`/runs/${r.run_id}`).catch(() => null)
    if (!detail) continue
    if (detail.status !== 'completed' && detail.status !== 'failed') continue
    const out = detail.output as Record<string, unknown> | undefined
    const msgs = (out?.messages as Message[] | undefined) ?? null
    if (Array.isArray(msgs) && msgs.length > 0) {
      return msgs.map(coerceMessage)
    }
  }

  // Fallback: stitch user/assistant pairs across all runs (oldest first).
  const oldestFirst = [...sortedNewestFirst].reverse()
  const stitched: Message[] = []
  for (const r of oldestFirst) {
    const detail = await apiFetch<Run>(`/runs/${r.run_id}`).catch(() => null)
    if (!detail) continue
    const input = detail.input as Record<string, unknown> | undefined
    const out = detail.output as Record<string, unknown> | undefined
    const inputMsgs = (input?.messages as Message[] | undefined) ?? []
    if (inputMsgs.length > 0) {
      const lastUser = [...inputMsgs].reverse().find((m) => m.role === 'user')
      if (lastUser) stitched.push(coerceMessage(lastUser))
    } else if (typeof input?.input === 'string' && input.input) {
      stitched.push({ role: 'user', content: input.input as string })
    }
    const response = (out?.response as string | undefined) ?? ''
    if (response) stitched.push({ role: 'assistant', content: response })
  }
  return stitched
}

function coerceMessage(m: Message): Message {
  return {
    role: m.role,
    content: typeof m.content === 'string' ? m.content : JSON.stringify(m.content),
    name: m.name,
    tool_call_id: m.tool_call_id,
  }
}
