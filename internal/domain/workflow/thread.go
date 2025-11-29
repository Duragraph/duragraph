package workflow

import (
	"time"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

// Message represents a message in a thread
type Message struct {
	ID        string
	Role      string // user, assistant, system
	Content   string
	Metadata  map[string]interface{}
	CreatedAt time.Time
}

// Thread represents a conversation thread aggregate
type Thread struct {
	id        string
	messages  []Message
	metadata  map[string]interface{}
	createdAt time.Time
	updatedAt time.Time

	// Uncommitted events
	events []eventbus.Event
}

// NewThread creates a new Thread aggregate
func NewThread(metadata map[string]interface{}) (*Thread, error) {
	now := time.Now()
	threadID := pkguuid.New()

	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	thread := &Thread{
		id:        threadID,
		messages:  make([]Message, 0),
		metadata:  metadata,
		createdAt: now,
		updatedAt: now,
		events:    make([]eventbus.Event, 0),
	}

	thread.recordEvent(ThreadCreated{
		ThreadID:   threadID,
		Metadata:   metadata,
		OccurredAt: now,
	})

	return thread, nil
}

// ReconstructThread reconstructs a Thread from persisted data
func ReconstructThread(
	id string,
	messages []Message,
	metadata map[string]interface{},
	createdAt, updatedAt time.Time,
) (*Thread, error) {
	if messages == nil {
		messages = make([]Message, 0)
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	return &Thread{
		id:        id,
		messages:  messages,
		metadata:  metadata,
		createdAt: createdAt,
		updatedAt: updatedAt,
		events:    make([]eventbus.Event, 0),
	}, nil
}

// ID returns the thread ID
func (t *Thread) ID() string {
	return t.id
}

// Messages returns the messages
func (t *Thread) Messages() []Message {
	return t.messages
}

// Metadata returns the metadata
func (t *Thread) Metadata() map[string]interface{} {
	return t.metadata
}

// CreatedAt returns the creation time
func (t *Thread) CreatedAt() time.Time {
	return t.createdAt
}

// UpdatedAt returns the last update time
func (t *Thread) UpdatedAt() time.Time {
	return t.updatedAt
}

// AddMessage adds a message to the thread
func (t *Thread) AddMessage(role, content string, metadata map[string]interface{}) (*Message, error) {
	if role == "" {
		return nil, errors.InvalidInput("role", "role is required")
	}
	if content == "" {
		return nil, errors.InvalidInput("content", "content is required")
	}

	// Validate role
	validRoles := map[string]bool{
		"user":      true,
		"assistant": true,
		"system":    true,
	}
	if !validRoles[role] {
		return nil, errors.InvalidInput("role", "role must be user, assistant, or system")
	}

	now := time.Now()
	messageID := pkguuid.New()

	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	message := Message{
		ID:        messageID,
		Role:      role,
		Content:   content,
		Metadata:  metadata,
		CreatedAt: now,
	}

	t.messages = append(t.messages, message)
	t.updatedAt = now

	t.recordEvent(MessageAdded{
		MessageID:  messageID,
		ThreadID:   t.id,
		Role:       role,
		Content:    content,
		Metadata:   metadata,
		OccurredAt: now,
	})

	return &message, nil
}

// UpdateMetadata updates the thread metadata
func (t *Thread) UpdateMetadata(metadata map[string]interface{}) error {
	now := time.Now()

	t.metadata = metadata
	t.updatedAt = now

	t.recordEvent(ThreadUpdated{
		ThreadID:   t.id,
		Metadata:   metadata,
		OccurredAt: now,
	})

	return nil
}

// Events returns the uncommitted events
func (t *Thread) Events() []eventbus.Event {
	return t.events
}

// ClearEvents clears the uncommitted events
func (t *Thread) ClearEvents() {
	t.events = make([]eventbus.Event, 0)
}

// recordEvent adds an event to the uncommitted events list
func (t *Thread) recordEvent(event eventbus.Event) {
	t.events = append(t.events, event)
}
