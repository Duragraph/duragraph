import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import { isActiveRun, runsPollInterval, type Run, type V2Run } from '@/types/entities'

// runsPollInterval — adaptive refetch cadence for run-list queries.
// Fast (1.5 s) while at least one run is non-terminal so live status
// transitions surface promptly; slow (15 s) when everything is
// settled, so the dashboard isn't refetching the whole runs list
// every few seconds for views that haven't changed in hours.
export { runsPollInterval }

export function useRuns(threadId: string | null) {
  return useQuery({
    queryKey: ['runs', threadId],
    queryFn: () => apiFetch<Run[]>(`/threads/${threadId}/runs`),
    enabled: !!threadId,
    refetchInterval: (q) => runsPollInterval(q.state.data as Run[] | undefined),
  })
}

export function useRun(runId: string | null) {
  return useQuery({
    queryKey: ['run', runId],
    queryFn: () => apiFetch<V2Run>(`/runs/${runId}`),
    enabled: !!runId,
    refetchInterval: (q) => q.state.data && isActiveRun(q.state.data) ? 2000 : false,
  })
}

export function useAllRuns() {
  return useQuery({
    queryKey: ['runs'],
    queryFn: () => apiFetch<V2Run[]>('/runs'),
    refetchInterval: (q) => runsPollInterval(q.state.data),
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
