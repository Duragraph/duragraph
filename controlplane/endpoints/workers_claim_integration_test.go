package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// --- claim-test fixtures -----------------------------------------------------

// resetClaimTables truncates everything the claim path touches. Every claim
// test starts from an empty queue so priority/limit assertions mean what they
// say.
func resetClaimTables(t *testing.T, ctx context.Context) {
	t.Helper()
	if _, err := testPool.Exec(ctx,
		"TRUNCATE workers, runs, snapshots, events, outbox, event_streams, assistants, threads CASCADE"); err != nil {
		t.Fatal(err)
	}
}

// seedClaimWorker registers a worker directly (no HTTP) advertising graphs.
func seedClaimWorker(t *testing.T, ctx context.Context, graphs ...string) string {
	t.Helper()
	if graphs == nil {
		graphs = []string{} // workers.graphs is NOT NULL; a nil slice binds as NULL
	}
	wid := uuid.NewString()
	if _, err := testPool.Exec(ctx,
		`INSERT INTO workers (worker_id, graphs, capacity, status, lease_expires_at, last_heartbeat_at)
		 VALUES ($1, $2, 4, 'online', now() + interval '60 seconds', now())`, wid, graphs); err != nil {
		t.Fatal(err)
	}
	return wid
}

// seedClaimAssistant creates an assistant bound to a graph name. runs.graph_id
// is not populated by the create path, so the assistant's graph_id is what the
// claim filter actually resolves through (see claimSQL's COALESCE).
func seedClaimAssistant(t *testing.T, ctx context.Context, graph string) string {
	t.Helper()
	var aid string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO assistants (name, graph_id) VALUES ($1, $1) RETURNING id`, graph).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	return aid
}

// seedQueuedRun inserts a queued run for an assistant at a given priority.
func seedQueuedRun(t *testing.T, ctx context.Context, aid string, priority int) string {
	t.Helper()
	var rid string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO runs (assistant_id, status, priority) VALUES ($1,'queued',$2) RETURNING id`,
		aid, priority).Scan(&rid); err != nil {
		t.Fatal(err)
	}
	return rid
}

// claim POSTs a claim and decodes the response. Fails the test on a non-200.
func claim(t *testing.T, e *echo.Echo, wid string, maxRuns int) WorkerClaimResponse {
	t.Helper()
	rec := doJSON(t, e, http.MethodPost, "/api/v1/workers/"+wid+"/runs/claim",
		`{"max_runs":`+itoa(maxRuns)+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got WorkerClaimResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("claim decode: %v (%s)", err, rec.Body.String())
	}
	return got
}

// --- tests -------------------------------------------------------------------

// TestWorkersClaimLeasesMatchingRun proves the happy path: a queued run whose
// graph the worker advertises is leased to it — status in_progress, worker_id
// set, lease_epoch incremented — and a run.started event lands on the run's own
// aggregate.
func TestWorkersClaimLeasesMatchingRun(t *testing.T) {
	ctx := context.Background()
	resetClaimTables(t, ctx)
	e := newTestServerWithWorkers()

	wid := seedClaimWorker(t, ctx, "counter")
	aid := seedClaimAssistant(t, ctx, "counter")
	rid := seedQueuedRun(t, ctx, aid, 0)

	got := claim(t, e, wid, 5)
	if len(got.Runs) != 1 {
		t.Fatalf("claimed runs: want 1, got %d (%+v)", len(got.Runs), got.Runs)
	}
	cr := got.Runs[0]
	if cr.Run.RunId.String() != rid {
		t.Errorf("claimed run id: want %s, got %s", rid, cr.Run.RunId)
	}
	if cr.LeaseEpoch != 1 {
		t.Errorf("lease_epoch in response: want 1, got %d", cr.LeaseEpoch)
	}
	if cr.CheckpointID != nil {
		t.Errorf("fresh run must have null checkpoint_id, got %d", *cr.CheckpointID)
	}
	// The graph reported is the one the run actually matched on — resolved
	// through the assistant, since runs.graph_id is unpopulated today.
	if cr.GraphID != "counter" {
		t.Errorf("graph_id: want counter, got %q", cr.GraphID)
	}

	// The row itself: leased, to this worker, epoch bumped, started_at stamped.
	var status string
	var workerID *string
	var epoch, version int
	var startedAt *time.Time
	if err := testPool.QueryRow(ctx,
		`SELECT status, worker_id::text, lease_epoch, version, started_at FROM runs WHERE id=$1`, rid).
		Scan(&status, &workerID, &epoch, &version, &startedAt); err != nil {
		t.Fatal(err)
	}
	if status != "in_progress" {
		t.Errorf("run status: want in_progress, got %s", status)
	}
	if workerID == nil || *workerID != wid {
		t.Errorf("run worker_id: want %s, got %v", wid, workerID)
	}
	if epoch != 1 {
		t.Errorf("lease_epoch: want 1 (0+1), got %d", epoch)
	}
	if version != 1 {
		t.Errorf("version: want 1 (0+1), got %d", version)
	}
	if startedAt == nil {
		t.Error("started_at must be stamped on claim")
	}

	// The event must carry the REAL run id as aggregate_id — a synthetic
	// aggregate would strand every later write on this run (checkpoints
	// resolve their stream by aggregate_id).
	if n := countRows(t, ctx,
		`SELECT count(*) FROM events WHERE event_type='run.started' AND aggregate_id=$1 AND aggregate_type='Run'`, rid); n != 1 {
		t.Errorf("run.started events for run: want 1, got %d", n)
	}
	if n := countRows(t, ctx, `SELECT count(*) FROM events WHERE event_type='run.started'`); n != 1 {
		t.Errorf("run.started events overall: want 1 (none on a bogus aggregate), got %d", n)
	}
	if n := countRows(t, ctx, `SELECT count(*) FROM outbox WHERE event_type='run.started' AND aggregate_id=$1`, rid); n != 1 {
		t.Errorf("run.started outbox rows: want 1, got %d", n)
	}
}

// TestWorkersClaimFiltersByGraph proves a run whose graph the worker does NOT
// advertise is left queued — the claim is graph-scoped dispatch, not a queue pop.
func TestWorkersClaimFiltersByGraph(t *testing.T) {
	ctx := context.Background()
	resetClaimTables(t, ctx)
	e := newTestServerWithWorkers()

	wid := seedClaimWorker(t, ctx, "counter")
	mine := seedQueuedRun(t, ctx, seedClaimAssistant(t, ctx, "counter"), 0)
	theirs := seedQueuedRun(t, ctx, seedClaimAssistant(t, ctx, "summarizer"), 0)

	got := claim(t, e, wid, 10)
	if len(got.Runs) != 1 || got.Runs[0].Run.RunId.String() != mine {
		t.Fatalf("want only the counter run %s claimed, got %+v", mine, got.Runs)
	}
	if s := runStatus(t, ctx, theirs); s != "queued" {
		t.Errorf("run for an unadvertised graph must stay queued, got %s", s)
	}
}

// TestWorkersClaimPriorityOrder proves ORDER BY priority DESC, created_at:
// the highest-priority run goes first even though it was queued last.
func TestWorkersClaimPriorityOrder(t *testing.T) {
	ctx := context.Background()
	resetClaimTables(t, ctx)
	e := newTestServerWithWorkers()

	wid := seedClaimWorker(t, ctx, "counter")
	aid := seedClaimAssistant(t, ctx, "counter")
	low := seedQueuedRun(t, ctx, aid, 0)
	high := seedQueuedRun(t, ctx, aid, 10)

	got := claim(t, e, wid, 1)
	if len(got.Runs) != 1 {
		t.Fatalf("claimed runs: want 1, got %d", len(got.Runs))
	}
	if got.Runs[0].Run.RunId.String() != high {
		t.Errorf("want the priority-10 run %s first, got %s", high, got.Runs[0].Run.RunId)
	}
	if s := runStatus(t, ctx, low); s != "queued" {
		t.Errorf("lower-priority run must stay queued, got %s", s)
	}
}

// TestWorkersClaimRespectsMaxRuns proves LIMIT :max_runs bounds one claim, and
// that the remainder stays claimable.
func TestWorkersClaimRespectsMaxRuns(t *testing.T) {
	ctx := context.Background()
	resetClaimTables(t, ctx)
	e := newTestServerWithWorkers()

	wid := seedClaimWorker(t, ctx, "counter")
	aid := seedClaimAssistant(t, ctx, "counter")
	for i := 0; i < 5; i++ {
		seedQueuedRun(t, ctx, aid, 0)
	}

	got := claim(t, e, wid, 2)
	if len(got.Runs) != 2 {
		t.Fatalf("max_runs=2: want 2 claimed, got %d", len(got.Runs))
	}
	if n := countRows(t, ctx, `SELECT count(*) FROM runs WHERE status='queued'`); n != 3 {
		t.Errorf("still-queued runs: want 3, got %d", n)
	}
	// An absent body claims the default of one.
	rec := doJSON(t, e, http.MethodPost, "/api/v1/workers/"+wid+"/runs/claim", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("empty-body claim: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var deflt WorkerClaimResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &deflt)
	if len(deflt.Runs) != defaultClaimBatch {
		t.Errorf("default batch: want %d, got %d", defaultClaimBatch, len(deflt.Runs))
	}
}

// TestWorkersClaimCheckpointID proves the resume contract: checkpoint_id is
// null for a fresh run and carries the LATEST snapshot id for a run that has
// checkpointed.
func TestWorkersClaimCheckpointID(t *testing.T) {
	ctx := context.Background()
	resetClaimTables(t, ctx)
	e := newTestServerWithWorkers()

	wid := seedClaimWorker(t, ctx, "counter")
	aid := seedClaimAssistant(t, ctx, "counter")
	fresh := seedQueuedRun(t, ctx, aid, 0)
	resuming := seedQueuedRun(t, ctx, aid, 0)

	// A run that has checkpointed: two snapshots, so "latest" is provable.
	seedSnapshot(t, ctx, resuming, 1, `{"count":1}`)
	want := seedSnapshot(t, ctx, resuming, 2, `{"count":2}`)

	got := claim(t, e, wid, 10)
	if len(got.Runs) != 2 {
		t.Fatalf("claimed runs: want 2, got %d", len(got.Runs))
	}
	byRun := map[string]*int64{}
	for _, r := range got.Runs {
		byRun[r.Run.RunId.String()] = r.CheckpointID
	}
	if ck, ok := byRun[fresh]; !ok || ck != nil {
		t.Errorf("fresh run checkpoint_id: want null, got %v", ck)
	}
	ck, ok := byRun[resuming]
	if !ok || ck == nil {
		t.Fatalf("resuming run checkpoint_id: want %d, got nil", want)
	}
	if *ck != want {
		t.Errorf("resuming run checkpoint_id: want the latest snapshot %d, got %d", want, *ck)
	}
}

// TestWorkersClaimSkipsLockedRuns proves the FOR UPDATE SKIP LOCKED clause
// directly: with one candidate row locked by another transaction, the claim
// steps over it and takes the next one instead of blocking on it.
//
// The request carries a deadline, so a claim that used a plain FOR UPDATE
// would fail this test (context deadline while waiting on the lock) rather
// than hang the suite.
func TestWorkersClaimSkipsLockedRuns(t *testing.T) {
	ctx := context.Background()
	resetClaimTables(t, ctx)
	e := newTestServerWithWorkers()

	wid := seedClaimWorker(t, ctx, "counter")
	aid := seedClaimAssistant(t, ctx, "counter")
	locked := seedQueuedRun(t, ctx, aid, 10) // highest priority: chosen first...
	free := seedQueuedRun(t, ctx, aid, 0)    // ...unless it is locked away

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var held string
	if err := tx.QueryRow(ctx, `SELECT id FROM runs WHERE id=$1 FOR UPDATE`, locked).Scan(&held); err != nil {
		t.Fatal(err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workers/"+wid+"/runs/claim",
		strings.NewReader(`{"max_runs":5}`)).WithContext(reqCtx)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim with a locked candidate: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got WorkerClaimResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Runs) != 1 || got.Runs[0].Run.RunId.String() != free {
		t.Fatalf("want only the unlocked run %s claimed, got %+v", free, got.Runs)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if s := runStatus(t, ctx, locked); s != "queued" {
		t.Errorf("the locked run must be left queued for the next claim, got %s", s)
	}
}

// TestWorkersClaimConcurrentNoDoubleClaim drives two claims at once against a
// queue of two runs and proves no run is handed to both workers — the property
// SKIP LOCKED exists to guarantee.
func TestWorkersClaimConcurrentNoDoubleClaim(t *testing.T) {
	ctx := context.Background()
	resetClaimTables(t, ctx)
	e := newTestServerWithWorkers()

	w1 := seedClaimWorker(t, ctx, "counter")
	w2 := seedClaimWorker(t, ctx, "counter")
	aid := seedClaimAssistant(t, ctx, "counter")
	seedQueuedRun(t, ctx, aid, 0)
	seedQueuedRun(t, ctx, aid, 0)

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		codes []int
		runs  []string
	)
	start := make(chan struct{})
	for _, wid := range []string{w1, w2} {
		wg.Add(1)
		go func(wid string) {
			defer wg.Done()
			<-start
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/workers/"+wid+"/runs/claim",
				strings.NewReader(`{"max_runs":2}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			e.ServeHTTP(rec, req)
			var got WorkerClaimResponse
			_ = json.Unmarshal(rec.Body.Bytes(), &got)
			mu.Lock()
			defer mu.Unlock()
			codes = append(codes, rec.Code)
			for _, r := range got.Runs {
				runs = append(runs, r.Run.RunId.String())
			}
		}(wid)
	}
	close(start)
	wg.Wait()

	for _, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("concurrent claim: want 200, got %d", code)
		}
	}
	seen := map[string]int{}
	for _, r := range runs {
		seen[r]++
	}
	for rid, n := range seen {
		if n > 1 {
			t.Errorf("run %s was claimed %d times — concurrent claims overlapped", rid, n)
		}
	}
	if len(runs) != 2 || len(seen) != 2 {
		t.Errorf("both queued runs should be claimed exactly once between the two workers, got %v", runs)
	}
	// Each run ended up owned by exactly one worker, with one run.started each.
	if n := countRows(t, ctx, `SELECT count(*) FROM runs WHERE status='in_progress' AND worker_id IS NOT NULL`); n != 2 {
		t.Errorf("in_progress runs: want 2, got %d", n)
	}
	if n := countRows(t, ctx, `SELECT count(*) FROM events WHERE event_type='run.started'`); n != 2 {
		t.Errorf("run.started events: want 2, got %d", n)
	}
}

// TestWorkersClaimBadRequests covers the boundary: a malformed worker id is a
// 422 from pathUUIDString (never interpolated into the query), and an
// unregistered worker is a 409 telling it to re-register.
func TestWorkersClaimBadRequests(t *testing.T) {
	ctx := context.Background()
	resetClaimTables(t, ctx)
	e := newTestServerWithWorkers()

	rec := doJSON(t, e, http.MethodPost, "/api/v1/workers/not-a-uuid/runs/claim", `{"max_runs":1}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("malformed worker id: want 422, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "SQLSTATE") {
		t.Errorf("422 body must not leak a driver error: %s", rec.Body.String())
	}

	rec = doJSON(t, e, http.MethodPost, "/api/v1/workers/"+uuid.NewString()+"/runs/claim", `{"max_runs":1}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("unregistered worker: want 409, got %d: %s", rec.Code, rec.Body.String())
	}

	// A registered worker advertising no graphs claims nothing, successfully.
	wid := seedClaimWorker(t, ctx)
	seedQueuedRun(t, ctx, seedClaimAssistant(t, ctx, "counter"), 0)
	got := claim(t, e, wid, 5)
	if len(got.Runs) != 0 {
		t.Errorf("worker with no graphs: want 0 claimed, got %d", len(got.Runs))
	}
}
