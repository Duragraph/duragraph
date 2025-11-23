package workflow

import (
	"time"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

// NodeType represents the type of a graph node
type NodeType string

const (
	NodeTypeStart     NodeType = "start"
	NodeTypeLLM       NodeType = "llm"
	NodeTypeTool      NodeType = "tool"
	NodeTypeCondition NodeType = "condition"
	NodeTypeEnd       NodeType = "end"
	NodeTypeSubgraph  NodeType = "subgraph"
	NodeTypeHuman     NodeType = "human"
)

// Node represents a node in the graph
type Node struct {
	ID       string                 `json:"id"`
	Type     NodeType               `json:"type"`
	Config   map[string]interface{} `json:"config,omitempty"`
	Position map[string]float64     `json:"position,omitempty"` // For UI
}

// Edge represents an edge in the graph
type Edge struct {
	ID        string                 `json:"id"`
	Source    string                 `json:"source"`
	Target    string                 `json:"target"`
	Condition map[string]interface{} `json:"condition,omitempty"` // For conditional edges
}

// Graph represents a workflow graph aggregate
type Graph struct {
	id          string
	assistantID string
	name        string
	version     string
	description string
	nodes       []Node
	edges       []Edge
	config      map[string]interface{}
	createdAt   time.Time
	updatedAt   time.Time

	// Uncommitted events
	events []eventbus.Event
}

// NewGraph creates a new Graph aggregate
func NewGraph(assistantID, name, version, description string, nodes []Node, edges []Edge, config map[string]interface{}) (*Graph, error) {
	if assistantID == "" {
		return nil, errors.InvalidInput("assistant_id", "assistant_id is required")
	}
	if name == "" {
		return nil, errors.InvalidInput("name", "name is required")
	}
	if version == "" {
		version = "1.0.0"
	}

	// Validate graph structure
	if err := validateGraph(nodes, edges); err != nil {
		return nil, err
	}

	now := time.Now()
	graphID := pkguuid.New()

	if config == nil {
		config = make(map[string]interface{})
	}

	graph := &Graph{
		id:          graphID,
		assistantID: assistantID,
		name:        name,
		version:     version,
		description: description,
		nodes:       nodes,
		edges:       edges,
		config:      config,
		createdAt:   now,
		updatedAt:   now,
		events:      make([]eventbus.Event, 0),
	}

	graph.recordEvent(GraphDefined{
		GraphID:     graphID,
		AssistantID: assistantID,
		Name:        name,
		Version:     version,
		Description: description,
		Nodes:       nodes,
		Edges:       edges,
		Config:      config,
		OccurredAt:  now,
	})

	return graph, nil
}

// ID returns the graph ID
func (g *Graph) ID() string {
	return g.id
}

// AssistantID returns the assistant ID
func (g *Graph) AssistantID() string {
	return g.assistantID
}

// Name returns the graph name
func (g *Graph) Name() string {
	return g.name
}

// Version returns the graph version
func (g *Graph) Version() string {
	return g.version
}

// Description returns the graph description
func (g *Graph) Description() string {
	return g.description
}

// Nodes returns the graph nodes
func (g *Graph) Nodes() []Node {
	return g.nodes
}

// Edges returns the graph edges
func (g *Graph) Edges() []Edge {
	return g.edges
}

// Config returns the graph config
func (g *Graph) Config() map[string]interface{} {
	return g.config
}

// CreatedAt returns the creation time
func (g *Graph) CreatedAt() time.Time {
	return g.createdAt
}

// UpdatedAt returns the last update time
func (g *Graph) UpdatedAt() time.Time {
	return g.updatedAt
}

// Update updates the graph
func (g *Graph) Update(name, description *string, nodes []Node, edges []Edge, config map[string]interface{}) error {
	// If nodes/edges are provided, validate
	if nodes != nil && edges != nil {
		if err := validateGraph(nodes, edges); err != nil {
			return err
		}
	}

	now := time.Now()

	event := GraphUpdated{
		GraphID:    g.id,
		OccurredAt: now,
	}

	if name != nil && *name != "" {
		g.name = *name
		event.Name = name
	}
	if description != nil {
		g.description = *description
		event.Description = description
	}
	if nodes != nil {
		g.nodes = nodes
		event.Nodes = nodes
	}
	if edges != nil {
		g.edges = edges
		event.Edges = edges
	}
	if config != nil {
		g.config = config
		event.Config = config
	}

	g.updatedAt = now
	g.recordEvent(event)

	return nil
}

// Events returns the uncommitted events
func (g *Graph) Events() []eventbus.Event {
	return g.events
}

// ClearEvents clears the uncommitted events
func (g *Graph) ClearEvents() {
	g.events = make([]eventbus.Event, 0)
}

// recordEvent adds an event to the uncommitted events list
func (g *Graph) recordEvent(event eventbus.Event) {
	g.events = append(g.events, event)
}

// validateGraph validates the graph structure
func validateGraph(nodes []Node, edges []Edge) error {
	if len(nodes) == 0 {
		return errors.InvalidInput("nodes", "at least one node is required")
	}

	// Build node ID map
	nodeMap := make(map[string]bool)
	hasStart := false
	hasEnd := false

	for _, node := range nodes {
		if node.ID == "" {
			return errors.InvalidInput("node.id", "node ID is required")
		}
		if nodeMap[node.ID] {
			return errors.InvalidInput("node.id", "duplicate node ID: "+node.ID)
		}
		nodeMap[node.ID] = true

		if node.Type == NodeTypeStart {
			hasStart = true
		}
		if node.Type == NodeTypeEnd {
			hasEnd = true
		}
	}

	if !hasStart {
		return errors.InvalidInput("nodes", "graph must have at least one start node")
	}
	if !hasEnd {
		return errors.InvalidInput("nodes", "graph must have at least one end node")
	}

	// Validate edges
	for _, edge := range edges {
		if edge.Source == "" || edge.Target == "" {
			return errors.InvalidInput("edge", "edge source and target are required")
		}
		if !nodeMap[edge.Source] {
			return errors.InvalidInput("edge.source", "source node not found: "+edge.Source)
		}
		if !nodeMap[edge.Target] {
			return errors.InvalidInput("edge.target", "target node not found: "+edge.Target)
		}
	}

	return nil
}
