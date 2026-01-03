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
  id: string // API returns 'id', not 'thread_id'
  messages: Message[]
  metadata?: Record<string, unknown>
  created_at: number // Unix timestamp
  updated_at: number // Unix timestamp
}

export interface ThreadsResponse {
  threads: Thread[]
  total: number
}

export interface Message {
  id: string
  role: "user" | "assistant" | "system"
  content: string
  created_at: number // Unix timestamp
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
