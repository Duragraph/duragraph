package handlers

import (
	"context"
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

// streamByRunID subscribes to run-specific NATS topics and streams events to the client.
func (h *StreamHandler) streamByRunID(c echo.Context, runID string) error {
	modes := parseStreamModes(c)
	formatter := streaming.NewEventFormatter(modes)

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	ctx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()

	topic := fmt.Sprintf("duragraph.stream.%s.>", runID)
	messages, err := h.subscriber.SubscribeWithContext(ctx, topic)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			if _, err := fmt.Fprintf(c.Response(), ": keepalive\n\n"); err != nil {
				return nil
			}
			c.Response().Flush()

		case msg, ok := <-messages:
			if !ok {
				return nil
			}

			var event map[string]interface{}
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				msg.Ack()
				continue
			}

			eventType, _ := event["event_type"].(string)
			mappedType := mapEventType(eventType)

			if !formatter.ShouldSend(mappedType) {
				msg.Ack()
				continue
			}

			payload := event["payload"]
			data, _ := formatter.FormatSSE(mappedType, payload)
			if _, err := c.Response().Write(data); err != nil {
				msg.Ack()
				return nil
			}
			c.Response().Flush()

			msg.Ack()

			if isTerminalEvent(eventType) {
				endData, _ := formatter.FormatEnd(runID)
				c.Response().Write(endData)
				c.Response().Flush()
				return nil
			}
		}
	}
}

// JoinThreadStream handles GET /threads/:thread_id/stream (LangGraph compatible)
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

// streamByThreadID streams all run events for a thread.
// Thread streams subscribe to the global topic and filter by thread_id in the
// payload, since thread_id is not part of the NATS subject hierarchy.
func (h *StreamHandler) streamByThreadID(c echo.Context, threadID string) error {
	modes := parseStreamModes(c)
	formatter := streaming.NewEventFormatter(modes)

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	ctx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()

	topic := "duragraph.runs.run.>"
	messages, err := h.subscriber.SubscribeWithContext(ctx, topic)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			if _, err := fmt.Fprintf(c.Response(), ": keepalive\n\n"); err != nil {
				return nil
			}
			c.Response().Flush()

		case msg, ok := <-messages:
			if !ok {
				return nil
			}

			var event map[string]interface{}
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				msg.Ack()
				continue
			}

			payload, _ := event["payload"].(map[string]interface{})
			eventThreadID, _ := payload["thread_id"].(string)
			if eventThreadID != threadID {
				msg.Ack()
				continue
			}

			eventType, _ := event["event_type"].(string)
			mappedType := mapEventType(eventType)

			if !formatter.ShouldSend(mappedType) {
				msg.Ack()
				continue
			}

			data, _ := formatter.FormatSSE(mappedType, payload)
			if _, err := c.Response().Write(data); err != nil {
				msg.Ack()
				return nil
			}
			c.Response().Flush()

			msg.Ack()
		}
	}
}

// isTerminalEvent returns true if the event type indicates the run has ended.
func isTerminalEvent(eventType string) bool {
	switch eventType {
	case "run.completed", "run.failed", "run.cancelled":
		return true
	default:
		return false
	}
}

// mapEventType maps internal event types to streaming mode compatible types
func mapEventType(eventType string) string {
	switch eventType {
	case "metadata":
		return "metadata"
	case "run.started", "run.in_progress":
		return "values"
	case "run.completed", "run.success":
		return "values"
	case "run.failed", "run.error":
		return "error"
	case "node.started", "node_start":
		return "updates"
	case "node.completed", "node_end":
		return "values"
	case "message.chunk", "message_chunk":
		return "message_chunk"
	case "message.completed", "message":
		return "message"
	case "values":
		return "values"
	case "updates":
		return "updates"
	case "debug":
		return "debug"
	default:
		return eventType
	}
}
