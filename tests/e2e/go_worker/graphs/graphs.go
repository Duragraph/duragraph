// Package graphs provides mock graph definitions for testing.
package graphs

// NodeType represents the type of a node.
type NodeType string

const (
	NodeTypeInput    NodeType = "input"
	NodeTypeOutput   NodeType = "output"
	NodeTypeLLM      NodeType = "llm"
	NodeTypeTool     NodeType = "tool"
	NodeTypeRouter   NodeType = "router"
	NodeTypeHuman    NodeType = "human"
	NodeTypeSubgraph NodeType = "subgraph"
)

// Node represents a node in a graph.
type Node struct {
	ID     string                 `json:"id"`
	Type   NodeType               `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// Edge represents an edge in a graph.
type Edge struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Condition string `json:"condition,omitempty"`
}

// Graph represents a graph definition.
type Graph struct {
	ID          string `json:"graph_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Nodes       []Node `json:"nodes"`
	Edges       []Edge `json:"edges"`
	EntryPoint  string `json:"entry_point"`
}

// All available graphs.
var All = map[string]Graph{
	"simple_echo":     SimpleEcho,
	"multi_step":      MultiStep,
	"branching":       Branching,
	"tool_calling":    ToolCalling,
	"human_interrupt": HumanInterrupt,
	"long_running":    LongRunning,
	"failure":         Failure,
}

// SimpleEcho is a basic echo graph.
var SimpleEcho = Graph{
	ID:          "simple_echo",
	Name:        "Simple Echo",
	Description: "Echoes input back with minimal processing",
	Nodes: []Node{
		{ID: "start", Type: NodeTypeInput},
		{ID: "echo", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock"}},
		{ID: "end", Type: NodeTypeOutput},
	},
	Edges: []Edge{
		{Source: "start", Target: "echo"},
		{Source: "echo", Target: "end"},
	},
	EntryPoint: "start",
}

// MultiStep is a multi-step processing graph.
var MultiStep = Graph{
	ID:          "multi_step",
	Name:        "Multi-Step",
	Description: "Multiple LLM calls in sequence",
	Nodes: []Node{
		{ID: "start", Type: NodeTypeInput},
		{ID: "analyze", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "analyze"}},
		{ID: "process", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "process"}},
		{ID: "summarize", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "summarize"}},
		{ID: "end", Type: NodeTypeOutput},
	},
	Edges: []Edge{
		{Source: "start", Target: "analyze"},
		{Source: "analyze", Target: "process"},
		{Source: "process", Target: "summarize"},
		{Source: "summarize", Target: "end"},
	},
	EntryPoint: "start",
}

// Branching is a graph with conditional branching.
var Branching = Graph{
	ID:          "branching",
	Name:        "Branching",
	Description: "Conditional routing based on input",
	Nodes: []Node{
		{ID: "start", Type: NodeTypeInput},
		{ID: "classify", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "classify"}},
		{ID: "router", Type: NodeTypeRouter},
		{ID: "path_a", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "path_a"}},
		{ID: "path_b", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "path_b"}},
		{ID: "merge", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "merge"}},
		{ID: "end", Type: NodeTypeOutput},
	},
	Edges: []Edge{
		{Source: "start", Target: "classify"},
		{Source: "classify", Target: "router"},
		{Source: "router", Target: "path_a", Condition: "category == 'A'"},
		{Source: "router", Target: "path_b", Condition: "category == 'B'"},
		{Source: "path_a", Target: "merge"},
		{Source: "path_b", Target: "merge"},
		{Source: "merge", Target: "end"},
	},
	EntryPoint: "start",
}

// ToolCalling is a graph with tool calls.
var ToolCalling = Graph{
	ID:          "tool_calling",
	Name:        "Tool Calling",
	Description: "Demonstrates tool usage",
	Nodes: []Node{
		{ID: "start", Type: NodeTypeInput},
		{ID: "plan", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "plan"}},
		{ID: "search", Type: NodeTypeTool, Config: map[string]interface{}{"tool": "web_search"}},
		{ID: "calculator", Type: NodeTypeTool, Config: map[string]interface{}{"tool": "calculator"}},
		{ID: "synthesize", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "synthesize"}},
		{ID: "end", Type: NodeTypeOutput},
	},
	Edges: []Edge{
		{Source: "start", Target: "plan"},
		{Source: "plan", Target: "search"},
		{Source: "search", Target: "calculator"},
		{Source: "calculator", Target: "synthesize"},
		{Source: "synthesize", Target: "end"},
	},
	EntryPoint: "start",
}

// HumanInterrupt is a graph with human-in-the-loop.
var HumanInterrupt = Graph{
	ID:          "human_interrupt",
	Name:        "Human Interrupt",
	Description: "Requires human approval at certain steps",
	Nodes: []Node{
		{ID: "start", Type: NodeTypeInput},
		{ID: "draft", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "draft"}},
		{ID: "review", Type: NodeTypeHuman, Config: map[string]interface{}{
			"prompt":          "Please review and approve the draft",
			"required_fields": []string{"approved", "feedback"},
		}},
		{ID: "revise", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "purpose": "revise"}},
		{ID: "end", Type: NodeTypeOutput},
	},
	Edges: []Edge{
		{Source: "start", Target: "draft"},
		{Source: "draft", Target: "review"},
		{Source: "review", Target: "revise"},
		{Source: "revise", Target: "end"},
	},
	EntryPoint: "start",
}

// LongRunning is a graph that simulates long-running operations.
var LongRunning = Graph{
	ID:          "long_running",
	Name:        "Long Running",
	Description: "Simulates time-consuming operations",
	Nodes: []Node{
		{ID: "start", Type: NodeTypeInput},
		{ID: "step1", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "delay_ms": 500}},
		{ID: "step2", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "delay_ms": 500}},
		{ID: "step3", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "delay_ms": 500}},
		{ID: "step4", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "delay_ms": 500}},
		{ID: "step5", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "delay_ms": 500}},
		{ID: "end", Type: NodeTypeOutput},
	},
	Edges: []Edge{
		{Source: "start", Target: "step1"},
		{Source: "step1", Target: "step2"},
		{Source: "step2", Target: "step3"},
		{Source: "step3", Target: "step4"},
		{Source: "step4", Target: "step5"},
		{Source: "step5", Target: "end"},
	},
	EntryPoint: "start",
}

// Failure is a graph that simulates failures.
var Failure = Graph{
	ID:          "failure",
	Name:        "Failure",
	Description: "Simulates various failure modes",
	Nodes: []Node{
		{ID: "start", Type: NodeTypeInput},
		{ID: "process", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock"}},
		{ID: "fail", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "mock", "fail": true}},
		{ID: "end", Type: NodeTypeOutput},
	},
	Edges: []Edge{
		{Source: "start", Target: "process"},
		{Source: "process", Target: "fail"},
		{Source: "fail", Target: "end"},
	},
	EntryPoint: "start",
}

// Get returns a graph by ID.
func Get(id string) (Graph, bool) {
	g, ok := All[id]
	return g, ok
}

// IDs returns all graph IDs.
func IDs() []string {
	ids := make([]string, 0, len(All))
	for id := range All {
		ids = append(ids, id)
	}
	return ids
}
