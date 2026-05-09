import { useState } from 'react'
import { apiFetch } from '@/lib/api'

interface ApprovalDialogProps {
  runId: string
  threadId: string
  prompt?: string
  onResolved: () => void
}

export function ApprovalDialog({
  runId,
  threadId,
  prompt,
  onResolved,
}: ApprovalDialogProps) {
  const [feedback, setFeedback] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function handleAction(action: 'approve' | 'reject') {
    setSubmitting(true)
    try {
      await apiFetch(`/threads/${threadId}/runs/${runId}/resume`, {
        method: 'POST',
        body: JSON.stringify({
          action,
          feedback: feedback || undefined,
        }),
      })
      onResolved()
    } catch {
      setSubmitting(false)
    }
  }

  return (
    <div className="border border-orange-300 bg-orange-50 p-4">
      <div className="mb-3 flex items-center gap-2">
        <span className="inline-block h-2 w-2 bg-orange-500 animate-pulse" />
        <span className="text-sm font-semibold text-orange-800">
          Human Review Required
        </span>
      </div>

      {prompt && (
        <p className="mb-3 text-sm text-orange-700">{prompt}</p>
      )}

      <textarea
        value={feedback}
        onChange={(e) => setFeedback(e.target.value)}
        placeholder="Optional feedback..."
        className="mb-3 w-full border border-orange-200 bg-white p-2 text-sm font-mono focus:outline-none focus:border-orange-400"
        rows={3}
      />

      <div className="flex gap-2">
        <button
          onClick={() => handleAction('approve')}
          disabled={submitting}
          className="bg-green-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-green-700 disabled:opacity-50"
        >
          {submitting ? 'Submitting...' : 'Approve'}
        </button>
        <button
          onClick={() => handleAction('reject')}
          disabled={submitting}
          className="bg-red-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
        >
          Reject
        </button>
      </div>
    </div>
  )
}
