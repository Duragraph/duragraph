package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/duragraph/duragraph/internal/domain/execution"
	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

// StreamingBridge connects the in-process eventBus to NATS for real-time streaming.
// Events are published to both a global topic (duragraph.runs.run.{event_type})
// and a run-specific topic (duragraph.stream.{run_id}.{event_type}) so that
// subscribers can filter efficiently at the NATS level.
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
	b.eventBus.Subscribe(execution.EventTypeNodeStarted, b.handleNodeStarted)
	b.eventBus.Subscribe(execution.EventTypeNodeCompleted, b.handleNodeCompleted)
	b.eventBus.Subscribe(execution.EventTypeNodeFailed, b.handleNodeFailed)
	b.eventBus.Subscribe(execution.EventTypeNodeSkipped, b.handleNodeSkipped)

	// Run-level events drive the SSE stream's run lifecycle messages
	// (start, completion, HITL pause, failure). The worker handler
	// publishes these in response to worker-reported events.
	b.eventBus.Subscribe(run.EventTypeRunStarted, b.handleRunStarted)
	b.eventBus.Subscribe(run.EventTypeRunCompleted, b.handleRunCompleted)
	b.eventBus.Subscribe(run.EventTypeRunFailed, b.handleRunFailed)
	b.eventBus.Subscribe(run.EventTypeRunRequiresAction, b.handleRunRequiresAction)

	b.eventBus.Subscribe("streaming.metadata", b.handleMetadataEvent)
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

	if err := b.publishStreamEvent(ctx, nodeEvent.RunID, "node_end", map[string]interface{}{
		"node_id":     nodeEvent.NodeID,
		"node_type":   nodeEvent.NodeType,
		"output":      nodeEvent.Output,
		"duration_ms": nodeEvent.DurationMs,
		"timestamp":   nodeEvent.OccurredAt,
	}); err != nil {
		return err
	}

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

// handleRunStarted handles run started events
func (b *StreamingBridge) handleRunStarted(ctx context.Context, event eventbus.Event) error {
	runEvent, ok := event.(run.RunStarted)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, runEvent.RunID, "run_started", map[string]interface{}{
		"run_id":    runEvent.RunID,
		"timestamp": runEvent.OccurredAt,
	})
}

// handleRunCompleted handles run completed events
func (b *StreamingBridge) handleRunCompleted(ctx context.Context, event eventbus.Event) error {
	runEvent, ok := event.(run.RunCompleted)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, runEvent.RunID, "run_completed", map[string]interface{}{
		"run_id":    runEvent.RunID,
		"output":    runEvent.Output,
		"timestamp": runEvent.OccurredAt,
	})
}

// handleRunFailed handles run failed events
func (b *StreamingBridge) handleRunFailed(ctx context.Context, event eventbus.Event) error {
	runEvent, ok := event.(run.RunFailed)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, runEvent.RunID, "run_failed", map[string]interface{}{
		"run_id":    runEvent.RunID,
		"error":     runEvent.Error,
		"timestamp": runEvent.OccurredAt,
	})
}

// handleRunRequiresAction handles HITL pause events so Studio's
// ApprovalDialog can open via the SSE stream.
func (b *StreamingBridge) handleRunRequiresAction(ctx context.Context, event eventbus.Event) error {
	runEvent, ok := event.(run.RunRequiresAction)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, runEvent.RunID, "run_requires_action", map[string]interface{}{
		"run_id":       runEvent.RunID,
		"interrupt_id": runEvent.InterruptID,
		"reason":       runEvent.Reason,
		"tool_calls":   runEvent.ToolCalls,
		"timestamp":    runEvent.OccurredAt,
	})
}

// handleMetadataEvent handles the initial metadata event for LangGraph compatibility
func (b *StreamingBridge) handleMetadataEvent(ctx context.Context, event eventbus.Event) error {
	metaEvent, ok := event.(*MetadataStreamEvent)
	if !ok {
		return nil
	}

	return b.publishStreamEvent(ctx, metaEvent.RunID, "metadata", map[string]interface{}{
		"run_id":       metaEvent.RunID,
		"thread_id":    metaEvent.ThreadID,
		"assistant_id": metaEvent.AssistantID,
		"graph_id":     metaEvent.GraphID,
		"timestamp":    time.Now(),
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

// publishStreamEvent publishes an event to both global and run-specific NATS topics.
func (b *StreamingBridge) publishStreamEvent(ctx context.Context, runID, eventType string, payload map[string]interface{}) error {
	envelope := map[string]interface{}{
		"aggregate_id":   runID,
		"aggregate_type": "run",
		"event_type":     eventType,
		"payload":        payload,
		"timestamp":      time.Now(),
	}

	globalTopic := fmt.Sprintf("duragraph.runs.run.%s", eventType)
	if err := b.publisher.Publish(ctx, globalTopic, envelope); err != nil {
		return err
	}

	runTopic := fmt.Sprintf("duragraph.stream.%s.%s", runID, eventType)
	return b.publisher.Publish(ctx, runTopic, envelope)
}

// MetadataStreamEvent is emitted at the start of a run for LangGraph compatibility.
type MetadataStreamEvent struct {
	RunID       string
	ThreadID    string
	AssistantID string
	GraphID     string
}

func (e *MetadataStreamEvent) EventType() string     { return "streaming.metadata" }
func (e *MetadataStreamEvent) AggregateID() string   { return e.RunID }
func (e *MetadataStreamEvent) AggregateType() string { return "run" }

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

// EmitMetadataEvent emits the initial metadata event for a run
func EmitMetadataEvent(eventBus *eventbus.EventBus, ctx context.Context, runID, threadID, assistantID, graphID string) error {
	return eventBus.Publish(ctx, &MetadataStreamEvent{
		RunID:       runID,
		ThreadID:    threadID,
		AssistantID: assistantID,
		GraphID:     graphID,
	})
}

// EmitValuesEvent emits a values event
func EmitValuesEvent(eventBus *eventbus.EventBus, ctx context.Context, runID string, values map[string]interface{}) error {
	return eventBus.Publish(ctx, &ValuesStreamEvent{
		RunID:  runID,
		Values: values,
	})
}

// EmitMessageChunk emits a message chunk event
func EmitMessageChunk(eventBus *eventbus.EventBus, ctx context.Context, runID, content, role, id string) error {
	return eventBus.Publish(ctx, &MessageChunkStreamEvent{
		RunID:   runID,
		Content: content,
		Role:    role,
		ID:      id,
	})
}

// EmitUpdatesEvent emits an updates event
func EmitUpdatesEvent(eventBus *eventbus.EventBus, ctx context.Context, runID, nodeID string, delta map[string]interface{}) error {
	return eventBus.Publish(ctx, &UpdatesStreamEvent{
		RunID:  runID,
		NodeID: nodeID,
		Delta:  delta,
	})
}

// EmitDebugEvent emits a debug event
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
