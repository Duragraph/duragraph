package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
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

// Stream handles GET /stream?run_id=xxx
func (h *StreamHandler) Stream(c echo.Context) error {
	runID := c.QueryParam("run_id")

	if runID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "run_id query parameter is required",
		})
	}

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

			// Send event to client
			eventType := event["event_type"].(string)
			data, _ := json.Marshal(event["payload"])

			fmt.Fprintf(c.Response(), "event: %s\n", eventType)
			fmt.Fprintf(c.Response(), "data: %s\n\n", string(data))
			c.Response().Flush()

			// Acknowledge message
			msg.Ack()

			// Check if run completed or failed
			if eventType == "run.completed" || eventType == "run.failed" || eventType == "run.cancelled" {
				return nil
			}
		}
	}
}
