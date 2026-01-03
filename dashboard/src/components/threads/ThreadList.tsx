import { useState, useMemo } from "react"
import { Link } from "@tanstack/react-router"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { MessageSquare, MoreHorizontal, Eye, Trash2 } from "lucide-react"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { DeleteConfirmationDialog } from "@/components/ui/delete-confirmation-dialog"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/api/client"
import type { Thread, ThreadsResponse } from "@/types/entities"
import { toast } from "sonner"
import { SearchFilter } from "@/components/common/SearchFilter"

interface ThreadListProps {
  showFilters?: boolean
}

export function ThreadList({ showFilters = true }: ThreadListProps) {
  const [search, setSearch] = useState("")

  const { data, isLoading, error } = useQuery({
    queryKey: ["threads"],
    queryFn: () => api.get<ThreadsResponse>("/threads"),
  })
  const threads = data?.threads

  // Filter threads based on search
  const filteredThreads = useMemo(() => {
    if (!threads) return []
    if (!search) return threads
    const searchLower = search.toLowerCase()
    return threads.filter((thread) =>
      thread.id.toLowerCase().includes(searchLower)
    )
  }, [threads, search])

  const handleClearFilters = () => {
    setSearch("")
  }

  if (isLoading) {
    return <ThreadListSkeleton />
  }

  if (error) {
    return (
      <Card className="p-6">
        <p className="text-sm text-red-600">
          Failed to load threads: {error.message}
        </p>
      </Card>
    )
  }

  if (!threads || threads.length === 0) {
    return (
      <Card className="p-12 text-center">
        <MessageSquare className="h-12 w-12 mx-auto text-muted-foreground/30 mb-4" />
        <h3 className="text-lg font-semibold mb-2">
          No threads yet
        </h3>
        <p className="text-muted-foreground mb-4">
          Threads are created when you start a new conversation
        </p>
        <Button>Create Thread</Button>
      </Card>
    )
  }

  return (
    <div>
      {showFilters && (
        <SearchFilter
          searchValue={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search threads..."
          onClear={handleClearFilters}
        />
      )}

      {filteredThreads.length === 0 ? (
        <Card className="p-12 text-center">
          <MessageSquare className="h-12 w-12 mx-auto text-muted-foreground/30 mb-4" />
          <h3 className="text-lg font-semibold mb-2">No matching threads</h3>
          <p className="text-muted-foreground">
            Try adjusting your search.
          </p>
          <Button variant="outline" className="mt-4" onClick={handleClearFilters}>
            Clear Search
          </Button>
        </Card>
      ) : (
        <Card>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[300px]">Thread ID</TableHead>
                <TableHead>Metadata</TableHead>
                <TableHead className="w-[180px]">Created</TableHead>
                <TableHead className="w-[180px]">Updated</TableHead>
                <TableHead className="w-[60px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredThreads.map((thread) => (
                <ThreadRow key={thread.id} thread={thread} />
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </div>
  )
}

interface ThreadRowProps {
  thread: Thread
}

function ThreadRow({ thread }: ThreadRowProps) {
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const queryClient = useQueryClient()

  const deleteMutation = useMutation({
    mutationFn: () => api.delete(`/threads/${thread.id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["threads"] })
      toast.success("Thread deleted successfully")
      setDeleteDialogOpen(false)
    },
    onError: (error: Error) => {
      toast.error(`Failed to delete thread: ${error.message}`)
    },
  })

  return (
    <>
      <TableRow className="cursor-pointer hover:bg-muted/50">
        <TableCell>
          <Link
            to="/threads/$threadId"
            params={{ threadId: thread.id }}
            className="font-mono text-sm hover:underline"
          >
            {thread.id}
          </Link>
        </TableCell>
        <TableCell className="text-sm text-muted-foreground">
          {thread.metadata ? Object.keys(thread.metadata).length + " keys" : "—"}
        </TableCell>
        <TableCell className="text-muted-foreground">
          {new Date(thread.created_at * 1000).toLocaleString()}
        </TableCell>
        <TableCell className="text-muted-foreground">
          {new Date(thread.updated_at * 1000).toLocaleString()}
        </TableCell>
        <TableCell>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" onClick={(e) => e.stopPropagation()}>
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to="/threads/$threadId" params={{ threadId: thread.id }}>
                  <Eye className="h-4 w-4 mr-2" />
                  View
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem
                className="text-red-600"
                onClick={() => setDeleteDialogOpen(true)}
              >
                <Trash2 className="h-4 w-4 mr-2" />
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </TableCell>
      </TableRow>

      <DeleteConfirmationDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        title="Delete Thread"
        description={`Are you sure you want to delete this thread? This will also delete all messages and runs associated with it. This action cannot be undone.`}
        onConfirm={async () => { await deleteMutation.mutateAsync() }}
        isPending={deleteMutation.isPending}
      />
    </>
  )
}

function ThreadListSkeleton() {
  return (
    <Card className="p-4 space-y-4">
      {[...Array(5)].map((_, i) => (
        <Skeleton key={i} className="h-12 w-full" />
      ))}
    </Card>
  )
}
