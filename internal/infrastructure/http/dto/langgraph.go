package dto

import "time"

// LangGraph API DTOs for compatibility with LangGraph Cloud API

// CreateRunRequest represents the request to create a run
type CreateRunRequest struct {
	AssistantID       string                 `json:"assistant_id"`
	ThreadID          string                 `json:"thread_id,omitempty"`
	Input             map[string]interface{} `json:"input,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	Config            map[string]interface{} `json:"config,omitempty"`
	Kwargs            map[string]interface{} `json:"kwargs,omitempty"`
	StreamMode        []string               `json:"stream_mode,omitempty"`
	OnCompletion      string                 `json:"on_completion,omitempty"`
	InterruptBefore   []string               `json:"interrupt_before,omitempty"`   // Node IDs to interrupt before execution
	InterruptAfter    []string               `json:"interrupt_after,omitempty"`    // Node IDs to interrupt after execution
	Webhook           string                 `json:"webhook,omitempty"`            // Webhook URL for completion notification
	MultitaskStrategy string                 `json:"multitask_strategy,omitempty"` // Strategy for concurrent runs: reject, interrupt, rollback, enqueue
}

// CreateRunResponse represents the response from creating a run
type CreateRunResponse struct {
	RunID       string                 `json:"run_id"`
	ThreadID    string                 `json:"thread_id"`
	AssistantID string                 `json:"assistant_id"`
	Status      string                 `json:"status"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Kwargs      map[string]interface{} `json:"kwargs,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// GetRunResponse represents the response from getting a run
type GetRunResponse struct {
	RunID             string                 `json:"run_id"`
	ThreadID          string                 `json:"thread_id"`
	AssistantID       string                 `json:"assistant_id"`
	Status            string                 `json:"status"`
	Input             map[string]interface{} `json:"input,omitempty"`
	Output            map[string]interface{} `json:"output,omitempty"`
	Error             string                 `json:"error,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	Config            map[string]interface{} `json:"config,omitempty"`
	Kwargs            map[string]interface{} `json:"kwargs,omitempty"`
	MultitaskStrategy string                 `json:"multitask_strategy,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	StartedAt         *time.Time             `json:"started_at,omitempty"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// SubmitToolOutputsRequest represents the request to submit tool outputs
type SubmitToolOutputsRequest struct {
	ToolOutputs []ToolOutput `json:"tool_outputs"`
}

// ToolOutput represents a tool output
type ToolOutput struct {
	ToolCallID string `json:"tool_call_id"`
	Output     string `json:"output"`
}

// CreateAssistantRequest represents the request to create an assistant
type CreateAssistantRequest struct {
	GraphID      string                   `json:"graph_id,omitempty"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description,omitempty"`
	Model        string                   `json:"model,omitempty"`
	Instructions string                   `json:"instructions,omitempty"`
	Tools        []map[string]interface{} `json:"tools,omitempty"`
	Metadata     map[string]interface{}   `json:"metadata,omitempty"`
	Config       map[string]interface{}   `json:"config,omitempty"`
	Context      []map[string]interface{} `json:"context,omitempty"`
}

// CreateAssistantResponse represents the response from creating an assistant
type CreateAssistantResponse struct {
	AssistantID  string                   `json:"assistant_id"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description,omitempty"`
	Model        string                   `json:"model,omitempty"`
	Instructions string                   `json:"instructions,omitempty"`
	Tools        []map[string]interface{} `json:"tools,omitempty"`
	CreatedAt    time.Time                `json:"created_at"`
}

// CreateThreadRequest represents the request to create a thread
type CreateThreadRequest struct {
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// CreateThreadResponse represents the response from creating a thread
type CreateThreadResponse struct {
	ThreadID  string                 `json:"thread_id"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// UpdateAssistantRequest represents the request to update an assistant
type UpdateAssistantRequest struct {
	Name         *string                  `json:"name,omitempty"`
	Description  *string                  `json:"description,omitempty"`
	Model        *string                  `json:"model,omitempty"`
	Instructions *string                  `json:"instructions,omitempty"`
	Tools        []map[string]interface{} `json:"tools,omitempty"`
}

// AssistantResponse represents an assistant resource
type AssistantResponse struct {
	ID           string                   `json:"assistant_id"`
	GraphID      string                   `json:"graph_id,omitempty"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description,omitempty"`
	Model        string                   `json:"model,omitempty"`
	Instructions string                   `json:"instructions,omitempty"`
	Tools        []map[string]interface{} `json:"tools,omitempty"`
	Metadata     map[string]interface{}   `json:"metadata,omitempty"`
	Config       map[string]interface{}   `json:"config,omitempty"`
	Context      []map[string]interface{} `json:"context,omitempty"`
	Version      int                      `json:"version"`
	CreatedAt    int64                    `json:"created_at"`
	UpdatedAt    int64                    `json:"updated_at"`
}

// ListAssistantsResponse represents the response from listing assistants
type ListAssistantsResponse struct {
	Assistants []AssistantResponse `json:"assistants"`
	Total      int                 `json:"total"`
}

// UpdateThreadRequest represents the request to update a thread
type UpdateThreadRequest struct {
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ThreadResponse represents a thread resource
type ThreadResponse struct {
	ID        string                 `json:"id"`
	Messages  []MessageResponse      `json:"messages"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt int64                  `json:"created_at"`
	UpdatedAt int64                  `json:"updated_at"`
}

// MessageResponse represents a message in a thread
type MessageResponse struct {
	ID        string                 `json:"id"`
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt int64                  `json:"created_at"`
}

// ListThreadsResponse represents the response from listing threads
type ListThreadsResponse struct {
	Threads []ThreadResponse `json:"threads"`
	Total   int              `json:"total"`
}

// AddMessageRequest represents the request to add a message
type AddMessageRequest struct {
	Role     string                 `json:"role"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// SearchAssistantsRequest represents the request to search assistants
type SearchAssistantsRequest struct {
	GraphID  string                 `json:"graph_id,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Limit    int                    `json:"limit,omitempty"`
	Offset   int                    `json:"offset,omitempty"`
}

// CountAssistantsRequest represents the request to count assistants
type CountAssistantsRequest struct {
	GraphID  string                 `json:"graph_id,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// SearchThreadsRequest represents the request to search threads
type SearchThreadsRequest struct {
	Status   string                 `json:"status,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Limit    int                    `json:"limit,omitempty"`
	Offset   int                    `json:"offset,omitempty"`
}

// CountThreadsRequest represents the request to count threads
type CountThreadsRequest struct {
	Status   string                 `json:"status,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// CountResponse represents a count response
type CountResponse struct {
	Count int `json:"count"`
}

// ThreadStateResponse represents the current state of a thread
type ThreadStateResponse struct {
	Values       map[string]interface{}   `json:"values"`
	Next         []string                 `json:"next"`
	Tasks        []map[string]interface{} `json:"tasks,omitempty"`
	Metadata     map[string]interface{}   `json:"metadata,omitempty"`
	CreatedAt    int64                    `json:"created_at"`
	CheckpointID string                   `json:"checkpoint_id,omitempty"`
	CheckpointNS string                   `json:"checkpoint_ns,omitempty"`
	ParentConfig map[string]interface{}   `json:"parent_config,omitempty"`
}

// UpdateThreadStateRequest represents the request to update thread state
type UpdateThreadStateRequest struct {
	Values       map[string]interface{} `json:"values"`
	AsNode       string                 `json:"as_node,omitempty"`
	CheckpointNS string                 `json:"checkpoint_ns,omitempty"`
}

// ThreadHistoryEntry represents a single entry in thread history
type ThreadHistoryEntry struct {
	CheckpointID       string                 `json:"checkpoint_id"`
	ParentCheckpointID string                 `json:"parent_checkpoint_id,omitempty"`
	Values             map[string]interface{} `json:"values"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt          int64                  `json:"created_at"`
}

// GetThreadHistoryRequest represents the request to get thread history
type GetThreadHistoryRequest struct {
	CheckpointNS string `json:"checkpoint_ns,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Before       string `json:"before,omitempty"`
}

// CopyThreadRequest represents the request to copy a thread
type CopyThreadRequest struct {
	CheckpointID string `json:"checkpoint_id,omitempty"`
}

// CopyThreadResponse represents the response from copying a thread
type CopyThreadResponse struct {
	ThreadID string `json:"thread_id"`
}

// AssistantVersionResponse represents a version of an assistant
type AssistantVersionResponse struct {
	ID          string                 `json:"id"`
	AssistantID string                 `json:"assistant_id"`
	Version     int                    `json:"version"`
	GraphID     string                 `json:"graph_id,omitempty"`
	Config      map[string]interface{} `json:"config"`
	Context     []interface{}          `json:"context,omitempty"`
	CreatedAt   int64                  `json:"created_at"`
}

// CreateAssistantVersionRequest represents the request to create a new version
type CreateAssistantVersionRequest struct {
	GraphID string                 `json:"graph_id,omitempty"`
	Config  map[string]interface{} `json:"config,omitempty"`
	Context []interface{}          `json:"context,omitempty"`
}

// SetLatestVersionRequest represents the request to set the latest version
type SetLatestVersionRequest struct {
	Version int `json:"version"`
}

// AssistantSchemaResponse represents the schema of an assistant
type AssistantSchemaResponse struct {
	GraphID      string                 `json:"graph_id,omitempty"`
	InputSchema  map[string]interface{} `json:"input_schema"`
	OutputSchema map[string]interface{} `json:"output_schema"`
	StateSchema  map[string]interface{} `json:"state_schema"`
	ConfigSchema map[string]interface{} `json:"config_schema"`
}

// ResumeRunRequest represents a LangGraph-compatible resume request
// This supports the Command pattern for human-in-the-loop workflows
type ResumeRunRequest struct {
	// Input to resume with (replaces pending task input)
	Input map[string]interface{} `json:"input,omitempty"`

	// Command to execute when resuming
	Command *Command `json:"command,omitempty"`

	// Config overrides for the resumed run
	Config map[string]interface{} `json:"config,omitempty"`
}

// Command represents a LangGraph Command object for resuming with specific actions
type Command struct {
	// Resume value - the value to resume with (for interrupt_before/after)
	Resume interface{} `json:"resume,omitempty"`

	// Update state values before resuming
	Update map[string]interface{} `json:"update,omitempty"`

	// Send messages/values to specific nodes
	Send []SendMessage `json:"send,omitempty"`

	// Goto a specific node (skip current)
	Goto string `json:"goto,omitempty"`
}

// SendMessage represents a message to send to a specific node
type SendMessage struct {
	Node    string      `json:"node"`
	Message interface{} `json:"message"`
}

// InterruptResponse represents the response when a run is interrupted
type InterruptResponse struct {
	RunID         string                   `json:"run_id"`
	ThreadID      string                   `json:"thread_id"`
	Status        string                   `json:"status"`
	InterruptType string                   `json:"interrupt_type,omitempty"` // "before" or "after"
	NodeID        string                   `json:"node_id,omitempty"`
	Reason        string                   `json:"reason,omitempty"`
	State         map[string]interface{}   `json:"state,omitempty"`
	ToolCalls     []map[string]interface{} `json:"tool_calls,omitempty"`
}

// GraphNodeResponse represents a node in a graph
type GraphNodeResponse struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Config   map[string]interface{} `json:"data,omitempty"`
	Position map[string]float64     `json:"metadata,omitempty"`
}

// GraphEdgeResponse represents an edge in a graph
type GraphEdgeResponse struct {
	ID        string                 `json:"id,omitempty"`
	Source    string                 `json:"source"`
	Target    string                 `json:"target"`
	Condition map[string]interface{} `json:"data,omitempty"`
}

// GraphResponse represents the graph structure for an assistant
type GraphResponse struct {
	Nodes  []GraphNodeResponse    `json:"nodes"`
	Edges  []GraphEdgeResponse    `json:"edges"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// SubgraphInfoResponse represents information about a subgraph
type SubgraphInfoResponse struct {
	Namespace string `json:"namespace"`
	GraphID   string `json:"graph_id,omitempty"`
}
