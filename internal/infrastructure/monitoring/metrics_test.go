package monitoring

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
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
	if m.AssistantsTotal == nil {
		t.Error("AssistantsTotal should not be nil")
	}
	if m.ThreadsTotal == nil {
		t.Error("ThreadsTotal should not be nil")
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

func TestRecordRunCreated_LabelsTenant(t *testing.T) {
	m := NewMetrics("test_run_create")

	m.RecordRunCreated("tenant-1", "asst-1")
	m.RecordRunCreated("tenant-1", "asst-1")
	m.RecordRunCreated("tenant-2", "asst-2")

	if got := testutil.ToFloat64(m.RunsTotal.WithLabelValues("tenant-1", "asst-1")); got != 2 {
		t.Errorf("expected runs_total{tenant=t1,asst=a1}=2, got %v", got)
	}
	if got := testutil.ToFloat64(m.RunsTotal.WithLabelValues("tenant-2", "asst-2")); got != 1 {
		t.Errorf("expected runs_total{tenant=t2,asst=a2}=1, got %v", got)
	}
	// Active gauge bumps once per RecordRunCreated, scoped to tenant.
	if got := testutil.ToFloat64(m.RunsActive.WithLabelValues("tenant-1")); got != 2 {
		t.Errorf("expected runs_active{tenant=t1}=2, got %v", got)
	}
	if got := testutil.ToFloat64(m.RunsActive.WithLabelValues("tenant-2")); got != 1 {
		t.Errorf("expected runs_active{tenant=t2}=1, got %v", got)
	}
}

func TestRecordRunCreated_EmptyTenantID(t *testing.T) {
	// Empty tenant_id is the single-tenant deployment mode — must not panic.
	m := NewMetrics("test_run_empty_tenant")
	m.RecordRunCreated("", "asst-1")
	if got := testutil.ToFloat64(m.RunsTotal.WithLabelValues("", "asst-1")); got != 1 {
		t.Errorf("expected runs_total{tenant=,asst=a1}=1, got %v", got)
	}
}

func TestRecordRunCompleted_NoPanic(t *testing.T) {
	m := NewMetrics("test_run_complete")
	m.RecordRunCompleted("tenant-1", "asst-1", "success", 5*time.Second)
	m.RecordRunCompleted("tenant-1", "asst-1", "error", 2*time.Second)
}

func TestIncDecRunsActive(t *testing.T) {
	m := NewMetrics("test_runs_active")
	m.IncRunsActive("tenant-1")
	m.IncRunsActive("tenant-1")
	m.DecRunsActive("tenant-1")
	if got := testutil.ToFloat64(m.RunsActive.WithLabelValues("tenant-1")); got != 1 {
		t.Errorf("expected runs_active{tenant=t1}=1, got %v", got)
	}
}

func TestIncDecAssistants(t *testing.T) {
	m := NewMetrics("test_assistants")
	m.IncAssistants("tenant-1")
	m.IncAssistants("tenant-1")
	m.IncAssistants("tenant-2")
	m.DecAssistants("tenant-1")
	if got := testutil.ToFloat64(m.AssistantsTotal.WithLabelValues("tenant-1")); got != 1 {
		t.Errorf("expected assistants_total{tenant=t1}=1, got %v", got)
	}
	if got := testutil.ToFloat64(m.AssistantsTotal.WithLabelValues("tenant-2")); got != 1 {
		t.Errorf("expected assistants_total{tenant=t2}=1, got %v", got)
	}
}

func TestIncDecThreads(t *testing.T) {
	m := NewMetrics("test_threads")
	m.IncThreads("tenant-1")
	m.IncThreads("tenant-1")
	m.DecThreads("tenant-1")
	if got := testutil.ToFloat64(m.ThreadsTotal.WithLabelValues("tenant-1")); got != 1 {
		t.Errorf("expected threads_total{tenant=t1}=1, got %v", got)
	}
}

func TestRecordNodeExecution_NoPanic(t *testing.T) {
	m := NewMetrics("test_node")
	m.RecordNodeExecution("llm", "success", 100*time.Millisecond)
	m.RecordNodeExecution("tool", "error", 50*time.Millisecond)
}

func TestRecordLLMRequest_LabelsTenant(t *testing.T) {
	m := NewMetrics("test_llm")
	m.RecordLLMRequest("tenant-1", "openai", "gpt-4", "success", 2*time.Second, 100, 200)
	m.RecordLLMRequest("tenant-1", "anthropic", "claude-3", "error", time.Second, 50, 0)

	if got := testutil.ToFloat64(m.LLMRequestsTotal.WithLabelValues("tenant-1", "openai", "gpt-4", "success")); got != 1 {
		t.Errorf("expected llm_requests_total to be 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.LLMTokensTotal.WithLabelValues("tenant-1", "openai", "gpt-4", "prompt")); got != 100 {
		t.Errorf("expected llm_tokens_total{type=prompt}=100, got %v", got)
	}
	if got := testutil.ToFloat64(m.LLMTokensTotal.WithLabelValues("tenant-1", "openai", "gpt-4", "completion")); got != 200 {
		t.Errorf("expected llm_tokens_total{type=completion}=200, got %v", got)
	}
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
