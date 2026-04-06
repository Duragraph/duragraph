import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { Assistant } from '@/types/entities'

export function useAssistants() {
  return useQuery({
    queryKey: ['assistants'],
    queryFn: () => apiFetch<Assistant[]>('/assistants'),
  })
}

export function useCreateAssistant() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { graph_id: string; name: string; config?: Record<string, unknown> }) =>
      apiFetch<Assistant>('/assistants', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['assistants'] }),
  })
}
