package activities

import (
	"context"
	"fmt"
)

// LLMCallActivity is a stub activity that simulates LLM calls
func LLMCallActivity(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
	// TODO: integrate with LangChain/LangGraph in future
	return map[string]interface{}{
		"response": fmt.Sprintf("stub LLM response for prompt=%v", args["prompt"]),
	}, nil
}

// ToolActivity is a stub activity that simulates tool calls
func ToolActivity(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
	tool := fmt.Sprintf("%v", args["name"])
	return map[string]interface{}{
		"tool":   tool,
		"status": "completed",
	}, nil
}

// EinoRunner is a placeholder for the workflow runner skeleton
type EinoRunner interface {
	Run(ctx context.Context, spec interface{}, inputs map[string]interface{}) (map[string]interface{}, error)
}
