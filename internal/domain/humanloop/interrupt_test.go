package humanloop

import (
	"errors"
	"testing"

	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

func TestNewInterrupt_Valid(t *testing.T) {
	state := map[string]interface{}{"key": "value"}
	toolCalls := []map[string]interface{}{
		{"id": "call-1", "name": "search"},
	}

	interrupt, err := NewInterrupt("run-1", "node-1", ReasonToolCall, state, toolCalls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if interrupt.ID() == "" {
		t.Error("ID should not be empty")
	}
	if interrupt.RunID() != "run-1" {
		t.Error("wrong RunID")
	}
	if interrupt.NodeID() != "node-1" {
		t.Error("wrong NodeID")
	}
	if interrupt.Reason() != ReasonToolCall {
		t.Errorf("expected reason=%s, got %s", ReasonToolCall, interrupt.Reason())
	}
	if interrupt.State()["key"] != "value" {
		t.Error("state not set")
	}
	if len(interrupt.ToolCalls()) != 1 {
		t.Error("tool calls not set")
	}
	if interrupt.IsResolved() {
		t.Error("should not be resolved initially")
	}
	if interrupt.ResolvedAt() != nil {
		t.Error("ResolvedAt should be nil")
	}
	if interrupt.CreatedAt().IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestNewInterrupt_NilState(t *testing.T) {
	interrupt, _ := NewInterrupt("run-1", "node-1", ReasonApprovalRequired, nil, nil)
	if interrupt.State() == nil {
		t.Error("nil state should be initialized to empty map")
	}
	if interrupt.ToolCalls() == nil {
		t.Error("nil toolCalls should be initialized to empty slice")
	}
}

func TestNewInterrupt_MissingRunID(t *testing.T) {
	_, err := NewInterrupt("", "node-1", ReasonToolCall, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing run_id")
	}
}

func TestNewInterrupt_MissingNodeID(t *testing.T) {
	_, err := NewInterrupt("run-1", "", ReasonToolCall, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing node_id")
	}
}

func TestNewInterrupt_EmitsEvent(t *testing.T) {
	interrupt, _ := NewInterrupt("run-1", "node-1", ReasonToolCall, nil, nil)
	events := interrupt.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	ic, ok := events[0].(InterruptCreated)
	if !ok {
		t.Fatalf("expected InterruptCreated, got %T", events[0])
	}
	if ic.InterruptID != interrupt.ID() {
		t.Error("event ID mismatch")
	}
	if ic.RunID != "run-1" {
		t.Error("event RunID mismatch")
	}
	if ic.NodeID != "node-1" {
		t.Error("event NodeID mismatch")
	}
	if ic.Reason != string(ReasonToolCall) {
		t.Error("event Reason mismatch")
	}
	if ic.EventType() != EventTypeInterruptCreated {
		t.Error("wrong event type")
	}
	if ic.AggregateType() != "interrupt" {
		t.Error("wrong aggregate type")
	}
	if ic.AggregateID() != interrupt.ID() {
		t.Error("wrong aggregate ID")
	}
}

func TestInterrupt_Resolve(t *testing.T) {
	interrupt, _ := NewInterrupt("run-1", "node-1", ReasonToolCall, nil, nil)
	interrupt.ClearEvents()

	toolOutputs := []map[string]interface{}{
		{"tool_call_id": "call-1", "output": "result"},
	}
	err := interrupt.Resolve(toolOutputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !interrupt.IsResolved() {
		t.Error("should be resolved")
	}
	if interrupt.ResolvedAt() == nil {
		t.Error("ResolvedAt should be set")
	}

	events := interrupt.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ir, ok := events[0].(InterruptResolved)
	if !ok {
		t.Fatalf("expected InterruptResolved, got %T", events[0])
	}
	if ir.EventType() != EventTypeInterruptResolved {
		t.Error("wrong event type")
	}
	if ir.AggregateID() != interrupt.ID() {
		t.Error("wrong aggregate ID")
	}
	if ir.AggregateType() != "interrupt" {
		t.Error("wrong aggregate type")
	}
	if ir.RunID != "run-1" {
		t.Error("wrong RunID in event")
	}
}

func TestInterrupt_ResolveAlreadyResolved(t *testing.T) {
	interrupt, _ := NewInterrupt("run-1", "node-1", ReasonToolCall, nil, nil)
	_ = interrupt.Resolve(nil)

	err := interrupt.Resolve(nil)
	if err == nil {
		t.Fatal("expected error when resolving already-resolved interrupt")
	}
	if !errors.Is(err, pkgerrors.ErrInvalidState) {
		t.Error("should be an InvalidState error")
	}
}

func TestInterrupt_ClearEvents(t *testing.T) {
	interrupt, _ := NewInterrupt("run-1", "node-1", ReasonToolCall, nil, nil)
	if len(interrupt.Events()) == 0 {
		t.Fatal("should have events after creation")
	}
	interrupt.ClearEvents()
	if len(interrupt.Events()) != 0 {
		t.Error("events should be empty")
	}
}

func TestInterruptReasons(t *testing.T) {
	reasons := map[InterruptReason]string{
		ReasonToolCall:         "tool_call",
		ReasonApprovalRequired: "approval_required",
		ReasonInputNeeded:      "input_needed",
	}
	for r, expected := range reasons {
		if string(r) != expected {
			t.Errorf("expected %s, got %s", expected, string(r))
		}
	}
}
