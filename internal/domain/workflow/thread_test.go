package workflow

import (
	"testing"
)

func TestNewThread_Valid(t *testing.T) {
	th, err := NewThread(map[string]interface{}{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if th.ID() == "" {
		t.Error("ID should not be empty")
	}
	if len(th.Messages()) != 0 {
		t.Error("new thread should have no messages")
	}
	if th.Metadata()["key"] != "value" {
		t.Error("metadata not set")
	}
	if th.CreatedAt().IsZero() || th.UpdatedAt().IsZero() {
		t.Error("timestamps should be set")
	}
}

func TestNewThread_NilMetadata(t *testing.T) {
	th, err := NewThread(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if th.Metadata() == nil {
		t.Error("nil metadata should be initialized to empty map")
	}
}

func TestNewThread_EmitsEvent(t *testing.T) {
	th, _ := NewThread(nil)
	events := th.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	tc, ok := events[0].(ThreadCreated)
	if !ok {
		t.Fatalf("expected ThreadCreated, got %T", events[0])
	}
	if tc.ThreadID != th.ID() {
		t.Error("event thread ID should match")
	}
	if tc.EventType() != EventTypeThreadCreated {
		t.Error("wrong event type")
	}
	if tc.AggregateType() != "thread" {
		t.Error("wrong aggregate type")
	}
}

func TestThread_AddMessage_Valid(t *testing.T) {
	th, _ := NewThread(nil)
	th.ClearEvents()

	msg, err := th.AddMessage("user", "hello world", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ID == "" {
		t.Error("message ID should not be empty")
	}
	if msg.Role != "user" {
		t.Error("wrong role")
	}
	if msg.Content != "hello world" {
		t.Error("wrong content")
	}
	if msg.Metadata == nil {
		t.Error("nil metadata should be initialized")
	}
	if msg.CreatedAt.IsZero() {
		t.Error("created_at should be set")
	}
	if len(th.Messages()) != 1 {
		t.Errorf("expected 1 message, got %d", len(th.Messages()))
	}
}

func TestThread_AddMessage_AllRoles(t *testing.T) {
	roles := []string{"user", "assistant", "system"}
	for _, role := range roles {
		th, _ := NewThread(nil)
		_, err := th.AddMessage(role, "content", nil)
		if err != nil {
			t.Errorf("AddMessage(%s) should not error: %v", role, err)
		}
	}
}

func TestThread_AddMessage_InvalidRole(t *testing.T) {
	th, _ := NewThread(nil)
	_, err := th.AddMessage("admin", "content", nil)
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestThread_AddMessage_EmptyRole(t *testing.T) {
	th, _ := NewThread(nil)
	_, err := th.AddMessage("", "content", nil)
	if err == nil {
		t.Fatal("expected error for empty role")
	}
}

func TestThread_AddMessage_EmptyContent(t *testing.T) {
	th, _ := NewThread(nil)
	_, err := th.AddMessage("user", "", nil)
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestThread_AddMessage_EmitsEvent(t *testing.T) {
	th, _ := NewThread(nil)
	th.ClearEvents()

	_, _ = th.AddMessage("user", "hello", map[string]interface{}{"source": "test"})

	events := th.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ma, ok := events[0].(MessageAdded)
	if !ok {
		t.Fatalf("expected MessageAdded, got %T", events[0])
	}
	if ma.ThreadID != th.ID() {
		t.Error("wrong thread ID")
	}
	if ma.Role != "user" {
		t.Error("wrong role")
	}
	if ma.Content != "hello" {
		t.Error("wrong content")
	}
	if ma.EventType() != EventTypeMessageAdded {
		t.Error("wrong event type")
	}
	if ma.AggregateType() != "thread" {
		t.Error("wrong aggregate type")
	}
}

func TestThread_AddMultipleMessages(t *testing.T) {
	th, _ := NewThread(nil)
	_, _ = th.AddMessage("user", "hello", nil)
	_, _ = th.AddMessage("assistant", "hi there", nil)
	_, _ = th.AddMessage("user", "thanks", nil)

	if len(th.Messages()) != 3 {
		t.Errorf("expected 3 messages, got %d", len(th.Messages()))
	}
	if th.Messages()[0].Role != "user" {
		t.Error("first message should be user")
	}
	if th.Messages()[1].Role != "assistant" {
		t.Error("second message should be assistant")
	}
}

func TestThread_UpdateMetadata(t *testing.T) {
	th, _ := NewThread(map[string]interface{}{"old": true})
	th.ClearEvents()

	err := th.UpdateMetadata(map[string]interface{}{"new": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if th.Metadata()["new"] != true {
		t.Error("metadata not updated")
	}
	if _, exists := th.Metadata()["old"]; exists {
		t.Error("old metadata should be replaced")
	}

	events := th.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	tu, ok := events[0].(ThreadUpdated)
	if !ok {
		t.Fatalf("expected ThreadUpdated, got %T", events[0])
	}
	if tu.EventType() != EventTypeThreadUpdated {
		t.Error("wrong event type")
	}
}

func TestReconstructThread(t *testing.T) {
	th, _ := NewThread(nil)
	_, _ = th.AddMessage("user", "hello", nil)

	reconstructed, err := ReconstructThread(
		th.ID(), th.Messages(), th.Metadata(), th.CreatedAt(), th.UpdatedAt(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reconstructed.ID() != th.ID() {
		t.Error("ID mismatch")
	}
	if len(reconstructed.Messages()) != 1 {
		t.Error("messages not reconstructed")
	}
	if len(reconstructed.Events()) != 0 {
		t.Error("reconstructed should have no uncommitted events")
	}
}

func TestReconstructThread_NilFields(t *testing.T) {
	th, err := ReconstructThread("id-1", nil, nil, th_now(), th_now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if th.Messages() == nil {
		t.Error("nil messages should be initialized")
	}
	if th.Metadata() == nil {
		t.Error("nil metadata should be initialized")
	}
}

func TestThread_ClearEvents(t *testing.T) {
	th, _ := NewThread(nil)
	if len(th.Events()) == 0 {
		t.Fatal("should have events after creation")
	}
	th.ClearEvents()
	if len(th.Events()) != 0 {
		t.Error("events should be empty")
	}
}
