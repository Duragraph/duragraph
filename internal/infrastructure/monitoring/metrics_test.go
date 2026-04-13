package monitoring

import (
	"testing"
	"time"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics("test_monitoring")
	if m == nil {
		t.Fatal("metrics should not be nil")
	}
	if m.HTTPRequestsTotal == nil {
		t.Error("HTTPRequestsTotal should not be nil")
	}
	if m.RunsTotal == nil {
		t.Error("RunsTotal should not be nil")
	}
	if m.LLMRequestsTotal == nil {
		t.Error("LLMRequestsTotal should not be nil")
	}
	if m.WorkersActive == nil {
		t.Error("WorkersActive should not be nil")
	}
	if m.ErrorsTotal == nil {
		t.Error("ErrorsTotal should not be nil")
	}
}

func TestNewMetrics_DefaultNamespace(t *testing.T) {
	m := NewMetrics("")
	if m == nil {
		t.Fatal("metrics should not be nil")
	}
}

func TestRecordHTTPRequest_NoPanic(t *testing.T) {
	m := NewMetrics("test_http")
	m.RecordHTTPRequest("GET", "/api/v1/runs", 200, 50*time.Millisecond, 0, 1024)
	m.RecordHTTPRequest("POST", "/api/v1/runs", 201, 100*time.Millisecond, 512, 256)
	m.RecordHTTPRequest("GET", "/api/v1/runs/123", 404, 10*time.Millisecond, 0, 64)
}

func TestRecordRunCreated_NoPanic(t *testing.T) {
	m := NewMetrics("test_run")
	m.RecordRunCreated("asst-1")
	m.RecordRunCreated("asst-2")
}

func TestRecordRunCompleted_NoPanic(t *testing.T) {
	m := NewMetrics("test_run_complete")
	m.RecordRunCompleted("asst-1", "success", 5*time.Second)
	m.RecordRunCompleted("asst-1", "error", 2*time.Second)
}

func TestRecordNodeExecution_NoPanic(t *testing.T) {
	m := NewMetrics("test_node")
	m.RecordNodeExecution("llm", "success", 100*time.Millisecond)
	m.RecordNodeExecution("tool", "error", 50*time.Millisecond)
}

func TestRecordLLMRequest_NoPanic(t *testing.T) {
	m := NewMetrics("test_llm")
	m.RecordLLMRequest("openai", "gpt-4", "success", 2*time.Second, 100, 200)
	m.RecordLLMRequest("anthropic", "claude-3", "error", time.Second, 50, 0)
}

func TestRecordToolExecution_NoPanic(t *testing.T) {
	m := NewMetrics("test_tool")
	m.RecordToolExecution("http_request", "success", 500*time.Millisecond)
	m.RecordToolExecution("json_processor", "error", 10*time.Millisecond)
}

func TestRecordError_NoPanic(t *testing.T) {
	m := NewMetrics("test_error")
	m.RecordError("domain", "NOT_FOUND")
	m.RecordError("infrastructure", "DB_TIMEOUT")
}
