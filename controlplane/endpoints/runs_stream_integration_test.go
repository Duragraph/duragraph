package endpoints

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duragraph/duragraph/controlplane/nats"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go/jetstream"
)

// sseFrame is one decoded Server-Sent Event (event + data lines).
type sseFrame struct{ Event, Data string }

// readSSE connects to url, reads Server-Sent frames until `want` frames arrive
// or the deadline passes, and returns whatever was collected. Uses a real HTTP
// client (streaming) — httptest.NewRecorder buffers and can't do this.
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

// seedRunWithStream creates an assistant, a stateless run, and the run's
// event_streams row (aggregate_type='Run', aggregate_id=<run id>) so
// insertEvent can append catch-up events against it. Returns the run's uuid.
func seedRunWithStream(t *testing.T, ctx context.Context) uuid.UUID {
	t.Helper()
	var assistantID uuid.UUID
	if err := testPool.QueryRow(ctx,
		`INSERT INTO assistants (graph_id, name) VALUES ('hello_world', 'stream-test') RETURNING id`,
	).Scan(&assistantID); err != nil {
		t.Fatalf("seed assistant: %v", err)
	}
	var runID uuid.UUID
	if err := testPool.QueryRow(ctx,
		`INSERT INTO runs (assistant_id, status) VALUES ($1, 'in_progress') RETURNING id`, assistantID,
	).Scan(&runID); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO event_streams (aggregate_type, aggregate_id, version) VALUES ('Run', $1, 0)`, runID,
	); err != nil {
		t.Fatalf("seed event_stream: %v", err)
	}
	return runID
}

// insertEvent appends one row directly to the events table (as if already
// relayed/emitted), for a run seeded by seedRunWithStream. Returns the
// generated event_id so callers (e.g. the dedup test) can republish it live.
func insertEvent(t *testing.T, ctx context.Context, runID uuid.UUID, version int, eventType, payload string) uuid.UUID {
	t.Helper()
	var streamID uuid.UUID
	if err := testPool.QueryRow(ctx,
		`SELECT stream_id FROM event_streams WHERE aggregate_type='Run' AND aggregate_id=$1`, runID,
	).Scan(&streamID); err != nil {
		t.Fatalf("insertEvent: lookup stream: %v", err)
	}
	eventID := uuid.New()
	if _, err := testPool.Exec(ctx, `
		INSERT INTO events (event_id, stream_id, aggregate_type, aggregate_id, event_type, event_version, payload)
		VALUES ($1, $2, 'Run', $3, $4, $5, $6)`,
		eventID, streamID, runID, eventType, version, payload,
	); err != nil {
		t.Fatalf("insertEvent: %v", err)
	}
	return eventID
}

// envelopeFor builds the relay envelope shape (nats/relay.go envelope()) for
// a live publish in tests, with a fresh event_id.
func envelopeFor(rid uuid.UUID, eventType, payload string) map[string]any {
	var p any
	_ = json.Unmarshal([]byte(payload), &p)
	return map[string]any{
		"event_id":       uuid.NewString(),
		"aggregate_type": "Run",
		"aggregate_id":   rid.String(),
		"event_type":     eventType,
		"payload":        p,
	}
}

// mustJS returns a JetStream context over the shared embedded testNATS
// connection, for tests that publish live events via nats.Publisher.
func mustJS(t *testing.T) jetstream.JetStream {
	t.Helper()
	js, err := jetstream.New(testNATS)
	if err != nil {
		t.Fatalf("mustJS: %v", err)
	}
	return js
}

// newStreamTestServer mounts the runs group with a live Subscriber over the
// shared embedded testNATS connection.
func newStreamTestServer() *Server {
	return &Server{Tenant: testPool, Subscriber: nats.NewSubscriberFromConn(testNATS)}
}

// TestStreamPerRunCatchUpAndLive proves the central lossless property:
// subscribe-first, then catch-up (persisted events), then live (deduped by
// event_id), closing on the run's terminal event.
func TestStreamPerRunCatchUpAndLive(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, events, outbox, event_streams, assistants CASCADE"); err != nil {
		t.Fatal(err)
	}
	// seed a run + two catch-up events (as if already emitted)
	rid := seedRunWithStream(t, ctx) // helper: assistant + run + event_stream, returns run uuid
	insertEvent(t, ctx, rid, 1, "run.started", `{"n":1}`)
	insertEvent(t, ctx, rid, 2, "execution.node_completed", `{"node":"A"}`)

	e := echo.New()
	s := newStreamTestServer()
	s.RegisterRuns(e.Group("/api/v1"))
	srv := httptest.NewServer(e)
	defer srv.Close()

	url := srv.URL + "/api/v1/threads/" + uuid.Nil.String() + "/runs/" + rid.String() + "/stream"
	// Read in a goroutine; publish live + terminal after the stream is up.
	done := make(chan []sseFrame, 1)
	go func() { done <- readSSE(t, url, 4, 5*time.Second) }()
	time.Sleep(300 * time.Millisecond)  // let subscribe+catchup run
	pub := nats.NewPublisher(mustJS(t)) // JetStream publisher on testNATS
	_ = pub.PublishWithID(ctx, nats.SubjectFor("execution.node_completed"), uuid.NewString(), envelopeFor(rid, "execution.node_completed", `{"node":"B"}`))
	_ = pub.PublishWithID(ctx, nats.SubjectFor("run.completed"), uuid.NewString(), envelopeFor(rid, "run.completed", `{}`))

	frames := <-done
	// Expect 4: 2 catch-up (run.started, node_completed A) + 2 live (node_completed B, run.completed)
	if len(frames) != 4 {
		t.Fatalf("want 4 frames, got %d: %+v", len(frames), frames)
	}
	if frames[0].Event != "run.started" || frames[3].Event != "run.completed" {
		t.Errorf("frame order wrong: %+v", frames)
	}
}

// TestStreamRequiresNATS proves the SSE endpoint 503s immediately when the
// server has no Subscriber (NATS disabled) — no DB round-trip, no streaming.
func TestStreamRequiresNATS(t *testing.T) {
	e := echo.New()
	s := &Server{Tenant: testPool}
	s.RegisterRuns(e.Group("/api/v1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/threads/"+uuid.Nil.String()+"/runs/"+uuid.New().String()+"/stream", nil)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestStreamDedup proves an event whose event_id is present in BOTH the
// catch-up events row and a subsequently-published live envelope is emitted
// only once — the seen-by-event_id dedup guard.
func TestStreamDedup(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, events, outbox, event_streams, assistants CASCADE"); err != nil {
		t.Fatal(err)
	}
	rid := seedRunWithStream(t, ctx)
	dupID := insertEvent(t, ctx, rid, 1, "run.started", `{"n":1}`)

	e := echo.New()
	s := newStreamTestServer()
	s.RegisterRuns(e.Group("/api/v1"))
	srv := httptest.NewServer(e)
	defer srv.Close()

	url := srv.URL + "/api/v1/threads/" + uuid.Nil.String() + "/runs/" + rid.String() + "/stream"
	// Only 2 frames are ever legitimate here: the catch-up run.started, and
	// the live run.completed. If dedup is broken, the duplicated run.started
	// would arrive as a THIRD frame before run.completed, and frames[1] would
	// be the dup (not run.completed) — readSSE stops at `want` frames, so a
	// dedup failure surfaces as a wrong frames[1].Event below.
	done := make(chan []sseFrame, 1)
	go func() { done <- readSSE(t, url, 2, 5*time.Second) }()
	time.Sleep(300 * time.Millisecond)

	pub := nats.NewPublisher(mustJS(t))
	// Republish the SAME event_id as the catch-up row — must be deduped.
	dupEnvelope := map[string]any{
		"event_id":       dupID.String(),
		"aggregate_type": "Run",
		"aggregate_id":   rid.String(),
		"event_type":     "run.started",
		"payload":        map[string]any{"n": 1},
	}
	_ = pub.PublishWithID(ctx, nats.SubjectFor("run.started"), uuid.NewString(), dupEnvelope)
	// A genuine new terminal event, with its own event_id.
	_ = pub.PublishWithID(ctx, nats.SubjectFor("run.completed"), uuid.NewString(), envelopeFor(rid, "run.completed", `{}`))

	frames := <-done
	if len(frames) != 2 {
		t.Fatalf("want 2 frames (catch-up run.started + live run.completed; dup dropped), got %d: %+v", len(frames), frames)
	}
	if frames[0].Event != "run.started" {
		t.Errorf("frame 0: want run.started (catch-up), got %+v", frames[0])
	}
	if frames[1].Event != "run.completed" {
		t.Errorf("frame 1: want run.completed (live terminal, dup dropped), got %+v", frames[1])
	}
}
