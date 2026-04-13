//go:build integration

package nats_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	dgNats "github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
	natsgo "github.com/nats-io/nats.go"
)

func natsURL() string {
	if u := os.Getenv("TEST_NATS_URL"); u != "" {
		return u
	}
	return "nats://127.0.0.1:4223"
}

func TestTaskQueue_PublishAndSubscribe(t *testing.T) {
	url := natsURL()

	queue, err := dgNats.NewTaskQueue(url)
	if err != nil {
		t.Fatalf("NewTaskQueue: %v", err)
	}
	defer queue.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	graphID := fmt.Sprintf("test-graph-%d", time.Now().UnixNano())
	received := make(chan dgNats.TaskMessage, 1)

	err = queue.Subscribe(ctx, "test-consumer", []string{graphID}, func(ctx context.Context, msg dgNats.TaskMessage) error {
		received <- msg
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	taskMsg := dgNats.TaskMessage{
		TaskID:      42,
		RunID:       "run-123",
		GraphID:     graphID,
		ThreadID:    "thread-456",
		AssistantID: "asst-789",
		Input:       map[string]interface{}{"msg": "hello"},
		CreatedAt:   time.Now(),
	}

	if err := queue.Publish(ctx, taskMsg); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case msg := <-received:
		if msg.TaskID != 42 {
			t.Errorf("taskID = %d, want 42", msg.TaskID)
		}
		if msg.RunID != "run-123" {
			t.Errorf("runID = %q", msg.RunID)
		}
		if msg.GraphID != graphID {
			t.Errorf("graphID = %q", msg.GraphID)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for message")
	}
}

func TestTaskQueue_PublishRunEvent(t *testing.T) {
	url := natsURL()

	queue, err := dgNats.NewTaskQueue(url)
	if err != nil {
		t.Fatalf("NewTaskQueue: %v", err)
	}
	defer queue.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	received := make(chan map[string]interface{}, 1)

	err = queue.SubscribeRunEvents(ctx, runID, func(eventType string, data map[string]interface{}) {
		result := map[string]interface{}{
			"event_type": eventType,
			"data":       data,
		}
		received <- result
	})
	if err != nil {
		t.Fatalf("SubscribeRunEvents: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	data := map[string]interface{}{"status": "completed"}
	if err := queue.PublishRunEvent(ctx, runID, "completed", data); err != nil {
		t.Fatalf("PublishRunEvent: %v", err)
	}

	select {
	case msg := <-received:
		if msg["event_type"] != "completed" {
			t.Errorf("eventType = %v", msg["event_type"])
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for run event")
	}
}

func TestTaskQueue_MultipleGraphSubscription(t *testing.T) {
	url := natsURL()

	queue, err := dgNats.NewTaskQueue(url)
	if err != nil {
		t.Fatalf("NewTaskQueue: %v", err)
	}
	defer queue.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	graph1 := fmt.Sprintf("graph-a-%d", time.Now().UnixNano())
	graph2 := fmt.Sprintf("graph-b-%d", time.Now().UnixNano())
	received := make(chan dgNats.TaskMessage, 2)

	err = queue.Subscribe(ctx, "multi-consumer", []string{graph1, graph2}, func(ctx context.Context, msg dgNats.TaskMessage) error {
		received <- msg
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	queue.Publish(ctx, dgNats.TaskMessage{TaskID: 1, GraphID: graph1, RunID: "r1", CreatedAt: time.Now()})
	queue.Publish(ctx, dgNats.TaskMessage{TaskID: 2, GraphID: graph2, RunID: "r2", CreatedAt: time.Now()})

	count := 0
	timeout := time.After(5 * time.Second)
	for count < 2 {
		select {
		case <-received:
			count++
		case <-timeout:
			t.Fatalf("received %d/2 messages before timeout", count)
		}
	}
}

func TestNatsRawPubSub(t *testing.T) {
	url := natsURL()

	nc, err := natsgo.Connect(url)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer nc.Close()

	subject := fmt.Sprintf("test.integration.%d", time.Now().UnixNano())
	received := make(chan []byte, 1)

	sub, err := nc.Subscribe(subject, func(msg *natsgo.Msg) {
		received <- msg.Data
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	payload := map[string]string{"hello": "world"}
	data, _ := json.Marshal(payload)
	nc.Publish(subject, data)

	select {
	case msg := <-received:
		var result map[string]string
		json.Unmarshal(msg, &result)
		if result["hello"] != "world" {
			t.Errorf("got %v", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}
