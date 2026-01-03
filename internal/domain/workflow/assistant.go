package workflow

import (
	"time"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

// Assistant represents an AI assistant aggregate
type Assistant struct {
	id           string
	name         string
	description  string
	model        string
	instructions string
	tools        []map[string]interface{}
	metadata     map[string]interface{}
	createdAt    time.Time
	updatedAt    time.Time

	// Uncommitted events
	events []eventbus.Event
}

// NewAssistant creates a new Assistant aggregate
func NewAssistant(name, description, model, instructions string, tools []map[string]interface{}, metadata map[string]interface{}) (*Assistant, error) {
	if name == "" {
		return nil, errors.InvalidInput("name", "name is required")
	}

	now := time.Now()
	assistantID := pkguuid.New()

	if tools == nil {
		tools = make([]map[string]interface{}, 0)
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	assistant := &Assistant{
		id:           assistantID,
		name:         name,
		description:  description,
		model:        model,
		instructions: instructions,
		tools:        tools,
		metadata:     metadata,
		createdAt:    now,
		updatedAt:    now,
		events:       make([]eventbus.Event, 0),
	}

	assistant.recordEvent(AssistantCreated{
		AssistantID:  assistantID,
		Name:         name,
		Description:  description,
		Model:        model,
		Instructions: instructions,
		Tools:        tools,
		OccurredAt:   now,
	})

	return assistant, nil
}

// ReconstructAssistant reconstructs an Assistant from persisted data
func ReconstructAssistant(
	id, name, description, model, instructions string,
	tools []map[string]interface{},
	metadata map[string]interface{},
	createdAt, updatedAt time.Time,
) (*Assistant, error) {
	if tools == nil {
		tools = make([]map[string]interface{}, 0)
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	return &Assistant{
		id:           id,
		name:         name,
		description:  description,
		model:        model,
		instructions: instructions,
		tools:        tools,
		metadata:     metadata,
		createdAt:    createdAt,
		updatedAt:    updatedAt,
		events:       make([]eventbus.Event, 0),
	}, nil
}

// ID returns the assistant ID
func (a *Assistant) ID() string {
	return a.id
}

// Name returns the assistant name
func (a *Assistant) Name() string {
	return a.name
}

// Description returns the assistant description
func (a *Assistant) Description() string {
	return a.description
}

// Model returns the model
func (a *Assistant) Model() string {
	return a.model
}

// Instructions returns the instructions
func (a *Assistant) Instructions() string {
	return a.instructions
}

// Tools returns the tools
func (a *Assistant) Tools() []map[string]interface{} {
	return a.tools
}

// Metadata returns the metadata
func (a *Assistant) Metadata() map[string]interface{} {
	return a.metadata
}

// CreatedAt returns the creation time
func (a *Assistant) CreatedAt() time.Time {
	return a.createdAt
}

// UpdatedAt returns the last update time
func (a *Assistant) UpdatedAt() time.Time {
	return a.updatedAt
}

// Update updates the assistant
func (a *Assistant) Update(name, description, model, instructions *string, tools []map[string]interface{}) error {
	now := time.Now()

	event := AssistantUpdated{
		AssistantID: a.id,
		OccurredAt:  now,
	}

	if name != nil && *name != "" {
		a.name = *name
		event.Name = name
	}
	if description != nil {
		a.description = *description
		event.Description = description
	}
	if model != nil && *model != "" {
		a.model = *model
		event.Model = model
	}
	if instructions != nil {
		a.instructions = *instructions
		event.Instructions = instructions
	}
	if tools != nil {
		a.tools = tools
		event.Tools = tools
	}

	a.updatedAt = now
	a.recordEvent(event)

	return nil
}

// Delete marks the assistant as deleted
func (a *Assistant) Delete() error {
	now := time.Now()

	a.recordEvent(AssistantDeleted{
		AssistantID: a.id,
		OccurredAt:  now,
	})

	return nil
}

// Events returns the uncommitted events
func (a *Assistant) Events() []eventbus.Event {
	return a.events
}

// ClearEvents clears the uncommitted events
func (a *Assistant) ClearEvents() {
	a.events = make([]eventbus.Event, 0)
}

// recordEvent adds an event to the uncommitted events list
func (a *Assistant) recordEvent(event eventbus.Event) {
	a.events = append(a.events, event)
}
