// Package reaper fails runs the execution path abandoned. A run that a worker
// leased and then died on (past the redelivery window), or that was queued and
// never dispatched (command lost), would otherwise sit non-terminal forever.
// Detection is time-since-start, NOT worker-lease, so the reaper never fails a
// run JetStream is still redelivering/resuming. Source: the run-reaper design doc.
package reaper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/duragraph/duragraph/controlplane/eventstore"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Interval             time.Duration // tick period; default 60s
	InProgressStaleAfter time.Duration // default 30m (> max_deliver × ack_wait)
	QueuedStaleAfter     time.Duration // default 10m
}

func (c *Config) defaults() {
	if c.Interval == 0 {
		c.Interval = 60 * time.Second
	}
	if c.InProgressStaleAfter == 0 {
		c.InProgressStaleAfter = 30 * time.Minute
	}
	if c.QueuedStaleAfter == 0 {
		c.QueuedStaleAfter = 10 * time.Minute
	}
}

type RunReaper struct {
	pool   *pgxpool.Pool
	cfg    Config
	stopCh chan struct{}
}

func NewRunReaper(pool *pgxpool.Pool, cfg Config) *RunReaper {
	cfg.defaults()
	return &RunReaper{pool: pool, cfg: cfg, stopCh: make(chan struct{})}
}

// Start ticks on Interval until ctx is canceled or Stop is called.
func (r *RunReaper) Start(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.stopCh:
			return nil
		case <-ticker.C:
			if n, err := r.reapOnce(ctx); err != nil {
				slog.Error("reaper: tick failed", "err", err)
			} else if n > 0 {
				slog.Info("reaper: failed stuck runs", "count", n)
			}
		}
	}
}

// Stop signals Start to exit. Idempotent.
func (r *RunReaper) Stop() {
	select {
	case <-r.stopCh:
	default:
		close(r.stopCh)
	}
}

// reapOnce fails, in one transaction, every run stuck past its threshold, emitting
// run.failed for each. Returns the number reaped.
func (r *RunReaper) reapOnce(ctx context.Context) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	rows, err := tx.Query(ctx, `
		SELECT id, status FROM runs
		WHERE (status = 'in_progress' AND started_at < now() - $1::interval)
		   OR (status = 'queued'      AND created_at < now() - $2::interval)
		FOR UPDATE SKIP LOCKED`,
		intervalStr(r.cfg.InProgressStaleAfter), intervalStr(r.cfg.QueuedStaleAfter))
	if err != nil {
		return 0, err
	}
	type stuck struct {
		id     uuid.UUID
		status string
	}
	var list []stuck
	for rows.Next() {
		var s stuck
		if err := rows.Scan(&s.id, &s.status); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, s)
	}
	rows.Close()
	if rows.Err() != nil {
		return 0, rows.Err()
	}

	for _, s := range list {
		reason := fmt.Sprintf("reaped: no active worker (stuck %s past threshold)", s.status)
		if _, err := tx.Exec(ctx,
			`UPDATE runs SET status='failed', completed_at=now(), error=$2 WHERE id=$1`,
			s.id, reason); err != nil {
			return 0, err
		}
		if err := eventstore.Append(ctx, tx, eventstore.Event{
			AggregateType: "Run", AggregateID: s.id, EventType: "run.failed",
			Payload: []byte(fmt.Sprintf(`{"reason":%q}`, reason)),
		}); err != nil {
			return 0, err
		}
	}
	if len(list) > 0 {
		if _, err := tx.Exec(ctx, `SELECT pg_notify('outbox_new', '')`); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(list), nil
}

// intervalStr renders a duration as a Postgres interval literal (seconds).
func intervalStr(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int64(d.Seconds()))
}
