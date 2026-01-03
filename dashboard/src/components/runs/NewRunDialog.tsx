import { useState, useEffect } from "react"
import { useNavigate } from "@tanstack/react-router"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/api/client"
import type { Assistant, Thread, Run } from "@/types/entities"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { Loader2, Play, Plus } from "lucide-react"
import { toast } from "sonner"

interface NewRunDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultThreadId?: string
  defaultAssistantId?: string
}

export function NewRunDialog({
  open,
  onOpenChange,
  defaultThreadId,
  defaultAssistantId,
}: NewRunDialogProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [assistantId, setAssistantId] = useState(defaultAssistantId || "")
  const [threadId, setThreadId] = useState(defaultThreadId || "")
  const [createNewThread, setCreateNewThread] = useState(!defaultThreadId)
  const [input, setInput] = useState('{\n  "message": "Hello"\n}')

  // Reset state when dialog opens
  useEffect(() => {
    if (open) {
      setAssistantId(defaultAssistantId || "")
      setThreadId(defaultThreadId || "")
      setCreateNewThread(!defaultThreadId)
      setInput('{\n  "message": "Hello"\n}')
    }
  }, [open, defaultAssistantId, defaultThreadId])

  // Fetch assistants
  const { data: assistantsData, isLoading: isLoadingAssistants } = useQuery({
    queryKey: ["assistants"],
    queryFn: () => api.get<{ assistants: Assistant[] }>("/assistants"),
    enabled: open,
  })

  // Fetch threads
  const { data: threadsData, isLoading: isLoadingThreads } = useQuery({
    queryKey: ["threads"],
    queryFn: () => api.get<{ threads: Thread[] }>("/threads"),
    enabled: open && !createNewThread,
  })

  const assistants = assistantsData?.assistants || []
  const threads = threadsData?.threads || []

  // Create thread mutation
  const createThreadMutation = useMutation({
    mutationFn: () => api.post<Thread>("/threads", {}),
  })

  // Create run mutation
  const createRunMutation = useMutation({
    mutationFn: (data: { thread_id: string; assistant_id: string; input: Record<string, unknown> }) =>
      api.post<Run>("/runs", data),
    onSuccess: (run) => {
      queryClient.invalidateQueries({ queryKey: ["runs"] })
      toast.success("Run started successfully")
      onOpenChange(false)
      navigate({ to: "/runs/$runId", params: { runId: run.run_id } })
    },
    onError: (error) => {
      toast.error(`Failed to start run: ${error.message}`)
    },
  })

  const handleSubmit = async () => {
    if (!assistantId) {
      toast.error("Please select an assistant")
      return
    }

    let parsedInput: Record<string, unknown>
    try {
      parsedInput = JSON.parse(input)
    } catch {
      toast.error("Invalid JSON input")
      return
    }

    let targetThreadId = threadId

    // Create a new thread if needed
    if (createNewThread) {
      try {
        const newThread = await createThreadMutation.mutateAsync()
        targetThreadId = newThread.id
        queryClient.invalidateQueries({ queryKey: ["threads"] })
      } catch (error) {
        toast.error(`Failed to create thread: ${(error as Error).message}`)
        return
      }
    }

    if (!targetThreadId) {
      toast.error("Please select or create a thread")
      return
    }

    createRunMutation.mutate({
      thread_id: targetThreadId,
      assistant_id: assistantId,
      input: parsedInput,
    })
  }

  const isLoading =
    createThreadMutation.isPending || createRunMutation.isPending

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Play className="h-5 w-5" />
            New Run
          </DialogTitle>
          <DialogDescription>
            Start a new workflow execution with an assistant and thread.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {/* Assistant Selection */}
          <div className="space-y-2">
            <Label htmlFor="assistant">Assistant</Label>
            <Select value={assistantId} onValueChange={setAssistantId}>
              <SelectTrigger id="assistant">
                <SelectValue placeholder="Select an assistant" />
              </SelectTrigger>
              <SelectContent>
                {isLoadingAssistants ? (
                  <SelectItem value="loading" disabled>
                    Loading...
                  </SelectItem>
                ) : assistants.length === 0 ? (
                  <SelectItem value="none" disabled>
                    No assistants available
                  </SelectItem>
                ) : (
                  assistants.map((assistant) => (
                    <SelectItem
                      key={assistant.assistant_id}
                      value={assistant.assistant_id}
                    >
                      {assistant.name}
                    </SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
          </div>

          {/* Thread Selection */}
          <div className="space-y-2">
            <Label>Thread</Label>
            <div className="flex gap-2">
              <Button
                type="button"
                variant={createNewThread ? "default" : "outline"}
                size="sm"
                onClick={() => setCreateNewThread(true)}
              >
                <Plus className="h-4 w-4 mr-1" />
                New Thread
              </Button>
              <Button
                type="button"
                variant={!createNewThread ? "default" : "outline"}
                size="sm"
                onClick={() => setCreateNewThread(false)}
                disabled={!!defaultThreadId}
              >
                Existing Thread
              </Button>
            </div>
            {!createNewThread && (
              <Select value={threadId} onValueChange={setThreadId}>
                <SelectTrigger>
                  <SelectValue placeholder="Select a thread" />
                </SelectTrigger>
                <SelectContent>
                  {isLoadingThreads ? (
                    <SelectItem value="loading" disabled>
                      Loading...
                    </SelectItem>
                  ) : threads.length === 0 ? (
                    <SelectItem value="none" disabled>
                      No threads available
                    </SelectItem>
                  ) : (
                    threads.map((thread) => (
                      <SelectItem key={thread.id} value={thread.id}>
                        {thread.id.slice(0, 12)}... ({thread.messages?.length || 0} messages)
                      </SelectItem>
                    ))
                  )}
                </SelectContent>
              </Select>
            )}
          </div>

          {/* Input JSON */}
          <div className="space-y-2">
            <Label htmlFor="input">Input (JSON)</Label>
            <Textarea
              id="input"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder='{"message": "Hello"}'
              className="font-mono text-sm h-32"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={isLoading || !assistantId}>
            {isLoading ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Starting...
              </>
            ) : (
              <>
                <Play className="h-4 w-4 mr-2" />
                Start Run
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
