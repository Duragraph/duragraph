package dto

import "time"

// Worker Protocol DTOs

// RegisterWorkerRequest represents the request to register a worker
type RegisterWorkerRequest struct {
	WorkerID         string             `json:"worker_id"`
	Name             string             `json:"name"`
	Capabilities     WorkerCapabilities `json:"capabilities"`
	GraphDefinitions []GraphDefinition  `json:"graph_definitions,omitempty"`
}

// WorkerCapabilities represents what a worker can do
type WorkerCapabilities struct {
	Graphs            []string `json:"graphs"`
	MaxConcurrentRuns int      `json:"max_concurrent_runs"`
}

// GraphDefinition represents a graph that a worker can execute
type GraphDefinition struct {
	GraphID     string           `json:"graph_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Nodes       []NodeDefinition `json:"nodes"`
	Edges       []EdgeDefinition `json:"edges"`
	EntryPoint  string           `json:"entry_point"`
}

// NodeDefinition represents a node in a graph
type NodeDefinition struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// EdgeDefinition represents an edge in a graph
type EdgeDefinition struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Condition string `json:"condition,omitempty"`
}

// RegisterWorkerResponse represents the response from registering a worker
type RegisterWorkerResponse struct {
	WorkerID       string `json:"worker_id"`
	Registered     bool   `json:"registered"`
	HeartbeatURL   string `json:"heartbeat_url"`
	PollURL        string `json:"poll_url"`
	DeregisterURL  string `json:"deregister_url"`
	EventStreamURL string `json:"event_stream_url"`
}

// HeartbeatRequest represents the request to send a heartbeat
type HeartbeatRequest struct {
	Status     string `json:"status"`
	ActiveRuns int    `json:"active_runs"`
	TotalRuns  int    `json:"total_runs"`
	FailedRuns int    `json:"failed_runs"`
}

// HeartbeatResponse represents the response from a heartbeat
type HeartbeatResponse struct {
	Acknowledged bool `json:"acknowledged"`
}

// PollRequest represents the request to poll for work
type PollRequest struct {
	MaxTasks int `json:"max_tasks,omitempty"`
}

// PollResponse represents the response from polling for work
type PollResponse struct {
	Tasks []WorkerTask `json:"tasks"`
}

// WorkerTask represents a task assigned to a worker
type WorkerTask struct {
	TaskID      string                 `json:"task_id"`
	RunID       string                 `json:"run_id"`
	ThreadID    string                 `json:"thread_id"`
	AssistantID string                 `json:"assistant_id"`
	GraphID     string                 `json:"graph_id"`
	Input       map[string]interface{} `json:"input"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Checkpoint  map[string]interface{} `json:"checkpoint,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// DeregisterWorkerResponse represents the response from deregistering a worker
type DeregisterWorkerResponse struct {
	WorkerID     string `json:"worker_id"`
	Deregistered bool   `json:"deregistered"`
}

// WorkerStatusResponse represents the status of a worker
type WorkerStatusResponse struct {
	WorkerID      string    `json:"worker_id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	ActiveRuns    int       `json:"active_runs"`
	TotalRuns     int       `json:"total_runs"`
	FailedRuns    int       `json:"failed_runs"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	RegisteredAt  time.Time `json:"registered_at"`
	Graphs        []string  `json:"graphs"`
}

// ListWorkersResponse represents the response from listing workers
type ListWorkersResponse struct {
	Workers []WorkerStatusResponse `json:"workers"`
	Total   int                    `json:"total"`
}

// WorkerEventRequest represents an event sent from a worker
type WorkerEventRequest struct {
	RunID     string                 `json:"run_id"`
	EventType string                 `json:"event_type"`
	NodeID    string                 `json:"node_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// WorkerEventResponse represents the response from sending an event
type WorkerEventResponse struct {
	Received bool `json:"received"`
}
