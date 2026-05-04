package mocks

import (
	"context"
	"sync"
)

// PublishedEvent is one captured publish() call in the EventPublisher
// recorder.
type PublishedEvent struct {
	Topic   string
	Payload interface{}
}

// EventPublisher is a recorder that satisfies the
// command.EventPublisher interface. Stores every Publish() call for
// test assertions; an optional Func override lets a test inject
// failure.
type EventPublisher struct {
	mu          sync.Mutex
	Events      []PublishedEvent
	PublishFunc func(ctx context.Context, topic string, payload interface{}) error
}

// NewEventPublisher constructs a fresh recorder.
func NewEventPublisher() *EventPublisher {
	return &EventPublisher{Events: make([]PublishedEvent, 0)}
}

// Publish records the call and returns the result of PublishFunc if
// set, else nil.
func (p *EventPublisher) Publish(ctx context.Context, topic string, payload interface{}) error {
	p.mu.Lock()
	p.Events = append(p.Events, PublishedEvent{Topic: topic, Payload: payload})
	p.mu.Unlock()
	if p.PublishFunc != nil {
		return p.PublishFunc(ctx, topic, payload)
	}
	return nil
}

// Count returns how many Publish() calls have been recorded.
func (p *EventPublisher) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.Events)
}
