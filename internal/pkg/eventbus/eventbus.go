package eventbus

import (
	"context"
	"sync"
)

// Event is the interface that all domain events must implement
type Event interface {
	EventType() string
	AggregateID() string
	AggregateType() string
}

// Handler is a function that handles an event
type Handler func(ctx context.Context, event Event) error

// EventBus is an in-process event bus for domain events
type EventBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// New creates a new EventBus
func New() *EventBus {
	return &EventBus{
		handlers: make(map[string][]Handler),
	}
}

// Subscribe registers a handler for a specific event type
func (b *EventBus) Subscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Publish publishes an event to all registered handlers
func (b *EventBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	handlers := b.handlers[event.EventType()]
	b.mu.RUnlock()

	var wg sync.WaitGroup
	errCh := make(chan error, len(handlers))

	for _, handler := range handlers {
		wg.Add(1)
		go func(h Handler) {
			defer wg.Done()
			if err := h(ctx, event); err != nil {
				errCh <- err
			}
		}(handler)
	}

	wg.Wait()
	close(errCh)

	// Collect errors
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		// Return first error (could be enhanced to return multi-error)
		return errors[0]
	}

	return nil
}

// PublishSync publishes an event synchronously to all registered handlers
func (b *EventBus) PublishSync(ctx context.Context, event Event) error {
	b.mu.RLock()
	handlers := b.handlers[event.EventType()]
	b.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			return err
		}
	}

	return nil
}

// Unsubscribe removes all handlers for a specific event type (for testing)
func (b *EventBus) Unsubscribe(eventType string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.handlers, eventType)
}

// Clear removes all handlers (for testing)
func (b *EventBus) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers = make(map[string][]Handler)
}
