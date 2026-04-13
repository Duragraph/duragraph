package workflow

import (
	"testing"
)

func TestNewAssistant_Valid(t *testing.T) {
	a, err := NewAssistant("my-bot", "A helpful bot", "gpt-4", "Be helpful", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.ID() == "" {
		t.Error("ID should not be empty")
	}
	if a.Name() != "my-bot" {
		t.Errorf("expected name=my-bot, got %s", a.Name())
	}
	if a.Description() != "A helpful bot" {
		t.Error("wrong description")
	}
	if a.Model() != "gpt-4" {
		t.Error("wrong model")
	}
	if a.Instructions() != "Be helpful" {
		t.Error("wrong instructions")
	}
	if a.Tools() == nil {
		t.Error("tools should be initialized")
	}
	if a.Metadata() == nil {
		t.Error("metadata should be initialized")
	}
	if a.CreatedAt().IsZero() || a.UpdatedAt().IsZero() {
		t.Error("timestamps should be set")
	}
}

func TestNewAssistant_MissingName(t *testing.T) {
	_, err := NewAssistant("", "desc", "gpt-4", "instructions", nil, nil)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestNewAssistant_EmitsEvent(t *testing.T) {
	a, _ := NewAssistant("bot", "desc", "gpt-4", "inst", nil, nil)
	events := a.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ac, ok := events[0].(AssistantCreated)
	if !ok {
		t.Fatalf("expected AssistantCreated, got %T", events[0])
	}
	if ac.AssistantID != a.ID() {
		t.Error("event ID should match assistant ID")
	}
	if ac.EventType() != EventTypeAssistantCreated {
		t.Error("wrong event type")
	}
	if ac.AggregateType() != "assistant" {
		t.Error("wrong aggregate type")
	}
	if ac.AggregateID() != a.ID() {
		t.Error("aggregate ID should match")
	}
}

func TestAssistant_Update(t *testing.T) {
	a, _ := NewAssistant("bot", "desc", "gpt-4", "inst", nil, nil)
	a.ClearEvents()

	newName := "new-bot"
	newDesc := "new desc"
	newModel := "claude-3"
	newInst := "be creative"

	err := a.Update(&newName, &newDesc, &newModel, &newInst, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name() != "new-bot" {
		t.Error("name not updated")
	}
	if a.Description() != "new desc" {
		t.Error("description not updated")
	}
	if a.Model() != "claude-3" {
		t.Error("model not updated")
	}
	if a.Instructions() != "be creative" {
		t.Error("instructions not updated")
	}

	events := a.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	_, ok := events[0].(AssistantUpdated)
	if !ok {
		t.Fatalf("expected AssistantUpdated, got %T", events[0])
	}
}

func TestAssistant_UpdatePartial(t *testing.T) {
	a, _ := NewAssistant("bot", "desc", "gpt-4", "inst", nil, nil)
	a.ClearEvents()

	newName := "updated-bot"
	err := a.Update(&newName, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name() != "updated-bot" {
		t.Error("name not updated")
	}
	if a.Description() != "desc" {
		t.Error("description should remain unchanged")
	}
	if a.Model() != "gpt-4" {
		t.Error("model should remain unchanged")
	}
}

func TestAssistant_UpdateEmptyNameIgnored(t *testing.T) {
	a, _ := NewAssistant("bot", "desc", "gpt-4", "inst", nil, nil)
	empty := ""
	_ = a.Update(&empty, nil, nil, nil, nil)
	if a.Name() != "bot" {
		t.Error("empty name should not update")
	}
}

func TestAssistant_UpdateTools(t *testing.T) {
	a, _ := NewAssistant("bot", "desc", "gpt-4", "inst", nil, nil)
	tools := []map[string]interface{}{
		{"type": "function", "name": "search"},
	}
	_ = a.Update(nil, nil, nil, nil, tools)
	if len(a.Tools()) != 1 {
		t.Errorf("expected 1 tool, got %d", len(a.Tools()))
	}
}

func TestAssistant_Delete(t *testing.T) {
	a, _ := NewAssistant("bot", "desc", "gpt-4", "inst", nil, nil)
	a.ClearEvents()

	err := a.Delete()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := a.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ad, ok := events[0].(AssistantDeleted)
	if !ok {
		t.Fatalf("expected AssistantDeleted, got %T", events[0])
	}
	if ad.AssistantID != a.ID() {
		t.Error("wrong assistant ID in event")
	}
	if ad.EventType() != EventTypeAssistantDeleted {
		t.Error("wrong event type")
	}
}

func TestReconstructAssistant(t *testing.T) {
	a, _ := NewAssistant("bot", "desc", "gpt-4", "inst", nil, nil)

	reconstructed, err := ReconstructAssistant(
		a.ID(), a.Name(), a.Description(), a.Model(), a.Instructions(),
		nil, nil, a.CreatedAt(), a.UpdatedAt(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reconstructed.ID() != a.ID() {
		t.Error("ID mismatch")
	}
	if reconstructed.Name() != a.Name() {
		t.Error("name mismatch")
	}
	if len(reconstructed.Events()) != 0 {
		t.Error("reconstructed should have no uncommitted events")
	}
	if reconstructed.Tools() == nil {
		t.Error("nil tools should be initialized")
	}
	if reconstructed.Metadata() == nil {
		t.Error("nil metadata should be initialized")
	}
}

func TestAssistant_ClearEvents(t *testing.T) {
	a, _ := NewAssistant("bot", "desc", "gpt-4", "inst", nil, nil)
	if len(a.Events()) == 0 {
		t.Fatal("should have events after creation")
	}
	a.ClearEvents()
	if len(a.Events()) != 0 {
		t.Error("events should be empty after ClearEvents")
	}
}
