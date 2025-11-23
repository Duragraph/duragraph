package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxMessage represents a message in the outbox
type OutboxMessage struct {
	ID            int64
	EventID       string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       map[string]interface{}
	Metadata      map[string]interface{}
	CreatedAt     time.Time
	Published     bool
	PublishedAt   *time.Time
	Attempts      int
	LastError     string
	NextRetryAt   *time.Time
}

// Outbox implements the outbox pattern for reliable event publishing
type Outbox struct {
	pool *pgxpool.Pool
}

// NewOutbox creates a new outbox
func NewOutbox(pool *pgxpool.Pool) *Outbox {
	return &Outbox{pool: pool}
}

// GetUnpublished retrieves unpublished messages from the outbox
func (o *Outbox) GetUnpublished(ctx context.Context, limit int) ([]*OutboxMessage, error) {
	rows, err := o.pool.Query(ctx, `
		SELECT id, event_id, aggregate_type, aggregate_id, event_type, payload, metadata,
		       created_at, published, published_at, attempts, last_error, next_retry_at
		FROM outbox
		WHERE NOT published AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)

	if err != nil {
		return nil, errors.Internal("failed to query outbox", err)
	}
	defer rows.Close()

	messages := make([]*OutboxMessage, 0)

	for rows.Next() {
		msg := &OutboxMessage{}
		var payloadJSON, metadataJSON []byte

		err := rows.Scan(
			&msg.ID,
			&msg.EventID,
			&msg.AggregateType,
			&msg.AggregateID,
			&msg.EventType,
			&payloadJSON,
			&metadataJSON,
			&msg.CreatedAt,
			&msg.Published,
			&msg.PublishedAt,
			&msg.Attempts,
			&msg.LastError,
			&msg.NextRetryAt,
		)

		if err != nil {
			return nil, errors.Internal("failed to scan outbox message", err)
		}

		if err := json.Unmarshal(payloadJSON, &msg.Payload); err != nil {
			return nil, errors.Internal("failed to unmarshal payload", err)
		}

		if err := json.Unmarshal(metadataJSON, &msg.Metadata); err != nil {
			return nil, errors.Internal("failed to unmarshal metadata", err)
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// MarkAsPublished marks a message as published
func (o *Outbox) MarkAsPublished(ctx context.Context, id int64) error {
	now := time.Now()

	_, err := o.pool.Exec(ctx, `
		UPDATE outbox
		SET published = TRUE, published_at = $1
		WHERE id = $2
	`, now, id)

	if err != nil {
		return errors.Internal("failed to mark message as published", err)
	}

	return nil
}

// MarkAsFailed marks a message as failed and schedules retry
func (o *Outbox) MarkAsFailed(ctx context.Context, id int64, errorMsg string) error {
	// Exponential backoff: 1min, 2min, 4min, 8min, etc.
	var attempts int
	err := o.pool.QueryRow(ctx, `SELECT attempts FROM outbox WHERE id = $1`, id).Scan(&attempts)
	if err != nil {
		return errors.Internal("failed to get attempts", err)
	}

	backoffMinutes := 1 << attempts // 2^attempts
	if backoffMinutes > 60 {
		backoffMinutes = 60 // Max 1 hour
	}

	nextRetry := time.Now().Add(time.Duration(backoffMinutes) * time.Minute)

	_, err = o.pool.Exec(ctx, `
		UPDATE outbox
		SET attempts = attempts + 1, last_error = $1, next_retry_at = $2
		WHERE id = $3
	`, errorMsg, nextRetry, id)

	if err != nil {
		return errors.Internal("failed to mark message as failed", err)
	}

	return nil
}

// Cleanup removes old published messages
func (o *Outbox) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	result, err := o.pool.Exec(ctx, `
		DELETE FROM outbox
		WHERE published = TRUE
		  AND published_at < NOW() - INTERVAL '$1 days'
	`, retentionDays)

	if err != nil {
		return 0, errors.Internal("failed to cleanup outbox", err)
	}

	return result.RowsAffected(), nil
}
