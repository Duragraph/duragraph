import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { Run } from '@/types/entities'

export function useRuns(threadId: string | null) {
  return useQuery({
    queryKey: ['runs', threadId],
    queryFn: () => apiFetch<Run[]>(`/threads/${threadId}/runs`),
    enabled: !!threadId,
    refetchInterval: 5000,
  })
}

export function useRun(runId: string | null) {
  return useQuery({
    queryKey: ['run', runId],
    queryFn: () => apiFetch<Run>(`/runs/${runId}`),
    enabled: !!runId,
  })
}

export function useAllRuns() {
  return useQuery({
    queryKey: ['runs'],
    queryFn: () => apiFetch<Run[]>('/runs'),
    refetchInterval: 5000,
  })
}

export function useCreateRun() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: {
      thread_id: string
      assistant_id: string
      input: Record<string, unknown>
    }) =>
      apiFetch<Run>(`/threads/${body.thread_id}/runs`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['runs'] }),
  })
}
