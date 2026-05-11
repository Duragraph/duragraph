import { useState } from "react"
import { createFileRoute } from "@tanstack/react-router"
import { useAssistants } from "@/api/assistants"
import type { Deployment } from "@/types/entities"

// Deployments is the agent fleet management view ported from studio.
// The Deployment shape is real but the engine doesn't persist
// deployments yet — MOCK_DEPLOYMENTS keeps the page renderable until
// the backend lands. When that happens, swap the const for a
// `useDeployments()` query hook.

export const Route = createFileRoute("/_app/deployments")({
  component: DeploymentsPage,
})

const MOCK_DEPLOYMENTS: Deployment[] = [
  {
    deployment_id: "dep-001",
    assistant_id: "a-123",
    assistant_name: "Support Bot",
    graph_id: "support_v2",
    status: "active",
    workers: 3,
    active_runs: 12,
    completed_runs: 1847,
    failed_runs: 23,
    created_at: "2026-04-01T10:00:00Z",
    updated_at: "2026-04-08T14:30:00Z",
  },
  {
    deployment_id: "dep-002",
    assistant_id: "a-456",
    assistant_name: "Data Pipeline",
    graph_id: "etl_agent",
    status: "active",
    workers: 1,
    active_runs: 2,
    completed_runs: 542,
    failed_runs: 8,
    created_at: "2026-03-15T08:00:00Z",
    updated_at: "2026-04-08T12:00:00Z",
  },
]

const STATUS_STYLES: Record<string, string> = {
  active: "bg-green-100 text-green-700 border-green-300",
  stopped: "bg-muted text-muted-foreground border-border",
  error: "bg-red-100 text-red-700 border-red-300",
  deploying: "bg-yellow-100 text-yellow-700 border-yellow-300",
}

function DeploymentsPage() {
  const { data: assistants } = useAssistants()
  const [deployments] = useState<Deployment[]>(MOCK_DEPLOYMENTS)
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const selected = selectedId
    ? deployments.find((d) => d.deployment_id === selectedId)
    : null

  return (
    <div className="flex h-full -m-6">
      <div className="flex-1 flex flex-col overflow-hidden">
        <div className="flex items-center justify-between border-b border-border bg-card px-6 py-3">
          <div>
            <h2 className="text-sm font-semibold">Deployments</h2>
            <p className="text-xs text-muted-foreground">
              {deployments.length} deployment
              {deployments.length !== 1 ? "s" : ""}
              {" · "}
              {assistants?.length ?? 0} assistant
              {(assistants?.length ?? 0) !== 1 ? "s" : ""} available
            </p>
          </div>
          <button className="border border-input bg-primary text-primary-foreground px-4 py-1.5 text-xs font-medium hover:opacity-90">
            New Deployment
          </button>
        </div>

        <div className="flex-1 overflow-y-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50 text-left text-xs font-medium text-muted-foreground">
                <th className="px-6 py-2.5">Assistant</th>
                <th className="px-4 py-2.5">Graph</th>
                <th className="px-4 py-2.5">Status</th>
                <th className="px-4 py-2.5 text-right">Workers</th>
                <th className="px-4 py-2.5 text-right">Active</th>
                <th className="px-4 py-2.5 text-right">Completed</th>
                <th className="px-4 py-2.5 text-right">Failed</th>
                <th className="px-6 py-2.5">Updated</th>
              </tr>
            </thead>
            <tbody>
              {deployments.map((dep) => (
                <tr
                  key={dep.deployment_id}
                  onClick={() => setSelectedId(dep.deployment_id)}
                  className={`border-b border-border cursor-pointer transition-colors ${
                    selectedId === dep.deployment_id
                      ? "bg-accent"
                      : "hover:bg-muted/30"
                  }`}
                >
                  <td className="px-6 py-3">
                    <div className="font-medium">{dep.assistant_name}</div>
                    <div className="text-xs text-muted-foreground font-mono">
                      {dep.assistant_id.slice(0, 12)}
                    </div>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs">{dep.graph_id}</td>
                  <td className="px-4 py-3">
                    <span
                      className={`inline-block border px-2 py-0.5 text-xs font-medium ${
                        STATUS_STYLES[dep.status] ?? ""
                      }`}
                    >
                      {dep.status}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right font-mono">
                    {dep.workers}
                  </td>
                  <td className="px-4 py-3 text-right font-mono">
                    {dep.active_runs}
                  </td>
                  <td className="px-4 py-3 text-right font-mono">
                    {dep.completed_runs.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right font-mono">
                    {dep.failed_runs}
                  </td>
                  <td className="px-6 py-3 text-xs text-muted-foreground">
                    {new Date(dep.updated_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {selected && (
        <div className="w-80 border-l border-border bg-card flex flex-col overflow-y-auto">
          <div className="border-b border-border p-4">
            <h3 className="text-sm font-semibold">{selected.assistant_name}</h3>
            <p className="text-xs text-muted-foreground font-mono mt-0.5">
              {selected.deployment_id}
            </p>
          </div>

          <div className="p-4 space-y-4">
            <div>
              <label className="text-xs font-medium text-muted-foreground">
                Status
              </label>
              <div className="mt-1">
                <span
                  className={`inline-block border px-2 py-0.5 text-xs font-medium ${
                    STATUS_STYLES[selected.status] ?? ""
                  }`}
                >
                  {selected.status}
                </span>
              </div>
            </div>

            <div>
              <label className="text-xs font-medium text-muted-foreground">
                Graph
              </label>
              <div className="mt-1 text-sm font-mono">{selected.graph_id}</div>
            </div>

            <div>
              <label className="text-xs font-medium text-muted-foreground">
                Workers
              </label>
              <div className="mt-1 text-sm font-mono">{selected.workers}</div>
            </div>

            <div className="grid grid-cols-3 gap-3">
              <div className="border border-border p-2">
                <div className="text-xs text-muted-foreground">Active</div>
                <div className="text-lg font-semibold font-mono">
                  {selected.active_runs}
                </div>
              </div>
              <div className="border border-border p-2">
                <div className="text-xs text-muted-foreground">Done</div>
                <div className="text-lg font-semibold font-mono text-green-600">
                  {selected.completed_runs.toLocaleString()}
                </div>
              </div>
              <div className="border border-border p-2">
                <div className="text-xs text-muted-foreground">Failed</div>
                <div className="text-lg font-semibold font-mono text-red-600">
                  {selected.failed_runs}
                </div>
              </div>
            </div>

            <div>
              <label className="text-xs font-medium text-muted-foreground">
                Created
              </label>
              <div className="mt-1 text-xs text-muted-foreground">
                {new Date(selected.created_at).toLocaleString()}
              </div>
            </div>

            <div>
              <label className="text-xs font-medium text-muted-foreground">
                Last Updated
              </label>
              <div className="mt-1 text-xs text-muted-foreground">
                {new Date(selected.updated_at).toLocaleString()}
              </div>
            </div>

            <div className="pt-2 space-y-2">
              {selected.status === "active" ? (
                <button className="w-full border border-destructive text-destructive px-3 py-1.5 text-xs hover:bg-destructive hover:text-destructive-foreground">
                  Stop Deployment
                </button>
              ) : (
                <button className="w-full border border-input bg-primary text-primary-foreground px-3 py-1.5 text-xs hover:opacity-90">
                  Start Deployment
                </button>
              )}
              <button className="w-full border border-input bg-background px-3 py-1.5 text-xs hover:bg-accent">
                Scale Workers
              </button>
              <button className="w-full border border-input bg-background px-3 py-1.5 text-xs hover:bg-accent">
                View Logs
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
