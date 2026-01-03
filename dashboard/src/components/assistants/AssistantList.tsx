import { useState, useMemo } from "react"
import { Link } from "@tanstack/react-router"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Bot, Pencil, Trash2, MoreHorizontal } from "lucide-react"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { DeleteConfirmationDialog } from "@/components/ui/delete-confirmation-dialog"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/api/client"
import type { Assistant, AssistantsResponse } from "@/types/entities"
import { toast } from "sonner"
import { SearchFilter } from "@/components/common/SearchFilter"

interface AssistantListProps {
  showFilters?: boolean
}

export function AssistantList({ showFilters = true }: AssistantListProps) {
  const [search, setSearch] = useState("")

  const { data, isLoading, error } = useQuery({
    queryKey: ["assistants"],
    queryFn: () => api.get<AssistantsResponse>("/assistants"),
  })
  const assistants = data?.assistants

  // Filter assistants based on search
  const filteredAssistants = useMemo(() => {
    if (!assistants) return []
    if (!search) return assistants
    const searchLower = search.toLowerCase()
    return assistants.filter(
      (assistant) =>
        assistant.name.toLowerCase().includes(searchLower) ||
        assistant.assistant_id.toLowerCase().includes(searchLower) ||
        assistant.description?.toLowerCase().includes(searchLower)
    )
  }, [assistants, search])

  const handleClearFilters = () => {
    setSearch("")
  }

  if (isLoading) {
    return <AssistantListSkeleton />
  }

  if (error) {
    return (
      <Card className="p-6">
        <p className="text-sm text-red-600">
          Failed to load assistants: {error.message}
        </p>
      </Card>
    )
  }

  if (!assistants || assistants.length === 0) {
    return (
      <Card className="p-12 text-center">
        <Bot className="h-12 w-12 mx-auto text-muted-foreground/30 mb-4" />
        <h3 className="text-lg font-semibold mb-2">
          No assistants yet
        </h3>
        <p className="text-muted-foreground mb-4">
          Create your first assistant to get started
        </p>
      </Card>
    )
  }

  return (
    <div>
      {showFilters && (
        <SearchFilter
          searchValue={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search assistants..."
          onClear={handleClearFilters}
        />
      )}

      {filteredAssistants.length === 0 ? (
        <Card className="p-12 text-center">
          <Bot className="h-12 w-12 mx-auto text-muted-foreground/30 mb-4" />
          <h3 className="text-lg font-semibold mb-2">No matching assistants</h3>
          <p className="text-muted-foreground">
            Try adjusting your search.
          </p>
          <Button variant="outline" className="mt-4" onClick={handleClearFilters}>
            Clear Search
          </Button>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {filteredAssistants.map((assistant) => (
            <AssistantCard key={assistant.assistant_id} assistant={assistant} />
          ))}
        </div>
      )}
    </div>
  )
}

interface AssistantCardProps {
  assistant: Assistant
}

function AssistantCard({ assistant }: AssistantCardProps) {
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const queryClient = useQueryClient()

  const deleteMutation = useMutation({
    mutationFn: () => api.delete(`/assistants/${assistant.assistant_id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["assistants"] })
      toast.success("Assistant deleted successfully")
      setDeleteDialogOpen(false)
    },
    onError: (error: Error) => {
      toast.error(`Failed to delete assistant: ${error.message}`)
    },
  })

  return (
    <>
      <Link
        to="/assistants/$assistantId"
        params={{ assistantId: assistant.assistant_id }}
        className="block"
      >
        <Card className="hover:shadow-md transition-shadow cursor-pointer">
          <CardHeader className="pb-3">
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-3">
                <div className="h-10 w-10 rounded-lg bg-primary/10 flex items-center justify-center">
                  <Bot className="h-5 w-5 text-primary" />
                </div>
                <div>
                  <CardTitle className="text-base">{assistant.name}</CardTitle>
                  <p className="text-xs text-muted-foreground font-mono mt-0.5">
                    {assistant.assistant_id.slice(0, 8)}...
                  </p>
                </div>
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    onClick={(e) => e.preventDefault()}
                  >
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={(e) => e.stopPropagation()}>
                    <Pencil className="h-4 w-4 mr-2" />
                    Edit
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    className="text-red-600"
                    onClick={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                      setDeleteDialogOpen(true)
                    }}
                  >
                    <Trash2 className="h-4 w-4 mr-2" />
                    Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground line-clamp-2">
              {assistant.description || "No description"}
            </p>
            <div className="mt-4 pt-4 border-t">
              <p className="text-xs text-muted-foreground">
                Created {new Date(assistant.created_at * 1000).toLocaleDateString()}
              </p>
            </div>
          </CardContent>
        </Card>
      </Link>

      <DeleteConfirmationDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        title="Delete Assistant"
        description={`Are you sure you want to delete "${assistant.name}"? This action cannot be undone.`}
        onConfirm={async () => { await deleteMutation.mutateAsync() }}
        isPending={deleteMutation.isPending}
      />
    </>
  )
}

function AssistantListSkeleton() {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      {[...Array(6)].map((_, i) => (
        <Card key={i} className="p-6">
          <Skeleton className="h-10 w-10 rounded-lg mb-4" />
          <Skeleton className="h-5 w-32 mb-2" />
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-3/4 mt-2" />
        </Card>
      ))}
    </div>
  )
}
