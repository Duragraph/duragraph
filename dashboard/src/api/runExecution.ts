import type { Graph } from "@/types/entities"

type ObjectValue = Record<string, unknown>
const record = (value: unknown): value is ObjectValue => typeof value === "object" && value !== null && !Array.isArray(value)

// GraphSchema documents state_schema as an object. The current control plane
// embeds nodes/edges there; other implementations may return only JSON schema.
// Never assume every GraphSchema contains visualizable topology.
export function graphTopology(schema: unknown): Graph | null {
  if (!record(schema) || !record(schema.state_schema)) return null
  const { nodes, edges } = schema.state_schema
  if (!Array.isArray(nodes) || !Array.isArray(edges)) return null
  if (!nodes.every(n => record(n) && typeof n.id === "string" && typeof n.type === "string") ||
      !edges.every(e => record(e) && typeof e.source === "string" && typeof e.target === "string")) return null
  return { nodes: nodes as Graph["nodes"], edges: edges as Graph["edges"] }
}

export type NodeEventStatus = "running" | "completed" | "error"
export interface ExecutionRecord {
  nodeId: string
  status: NodeEventStatus
  input?: unknown
  output?: unknown
  error?: string
  durationMs?: number
}
export interface RunExecution {
  nodes: Record<string, ExecutionRecord>
  input?: unknown
  output?: unknown
  error?: string
}
export const emptyExecution = (): RunExecution => ({ nodes: {} })

// The v2 SSE endpoint emits event: execution.node_* / run.* and data: JSON.
// Only payloads actually observed on this run populate execution detail.
export function applyExecutionEvent(previous: RunExecution, event: string, payload: unknown): RunExecution {
  if (!record(payload)) return previous
  if (event === "run.created" && "input" in payload) return { ...previous, input: payload.input }
  if (event === "run.completed" && "output" in payload) return { ...previous, output: payload.output }
  if (event === "run.failed" && typeof payload.error === "string") return { ...previous, error: payload.error }
  const statuses: Record<string, NodeEventStatus> = {
    "execution.node_started": "running",
    "execution.node_completed": "completed",
    "execution.node_failed": "error",
  }
  const status = statuses[event]
  const nodeId = payload.node_id ?? payload.node
  if (!status || typeof nodeId !== "string") return previous
  const before = previous.nodes[nodeId]
  return {
    ...previous,
    nodes: {
      ...previous.nodes,
      [nodeId]: {
        ...before,
        nodeId,
        status,
        ...( "input" in payload ? { input: payload.input } : {}),
        ...( "output" in payload ? { output: payload.output } : {}),
        ...(typeof payload.error === "string" ? { error: payload.error } : {}),
        ...(typeof payload.duration_ms === "number" ? { durationMs: payload.duration_ms } : {}),
      },
    },
  }
}
