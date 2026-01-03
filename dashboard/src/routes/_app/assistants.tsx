import { createFileRoute, Outlet, useMatch } from "@tanstack/react-router"
import { PageHeader } from "@/components/layout/PageHeader"
import { AssistantList } from "@/components/assistants/AssistantList"
import { CreateAssistantDialog } from "@/components/assistants/CreateAssistantDialog"

export const Route = createFileRoute("/_app/assistants")({
  component: AssistantsLayout,
})

function AssistantsLayout() {
  const childMatch = useMatch({ from: "/_app/assistants/$assistantId", shouldThrow: false })

  if (childMatch) {
    return <Outlet />
  }

  return <AssistantsPage />
}

function AssistantsPage() {
  return (
    <div>
      <PageHeader
        title="Assistants"
        description="Manage your workflow assistants"
        actions={<CreateAssistantDialog />}
      />

      <AssistantList />
    </div>
  )
}
