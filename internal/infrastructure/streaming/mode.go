package streaming

import "strings"

// StreamMode defines the type of streaming content
type StreamMode string

const (
	// ModeValues sends full state after each node execution
	ModeValues StreamMode = "values"

	// ModeMessages sends streaming LLM tokens
	ModeMessages StreamMode = "messages"

	// ModeUpdates sends state diffs/deltas
	ModeUpdates StreamMode = "updates"

	// ModeDebug sends detailed execution info
	ModeDebug StreamMode = "debug"

	// ModeEvents sends all events (default LangGraph behavior)
	ModeEvents StreamMode = "events"
)

// ParseStreamModes parses stream modes from a comma-separated string or slice
func ParseStreamModes(modes []string) []StreamMode {
	result := make([]StreamMode, 0)
	seen := make(map[StreamMode]bool)

	for _, m := range modes {
		// Handle comma-separated values
		for _, part := range strings.Split(m, ",") {
			mode := StreamMode(strings.TrimSpace(strings.ToLower(part)))
			if isValidMode(mode) && !seen[mode] {
				result = append(result, mode)
				seen[mode] = true
			}
		}
	}

	// Default to events if no valid modes specified
	if len(result) == 0 {
		result = append(result, ModeEvents)
	}

	return result
}

func isValidMode(mode StreamMode) bool {
	switch mode {
	case ModeValues, ModeMessages, ModeUpdates, ModeDebug, ModeEvents:
		return true
	default:
		return false
	}
}

// StreamEvent represents an event to be streamed
type StreamEvent struct {
	Event string                 `json:"event"`
	Data  map[string]interface{} `json:"data"`
	RunID string                 `json:"run_id,omitempty"`
}

// NodeStartEvent represents a node starting execution
type NodeStartEvent struct {
	NodeID   string                 `json:"node_id"`
	NodeType string                 `json:"node_type"`
	Input    map[string]interface{} `json:"input,omitempty"`
}

// NodeEndEvent represents a node completing execution
type NodeEndEvent struct {
	NodeID   string                 `json:"node_id"`
	NodeType string                 `json:"node_type"`
	Output   map[string]interface{} `json:"output,omitempty"`
	Duration int64                  `json:"duration_ms"`
}

// StateUpdateEvent represents a state update
type StateUpdateEvent struct {
	Values map[string]interface{} `json:"values"`
}

// MessageChunkEvent represents an LLM token/chunk
type MessageChunkEvent struct {
	Content string `json:"content"`
	Role    string `json:"role,omitempty"`
	ID      string `json:"id,omitempty"`
}

// DebugEvent represents debug information
type DebugEvent struct {
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}
