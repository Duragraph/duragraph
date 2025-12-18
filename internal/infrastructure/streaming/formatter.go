package streaming

import (
	"encoding/json"
	"fmt"
)

// EventFormatter formats events for Server-Sent Events streaming
type EventFormatter struct {
	modes []StreamMode
}

// NewEventFormatter creates a new event formatter with the specified modes
func NewEventFormatter(modes []StreamMode) *EventFormatter {
	return &EventFormatter{
		modes: modes,
	}
}

// ShouldSend checks if an event should be sent based on configured modes
func (f *EventFormatter) ShouldSend(eventType string) bool {
	for _, mode := range f.modes {
		switch mode {
		case ModeEvents:
			return true // events mode sends everything
		case ModeValues:
			if eventType == "values" || eventType == "state" || eventType == "end" {
				return true
			}
		case ModeMessages:
			if eventType == "message" || eventType == "message_chunk" || eventType == "end" {
				return true
			}
		case ModeUpdates:
			if eventType == "updates" || eventType == "delta" || eventType == "end" {
				return true
			}
		case ModeDebug:
			return true // debug mode sends everything
		}
	}
	return false
}

// FormatSSE formats an event for Server-Sent Events
func (f *EventFormatter) FormatSSE(eventType string, data interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, jsonData)), nil
}

// FormatStateEvent formats a state/values event
func (f *EventFormatter) FormatStateEvent(values map[string]interface{}) ([]byte, error) {
	return f.FormatSSE("values", StateUpdateEvent{
		Values: values,
	})
}

// FormatMessageChunk formats a message chunk event
func (f *EventFormatter) FormatMessageChunk(content string, role string, id string) ([]byte, error) {
	return f.FormatSSE("message_chunk", MessageChunkEvent{
		Content: content,
		Role:    role,
		ID:      id,
	})
}

// FormatNodeStart formats a node start event
func (f *EventFormatter) FormatNodeStart(nodeID, nodeType string, input map[string]interface{}) ([]byte, error) {
	return f.FormatSSE("node_start", NodeStartEvent{
		NodeID:   nodeID,
		NodeType: nodeType,
		Input:    input,
	})
}

// FormatNodeEnd formats a node end event
func (f *EventFormatter) FormatNodeEnd(nodeID, nodeType string, output map[string]interface{}, durationMs int64) ([]byte, error) {
	return f.FormatSSE("node_end", NodeEndEvent{
		NodeID:   nodeID,
		NodeType: nodeType,
		Output:   output,
		Duration: durationMs,
	})
}

// FormatDebug formats a debug event
func (f *EventFormatter) FormatDebug(level, message string, data map[string]interface{}) ([]byte, error) {
	return f.FormatSSE("debug", DebugEvent{
		Level:   level,
		Message: message,
		Data:    data,
	})
}

// FormatEnd formats an end event
func (f *EventFormatter) FormatEnd(runID string) ([]byte, error) {
	return f.FormatSSE("end", map[string]string{
		"run_id": runID,
	})
}

// FormatError formats an error event
func (f *EventFormatter) FormatError(message string, code string) ([]byte, error) {
	return f.FormatSSE("error", map[string]string{
		"message": message,
		"code":    code,
	})
}
