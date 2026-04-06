export interface Assistant {
  assistant_id: string
  graph_id: string
  name: string
  description?: string
  config?: Record<string, unknown>
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface Thread {
  thread_id: string
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface Message {
  role: 'user' | 'assistant' | 'system' | 'tool'
  content: string
  name?: string
  tool_call_id?: string
}

export type RunStatus =
  | 'queued'
  | 'in_progress'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'requires_action'

export interface Run {
  run_id: string
  thread_id: string
  assistant_id: string
  status: RunStatus
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  error?: string
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface RunEvent {
  event: string
  data: Record<string, unknown>
}

export interface NodeExecution {
  node_id: string
  node_type: string
  status: 'started' | 'completed' | 'failed'
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  started_at?: string
  completed_at?: string
  duration_ms?: number
  error?: string
}
