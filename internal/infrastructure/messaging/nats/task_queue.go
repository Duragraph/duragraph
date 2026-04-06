package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

// TaskMessage represents a task published to the NATS task queue.
type TaskMessage struct {
	TaskID      int64                  `json:"task_id"`
	RunID       string                 `json:"run_id"`
	GraphID     string                 `json:"graph_id"`
	ThreadID    string                 `json:"thread_id"`
	AssistantID string                 `json:"assistant_id"`
	Input       map[string]interface{} `json:"input"`
	Config      map[string]interface{} `json:"config"`
	CreatedAt   time.Time              `json:"created_at"`
}

// TaskQueue provides NATS JetStream-based task notification.
// PostgreSQL remains the source of truth; NATS is the notification layer.
type TaskQueue struct {
	nc     *natsgo.Conn
	js     natsgo.JetStreamContext
	stream string
}

// NewTaskQueue creates a new NATS JetStream task queue.
func NewTaskQueue(natsURL string) (*TaskQueue, error) {
	nc, err := natsgo.Connect(natsURL,
		natsgo.RetryOnFailedConnect(true),
		natsgo.MaxReconnects(-1),
		natsgo.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	streamName := "duragraph-tasks"

	_, err = js.StreamInfo(streamName)
	if err != nil {
		_, err = js.AddStream(&natsgo.StreamConfig{
			Name:      streamName,
			Subjects:  []string{"duragraph.tasks.>"},
			Storage:   natsgo.FileStorage,
			Replicas:  1,
			Retention: natsgo.WorkQueuePolicy,
			MaxAge:    24 * time.Hour,
		})
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to create tasks stream: %w", err)
		}
	}

	return &TaskQueue{
		nc:     nc,
		js:     js,
		stream: streamName,
	}, nil
}

// Publish sends a task notification so workers know work is available.
func (q *TaskQueue) Publish(ctx context.Context, msg TaskMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal task message: %w", err)
	}

	subject := fmt.Sprintf("duragraph.tasks.assign.%s", msg.GraphID)

	_, err = q.js.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish task: %w", err)
	}

	return nil
}

// SubscribeHandler is called when a task notification is received.
type SubscribeHandler func(ctx context.Context, msg TaskMessage) error

// Subscribe listens for task notifications for the given graph IDs.
func (q *TaskQueue) Subscribe(ctx context.Context, consumerGroup string, graphIDs []string, handler SubscribeHandler) error {
	for _, graphID := range graphIDs {
		subject := fmt.Sprintf("duragraph.tasks.assign.%s", graphID)
		durableName := fmt.Sprintf("%s-%s", consumerGroup, graphID)

		sub, err := q.js.Subscribe(subject, func(m *natsgo.Msg) {
			var taskMsg TaskMessage
			if err := json.Unmarshal(m.Data, &taskMsg); err != nil {
				log.Printf("failed to unmarshal task message: %v", err)
				m.Nak()
				return
			}

			if err := handler(ctx, taskMsg); err != nil {
				log.Printf("failed to handle task %d: %v", taskMsg.TaskID, err)
				m.Nak()
				return
			}

			m.Ack()
		},
			natsgo.Durable(durableName),
			natsgo.ManualAck(),
			natsgo.AckWait(30*time.Second),
			natsgo.MaxDeliver(3),
		)
		if err != nil {
			return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
		}

		go func(s *natsgo.Subscription) {
			<-ctx.Done()
			s.Unsubscribe()
		}(sub)
	}

	return nil
}

// PublishRunEvent sends a run lifecycle event to NATS for real-time notification.
func (q *TaskQueue) PublishRunEvent(ctx context.Context, runID, eventType string, data map[string]interface{}) error {
	payload := map[string]interface{}{
		"run_id":     runID,
		"event_type": eventType,
		"data":       data,
		"timestamp":  time.Now(),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal run event: %w", err)
	}

	subject := fmt.Sprintf("duragraph.runs.%s.%s", runID, eventType)
	_, err = q.js.Publish(subject, jsonData)
	return err
}

// SubscribeRunEvents subscribes to run lifecycle events for a specific run.
func (q *TaskQueue) SubscribeRunEvents(ctx context.Context, runID string, handler func(eventType string, data map[string]interface{})) error {
	subject := fmt.Sprintf("duragraph.runs.%s.>", runID)

	sub, err := q.js.Subscribe(subject, func(m *natsgo.Msg) {
		var payload map[string]interface{}
		if err := json.Unmarshal(m.Data, &payload); err != nil {
			m.Ack()
			return
		}

		eventType, _ := payload["event_type"].(string)
		data, _ := payload["data"].(map[string]interface{})
		handler(eventType, data)
		m.Ack()
	}, natsgo.OrderedConsumer())

	if err != nil {
		return fmt.Errorf("failed to subscribe to run events: %w", err)
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()

	return nil
}

func (q *TaskQueue) Close() error {
	q.nc.Close()
	return nil
}
