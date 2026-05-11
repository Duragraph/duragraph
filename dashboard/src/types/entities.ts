// Run status enum matching backend
export type RunStatus =
  | "queued"
  | "in_progress"
  | "completed"
  | "failed"
  | "cancelled"
  | "requires_action"

export interface Run {
  run_id: string
  thread_id: string
  assistant_id: string
  status: RunStatus
  input: Record<string, unknown>
  output?: Record<string, unknown>
  error?: string
  required_action?: RequiredAction
  metadata?: Record<string, unknown>
  created_at: string
  started_at?: string
  completed_at?: string
  updated_at?: string
}

export interface RequiredAction {
  type: "submit_tool_outputs"
  submit_tool_outputs: {
    tool_calls: ToolCall[]
  }
}

export interface ToolCall {
  id: string
  type: "function"
  function: {
    name: string
    arguments: string
  }
}

export interface ToolOutput {
  tool_call_id: string
  output: string
}

export interface Assistant {
  assistant_id: string
  name: string
  description?: string
  model?: string
  graph_id?: string
  config?: Record<string, unknown>
  metadata?: Record<string, unknown>
  version: number
  created_at: number // Unix timestamp
  updated_at: number // Unix timestamp
}

export interface AssistantsResponse {
  assistants: Assistant[]
  total: number
}

export interface Thread {
  thread_id: string
  metadata?: Record<string, unknown>
  status?: string
  values?: Record<string, unknown>
  created_at: string // ISO 8601
  updated_at: string // ISO 8601
  // The list endpoint does not include messages. The detail page populates
  // this client-side by fetching the thread's runs and reading
  // run.output.messages from the latest one.
  messages?: Message[]
}

export interface ThreadsResponse {
  threads: Thread[]
  total: number
}

export interface Message {
  // id + created_at are populated when a message has been persisted to
  // a thread; they're undefined for in-flight playground messages that
  // have not yet been committed (the playground emits user/assistant
  // pairs to local state before the run completes).
  id?: string
  role: "user" | "assistant" | "system" | "tool"
  content: string
  created_at?: number // Unix timestamp
  // Tool-role messages carry the tool name + the originating call ID
  // so the LLM can correlate the response to its prior tool_call.
  name?: string
  tool_call_id?: string
}

export interface Graph {
  graph_id: string
  name: string
  nodes: GraphNode[]
  edges: GraphEdge[]
  entry_point: string
}

export interface GraphNode {
  id: string
  type: "llm" | "tool" | "conditional" | "human" | "start" | "end"
  config?: Record<string, unknown>
  position?: { x: number; y: number }
}

export interface GraphEdge {
  id: string
  source: string
  target: string
  condition?: string
}

// --- Run streaming + per-node execution -------------------------------
//
// Returned from the /api/v1/stream?run_id= SSE endpoint. The shape is
// intentionally generic — the inner `data` payload differs per event
// type (`run.started`, `run.completed`, `node.started`, ...) and is
// best parsed at the consumer.

export interface RunEvent {
  event: string
  data: Record<string, unknown>
}

// NodeExecution is one step of a run's execution graph — emitted as a
// node moves through started → completed/failed. Used in the run
// inspector / reasoning-trace UI.
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

// --- Workflow editor (builder route) ----------------------------------
//
// The editor's working-state types are intentionally separate from the
// persisted `Graph` / `GraphNode` types above:
//   * `EditorNode` carries (x, y) directly while `GraphNode` uses
//     `position: { x, y }` (server canonical form).
//   * `EditorNode` has `label` + `isEntrypoint` for the in-editor UI;
//     these collapse to graph metadata on save.
// Conversion happens in the editor store's toDefinition() / loadGraph().

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

export interface GraphDefinition {
  id: string
  name: string
  description: string
  nodes: EditorNode[]
  edges: EditorEdge[]
  created_at?: string
  updated_at?: string
}

// --- Deployments (agent fleet management) -----------------------------
//
// Today the deployments view ships with mock data only — the engine
// doesn't persist Deployment yet. Keeping the type so the placeholder
// UI compiles, ready for the backend to land.
export interface Deployment {
  deployment_id: string
  assistant_id: string
  assistant_name: string
  graph_id: string
  status: "active" | "stopped" | "error" | "deploying"
  workers: number
  active_runs: number
  completed_runs: number
  failed_runs: number
  created_at: string
  updated_at: string
  config?: Record<string, unknown>
}
