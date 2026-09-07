package worker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/duragraph/duragraph/controlplane/endpoints"
	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/worker"
)

// TestSingleUserRunEndToEnd is the event-store proof for one user: a run
// created through the PUBLIC HTTP API must reach a worker and complete, driven
// only by the event store and the outbox.
//
// WHY THIS TEST EXISTS. The two pre-existing end-to-end tests
// (TestExecuteRunEndToEnd, TestStreamEndToEnd) both start by hand-publishing a
// run.created ENVELOPE straight onto the RUNS stream — they say so in their own
// comments ("This bypasses the DB events/outbox table for run.created itself").
// That is a fine way to test the dispatcher and the SSE bridge, but it means
// the first link of the chain was never covered: nothing proved that creating a
// run through POST /threads/{id}/runs actually produces an events row, an
// outbox row, a NOTIFY, and ultimately a running worker. Every stage was tested
// except the join between them.
//
// The chain exercised here, with NOTHING synthesized:
//
//	POST /api/v1/threads/{tid}/runs   (real endpoint, real writeTx)
//	  -> events + outbox rows, pg_notify('outbox_new')          [event store]
//	  -> Relay LISTENs, drains the outbox, publishes to RUNS     [relay]
//	  -> RunProcessor turns run.created into worker.graph.execute[dispatch]
//	  -> Runner leases the run and executes the graph            [worker]
//	  -> runs.status = 'completed', execution_history filled     [state]
//
// If any single link is broken this test hangs and then fails, which is the
// point: it is the smallest thing that says "the platform works for one user".
func TestSingleUserRunEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	nc, js, err := dnats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Drain() //nolint:errcheck
	if err := dnats.EnsureConsumers(ctx, js); err != nil {
		t.Fatalf("ensure consumers: %v", err)
	}
	purgeStream(t, ctx, js, "RUNS")
	purgeStream(t, ctx, js, "WORKER_COMMANDS")

	// A thread and an assistant with a real graph. No run is seeded — creating
	// it through the API is the thing under test.
	var tid, aid string
	if err := pool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO assistants (name, graph_id) VALUES ('single-user','counter') RETURNING id`).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	seedCounterGraph(t, ctx, pool, uuid.MustParse(aid), false)

	// The runs API, mounted for real. This is the entry point a user hits.
	e := echo.New()
	(&endpoints.Server{Tenant: pool}).RegisterRuns(e.Group("/api/v1"))
	apiSrv := httptest.NewServer(e)
	defer apiSrv.Close()

	// The real outbox relay: LISTENs on outbox_new, drains the table, publishes
	// each row to NATS. This is the link the other e2e tests skip.
	relay := dnats.NewRelay(
		dnats.NewOutboxDrain(pool), dnats.NewPublisher(js),
		listenerDSNFromPool(), 200*time.Millisecond, 20,
	)
	go func() { _ = relay.Start(ctx) }()
	defer relay.Stop()

	// The dispatcher: run.created -> worker.graph.execute.
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	// A live worker consuming the graph-executor consumer.
	cl := worker.NewClient(serverURL, uuid.New(), nil)
	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := worker.NewRunner(js, cl, dnats.GraphExecutorMaxDeliver)
	go func() { _ = runner.Start(ctx) }()

	// --- the user's actual request ---
	body, _ := json.Marshal(map[string]any{
		"assistant_id": aid,
		"input":        map[string]any{"count": 0},
	})
	resp, err := http.Post(apiSrv.URL+"/api/v1/threads/"+tid+"/runs",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("create run: status %d: %s", resp.StatusCode, buf.String())
	}
	var created struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatal("create response carried no run_id")
	}
	rid := uuid.MustParse(created.RunID)

	// The event store must have recorded the creation BEFORE anything
	// downstream can work. Asserting it separately means a failure downstream
	// is attributable: if this passes and the wait below times out, the break
	// is in the relay or the dispatcher, not in the write.
	var nEvents, nOutbox int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='run.created'`, rid).Scan(&nEvents); err != nil {
		t.Fatal(err)
	}
	if nEvents != 1 {
		t.Fatalf("event store: want exactly 1 run.created event for the run, got %d", nEvents)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE aggregate_id=$1 AND event_type='run.created'`, rid).Scan(&nOutbox); err != nil {
		t.Fatal(err)
	}
	if nOutbox != 1 {
		t.Fatalf("outbox: want exactly 1 mirrored run.created row, got %d", nOutbox)
	}

	// --- and now the whole chain, unaided ---
	waitForRunStatus(t, ctx, pool, rid, "completed", 30*time.Second)

	// The run actually executed rather than merely being marked done.
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 1)

	// The relay must have marked the row published — an outbox row that stays
	// unpublished would be redelivered forever.
	var published bool
	if err := pool.QueryRow(ctx,
		`SELECT published FROM outbox WHERE aggregate_id=$1 AND event_type='run.created'`, rid).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if !published {
		t.Error("outbox row for run.created was never marked published")
	}

	// The full lifecycle is in the event log, which is what makes the run
	// reconstructible: created (API) -> started (worker lease) -> completed.
	for _, want := range []string{"run.created", "run.started", "run.completed"} {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type=$2`, rid, want).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("event log is missing %q for the run", want)
		}
	}
}
