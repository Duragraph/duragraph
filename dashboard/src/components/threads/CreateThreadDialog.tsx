import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { api } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Plus, Loader2 } from "lucide-react"
import { toast } from "sonner"

interface CreateThreadDialogProps {
  trigger?: React.ReactNode
}

interface ThreadResponse {
  thread_id: string
}

export function CreateThreadDialog({ trigger }: CreateThreadDialogProps) {
  const [open, setOpen] = useState(false)
  const [metadata, setMetadata] = useState("")
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const createMutation = useMutation({
    mutationFn: (data: { metadata?: Record<string, unknown> }) =>
      api.post<ThreadResponse>("/threads", data),
    onSuccess: (response) => {
      queryClient.invalidateQueries({ queryKey: ["threads"] })
      toast.success("Thread created successfully")
      setOpen(false)
      resetForm()
      // Navigate to the new thread
      if (response?.thread_id) {
        navigate({ to: "/threads/$threadId", params: { threadId: response.thread_id } })
      }
    },
    onError: (error: Error) => {
      toast.error(`Failed to create thread: ${error.message}`)
    },
  })

  const resetForm = () => {
    setMetadata("")
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    let parsedMetadata: Record<string, unknown> | undefined

    if (metadata.trim()) {
      try {
        parsedMetadata = JSON.parse(metadata)
      } catch {
        toast.error("Invalid JSON in metadata field")
        return
      }
    }

    createMutation.mutate({
      metadata: parsedMetadata,
    })
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {trigger || (
          <Button>
            <Plus className="h-4 w-4 mr-2" />
            Create Thread
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Create New Thread</DialogTitle>
            <DialogDescription>
              Create a new conversation thread. Threads hold messages and can be used with
              assistants to run workflows.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="metadata">Metadata (JSON, optional)</Label>
              <Textarea
                id="metadata"
                value={metadata}
                onChange={(e) => setMetadata(e.target.value)}
                placeholder='{"key": "value"}'
                rows={4}
                className="font-mono text-sm"
              />
              <p className="text-xs text-muted-foreground">
                Optional JSON object for custom metadata.
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={createMutation.isPending}>
              {createMutation.isPending && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              Create
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
