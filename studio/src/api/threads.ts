import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { Thread } from '@/types/entities'

export function useThreads() {
  return useQuery({
    queryKey: ['threads'],
    queryFn: () => apiFetch<Thread[]>('/threads'),
  })
}

export function useThread(threadId: string | null) {
  return useQuery({
    queryKey: ['thread', threadId],
    queryFn: () => apiFetch<Thread>(`/threads/${threadId}`),
    enabled: !!threadId,
  })
}

export function useCreateThread() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body?: { metadata?: Record<string, unknown> }) =>
      apiFetch<Thread>('/threads', {
        method: 'POST',
        body: JSON.stringify(body ?? {}),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['threads'] }),
  })
}
