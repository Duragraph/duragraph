package worker_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duragraph/duragraph/controlplane/endpoints"
	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/worker"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// sseFrame is one decoded Server-Sent Event (event + data lines). Mirrors the
// endpoints package's own (unexported there) helper of the same name —
// duplicated at the small size actually needed here rather than exporting it
// across packages for one test.
type sseFrame struct{ Event, Data string }

// readSSE connects to url, reads Server-Sent frames until `want` frames
// arrive or the deadline passes, and returns whatever was collected. Uses a
// real HTTP client (streaming) — httptest.NewRecorder buffers and can't do
// this.
func readSSE(t *testing.T, url string, want int, deadline time.Duration) []sseFrame {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sse connect: %v", err)
	}
	frames := make(chan sseFrame, 32)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		var ev, data string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "":
				if ev != "" {
					frames <- sseFrame{Event: ev, Data: data}
					ev, data = "", ""
				}
			}
		}
	}()
	var got []sseFrame
	timeout := time.After(deadline)
	for len(got) < want {
		select {
		case f := <-frames:
			got = append(got, f)
		case <-timeout:
			return got
		}
	}
	return got
}

// listenerDSNFromPool returns a bare pgx URL pointing at the shared
// testcontainer's Postgres. The relay's LISTEN connection must NOT use a pool
// (LISTEN requires session affinity that PgBouncer transaction-pooling would
// drop) — mirrors controlplane/nats/nats_integration_test.go's helper of the
// same name.
func listenerDSNFromPool() string {
	cc := testPool.Config().ConnConfig
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cc.User, cc.Password, cc.Host, cc.Port, cc.Database,
	)
}

// TestStreamEndToEnd is the streaming integration proof: a real outbox relay
// (DB -> NATS), run-processor (run.created -> worker.graph.execute), and a
// live Runner executing the 2-step counter graph, all feeding the SSE bridge
// (controlplane/endpoints RunsStreamPerRun) over a live Subscriber. The SSE
// client must see the real event sequence a worker execution actually
// produces: run.started, two execution.node_completed (nodes A and B), then
// run.completed — thin passthrough frames, sourced from actual DB events
// relayed onto NATS, not synthesized. (run.created itself is triggered by a
// direct publish that never touches the DB events/outbox table — mirroring
// TestExecuteRunEndToEnd's run-processor trigger — and the SSE connection is
// opened only after that publish completes, so it is deliberately excluded:
// see the comment at the publish site below.)
//
// Lands in controlplane/worker (package worker_test) rather than
// controlplane/endpoints: this package already has the full worker e2e
// harness (testPool/natsURL/serverURL + newPool/seedThreadAssistantRun/
// purgeStream from worker_testmain_test.go and the run-processor+Runner
// wiring pattern from execution_integration_test.go's
// TestExecuteRunEndToEnd). Reusing it here only costs mounting a second
// httptest server (the runs SSE group) plus one real outbox Relay — far
// cheaper than porting testcontainers Postgres + embedded NATS + the
// run-processor/Runner wiring into the endpoints package's test harness,
// which has no worker-side pieces at all.
func TestStreamEndToEnd(t *testing.T) {
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

	_, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	// The worker's LoadGraph fetches the graph registered for the run's
	// assistant, so seed the counter graph on that assistant.
	seedCounterGraph(t, ctx, pool, aid, false)

	// run-processor: turns run.created into a worker.graph.execute command.
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	// A live worker + Runner consuming the graph-executor consumer, driving
	// the real counter graph (nodes A, B) via HTTP calls to the worker
	// endpoints — each call (leaseRun/nodeEvent/terminalRun) writes real
	// events + outbox rows on pool via writeTx.
	wid := uuid.New()
	cl := worker.NewClient(serverURL, wid, nil)
	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := worker.NewRunner(js, cl, dnats.GraphExecutorMaxDeliver)
	go func() { _ = runner.Start(ctx) }()

	// The real outbox relay: drains pool's outbox table and publishes each
	// row to NATS, exactly as production does — without this, the worker's
	// events would land in the DB but never reach the SSE bridge's live feed.
	drain := dnats.NewOutboxDrain(pool)
	relay := dnats.NewRelay(drain, dnats.NewPublisher(js), listenerDSNFromPool(), 200*time.Millisecond, 20)
	go func() { _ = relay.Start(ctx) }()
	defer relay.Stop()

	// Kick off the pipeline: publish the real relay envelope shape for
	// run.created onto RUNS (mirrors TestExecuteRunEndToEnd — the run id is
	// the envelope's aggregate_id, not a top-level run_id). This bypasses the
	// DB events/outbox table for run.created itself (it's never written
	// there, so it can never appear in the SSE catch-up query), and
	// PublishWithID blocks for the JetStream ack — so by the time this
	// returns, the publish is already complete. The SSE client's core-NATS
	// Subscribe (below) only ever sees messages published AFTER it
	// subscribes (no backlog for a fresh core subscription), so opening the
	// SSE connection only now guarantees it never observes this run.created
	// publish either — exactly matching the brief's expected sequence
	// (run.started onward, no run.created).
	envelope := map[string]any{
		"event_id":       uuid.New().String(),
		"aggregate_type": "Run",
		"aggregate_id":   rid.String(),
		"event_type":     "run.created",
		"payload":        map[string]any{},
		"metadata":       map[string]any{},
	}
	if err := dnats.NewPublisher(js).PublishWithID(ctx, dnats.SubjectFor("run.created"), rid.String(), envelope); err != nil {
		t.Fatalf("publish run.created: %v", err)
	}

	// The SSE bridge itself: the runs group mounted on its own httptest
	// server, with a live Subscriber over the same embedded NATS connection.
	// Opened only after the run.created publish above (see comment).
	e := echo.New()
	(&endpoints.Server{Tenant: pool, Subscriber: dnats.NewSubscriberFromConn(nc)}).RegisterRuns(e.Group("/api/v1"))
	sseSrv := httptest.NewServer(e)
	defer sseSrv.Close()

	url := sseSrv.URL + "/api/v1/threads/" + uuid.Nil.String() + "/runs/" + rid.String() + "/stream"
	done := make(chan []sseFrame, 1)
	go func() { done <- readSSE(t, url, 4, 15*time.Second) }()

	frames := <-done
	want := []string{"run.started", "execution.node_completed", "execution.node_completed", "run.completed"}
	if len(frames) != len(want) {
		t.Fatalf("want %d frames, got %d: %+v", len(want), len(frames), frames)
	}
	for i, w := range want {
		if frames[i].Event != w {
			t.Errorf("frame %d: want %s, got %s (full: %+v)", i, w, frames[i].Event, frames)
		}
	}

	waitForRunStatus(t, ctx, pool, rid, "completed", 10*time.Second)
}
