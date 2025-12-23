package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
	"github.com/duragraph/duragraph/internal/infrastructure/streaming"
	"github.com/labstack/echo/v4"
)

// StreamHandler handles SSE streaming for run events
type StreamHandler struct {
	subscriber *nats.Subscriber
}

// NewStreamHandler creates a new StreamHandler
func NewStreamHandler(subscriber *nats.Subscriber) *StreamHandler {
	return &StreamHandler{
		subscriber: subscriber,
	}
}

// parseStreamModes extracts stream modes from query parameters
func parseStreamModes(c echo.Context) []streaming.StreamMode {
	modes := c.QueryParams()["stream_mode"]
	if len(modes) == 0 {
		// Check for comma-separated value in single param
		if modeParam := c.QueryParam("stream_mode"); modeParam != "" {
			modes = strings.Split(modeParam, ",")
		}
	}
	return streaming.ParseStreamModes(modes)
}

// StreamRun handles GET /threads/:thread_id/runs/:run_id/stream (LangGraph compatible)
func (h *StreamHandler) StreamRun(c echo.Context) error {
	runID := c.Param("run_id")
	if runID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "run_id is required in path",
		})
	}

	return h.streamByRunID(c, runID)
}

// Stream handles GET /stream?run_id=xxx (legacy endpoint)
func (h *StreamHandler) Stream(c echo.Context) error {
	runID := c.QueryParam("run_id")

	if runID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "run_id query parameter is required",
		})
	}

	return h.streamByRunID(c, runID)
}

// streamByRunID is the common streaming implementation
func (h *StreamHandler) streamByRunID(c echo.Context, runID string) error {
	// Parse stream modes
	modes := parseStreamModes(c)
	formatter := streaming.NewEventFormatter(modes)

	// Set SSE headers
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	// Subscribe to run events
	topic := fmt.Sprintf("duragraph.runs.run.>")
	messages, err := h.subscriber.Subscribe(topic)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			// Send keepalive
			fmt.Fprintf(c.Response(), ": keepalive\n\n")
			c.Response().Flush()

		case msg := <-messages:
			// Parse message
			var event map[string]interface{}
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				continue
			}

			// Filter by run ID
			if aggregateID, ok := event["aggregate_id"].(string); !ok || aggregateID != runID {
				continue
			}

			// Get event type
			eventType, _ := event["event_type"].(string)

			// Map internal event types to stream mode compatible types
			mappedType := mapEventType(eventType)

			// Check if event should be sent based on stream mode
			if !formatter.ShouldSend(mappedType) {
				msg.Ack()
				continue
			}

			// Format and send event
			payload := event["payload"]
			data, _ := formatter.FormatSSE(mappedType, payload)
			c.Response().Write(data)
			c.Response().Flush()

			// Acknowledge message
			msg.Ack()

			// Check if run completed or failed
			if eventType == "run.completed" || eventType == "run.failed" || eventType == "run.cancelled" {
				endData, _ := formatter.FormatEnd(runID)
				c.Response().Write(endData)
				c.Response().Flush()
				return nil
			}
		}
	}
}

// JoinThreadStream handles GET /threads/:thread_id/stream (LangGraph compatible)
// This streams output from all runs on a thread in real-time
func (h *StreamHandler) JoinThreadStream(c echo.Context) error {
	threadID := c.Param("thread_id")
	if threadID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "thread_id is required in path",
		})
	}

	return h.streamByThreadID(c, threadID)
}

// streamByThreadID streams all run events for a thread
func (h *StreamHandler) streamByThreadID(c echo.Context, threadID string) error {
	// Parse stream modes
	modes := parseStreamModes(c)
	formatter := streaming.NewEventFormatter(modes)

	// Set SSE headers
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	// Subscribe to all run events
	topic := fmt.Sprintf("duragraph.runs.run.>")
	messages, err := h.subscriber.Subscribe(topic)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			// Send keepalive
			fmt.Fprintf(c.Response(), ": keepalive\n\n")
			c.Response().Flush()

		case msg := <-messages:
			// Parse message
			var event map[string]interface{}
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				continue
			}

			// Filter by thread ID from payload
			payload, _ := event["payload"].(map[string]interface{})
			eventThreadID, _ := payload["thread_id"].(string)
			if eventThreadID != threadID {
				msg.Ack()
				continue
			}

			// Get event type
			eventType, _ := event["event_type"].(string)

			// Map internal event types to stream mode compatible types
			mappedType := mapEventType(eventType)

			// Check if event should be sent based on stream mode
			if !formatter.ShouldSend(mappedType) {
				msg.Ack()
				continue
			}

			// Format and send event
			data, _ := formatter.FormatSSE(mappedType, payload)
			c.Response().Write(data)
			c.Response().Flush()

			// Acknowledge message
			msg.Ack()

			// Note: Thread streams remain open indefinitely, client must close
		}
	}
}

// mapEventType maps internal event types to streaming mode compatible types
func mapEventType(eventType string) string {
	switch eventType {
	case "run.started", "run.in_progress":
		return "values"
	case "run.completed", "run.success":
		return "values"
	case "run.failed", "run.error":
		return "error"
	case "node.started":
		return "updates"
	case "node.completed":
		return "values"
	case "message.chunk":
		return "message_chunk"
	case "message.completed":
		return "message"
	default:
		return eventType
	}
}
