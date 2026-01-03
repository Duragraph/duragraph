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
import { Badge } from "@/components/ui/badge"
import { RunStatusBadge } from "./RunStatusBadge"
import { MoreHorizontal, Eye, Play, Radio, Trash2, XCircle } from "lucide-react"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { DeleteConfirmationDialog } from "@/components/ui/delete-confirmation-dialog"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/api/client"
import type { Run } from "@/types/entities"
import { useRunsStream } from "@/hooks/useRunStream"
import { SearchFilter } from "@/components/common/SearchFilter"
import { toast } from "sonner"

interface RunTableProps {
  showLiveIndicator?: boolean
  showFilters?: boolean
}

const STATUS_OPTIONS = [
  { value: "queued", label: "Queued" },
  { value: "in_progress", label: "In Progress" },
  { value: "completed", label: "Completed" },
  { value: "failed", label: "Failed" },
  { value: "cancelled", label: "Cancelled" },
  { value: "requires_action", label: "Requires Action" },
]

export function RunTable({ showLiveIndicator = true, showFilters = true }: RunTableProps) {
  const [search, setSearch] = useState("")
  const [statusFilter, setStatusFilter] = useState("all")

  const { data: runs, isLoading, error } = useQuery({
    queryKey: ["runs"],
    queryFn: () => api.get<Run[]>("/runs"),
    // Refetch more frequently when there are in-progress runs
    refetchInterval: (query) => {
      const data = query.state.data
      const hasActiveRuns = data?.some(
        (run) => run.status === "in_progress" || run.status === "queued"
      )
      return hasActiveRuns ? 3000 : false
    },
  })

  // Enable streaming/polling for real-time updates
  const hasActiveRuns = runs?.some(
    (run) => run.status === "in_progress" || run.status === "queued"
  )
  useRunsStream({ enabled: hasActiveRuns })

  // Filter runs based on search and status
  const filteredRuns = useMemo(() => {
    if (!runs) return []
    return runs.filter((run) => {
      const matchesSearch =
        !search ||
        run.run_id.toLowerCase().includes(search.toLowerCase()) ||
        run.assistant_id.toLowerCase().includes(search.toLowerCase()) ||
        run.thread_id.toLowerCase().includes(search.toLowerCase())
      const matchesStatus =
        statusFilter === "all" || run.status === statusFilter
      return matchesSearch && matchesStatus
    })
  }, [runs, search, statusFilter])

  const handleClearFilters = () => {
    setSearch("")
    setStatusFilter("all")
  }

  if (isLoading) {
    return <RunTableSkeleton />
  }

  if (error) {
    return (
      <Card className="p-6">
        <p className="text-sm text-red-600">
          Failed to load runs: {error.message}
        </p>
      </Card>
    )
  }

  if (!runs || runs.length === 0) {
    return (
      <Card className="p-12 text-center">
        <Play className="h-12 w-12 mx-auto text-muted-foreground/30 mb-4" />
        <h3 className="text-lg font-semibold mb-2">No runs found</h3>
        <p className="text-muted-foreground">
          Create an assistant and start a new run.
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
          searchPlaceholder="Search runs, assistants, threads..."
          filters={[
            {
              id: "status",
              label: "Status",
              value: statusFilter,
              options: STATUS_OPTIONS,
              onChange: setStatusFilter,
            },
          ]}
          onClear={handleClearFilters}
        />
      )}

      <Card>
        {showLiveIndicator && hasActiveRuns && (
          <div className="px-4 py-2 border-b bg-muted/30 flex items-center gap-2">
            <Badge variant="outline" className="text-green-600 border-green-600 gap-1">
              <Radio className="h-3 w-3 animate-pulse" />
              Live
            </Badge>
            <span className="text-sm text-muted-foreground">
              Auto-refreshing while runs are in progress
            </span>
          </div>
        )}
        {filteredRuns.length === 0 ? (
          <div className="p-12 text-center">
            <Play className="h-12 w-12 mx-auto text-muted-foreground/30 mb-4" />
            <h3 className="text-lg font-semibold mb-2">No matching runs</h3>
            <p className="text-muted-foreground">
              Try adjusting your search or filters.
            </p>
            <Button variant="outline" className="mt-4" onClick={handleClearFilters}>
              Clear Filters
            </Button>
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[140px]">Status</TableHead>
                <TableHead className="w-[200px]">Run ID</TableHead>
                <TableHead>Assistant</TableHead>
                <TableHead className="w-[200px]">Thread</TableHead>
                <TableHead className="w-[160px]">Created</TableHead>
                <TableHead className="w-[60px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredRuns.map((run) => (
                <RunRow key={run.run_id} run={run} />
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
    </div>
  )
}

interface RunRowProps {
  run: Run
}

function RunRow({ run }: RunRowProps) {
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const queryClient = useQueryClient()

  const deleteMutation = useMutation({
    mutationFn: () => api.delete(`/threads/${run.thread_id}/runs/${run.run_id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["runs"] })
      toast.success("Run deleted successfully")
      setDeleteDialogOpen(false)
    },
    onError: (error: Error) => {
      toast.error(`Failed to delete run: ${error.message}`)
    },
  })

  const cancelMutation = useMutation({
    mutationFn: () => api.post(`/runs/${run.run_id}/cancel`, {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["runs"] })
      toast.success("Run cancelled")
    },
    onError: (error: Error) => {
      toast.error(`Failed to cancel run: ${error.message}`)
    },
  })

  const canCancel = run.status === "in_progress" || run.status === "queued"

  return (
    <>
      <TableRow className="cursor-pointer hover:bg-muted/50">
        <TableCell>
          <RunStatusBadge status={run.status} />
        </TableCell>
        <TableCell>
          <Link
            to="/runs/$runId"
            params={{ runId: run.run_id }}
            className="font-mono text-sm hover:underline"
          >
            {run.run_id.slice(0, 8)}...
          </Link>
        </TableCell>
        <TableCell>
          <Link
            to="/assistants/$assistantId"
            params={{ assistantId: run.assistant_id }}
            className="text-sm hover:underline"
          >
            {run.assistant_id.slice(0, 8)}...
          </Link>
        </TableCell>
        <TableCell>
          <Link
            to="/threads/$threadId"
            params={{ threadId: run.thread_id }}
            className="font-mono text-sm hover:underline"
          >
            {run.thread_id.slice(0, 8)}...
          </Link>
        </TableCell>
        <TableCell className="text-muted-foreground">
          {new Date(run.created_at).toLocaleString()}
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
                <Link to="/runs/$runId" params={{ runId: run.run_id }}>
                  <Eye className="h-4 w-4 mr-2" />
                  View Details
                </Link>
              </DropdownMenuItem>
              {canCancel && (
                <DropdownMenuItem onClick={() => cancelMutation.mutate()}>
                  <XCircle className="h-4 w-4 mr-2" />
                  Cancel Run
                </DropdownMenuItem>
              )}
              <DropdownMenuSeparator />
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
        title="Delete Run"
        description="Are you sure you want to delete this run? This action cannot be undone."
        onConfirm={async () => { await deleteMutation.mutateAsync() }}
        isPending={deleteMutation.isPending}
      />
    </>
  )
}

function RunTableSkeleton() {
  return (
    <Card className="p-4 space-y-4">
      {[...Array(5)].map((_, i) => (
        <Skeleton key={i} className="h-12 w-full" />
      ))}
    </Card>
  )
}
