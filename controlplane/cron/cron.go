// Package cron fires due cron jobs. A crons row describes a schedule but
// nothing ever acted on it: next_run_at was only ever SELECTed, never set and
// never polled, so a created cron sat inert forever.
//
// Shape mirrors controlplane/reaper: a ticker in the control plane claiming
// rows with FOR UPDATE SKIP LOCKED. Not pg_cron (a superuser extension that
// would need enabling in every tenant database, and that would have to
// hand-write the runs+events+outbox triple in SQL) and not JetStream (which has
// no scheduled delivery). Firing here means a cron-created run takes the SAME
// event-sourced path as an API-created one — run.created to the outbox, relayed
// to NATS, dispatched to a worker — rather than a second route to the same
// outcome.
package cron

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	cronlib "github.com/robfig/cron/v3"

	"github.com/duragraph/duragraph/controlplane/eventstore"
)

// Parser accepts standard 5-field cron expressions (minute hour dom month dow).
var Parser = cronlib.NewParser(
	cronlib.Minute | cronlib.Hour | cronlib.Dom | cronlib.Month | cronlib.Dow,
)

// NextRun returns the first firing of schedule strictly after from.
//
// Computing from NOW rather than from the previous due time is what makes a
// missed window simply skip: if the control plane was down for three hours, a
// five-minute cron fires once on recovery and is next due five minutes later,
// instead of replaying thirty-six runs nobody wants.
func NextRun(schedule string, from time.Time) (time.Time, error) {
	s, err := Parser.Parse(schedule)
	if err != nil {
		return time.Time{}, err
	}
	return s.Next(from).UTC(), nil
}

// Validate reports whether schedule is a usable cron expression.
func Validate(schedule string) error {
	_, err := Parser.Parse(schedule)
	return err
}

type Config struct {
	Interval time.Duration // tick period; default 30s
	Batch    int           // max crons fired per tick; default 100
}

func (c *Config) defaults() {
	if c.Interval == 0 {
		c.Interval = 30 * time.Second
	}
	if c.Batch == 0 {
		c.Batch = 100
	}
}

type Scheduler struct {
	pool   *pgxpool.Pool
	cfg    Config
	stopCh chan struct{}
}

func NewScheduler(pool *pgxpool.Pool, cfg Config) *Scheduler {
	cfg.defaults()
	return &Scheduler{pool: pool, cfg: cfg, stopCh: make(chan struct{})}
}

// Start ticks on Interval until ctx is canceled or Stop is called.
func (s *Scheduler) Start(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.stopCh:
			return nil
		case <-ticker.C:
			if n, err := s.FireDue(ctx); err != nil {
				slog.Error("cron: tick failed", "err", err)
			} else if n > 0 {
				slog.Info("cron: fired due jobs", "count", n)
			}
		}
	}
}

// Stop signals Start to exit. Idempotent.
func (s *Scheduler) Stop() {
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
}

type dueCron struct {
	id          uuid.UUID
	threadID    *uuid.UUID
	assistantID uuid.UUID
	schedule    string
	input       []byte
}

// FireDue creates a run for every cron currently due and advances its
// next_run_at. Returns how many fired.
//
// One transaction per cron, not one for the batch: a single unparseable
// schedule must not roll back the runs already created for the others.
func (s *Scheduler) FireDue(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	fired := 0
	for i := 0; i < s.cfg.Batch; i++ {
		ok, err := s.fireOne(ctx, now)
		if err != nil {
			return fired, err
		}
		if !ok {
			break
		}
		fired++
	}
	return fired, nil
}

// fireOne claims a single due cron, creates its run, and advances the
// schedule — all in one transaction. Reports whether one was found.
//
// The claim is FOR UPDATE SKIP LOCKED so that running more than one control
// plane does not fire the same cron twice: the second replica's SELECT steps
// over the row the first is holding, and by commit next_run_at has moved into
// the future so it is no longer due.
func (s *Scheduler) fireOne(ctx context.Context, now time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	var c dueCron
	err = tx.QueryRow(ctx, `
		SELECT id, thread_id, assistant_id, schedule, input
		FROM crons
		WHERE next_run_at IS NOT NULL
		  AND next_run_at <= $1
		  AND (end_time IS NULL OR end_time > $1)
		ORDER BY next_run_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1`, now).Scan(&c.id, &c.threadID, &c.assistantID, &c.schedule, &c.input)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	next, err := NextRun(c.schedule, now)
	if err != nil {
		// An unparseable schedule can never fire. Clearing next_run_at retires
		// it instead of leaving it permanently due and re-selected on every
		// tick — the row stays for the operator to see and fix.
		slog.Error("cron: retiring job with an invalid schedule",
			"cron_id", c.id, "schedule", c.schedule, "err", err)
		if _, uerr := tx.Exec(ctx,
			`UPDATE crons SET next_run_at = NULL WHERE id = $1`, c.id); uerr != nil {
			return false, uerr
		}
		return true, tx.Commit(ctx)
	}

	runID := uuid.New()
	if err := eventstore.Append(ctx, tx, eventstore.Event{
		AggregateType: "Run",
		AggregateID:   runID,
		EventType:     "run.created",
		Payload: mustJSON(map[string]any{
			"run_id":       runID.String(),
			"assistant_id": c.assistantID.String(),
			"cron_id":      c.id.String(),
		}),
	}); err != nil {
		return false, err
	}
	// metadata records the origin so a run that appears unprompted can be
	// traced back to the cron that asked for it.
	if _, err := tx.Exec(ctx, `
		INSERT INTO runs (id, thread_id, assistant_id, status, input, metadata)
		VALUES ($1, $2, $3, 'queued', $4, $5)`,
		runID, c.threadID, c.assistantID, jsonOrEmpty(c.input),
		mustJSON(map[string]any{"cron_id": c.id.String()})); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE crons SET next_run_at = $2 WHERE id = $1`, c.id, next); err != nil {
		return false, err
	}
	// Same notify the API path fires, so the relay picks the run up immediately
	// rather than on its next poll.
	if _, err := tx.Exec(ctx, `SELECT pg_notify('outbox_new', '')`); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
