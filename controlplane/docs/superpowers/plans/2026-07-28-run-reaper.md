# Run Reaper Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a control-plane reaper that fails runs the execution path leaves stuck (`in_progress` past the redelivery window, or `queued` past a dispatch timeout), emitting `run.failed` so no run stays non-terminal forever.

**Architecture:** A background worker (mirroring `nats.CleanupWorker`) ticks on an interval; each tick, in one tenant-DB transaction, selects stuck runs (`FOR UPDATE SKIP LOCKED`), marks them `failed`, and emits `run.failed` through the shared transactional-outbox append. To keep one append implementation (no duplicated append SQL), `Event` + the append are first extracted into a small `controlplane/eventstore` package used by both `endpoints.writeTx` and the reaper.

**Tech Stack:** Go 1.25, pgx v5 (`pgxpool`), testcontainers Postgres.

## Global Constraints

- Design doc: `controlplane/docs/superpowers/specs/2026-07-28-run-reaper-design.md` — the contract; this plan implements it.
- **Detection is time-since-start, NOT worker-lease.** `in_progress` stale threshold (default 30 min) must exceed the redelivery window (`max_deliver × ack_wait ≈ 25 min`) so the reaper never fails a run JetStream is still recovering. The "fresh in_progress run is NOT reaped" test is the load-bearing guard.
- The reaper only touches `in_progress` and `queued`. It ignores `requires_action` (HITL, legitimately non-terminal), `completed`, `failed`, `cancelled`.
- `run.failed` is emitted via the shared outbox append (events + outbox + `pg_notify`) so the relay publishes it — not a silent DB flip.
- The `eventstore` extraction must be behavior-preserving: the existing `controlplane/endpoints` integration suite must still pass unchanged. `endpoints.Event` becomes a type alias so existing handler literals and generated `*_gen.go` compile without regeneration (never hand-edit `*_gen.go`).
- Integration tests: testcontainers Postgres, no build tag, standard CI. Never weaken assertions.
- Never hand-edit go.mod/go.sum. Conventional commits. No PR/push/merge without explicit approval.
- Run all commands from the worktree root `~/worktrees/duragraph/feat/controlplane-server` (branch `feat/controlplane-run-reaper`).

---

## File Structure

- `controlplane/eventstore/eventstore.go` (new) — `Event` + `Append` + private `jsonOrEmpty`.
- `controlplane/eventstore/eventstore_integration_test.go` (new) — direct Append test.
- `controlplane/endpoints/runtime.go` (modify) — drop local `Event`/`appendEvent`; alias `Event`; `writeTx` calls `eventstore.Append`. KEEP the local `jsonOrEmpty` (still used by `workers.go`).
- `controlplane/reaper/reaper.go` (new) — `Config`, `RunReaper`, `NewRunReaper`, `Start`/`Stop`, `reapOnce`.
- `controlplane/reaper/reaper_integration_test.go` (new) — reaper tests + a package `TestMain`.
- `controlplane/server/server.go` (modify) — construct + wire the reaper alongside relay/cleanup/run-processor.

---

## Task 1: Extract `eventstore` (Event + Append), behavior-preserving

**Files:**
- Create: `controlplane/eventstore/eventstore.go`
- Create: `controlplane/eventstore/eventstore_integration_test.go`
- Modify: `controlplane/endpoints/runtime.go`

**Interfaces:**
- Produces: `eventstore.Event{ AggregateType string; AggregateID uuid.UUID; EventType string; Payload []byte; Metadata []byte }`; `func eventstore.Append(ctx context.Context, tx pgx.Tx, e Event) error` (EnsureStream + INSERT events + INSERT outbox, identical to the current `appendEvent`).
- Consumes (endpoints): `endpoints.Event` becomes `type Event = eventstore.Event`; `writeTx` calls `eventstore.Append` per event.

- [ ] **Step 1: Write the failing eventstore test**

Create `controlplane/eventstore/eventstore_integration_test.go` (package `eventstore_test`) with a testcontainer `TestMain` (mirror `controlplane/endpoints/assistants_integration_test.go`'s `TestMain` + `applyTenantMigrations` — a Postgres container with the tenant migrations applied, exposing `testPool`). Then:

```go
func TestAppendWritesEventAndOutbox(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	aggID := uuid.New()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := eventstore.Append(ctx, tx, eventstore.Event{
		AggregateType: "Run", AggregateID: aggID, EventType: "run.failed",
		Payload: []byte(`{"reason":"test"}`),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var evCount, obCount int
	_ = testPool.QueryRow(ctx, `SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='run.failed'`, aggID).Scan(&evCount)
	_ = testPool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE aggregate_id=$1 AND event_type='run.failed'`, aggID).Scan(&obCount)
	if evCount != 1 || obCount != 1 {
		t.Errorf("want 1 event + 1 outbox row, got %d/%d", evCount, obCount)
	}
}
```

- [ ] **Step 2: Run it — fails (package doesn't exist)**

Run: `go test ./controlplane/eventstore/ -run TestAppendWritesEventAndOutbox -v`
Expected: FAIL — `package controlplane/eventstore` / `undefined: eventstore.Append`.

- [ ] **Step 3: Create the eventstore package**

Create `controlplane/eventstore/eventstore.go` — the `Event` struct + the `appendEvent` body (renamed `Append`) + a private `jsonOrEmpty`, all moved verbatim from `endpoints/runtime.go`:

```go
// Package eventstore holds the transactional-outbox event append shared by the
// HTTP write path (endpoints.writeTx) and the run reaper. One append
// implementation → no duplicated append SQL. Mirrors outbox.sql's EnsureStream +
// AppendEvent + EnqueueOutbox.
package eventstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Event is one domain event to append within a write transaction. EventID is
// app-generated so the mirrored outbox row carries the identical id for
// JetStream dedup.
type Event struct {
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       []byte
	Metadata      []byte
}

// Append ensures the aggregate's stream, appends the event (advancing version),
// and mirrors it to the outbox — all on the caller's tx.
func Append(ctx context.Context, tx pgx.Tx, e Event) error {
	var streamID uuid.UUID
	var version int
	err := tx.QueryRow(ctx, `
		INSERT INTO event_streams (stream_id, aggregate_type, aggregate_id, version)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (aggregate_type, aggregate_id) DO UPDATE SET updated_at = now()
		RETURNING stream_id, version`,
		uuid.New(), e.AggregateType, e.AggregateID).Scan(&streamID, &version)
	if err != nil {
		return err
	}
	eventID := uuid.New()
	nextVersion := version + 1
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (event_id, stream_id, aggregate_type, aggregate_id,
		                    event_type, event_version, payload, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		eventID, streamID, e.AggregateType, e.AggregateID,
		e.EventType, nextVersion, jsonOrEmpty(e.Payload), jsonOrEmpty(e.Metadata)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (event_id, aggregate_type, aggregate_id, event_type, payload, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id) DO NOTHING`,
		eventID, e.AggregateType, e.AggregateID, e.EventType,
		jsonOrEmpty(e.Payload), jsonOrEmpty(e.Metadata))
	return err
}

func jsonOrEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}
```

- [ ] **Step 4: Point endpoints at eventstore (behavior-preserving)**

In `controlplane/endpoints/runtime.go`:
- Delete the local `Event` struct definition and the `appendEvent` func.
- KEEP the local `jsonOrEmpty` (it is still used by `workers.go` lines ~158, ~230 — verify with `grep -n jsonOrEmpty controlplane/endpoints/*.go`).
- Add `type Event = eventstore.Event` (a type alias, so existing `Event{...}` literals in handlers and generated `*_gen.go` compile unchanged).
- In `writeTx`, replace `if err := appendEvent(ctx, tx, e); err != nil {` with `if err := eventstore.Append(ctx, tx, e); err != nil {`.
- Add the import `"github.com/duragraph/duragraph/controlplane/eventstore"`.

- [ ] **Step 5: Build + eventstore test + full endpoints regression**

Run: `go build ./controlplane/...`
Expected: clean (the `Event` alias keeps all handler + `*_gen.go` references compiling).

Run: `go test ./controlplane/eventstore/ -run TestAppendWritesEventAndOutbox -v`
Expected: PASS.

Run: `go test ./controlplane/endpoints/ -count=1`
Expected: PASS — the extraction is behavior-preserving; every existing group (assistants/threads/runs/crons/store/system/workers) still green.

- [ ] **Step 6: Commit**

```bash
git add controlplane/eventstore/ controlplane/endpoints/runtime.go
git commit -m "refactor(controlplane): extract transactional event append into eventstore package"
```

---

## Task 2: RunReaper + server wiring

**Files:**
- Create: `controlplane/reaper/reaper.go`
- Create: `controlplane/reaper/reaper_integration_test.go`
- Modify: `controlplane/server/server.go`

**Interfaces:**
- Consumes: `*pgxpool.Pool` (tenant); `eventstore.Append`, `eventstore.Event`.
- Produces:
  - `reaper.Config{ Interval, InProgressStaleAfter, QueuedStaleAfter time.Duration }` with a `defaults()` (60s / 30m / 10m).
  - `reaper.RunReaper` with `func NewRunReaper(pool *pgxpool.Pool, cfg Config) *RunReaper`, `func (r *RunReaper) Start(ctx context.Context) error`, `func (r *RunReaper) Stop()`, `func (r *RunReaper) reapOnce(ctx context.Context) (int, error)`.

- [ ] **Step 1: Write the failing reaper test**

Create `controlplane/reaper/reaper_integration_test.go` (package `reaper` — internal, so it can call `reapOnce`) with a testcontainer `TestMain` (mirror the endpoints harness: Postgres container + `applyTenantMigrations`, exposing `testPool`). Then:

```go
func seedRun(t *testing.T, ctx context.Context, status string, startedAgo, createdAgo string) string {
	t.Helper()
	var aid, rid string
	if err := testPool.QueryRow(ctx, `INSERT INTO assistants (name) VALUES ('a') RETURNING id`).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	// started_at is NULL for queued; set for in_progress. created_at controls queued staleness.
	err := testPool.QueryRow(ctx, `
		INSERT INTO runs (assistant_id, status, created_at, started_at)
		VALUES ($1, $2, now() - $3::interval,
		        CASE WHEN $2 = 'in_progress' THEN now() - $4::interval ELSE NULL END)
		RETURNING id`, aid, status, createdAgo, startedAgo).Scan(&rid)
	if err != nil {
		t.Fatal(err)
	}
	return rid
}

func TestReaperFailsStuckRuns(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, events, outbox, event_streams, assistants CASCADE"); err != nil {
		t.Fatal(err)
	}
	staleIP  := seedRun(t, ctx, "in_progress", "40 minutes", "45 minutes") // stale → fail
	staleQ   := seedRun(t, ctx, "queued",      "0", "20 minutes")          // stale → fail
	freshIP  := seedRun(t, ctx, "in_progress", "2 minutes", "3 minutes")   // fresh → untouched
	freshQ   := seedRun(t, ctx, "queued",      "0", "1 minute")            // fresh → untouched

	r := NewRunReaper(testPool, Config{InProgressStaleAfter: 30 * time.Minute, QueuedStaleAfter: 10 * time.Minute})
	n, err := r.reapOnce(ctx)
	if err != nil {
		t.Fatalf("reapOnce: %v", err)
	}
	if n != 2 {
		t.Errorf("reaped count: want 2, got %d", n)
	}
	assertStatus(t, ctx, staleIP, "failed")
	assertStatus(t, ctx, staleQ, "failed")
	assertStatus(t, ctx, freshIP, "in_progress") // NOT reaped — mid-recovery window
	assertStatus(t, ctx, freshQ, "queued")
	// run.failed emitted for the two stuck runs (so the relay publishes them)
	for _, id := range []string{staleIP, staleQ} {
		var c int
		_ = testPool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE aggregate_id=$1 AND event_type='run.failed'`, id).Scan(&c)
		if c != 1 {
			t.Errorf("run %s: want 1 run.failed outbox row, got %d", id, c)
		}
	}
}

func TestReaperIdempotent(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, events, outbox, event_streams, assistants CASCADE"); err != nil {
		t.Fatal(err)
	}
	_ = seedRun(t, ctx, "in_progress", "40 minutes", "45 minutes")
	r := NewRunReaper(testPool, Config{InProgressStaleAfter: 30 * time.Minute, QueuedStaleAfter: 10 * time.Minute})
	if _, err := r.reapOnce(ctx); err != nil {
		t.Fatal(err)
	}
	n, err := r.reapOnce(ctx) // second tick: nothing left non-terminal
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("second tick reaped: want 0, got %d", n)
	}
}

func assertStatus(t *testing.T, ctx context.Context, id, want string) {
	t.Helper()
	var got string
	if err := testPool.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("run %s status: want %s, got %s", id, want, got)
	}
}
```

- [ ] **Step 2: Run it — fails (package doesn't exist)**

Run: `go test ./controlplane/reaper/ -run TestReaper -v`
Expected: FAIL — `undefined: NewRunReaper`.

- [ ] **Step 3: Write the reaper**

Create `controlplane/reaper/reaper.go`:

```go
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
	"github.com/jackc/pgx/v5"
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
```

- [ ] **Step 4: Build + run the reaper tests**

Run: `go build ./controlplane/...`
Run: `go test ./controlplane/reaper/ -run TestReaper -v -count=1`
Expected: PASS — `TestReaperFailsStuckRuns` (2 reaped, fresh untouched, run.failed emitted) + `TestReaperIdempotent`.

- [ ] **Step 5: Wire the reaper into the server**

In `controlplane/server/server.go`, mirror the cleanup-worker wiring:
- Add fields `reaper *reaper.RunReaper` and `reaperDone chan error`; init `reaperDone: make(chan error, 1)` in `New`.
- In `New`, after the pools are open (the reaper needs only `s.tenant`, not NATS), construct `s.reaper = reaper.NewRunReaper(s.tenant, reaper.Config{})` (defaults). Do this unconditionally when `s.tenant != nil`.
- In `Run`, gated by `s.cfg.Relays` (consistent with relay/cleanup), launch: `if s.reaper != nil && s.cfg.Relays { go func() { s.reaperDone <- s.reaper.Start(ctx) }() }`.
- In `Shutdown`, `if s.reaper != nil { s.reaper.Stop() }` and drain `s.reaperDone` (bounded by the drain timeout, like `cleanupDone`).
- Add the import `"github.com/duragraph/duragraph/controlplane/reaper"`.

- [ ] **Step 6: Build + server regression**

Run: `go build ./... && go vet ./controlplane/...`
Run: `go test ./controlplane/server/ -count=1`
Expected: PASS — server still constructs, runs, and shuts down cleanly with the reaper wired.

- [ ] **Step 7: Full regression**

Run: `go test ./controlplane/... -count=1`
Expected: all PASS (eventstore, endpoints, nats, reaper, server, worker).

- [ ] **Step 8: Commit**

```bash
git add controlplane/reaper/ controlplane/server/server.go
git commit -m "feat(controlplane): run reaper — fail runs stuck past the redelivery window"
```

---

## Self-Review

**Spec coverage:**
- eventstore extraction (DRY append) → Task 1. ✓
- Reaper detection (time-since-start; in_progress 30m / queued 10m; disjoint arms via nullable started_at) → Task 2 Step 3. ✓
- Emit run.failed via shared append → Task 2 (eventstore.Append in reapOnce). ✓
- Does-not-preempt-recovery guard → `freshIP` stays `in_progress` assertion (Task 2 Step 1). ✓
- Idempotent → `TestReaperIdempotent`. ✓
- Ignores requires_action/terminal → the `WHERE status IN ('in_progress','queued')` filter (Task 2 Step 3). ✓
- Server wiring mirroring cleanup → Task 2 Step 5. ✓

**Placeholder scan:** none. All code is complete. The reaper test's `TestMain` is described as "mirror the endpoints harness" rather than transcribed — that harness (testcontainer Postgres + `applyTenantMigrations`) is an established, copy-adaptable pattern in `assistants_integration_test.go` and `nats_integration_test.go`; the implementer copies it. This is a stated reuse, not a vague requirement.

**Type consistency:** `eventstore.Event`/`eventstore.Append` used identically in Task 1 (def) and Task 2 (reapOnce). `endpoints.Event = eventstore.Event` alias keeps handler + `*_gen.go` literals compiling. `Config`/`RunReaper`/`reapOnce` signatures match between the test (Step 1) and impl (Step 3). Server fields (`reaper`, `reaperDone`) mirror the existing `cleanup`/`cleanupDone` exactly.

**Open risks for the implementer:**
- Task 1: confirm `jsonOrEmpty` stays in `endpoints` (used by `workers.go`) — do NOT delete it there; only `Event`/`appendEvent` move. Grep before deleting.
- Task 2: `started_at` is nullable; the `in_progress` arm requires it non-null-and-old (a queued run has NULL started_at, so it can't match the in_progress arm — the arms are disjoint). Confirm the seed helper sets `started_at` only for `in_progress`.
- Task 2: `reapOnce` must be lowercase and the test in `package reaper` (internal) to call it.

---

## Notes for the implementer

- The 30-min `in_progress` threshold is deliberately > the ~25-min JetStream redelivery window. Do not lower it to a worker-lease timescale — that would fail runs mid-recovery (the design's central constraint).
- Emit `run.failed` ONLY through `eventstore.Append` (+ the single `pg_notify` per tick) — never a bare `UPDATE runs SET status='failed'` without the event, or observers won't see the failure.
- Do not push/PR/merge without explicit approval.
