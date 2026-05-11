// entities.ts holds the SHRINKING set of frontend-only types that
// don't have a Go DTO counterpart. Anything that crosses the wire to
// the engine must come from `@/types/generated` (regenerated via
// `task gen:types` against internal/infrastructure/http/dto).
//
// What lives here:
//   * Domain enums that the engine returns as bare `string` in its
//     DTOs but that we narrow on the frontend (RunStatus). When the
//     Go DTOs adopt typed status fields, move these out.
//   * SSE / NATS event payloads (RunEvent, NodeExecution). Not in
//     the REST DTOs; until those events get their own Go DTO module
//     they stay here.
//   * Editor working-state types (EditorNode, EditorEdge,
//     GraphDefinitionLocal, EditorNodeType). Local zustand shape
//     only — never crosses the wire. The wire-canonical equivalents
//     (GraphDefinition / NodeDefinition / EdgeDefinition) live in
//     generated.ts and are what worker registration uses.
//
// Convenience re-exports (Run, Assistant, Thread, Message, Graph,
// ToolOutput) point at the generated DTO shapes so existing imports
// `from "@/types/entities"` keep working during the migration. New
// code should import from "@/types/generated" directly.

import type {
  GetRunResponse,
  AssistantResponse,
  ListAssistantsResponse,
  ThreadResponse,
  ListThreadsResponse,
  MessageResponse,
  GraphResponse,
  GraphNodeResponse,
  GraphEdgeResponse,
  ToolOutput as GeneratedToolOutput,
} from "./generated"

// --- Wire types: re-exported as the legacy frontend names ----------
// Drop these aliases when every import site has migrated to the
// generated names. Until then, this preserves the route files.
//
// Some aliases extend the generated DTO with frontend-only fields
// where the engine's wire shape has a known gap:
//   * Thread.messages — the list endpoint doesn't return messages;
//     /threads/$threadId hydrates this client-side by reading
//     run.output.messages from the latest run.
//   * Message.id/created_at — optimistic playground messages don't
//     have these until the engine echoes the persisted record back.
//   * Message.name/tool_call_id — tool-role payload fields not in
//     the current Go DTO; populated client-side from SSE events.

export type Run = GetRunResponse
export type Assistant = AssistantResponse
export type AssistantsResponse = ListAssistantsResponse
export type Thread = ThreadResponse & {
  messages?: Message[]
}
export type ThreadsResponse = ListThreadsResponse
export type Message = Omit<MessageResponse, "id" | "created_at"> & {
  id?: string
  created_at?: number
  name?: string
  tool_call_id?: string
}
// Graph / GraphNode / GraphEdge override:
//
//   * Graph.entry_point — present on the wire payload but missing
//     from the current Go DTO (gap to close: add to
//     internal/infrastructure/http/dto/langgraph.go's GraphResponse).
//   * GraphNode.position + GraphNode.config — same gap; the
//     dashboard's xyflow visualisation needs (x, y) and node-level
//     configuration.
//   * GraphEdge.condition — same gap; conditional branches in the
//     domain Graph carry a `condition` field that GraphEdgeResponse
//     doesn't yet expose.
//
// These are typed-superset overrides so callers compile today.
// They will go away as soon as the Go DTO is brought in line with
// the wire shape and `task gen:types` regenerates.
export type GraphNode = GraphNodeResponse & {
  position?: { x: number; y: number }
  config?: Record<string, unknown>
}
export type GraphEdge = GraphEdgeResponse & {
  condition?: string
}
// Graph overrides `nodes` and `edges` so the union with frontend-only
// fields propagates down — `GraphResponse.nodes` is typed as
// `GraphNodeResponse[]`, but consumers expect the augmented
// `GraphNode[]` with `position` + `config`.
export type Graph = Omit<GraphResponse, "nodes" | "edges"> & {
  entry_point?: string
  nodes: GraphNode[]
  edges: GraphEdge[]
}
export type ToolOutput = GeneratedToolOutput

// --- Run status enum -----------------------------------------------
//
// The engine's Run DTO emits status as a bare `string`. The domain
// (internal/domain/run/run.go) actually has these six values, so we
// narrow on the frontend. Cast at the route layer:
//   const status = response.status as RunStatus

export type RunStatus =
  | "queued"
  | "in_progress"
  | "completed"
  | "failed"
  | "cancelled"
  | "requires_action"

// --- Required-action / tool-call shapes ----------------------------
//
// LangGraph-compat: when a run needs a human-in-the-loop tool output,
// the engine returns a `required_action` payload. This isn't in the
// Go DTO yet (gap to close on the backend); when it lands, move
// these into generated.ts.

export interface ToolCall {
  id: string
  type: "function"
  function: {
    name: string
    arguments: string
  }
}

export interface RequiredAction {
  type: "submit_tool_outputs"
  submit_tool_outputs: {
    tool_calls: ToolCall[]
  }
}

// --- SSE / NATS event payloads -------------------------------------
//
// Emitted by the /api/v1/stream and /api/v1/threads/.../runs/.../stream
// endpoints. Not present in the REST DTOs — they live in the
// `internal/infrastructure/messaging` events package. AsyncAPI codegen
// is a follow-up; until then, hand-roll the minimum here.

export interface RunEvent {
  event: string
  data: Record<string, unknown>
}

export interface NodeExecution {
  node_id: string
  node_type: string
  status: "started" | "completed" | "failed"
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  started_at?: string
  completed_at?: string
  duration_ms?: number
  error?: string
}

// --- Editor working-state types ------------------------------------
//
// Used only by the workflow builder's zustand store
// (src/stores/editor.ts). The persisted form crosses the wire as
// `GraphDefinition` (in generated.ts) — these types are the in-editor
// representation only.

export type EditorNodeType =
  | "function"
  | "llm"
  | "tool"
  | "router"
  | "human"
  | "subgraph"

export interface EditorNode {
  id: string
  type: EditorNodeType
  label: string
  x: number
  y: number
  config: Record<string, unknown>
  isEntrypoint?: boolean
}

export interface EditorEdge {
  id: string
  source: string
  target: string
  label?: string
}

// GraphDefinitionLocal is the editor's working-graph shape (uses
// EditorNode/EditorEdge with `x`/`y` flat fields). The wire-canonical
// `GraphDefinition` (in generated.ts) uses `NodeDefinition` with a
// `position: { x, y }` object. Convert at save time in the editor
// store's `toDefinition()`.
export interface GraphDefinitionLocal {
  id: string
  name: string
  description: string
  nodes: EditorNode[]
  edges: EditorEdge[]
  created_at?: string
  updated_at?: string
}
