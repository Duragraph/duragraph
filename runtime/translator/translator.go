package translator

import (
	"fmt"
	"log"
)

// WorkflowSpec represents a generic workflow specification
type WorkflowSpec struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Nodes       []Node                 `json:"nodes"`
	Edges       []Edge                 `json:"edges"`
	Config      map[string]interface{} `json:"config"`
}

// Node represents a workflow node
type Node struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // "start", "llm", "tool", "condition", "end"
	Config   map[string]interface{} `json:"config"`
	Position map[string]float64     `json:"position,omitempty"`
}

// Edge represents a workflow edge
type Edge struct {
	Source    string                 `json:"source"`
	Target    string                 `json:"target"`
	Condition map[string]interface{} `json:"condition,omitempty"`
}

// TemporalWorkflowIR represents the intermediate representation for Temporal
type TemporalWorkflowIR struct {
	WorkflowID string                 `json:"workflow_id"`
	Activities []ActivityDefinition   `json:"activities"`
	Execution  []ExecutionStep        `json:"execution"`
	Config     map[string]interface{} `json:"config"`
}

// ActivityDefinition represents a Temporal activity
type ActivityDefinition struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Config      map[string]interface{} `json:"config"`
	Timeout     string                 `json:"timeout,omitempty"`
	RetryPolicy map[string]interface{} `json:"retry_policy,omitempty"`
}

// ExecutionStep represents a step in workflow execution
type ExecutionStep struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // "activity", "condition", "parallel", "sequence"
	ActivityRef string                 `json:"activity_ref,omitempty"`
	Condition   string                 `json:"condition,omitempty"`
	Next        []string               `json:"next,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// TranslateEinoToIR converts Eino workflow spec to Temporal IR
func TranslateEinoToIR(spec WorkflowSpec) (*TemporalWorkflowIR, error) {
	log.Printf("[translator] Converting Eino spec '%s' to Temporal IR", spec.Name)

	ir := &TemporalWorkflowIR{
		WorkflowID: fmt.Sprintf("%s_workflow", sanitizeName(spec.Name)),
		Activities: []ActivityDefinition{},
		Execution:  []ExecutionStep{},
		Config:     spec.Config,
	}

	// Convert nodes to activities
	activityMap := make(map[string]string)
	for _, node := range spec.Nodes {
		if node.Type != "start" && node.Type != "end" {
			activity := convertNodeToActivity(node)
			ir.Activities = append(ir.Activities, activity)
			activityMap[node.ID] = activity.Name
		}
	}

	// Convert edges to execution steps
	for _, node := range spec.Nodes {
		if node.Type != "end" {
			step := convertNodeToExecutionStep(node, spec.Edges, activityMap)
			ir.Execution = append(ir.Execution, step)
		}
	}

	log.Printf("[translator] Generated IR with %d activities and %d execution steps",
		len(ir.Activities), len(ir.Execution))

	return ir, nil
}

// TranslateLangGraphToIR converts LangGraph workflow spec to Temporal IR
func TranslateLangGraphToIR(spec WorkflowSpec) (*TemporalWorkflowIR, error) {
	log.Printf("[translator] Converting LangGraph spec '%s' to Temporal IR", spec.Name)

	// For now, use the same logic as Eino - can be specialized later
	return TranslateEinoToIR(spec)
}

func convertNodeToActivity(node Node) ActivityDefinition {
	activityName := fmt.Sprintf("%s_activity", sanitizeName(node.ID))

	activity := ActivityDefinition{
		Name:   activityName,
		Type:   node.Type,
		Config: node.Config,
	}

	// Set default timeouts based on activity type
	switch node.Type {
	case "llm":
		activity.Timeout = "10m"
	case "tool":
		activity.Timeout = "5m"
	default:
		activity.Timeout = "1m"
	}

	return activity
}

func convertNodeToExecutionStep(node Node, edges []Edge, activityMap map[string]string) ExecutionStep {
	step := ExecutionStep{
		ID:     node.ID,
		Config: node.Config,
	}

	// Find outgoing edges
	var nextNodes []string
	for _, edge := range edges {
		if edge.Source == node.ID {
			nextNodes = append(nextNodes, edge.Target)
		}
	}
	step.Next = nextNodes

	// Set step type and activity reference
	switch node.Type {
	case "start":
		step.Type = "start"
	case "end":
		step.Type = "end"
	case "condition":
		step.Type = "condition"
		// TODO: Extract condition logic from node config
	default:
		step.Type = "activity"
		if activityRef, exists := activityMap[node.ID]; exists {
			step.ActivityRef = activityRef
		}
	}

	return step
}

func sanitizeName(name string) string {
	result := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result += string(r)
		} else {
			result += "_"
		}
	}
	if result == "" {
		result = "generated"
	}
	return result
}

// IRToTemporalCode generates Temporal workflow code from IR
func IRToTemporalCode(ir *TemporalWorkflowIR, language string) (string, error) {
	switch language {
	case "go":
		return generateGoWorkflowCode(ir)
	case "python":
		return generatePythonWorkflowCode(ir)
	default:
		return "", fmt.Errorf("unsupported language: %s", language)
	}
}

func generateGoWorkflowCode(ir *TemporalWorkflowIR) (string, error) {
	// TODO: Implement Go code generation from IR
	return fmt.Sprintf(`// Generated Go workflow for %s
// TODO: Implement full code generation
package main

func %s(ctx workflow.Context, input map[string]interface{}) error {
    // Generated workflow logic will go here
    return nil
}
`, ir.WorkflowID, ir.WorkflowID), nil
}

func generatePythonWorkflowCode(ir *TemporalWorkflowIR) (string, error) {
	// TODO: Implement Python code generation from IR
	return fmt.Sprintf(`# Generated Python workflow for %s
# TODO: Implement full code generation

@workflow.defn
class %sWorkflow:
    @workflow.run
    async def run(self, input_data):
        # Generated workflow logic will go here
        pass
`, ir.WorkflowID, ir.WorkflowID), nil
}

// HardcodedWorkflow is the legacy method - kept for backward compatibility
func HardcodedWorkflow(input string) []string {
	log.Printf("[translator] Legacy method called with input: %s", input)
	steps := []string{
		"input",
		"llm_call: echo(" + input + ")",
		"end",
	}
	log.Printf("[translator] Generated legacy steps: %v", steps)
	return steps
}
