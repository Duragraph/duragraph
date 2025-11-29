package dto

import "time"

// LangGraph API DTOs for compatibility with LangGraph Cloud API

// CreateRunRequest represents the request to create a run
type CreateRunRequest struct {
	AssistantID string                 `json:"assistant_id"`
	ThreadID    string                 `json:"thread_id,omitempty"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// CreateRunResponse represents the response from creating a run
type CreateRunResponse struct {
	RunID       string                 `json:"run_id"`
	ThreadID    string                 `json:"thread_id"`
	AssistantID string                 `json:"assistant_id"`
	Status      string                 `json:"status"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// GetRunResponse represents the response from getting a run
type GetRunResponse struct {
	RunID       string                 `json:"run_id"`
	ThreadID    string                 `json:"thread_id"`
	AssistantID string                 `json:"assistant_id"`
	Status      string                 `json:"status"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at"`
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
	Name         string                   `json:"name"`
	Description  string                   `json:"description,omitempty"`
	Model        string                   `json:"model,omitempty"`
	Instructions string                   `json:"instructions,omitempty"`
	Tools        []map[string]interface{} `json:"tools,omitempty"`
	Metadata     map[string]interface{}   `json:"metadata,omitempty"`
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
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description,omitempty"`
	Model        string                   `json:"model,omitempty"`
	Instructions string                   `json:"instructions,omitempty"`
	Tools        []map[string]interface{} `json:"tools,omitempty"`
	Metadata     map[string]interface{}   `json:"metadata,omitempty"`
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
