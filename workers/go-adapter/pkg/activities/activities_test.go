package activities

import (
	"context"
	"testing"
)

func TestLLMCallActivity(t *testing.T) {
	args := map[string]interface{}{"prompt": "hello"}
	result, err := LLMCallActivity(context.Background(), args)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result["response"] == "" {
		t.Fatal("expected response to be non-empty")
	}
}

func TestToolActivity(t *testing.T) {
	args := map[string]interface{}{"name": "echo", "input": map[string]interface{}{"x": 42}}
	result, err := ToolActivity(context.Background(), args)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result["status"] != "completed" {
		t.Fatalf("expected status=completed, got %v", result["status"])
	}
	if result["tool"] != "echo" {
		t.Fatalf("expected tool=echo, got %v", result["tool"])
	}
}
