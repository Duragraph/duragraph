package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Relay constants from relay.d2:
//   - outboxNotifyChannel: the pg_notify channel the event store signals
//     on after committing an outbox row. Must match the channel the
//     endpoints.Server.writeTx uses (it does — see runtime.go).
//   - safetyNetInterval: how often the relay drains even without a
//     NOTIFY. Belt-and-braces for the rare cases where pg_notify can
//     drop or coalesce (process restart window, broker network blip).
//     30s caps staleness without burning CPU on near-empty polls.
//   - reconnect backoff: exponential 1s → 2s → 4s → ... → 30s cap.
const (
	outboxNotifyChannel     = "outbox_new"
	defaultSafetyNet        = 30 * time.Second
	initialReconnectBackoff = 1 * time.Second
	maxReconnectBackoff     = 30 * time.Second
	defaultBatchSize        = 100

	// Exported aliases for callers (server composition) that want
	// the same constants without hardcoding them.
	DefaultSafetyNet = defaultSafetyNet
	DefaultBatchSize = defaultBatchSize
)

// Relay implements the six-step outbox relay from relay.d2:
//
//  1. LISTEN outbox_new — dedicated pgx.Conn (direct PG, NOT pool).
//  2. 30s safety-net timeout — catches coalesced/missed NOTIFYs.
//  3. Drain unpublished rows — main pgxpool, SELECT ... NOT published.
//  4. Publish to JetStream — Nats-Msg-Id = event_id (dedup 2m window).
//  5. Startup backlog drain — on first connect, drain unconditionally.
//  6. Reconnect on conn drop — exponential backoff 1s..30s cap.
//
// The relay holds ONE dedicated pgx.Conn for LISTEN (session affinity
// that a pooler would drop between TXs) and uses the main pgxpool for
// the drain itself. A Stop signal or ctx cancel cleanly ends both.
//
// Stop cancels the run context so any in-flight WaitForNotification
// unblocks immediately — without this the relay would wait up to
// safetyNet (default 30s) before noticing the stop signal. The listener
// conn itself is only ever touched by Start's own goroutine (pgx.Conn is
// not safe for concurrent Close + Wait), so shutdown is driven entirely
// by context cancellation rather than a cross-goroutine conn.Close.
type Relay struct {
	drain       *OutboxDrain
	publisher   *Publisher
	listenerDSN string
	safetyNet   time.Duration
	batchSize   int
	stopCh      chan struct{}

	cancelMu sync.Mutex
	cancel   context.CancelFunc // set by Start, called by Stop to unblock the wait
}

// NewRelay constructs the relay.
//
//   - drain: OutboxDrain over the tenant pgxpool (for FetchUnpublished /
//     MarkPublished / MarkFailed).
//   - publisher: Publisher over a JetStream context for the same tenant
//     account.
//   - listenerDSN: a Postgres DSN that bypasses any connection pooler.
//     LISTEN requires session affinity that PgBouncer transaction-pooling
//     drops between TXs. If empty, Start returns an error.
//   - safetyNet: forced drain interval absent a NOTIFY. Zero ⇒ default.
//   - batchSize: max rows drained per wake-up. Zero ⇒ default (100).
func NewRelay(drain *OutboxDrain, publisher *Publisher, listenerDSN string, safetyNet time.Duration, batchSize int) *Relay {
	if safetyNet == 0 {
		safetyNet = defaultSafetyNet
	}
	if batchSize == 0 {
		batchSize = defaultBatchSize
	}
	return &Relay{
		drain:       drain,
		publisher:   publisher,
		listenerDSN: listenerDSN,
		safetyNet:   safetyNet,
		batchSize:   batchSize,
		stopCh:      make(chan struct{}),
	}
}

// errStopRequested is the sentinel listenLoop returns when Stop was
// called. Distinguished from a real error so Start can exit cleanly
// without logging it as a failure.
var errStopRequested = errors.New("relay: stop requested")

// Start drives the LISTEN loop until ctx is canceled or Stop is called.
// Returns ctx.Err() on shutdown, nil on Stop.
//
// The outer loop is the reconnect loop: if the listener connection
// drops (broker restart, network blip), we wait initialReconnectBackoff
// (doubling up to maxReconnectBackoff) and reconnect. The inner
// listenLoop handles per-NOTIFY drains until the connection fails.
func (r *Relay) Start(ctx context.Context) error {
	if r.listenerDSN == "" {
		return errors.New("relay: listener DSN is required")
	}
	if r.drain == nil || r.publisher == nil {
		return errors.New("relay: drain and publisher are required")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	r.cancelMu.Lock()
	r.cancel = cancel
	r.cancelMu.Unlock()

	backoff := initialReconnectBackoff
	for {
		if err := r.shouldStop(runCtx); err != nil {
			return err
		}

		conn, err := pgx.Connect(runCtx, r.listenerDSN)
		if err != nil {
			slog.Error("relay: listener connect failed", "err", err, "retry_in", backoff)
			if waitErr := r.sleepOrStop(runCtx, backoff); waitErr != nil {
				return waitErr
			}
			backoff = nextBackoff(backoff)
			continue
		}

		if _, err := conn.Exec(runCtx, "LISTEN "+outboxNotifyChannel); err != nil {
			slog.Error("relay: LISTEN failed", "err", err, "retry_in", backoff)
			_ = conn.Close(context.Background())
			if waitErr := r.sleepOrStop(runCtx, backoff); waitErr != nil {
				return waitErr
			}
			backoff = nextBackoff(backoff)
			continue
		}

		// Connection is up. Reset backoff so a future fail-fast hits
		// the short delay first, not the long one.
		backoff = initialReconnectBackoff
		slog.Info("relay: LISTEN established", "channel", outboxNotifyChannel)

		// Step 5: startup backlog drain — rows may have been written
		// while we were down. Process before entering the wait loop.
		if err := r.processOutbox(runCtx); err != nil {
			slog.Error("relay: initial drain failed", "err", err)
		}

		loopErr := r.listenLoop(runCtx, conn)
		_ = conn.Close(context.Background())

		if errors.Is(loopErr, context.Canceled) || errors.Is(loopErr, context.DeadlineExceeded) {
			return loopErr
		}
		if loopErr == errStopRequested {
			return nil
		}
		if loopErr != nil {
			slog.Error("relay: listener loop ended, reconnecting", "err", loopErr)
		}
	}
}

// listenLoop blocks on conn.WaitForNotification with a timeout of
// safetyNet. A timeout fires a safety-net drain; an actual NOTIFY
// fires a normal drain. Any other error returns to Start for
// reconnect.
func (r *Relay) listenLoop(ctx context.Context, conn *pgx.Conn) error {
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
			// Real NOTIFY — drain.
		case errors.Is(err, context.DeadlineExceeded):
			// Safety-net interval elapsed — drain anyway.
		case errors.Is(err, context.Canceled):
			select {
			case <-r.stopCh:
				return errStopRequested // Stop() → clean shutdown (Start returns nil)
			default:
				return ctx.Err() // parent ctx canceled
			}
		default:
			return fmt.Errorf("wait for notification: %w", err)
		}

		if err := r.processOutbox(ctx); err != nil {
			slog.Error("relay: drain failed", "err", err)
		}
	}
}

// Stop signals Start to exit and cancels the run context so any in-flight
// WaitForNotification unblocks immediately. The listener conn is closed only
// by Start's own goroutine, so there is no cross-goroutine use of the conn.
// Idempotent.
func (r *Relay) Stop() {
	select {
	case <-r.stopCh:
		return // already stopped
	default:
		close(r.stopCh)
	}
	r.cancelMu.Lock()
	cancel := r.cancel
	r.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *Relay) shouldStop(ctx context.Context) error {
	select {
	case <-r.stopCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (r *Relay) sleepOrStop(ctx context.Context, d time.Duration) error {
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
// to NATS. On publish failure the row is marked failed (with backoff)
// rather than retrying in-process — the next NOTIFY or safety-net tick
// picks it up once next_retry_at passes.
func (r *Relay) processOutbox(ctx context.Context) error {
	rows, err := r.drain.FetchUnpublished(ctx, r.batchSize)
	if err != nil {
		return fmt.Errorf("fetch unpublished: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows {
		if err := r.publishRow(ctx, row); err != nil {
			_ = r.drain.MarkFailed(ctx, row.ID, err.Error())
			slog.Warn("relay: publish failed, marked for retry",
				"outbox_id", row.ID, "event_id", row.EventID, "err", err)
			continue
		}
		if err := r.drain.MarkPublished(ctx, row.ID); err != nil {
			return fmt.Errorf("mark published: %w", err)
		}
	}
	return nil
}

// publishRow publishes a single outbox row to its canonical JetStream
// subject. The payload is forwarded as-is (already JSON from the
// events table); Nats-Msg-Id is the outbox event_id so JetStream's
// dedup window collapses retries.
func (r *Relay) publishRow(ctx context.Context, row OutboxRow) error {
	subject := SubjectFor(row.EventType)
	payload := envelope(row)
	return r.publisher.PublishWithID(ctx, subject, row.EventID, payload)
}

// envelope is the JSON body published to JetStream. Mirrors the
// legacy relay's shape (event_id + aggregate_* + event_type + payload
// + metadata) so existing consumers (cli `events tail`, audit log)
// keep working.
func envelope(row OutboxRow) map[string]any {
	var payload, metadata any
	if len(row.Payload) > 0 {
		var p any
		_ = json.Unmarshal(row.Payload, &p)
		payload = p
	}
	if len(row.Metadata) > 0 {
		var m any
		_ = json.Unmarshal(row.Metadata, &m)
		metadata = m
	}
	return map[string]any{
		"event_id":       row.EventID,
		"aggregate_type": row.AggregateType,
		"aggregate_id":   row.AggregateID,
		"event_type":     row.EventType,
		"payload":        payload,
		"metadata":       metadata,
	}
}

// CleanupWorker periodically prunes old published outbox rows.
// CLAUDE.md default: hourly, 7-day retention. Independent of the
// relay's LISTEN loop — cleanup is rare and doesn't benefit from
// event-driven wake-up.
type CleanupWorker struct {
	drain         *OutboxDrain
	interval      time.Duration
	retentionDays int
	stopCh        chan struct{}
}

// NewCleanupWorker constructs the cleanup worker.
func NewCleanupWorker(drain *OutboxDrain, interval time.Duration, retentionDays int) *CleanupWorker {
	return &CleanupWorker{
		drain:         drain,
		interval:      interval,
		retentionDays: retentionDays,
		stopCh:        make(chan struct{}),
	}
}

// Start ticks on interval until ctx is canceled or Stop is called.
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
			deleted, err := w.drain.Cleanup(ctx, w.retentionDays)
			if err != nil {
				slog.Error("outbox cleanup error", "err", err)
			} else if deleted > 0 {
				slog.Info("cleaned up old outbox messages", "count", deleted)
			}
		}
	}
}

// Stop signals Start to exit. Idempotent.
func (w *CleanupWorker) Stop() {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}

// RelayFromPool is a convenience constructor that wires the common
// case: a tenant pgxpool for the drain + a JetStream publisher, with
// defaults for safetyNet and batchSize.
func RelayFromPool(pool *pgxpool.Pool, publisher *Publisher, listenerDSN string) *Relay {
	return NewRelay(NewOutboxDrain(pool), publisher, listenerDSN, defaultSafetyNet, defaultBatchSize)
}
