package endpoints

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestMalformedIdentifiersAreValidationErrors pins the API boundary for the
// thread state/checkpoint/history reads.
//
// These handlers used to interpolate raw path parameters straight into SQL and
// let Postgres do the validating, so every malformed identifier came back as a
// 500 carrying the raw driver text — e.g.
//
//	{"message":"ERROR: invalid input syntax for type bigint: \"not-a-bigint\" (SQLSTATE 22P02)"}
//
// which reports a client error as a server error AND leaks the column type.
//
// 422 is the contract-declared answer: these paths declare exactly "200" and
// "422" in the OpenAPI, so a 400 would be as undeclared as the 500 it replaces.
func TestMalformedIdentifiersAreValidationErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	var tid string
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&tid); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, method, path, body string }{
		{"bad checkpoint id in path", "GET", "/api/v1/threads/" + tid + "/state/not-a-bigint", ""},
		{"checkpoint id out of int64 range", "GET", "/api/v1/threads/" + tid + "/state/99999999999999999999", ""},
		{"bad thread id on get_state", "GET", "/api/v1/threads/not-a-uuid/state", ""},
		{"bad thread id on get_history", "GET", "/api/v1/threads/not-a-uuid/history", ""},
		{"bad thread id on get_checkpoint_state", "GET", "/api/v1/threads/not-a-uuid/state/1", ""},
		{"bad checkpoint id in body", "POST", "/api/v1/threads/" + tid + "/state/checkpoint", `{"checkpoint":{"checkpoint_id":"abc"}}`},
		{"bad thread id on create_checkpoint", "POST", "/api/v1/threads/not-a-uuid/state/checkpoint", `{"checkpoint":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("want 422, got %d: %s", rec.Code, rec.Body.String())
			}
			// The response must not leak the schema or the driver.
			for _, leak := range []string{"SQLSTATE", "invalid input syntax", "bigint", "uuid:", "snapshots"} {
				if strings.Contains(rec.Body.String(), leak) {
					t.Errorf("response leaks internals (%q): %s", leak, rec.Body.String())
				}
			}
		})
	}
}

// TestWellFormedIdentifiersStillResolve is the companion guard: tightening the
// boundary must not turn a legitimate miss into a validation error. A
// well-formed id that simply matches nothing keeps its existing behavior.
func TestWellFormedIdentifiersStillResolve(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	var tid string
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&tid); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, method, path, body string
		want                     int
	}{
		{"unknown but valid checkpoint id", "GET", "/api/v1/threads/" + tid + "/state/424242", "", http.StatusNotFound},
		{"unknown but valid thread id", "GET", "/api/v1/threads/11111111-1111-1111-1111-111111111111/state", "", http.StatusNotFound},
		{"history on a thread with no snapshots", "GET", "/api/v1/threads/" + tid + "/history", "", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			e.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("want %d, got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestWorkerPathParamsAreValidated extends the boundary to the worker and
// checkpoint routes, which kept the raw-parameter pattern after the thread
// state paths were fixed. Every one of these answered 500 with the driver text:
//
//	{"message":"ERROR: invalid input syntax for type uuid: \"not-a-uuid\" (SQLSTATE 22P02)"}
//
// These routes are internal (absent from the public OpenAPI), so no declared
// response set forces the code — 422 is chosen to match the thread paths rather
// than invent a second convention for the same class of mistake.
func TestWorkerPathParamsAreValidated(t *testing.T) {
	ctx := context.Background()
	e := newTestServerWithWorkers()

	var aid string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO assistants (name) VALUES ('pp-worker') RETURNING id`).Scan(&aid); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, method, path, body string }{
		{"heartbeat: malformed worker id", "POST", "/api/v1/workers/not-a-uuid/heartbeat", `{"status":"idle","active_runs":0}`},
		{"deregister: malformed worker id", "POST", "/api/v1/workers/not-a-uuid/deregister", ""},
		{"events: malformed worker id", "POST", "/api/v1/workers/not-a-uuid/runs/" + aid + "/events", `{"events":[]}`},
		{"events: malformed run id", "POST", "/api/v1/workers/" + aid + "/runs/not-a-uuid/events", `{"events":[]}`},
		{"read checkpoint: malformed thread id", "GET", "/api/v1/threads/not-a-uuid/checkpoints/1", ""},
		{"read checkpoint: malformed checkpoint id", "GET", "/api/v1/threads/" + aid + "/checkpoints/not-a-bigint", ""},
		{"load graph: malformed run id", "GET", "/api/v1/workers/runs/not-a-uuid/graph", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("want 422, got %d: %s", rec.Code, rec.Body.String())
			}
			for _, leak := range []string{"SQLSTATE", "invalid input syntax", "bigint", "uuid:", "snapshots", "workers"} {
				if strings.Contains(rec.Body.String(), leak) {
					t.Errorf("response leaks internals (%q): %s", leak, rec.Body.String())
				}
			}
		})
	}
}

// TestAssistantGraphAcceptsGraphIdOrAssistantId covers the one path parameter in
// this group that must NOT be rejected when it isn't a UUID.
//
// The OpenAPI types assistant_id on /assistants/{assistant_id}/graph as
// anyOf[{format: uuid, "Assistant ID"}, {string, "Graph ID"}] — so a non-UUID is
// a legal request selecting the graph by name. The handler interpolated it raw
// into `WHERE assistant_id = $1` (a uuid column), so the legal form 500ed with
// SQLSTATE 22P02. Validating it as a UUID would have been the WRONG fix: it
// would turn a supported request into a 422.
func TestAssistantGraphAcceptsGraphIdOrAssistantId(t *testing.T) {
	ctx := context.Background()
	e := echo.New()
	(&Server{Tenant: testPool}).RegisterAssistants(e.Group("/api/v1"))

	var aid string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO assistants (name) VALUES ('pp-graph') RETURNING id`).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO graphs (assistant_id, name, version, nodes, edges)
		 VALUES ($1, 'named-graph', '1', '[{"id":"A"}]'::jsonb, '[]'::jsonb)`, aid); err != nil {
		t.Fatal(err)
	}

	// Both arms of the anyOf must resolve the SAME graph.
	for _, tc := range []struct{ name, id string }{
		{"by assistant uuid", aid},
		{"by graph name", "named-graph"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/api/v1/assistants/"+tc.id+"/graph", nil)
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"A"`) {
				t.Errorf("want the seeded graph's node, got: %s", rec.Body.String())
			}
		})
	}

	// A name matching nothing is a miss, not a validation error or a 500.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/assistants/no-such-graph/graph", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown graph name: want 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "SQLSTATE") {
		t.Errorf("response leaks the driver error: %s", rec.Body.String())
	}
}
