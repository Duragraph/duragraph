package execution

// MessageChunkCallback is called for each LLM token chunk during streaming
type MessageChunkCallback func(content, role, id string) error

// ExecutionState represents the state of a graph execution
type ExecutionState struct {
	RunID          string
	CurrentNodes   []string                          // Nodes currently executing (for parallel execution)
	CompletedNodes map[string]bool                   // Nodes that have completed
	NodeOutputs    map[string]map[string]interface{} // Node ID -> Output
	GlobalState    map[string]interface{}            // Shared state across nodes
	Iteration      int                               // Current iteration for loops
	StreamEnabled  bool                              // Whether streaming is enabled
	MessageChunk   MessageChunkCallback              // Callback for LLM token streaming
}

// NewExecutionState creates a new execution state
func NewExecutionState(runID string) *ExecutionState {
	return &ExecutionState{
		RunID:          runID,
		CurrentNodes:   make([]string, 0),
		CompletedNodes: make(map[string]bool),
		NodeOutputs:    make(map[string]map[string]interface{}),
		GlobalState:    make(map[string]interface{}),
		Iteration:      0,
	}
}

// MarkNodeStarted marks a node as started
func (s *ExecutionState) MarkNodeStarted(nodeID string) {
	s.CurrentNodes = append(s.CurrentNodes, nodeID)
}

// MarkNodeCompleted marks a node as completed
func (s *ExecutionState) MarkNodeCompleted(nodeID string, output map[string]interface{}) {
	s.CompletedNodes[nodeID] = true
	s.NodeOutputs[nodeID] = output

	// Remove from current nodes
	for i, id := range s.CurrentNodes {
		if id == nodeID {
			s.CurrentNodes = append(s.CurrentNodes[:i], s.CurrentNodes[i+1:]...)
			break
		}
	}
}

// IsNodeCompleted checks if a node has completed
func (s *ExecutionState) IsNodeCompleted(nodeID string) bool {
	return s.CompletedNodes[nodeID]
}

// GetNodeOutput retrieves the output of a completed node
func (s *ExecutionState) GetNodeOutput(nodeID string) map[string]interface{} {
	return s.NodeOutputs[nodeID]
}

// UpdateGlobalState updates the global state
func (s *ExecutionState) UpdateGlobalState(key string, value interface{}) {
	s.GlobalState[key] = value
}

// GetGlobalState retrieves a value from global state
func (s *ExecutionState) GetGlobalState(key string) interface{} {
	return s.GlobalState[key]
}

// IncrementIteration increments the iteration counter
func (s *ExecutionState) IncrementIteration() {
	s.Iteration++
}

// SetStreamingCallback sets the streaming callback for LLM token streaming
func (s *ExecutionState) SetStreamingCallback(callback MessageChunkCallback) {
	s.StreamEnabled = true
	s.MessageChunk = callback
}

// EmitMessageChunk emits a message chunk if streaming is enabled
func (s *ExecutionState) EmitMessageChunk(content, role, id string) error {
	if s.StreamEnabled && s.MessageChunk != nil {
		return s.MessageChunk(content, role, id)
	}
	return nil
}

// Clone creates a deep copy of the state (for subgraphs)
func (s *ExecutionState) Clone() *ExecutionState {
	clone := &ExecutionState{
		RunID:          s.RunID,
		CurrentNodes:   make([]string, len(s.CurrentNodes)),
		CompletedNodes: make(map[string]bool),
		NodeOutputs:    make(map[string]map[string]interface{}),
		GlobalState:    make(map[string]interface{}),
		Iteration:      s.Iteration,
		StreamEnabled:  s.StreamEnabled,
		MessageChunk:   s.MessageChunk,
	}

	copy(clone.CurrentNodes, s.CurrentNodes)

	for k, v := range s.CompletedNodes {
		clone.CompletedNodes[k] = v
	}

	for k, v := range s.NodeOutputs {
		clone.NodeOutputs[k] = v
	}

	for k, v := range s.GlobalState {
		clone.GlobalState[k] = v
	}

	return clone
}
