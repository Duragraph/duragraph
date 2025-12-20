package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/duragraph/duragraph/internal/domain/execution"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

// StreamingBridge connects the in-process eventBus to NATS for real-time streaming
type StreamingBridge struct {
	eventBus  *eventbus.EventBus
	publisher *nats.Publisher
}

// NewStreamingBridge creates a new streaming bridge
func NewStreamingBridge(eventBus *eventbus.EventBus, publisher *nats.Publisher) *StreamingBridge {
	return &StreamingBridge{
		eventBus:  eventBus,
		publisher: publisher,
	}
}

// Start registers event handlers and starts the bridge
func (b *StreamingBridge) Start() {
	// Subscribe to node execution events
	b.eventBus.Subscribe(execution.EventTypeNodeStarted, b.handleNodeStarted)
	b.eventBus.Subscribe(execution.EventTypeNodeCompleted, b.handleNodeCompleted)
	b.eventBus.Subscribe(execution.EventTypeNodeFailed, b.handleNodeFailed)
	b.eventBus.Subscribe(execution.EventTypeNodeSkipped, b.handleNodeSkipped)

	// Subscribe to streaming-specific events
	b.eventBus.Subscribe("streaming.values", b.handleValuesEvent)
	b.eventBus.Subscribe("streaming.message_chunk", b.handleMessageChunk)
	b.eventBus.Subscribe("streaming.updates", b.handleUpdatesEvent)
	b.eventBus.Subscribe("streaming.debug", b.handleDebugEvent)
}

// handleNodeStarted handles node started events
func (b *StreamingBridge) handleNodeStarted(ctx context.Context, event eventbus.Event) error {
	nodeEvent, ok := event.(execution.NodeStarted)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, nodeEvent.RunID, "node_start", map[string]interface{}{
		"node_id":   nodeEvent.NodeID,
		"node_type": nodeEvent.NodeType,
		"input":     nodeEvent.Input,
		"timestamp": nodeEvent.OccurredAt,
	})
}

// handleNodeCompleted handles node completed events
func (b *StreamingBridge) handleNodeCompleted(ctx context.Context, event eventbus.Event) error {
	nodeEvent, ok := event.(execution.NodeCompleted)
	if !ok {
		return nil
	}

	// Publish as both node_end and values event
	if err := b.publishStreamEvent(ctx, nodeEvent.RunID, "node_end", map[string]interface{}{
		"node_id":     nodeEvent.NodeID,
		"node_type":   nodeEvent.NodeType,
		"output":      nodeEvent.Output,
		"duration_ms": nodeEvent.DurationMs,
		"timestamp":   nodeEvent.OccurredAt,
	}); err != nil {
		return err
	}

	// Also publish as values event for values streaming mode
	return b.publishStreamEvent(ctx, nodeEvent.RunID, "values", map[string]interface{}{
		"values":    nodeEvent.Output,
		"node_id":   nodeEvent.NodeID,
		"timestamp": nodeEvent.OccurredAt,
	})
}

// handleNodeFailed handles node failed events
func (b *StreamingBridge) handleNodeFailed(ctx context.Context, event eventbus.Event) error {
	nodeEvent, ok := event.(execution.NodeFailed)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, nodeEvent.RunID, "error", map[string]interface{}{
		"node_id":   nodeEvent.NodeID,
		"node_type": nodeEvent.NodeType,
		"error":     nodeEvent.Error,
		"input":     nodeEvent.Input,
		"timestamp": nodeEvent.OccurredAt,
	})
}

// handleNodeSkipped handles node skipped events
func (b *StreamingBridge) handleNodeSkipped(ctx context.Context, event eventbus.Event) error {
	nodeEvent, ok := event.(execution.NodeSkipped)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, nodeEvent.RunID, "node_skipped", map[string]interface{}{
		"node_id":   nodeEvent.NodeID,
		"node_type": nodeEvent.NodeType,
		"reason":    nodeEvent.Reason,
		"timestamp": nodeEvent.OccurredAt,
	})
}

// handleValuesEvent handles explicit values streaming events
func (b *StreamingBridge) handleValuesEvent(ctx context.Context, event eventbus.Event) error {
	valuesEvent, ok := event.(*ValuesStreamEvent)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, valuesEvent.RunID, "values", map[string]interface{}{
		"values":    valuesEvent.Values,
		"timestamp": time.Now(),
	})
}

// handleMessageChunk handles LLM token streaming events
func (b *StreamingBridge) handleMessageChunk(ctx context.Context, event eventbus.Event) error {
	chunkEvent, ok := event.(*MessageChunkStreamEvent)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, chunkEvent.RunID, "message_chunk", map[string]interface{}{
		"content":   chunkEvent.Content,
		"role":      chunkEvent.Role,
		"id":        chunkEvent.ID,
		"timestamp": time.Now(),
	})
}

// handleUpdatesEvent handles state delta/updates events
func (b *StreamingBridge) handleUpdatesEvent(ctx context.Context, event eventbus.Event) error {
	updateEvent, ok := event.(*UpdatesStreamEvent)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, updateEvent.RunID, "updates", map[string]interface{}{
		"delta":     updateEvent.Delta,
		"node_id":   updateEvent.NodeID,
		"timestamp": time.Now(),
	})
}

// handleDebugEvent handles debug streaming events
func (b *StreamingBridge) handleDebugEvent(ctx context.Context, event eventbus.Event) error {
	debugEvent, ok := event.(*DebugStreamEvent)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, debugEvent.RunID, "debug", map[string]interface{}{
		"level":     debugEvent.Level,
		"message":   debugEvent.Message,
		"data":      debugEvent.Data,
		"timestamp": time.Now(),
	})
}

// publishStreamEvent publishes an event to NATS for streaming
func (b *StreamingBridge) publishStreamEvent(ctx context.Context, runID, eventType string, payload map[string]interface{}) error {
	topic := fmt.Sprintf("duragraph.runs.run.%s", eventType)

	envelope := map[string]interface{}{
		"aggregate_id":   runID,
		"aggregate_type": "run",
		"event_type":     eventType,
		"payload":        payload,
		"timestamp":      time.Now(),
	}

	return b.publisher.Publish(ctx, topic, envelope)
}

// Streaming event types for eventBus

// ValuesStreamEvent represents a values streaming event
type ValuesStreamEvent struct {
	RunID  string
	Values map[string]interface{}
}

func (e *ValuesStreamEvent) EventType() string     { return "streaming.values" }
func (e *ValuesStreamEvent) AggregateID() string   { return e.RunID }
func (e *ValuesStreamEvent) AggregateType() string { return "run" }

// MessageChunkStreamEvent represents an LLM token chunk
type MessageChunkStreamEvent struct {
	RunID   string
	Content string
	Role    string
	ID      string
}

func (e *MessageChunkStreamEvent) EventType() string     { return "streaming.message_chunk" }
func (e *MessageChunkStreamEvent) AggregateID() string   { return e.RunID }
func (e *MessageChunkStreamEvent) AggregateType() string { return "run" }

// UpdatesStreamEvent represents a state delta/update
type UpdatesStreamEvent struct {
	RunID  string
	NodeID string
	Delta  map[string]interface{}
}

func (e *UpdatesStreamEvent) EventType() string     { return "streaming.updates" }
func (e *UpdatesStreamEvent) AggregateID() string   { return e.RunID }
func (e *UpdatesStreamEvent) AggregateType() string { return "run" }

// DebugStreamEvent represents a debug event
type DebugStreamEvent struct {
	RunID   string
	Level   string
	Message string
	Data    map[string]interface{}
}

func (e *DebugStreamEvent) EventType() string     { return "streaming.debug" }
func (e *DebugStreamEvent) AggregateID() string   { return e.RunID }
func (e *DebugStreamEvent) AggregateType() string { return "run" }

// Helper to emit values event
func EmitValuesEvent(eventBus *eventbus.EventBus, ctx context.Context, runID string, values map[string]interface{}) error {
	return eventBus.Publish(ctx, &ValuesStreamEvent{
		RunID:  runID,
		Values: values,
	})
}

// Helper to emit message chunk event
func EmitMessageChunk(eventBus *eventbus.EventBus, ctx context.Context, runID, content, role, id string) error {
	return eventBus.Publish(ctx, &MessageChunkStreamEvent{
		RunID:   runID,
		Content: content,
		Role:    role,
		ID:      id,
	})
}

// Helper to emit updates event
func EmitUpdatesEvent(eventBus *eventbus.EventBus, ctx context.Context, runID, nodeID string, delta map[string]interface{}) error {
	return eventBus.Publish(ctx, &UpdatesStreamEvent{
		RunID:  runID,
		NodeID: nodeID,
		Delta:  delta,
	})
}

// Helper to emit debug event
func EmitDebugEvent(eventBus *eventbus.EventBus, ctx context.Context, runID, level, message string, data map[string]interface{}) error {
	return eventBus.Publish(ctx, &DebugStreamEvent{
		RunID:   runID,
		Level:   level,
		Message: message,
		Data:    data,
	})
}

// SerializeEvent serializes an event to JSON for debugging
func SerializeEvent(event interface{}) ([]byte, error) {
	return json.Marshal(event)
}
