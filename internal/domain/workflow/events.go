package workflow

import "time"

// Assistant events
const (
	EventTypeAssistantCreated = "assistant.created"
	EventTypeAssistantUpdated = "assistant.updated"
	EventTypeAssistantDeleted = "assistant.deleted"
)

// AssistantCreated event
type AssistantCreated struct {
	AssistantID  string                   `json:"assistant_id"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description,omitempty"`
	Model        string                   `json:"model,omitempty"`
	Instructions string                   `json:"instructions,omitempty"`
	Tools        []map[string]interface{} `json:"tools,omitempty"`
	Metadata     map[string]interface{}   `json:"metadata,omitempty"`
	OccurredAt   time.Time                `json:"occurred_at"`
}

func (e AssistantCreated) EventType() string     { return EventTypeAssistantCreated }
func (e AssistantCreated) AggregateID() string   { return e.AssistantID }
func (e AssistantCreated) AggregateType() string { return "assistant" }

// AssistantUpdated event
type AssistantUpdated struct {
	AssistantID  string                   `json:"assistant_id"`
	Name         *string                  `json:"name,omitempty"`
	Description  *string                  `json:"description,omitempty"`
	Model        *string                  `json:"model,omitempty"`
	Instructions *string                  `json:"instructions,omitempty"`
	Tools        []map[string]interface{} `json:"tools,omitempty"`
	Metadata     map[string]interface{}   `json:"metadata,omitempty"`
	OccurredAt   time.Time                `json:"occurred_at"`
}

func (e AssistantUpdated) EventType() string     { return EventTypeAssistantUpdated }
func (e AssistantUpdated) AggregateID() string   { return e.AssistantID }
func (e AssistantUpdated) AggregateType() string { return "assistant" }

// AssistantDeleted event
type AssistantDeleted struct {
	AssistantID string    `json:"assistant_id"`
	OccurredAt  time.Time `json:"occurred_at"`
}

func (e AssistantDeleted) EventType() string     { return EventTypeAssistantDeleted }
func (e AssistantDeleted) AggregateID() string   { return e.AssistantID }
func (e AssistantDeleted) AggregateType() string { return "assistant" }

// Thread events
const (
	EventTypeThreadCreated = "thread.created"
	EventTypeThreadUpdated = "thread.updated"
	EventTypeMessageAdded  = "thread.message_added"
)

// ThreadCreated event
type ThreadCreated struct {
	ThreadID   string                 `json:"thread_id"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
}

func (e ThreadCreated) EventType() string     { return EventTypeThreadCreated }
func (e ThreadCreated) AggregateID() string   { return e.ThreadID }
func (e ThreadCreated) AggregateType() string { return "thread" }

// ThreadUpdated event
type ThreadUpdated struct {
	ThreadID   string                 `json:"thread_id"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
}

func (e ThreadUpdated) EventType() string     { return EventTypeThreadUpdated }
func (e ThreadUpdated) AggregateID() string   { return e.ThreadID }
func (e ThreadUpdated) AggregateType() string { return "thread" }

// MessageAdded event
type MessageAdded struct {
	MessageID  string                 `json:"message_id"`
	ThreadID   string                 `json:"thread_id"`
	Role       string                 `json:"role"`
	Content    string                 `json:"content"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
}

func (e MessageAdded) EventType() string     { return EventTypeMessageAdded }
func (e MessageAdded) AggregateID() string   { return e.ThreadID }
func (e MessageAdded) AggregateType() string { return "thread" }

// Graph events
const (
	EventTypeGraphDefined = "graph.defined"
	EventTypeGraphUpdated = "graph.updated"
)

// GraphDefined event
type GraphDefined struct {
	GraphID     string                 `json:"graph_id"`
	AssistantID string                 `json:"assistant_id"`
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description,omitempty"`
	Nodes       []Node                 `json:"nodes"`
	Edges       []Edge                 `json:"edges"`
	Config      map[string]interface{} `json:"config,omitempty"`
	OccurredAt  time.Time              `json:"occurred_at"`
}

func (e GraphDefined) EventType() string     { return EventTypeGraphDefined }
func (e GraphDefined) AggregateID() string   { return e.GraphID }
func (e GraphDefined) AggregateType() string { return "graph" }

// GraphUpdated event
type GraphUpdated struct {
	GraphID     string                 `json:"graph_id"`
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Nodes       []Node                 `json:"nodes,omitempty"`
	Edges       []Edge                 `json:"edges,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	OccurredAt  time.Time              `json:"occurred_at"`
}

func (e GraphUpdated) EventType() string     { return EventTypeGraphUpdated }
func (e GraphUpdated) AggregateID() string   { return e.GraphID }
func (e GraphUpdated) AggregateType() string { return "graph" }
