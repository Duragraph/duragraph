package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
)

const (
	// outboxNotifyChannel is the pg_notify channel the event store
	// signals on after committing an outbox row. Must match the
	// channel name used by event_store.go's saveEventsWithTx.
	outboxNotifyChannel = "outbox_new"

	// defaultSafetyNetInterval is how often the relay drains the
	// outbox even without a NOTIFY signal. Belt-and-braces for the
	// rare cases where pg_notify can drop or coalesce — Postgres
	// guarantees at-least-one notification per LISTENing session per
	// commit-burst, but a process restart or connection blip can
	// strand rows briefly. 30s caps that staleness without burning
	// CPU on near-empty polls.
	defaultSafetyNetInterval = 30 * time.Second

	// initialReconnectBackoff is the wait after a failed listener
	// connect / LISTEN before retrying. Doubles up to maxReconnectBackoff.
	initialReconnectBackoff = 1 * time.Second
	maxReconnectBackoff     = 30 * time.Second
)

// OutboxRelay reads pending outbox rows and publishes them to NATS.
//
// Wake-up model (post-Phase-3): the relay holds ONE dedicated
// `pgx.Conn` (NOT pooled — LISTEN requires session affinity that
// PgBouncer transaction-pooling drops between TXs). The connection
// stays in `LISTEN outbox_new` and wakes up on every `pg_notify` the
// event store commits. A `defaultSafetyNetInterval` timeout around
// each WaitForNotification forces a drain even if notifications are
// missed (process restart window, broker network blip), so the worst
// case is bounded staleness rather than indefinite hang.
//
// The drain itself uses the main pool (via the postgres.Outbox
// wrapper) — the listener connection is just for the wake-up signal.
type OutboxRelay struct {
	outbox      *postgres.Outbox
	publisher   *nats.Publisher
	listenerDSN string
	safetyNet   time.Duration
	batchSize   int
	stopCh      chan struct{}
}

// NewOutboxRelay constructs the relay.
//
//   - listenerDSN: a Postgres DSN that bypasses any connection pooler.
//     If empty, the relay returns an error on Start.
//   - safetyNet: how long to wait between forced drains absent a
//     NOTIFY signal. Zero means "use defaultSafetyNetInterval".
//   - batchSize: maximum rows drained per wake-up.
func NewOutboxRelay(outbox *postgres.Outbox, publisher *nats.Publisher, listenerDSN string, safetyNet time.Duration, batchSize int) *OutboxRelay {
	if safetyNet == 0 {
		safetyNet = defaultSafetyNetInterval
	}
	return &OutboxRelay{
		outbox:      outbox,
		publisher:   publisher,
		listenerDSN: listenerDSN,
		safetyNet:   safetyNet,
		batchSize:   batchSize,
		stopCh:      make(chan struct{}),
	}
}

// Start drives the LISTEN loop until ctx is canceled or Stop is
// called. Returns ctx.Err() on shutdown.
//
// The outer loop is the reconnect loop: if the listener connection
// drops (broker restart, network blip), we wait initialReconnectBackoff
// (doubling up to maxReconnectBackoff) and reconnect. The inner
// listenLoop handles per-NOTIFY drains until the connection fails.
func (r *OutboxRelay) Start(ctx context.Context) error {
	if r.listenerDSN == "" {
		return errors.New("outbox relay: listener DSN is required")
	}

	backoff := initialReconnectBackoff
	for {
		// Cooperative shutdown: stopCh and ctx both end the loop.
		if err := r.shouldStop(ctx); err != nil {
			return err
		}

		conn, err := pgx.Connect(ctx, r.listenerDSN)
		if err != nil {
			slog.Error("outbox relay: listener connect failed", "err", err, "retry_in", backoff)
			if waitErr := r.sleepOrStop(ctx, backoff); waitErr != nil {
				return waitErr
			}
			backoff = nextBackoff(backoff)
			continue
		}

		if _, err := conn.Exec(ctx, "LISTEN "+outboxNotifyChannel); err != nil {
			slog.Error("outbox relay: LISTEN failed", "err", err, "retry_in", backoff)
			_ = conn.Close(context.Background())
			if waitErr := r.sleepOrStop(ctx, backoff); waitErr != nil {
				return waitErr
			}
			backoff = nextBackoff(backoff)
			continue
		}

		// Connection is up. Reset backoff so a future fail-fast hits
		// the short delay first, not the long one.
		backoff = initialReconnectBackoff
		slog.Info("outbox relay: LISTEN established", "channel", outboxNotifyChannel)

		// Process anything pending right after first connecting —
		// rows may have been written while we were down.
		if err := r.processOutbox(ctx); err != nil {
			slog.Error("outbox relay: initial drain failed", "err", err)
		}

		// Run until the conn dies or ctx is canceled.
		loopErr := r.listenLoop(ctx, conn)
		_ = conn.Close(context.Background())

		if errors.Is(loopErr, context.Canceled) || errors.Is(loopErr, context.DeadlineExceeded) {
			return loopErr
		}
		// Stop signal or context — propagate. Otherwise transient,
		// loop back and reconnect.
		if loopErr == errStopRequested {
			return nil
		}
		if loopErr != nil {
			slog.Error("outbox relay: listener loop ended, reconnecting", "err", loopErr)
		}
	}
}

// errStopRequested is the sentinel listenLoop returns when Stop() was
// called. Distinguished from a real error so Start can return cleanly
// without logging it as a failure.
var errStopRequested = errors.New("outbox relay: stop requested")

// listenLoop blocks on conn.WaitForNotification with a timeout of
// safetyNet. A timeout fires a safety-net drain; an actual NOTIFY
// fires a normal drain. Any other error returns to Start for
// reconnect.
func (r *OutboxRelay) listenLoop(ctx context.Context, conn *pgx.Conn) error {
	for {
		select {
		case <-r.stopCh:
			return errStopRequested
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		waitCtx, cancel := context.WithTimeout(ctx, r.safetyNet)
		_, err := conn.WaitForNotification(waitCtx)
		cancel()

		switch {
		case err == nil:
			// Real NOTIFY received — drain.
		case errors.Is(err, context.DeadlineExceeded):
			// Safety-net interval elapsed — drain anyway.
		case errors.Is(err, context.Canceled):
			// Parent ctx canceled — propagate cleanly.
			return ctx.Err()
		default:
			// Conn-level error (broken socket, server gone, etc.).
			// Return to outer loop for reconnect.
			return fmt.Errorf("wait for notification: %w", err)
		}

		if err := r.processOutbox(ctx); err != nil {
			// Drain failures are logged but don't kill the loop —
			// next NOTIFY or safety-net tick will retry.
			slog.Error("outbox relay: drain failed", "err", err)
		}
	}
}

// Stop signals Start to exit. Idempotent.
func (r *OutboxRelay) Stop() {
	select {
	case <-r.stopCh:
		// already stopped
	default:
		close(r.stopCh)
	}
}

func (r *OutboxRelay) shouldStop(ctx context.Context) error {
	select {
	case <-r.stopCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (r *OutboxRelay) sleepOrStop(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-r.stopCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > maxReconnectBackoff {
		return maxReconnectBackoff
	}
	return d
}

// processOutbox drains pending rows up to batchSize and publishes each
// to NATS. Uses the main pool via the postgres.Outbox wrapper. Same
// semantics as the previous polling implementation.
func (r *OutboxRelay) processOutbox(ctx context.Context) error {
	messages, err := r.outbox.GetUnpublished(ctx, r.batchSize)
	if err != nil {
		return fmt.Errorf("failed to get unpublished messages: %w", err)
	}
	if len(messages) == 0 {
		return nil
	}

	for _, msg := range messages {
		if err := r.publishMessage(ctx, msg); err != nil {
			r.outbox.MarkAsFailed(ctx, msg.ID, err.Error())
			continue
		}
		if err := r.outbox.MarkAsPublished(ctx, msg.ID); err != nil {
			return fmt.Errorf("failed to mark message as published: %w", err)
		}
	}
	return nil
}

// publishMessage publishes a single message to NATS with Nats-Msg-Id
// set to the outbox event ID so JetStream dedups retries.
func (r *OutboxRelay) publishMessage(ctx context.Context, msg *postgres.OutboxMessage) error {
	topic := buildTopic(msg.AggregateType, msg.EventType)

	envelope := map[string]interface{}{
		"event_id":       msg.EventID,
		"aggregate_type": msg.AggregateType,
		"aggregate_id":   msg.AggregateID,
		"event_type":     msg.EventType,
		"payload":        msg.Payload,
		"metadata":       msg.Metadata,
		"timestamp":      msg.CreatedAt,
	}

	if err := r.publisher.PublishWithID(ctx, topic, msg.EventID, envelope); err != nil {
		return fmt.Errorf("failed to publish to NATS: %w", err)
	}
	return nil
}

// buildTopic builds a NATS topic from aggregate and event types
// (e.g. `duragraph.events.run.created`). Preserved from the previous
// implementation — the CLI's `events tail` subject mapping depends on
// this exact format. See internal/dev/cli/nats.go.
func buildTopic(aggregateType, eventType string) string {
	category := "events"
	if aggregateType == "execution" {
		category = "executions"
	} else if aggregateType == "run" {
		category = "runs"
	}
	return fmt.Sprintf("duragraph.%s.%s.%s", category, aggregateType, eventType)
}

// CleanupWorker periodically cleans up old published messages.
// Unchanged from the polling-era relay — cleanup is rare (every hour)
// and doesn't benefit from event-driven wake-up.
type CleanupWorker struct {
	outbox        *postgres.Outbox
	interval      time.Duration
	retentionDays int
	stopCh        chan struct{}
}

// NewCleanupWorker creates a new cleanup worker.
func NewCleanupWorker(outbox *postgres.Outbox, interval time.Duration, retentionDays int) *CleanupWorker {
	return &CleanupWorker{
		outbox:        outbox,
		interval:      interval,
		retentionDays: retentionDays,
		stopCh:        make(chan struct{}),
	}
}

// Start starts the cleanup worker.
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
				slog.Error("outbox cleanup error", "err", err)
			} else if deleted > 0 {
				slog.Info("cleaned up old outbox messages", "count", deleted)
			}
		}
	}
}

// Stop stops the cleanup worker.
func (w *CleanupWorker) Stop() {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}
