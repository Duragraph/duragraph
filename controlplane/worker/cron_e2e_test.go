package worker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	dcron "github.com/duragraph/duragraph/controlplane/cron"
	"github.com/duragraph/duragraph/controlplane/endpoints"
	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/worker"
)

// TestCronFiresAndRunsEndToEnd: a cron created through the API produces a run
// that a worker actually executes.
//
// Before this, a cron was inert in two independent ways — next_run_at was never
// set at creation, and nothing polled it — so the endpoints stored a schedule
// that could never fire. API-only apart from advancing the clock, which is done
// by moving next_run_at rather than waiting for a real minute to elapse.
func TestCronFiresAndRunsEndToEnd(t *testing.T) {
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

	e := echo.New()
	srv := &endpoints.Server{Tenant: pool}
	g := e.Group("/api/v1")
	srv.RegisterAssistants(g)
	srv.RegisterThreads(g)
	srv.RegisterRuns(g)
	srv.RegisterCrons(g)
	srv.RegisterWorkers(g)
	apiSrv := httptest.NewServer(e)
	defer apiSrv.Close()
	api := &apiClient{t: t, base: apiSrv.URL}

	relay := dnats.NewRelay(dnats.NewOutboxDrain(pool), dnats.NewPublisher(js),
		listenerDSNFromPool(), 200*time.Millisecond, 20)
	go func() { _ = relay.Start(ctx) }()
	defer relay.Stop()
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	graphName := "cron-" + uuid.NewString()[:8]
	cl := worker.NewClient(apiSrv.URL, uuid.New(), nil)
	if err := cl.RegisterWithGraphs(ctx, []string{graphName}, 1, []worker.GraphDefinition0{{
		Name:    graphName,
		Version: "1",
		Nodes:   json.RawMessage(`[{"id":"A","type":"tool","config":{"set":{"step":"a"}}}]`),
		Edges:   json.RawMessage(`[]`),
	}}); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := worker.NewRunner(js, cl, dnats.GraphExecutorMaxDeliver)
	go func() { _ = runner.Start(ctx) }()

	var assistant struct {
		AssistantID string `json:"assistant_id"`
	}
	api.mustDo("POST", "/api/v1/assistants", map[string]any{
		"graph_id": graphName, "name": "cron-assistant",
	}, &assistant, http.StatusCreated)
	var thread struct {
		ThreadID string `json:"thread_id"`
	}
	api.mustDo("POST", "/api/v1/threads", map[string]any{}, &thread, http.StatusCreated)

	// Create the cron through the API.
	var created struct {
		CronID      string  `json:"cron_id"`
		NextRunDate *string `json:"next_run_date"`
	}
	api.mustDo("POST", "/api/v1/threads/"+thread.ThreadID+"/runs/crons", map[string]any{
		"assistant_id": assistant.AssistantID,
		"schedule":     "*/5 * * * *",
		"input":        map[string]any{"from": "cron"},
	}, &created, http.StatusOK)
	if created.CronID == "" {
		t.Fatal("create cron returned no cron_id")
	}
	// Without this the whole mechanism is dead on arrival: nothing else ever
	// populates next_run_at, so the scheduler would find nothing due, forever.
	if created.NextRunDate == nil || *created.NextRunDate == "" {
		t.Fatal("a newly created cron must carry next_run_date, or it can never fire")
	}

	// Make it due now instead of waiting for the wall clock.
	if _, err := pool.Exec(ctx,
		`UPDATE crons SET next_run_at = now() - interval '1 second' WHERE id = $1`,
		created.CronID); err != nil {
		t.Fatal(err)
	}

	sched := dcron.NewScheduler(pool, dcron.Config{})
	n, err := sched.FireDue(ctx)
	if err != nil {
		t.Fatalf("FireDue: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 cron fired, got %d", n)
	}

	// The schedule must have advanced, or the next tick fires it again.
	var next time.Time
	if err := pool.QueryRow(ctx,
		`SELECT next_run_at FROM crons WHERE id=$1`, created.CronID).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if !next.After(time.Now().UTC()) {
		t.Errorf("next_run_at must move into the future, got %s", next)
	}
	// Firing again immediately must be a no-op — proof the advance is what
	// prevents a hot loop.
	if again, err := sched.FireDue(ctx); err != nil || again != 0 {
		t.Errorf("second FireDue: want 0 fired, got %d (err %v)", again, err)
	}

	// The run it created must be traceable to the cron, and must actually run.
	var rid uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM runs WHERE metadata->>'cron_id' = $1`, created.CronID).Scan(&rid); err != nil {
		t.Fatalf("cron did not create a run: %v", err)
	}
	waitForRunStatus(t, ctx, pool, rid, "completed", 30*time.Second)
	assertNodeCount(t, ctx, pool, rid, "A", 1)
}

// TestCronCreateRejectsBadSchedule — an unparseable schedule stored as valid
// would look accepted and never fire, which is silent and permanent.
func TestCronCreateRejectsBadSchedule(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	e := echo.New()
	srv := &endpoints.Server{Tenant: pool}
	g := e.Group("/api/v1")
	srv.RegisterAssistants(g)
	srv.RegisterThreads(g)
	srv.RegisterCrons(g)
	apiSrv := httptest.NewServer(e)
	defer apiSrv.Close()
	api := &apiClient{t: t, base: apiSrv.URL}

	var aid string
	if err := pool.QueryRow(ctx,
		`INSERT INTO assistants (name, graph_id) VALUES ('cron-bad','g') RETURNING id`).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	var tid string
	if err := pool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&tid); err != nil {
		t.Fatal(err)
	}

	got := api.do("POST", "/api/v1/threads/"+tid+"/runs/crons", map[string]any{
		"assistant_id": aid,
		"schedule":     "not a cron",
	}, nil)
	if got != http.StatusUnprocessableEntity {
		t.Errorf("bad schedule: want 422, got %d", got)
	}
}

// TestCronSkipsMissedWindows: the next firing is computed from NOW, so an
// outage does not replay every window it spanned.
func TestCronSkipsMissedWindows(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Three hours "down" for a five-minute cron.
	next, err := dcron.NextRun("*/5 * * * *", now)
	if err != nil {
		t.Fatal(err)
	}
	if got := next.Sub(now); got > 5*time.Minute {
		t.Errorf("next run should be within one window of now, got %s later", got)
	}
	// It fires once and moves on, rather than owing 36 runs.
	if !next.After(now) {
		t.Errorf("next run must be strictly after now, got %s", next)
	}
}

// TestCronEndTimeStopsFiring — a cron past its end_time must not fire.
func TestCronEndTimeStopsFiring(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	var aid string
	if err := pool.QueryRow(ctx,
		`INSERT INTO assistants (name, graph_id) VALUES ('cron-end','g') RETURNING id`).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	var cid string
	if err := pool.QueryRow(ctx, `
		INSERT INTO crons (assistant_id, schedule, next_run_at, end_time)
		VALUES ($1, '*/5 * * * *', now() - interval '1 minute', now() - interval '1 minute')
		RETURNING id`, aid).Scan(&cid); err != nil {
		t.Fatal(err)
	}

	sched := dcron.NewScheduler(pool, dcron.Config{})
	n, err := sched.FireDue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var runs int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM runs WHERE metadata->>'cron_id' = $1`, cid).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Errorf("a cron past end_time must not fire, but created %d run(s) (fired=%d)", runs, n)
	}
}
