package streaming

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

func TestParseStreamModes_Default(t *testing.T) {
	modes := ParseStreamModes(nil)
	if len(modes) != 1 || modes[0] != ModeEvents {
		t.Errorf("expected [events], got %v", modes)
	}
}

func TestParseStreamModes_Single(t *testing.T) {
	modes := ParseStreamModes([]string{"values"})
	if len(modes) != 1 || modes[0] != ModeValues {
		t.Errorf("expected [values], got %v", modes)
	}
}

func TestParseStreamModes_Multiple(t *testing.T) {
	modes := ParseStreamModes([]string{"values", "updates"})
	if len(modes) != 2 {
		t.Fatalf("expected 2 modes, got %d", len(modes))
	}
	if modes[0] != ModeValues || modes[1] != ModeUpdates {
		t.Errorf("expected [values updates], got %v", modes)
	}
}

func TestParseStreamModes_CommaSeparated(t *testing.T) {
	modes := ParseStreamModes([]string{"values,messages"})
	if len(modes) != 2 {
		t.Fatalf("expected 2 modes, got %d", len(modes))
	}
	if modes[0] != ModeValues || modes[1] != ModeMessages {
		t.Errorf("expected [values messages], got %v", modes)
	}
}

func TestParseStreamModes_Dedup(t *testing.T) {
	modes := ParseStreamModes([]string{"values", "values"})
	if len(modes) != 1 {
		t.Errorf("expected 1 mode after dedup, got %d", len(modes))
	}
}

func TestParseStreamModes_Invalid(t *testing.T) {
	modes := ParseStreamModes([]string{"invalid"})
	if len(modes) != 1 || modes[0] != ModeEvents {
		t.Errorf("invalid mode should fall back to events, got %v", modes)
	}
}

func TestEventFormatter_ShouldSend_EventsMode(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeEvents})

	for _, eventType := range []string{"values", "updates", "message", "debug", "node_start", "metadata"} {
		if !f.ShouldSend(eventType) {
			t.Errorf("events mode should send %q", eventType)
		}
	}
}

func TestEventFormatter_ShouldSend_ValuesMode(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeValues})

	if !f.ShouldSend("values") {
		t.Error("values mode should send values")
	}
	if !f.ShouldSend("state") {
		t.Error("values mode should send state")
	}
	if !f.ShouldSend("metadata") {
		t.Error("metadata should always be sent")
	}
	if f.ShouldSend("message_chunk") {
		t.Error("values mode should not send message_chunk")
	}
	if f.ShouldSend("updates") {
		t.Error("values mode should not send updates")
	}
}

func TestEventFormatter_ShouldSend_MessagesMode(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeMessages})

	if !f.ShouldSend("message") {
		t.Error("messages mode should send message")
	}
	if !f.ShouldSend("message_chunk") {
		t.Error("messages mode should send message_chunk")
	}
	if f.ShouldSend("values") {
		t.Error("messages mode should not send values")
	}
}

func TestEventFormatter_ShouldSend_UpdatesMode(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeUpdates})

	if !f.ShouldSend("updates") {
		t.Error("updates mode should send updates")
	}
	if !f.ShouldSend("delta") {
		t.Error("updates mode should send delta")
	}
	if f.ShouldSend("values") {
		t.Error("updates mode should not send values")
	}
}

func TestEventFormatter_ShouldSend_MetadataAlwaysSent(t *testing.T) {
	for _, mode := range []StreamMode{ModeValues, ModeMessages, ModeUpdates, ModeDebug, ModeEvents} {
		f := NewEventFormatter([]StreamMode{mode})
		if !f.ShouldSend("metadata") {
			t.Errorf("metadata should always be sent in %s mode", mode)
		}
	}
}

func TestEventFormatter_FormatSSE(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeEvents})

	data, err := f.FormatSSE("values", map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("FormatSSE error: %v", err)
	}

	expected := "event: values\ndata: {\"key\":\"val\"}\n\n"
	if string(data) != expected {
		t.Errorf("got %q, want %q", string(data), expected)
	}
}

func TestEventFormatter_FormatEnd(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeEvents})

	data, err := f.FormatEnd("run-123")
	if err != nil {
		t.Fatalf("FormatEnd error: %v", err)
	}

	got := string(data)
	if got != "event: end\ndata: {\"run_id\":\"run-123\"}\n\n" {
		t.Errorf("got %q", got)
	}
}

func TestEventFormatter_FormatError(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeEvents})

	data, err := f.FormatError("something broke", "INTERNAL")
	if err != nil {
		t.Fatalf("FormatError error: %v", err)
	}

	got := string(data)
	if got != "event: error\ndata: {\"code\":\"INTERNAL\",\"message\":\"something broke\"}\n\n" {
		t.Errorf("got %q", got)
	}
}

func TestMetadataStreamEvent_Interface(t *testing.T) {
	e := &MetadataStreamEvent{
		RunID:       "r1",
		ThreadID:    "t1",
		AssistantID: "a1",
		GraphID:     "g1",
	}

	if e.EventType() != "streaming.metadata" {
		t.Errorf("EventType = %q", e.EventType())
	}
	if e.AggregateID() != "r1" {
		t.Errorf("AggregateID = %q", e.AggregateID())
	}
	if e.AggregateType() != "run" {
		t.Errorf("AggregateType = %q", e.AggregateType())
	}
}

func TestValuesStreamEvent_Interface(t *testing.T) {
	e := &ValuesStreamEvent{RunID: "r1", Values: map[string]interface{}{"k": "v"}}
	if e.EventType() != "streaming.values" {
		t.Errorf("EventType = %q", e.EventType())
	}
}

func TestMessageChunkStreamEvent_Interface(t *testing.T) {
	e := &MessageChunkStreamEvent{RunID: "r1", Content: "hello", Role: "assistant", ID: "m1"}
	if e.EventType() != "streaming.message_chunk" {
		t.Errorf("EventType = %q", e.EventType())
	}
}

func TestUpdatesStreamEvent_Interface(t *testing.T) {
	e := &UpdatesStreamEvent{RunID: "r1", NodeID: "n1", Delta: map[string]interface{}{}}
	if e.EventType() != "streaming.updates" {
		t.Errorf("EventType = %q", e.EventType())
	}
}

func TestDebugStreamEvent_Interface(t *testing.T) {
	e := &DebugStreamEvent{RunID: "r1", Level: "info", Message: "test"}
	if e.EventType() != "streaming.debug" {
		t.Errorf("EventType = %q", e.EventType())
	}
}

func TestIsValidMode(t *testing.T) {
	valid := []StreamMode{ModeValues, ModeMessages, ModeUpdates, ModeDebug, ModeEvents}
	for _, m := range valid {
		if !isValidMode(m) {
			t.Errorf("%q should be valid", m)
		}
	}
	if isValidMode("invalid") {
		t.Error("'invalid' should not be valid")
	}
}

func TestEventFormatter_FormatStateEvent(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeValues})

	data, err := f.FormatStateEvent(map[string]interface{}{"count": 42})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	got := string(data)
	if !strings.HasPrefix(got, "event: values\n") {
		t.Errorf("expected values event, got %q", got)
	}
	if !strings.Contains(got, `"count":42`) {
		t.Errorf("expected count in data, got %q", got)
	}
}

func TestEventFormatter_FormatMessageChunk(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeMessages})

	data, err := f.FormatMessageChunk("Hello", "assistant", "msg-1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	got := string(data)
	if !strings.HasPrefix(got, "event: message_chunk\n") {
		t.Errorf("expected message_chunk event, got %q", got)
	}
	if !strings.Contains(got, `"content":"Hello"`) {
		t.Errorf("expected content in data, got %q", got)
	}
}

func TestEventFormatter_FormatNodeStart(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeEvents})

	data, err := f.FormatNodeStart("node-1", "llm", map[string]interface{}{"key": "val"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	got := string(data)
	if !strings.HasPrefix(got, "event: node_start\n") {
		t.Errorf("expected node_start, got %q", got)
	}
	if !strings.Contains(got, `"node_id":"node-1"`) {
		t.Errorf("expected node_id, got %q", got)
	}
}

func TestEventFormatter_FormatNodeEnd(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeEvents})

	data, err := f.FormatNodeEnd("node-1", "llm", map[string]interface{}{"result": "ok"}, 150)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	got := string(data)
	if !strings.HasPrefix(got, "event: node_end\n") {
		t.Errorf("expected node_end, got %q", got)
	}
	if !strings.Contains(got, `"duration_ms":150`) {
		t.Errorf("expected duration, got %q", got)
	}
}

func TestEventFormatter_FormatDebug(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeDebug})

	data, err := f.FormatDebug("info", "processing node", map[string]interface{}{"node": "n1"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	got := string(data)
	if !strings.HasPrefix(got, "event: debug\n") {
		t.Errorf("expected debug, got %q", got)
	}
	if !strings.Contains(got, `"level":"info"`) {
		t.Errorf("expected level, got %q", got)
	}
}

func TestEventFormatter_ShouldSend_MultipleModesOR(t *testing.T) {
	f := NewEventFormatter([]StreamMode{ModeValues, ModeMessages})

	if !f.ShouldSend("values") {
		t.Error("should send values")
	}
	if !f.ShouldSend("message") {
		t.Error("should send message")
	}
	if !f.ShouldSend("end") {
		t.Error("should send end")
	}
}

func TestEventFormatter_ShouldSend_NoModes(t *testing.T) {
	f := NewEventFormatter(nil)

	if f.ShouldSend("values") {
		t.Error("no modes should not send values")
	}
	if !f.ShouldSend("metadata") {
		t.Error("metadata should always be sent")
	}
}

func TestEmitMetadataEvent(t *testing.T) {
	eb := eventbus.New()
	var received bool
	eb.Subscribe("streaming.metadata", func(ctx context.Context, event eventbus.Event) error {
		received = true
		meta, ok := event.(*MetadataStreamEvent)
		if !ok {
			t.Errorf("expected MetadataStreamEvent, got %T", event)
		}
		if meta.RunID != "run-1" || meta.ThreadID != "t-1" {
			t.Errorf("unexpected event: %+v", meta)
		}
		return nil
	})

	err := EmitMetadataEvent(eb, context.Background(), "run-1", "t-1", "a-1", "g-1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !received {
		t.Error("event not received")
	}
}

func TestEmitValuesEvent(t *testing.T) {
	eb := eventbus.New()
	var received bool
	eb.Subscribe("streaming.values", func(ctx context.Context, event eventbus.Event) error {
		received = true
		ve, ok := event.(*ValuesStreamEvent)
		if !ok {
			t.Errorf("expected ValuesStreamEvent, got %T", event)
		}
		if ve.RunID != "run-1" {
			t.Errorf("run_id: %q", ve.RunID)
		}
		if ve.Values["key"] != "val" {
			t.Errorf("values: %v", ve.Values)
		}
		return nil
	})

	EmitValuesEvent(eb, context.Background(), "run-1", map[string]interface{}{"key": "val"})
	if !received {
		t.Error("event not received")
	}
}

func TestEmitMessageChunk(t *testing.T) {
	eb := eventbus.New()
	var received bool
	eb.Subscribe("streaming.message_chunk", func(ctx context.Context, event eventbus.Event) error {
		received = true
		mc, ok := event.(*MessageChunkStreamEvent)
		if !ok {
			t.Errorf("expected MessageChunkStreamEvent, got %T", event)
		}
		if mc.Content != "hi" || mc.Role != "assistant" {
			t.Errorf("unexpected: %+v", mc)
		}
		return nil
	})

	EmitMessageChunk(eb, context.Background(), "run-1", "hi", "assistant", "m-1")
	if !received {
		t.Error("event not received")
	}
}

func TestEmitUpdatesEvent(t *testing.T) {
	eb := eventbus.New()
	var received bool
	eb.Subscribe("streaming.updates", func(ctx context.Context, event eventbus.Event) error {
		received = true
		ue, ok := event.(*UpdatesStreamEvent)
		if !ok {
			t.Errorf("expected UpdatesStreamEvent, got %T", event)
		}
		if ue.NodeID != "n-1" {
			t.Errorf("node_id: %q", ue.NodeID)
		}
		return nil
	})

	EmitUpdatesEvent(eb, context.Background(), "run-1", "n-1", map[string]interface{}{"x": 1})
	if !received {
		t.Error("event not received")
	}
}

func TestEmitDebugEvent(t *testing.T) {
	eb := eventbus.New()
	var received bool
	eb.Subscribe("streaming.debug", func(ctx context.Context, event eventbus.Event) error {
		received = true
		de, ok := event.(*DebugStreamEvent)
		if !ok {
			t.Errorf("expected DebugStreamEvent, got %T", event)
		}
		if de.Level != "error" || de.Message != "failed" {
			t.Errorf("unexpected: %+v", de)
		}
		return nil
	})

	EmitDebugEvent(eb, context.Background(), "run-1", "error", "failed", nil)
	if !received {
		t.Error("event not received")
	}
}

func TestSerializeEvent(t *testing.T) {
	event := &MetadataStreamEvent{
		RunID:       "r1",
		ThreadID:    "t1",
		AssistantID: "a1",
		GraphID:     "g1",
	}

	data, err := SerializeEvent(event)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["RunID"] != "r1" {
		t.Errorf("unexpected: %v", m)
	}
}
