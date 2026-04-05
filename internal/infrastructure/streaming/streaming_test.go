package streaming

import (
	"testing"
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
