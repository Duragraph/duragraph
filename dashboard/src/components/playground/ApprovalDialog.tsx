import { useState } from "react"
import { Check, ShieldAlert, X } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { apiFetch } from "@/lib/api"

interface ApprovalDialogProps {
  runId: string
  threadId: string
  prompt?: string
  onResolved: () => void
}

// Inline HITL approval surface — rendered alongside the conversation
// when the engine emits a `run_requires_action` event. Uses
// <Alert variant="default"> with a custom-tinted accent so the
// approval-needed state stands out without yelling like
// `variant="destructive"`.
export function ApprovalDialog({
  runId,
  threadId,
  prompt,
  onResolved,
}: ApprovalDialogProps) {
  const [feedback, setFeedback] = useState("")
  const [submitting, setSubmitting] = useState(false)

  async function handleAction(action: "approve" | "reject") {
    setSubmitting(true)
    try {
      await apiFetch(`/threads/${threadId}/runs/${runId}/resume`, {
        method: "POST",
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
    <Alert className="border-amber-500/40 bg-amber-50/60 dark:border-amber-500/30 dark:bg-amber-950/30">
      <ShieldAlert className="size-4 text-amber-600 dark:text-amber-400" />
      <AlertTitle className="text-amber-900 dark:text-amber-200">
        Human review required
      </AlertTitle>
      <AlertDescription className="grid gap-3 text-amber-900/80 dark:text-amber-100/80">
        {prompt && <p>{prompt}</p>}
        <div className="grid gap-1.5">
          <Label
            htmlFor="approval-feedback"
            className="text-xs text-amber-900/70 dark:text-amber-100/70"
          >
            Optional feedback
          </Label>
          <Textarea
            id="approval-feedback"
            value={feedback}
            onChange={(e) => setFeedback(e.target.value)}
            placeholder="What should the agent know?"
            rows={3}
            className="bg-background"
          />
        </div>
        <div className="flex gap-2">
          <Button
            size="sm"
            onClick={() => handleAction("approve")}
            disabled={submitting}
            className="bg-emerald-600 text-white hover:bg-emerald-600/90"
          >
            <Check className="size-4" />
            {submitting ? "Submitting…" : "Approve"}
          </Button>
          <Button
            size="sm"
            variant="destructive"
            onClick={() => handleAction("reject")}
            disabled={submitting}
          >
            <X className="size-4" />
            Reject
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  )
}
