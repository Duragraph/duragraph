import { createFileRoute, Outlet, useMatch } from "@tanstack/react-router"
import { PageHeader } from "@/components/layout/PageHeader"
import { ThreadList } from "@/components/threads/ThreadList"
import { CreateThreadDialog } from "@/components/threads/CreateThreadDialog"

export const Route = createFileRoute("/_app/threads")({
  component: ThreadsLayout,
})

function ThreadsLayout() {
  const childMatch = useMatch({ from: "/_app/threads/$threadId", shouldThrow: false })

  if (childMatch) {
    return <Outlet />
  }

  return <ThreadsPage />
}

function ThreadsPage() {
  return (
    <div>
      <PageHeader
        title="Threads"
        description="Manage conversation threads"
        actions={<CreateThreadDialog />}
      />

      <ThreadList />
    </div>
  )
}
