package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxRow is one row from the rebuild's outbox table (001_event_sourcing
// migration). The relay drains these and publishes each to NATS via the
// Publisher. event_id matches the events.event_id for JetStream dedup.
type OutboxRow struct {
	ID            int64
	EventID       string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       json.RawMessage
	Metadata      json.RawMessage
	Attempts      int
}

// OutboxDrain is the data-layer surface the relay uses. It mirrors the
// drain-path query set in controlplane/db/outbox/outbox.sql
// (FetchUnpublished / MarkPublished / MarkFailed / CleanupPublished).
// All four methods run on the main pgxpool — the relay's LISTEN
// connection is only for the wake-up signal.
type OutboxDrain struct {
	pool *pgxpool.Pool
}

// NewOutboxDrain wraps a pgxpool for outbox drainage. The pool must
// point at the same tenant DB whose events/outbox tables the
// endpoints.Server writes.
func NewOutboxDrain(pool *pgxpool.Pool) *OutboxDrain {
	return &OutboxDrain{pool: pool}
}

// FetchUnpublished drains a FIFO batch of rows eligible to publish:
// never-published AND (never retried OR past their backoff). Ordered by
// id for FIFO publish. LIMIT caps the batch size.
//
// Does NOT use FOR UPDATE SKIP LOCKED — the relay is single-instance
// per tenant DB by design (one LISTEN connection, one drain loop); if
// multi-instance relays ever become a thing, add SKIP LOCKED here.
func (o *OutboxDrain) FetchUnpublished(ctx context.Context, limit int) ([]OutboxRow, error) {
	if o.pool == nil {
		return nil, errors.New("outbox: nil pool")
	}
	rows, err := o.pool.Query(ctx, `
		SELECT id, event_id, aggregate_type, aggregate_id, event_type, payload, metadata, attempts
		FROM outbox
		WHERE NOT published
		  AND (next_retry_at IS NULL OR next_retry_at <= now())
		ORDER BY id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox: fetch unpublished: %w", err)
	}
	defer rows.Close()

	var out []OutboxRow
	for rows.Next() {
		var r OutboxRow
		if err := rows.Scan(
			&r.ID, &r.EventID, &r.AggregateType, &r.AggregateID,
			&r.EventType, &r.Payload, &r.Metadata, &r.Attempts,
		); err != nil {
			return nil, fmt.Errorf("outbox: scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkPublished marks a row delivered after a successful JetStream
// publish. Sets published + published_at, clears last_error.
func (o *OutboxDrain) MarkPublished(ctx context.Context, id int64) error {
	if o.pool == nil {
		return errors.New("outbox: nil pool")
	}
	_, err := o.pool.Exec(ctx, `
		UPDATE outbox
		SET published = TRUE, published_at = now(), last_error = NULL
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("outbox: mark published: %w", err)
	}
	return nil
}

// MarkFailed records a transient publish failure, increments attempts,
// and schedules the next retry with exponential backoff (2^attempts
// minutes, capped at 60). The caller passes in the failure error
// string; the backoff is computed here so the SQL stays declarative.
func (o *OutboxDrain) MarkFailed(ctx context.Context, id int64, errorMsg string) error {
	if o.pool == nil {
		return errors.New("outbox: nil pool")
	}
	var attempts int
	if err := o.pool.QueryRow(ctx, `SELECT attempts FROM outbox WHERE id = $1`, id).Scan(&attempts); err != nil {
		return fmt.Errorf("outbox: read attempts: %w", err)
	}
	backoffMin := 1 << attempts
	if backoffMin > 60 {
		backoffMin = 60
	}
	nextRetry := time.Now().Add(time.Duration(backoffMin) * time.Minute)
	_, err := o.pool.Exec(ctx, `
		UPDATE outbox
		SET attempts = attempts + 1, last_error = $2, next_retry_at = $3
		WHERE id = $1
	`, id, errorMsg, nextRetry)
	if err != nil {
		return fmt.Errorf("outbox: mark failed: %w", err)
	}
	return nil
}

// Cleanup prunes delivered rows older than the retention window. Called
// periodically by a cleanup goroutine (hourly, 7-day retention is the
// CLAUDE.md default).
func (o *OutboxDrain) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	if o.pool == nil {
		return 0, errors.New("outbox: nil pool")
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	ct, err := o.pool.Exec(ctx, `
		DELETE FROM outbox
		WHERE published = TRUE AND published_at < $1
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("outbox: cleanup: %w", err)
	}
	return ct.RowsAffected(), nil
}
