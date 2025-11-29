// API Type Definitions for DuraGraph Backend

// ============================================================================
// Enums
// ============================================================================

export type RunStatus =
	| 'queued'
	| 'in_progress'
	| 'requires_action'
	| 'cancelling'
	| 'completed'
	| 'failed'
	| 'cancelled'
	| 'expired';

export type NodeType = 'start' | 'llm' | 'tool' | 'condition' | 'end' | 'subgraph' | 'human';

export type MessageRole = 'user' | 'assistant' | 'system';

// ============================================================================
// Assistant Types
// ============================================================================

export interface Tool {
	type: 'code_interpreter' | 'retrieval' | 'function';
	function?: {
		name: string;
		description: string;
		parameters: any;
	};
}

export interface Assistant {
	id: string;
	name: string;
	description: string;
	model: string;
	instructions: string;
	tools: Tool[];
	metadata: Record<string, any>;
	created_at: string;
	updated_at: string;
}

export interface CreateAssistantRequest {
	name: string;
	description?: string;
	model: string;
	instructions?: string;
	tools?: Tool[];
	metadata?: Record<string, any>;
}

export interface UpdateAssistantRequest {
	name?: string;
	description?: string;
	model?: string;
	instructions?: string;
	tools?: Tool[];
	metadata?: Record<string, any>;
}

// ============================================================================
// Thread Types
// ============================================================================

export interface MessageContent {
	type: 'text' | 'image_file' | 'image_url';
	text?: {
		value: string;
		annotations?: any[];
	};
	image_file?: {
		file_id: string;
	};
	image_url?: {
		url: string;
	};
}

export interface Message {
	id: string;
	role: MessageRole;
	content: string | MessageContent[];
	metadata?: Record<string, any>;
	file_ids?: string[];
	created_at: string;
}

export interface Thread {
	id: string;
	messages?: Message[];
	metadata?: Record<string, any>;
	assistant_id?: string;
	created_at: string;
	updated_at?: string;
}

export interface CreateThreadRequest {
	metadata?: Record<string, any>;
}

export interface UpdateThreadRequest {
	metadata?: Record<string, any>;
}

export interface AddMessageRequest {
	role: MessageRole;
	content: string;
	metadata?: Record<string, any>;
}

// ============================================================================
// Run Types
// ============================================================================

export interface ToolCall {
	id: string;
	type: 'function';
	function: {
		name: string;
		arguments: string;
	};
}

export interface RequiredAction {
	type: 'submit_tool_outputs';
	submit_tool_outputs: {
		tool_calls: ToolCall[];
	};
}

export interface Run {
	id: string;
	thread_id: string;
	assistant_id: string;
	model: string;
	status: RunStatus;
	instructions?: string;
	input?: Record<string, any>;
	output?: Record<string, any>;
	error?: string;
	required_action?: RequiredAction;
	metadata?: Record<string, any>;
	created_at: string;
	started_at?: string;
	completed_at?: string;
	updated_at?: string;
}

export interface CreateRunRequest {
	thread_id: string;
	assistant_id: string;
	input?: Record<string, any>;
	instructions?: string;
	additional_instructions?: string;
	metadata?: Record<string, any>;
}

export interface SubmitToolOutputsRequest {
	tool_outputs: Array<{
		tool_call_id: string;
		output: any;
	}>;
}

// ============================================================================
// Graph Types
// ============================================================================

export interface GraphNode {
	id: string;
	type: NodeType;
	config?: Record<string, any>;
	position?: { x: number; y: number };
}

export interface GraphEdge {
	id: string;
	source: string;
	target: string;
	sourceHandle?: string;
	targetHandle?: string;
	condition?: Record<string, any>;
}

export interface Graph {
	id: string;
	assistant_id: string;
	name: string;
	version: string;
	description: string;
	nodes: GraphNode[];
	edges: GraphEdge[];
	config: Record<string, any>;
	created_at: string;
	updated_at: string;
}

export interface SaveGraphRequest {
	assistant_id: string;
	name: string;
	version?: string;
	description?: string;
	nodes: GraphNode[];
	edges: GraphEdge[];
	config?: Record<string, any>;
}

// ============================================================================
// Event Stream Types
// ============================================================================

export interface StreamEvent {
	type: string;
	data: Record<string, any>;
	timestamp: string;
}

export interface NodeStartedEvent extends StreamEvent {
	type: 'node_started';
	data: {
		run_id: string;
		node_id: string;
		node_type: string;
		input: Record<string, any>;
	};
}

export interface NodeCompletedEvent extends StreamEvent {
	type: 'node_completed';
	data: {
		run_id: string;
		node_id: string;
		node_type: string;
		output: Record<string, any>;
		duration_ms: number;
	};
}

export interface NodeFailedEvent extends StreamEvent {
	type: 'node_failed';
	data: {
		run_id: string;
		node_id: string;
		node_type: string;
		error: string;
	};
}

// ============================================================================
// API Response Types
// ============================================================================

export interface HealthResponse {
	status: string;
	version: string;
}

export interface ErrorResponse {
	error: string;
	message: string;
}

export interface ListResponse<T> {
	items: T[];
	total: number;
	limit: number;
	offset: number;
}
