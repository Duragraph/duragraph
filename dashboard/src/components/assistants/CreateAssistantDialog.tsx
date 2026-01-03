import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
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

interface CreateAssistantDialogProps {
  trigger?: React.ReactNode
}

export function CreateAssistantDialog({ trigger }: CreateAssistantDialogProps) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [graphId, setGraphId] = useState("")
  const queryClient = useQueryClient()

  const createMutation = useMutation({
    mutationFn: (data: { name: string; description?: string; graph_id?: string }) =>
      api.post("/assistants", data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["assistants"] })
      toast.success("Assistant created successfully")
      setOpen(false)
      resetForm()
    },
    onError: (error: Error) => {
      toast.error(`Failed to create assistant: ${error.message}`)
    },
  })

  const resetForm = () => {
    setName("")
    setDescription("")
    setGraphId("")
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) {
      toast.error("Name is required")
      return
    }
    createMutation.mutate({
      name: name.trim(),
      description: description.trim() || undefined,
      graph_id: graphId.trim() || undefined,
    })
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {trigger || (
          <Button>
            <Plus className="h-4 w-4 mr-2" />
            Create Assistant
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Create New Assistant</DialogTitle>
            <DialogDescription>
              Create a new assistant to run workflows. You can configure it with a graph and
              additional settings.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="name">Name *</Label>
              <Input
                id="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="My Assistant"
                required
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="description">Description</Label>
              <Textarea
                id="description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="A helpful assistant that..."
                rows={3}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="graph_id">Graph ID</Label>
              <Input
                id="graph_id"
                value={graphId}
                onChange={(e) => setGraphId(e.target.value)}
                placeholder="simple_echo"
              />
              <p className="text-xs text-muted-foreground">
                The ID of the graph this assistant will use for execution.
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
