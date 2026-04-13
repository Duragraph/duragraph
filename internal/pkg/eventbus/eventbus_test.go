package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testEvent struct {
	eventType     string
	aggregateID   string
	aggregateType string
}

func (e testEvent) EventType() string     { return e.eventType }
func (e testEvent) AggregateID() string   { return e.aggregateID }
func (e testEvent) AggregateType() string { return e.aggregateType }

func TestNew(t *testing.T) {
	bus := New()
	if bus == nil {
		t.Fatal("New() should return a non-nil EventBus")
	}
	if bus.handlers == nil {
		t.Fatal("handlers map should be initialized")
	}
}

func TestSubscribeAndPublish(t *testing.T) {
	bus := New()
	var received bool

	bus.Subscribe("test.event", func(ctx context.Context, event Event) error {
		received = true
		if event.EventType() != "test.event" {
			t.Errorf("expected event type test.event, got %s", event.EventType())
		}
		return nil
	})

	err := bus.Publish(context.Background(), testEvent{
		eventType:     "test.event",
		aggregateID:   "123",
		aggregateType: "test",
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if !received {
		t.Error("handler was not called")
	}
}

func TestPublish_MultipleHandlers(t *testing.T) {
	bus := New()
	var count int32

	for i := 0; i < 5; i++ {
		bus.Subscribe("multi.event", func(ctx context.Context, event Event) error {
			atomic.AddInt32(&count, 1)
			return nil
		})
	}

	err := bus.Publish(context.Background(), testEvent{eventType: "multi.event", aggregateID: "1", aggregateType: "t"})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if atomic.LoadInt32(&count) != 5 {
		t.Errorf("expected 5 handlers called, got %d", count)
	}
}

func TestPublish_NoHandlers(t *testing.T) {
	bus := New()
	err := bus.Publish(context.Background(), testEvent{eventType: "no.handlers", aggregateID: "1", aggregateType: "t"})
	if err != nil {
		t.Errorf("Publish with no handlers should not error, got: %v", err)
	}
}

func TestPublish_HandlerError(t *testing.T) {
	bus := New()
	bus.Subscribe("err.event", func(ctx context.Context, event Event) error {
		return fmt.Errorf("handler failed")
	})

	err := bus.Publish(context.Background(), testEvent{eventType: "err.event", aggregateID: "1", aggregateType: "t"})
	if err == nil {
		t.Error("Publish should return error when handler fails")
	}
}

func TestPublishSync(t *testing.T) {
	bus := New()
	order := make([]int, 0, 3)
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		idx := i
		bus.Subscribe("sync.event", func(ctx context.Context, event Event) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, idx)
			return nil
		})
	}

	err := bus.PublishSync(context.Background(), testEvent{eventType: "sync.event", aggregateID: "1", aggregateType: "t"})
	if err != nil {
		t.Fatalf("PublishSync returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 {
		t.Fatalf("expected 3 handlers called, got %d", len(order))
	}
	for i, v := range order {
		if v != i {
			t.Errorf("expected sequential order, at index %d got %d", i, v)
		}
	}
}

func TestPublishSync_HandlerError_StopsEarly(t *testing.T) {
	bus := New()
	var count int32

	bus.Subscribe("sync.err", func(ctx context.Context, event Event) error {
		atomic.AddInt32(&count, 1)
		return fmt.Errorf("fail")
	})
	bus.Subscribe("sync.err", func(ctx context.Context, event Event) error {
		atomic.AddInt32(&count, 1)
		return nil
	})

	err := bus.PublishSync(context.Background(), testEvent{eventType: "sync.err", aggregateID: "1", aggregateType: "t"})
	if err == nil {
		t.Error("PublishSync should return error when handler fails")
	}
	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("expected 1 handler called (early stop), got %d", count)
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := New()
	var called bool

	bus.Subscribe("unsub.event", func(ctx context.Context, event Event) error {
		called = true
		return nil
	})
	bus.Unsubscribe("unsub.event")

	err := bus.Publish(context.Background(), testEvent{eventType: "unsub.event", aggregateID: "1", aggregateType: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("handler should not be called after Unsubscribe")
	}
}

func TestClear(t *testing.T) {
	bus := New()
	var count int32

	bus.Subscribe("a", func(ctx context.Context, event Event) error {
		atomic.AddInt32(&count, 1)
		return nil
	})
	bus.Subscribe("b", func(ctx context.Context, event Event) error {
		atomic.AddInt32(&count, 1)
		return nil
	})
	bus.Clear()

	_ = bus.Publish(context.Background(), testEvent{eventType: "a", aggregateID: "1", aggregateType: "t"})
	_ = bus.Publish(context.Background(), testEvent{eventType: "b", aggregateID: "1", aggregateType: "t"})

	if atomic.LoadInt32(&count) != 0 {
		t.Errorf("expected 0 handlers called after Clear, got %d", count)
	}
}

func TestConcurrentSubscribeAndPublish(t *testing.T) {
	bus := New()
	var count int64
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			bus.Subscribe("concurrent.event", func(ctx context.Context, event Event) error {
				atomic.AddInt64(&count, 1)
				return nil
			})
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond)
			_ = bus.Publish(context.Background(), testEvent{eventType: "concurrent.event", aggregateID: "1", aggregateType: "t"})
		}()
	}

	wg.Wait()

	if atomic.LoadInt64(&count) == 0 {
		t.Error("expected at least some handlers to be called")
	}
}

func TestPublish_DifferentEventTypes(t *testing.T) {
	bus := New()
	var aCalled, bCalled bool

	bus.Subscribe("type.a", func(ctx context.Context, event Event) error {
		aCalled = true
		return nil
	})
	bus.Subscribe("type.b", func(ctx context.Context, event Event) error {
		bCalled = true
		return nil
	})

	_ = bus.Publish(context.Background(), testEvent{eventType: "type.a", aggregateID: "1", aggregateType: "t"})

	if !aCalled {
		t.Error("type.a handler should have been called")
	}
	if bCalled {
		t.Error("type.b handler should NOT have been called")
	}
}
