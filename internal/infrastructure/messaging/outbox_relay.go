package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
)

// OutboxRelay polls the outbox and publishes messages to NATS
type OutboxRelay struct {
	outbox    *postgres.Outbox
	publisher *nats.Publisher
	interval  time.Duration
	batchSize int
	stopCh    chan struct{}
}

// NewOutboxRelay creates a new outbox relay
func NewOutboxRelay(outbox *postgres.Outbox, publisher *nats.Publisher, interval time.Duration, batchSize int) *OutboxRelay {
	return &OutboxRelay{
		outbox:    outbox,
		publisher: publisher,
		interval:  interval,
		batchSize: batchSize,
		stopCh:    make(chan struct{}),
	}
}

// Start starts the outbox relay worker
func (r *OutboxRelay) Start(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.stopCh:
			return nil
		case <-ticker.C:
			if err := r.processOutbox(ctx); err != nil {
				// Log error but continue
				fmt.Printf("outbox relay error: %v\n", err)
			}
		}
	}
}

// Stop stops the outbox relay
func (r *OutboxRelay) Stop() {
	close(r.stopCh)
}

// processOutbox processes messages from the outbox
func (r *OutboxRelay) processOutbox(ctx context.Context) error {
	// Get unpublished messages
	messages, err := r.outbox.GetUnpublished(ctx, r.batchSize)
	if err != nil {
		return fmt.Errorf("failed to get unpublished messages: %w", err)
	}

	if len(messages) == 0 {
		return nil
	}

	// Process each message
	for _, msg := range messages {
		if err := r.publishMessage(ctx, msg); err != nil {
			// Mark as failed and continue
			r.outbox.MarkAsFailed(ctx, msg.ID, err.Error())
			continue
		}

		// Mark as published
		if err := r.outbox.MarkAsPublished(ctx, msg.ID); err != nil {
			return fmt.Errorf("failed to mark message as published: %w", err)
		}
	}

	return nil
}

// publishMessage publishes a single message to NATS
func (r *OutboxRelay) publishMessage(ctx context.Context, msg *postgres.OutboxMessage) error {
	// Build topic from event type
	topic := buildTopic(msg.AggregateType, msg.EventType)

	// Create event envelope
	envelope := map[string]interface{}{
		"event_id":       msg.EventID,
		"aggregate_type": msg.AggregateType,
		"aggregate_id":   msg.AggregateID,
		"event_type":     msg.EventType,
		"payload":        msg.Payload,
		"metadata":       msg.Metadata,
		"timestamp":      msg.CreatedAt,
	}

	// Publish to NATS
	if err := r.publisher.Publish(ctx, topic, envelope); err != nil {
		return fmt.Errorf("failed to publish to NATS: %w", err)
	}

	return nil
}

// buildTopic builds a NATS topic from aggregate and event types
func buildTopic(aggregateType, eventType string) string {
	// Format: duragraph.{category}.{aggregate}.{event}
	// Example: duragraph.events.run.created

	category := "events"
	if aggregateType == "execution" {
		category = "executions"
	} else if aggregateType == "run" {
		category = "runs"
	}

	return fmt.Sprintf("duragraph.%s.%s.%s", category, aggregateType, eventType)
}

// CleanupWorker periodically cleans up old published messages
type CleanupWorker struct {
	outbox        *postgres.Outbox
	interval      time.Duration
	retentionDays int
	stopCh        chan struct{}
}

// NewCleanupWorker creates a new cleanup worker
func NewCleanupWorker(outbox *postgres.Outbox, interval time.Duration, retentionDays int) *CleanupWorker {
	return &CleanupWorker{
		outbox:        outbox,
		interval:      interval,
		retentionDays: retentionDays,
		stopCh:        make(chan struct{}),
	}
}

// Start starts the cleanup worker
func (w *CleanupWorker) Start(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.stopCh:
			return nil
		case <-ticker.C:
			deleted, err := w.outbox.Cleanup(ctx, w.retentionDays)
			if err != nil {
				fmt.Printf("cleanup error: %v\n", err)
			} else if deleted > 0 {
				fmt.Printf("cleaned up %d old outbox messages\n", deleted)
			}
		}
	}
}

// Stop stops the cleanup worker
func (w *CleanupWorker) Stop() {
	close(w.stopCh)
}
