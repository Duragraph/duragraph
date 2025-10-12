package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// EinoSpec represents an Eino workflow specification
type EinoSpec struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Nodes       []EinoNode             `json:"nodes"`
	Edges       []EinoEdge             `json:"edges"`
	Config      map[string]interface{} `json:"config"`
}

type EinoNode struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // "llm", "tool", "condition", "start", "end"
	Config   map[string]interface{} `json:"config"`
	Position map[string]float64     `json:"position,omitempty"`
}

type EinoEdge struct {
	Source    string                 `json:"source"`
	Target    string                 `json:"target"`
	Condition map[string]interface{} `json:"condition,omitempty"`
}

// TemporalWorkflow represents the generated Temporal workflow
type TemporalWorkflow struct {
	PackageName    string   `json:"package_name"`
	WorkflowName   string   `json:"workflow_name"`
	Activities     []string `json:"activities"`
	GeneratedCode  string   `json:"generated_code"`
	WorkflowCode   string   `json:"workflow_code"`
	ActivitiesCode string   `json:"activities_code"`
}

func main() {
	var inputFile = flag.String("input", "", "Input Eino specification file (JSON)")
	var outputDir = flag.String("output", "./generated", "Output directory for generated Temporal code")
	flag.Parse()

	if *inputFile == "" {
		log.Fatal("Please provide input file with -input flag")
	}

	// Read Eino specification
	data, err := os.ReadFile(*inputFile)
	if err != nil {
		log.Fatalf("Failed to read input file: %v", err)
	}

	var spec EinoSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		log.Fatalf("Failed to parse Eino spec: %v", err)
	}

	// Generate Temporal workflow
	workflow, err := generateTemporalWorkflow(spec)
	if err != nil {
		log.Fatalf("Failed to generate workflow: %v", err)
	}

	// Write generated files
	if err := writeGeneratedFiles(*outputDir, workflow); err != nil {
		log.Fatalf("Failed to write files: %v", err)
	}

	fmt.Printf("✅ Successfully generated Temporal workflow from %s\n", *inputFile)
	fmt.Printf("📁 Output directory: %s\n", *outputDir)
}

func generateTemporalWorkflow(spec EinoSpec) (*TemporalWorkflow, error) {
	workflow := &TemporalWorkflow{
		PackageName:  "generated",
		WorkflowName: sanitizeName(spec.Name),
		Activities:   []string{},
	}

	// Generate workflow code
	workflow.WorkflowCode = generateWorkflowCode(spec, workflow)

	// Generate activities code
	workflow.ActivitiesCode = generateActivitiesCode(spec, workflow)

	// Combine all code
	workflow.GeneratedCode = fmt.Sprintf(`package %s

import (
	"context"
	"time"
	
	"go.temporal.io/sdk/workflow"
	"go.temporal.io/sdk/activity"
)

%s

%s
`, workflow.PackageName, workflow.WorkflowCode, workflow.ActivitiesCode)

	return workflow, nil
}

func generateWorkflowCode(spec EinoSpec, workflow *TemporalWorkflow) string {
	return fmt.Sprintf(`// %s workflow generated from Eino specification
func %sWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting workflow: %s")

	// TODO: Implement workflow logic based on nodes and edges
	// This is a basic template - extend based on Eino spec

	// Execute nodes in sequence (simplified)
	result := input
	%s

	logger.Info("Workflow completed successfully")
	return result, nil
}`, spec.Description, workflow.WorkflowName, generateNodeExecutions(spec.Nodes))
}

func generateNodeExecutions(nodes []EinoNode) string {
	var executions string
	for _, node := range nodes {
		switch node.Type {
		case "llm":
			executions += fmt.Sprintf(`
	// Execute LLM node: %s
	{
		ao := workflow.ActivityOptions{
			StartToCloseTimeout: time.Minute * 10,
		}
		ctx = workflow.WithActivityOptions(ctx, ao)
		
		var llmResult map[string]interface{}
		err := workflow.ExecuteActivity(ctx, LLMActivity, result).Get(ctx, &llmResult)
		if err != nil {
			return nil, err
		}
		result = llmResult
	}`, node.ID)
		case "tool":
			executions += fmt.Sprintf(`
	// Execute Tool node: %s
	{
		ao := workflow.ActivityOptions{
			StartToCloseTimeout: time.Minute * 5,
		}
		ctx = workflow.WithActivityOptions(ctx, ao)
		
		var toolResult map[string]interface{}
		err := workflow.ExecuteActivity(ctx, ToolActivity, result).Get(ctx, &toolResult)
		if err != nil {
			return nil, err
		}
		result = toolResult
	}`, node.ID)
		}
	}
	return executions
}

func generateActivitiesCode(spec EinoSpec, workflow *TemporalWorkflow) string {
	return `// Generated activities

func LLMActivity(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// TODO: Implement actual LLM call logic
	logger := activity.GetLogger(ctx)
	logger.Info("Executing LLM activity", "input", input)
	
	// Placeholder implementation
	return map[string]interface{}{
		"response": "Generated LLM response",
		"tokens": 42,
	}, nil
}

func ToolActivity(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// TODO: Implement actual tool call logic  
	logger := activity.GetLogger(ctx)
	logger.Info("Executing tool activity", "input", input)
	
	// Placeholder implementation
	return map[string]interface{}{
		"result": "Tool execution result",
		"status": "success",
	}, nil
}`
}

func writeGeneratedFiles(outputDir string, workflow *TemporalWorkflow) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	// Write main workflow file
	workflowFile := filepath.Join(outputDir, "workflow.go")
	if err := os.WriteFile(workflowFile, []byte(workflow.GeneratedCode), 0644); err != nil {
		return err
	}

	// Write go.mod file
	goModContent := fmt.Sprintf(`module %s

go 1.22

require (
	go.temporal.io/sdk v1.36.0
)
`, workflow.PackageName)

	goModFile := filepath.Join(outputDir, "go.mod")
	if err := os.WriteFile(goModFile, []byte(goModContent), 0644); err != nil {
		return err
	}

	return nil
}

func sanitizeName(name string) string {
	// Simple sanitization - make it a valid Go identifier
	result := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result += string(r)
		}
	}
	if result == "" {
		result = "GeneratedWorkflow"
	}
	// Ensure it starts with capital letter
	if len(result) > 0 && result[0] >= 'a' && result[0] <= 'z' {
		result = string(result[0]-32) + result[1:]
	}
	return result
}
